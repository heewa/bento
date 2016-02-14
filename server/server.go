package server

import (
	"fmt"
	"net"
	"net/rpc"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	log "github.com/inconshreveable/log15"

	"github.com/heewa/bento/config"
	"github.com/heewa/bento/service"
)

const (
	minRestartPause = 500 * time.Millisecond
	maxRestartPause = 1 * time.Minute
)

// Server is the backend that manages services
type Server struct {
	fifoAddr *net.UnixAddr

	services     map[string]*service.Service
	servicesLock sync.RWMutex

	serviceUpdates chan<- service.Info

	// watchedServices is a collection of restart-watched services as a map
	// from their name to a chanel that can be used to cancel the watch
	watchLock       sync.RWMutex
	watchedServices map[string]chan interface{}

	stop chan interface{}
}

// New creates a new Server
func New() (*Server, <-chan service.Info, error) {
	// Catch obvious address errors early
	addr, err := net.ResolveUnixAddr("unix", config.FifoPath)
	if err != nil {
		return nil, nil, err
	}

	// Make the stop channel with a buffer because the goroutine that reads
	// from it might be blocked on listening for RPC connections, which the
	// same entity that's stopping will need to break it out of
	stop := make(chan interface{}, 1)

	serv := &Server{
		fifoAddr: addr,

		services:        make(map[string]*service.Service),
		watchedServices: make(map[string]chan interface{}),

		stop: stop,
	}

	// Communicate with UI about service changes through a channel
	var updatesOut <-chan service.Info
	serv.serviceUpdates, updatesOut = serv.watchServices()

	return serv, updatesOut, nil
}

// Init runs the server, listening for RPC calls, blocking until exit
func (s *Server) Init(_ bool, _ *bool) error {
	log.Debug("Registering RPC interface")
	if err := rpc.Register(s); err != nil {
		return err
	}

	log.Info("Listening on fifo", "address", s.fifoAddr)
	listener, err := s.openFifo()
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

	// Periodically update mod time on fifo, so other instances can recover
	// from our crash/hang
	cancelHeartbeat, err := s.startHeartbeat()
	if err != nil {
		return err
	}

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

	close(cancelHeartbeat)

	// Stop all services
	var wait sync.WaitGroup
	for _, srvc := range s.services {
		if srvc.Running() {
			wait.Add(1)
			go func() {
				defer wait.Done()

				if err := s.Stop(StopArgs{srvc.Conf.Name}, nil); err != nil {
					log.Warn("Failed to stop service during shutdown", "service", srvc.Conf.Name, "err", err)
				}
			}()
		}
	}
	wait.Wait()

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
		if serv != nil {
			services = append(services, serv)
		}
	}

	return services
}

func (s *Server) addService(serv *service.Service, replace bool) error {
	s.servicesLock.Lock()
	defer s.servicesLock.Unlock()

	current := s.services[serv.Conf.Name]
	if current != nil && !replace {
		return fmt.Errorf("Service already exists (%s)", serv.Conf.Name)
	} else if current != nil && current.Running() {
		return fmt.Errorf("Can't replace a running service (%s)", serv.Conf.Name)
	}

	s.services[serv.Conf.Name] = serv

	// Notify watchers
	s.serviceUpdates <- serv.Info()

	if serv.Conf.AutoStart {
		go func() {
			if err := s.Start(StartArgs{serv.Conf.Name}, nil); err != nil {
				log.Warn("Failed to auto-start service", "service", serv.Conf.Name, "err", err)
			}
		}()
	}

	return nil
}

func (s *Server) removeService(name string) error {
	s.servicesLock.Lock()
	defer s.servicesLock.Unlock()

	srvc := s.services[name]
	if srvc == nil {
		return nil
	}

	if err := srvc.Stop(); err != nil {
		return err
	}

	delete(s.services, name)

	// Notify watchers
	info := srvc.Info()
	info.Dead = true
	s.serviceUpdates <- info

	return nil
}

func (s *Server) changeServicePermanence(name string, temp bool, cleanAfter time.Duration) bool {
	s.servicesLock.Lock()
	defer s.servicesLock.Unlock()

	srvc := s.services[name]
	if srvc == nil {
		return false
	}

	if temp {
		srvc.Conf.Temp = true
		srvc.Conf.CleanAfter = cleanAfter
	} else {
		srvc.Conf.Temp = false
		srvc.Conf.CleanAfter = 0
	}

	return true
}

func (s *Server) addServiceToRestartWatch(srvc *service.Service) {
	log.Info("Adding service to restart-watch list", "service", srvc.Conf.Name)

	s.watchLock.Lock()
	defer s.watchLock.Unlock()

	// Remove it if it's there already
	if cancel := s.watchedServices[srvc.Conf.Name]; cancel != nil {
		close(cancel)
	}

	cancel := make(chan interface{})
	s.watchedServices[srvc.Conf.Name] = cancel

	go func() {
		defer func() {
			log.Debug("Ending restart-watch for service", "service", srvc.Conf.Name)
		}()
		pauseTime := minRestartPause

		for {
			select {
			case <-cancel:
				return
			case <-time.After(maxRestartPause):
				// It's been running for a bit, so reset pauseTime
				if pauseTime != minRestartPause {
					log.Debug("Resetting restart pause", "service", srvc.Conf.Name)
					pauseTime = minRestartPause
				}
			case <-srvc.GetExitChan():
				// Start the service again, after a pause
				select {
				case <-cancel:
					return
				case <-srvc.GetStartChan():
					// Don't bother if it was started during the pause
				case <-time.After(pauseTime):
					pauseTime *= 2
					if pauseTime > maxRestartPause {
						pauseTime = maxRestartPause
					}

					if err := srvc.Start(s.serviceUpdates); err != nil {
						log.Warn("Failed to restart service", "service", srvc.Conf.Name, "pause-before-next-restart", pauseTime, "err", err)
					} else {
						log.Debug("Restarted service", "service", srvc.Conf.Name)
					}
				}
			}
		}

	}()
}

func (s *Server) removeServiceFromRestartWatch(name string) {
	log.Debug("Removing service from restart-watch list", "service", name)

	s.watchLock.Lock()
	defer s.watchLock.Unlock()

	if cancel := s.watchedServices[name]; cancel != nil {
		close(cancel)
	}

	delete(s.watchedServices, name)
}

// watchServices handles cleaning up temp services when they exit
// TODO: refactor this to be like the restart watch
func (s *Server) watchServices() (chan<- service.Info, <-chan service.Info) {
	// TODO: this whole thing should just be based on an event model, pub-sub
	// or some shit.

	updatesIn := make(chan service.Info)
	updatesOut := make(chan service.Info, 100)

	go func() {
		// Clean up channels
		defer func() {
			close(updatesOut)
		}()

		deathWatcherCancels := make(map[string]chan interface{})

		for {
			info := <-updatesIn

			// Drop if UI isn't keeping up
			select {
			case updatesOut <- info:
			default:
			}

			// Temp services need to be cleaned up after a timeout after ending
			if info.Temp {
				// Any change on a temp service should cancel a death watch
				cancel := deathWatcherCancels[info.Name]
				if cancel != nil {
					// Cancel the current death watcher
					close(cancel)
				}

				// If it exitted, start a new death watch
				if !info.Dead && !info.Running && !info.EndTime.IsZero() {
					cancel = make(chan interface{})
					deathWatcherCancels[info.Name] = cancel

					// Death Watch
					log.Debug("Watching for service death", "service", info.Name, "cleanAfter", info.CleanAfter)
					go func(name string, cleanAfter time.Duration, cancel <-chan interface{}) {
						select {
						case <-cancel:
						case <-time.After(cleanAfter):
							log.Info("Auto-cleaning service after timeout", "service", name)
							s.removeService(name)
						}
					}(info.Name, info.CleanAfter, cancel)
				} else {
					delete(deathWatcherCancels, info.Name)
				}
			}
		}
	}()

	return updatesIn, updatesOut
}

func (s *Server) openFifo() (*net.UnixListener, error) {
	// Check the mod time on the fifo file. If it's pretty old, delete it
	// so we can use that address. Fifo's can become dead like this if
	// the server hard-crashed, or it was killed with SIGKILL.
	fifoInfo, err := os.Stat(s.fifoAddr.String())
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	} else if err == nil {
		if time.Since(fifoInfo.ModTime()) < config.HeartbeatInterval*2 {
			return nil, fmt.Errorf("There's an active server still running.")
		}

		log.Warn("Removing stale fifo file.")
		if err := os.Remove(s.fifoAddr.String()); err != nil {
			return nil, err
		}
	}

	listener, err := net.ListenUnix("unix", s.fifoAddr)
	if err != nil {
		return nil, err
	}

	return listener, nil
}

func (s *Server) startHeartbeat() (chan<- interface{}, error) {
	cancel := make(chan interface{})

	go func() {
		for {
			select {
			case <-cancel:
				return
			case <-time.After(config.HeartbeatInterval):
				now := time.Now()
				if err := os.Chtimes(s.fifoAddr.String(), now, now); err != nil {
					log.Warn("Failed to update fifo mod time", "err", err)
				}
			}
		}
	}()

	return cancel, nil
}
