package server

import (
	"fmt"

	log "github.com/inconshreveable/log15"

	"github.com/heewa/servicetray/service"
)

// ListArgs -
type ListArgs struct {
	// If true, only running services are listed
	Running bool

	// If true, only temporary services are listed
	Temp bool
}

// ListResponse -
type ListResponse struct {
	Services []service.Info
}

// List returns a list of services
func (s *Server) List(args *ListArgs, reply *ListResponse) (err error) {
	defer func() {
		if r := recover(); r != nil {
			log.Crit("panic", "msg", r)
			err = fmt.Errorf("Server error: %v", r)
		}
	}()

	for _, serv := range s.listServices() {
		if !args.Running || serv.Running() {
			reply.Services = append(reply.Services, serv.Info())
		}
	}

	return nil
}
