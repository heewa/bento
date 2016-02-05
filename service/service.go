package service

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"sync"
	"syscall"
	"time"

	log "github.com/inconshreveable/log15"
)

const (
	maxOutputSize = 100 * 1024 * 1024 // 100mb
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

	// Output is locked by outLock
	outLock      sync.RWMutex
	stdout       []string
	stdoutShifts int
	stderr       []string
	stderrShifts int
}

// New creates a new Service
func New(name string, program string, args []string, dir string, env map[string]string) (*Service, error) {
	if dir == "" {
		// Try the current dir
		if curDir, err := os.Getwd(); err == nil {
			dir = curDir
		} else {
			// Try the user's home dir
			if usr, err := user.Current(); err == nil {
				dir = usr.HomeDir
			} else {
				// I guess root?
				dir = "/"
			}
		}
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
	s.stateLock.RLock()
	defer s.stateLock.RUnlock()

	running := s.Running()

	var runtime time.Duration
	if running {
		runtime = time.Since(s.startTime)
	} else {
		runtime = s.endTime.Sub(s.startTime)
	}

	info := Info{
		Service: *s,
		Running: running,
		Pid:     s.Pid(),

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
func (s *Service) Start(updates chan<- Info) error {
	if s.Running() {
		return fmt.Errorf("Service already running.")
	}

	// Update right after starting, but before we can race with the end-watcher
	defer func() {
		select {
		case updates <- s.Info():
		default:
		}
	}()

	s.stateLock.Lock()
	defer s.stateLock.Unlock()

	s.outLock.Lock()
	defer s.outLock.Unlock()

	// Clear out previous values, even ones we set on start, in case there's
	// an error.
	s.process = nil
	s.state = nil
	s.startTime = time.Time{}
	s.endTime = time.Time{}
	s.stdout = nil
	s.stdoutShifts = 0
	s.stderr = nil
	s.stderrShifts = 0

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

	// Get line-scanners for stdout/err
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stdout := bufio.NewScanner(pipe)

	pipe, err = cmd.StderrPipe()
	if err != nil {
		return err
	}
	stderr := bufio.NewScanner(pipe)

	if err := cmd.Start(); err != nil {
		return err
	}
	s.startTime = time.Now()
	s.exitChan = make(chan interface{})
	s.process = cmd.Process

	// Periodically send info about service, while it's running
	go func() {
		tick := time.Tick(3 * time.Second)
		for {
			select {
			case <-s.exitChan:
				return
			case <-tick:
				select {
				case updates <- s.Info():
				default:
				}
			}
		}
	}()

	go func() {
		// Read from stdout/err & throw in a tail-array. Completely exhaust
		// both before waiting for the cmd to exit, cuz Wait will close the
		// pipes before we can read everything from them. Read each one
		// separately in a goroutine so they don't block each other
		done := make(chan interface{})
		go watchOutput(stdout, &s.stdout, &s.stdoutShifts, &s.outLock, done)
		go watchOutput(stderr, &s.stderr, &s.stderrShifts, &s.outLock, done)
		<-done
		<-done

		// Wait for exit
		err := cmd.Wait()
		log.Info("Service exited", "name", s.Name, "program", s.Program, "err", err)

		// Update after we let go of lock
		defer func() {
			select {
			case updates <- s.Info():
			default:
			}
		}()

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

// Wait blocks until it stops running
func (s *Service) Wait() error {
	<-s.exitChan
	return nil
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

func (s *Service) Pid() int {
	s.stateLock.RLock()
	defer s.stateLock.RUnlock()

	if s.process != nil {
		return s.process.Pid
	} else if s.state != nil {
		return s.state.Pid()
	}
	return 0
}

// Stdout gets lines from stdout since a line index
func (s *Service) Stdout(pid, since int) (lines []string, newSince int, newPid int) {
	s.stateLock.RLock()
	defer s.stateLock.RUnlock()
	s.outLock.RLock()
	defer s.outLock.RUnlock()

	return getOutput(pid, since, s.Pid(), s.stdoutShifts, s.stdout)
}

// Stderr gets lines from stderr since a line index
func (s *Service) Stderr(pid, since int) (lines []string, newSince int, newPid int) {
	s.stateLock.RLock()
	defer s.stateLock.RUnlock()
	s.outLock.RLock()
	defer s.outLock.RUnlock()

	return getOutput(pid, since, s.Pid(), s.stderrShifts, s.stderr)
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

func watchOutput(out *bufio.Scanner, tail *[]string, shifts *int, lock *sync.RWMutex, done chan<- interface{}) {
	size := 0

	for out.Scan() {
		lock.Lock()

		line := out.Text()
		size += len(line)
		*tail = append(*tail, line)

		// Cut down by total size, cuz output could be a binary stream, and we
		// care about size more than # lines anyway.
		for len(*tail) > 1 && size > maxOutputSize {
			size -= len((*tail)[0])
			*tail = (*tail)[1:]
			*shifts += 1
		}

		lock.Unlock()
	}

	done <- struct{}{}
}

func getOutput(pid, since, currentPid, shifts int, outLines []string) ([]string, int, int) {
	// If pid doesn't match, there's been a restart since the last call, so
	// reset since
	if pid != currentPid {
		since = 0
	}

	// Look up where in the current buffer they are
	index := since - shifts
	if index < 0 {
		// They've fallen behind, just start at the earliest
		index = 0
	}

	numLines := len(outLines) - index
	lines := []string{}
	if numLines > 0 {
		lines = append([]string{}, outLines[index:]...)
	} else {
		// They're caught up
		numLines = 0
	}

	return lines, index + shifts + numLines, currentPid
}
