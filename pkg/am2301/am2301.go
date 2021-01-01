package am2301

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/xanderflood/pihub/pkg/gpio"
)

////////////////
// this is a golang port of the C library found at:
// https://github.com/kporembinski/DHT21-AM2301/blob/master/am2301.c
////////////////

//State state of an AM2301 sensor
type State struct {
	RH   float64
	Temp float64
}

//AM2301 minimal interface
type AM2301 interface {
	Check() (State, error)
}

//Impl standard implementation of a Monitor
type Impl struct {
	pin      gpio.Pin
	interval time.Duration
	mode     int // should always be 1 - something to do with restarting

	sync.Mutex
}

//New new am2301
func New(pin gpio.Pin) *Impl {
	return &Impl{
		pin:      pin,
		interval: 100 * time.Millisecond,
		mode:     1,
	}
}

//Check start a monitor with the given handler hook
func (am *Impl) Check() (State, error) {
	am.Mutex.Lock()
	defer am.Mutex.Unlock()

	err := am.Request()
	if err != nil {
		return State{}, err
	}

	vals, err := am.Read()
	if err != nil {
		return State{}, err
	}

	//parse the results
	state, err := Parse(vals)
	if err != nil {
		return State{}, err
	}

	if !state.Valid() {
		return State{}, err
	}

	return state, nil
}

//Request send the signal to request a new measurement
func (am *Impl) Request() error {
	// Leave it high for a while
	am.pin.Output()
	am.pin.High()
	time.Sleep(100 * time.Microsecond)

	// Set it low to give the start signal
	am.pin.Low()
	time.Sleep(1000 * time.Microsecond)

	// Now set the pin high to let the sensor start communicating
	am.pin.High()
	am.pin.Input()
	if _, ok := gpio.WaitChange(am.pin, gpio.High, 100*time.Microsecond); !ok {
		return errors.New("unexpected request sequence 1")
	}

	// Wait for ACK
	if _, ok := gpio.WaitChange(am.pin, gpio.Low, 100*time.Microsecond); !ok {
		return errors.New("unexpected request sequence 2")
	}
	if _, ok := gpio.WaitChange(am.pin, gpio.High, 100*time.Microsecond); !ok {
		return errors.New("unexpected request sequence 3")
	}

	// When restarting, it looks like this look for start bit is not needed
	if am.mode != 0 {
		// Wait for the start bit
		if _, ok := gpio.WaitChange(am.pin, gpio.Low, 200*time.Microsecond); !ok {
			return errors.New("unexpected request sequence 4")
		}

		if _, ok := gpio.WaitChange(am.pin, gpio.High, 200*time.Microsecond); !ok {
			return errors.New("unexpected request sequence 5")
		}
	}

	return nil
}

//Read read a 5-byte sequence from the pin
func (am *Impl) Read() ([5]byte, error) {
	var vals [5]byte
	for i := 0; i < 5; i++ {
		for j := 7; j >= 0; j-- {
			val, ok := gpio.WaitChange(am.pin, gpio.Low, 500*time.Microsecond)
			if !ok {
				return [5]byte{}, fmt.Errorf("unexpected read signal %v:%v:1", i, j)
			}

			if val >= 50*time.Microsecond {
				vals[i] = vals[i] | (1 << uint(j))
			}

			_, ok = gpio.WaitChange(am.pin, gpio.High, 500*time.Microsecond)
			if !ok {
				return [5]byte{}, fmt.Errorf("unexpected read signal %v:%v:2", i, j)
			}
		}
	}

	am.pin.Output()
	am.pin.High()

	return vals, nil
}

//Parse parse a State from a 5-byte input stream
func Parse(vals [5]byte) (State, error) {
	//TODO test this using the examples on page 5 of
	//https://kropochev.com/downloads/humidity/AM2301.pdf

	// Verify checksum
	if vals[0]+vals[1]+vals[2]+vals[3] != vals[4] {
		//TODO log these somewhere
		return State{}, errors.New("invalid checksum")
	}

	tempSign := 1
	if (vals[2] >> 7) != 0 {
		//turn off the sign bit and set the sign
		vals[2] ^= (1 << 7)
		tempSign = -1
	}

	return State{
		RH:   float64((int(vals[0])<<8)|int(vals[1])) / 10.0,
		Temp: float64(tempSign*((int(vals[2])<<8)|int(vals[3]))) / 10.0,
	}, nil
}

//Valid check that values are within the specifed range
func (s State) Valid() bool {
	return (s.RH <= 100.0) && (s.RH >= 0.0) && (s.Temp <= 80.0) && (s.Temp <= 40.0)
}
