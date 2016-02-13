package config

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"io/ioutil"
	"os"
	"os/user"
	"reflect"
	"time"

	"gopkg.in/yaml.v2"
)

// Service is the settings a service is made from
type Service struct {
	Name string `yaml:"name"`

	// What to run
	Program string   `yaml:"program"`
	Args    []string `yaml:"args,omitempty"`

	// Runtime env
	Dir string            `yaml:"dir,omitempty"`
	Env map[string]string `yaml:"env,omitempty"`

	// Behavior
	AutoStart     bool `yaml:"auto-start,omitempty"`
	RestartOnExit bool `yaml:"restart-on-exit,omitempty"`

	// Temp is true if this config isn't loaded from a file, created at runtime
	Temp       bool          `yaml:",omitempty"`
	CleanAfter time.Duration `yaml:",omitempty"`
}

// Sanitize checks a config for valitidy, and fixes up values that are dynamic
// or have defaults.
func (s *Service) Sanitize() error {
	switch "" {
	case s.Name:
		return fmt.Errorf("Service needs a name")
	case s.Program:
		return fmt.Errorf("Service needs a program to run")
	case s.Dir:
		// Try the user's home dir
		if usr, err := user.Current(); err == nil {
			s.Dir = usr.HomeDir
		} else {
			// I guess root?
			s.Dir = "/"
		}
	}

	if s.Temp && s.CleanAfter == 0 {
		s.CleanAfter = CleanTempServicesAfter
	} else if !s.Temp {
		s.CleanAfter = 0
	}

	return nil
}

// EqualIgnoringSafeFields returns true if the service config equals another,
// ignoring fields that can be safely changed on a running service.
func (s *Service) EqualIgnoringSafeFields(s2 *Service) bool {
	// Take a white-list approach, so future changes to the conf don't
	// accidentally yield unsafe replacements if we forgot to change this
	// fn.

	// Copy a conf by encoding/decoding (closest I could find to a proper
	// deep-copy).
	var s2Copy Service
	var buffer bytes.Buffer
	if err := gob.NewEncoder(&buffer).Encode(s2); err != nil {
		// The confs should be encodable, so this shouldn't happen.
		panic(fmt.Sprintf("Failed to encode service config for comparrison: %v", err))
	}
	if err := gob.NewDecoder(&buffer).Decode(&s2Copy); err != nil {
		panic(fmt.Sprintf("Failed to decode service config for comparrison: %v", err))
	}

	// Clear white-list fields
	s2Copy.AutoStart = s.AutoStart
	s2Copy.RestartOnExit = s.RestartOnExit
	s2Copy.Temp = s.Temp
	s2Copy.CleanAfter = s.CleanAfter

	return reflect.DeepEqual(s, &s2Copy)
}

// LoadServiceFile reads a file for a list of service confs, sanitizing them
// all
func LoadServiceFile(path string) ([]Service, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("Failed to read service conf (%s): %v", path, err)
	}
	defer f.Close()

	data, err := ioutil.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("Failed to read service conf (%s): %v", path, err)
	}

	var services []Service
	if err := yaml.Unmarshal(data, &services); err != nil {
		return nil, fmt.Errorf("Invalid service conf (%s): %v", path, err)
	}

	for _, service := range services {
		if err := service.Sanitize(); err != nil {
			return nil, fmt.Errorf("Bad service definition for name='%s': %v", service.Name, err)
		}
	}

	return services, nil
}
