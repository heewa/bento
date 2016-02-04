package server

import (
	"net"
	"net/rpc"
	"os"
	"os/signal"
	"sync"
	"syscall"

	log "github.com/inconshreveable/log15"

	"github.com/heewa/servicetray/service"
	"github.com/heewa/servicetray/tray"
)

// Server is the backend that manages services
type Server struct {
	fifoAddr *net.UnixAddr

	services     map[string]*service.Service
	servicesLock sync.RWMutex

	stop chan interface{}
}

// New creates a new Server
func New(fifoPath string) (*Server, error) {
	// Catch obvious address errors early
	addr, err := net.ResolveUnixAddr("unix", fifoPath)
	if err != nil {
		return nil, err
	}

	// Make the stop channel with a buffer because the goroutine that reads
	// from it might be blocked on listening for RPC connections, which the
	// same entity that's stopping will need to break it out of
	stop := make(chan interface{}, 1)

	return &Server{
		fifoAddr: addr,
		services: make(map[string]*service.Service),
		stop:     stop,
	}, nil
}

// Init runs the server, listening for RPC calls, blocking until exit
func (s *Server) Init(_ bool, _ *bool) error {
	log.Debug("Registering RPC interface")
	if err := rpc.Register(s); err != nil {
		return err
	}

	log.Info("Listening on fifo", "address", s.fifoAddr)
	listener, err := net.ListenUnix("unix", s.fifoAddr)
	if err != nil {
		return err
	}
	defer func() {
		if err := listener.Close(); err != nil {
			log.Error("Failed to close listener", "err", err)
		} else {
			log.Info("Closed listener")
		}
	}()

	// At this point, we've had chances to error out, and we're about to spin
	// up background goroutines to handle RPC. So finally start the UI.
	tray.Init()

	// Handle interrupt & kill signal, to try to clean up
	go func() {
		signals := make(chan os.Signal, 1)
		signal.Notify(signals, os.Interrupt, syscall.SIGTERM, syscall.SIGKILL)
		defer signal.Stop(signals)

		for {
			sig := <-signals
			log.Info("Got interrupt/kill signal", "signal", sig)

			var nothing bool
			if err := s.Exit(nothing, &nothing); err != nil {
				log.Error("Failed to exit", "err", err)
			} else {
				return
			}
		}
	}()

	done := false
	for !done {
		select {
		case <-s.stop:
			log.Info("Got request to stop")
			done = true
		default:
			log.Debug("Waiting for connections")
			if conn, err := listener.AcceptUnix(); err != nil {
				log.Warn("Failed to accept conn", "err", err)
			} else {
				log.Debug("Accepted a conn", "address", conn.RemoteAddr().String())
				go rpc.ServeConn(conn)
			}
		}
	}

	log.Info("Shut down server, stopping UI")
	tray.Quit()
	log.Info("All done")

	return nil
}

func (s *Server) getService(name string) *service.Service {
	s.servicesLock.RLock()
	defer s.servicesLock.RUnlock()

	return s.services[name]
}

func (s *Server) listServices() []*service.Service {
	s.servicesLock.RLock()
	defer s.servicesLock.RUnlock()

	services := make([]*service.Service, 0, len(s.services))
	for _, serv := range s.services {
		services = append(services, serv)
	}

	return services
}

func (s *Server) addService(serv *service.Service, replace bool) bool {
	s.servicesLock.Lock()
	defer s.servicesLock.Unlock()

	if !replace && s.services[serv.Name] != nil {
		return false
	}

	s.services[serv.Name] = serv
	return true
}
