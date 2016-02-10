package config_test

import (
	. "github.com/heewa/bento/config"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"bytes"
	"encoding/gob"
	"time"
)

var _ = Describe("Service", func() {
	// Service with all the fields set to something valid
	var aService Service

	BeforeEach(func() {
		aService = Service{
			Name: "SomeService",

			Program: "/bin/echo",
			Args:    []string{"yay"},

			Dir: "/",
			Env: make(map[string]string),
		}
		aService.Env["PATH"] = "/"
	})

	Describe("Sanitize()", func() {
		Context("When all the fields are set correctly", func() {
			It("should not error", func() {
				Expect(aService.Sanitize()).To(BeNil())
			})
		})

		Context("When there's no Name", func() {
			It("should error", func() {
				aService.Name = ""
				Expect(aService.Sanitize()).ToNot(BeNil())
			})
		})

		Context("When there's no Program", func() {
			It("should error", func() {
				aService.Program = ""
				Expect(aService.Sanitize()).ToNot(BeNil())
			})
		})

		Context("When there's no Dir", func() {
			It("should set it to something", func() {
				aService.Dir = ""
				Expect(aService.Sanitize()).To(BeNil())
				Expect(aService.Dir).ToNot(Equal(""))
			})
		})

		Describe("Temp Services", func() {
			Context("When there's no CleanAfter on a temp Service", func() {
				It("should set it to the default", func() {
					aService.Temp = true
					aService.CleanAfter = 0
					Expect(aService.Sanitize()).To(BeNil())
					Expect(aService.CleanAfter).To(Equal(CleanTempServicesAfter))
				})
			})

			Context("When there's a CleanAfter on a non-temp Service", func() {
				It("should set it to the 0", func() {
					aService.Temp = false
					aService.CleanAfter = 10 * time.Minute
					Expect(aService.Sanitize()).To(BeNil())
					Expect(aService.CleanAfter).To(Equal(time.Duration(0)))
				})
			})
		})
	})

	Describe("EqualIgnoringSafeFields()", func() {
		// Clone of aService
		var anotherService Service

		BeforeEach(func() {
			// Clone using gob encode/decode
			var buffer bytes.Buffer
			Expect(gob.NewEncoder(&buffer).Encode(aService)).To(BeNil())
			Expect(gob.NewDecoder(&buffer).Decode(&anotherService)).To(BeNil())
		})

		Context("When completely equal", func() {
			It("returns true", func() {
				Expect(aService.EqualIgnoringSafeFields(&anotherService)).To(Equal(true))
			})
		})

		Context("When unsafe fields are different", func() {
			It("returns false", func() {
				anotherService.Name = "DifferentService"
				Expect(aService.EqualIgnoringSafeFields(&anotherService)).To(Equal(false))
			})
		})

		Context("When safe & unsafe fields are different", func() {
			It("returns false", func() {
				anotherService.Name = "DifferentService"
				anotherService.AutoStart = true
				Expect(aService.EqualIgnoringSafeFields(&anotherService)).To(Equal(false))
			})
		})

		Context("When only unsafe fields are different", func() {
			It("returns true", func() {
				anotherService.AutoStart = true
				Expect(aService.EqualIgnoringSafeFields(&anotherService)).To(Equal(true))
			})
		})
	})
})
