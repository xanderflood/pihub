package main

import (
	"sync"
	"time"

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

	rhAdjustment float64
}
type HTGModuleConfig struct {
	TemperatureADCChannel int     `json:"temperature_adc_channel"`
	HumidityADCChannel    int     `json:"humidity_adc_channel"`
	RHAdjustment          float64 `json:"rh_adjustment"`
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

	m.rhAdjustment = config.RHAdjustment

	return nil
}

type HTGCalibrateRequest struct {
	TrueValue    *float64 `json:"true_value"`
	RHAdjustment *float64 `json:"rh_adjustment"`
}
type HTGCalibrateResponse struct {
	RHAdjustment float64 `json:"rh_adjustment"`
}

func (m *HTGModule) Act(action string, body Binder) (interface{}, error) {
	switch action {
	case "rh":
		val, err := m.rh.Read()
		return val + m.rhAdjustment, err
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
		if request.RHAdjustment != nil {
			adjustment = *request.RHAdjustment
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

		m.rhAdjustment = adjustment
		return HTGCalibrateResponse{
			RHAdjustment: adjustment,
		}, nil
	default:
		return nil, fmt.Errorf("no such action `%s`", action)
	}
}

// NOTE: The pin *must* have hardware PWM support via periph
type ServoModuleConfig struct {
	Pin          string  `json:"pin"`
	FrequencyHZ  int64   `json:"frequenzy_hz"`
	DutyRatioP90 float64 `json:"duty_ratio_p90"`
	DutyRatioN90 float64 `json:"duty_ratio_n90"`
}

func (c *ServoModuleConfig) Default() {
	c.FrequencyHZ = 50
	c.DutyRatioP90 = 0.05
	c.DutyRatioN90 = 0.10
}
func (c ServoModuleConfig) Validate() error {
	if c.Pin == "" {
		return errors.New("`pin` is a required field")
	}
	return nil
}
func (c ServoModuleConfig) DutyForAngle(deg float64) gpio.Duty {
	var normalizedValue = (deg - 90) / 180
	var dutyRatio = (normalizedValue+1)*c.DutyRatioP90 - normalizedValue*c.DutyRatioN90
	return dutyForRatio(dutyRatio)
}
func (c ServoModuleConfig) Frequency() physic.Frequency {
	return physic.Frequency(c.FrequencyHZ) * physic.Hertz
}

type ServoModule struct {
	config ServoModuleConfig
	pin    gpio.PinOut

	sync.Mutex
}

func (m *ServoModule) Stop() error {
	return m.pin.Halt()
}

func (m *ServoModule) Initialize(sp ServiceProvider, binder Binder) error {
	var err error
	if err = binder.BindData(&m.config); err != nil {
		return err
	}

	if m.pin, err = sp.GetGPIOByName(m.config.Pin); err != nil {
		return fmt.Errorf("failed getting i2c device: %w", err)
	}

	return nil
}

type ServoSetAngleRequest struct {
	Angle float64 `json:"angle"`
}

func (m *ServoModule) Act(action string, body Binder) (interface{}, error) {
	switch action {
	case "set":
		var request = &ServoSetAngleRequest{}
		if err := body.BindData(request); err != nil {
			return nil, err
		}

		var duty = m.config.DutyForAngle(request.Angle)
		if err := m.pin.PWM(duty, m.config.Frequency()); err != nil {
			return nil, fmt.Errorf("failed setting PWM: %w", err)
		}

		return nil, nil
	default:
		return nil, fmt.Errorf("no such action `%s`", action)
	}
}

func dutyForRatio(v float64) gpio.Duty {
	var floatVal = v * float64(gpio.DutyMax)
	return gpio.Duty(floatVal)
}

type HCSRO4Config struct {
	TriggerPin       string   `json:"trigger_pin"`
	EchoPin          string   `json:"echo_pin"`
	SpeedOfSoundMPMS *float64 `json:"speed_of_sound_mpms"`
}

func (c HCSRO4Config) Validate() error {
	if c.TriggerPin == "" {
		return errors.New("`trigger_pin` is a required field")
	}
	if c.EchoPin == "" {
		return errors.New("`echo_pin` is a required field")
	}
	return nil
}

type HCSRO4Module struct {
	trigger     gpio.PinOut
	echo        gpio.PinIn
	coefficient float64
}

func (m *HCSRO4Module) Initialize(sp ServiceProvider, binder Binder) error {
	var err error
	var config HCSRO4Config
	if err = binder.BindData(&config); err != nil {
		return err
	}

	if m.trigger, err = sp.GetGPIOByName(config.TriggerPin); err != nil {
		return fmt.Errorf("failed getting trigger pin: %w", err)
	}
	if m.echo, err = sp.GetGPIOByName(config.EchoPin); err != nil {
		return fmt.Errorf("failed getting echo pin: %w", err)
	}

	// 0.1715 = half of 0.343 m/micros the speed of sound
	m.coefficient = 0.1715
	if config.SpeedOfSoundMPMS != nil {
		// convert m/ms to meters/micros, then divide by
		// two since its a round trip.
		m.coefficient = *config.SpeedOfSoundMPMS / 2000.0
	}

	return nil
}

func (m *HCSRO4Module) Stop() error {
	_ = m.trigger.Halt()
	return m.echo.Halt()
}

// ServoSetAngleResponse represents a range reading in meters. The
// range value is nil when no object was in range.
type ServoSetAngleResponse struct {
	OutOfRange     bool     `json:"in_range,omitempty"`
	DistanceMeters *float64 `json:"distance_meters,omitempty"`
}

func ServoSetAngleResponseFor(val *float64) ServoSetAngleResponse {
	if val != nil {
		return ServoSetAngleResponse{DistanceMeters: val}
	}
	return ServoSetAngleResponse{OutOfRange: true}
}

func (m *HCSRO4Module) Act(action string, body Binder) (interface{}, error) {
	switch action {
	case "read_meters":
		return m.readDistanceM()
	default:
		return nil, fmt.Errorf("no such action `%s`", action)
	}
}

func (d *HCSRO4Module) readDistanceM() (*float64, error) {
	pulseDuration, err := d.readDuration()
	if err != nil || pulseDuration == nil {
		return nil, err
	}

	val := float64(*pulseDuration) * d.coefficient
	return &val, nil
}

const HCSRO4TimeoutDuration = 38_000 * time.Microsecond

func (m *HCSRO4Module) readDuration() (*time.Duration, error) {
	m.trigger.Out(gpio.Low)
	time.Sleep(2 * time.Microsecond)
	m.trigger.Out(gpio.High)
	time.Sleep(12 * time.Microsecond)
	m.trigger.Out(gpio.Low)

	m.echo.In(gpio.PullNoChange, gpio.RisingEdge)
	if !m.echo.WaitForEdge(HCSRO4TimeoutDuration) {
		return nil, errors.New("failed to read range")
	}

	m.echo.In(gpio.PullNoChange, gpio.FallingEdge)
	start := time.Now()
	if !m.echo.WaitForEdge(HCSRO4TimeoutDuration) {
		return nil, nil
	}

	dur := time.Since(start)
	return &dur, nil
}
