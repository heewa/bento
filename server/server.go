package server

import (
	"net"
	"net/rpc"
	"os"
	"os/signal"

	log "github.com/inconshreveable/log15"
)

// Server is the backend that manages services
type Server struct {
	fifoAddr *net.UnixAddr

	stop chan interface{}
}

// New creates a new Server
func New(fifoPath string) (*Server, error) {
	// Resolve the net address
	addr, err := net.ResolveUnixAddr("unix", fifoPath)
	if err != nil {
		return nil, err
	}

	return &Server{
		fifoAddr: addr,
		stop:     make(chan interface{}),
	}, nil
}

func (s *Server) Start(_ bool, _ *bool) error {
	log.Debug("Registering RPC interface")
	if err := rpc.Register(s); err != nil {
		return err
	}

	log.Debug("Listening on fifo", "address", s.fifoAddr)
	listener, err := net.ListenUnix("unix", s.fifoAddr)
	if err != nil {
		return err
	}
	defer listener.Close()

	// Handle interrupt & kill signal, to try to clean up
	go func() {
		signals := make(chan os.Signal, 1)
		signal.Notify(signals, os.Interrupt, os.Kill)
		defer signal.Stop(signals)

		for {
			sig := <-signals
			log.Warn("Got interrupt/kill signal", "signal", sig)

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

	log.Debug("Done listening")

	return nil
}

func (s *Server) Exit(_ bool, _ *bool) error {
	log.Warn("Exiting server")
	close(s.stop)

	log.Debug("Connecting to server to break out of listen loop")
	if conn, err := net.DialUnix("unix", nil, s.fifoAddr); err != nil {
		return err
	} else {
		conn.Close()
		log.Debug("Connected and closed")
	}

	return nil
}
