package main_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"testing"
)

func TestBento(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Bento Suite")
}
