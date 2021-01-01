package htg3535ch

import (
	"math"

	"github.com/xanderflood/pihub/pkg/ads1115"
)

// based on https://www.te.com/commerce/DocumentDelivery/DDEController?Action=showdoc&DocId=Data+Sheet%7FHPC123_K%7FA1%7Fpdf%7FEnglish%7FENG_DS_HPC123_K_A1.pdf%7FCAT-HSMM0001

//TemperatureK represents the HTG pin for measure temperature in Kelvins
type TemperatureK struct {
	TempADS             ads1115.ADS1115
	BatchResistanceOhms float64
	VCCVolts            func() (float64, error)
}

//NewDefaultTemperatureK creates a new TemperatureK with default wiring configuration
func NewDefaultTemperatureK(tPin int) TemperatureK {
	return NewTemperatureK(tPin, 10000.0, func() (float64, error) { return 5.0, nil })
}

//NewCalibrationTemperatureK creates a new TemperatureK with default wiring configuration
func NewCalibrationTemperatureK(tPin, vccPin int) TemperatureK {
	return NewTemperatureK(tPin, 10000.0, ads1115.New(vccPin).ReadVoltage)
}

//NewTemperatureK creates a new TemperatureK with default wiring configuration
func NewTemperatureK(tPin int, batchResistanceOhms float64, vccVolts func() (float64, error)) TemperatureK {
	return TemperatureK{
		TempADS:             ads1115.New(tPin),
		BatchResistanceOhms: batchResistanceOhms,
		VCCVolts:            vccVolts,
	}
}

//Read takes a reading from the underlying ADS1115 and converts the voltage
//value to a temperature reading in Kelvins.
func (s TemperatureK) Read() (float64, error) {
	v, err := s.TempADS.ReadVoltage()
	if err != nil {
		return 0, err
	}

	vcc, err := s.VCCVolts()
	if err != nil {
		return 0, err
	}

	ntcResistanceOhms := s.BatchResistanceOhms * v / (vcc - v)
	logR := math.Log(ntcResistanceOhms)
	temp := 1 / (8.61393e-04 + 2.56377e-04*logR + 1.68055e-07*logR*logR*logR)
	return temp, nil
}

//Humidity represents the HTG pin for measure relative humidity in percent
type Humidity struct {
	ads1115.ADS1115
}

//NewHumidity creates a new Humidity
func NewHumidity(pin int) Humidity {
	return Humidity{
		ADS1115: ads1115.New(pin),
	}
}

//Read takes a reading from the underlying ADS1115 and converts the voltage
//value to a relative humidity reading in percent.
func (s Humidity) Read() (float64, error) {
	v, err := s.ADS1115.ReadVoltage()
	if err != nil {
		return 0, err
	}

	return -1.564*v*v*v + 12.05*v*v + 8.22*v - 15.6, nil
}
