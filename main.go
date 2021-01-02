package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httputil"
	"strconv"

	rpio "github.com/stianeikeland/go-rpio"
	"github.com/xanderflood/pihub/pkg/gpio"
	"github.com/xanderflood/pihub/pkg/htg3535ch"
)

type Blob interface{}

type JSONHack struct {
	Blob
}

func (j JSONHack) Get(path ...string) (s interface{}, ok bool) {
	defer func() {
		if r := recover(); r != nil {
			s = nil
			ok = false
		}
	}()

	return j.GetUnsafe(path...), true
}

func (j JSONHack) GetUnsafe(path ...string) (s interface{}) {
	ref := j.Blob
	for _, seg := range path {
		ref = ref.(map[string]interface{})[seg]
	}

	return ref
}

type Module interface {
	Initialize(j JSONHack) error
	Act(action string, config JSONHack) (JSONHack, error)
}

type InitializeRequest struct {
	Modules map[string]ModuleSpec `json:"modules"`
}
type ModuleSpec struct {
	Source string   `json:"source"`
	Config JSONHack `json:"config"`
}
type InitializeResponse struct {
	NumModules int `json:"num_modules"`
}

type GetStateRequest struct {
	Module string   `json:"module"`
	Path   []string `json:"path"`
}
type GetStateResponse struct {
	Value JSONHack `json:"num_modules"`
}

type SetStateRequest struct {
	Module string   `json:"module"`
	Path   []string `json:"path"`
	Value  JSONHack `json:"num_modules"`
}

type ActRequest struct {
	Module string      `json:"module"`
	Action string      `json:"action"`
	Config interface{} `json:"config"`
}
type ActResponse struct {
	Result JSONHack `json:"result"`
}

type ManagerAgent struct {
	Modules map[string]Module
}

type ErrorResponse struct {
	Message string `json:"message"`
}

type Manager interface {
	InitializeModules(specs map[string]ModuleSpec) error
	Act(module string, action string, config JSONHack) (JSONHack, error)
}

type ModuleFactory func() Module

var ModuleIndex = map[string]ModuleFactory{
	"echo":      func() Module { return &EchoModule{} },
	"relay":     func() Module { return &RelayModule{} },
	"htg3535ch": func() Module { return &HTGModule{} },

	// TODO add some more modules!
}

func (a *ManagerAgent) InitializeModules(specs map[string]ModuleSpec) error {
	for name, spec := range specs {
		if factory, ok := ModuleIndex[spec.Source]; ok {
			a.Modules[name] = factory()
			if err := a.Modules[name].Initialize(spec.Config); err != nil {
				return errors.New("failed to initialize module") // TODO 500
			}
		} else {
			fmt.Println("404 no such module")
		}
	}
	return nil
}
func (a *ManagerAgent) Act(module string, action string, config JSONHack) (JSONHack, error) {
	if mod, ok := a.Modules[module]; ok {
		return mod.Act(action, config)
	}
	return JSONHack{}, errors.New("no such module") // TODO 404
}

func main() {
	router := buildMux()

	http.ListenAndServe("0.0.0.0:3141", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if bs, err := httputil.DumpRequest(r, true); err != nil {
			fmt.Println("failed dumping request -- aborting", err.Error())
			return
		} else {
			fmt.Println("---DUMPING REQUEST ---")
			fmt.Println(string(bs))
		}

		router.ServeHTTP(w, r)
	}))
}

func buildMux() *http.ServeMux {
	mgr := &ManagerAgent{
		Modules: map[string]Module{},
	}

	mux := http.NewServeMux()
	mux.Handle("/initialize", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		var req InitializeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			fmt.Println("failed decoding body", err.Error())
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		if err := mgr.InitializeModules(req.Modules); err != nil {
			fmt.Println("failed initializing modules", err.Error())
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		json.NewEncoder(w).Encode(InitializeResponse{NumModules: len(mgr.Modules)})

		return
	}))
	mux.Handle("/act", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		var req ActRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			fmt.Println("failed decoding body", err.Error())
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		if result, err := mgr.Act(req.Module, req.Action, JSONHack{Blob: req.Config}); err != nil {
			fmt.Println("action failed", err.Error())
			w.WriteHeader(http.StatusInternalServerError)
			return
		} else {
			json.NewEncoder(w).Encode(ActResponse{Result: result})
		}
	}))

	return mux
}

type EchoModule struct{}

func (e *EchoModule) Initialize(j JSONHack) error {
	return nil
}
func (e *EchoModule) Act(action string, config JSONHack) (JSONHack, error) {
	return JSONHack{
		Blob: map[string]interface{}{
			"action": action,
			"config": config.Blob,
		},
	}, nil
}

type RelayModule struct {
	pin gpio.OutputPin
}

func (m *RelayModule) Initialize(j JSONHack) error {
	if err := rpio.Open(); err != nil {
		return err
	}

	s := fmt.Sprintf("%.0f", j.GetUnsafe("pin").(float64))
	pin, _ := strconv.Atoi(s)
	m.pin = rpio.Pin(uint8(pin))
	return nil
}
func (m *RelayModule) Act(action string, config JSONHack) (JSONHack, error) {
	switch action {
	case "set":
		if state, ok := config.Get("state"); ok {
			gpio.Set(m.pin, state.(string) == "high")
			return JSONHack{}, nil
		}

		return JSONHack{}, errors.New("`state` is a required field for the `set` action")
	default:
		return JSONHack{}, fmt.Errorf("no such action `%s`", action)
	}
}

type HTGModule struct {
	tk htg3535ch.TemperatureK
	rh htg3535ch.Humidity
}

func (m *HTGModule) Initialize(j JSONHack) error {
	if err := rpio.Open(); err != nil {
		return err
	}

	s := fmt.Sprintf("%.0f", j.GetUnsafe("temperature_adc_channel").(float64))
	tempCh, _ := strconv.Atoi(s)
	s = fmt.Sprintf("%.0f", j.GetUnsafe("humidity_adc_channel").(float64))
	humCh, _ := strconv.Atoi(s)
	// s = fmt.Sprintf("%.0f", j.GetUnsafe("calibration_adc_channel").(float64))
	// calCh, _ := strconv.Atoi(s)

	// TODO if this provides reasonable values, fix the calibration wiring and try
	// using NewCalibrationTemperatureK again
	m.tk = htg3535ch.NewDefaultTemperatureK(tempCh)
	m.rh = htg3535ch.NewHumidity(humCh)
	return nil
}
func (m *HTGModule) Act(action string, config JSONHack) (JSONHack, error) {
	switch action {
	case "rh":
		val, err := m.rh.Read()
		return JSONHack{Blob: val}, err
	case "tk":
		val, err := m.tk.Read()
		return JSONHack{Blob: val}, err
	case "tc":
		val, err := m.tk.Read()
		return JSONHack{Blob: val - 273.15}, err
	case "tf":
		val, err := m.tk.Read()
		return JSONHack{Blob: (val-273.15)*9/5 + 32}, err
	default:
		return JSONHack{}, fmt.Errorf("no such action `%s`", action)
	}
}
