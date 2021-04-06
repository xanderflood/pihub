package main

import (
	"bytes"
	"io"
	"log"

	"periph.io/x/periph/conn/gpio"
	"periph.io/x/periph/conn/gpio/gpioreg"
	"periph.io/x/periph/conn/i2c"
	"periph.io/x/periph/conn/i2c/i2creg"
	"periph.io/x/periph/host"
	"periph.io/x/periph/host/bcm283x"

	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httputil"
)

////////////////////////
// The module runtime //
type Module interface {
	Initialize(p ServiceProvider, body Binder) error
	Act(action string, body Binder) (interface{}, error)

	Stop() error
}

type ModuleFactory func() Module

var ModuleIndex = map[string]ModuleFactory{
	"echo":      func() Module { return &EchoModule{} },
	"relay":     func() Module { return &RelayModule{} },
	"htg3535ch": func() Module { return &HTGModule{} },
	"i2c":       func() Module { return &I2CModule{} },
	"ads":       func() Module { return &ADS1115Module{} },
	"servo":     func() Module { return &ServoModule{} },
	"hcsro4":    func() Module { return &HCSRO4Module{} },
}

func (a *ManagerAgent) InitializeModules(specs map[string]ModuleSpec) error {
	for name, spec := range specs {
		if factory, ok := ModuleIndex[spec.Source]; ok {
			a.Modules[name] = factory()
			binder := &JSONBinder{requestBody: bytes.NewBuffer([]byte(spec.Config))}
			if err := a.Modules[name].Initialize(a.ServiceProvider, binder); err != nil {
				return fmt.Errorf("failed to initialize module: %w", err)
			}
		} else {
			return fmt.Errorf("404 no such module source: %s", spec.Source)
		}
	}
	return nil
}
func (a *ManagerAgent) Act(module string, action string, binder Binder) (interface{}, error) {
	if mod, ok := a.Modules[module]; ok {
		return mod.Act(action, binder)
	}
	return nil, errors.New("no such module") // TODO 404
}

////////////////
// HTTP Logic //
type InitializeRequest struct {
	Modules map[string]ModuleSpec `json:"modules"`
}
type ModuleSpec struct {
	Source string          `json:"source"`
	Config json.RawMessage `json:"config"`
}
type InitializeResponse struct {
	NumModules int `json:"num_modules"`
}

type ActRequest struct {
	Module string          `json:"module"`
	Action string          `json:"action"`
	Config json.RawMessage `json:"config"`
}
type ActResponse struct {
	Result interface{} `json:"result"`
}

type ManagerAgent struct {
	Modules         map[string]Module
	ServiceProvider ServiceProvider
}

func main() {
	router := buildMux()

	_ = http.ListenAndServe("0.0.0.0:3141", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

		if err := json.NewEncoder(w).Encode(InitializeResponse{NumModules: len(mgr.Modules)}); err != nil {
			fmt.Println("failed sending response", err.Error())
			return
		}
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

		actRequest := &JSONBinder{requestBody: bytes.NewBuffer([]byte(req.Config))}
		if result, err := mgr.Act(req.Module, req.Action, actRequest); err != nil {

			// errors that passed through the JSONBinder will be marked so we can
			// respond with a 400 instead.
			var iErr InputError
			if errors.As(err, &iErr) {
				w.WriteHeader(http.StatusBadRequest)
				if wErr := json.NewEncoder(w).Encode(map[string]interface{}{
					"mesage": fmt.Sprintf("invalid request: %s", err.Error()),
				}); wErr != nil {
					fmt.Println("failed writing HTTP response:", wErr.Error())
					return
				}
			}

			// otherwise, we respond with a 500
			w.WriteHeader(http.StatusInternalServerError)
			if wErr := json.NewEncoder(w).Encode(map[string]interface{}{
				"mesage": fmt.Sprintf("invalid request: %s", err.Error()),
			}); wErr != nil {
				fmt.Println("failed executing action:", wErr.Error())
				return
			}
			return
		} else {
			if err := json.NewEncoder(w).Encode(ActResponse{Result: result}); err != nil {
				fmt.Println("failed writing HTTP response:", err.Error())
				return
			}
		}
	}))

	return mux
}

//////////////////////////
// hardware interfacing //
type I2CDevice interface {
	Tx(w, r []byte) error
}
type ServiceProvider interface {
	GetGPIOByName(name string) (gpio.PinIO, error)
	GetDefaultI2CBus() (i2c.BusCloser, error)

	Close() error
}

func NewServiceProvider() (*ServiceAgent, error) {
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

func (a *ServiceAgent) GetDefaultI2CBus() (i2c.BusCloser, error) {
	return a.defaultI2CBus, nil
}
func (a *ServiceAgent) GetGPIOByName(name string) (gpio.PinIO, error) {
	if name == "18" {
		return bcm283x.GPIO18, nil
	}

	return gpioreg.ByName(name), nil
}
func (a *ServiceAgent) Close() error {
	return a.defaultI2CBus.Close()
}

//////////////////////
// module interface //
type Binder interface {
	BindData(ptr interface{}) error
}
type Validator interface {
	Validate() error
}

type Defaulter interface {
	Default()
}

type JSONBinder struct {
	requestBody io.Reader
}

type InputError struct {
	error
}

func (b *JSONBinder) BindData(ptr interface{}) error {
	if v, ok := ptr.(Defaulter); ok {
		v.Default()
	}

	if err := json.NewDecoder(b.requestBody).Decode(ptr); err != nil {
		return InputError{error: err}
	}

	if v, ok := ptr.(Validator); ok {
		if err := v.Validate(); err != nil {
			return InputError{error: err}
		}
	}

	return nil
}
