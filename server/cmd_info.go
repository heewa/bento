package server

import (
	"fmt"

	"github.com/heewa/servicetray/service"
)

// InfoArgs -
type InfoArgs struct {
	Name string
}

// InfoResponse -
type InfoResponse struct {
	Info service.Info
}

// Info gets info about service
func (s *Server) Info(args *InfoArgs, reply *InfoResponse) error {
	serv := s.getService(args.Name)
	if serv == nil {
		return fmt.Errorf("Service '%s' not found.", args.Name)
	}

	reply.Info = serv.Info()
	return nil
}
