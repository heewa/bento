package service

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"sync"
	"syscall"
	"time"

	log "github.com/inconshreveable/log15"
)

// Service -
type Service struct {
	Name    string
	Program string
	Args    []string
	Dir     string
	Env     map[string]string

	// Closed when process exits, no need for lock to use.
	exitChan chan interface{}

	// All these fields are locked by stateLock
	stateLock sync.RWMutex
	process   *os.Process
	state     *os.ProcessState
	startTime time.Time
	endTime   time.Time
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

	// Start off with an existing, but closed exit chan
	exitChan := make(chan interface{})
	close(exitChan)

	return &Service{
		Name:    name,
		Program: program,
		Args:    args,
		Dir:     dir,
		Env:     env,

		exitChan: exitChan,
	}, nil
}

// Info gets info about the service
func (s *Service) Info() Info {
	running := false
	runtime := s.endTime.Sub(s.startTime)
	if s.exitChan != nil {
		select {
		case <-s.exitChan:
		default:
			running = true
			runtime = time.Since(s.startTime)
		}
	}

	info := Info{
		Service: *s,
		Running: running,
		Pid:     s.process.Pid,

		StartTime: s.startTime,
		EndTime:   s.endTime,
		Runtime:   runtime,
	}

	if !running && s.state != nil {
		info.Succeeded = s.state.Success()
	}

	return info
}

// Start starts running the service
func (s *Service) Start() error {
	if s.Running() {
		return fmt.Errorf("Service already running.")
	}

	s.stateLock.Lock()
	defer s.stateLock.Unlock()

	// Clear out previous values, even ones we set on start, in case there's
	// an error.
	s.process = nil
	s.state = nil
	s.startTime = time.Time{}
	s.endTime = time.Time{}

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
	s.startTime = time.Now()
	s.exitChan = make(chan interface{})
	s.process = cmd.Process

	go func() {
		err := cmd.Wait()
		log.Info("Service exited", "name", s.Name, "program", s.Program, "err", err)

		s.stateLock.Lock()
		defer s.stateLock.Unlock()

		s.endTime = time.Now()
		s.state = cmd.ProcessState

		// Close exit chan last cuz it signals other goroutines
		close(s.exitChan)
	}()

	return nil
}

// Stop stops running the service
func (s *Service) Stop() error {
	// Try a sequence increasingly urgent signals
	signals := []os.Signal{os.Interrupt, syscall.SIGTERM, os.Kill}

	for _, sig := range signals {
		log.Debug("Sending service's proc signal", "service", s.Name, "signal", sig)
		if err := s.signal(sig); err != nil {
			return err
		}

		// Wait a bit for process to die
		select {
		case <-s.exitChan:
			return nil
		case <-time.After(3 * time.Second):
		}
	}

	return fmt.Errorf("Failed to stop service")
}

// Running returns true if service is currently running
func (s *Service) Running() bool {
	select {
	case <-s.exitChan:
		return false
	default:
	}
	return true
}

func (s *Service) signal(sig os.Signal) error {
	s.stateLock.RLock()
	defer s.stateLock.RUnlock()

	if !s.Running() {
		return fmt.Errorf("Service isn't running")
	}

	// Try to interrupt it first
	if err := s.process.Signal(sig); err != nil {
		return fmt.Errorf("Failed to signal process with %v: %v", sig, err)
	}

	return nil
}
