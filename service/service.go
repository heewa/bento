package service

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"sync"

	log "github.com/inconshreveable/log15"
)

// Service -
type Service struct {
	Name    string
	Program string
	Args    []string
	Dir     string
	Env     map[string]string

	// All these fields are locked by stateLock
	stateLock sync.RWMutex
	running   bool
	pid       int
	state     *os.ProcessState
}

// Info holds info about a service
type Info struct {
	Service

	Running   bool
	Pid       int
	Succeeded bool
}

// New creates a new Service
func New(name string, program string, args []string, dir string, env map[string]string) (*Service, error) {
	if dir == "" {
		usr, err := user.Current()
		if err != nil {
			return nil, err
		}
		dir = usr.HomeDir
	}

	return &Service{
		Name:    name,
		Program: program,
		Args:    args,
		Dir:     dir,
		Env:     env,
	}, nil
}

// Info gets info about the service
func (s *Service) Info() Info {
	s.stateLock.RLock()
	defer s.stateLock.RUnlock()

	info := Info{
		Service: *s,
		Running: s.running,
		Pid:     s.pid,
	}

	if !s.running && s.state != nil {
		info.Succeeded = s.state.Success()
	}

	return info
}

// Start starts running the service
func (s *Service) Start() error {
	s.stateLock.Lock()
	defer s.stateLock.Unlock()

	if s.running {
		return fmt.Errorf("Service already running.")
	}

	// Clear out previous values
	s.running = false
	s.pid = 0
	s.state = nil

	programPath, err := exec.LookPath(s.Program)
	if err != nil {
		return err
	}

	var envItems []string
	for key, value := range s.Env {
		envItems = append(envItems, fmt.Sprintf("%s=%s", key, value))
	}

	cmd := exec.Command(programPath, s.Args...)
	cmd.Dir = s.Dir
	cmd.Env = envItems

	if err := cmd.Start(); err != nil {
		return err
	}
	s.running = true
	s.pid = cmd.Process.Pid

	go func() {
		err := cmd.Wait()
		log.Debug("Service exited", "name", s.Name, "program", s.Program, "err", err)

		s.stateLock.Lock()
		defer s.stateLock.Unlock()

		s.running = false
		s.state = cmd.ProcessState
	}()

	return nil
}

// Stop stops running the service
func (s *Service) Stop() error {
	// TODO: Try to interrupt it first
	/*
		if err := s.cmd.Process.Signal(exec.Interrupt); err != nil {
			return err
		}
	*/

	//return s.cmd.Process.Kill()
	return nil
}

// Running returns true if service is currently running
func (s *Service) Running() bool {
	s.stateLock.RLock()
	defer s.stateLock.RUnlock()

	return s.running
}
