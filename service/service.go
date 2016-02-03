package service

import (
	"os/exec"
)

type Service struct {
	Name    string
	Program string
	Args    []string
	Dir     string
	Env     map[string]string

	cmd *exec.Cmd
}

func New(program string, args []string) (*Service, error) {
	programPath, err := exec.LookPath(program)
	if err != nil {
		return nil, err
	}

	return &Service{
		Name:    program,
		Program: programPath,

		cmd: exec.Command(programPath, args...),
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
