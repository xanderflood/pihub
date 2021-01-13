package htg3535ch

import (
	"math"

	"periph.io/x/periph/conn/physic"
	"periph.io/x/periph/experimental/conn/analog"
)

// based on https://www.te.com/commerce/DocumentDelivery/DDEController?Action=showdoc&DocId=Data+Sheet%7FHPC123_K%7FA1%7Fpdf%7FEnglish%7FENG_DS_HPC123_K_A1.pdf%7FCAT-HSMM0001

//TemperatureK represents the HTG pin for measure temperature in Kelvins
type TemperatureK struct {
	TempADS             analog.PinADC
	BatchResistanceOhms float64
	VCCVolts            analog.PinADC
}

//NewDefaultTemperatureK creates a new TemperatureK with default wiring configuration
func NewDefaultTemperatureK(pin analog.PinADC) TemperatureK {
	return NewTemperatureK(pin, 10000.0, nil)
}

//NewCalibrationTemperatureK creates a new TemperatureK with default wiring configuration
func NewCalibrationTemperatureK(tPin, vccPin analog.PinADC) TemperatureK {
	return NewTemperatureK(tPin, 10000.0, vccPin)
}

//NewTemperatureK creates a new TemperatureK with default wiring configuration
func NewTemperatureK(tPin analog.PinADC, batchResistanceOhms float64, vccVolts analog.PinADC) TemperatureK {
	return TemperatureK{
		TempADS:             tPin,
		BatchResistanceOhms: batchResistanceOhms,
		VCCVolts:            vccVolts,
	}
}

//Read takes a reading from the underlying ADS1115 and converts the voltage
//value to a temperature reading in Kelvins.
func (s TemperatureK) Read() (float64, error) {
	sample, err := s.TempADS.Read()
	if err != nil {
		return 0, err
	}
	v := float64(sample.V) / float64(physic.Volt)

	var vcc float64
	if s.VCCVolts != nil {
		sample, err = s.VCCVolts.Read()
		if err != nil {
			return 0, err
		}
		vcc = float64(sample.V) / float64(physic.Volt)
	} else {
		vcc = 5.0
	}

	ntcResistanceOhms := s.BatchResistanceOhms * v / (vcc - v)
	logR := math.Log(ntcResistanceOhms)
	temp := 1 / (8.61393e-04 + 2.56377e-04*logR + 1.68055e-07*logR*logR*logR)
	return temp, nil
}

//Humidity represents the HTG pin for measure relative humidity in percent
type Humidity struct {
	analog.PinADC
}

//NewHumidity creates a new Humidity
func NewHumidity(pin analog.PinADC) Humidity {
	return Humidity{
		PinADC: pin,
	}
}

//Read takes a reading from the underlying ADS1115 and converts the voltage
//value to a relative humidity reading in percent.
func (s Humidity) Read() (float64, error) {
	sample, err := s.PinADC.Read()
	if err != nil {
		return 0, err
	}
	v := float64(sample.V) / float64(physic.Volt)

	return -1.564*v*v*v + 12.05*v*v + 8.22*v - 15.6, nil
}
