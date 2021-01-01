package ads1115

import (
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"math"
	"os/exec"
	"strconv"
)

//ADS1115 represents an ADS1115 over I2C. The current implementation assumes
//that the device uses its default I2C address, 0x48.
type ADS1115 struct {
	pin int
}

func New(pin int) ADS1115 {
	return ADS1115{
		pin: pin,
	}
}

func (a ADS1115) ReadVoltage() (float64, error) {
	cmd := exec.Command("./ads1115", strconv.FormatInt(int64(a.pin), 10))

	// Combine stdout and stderr
	output, err := cmd.CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("failed executing ADS1115 read with message `%s`: %w", string(output), err)
	}

	bs, err := base64.StdEncoding.DecodeString(string(output))
	if err != nil {
		return 0, fmt.Errorf("failed decoding ADS1115 result: %w", err)
	}

	bits := binary.LittleEndian.Uint64(bs)
	return math.Float64frombits(bits), nil
}
