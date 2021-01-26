package main

import (
	"periph.io/x/periph/conn/gpio"
	"periph.io/x/periph/conn/gpio/gpioreg"
	"periph.io/x/periph/conn/i2c"
	"periph.io/x/periph/conn/physic"
	"periph.io/x/periph/experimental/conn/analog"
	"periph.io/x/periph/experimental/devices/ads1x15"

	"errors"
	"fmt"

	"github.com/xanderflood/pihub/pkg/htg3535ch"
)

////////////////////////
// The module library //
type EchoModule struct{}

func (*EchoModule) Stop() error { return nil }

func (e *EchoModule) Initialize(sp ServiceProvider, _ Binder) error {
	return nil
}
func (e *EchoModule) Act(action string, body Binder) (interface{}, error) {
	var reqVal interface{}
	request := &reqVal
	if err := body.BindData(request); err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"action": action,
		"config": request,
	}, nil
}

type RelayModule struct {
	pin gpio.PinOut
}
type RelayModuleConfig struct {
	Pin string `json:"pin"`
}
type RelaySetRequest struct {
	High bool `json:"high"`
}

func (r RelaySetRequest) Level() gpio.Level {
	return gpio.Level(r.High)
}

func (*RelayModule) Stop() error { return nil }

func (m *RelayModule) Initialize(sp ServiceProvider, binder Binder) error {
	var config = &RelayModuleConfig{}
	if err := binder.BindData(config); err != nil {
		return err
	}

	// Use gpioreg GPIO pin registry to find a GPIO pin by name.
	pin := gpioreg.ByName(config.Pin)
	if pin == nil {
		return errors.New("Failed to find pin")
	}
	if err := pin.Out(gpio.Low); err != nil {
		return err
	}
	m.pin = pin

	return nil
}
func (m *RelayModule) Act(action string, body Binder) (interface{}, error) {
	var request = &RelaySetRequest{}
	if err := body.BindData(request); err != nil {
		return nil, err
	}

	switch action {
	case "set":
		return nil, m.pin.Out(request.Level())
	default:
		return nil, fmt.Errorf("no such action `%s`", action)
	}
}

type I2CModule struct {
	dvc I2CDevice
}
type I2CModuleConfig struct {
	Address uint16 `json:"address"`
}
type I2CTransactRequest struct {
	Bytes          []byte `json:"bytes"`
	ResponseLength int    `json:"resp_len"`
}

func (*I2CModule) Stop() error { return nil }

func (m *I2CModule) Initialize(sp ServiceProvider, binder Binder) error {
	var config = &I2CModuleConfig{}
	if err := binder.BindData(config); err != nil {
		return err
	}

	var err error
	bus, err := sp.GetDefaultI2CBus()
	m.dvc = &i2c.Dev{Bus: bus, Addr: config.Address}
	if err != nil {
		return fmt.Errorf("failed getting i2c device: %w", err)
	}

	return nil
}
func (m *I2CModule) Act(action string, body Binder) (interface{}, error) {
	var request = &I2CTransactRequest{}
	if err := body.BindData(request); err != nil {
		return nil, err
	}

	switch action {
	case "transact":
		resp := make([]byte, request.ResponseLength)
		if err := m.dvc.Tx(request.Bytes, resp); err != nil {
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
type ADS1115ModuleConfig struct {
	Ch int `json:"channel_mask"`
}

func (m *ADS1115Module) Stop() error {
	return m.pin.Halt()
}

func (m *ADS1115Module) Initialize(sp ServiceProvider, binder Binder) error {
	var config = &ADS1115ModuleConfig{}
	if err := binder.BindData(config); err != nil {
		return err
	}

	bus, err := sp.GetDefaultI2CBus()
	if err != nil {
		return fmt.Errorf("failed getting i2c device: %w", err)
	}

	m.ads, err = ads1x15.NewADS1115(bus, &ads1x15.DefaultOpts)
	if err != nil {
		return fmt.Errorf("failed initializing ADS1115 device: %w", err)
	}

	m.pin, err = m.ads.PinForChannel(ads1x15.Channel(config.Ch),
		5*physic.Volt, 1*physic.Hertz, ads1x15.SaveEnergy)
	if err != nil {
		return fmt.Errorf("failed initializing ADS1115 device: %w", err)
	}

	return nil
}
func (m *ADS1115Module) Act(action string, _ Binder) (interface{}, error) {
	switch action {
	case "read":
		sample, err := m.pin.Read()
		return float64(sample.V) / float64(physic.Volt), err
	default:
		return nil, fmt.Errorf("no such action `%s`", action)
	}
}

type HTGModule struct {
	humidity    analog.PinADC
	temperature analog.PinADC

	tk htg3535ch.TemperatureK
	rh htg3535ch.Humidity

	rhCalibrationAdjustment float64
}
type HTGModuleConfig struct {
	TemperatureADCChannel   int     `json:"temperature_adc_channel"`
	HumidityADCChannel      int     `json:"humidity_adc_channel"`
	RHCalibrationAdjustment float64 `json:"rh_calibration_adjustment"`
}

func (m *HTGModule) Stop() error {
	_ = m.humidity.Halt()
	return m.temperature.Halt()
}

func (m *HTGModule) Initialize(sp ServiceProvider, binder Binder) error {
	var config = &HTGModuleConfig{}
	if err := binder.BindData(config); err != nil {
		return err
	}

	bus, err := sp.GetDefaultI2CBus()
	if err != nil {
		return fmt.Errorf("failed getting i2c device: %w", err)
	}

	ads, err := ads1x15.NewADS1115(bus, &ads1x15.DefaultOpts)
	if err != nil {
		return fmt.Errorf("failed initializing ADS1115 device: %w", err)
	}

	m.temperature, err = ads.PinForChannel(
		ads1x15.Channel(config.TemperatureADCChannel),
		5*physic.Volt, 1*physic.Hertz, ads1x15.BestQuality)
	if err != nil {
		return fmt.Errorf("failed initializing ADS1115 device: %w", err)
	}
	m.tk = htg3535ch.NewDefaultTemperatureK(m.temperature)

	m.humidity, err = ads.PinForChannel(
		ads1x15.Channel(config.HumidityADCChannel),
		5*physic.Volt, 1*physic.Hertz, ads1x15.BestQuality)
	if err != nil {
		return fmt.Errorf("failed initializing ADS1115 device: %w", err)
	}
	m.rh = htg3535ch.NewHumidity(m.humidity)

	m.rhCalibrationAdjustment = config.RHCalibrationAdjustment

	return nil
}

type HTGCalibrateRequest struct {
	TrueValue  *float64 `json:"true_value"`
	Adjustment *float64 `json:"adjustment"`
}
type HTGCalibrateResponse struct {
	Adjustment float64 `json:"adjustment"`
}

func (m *HTGModule) Act(action string, body Binder) (interface{}, error) {
	switch action {
	case "rh":
		val, err := m.rh.Read()
		return val + m.rhCalibrationAdjustment, err
	case "tk":
		val, err := m.tk.Read()
		return val, err
	case "tc":
		val, err := m.tk.Read()
		return val - 273.15, err
	case "tf":
		val, err := m.tk.Read()
		return (val-273.15)*9/5 + 32, err
	case "calibrate":
		var request = &HTGCalibrateRequest{}
		if err := body.BindData(request); err != nil {
			return nil, err
		}

		var adjustment float64
		if request.Adjustment != nil {
			adjustment = *request.Adjustment
		} else {
			var trueValue = 100.0
			if request.TrueValue != nil {
				trueValue = *request.TrueValue
			}

			val, err := m.rh.Read()
			if err != nil {
				return nil, err
			}
			adjustment = trueValue - val
		}

		m.rhCalibrationAdjustment = adjustment
		return HTGCalibrateResponse{
			Adjustment: adjustment,
		}, nil
	default:
		return nil, fmt.Errorf("no such action `%s`", action)
	}
}
