package am2301_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestAm2301(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Am2301 Suite")
}
