package server

import (
	"fmt"

	log "github.com/inconshreveable/log15"

	"github.com/heewa/bento/service"
)

// StartArgs -
type StartArgs struct {
	Name string
}

// StartResponse -
type StartResponse struct {
	Info service.Info
}

// Start runs a service, if it's stopped
func (s *Server) Start(args StartArgs, reply *StartResponse) (err error) {
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

	log.Info("Starting service", "service", serv.Conf.Name)
	err = serv.Start(s.serviceUpdates)

	// If started, and it's supposed to be watched, add to watchlist
	if err == nil && serv.Conf.RestartOnExit {
		s.addServiceToRestartWatch(serv)
	}

	// Set info regardless of error
	if reply != nil {
		reply.Info = serv.Info()
	}

	return err
}
