package server

import (
	"fmt"
	"time"

	log "github.com/inconshreveable/log15"

	"github.com/heewa/bento/service"
)

// StopArgs -
type StopArgs struct {
	Name string

	// Time to wait between escalation signals to the service's process
	EscalationInterval time.Duration
}

// StopResponse -
type StopResponse struct {
	Info service.Info
}

// Stop stops a service, if it's running
func (s *Server) Stop(args StopArgs, reply *StopResponse) (err error) {
	defer func() {
		if r := recover(); r != nil {
			log.Crit("panic", "msg", r)
			err = fmt.Errorf("Server error: %v", r)
		}
	}()

	serv := s.getService(args.Name)
	if serv == nil {
		return fmt.Errorf("Service '%s' not found.", args.Name)
	}

	// Before stopping, if it's being restart-watched, remove that so we
	// don't auto-restart it. Not just temporarily, this stop is a user's
	// request, so leave it un-watched until another start.
	if serv.Conf.RestartOnExit {
		s.removeServiceFromRestartWatch(serv.Conf.Name)
	}

	log.Info("Stopping service", "service", serv.Conf.Name)
	err = serv.Stop(args.EscalationInterval)

	// Set info regarless of error
	if reply != nil {
		reply.Info = serv.Info()
	}

	return err
}
