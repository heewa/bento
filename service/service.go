package service

import (
	"fmt"
	"os/exec"
	"os/user"
	"strings"
)

// Info holds info about a service
type Info struct {
	Name    string
	Program string
	Args    []string
	Dir     string
	Env     map[string]string

	Running   bool
	Pid       int
	Succeeded bool
}

// Service -
type Service struct {
	Name string
	cmd  *exec.Cmd
}

// New creates a new Service
func New(name string, program string, args []string, dir string, env map[string]string) (*Service, error) {
	if name == "" {
		name = program
	}

	programPath, err := exec.LookPath(program)
	if err != nil {
		return nil, err
	}

	cmd := exec.Command(programPath, args...)

	if dir != "" {
		cmd.Dir = dir
	} else {
		usr, err := user.Current()
		if err != nil {
			return nil, err
		}
		cmd.Dir = usr.HomeDir
	}

	if env != nil {
		var envItems []string
		for key, value := range env {
			envItems = append(envItems, fmt.Sprintf("%s=%s", key, value))
		}
		cmd.Env = envItems
	} else {
		// Explicitly set it to empty
		cmd.Env = []string{}
	}

	return &Service{
		Name: name,
		cmd:  cmd,
	}, nil
}

// Info gets info about the service
func (s *Service) Info() Info {
	info := Info{
		Name: s.Name,
		Dir:  s.cmd.Dir,
	}

	if len(s.cmd.Args) >= 1 {
		info.Program = s.cmd.Args[0]
	}
	if len(s.cmd.Args) >= 2 {
		info.Args = s.cmd.Args[1:]
	}

	info.Env = make(map[string]string)
	for _, item := range s.cmd.Env {
		parts := strings.Split(item, "=")
		if len(parts) == 2 {
			info.Env[parts[0]] = parts[1]
		} else {
			info.Env[parts[0]] = ""
		}
	}

	if s.Running() {
		info.Running = true
		info.Pid = s.cmd.Process.Pid
	} else {
		info.Pid = s.cmd.ProcessState.Pid()
		info.Succeeded = s.cmd.ProcessState.Success()
	}

	return info
}

// Start starts running the service
func (s *Service) Start() error {
	return s.cmd.Start()
}

// Stop stops running the service
func (s *Service) Stop() error {
	// TODO: Try to interrupt it first
	/*
		if err := s.cmd.Process.Signal(exec.Interrupt); err != nil {
			return err
		}
	*/

	return s.cmd.Process.Kill()
}

// Running returns true if service is currently running
func (s *Service) Running() bool {
	if s.cmd.Process != nil && s.cmd.ProcessState == nil {
		return true
	}
	return false
}
