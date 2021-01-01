package gpio

import (
	"fmt"
	"strings"
	"time"

	rpio "github.com/stianeikeland/go-rpio"
)

//State IO pin state
type State = rpio.State

//States state names
var States = map[State]string{
	Low:  "low",
	High: "high",
}

const (
	//Low signal
	Low = rpio.Low

	//High signal
	High = rpio.High
)

//ParseState parse a state from a string
func ParseState(s string) (State, error) {
	if strings.ToLower(s) == States[Low] {
		return Low, nil
	} else if strings.ToLower(s) == States[High] {
		return High, nil
	}
	return State(0), fmt.Errorf("unexpected string %s, expected HIGH or LOW", s)
}

//Setup initialize memory buffers for GPIO
func Setup() error {
	return rpio.Open()
}

//OutputPin minimal interface for a GPIO pin
//go:generate counterfeiter . OutputPin
type OutputPin interface {
	Output()
	High()
	Low()
}

//InputPin minimal interface for a GPIO pin
//go:generate counterfeiter . InputPin
type InputPin interface {
	Input()
	Read() rpio.State
}

//Pin minimal interface for a GPIO pin
//go:generate counterfeiter . Pin
type Pin interface {
	OutputPin
	InputPin
}

//Set sets the state of the pin
func Set(pin OutputPin, high bool) {
	pin.Output()
	if high {
		pin.High()
	} else {
		pin.Low()
	}
}

//WaitChange wait until the pin reliably reads `mode`, returning the elapsed duration
//If `timeout` ms elapse in the meantime, returns an error instead
func WaitChange(pin Pin, mode rpio.State, timeout time.Duration) (elapsed time.Duration, ok bool) {
	start := time.Now()

	for {
		elapsed = time.Now().Sub(start)

		if elapsed > timeout {
			return
		}

		a := pin.Read()
		b := pin.Read()
		c := pin.Read()

		if (a == b) && (b == c) && (c == mode) {
			ok = true
			return
		}
	}
}
