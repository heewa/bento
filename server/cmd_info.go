package server

import (
	"fmt"

	log "github.com/inconshreveable/log15"

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
func (s *Server) Info(args *InfoArgs, reply *InfoResponse) (err error) {
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

	reply.Info = serv.Info()
	return nil
}
