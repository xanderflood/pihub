package relay

import "github.com/xanderflood/pihub/pkg/gpio"

//Relay a relay module
type Relay interface {
	Set(bool)
}

//RelayAgent standard relay implementation
type RelayAgent struct {
	pin      gpio.OutputPin
	inverted bool
}

//New control a relay
func New(pin gpio.OutputPin, inverted bool) *RelayAgent {
	return &RelayAgent{
		pin:      pin,
		inverted: inverted,
	}
}

//On turn the relay on
func (r *RelayAgent) Set(on bool) {
	val := (on != r.inverted) //xor
	gpio.Set(r.pin, val)
}
