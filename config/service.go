package config

import (
	"fmt"
	"os"
	"os/user"
	"time"
)

// Service is the settings a service is made from
type Service struct {
	Name    string
	Program string
	Args    []string
	Dir     string
	Env     map[string]string

	// Temp is true if this config isn't loaded from a file, created at runtime
	Temp       bool
	CleanAfter time.Duration
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
