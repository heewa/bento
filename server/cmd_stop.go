package server

import (
	"fmt"

	log "github.com/inconshreveable/log15"

	"github.com/heewa/servicetray/service"
)

// StopArgs -
type StopArgs struct {
	Name string
}

// StopResponse -
type StopResponse struct {
	Info service.Info
}

// Stop stops a service, if it's running
func (s *Server) Stop(args StopArgs, reply *StopResponse) error {
	serv := s.getService(args.Name)
	if serv == nil {
		return fmt.Errorf("Service '%s' not found.", args.Name)
	}

	log.Info("Stopping service", "service", serv.Conf.Name)
	err := serv.Stop()

	// Set info regarless of error
	if reply != nil {
		reply.Info = serv.Info()
	}

	return err
}
