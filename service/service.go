package service

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	log "github.com/inconshreveable/log15"

	"github.com/heewa/bento/config"
)

const (
	shortTailLen  = 10
	maxOutputSize = 100 * 1024 * 1024 // 100mb
)

// Service represents a loaded service config. It manages running, stopping,
// and controlling its process.
type Service struct {
	Conf config.Service

	// Closed when process starts/exits, no need for lock to use.
	startChan chan interface{}
	exitChan  chan interface{}

	// All these fields are locked by stateLock
	stateLock   sync.RWMutex
	process     *os.Process
	state       *os.ProcessState
	startTime   time.Time
	endTime     time.Time
	userStopped bool

	// Output is locked by outLock
	outLock      sync.RWMutex
	stdout       []string
	stdoutShifts int
	stderr       []string
	stderrShifts int
	shortTail    []string
}

// New creates a new Service
func New(conf config.Service) (*Service, error) {

	// Start off with existing start & exit chans, but since it's not running,
	// exitChan should be closed, and startChan should be open.
	exitChan := make(chan interface{})
	close(exitChan)
	startChan := make(chan interface{})

	return &Service{
		Conf:      conf,
		startChan: startChan,
		exitChan:  exitChan,
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
		Service: &s.Conf,

		Running: running,
		Pid:     s.Pid(),

		// - running services haven't succeeded yet
		// - a service stopped by a user is succesfull, regardless of result
		// - a service that's in the restart watchlist is failed if not running
		// - otherwise use exit status
		Succeeded: !running && (s.userStopped || (!s.Conf.RestartOnExit && s.state != nil && s.state.Success())),

		StartTime: s.startTime,
		EndTime:   s.endTime,
		Runtime:   runtime,
	}

	func() {
		s.outLock.RLock()
		defer s.outLock.RUnlock()

		info.Tail = make([]string, len(s.shortTail))
		copy(info.Tail, s.shortTail)
	}()

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
	s.userStopped = false
	s.stdout = nil
	s.stdoutShifts = 0
	s.stderr = nil
	s.stderrShifts = 0

	programPath, err := exec.LookPath(s.Conf.Program)
	if err != nil {
		return err
	}

	var envItems []string
	for key, value := range s.Conf.Env {
		envItems = append(envItems, fmt.Sprintf("%s=%s", key, value))
	}

	cmd := exec.Command(programPath, s.Conf.Args...)
	cmd.Dir = s.Conf.Dir
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

	// Now that all the setup completed without failure, start the process
	if err := cmd.Start(); err != nil {
		return err
	}
	s.startTime = time.Now()
	s.exitChan = make(chan interface{})
	s.process = cmd.Process

	go s.sendPeriodicUpdates(updates)

	// Read from stdout/err & throw in a tail-array.
	outputDone := make(chan interface{})
	go s.watchOutput(stdout, &s.stdout, &s.stdoutShifts, outputDone)
	go s.watchOutput(stderr, &s.stderr, &s.stderrShifts, outputDone)

	go s.watchForExit(cmd, updates, outputDone)

	close(s.startChan)

	return nil
}

// Stop stops running the service
func (s *Service) Stop() error {
	if !s.Running() {
		return nil
	}

	// Try a sequence increasingly urgent signals
	signals := []os.Signal{os.Interrupt, syscall.SIGTERM, os.Kill}

	for _, sig := range signals {
		log.Debug("Sending service's proc signal", "service", s.Conf.Name, "signal", sig)
		if err := s.signal(sig); err != nil {
			return err
		}

		// Wait a bit for process to die
		select {
		case <-time.After(10 * time.Second):
		case <-s.exitChan:
			// Consider this the user's stop, not an unrelated exit.
			func() {
				s.stateLock.Lock()
				defer s.stateLock.Unlock()
				s.userStopped = true
			}()

			return nil
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

// GetStartChan returns a channel that'll be closed once the service starts
func (s *Service) GetStartChan() <-chan interface{} {
	return s.startChan
}

// GetExitChan returns a channel that'll be closed once the service stops
func (s *Service) GetExitChan() <-chan interface{} {
	return s.exitChan
}

// Pid gets the process id of a running or ended service.
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
func (s *Service) Stdout(pid, since, max int) (lines []string, newSince int, newPid int) {
	currentPid := s.Pid()

	s.outLock.RLock()
	defer s.outLock.RUnlock()

	return getOutput(pid, since, currentPid, s.stdoutShifts, max, s.stdout)
}

// Stderr gets lines from stderr since a line index
func (s *Service) Stderr(pid, since, max int) (lines []string, newSince int, newPid int) {
	currentPid := s.Pid()

	s.outLock.RLock()
	defer s.outLock.RUnlock()

	return getOutput(pid, since, currentPid, s.stderrShifts, max, s.stderr)
}

// Internal methods

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

func getOutput(pid, since, currentPid, shifts, max int, outLines []string) ([]string, int, int) {
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

	// Max counts in reverse from end
	if max > 0 && numLines > max {
		index += numLines - max
		numLines = max
	}

	lines := []string{}
	if numLines > 0 {
		lines = append([]string{}, outLines[index:]...)
	} else {
		// They're caught up
		numLines = 0
	}

	return lines, index + shifts + numLines, currentPid
}

// Internal goroutines - not regular helper fns

// sendPeriodicUpdates will send info about service to listeners while it's running
func (s *Service) sendPeriodicUpdates(updates chan<- Info) {
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
}

// watchForExit will wait for both outputs to finish, then wait for the
// process to end, before closing the exitChan to signal everyone else
func (s *Service) watchForExit(cmd *exec.Cmd, updates chan<- Info, outputDone <-chan interface{}) {
	// Completely exhaust both outputs before waiting for the cmd to exit,
	// cuz Wait will close the pipes before we can read everything from
	// them.
	<-outputDone
	<-outputDone

	// Wait for exit
	err := cmd.Wait()
	log.Info("Service exited", "name", s.Conf.Name, "program", s.Conf.Program, "err", err)

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

	// Open up startChan so it can be watched for closing
	s.startChan = make(chan interface{})

	// Close exit chan last cuz it signals other goroutines
	close(s.exitChan)
}

// watchOutput reads from stdout or stderr & puts lines on a capped slice
func (s *Service) watchOutput(out *bufio.Scanner, tail *[]string, shifts *int, done chan<- interface{}) {
	size := 0

	for out.Scan() {
		line := out.Text()

		func() {
			s.outLock.Lock()
			defer s.outLock.Unlock()

			if len(s.shortTail) >= shortTailLen {
				s.shortTail = append(s.shortTail[len(s.shortTail)-shortTailLen:], line)
			} else {
				s.shortTail = append(s.shortTail, line)
			}

			size += len(line)
			*tail = append(*tail, line)

			// Cut down by total size, cuz output could be a binary stream, and we
			// care about size more than # lines anyway.
			for len(*tail) > 1 && size > maxOutputSize {
				size -= len((*tail)[0])
				*tail = (*tail)[1:]
				*shifts++
			}

		}()
	}

	done <- struct{}{}
}
