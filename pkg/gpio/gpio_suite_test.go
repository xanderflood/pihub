package gpio_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestGpio(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Gpio Suite")
}
