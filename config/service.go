package config

import (
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
	AutoStart     bool `yaml:"autostart,omitempty"`
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
		// Try the current dir
		if curDir, err := os.Getwd(); err == nil {
			s.Dir = curDir
		} else {
			// Try the user's home dir
			if usr, err := user.Current(); err == nil {
				s.Dir = usr.HomeDir
			} else {
				// I guess root?
				s.Dir = "/"
			}
		}
	}

	if s.Temp && s.CleanAfter == 0 {
		s.CleanAfter = CleanTempServicesAfter
	} else if !s.Temp {
		s.CleanAfter = 0
	}

	return nil
}

func (s *Service) EqualIgnoringTemp(s2 *Service) bool {
	if s.Name != s2.Name || s.Program != s2.Program || s.Dir != s2.Dir {
		return false
	}

	if !reflect.DeepEqual(s.Args, s2.Args) || !reflect.DeepEqual(s.Env, s2.Env) {
		return false
	}

	return true
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
