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

	Output output
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

	info := Info{
		Service: &s.Conf,
	}

	info.Running = s.Running()
	info.Pid = s.Pid()

	info.StartTime = s.startTime
	info.EndTime = s.endTime
	if info.Running {
		info.Runtime = time.Since(s.startTime)
	} else {
		info.Runtime = s.endTime.Sub(s.startTime)
	}

	// - running services haven't succeeded yet
	// - a service stopped by a user is succesfull, regardless of result
	// - a service that's in the restart watchlist is failed if not running
	// - otherwise use exit status
	info.Succeeded = !info.Running && (s.userStopped || (!s.Conf.RestartOnExit && s.state != nil && s.state.Success()))

	tail, _, _, _ := s.Output.GetTail(info.Pid, 5)
	info.Tail = make([]string, 0, len(tail))
	for _, line := range tail {
		info.Tail = append(info.Tail, line.Line)
	}

	return info
}

// Start starts running the service
func (s *Service) Start(updates chan<- Info) error {
	if s.Running() {
		return fmt.Errorf("Service already running.")
	}
	log.Debug("Starting service", "service", s.Conf.Name)

	// Update right after starting, but before we can race with the end-watcher
	defer func() {
		select {
		case updates <- s.Info():
		default:
		}
	}()

	s.stateLock.Lock()
	defer s.stateLock.Unlock()

	// Clear out previous values, even ones we set on start, in case there's
	// an error.
	s.process = nil
	s.state = nil
	s.startTime = time.Time{}
	s.endTime = time.Time{}
	s.userStopped = false

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

	// Set the process group ID to 0, so it'll create a new one, which
	// it'll be in, plus all the subprocesses it might create. Then,
	// signals to that PGID will go to them alone, not the bento
	// server.
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Pgid:    0,
		Setpgid: true,
	}

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
	outputDone := s.Output.followNewProcess(s.process.Pid, stdout, stderr)
	go s.watchForExit(cmd, updates, outputDone)

	close(s.startChan)

	log.Info("Started service", "service", s.Conf.Name, "pid", s.process.Pid)

	return nil
}

// Stop stops running the service
func (s *Service) Stop(escalationInterval time.Duration) error {
	if !s.Running() {
		return nil
	}

	if escalationInterval == 0 {
		escalationInterval = config.EscalationInterval
	}

	pid := s.Pid()
	if pid == 0 {
		return fmt.Errorf("Failed to get service's pid to stop", "service", s.Conf.Name)
	}

	// Try a sequence increasingly urgent signals
	signals := []syscall.Signal{syscall.SIGINT, syscall.SIGTERM, syscall.SIGKILL}

	// In case killing the process itself fails, like if one of its child
	// processes is ignoring signals from its parent, get the PGID (process
	// group id) that we had the parent (the process we started) create.
	pids := []int{pid}
	if pgid, err := syscall.Getpgid(pid); err != nil {
		log.Warn("Failed to get pgid in case of a failed service stop", "service", s.Conf.Name, "pid", pid, "err", err)
	} else {
		pids = append(pids, -pgid)
	}

	for _, pid := range pids {
		for _, sig := range signals {
			log.Debug("Sending service's proc signal", "service", s.Conf.Name, "signal", sig, "pid", pid)
			if err := syscall.Kill(pid, sig); err != nil {
				return err
			}

			// Wait a bit for process to die
			select {
			case <-time.After(escalationInterval):
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
func (s *Service) watchForExit(cmd *exec.Cmd, updates chan<- Info, outputDone *sync.WaitGroup) {
	// Completely exhaust both outputs before waiting for the cmd to exit,
	// cuz Wait will close the pipes before we can read everything from
	// them.
	outputDone.Wait()

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
