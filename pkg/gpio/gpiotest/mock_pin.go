package gpiotest

type PinCheck func() error

type MockPin struct {
	index  int
	checks []PinCheck
}
