package service

import (
	"fmt"
	"os/exec"
	"os/user"
)

type Service struct {
	// Read-only, ignored at runtime
	Name    string
	Program string
	Args    []string
	Dir     string
	Env     map[string]string

	cmd *exec.Cmd
}

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
		Name:    program,
		Program: programPath,

		Args: args,
		Dir:  cmd.Dir,
		Env:  env,

		cmd: cmd,
	}, nil
}

func (s *Service) Start() error {
	return s.cmd.Start()
}

func (s *Service) Stop() error {
	// TODO: Try to interrupt it first
	/*
		if err := s.cmd.Process.Signal(exec.Interrupt); err != nil {
			return err
		}
	*/

	return s.cmd.Process.Kill()
}

func (s *Service) Running() bool {
	if s.cmd.Process != nil && s.cmd.ProcessState == nil {
		return true
	}
	return false
}
