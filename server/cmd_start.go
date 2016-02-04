package server

import (
	"fmt"

	log "github.com/inconshreveable/log15"

	"github.com/heewa/servicetray/service"
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
func (s *Server) Start(args StartArgs, reply *StartResponse) error {
	serv := s.getService(args.Name)
	if serv == nil {
		return fmt.Errorf("Service '%s' not found.", args.Name)
	}

	log.Info("Starting service", "service", serv.Name)
	err := serv.Start()

	// Set info regardless of error
	reply.Info = serv.Info()

	return err
}
