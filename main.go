package main

import (
	"log"

	"periph.io/x/periph/conn/i2c"
	"periph.io/x/periph/conn/i2c/i2creg"
	"periph.io/x/periph/conn/physic"
	"periph.io/x/periph/experimental/conn/analog"
	"periph.io/x/periph/experimental/devices/ads1x15"
	"periph.io/x/periph/host"

	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httputil"
	"strconv"

	"github.com/xanderflood/pihub/pkg/gpio"
	"github.com/xanderflood/pihub/pkg/htg3535ch"
)

////////////////////////
// The module runtime //
type Module interface {
	// TODO documentation endpoint

	Initialize(p ServiceProvider, j JSONHack) error
	Act(action string, config JSONHack) (JSONHack, error)
	Stop() error
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
	"i2c":       func() Module { return &I2CModule{} },
	"ads":       func() Module { return &ADS1115Module{} },
}

func (a *ManagerAgent) InitializeModules(specs map[string]ModuleSpec) error {
	for name, spec := range specs {
		if factory, ok := ModuleIndex[spec.Source]; ok {
			a.Modules[name] = factory()
			if err := a.Modules[name].Initialize(a.ServiceProvider, spec.Config); err != nil {
				return fmt.Errorf("failed to initialize module: %w", err)
			}
		} else {
			return fmt.Errorf("404 no such module source: %s", spec.Source)
		}
	}
	return nil
}
func (a *ManagerAgent) Act(module string, action string, config JSONHack) (JSONHack, error) {
	if mod, ok := a.Modules[module]; ok {
		return mod.Act(action, config)
	}
	return nil, errors.New("no such module") // TODO 404
}

////////////////
// HTTP Logic //
type JSONHack interface{}

func Get(ref interface{}, path ...string) (s interface{}, ok bool) {
	defer func() {
		if r := recover(); r != nil {
			s = nil
			ok = false
		}
	}()

	return GetUnsafe(ref, path...), true
}

func GetUnsafe(ref interface{}, path ...string) (s interface{}) {
	for _, seg := range path {
		ref = ref.(map[string]interface{})[seg]
	}

	return ref
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
	Modules         map[string]Module
	ServiceProvider ServiceProvider
}

type ErrorResponse struct {
	Message string `json:"message"`
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
	sp, err := NewServiceProvider()
	if err != nil {
		log.Fatal("failed initializing service provider")
	}

	mgr := &ManagerAgent{
		Modules:         map[string]Module{},
		ServiceProvider: sp,
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

		if result, err := mgr.Act(req.Module, req.Action, req.Config); err != nil {
			fmt.Println("action failed", err.Error())
			w.WriteHeader(http.StatusInternalServerError)
			return
		} else {
			json.NewEncoder(w).Encode(ActResponse{Result: result})
		}
	}))

	return mux
}

////////////////////////
// The module library //
type EchoModule struct{}

func (*EchoModule) Stop() error { return nil }

func (e *EchoModule) Initialize(sp ServiceProvider, config JSONHack) error {
	return nil
}
func (e *EchoModule) Act(action string, config JSONHack) (JSONHack, error) {
	return map[string]interface{}{
		"action": action,
		"config": config,
	}, nil
}

type RelayModule struct {
	pin gpio.OutputPin
}

func (*RelayModule) Stop() error { return nil }

func (m *RelayModule) Initialize(sp ServiceProvider, config JSONHack) error {
	s := fmt.Sprintf("%.0f", GetUnsafe(config, "pin").(float64))
	pin, _ := strconv.Atoi(s)

	var err error
	m.pin, err = sp.GetGPIOPin(uint8(pin))
	return err
}
func (m *RelayModule) Act(action string, config JSONHack) (JSONHack, error) {
	switch action {
	case "set":
		if state, ok := Get(config, "state"); ok {
			gpio.Set(m.pin, state.(string) == "high")
			return nil, nil
		}

		return nil, errors.New("`state` is a required field for the `set` action")
	default:
		return nil, fmt.Errorf("no such action `%s`", action)
	}
}

type I2CModule struct {
	dvc I2CDevice
}

func (*I2CModule) Stop() error { return nil }

func (m *I2CModule) Initialize(sp ServiceProvider, config JSONHack) error {
	s := fmt.Sprintf("%.0f", GetUnsafe(config, "address").(float64))
	addr, _ := strconv.Atoi(s)

	var err error
	bus, err := sp.GetDefaultI2CBus()
	m.dvc = &i2c.Dev{Bus: bus, Addr: uint16(addr)}
	if err != nil {
		return fmt.Errorf("failed getting i2c device: %w", err)
	}

	return nil
}
func (m *I2CModule) Act(action string, config JSONHack) (JSONHack, error) {
	switch action {
	case "transact":
		var msg []byte
		if bytesI, ok := Get(config, "bytes"); ok {
			bytesS := bytesI.([]interface{})

			msg = make([]byte, len(bytesS))
			for i, val := range bytesS {
				s := fmt.Sprintf("%.0f", val.(float64))
				valI, _ := strconv.Atoi(s)

				msg[i] = byte(valI)

			}
		} else {
			return nil, errors.New("`bytes` is a required field for the `transact` action")
		}

		s := fmt.Sprintf("%.0f", GetUnsafe(config, "resp_len").(float64))
		respLen, _ := strconv.Atoi(s)
		resp := make([]byte, respLen)

		if err := m.dvc.Tx(msg, resp); err != nil {
			return nil, fmt.Errorf("failed executing I2C transaction: %w", err)
		}

		return map[string]interface{}{
			"response": resp,
		}, nil

	default:
		return nil, fmt.Errorf("no such action `%s`", action)
	}
}

type ADS1115Module struct {
	ads *ads1x15.Dev
	pin analog.PinADC
}

func (m *ADS1115Module) Stop() error {
	return m.pin.Halt()
}

func (m *ADS1115Module) Initialize(sp ServiceProvider, config JSONHack) error {
	bus, err := sp.GetDefaultI2CBus()
	if err != nil {
		return fmt.Errorf("failed getting i2c device: %w", err)
	}

	m.ads, err = ads1x15.NewADS1115(bus, &ads1x15.DefaultOpts)
	if err != nil {
		return fmt.Errorf("failed initializing ADS1115 device: %w", err)
	}
	s := fmt.Sprintf("%.0f", GetUnsafe(config, "channel_mask").(float64))
	ch, _ := strconv.Atoi(s)

	m.pin, err = m.ads.PinForChannel(ads1x15.Channel(ch), 5*physic.Volt, 1*physic.Hertz, ads1x15.SaveEnergy)
	if err != nil {
		return fmt.Errorf("failed initializing ADS1115 device: %w", err)
	}

	return nil
}
func (m *ADS1115Module) Act(action string, config JSONHack) (JSONHack, error) {
	switch action {
	case "read":
		val, err := m.pin.Read()
		return val, err
	default:
		return nil, fmt.Errorf("no such action `%s`", action)
	}
}

type HTGModule struct {
	tk htg3535ch.TemperatureK
	rh htg3535ch.Humidity
}

func (*HTGModule) Stop() error { return nil }

func (m *HTGModule) Initialize(sp ServiceProvider, config JSONHack) error {
	s := fmt.Sprintf("%.0f", GetUnsafe(config, "temperature_adc_channel").(float64))
	tempCh, _ := strconv.Atoi(s)
	s = fmt.Sprintf("%.0f", GetUnsafe(config, "humidity_adc_channel").(float64))
	humCh, _ := strconv.Atoi(s)
	if calCh, ok := Get(config, "calibration_adc_channel"); ok && calCh != nil {
		calCh, _ := strconv.Atoi(fmt.Sprintf("%.0f", calCh.(float64)))
		m.tk = htg3535ch.NewCalibrationTemperatureK(tempCh, calCh)
	} else {
		m.tk = htg3535ch.NewDefaultTemperatureK(tempCh)
	}

	m.rh = htg3535ch.NewHumidity(humCh)
	return nil
}
func (m *HTGModule) Act(action string, config JSONHack) (JSONHack, error) {
	switch action {
	case "rh":
		val, err := m.rh.Read()
		return val, err
	case "tk":
		val, err := m.tk.Read()
		return val, err
	case "tc":
		val, err := m.tk.Read()
		return val - 273.15, err
	case "tf":
		val, err := m.tk.Read()
		return (val-273.15)*9/5 + 32, err
	default:
		return nil, fmt.Errorf("no such action `%s`", action)
	}
}

//////////////////////////
// hardware interfacing //
type I2CDevice interface {
	Tx(w, r []byte) error
}
type ServiceProvider interface {
	GetGPIOPin(p uint8) (gpio.Pin, error)
	GetDefaultI2CBus() (i2c.BusCloser, error)

	Close() error
}

func NewServiceProvider() (*ServiceAgent, error) {
	// TODO switch this over to periph.io? don't hurry though
	if err := gpio.Setup(); err != nil {
		fmt.Println("failed to identify a gpio bus - modules relying on gpio will fail to initialize: ", err.Error())
	}

	_, err := host.Init()
	if err != nil {
		fmt.Println("failed initializing perph.io host", err.Error())
		return nil, err
	}

	bus, err := i2creg.Open("")
	if err != nil {
		fmt.Println("failed to identify an i2c bus - modules relying on I2C will fail to initialize: ", err.Error())
	}

	return &ServiceAgent{
		defaultI2CBus: bus,
	}, nil
}

type ServiceAgent struct {
	defaultI2CBus i2c.BusCloser
}

func (a *ServiceAgent) GetGPIOPin(p uint8) (gpio.Pin, error) {

	return gpio.PinRef(p), nil
}
func (a *ServiceAgent) GetDefaultI2CBus() (i2c.BusCloser, error) {
	return a.defaultI2CBus, nil
}
func (a *ServiceAgent) Close() error {
	return a.defaultI2CBus.Close()
}
