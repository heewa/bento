package server

import (
	"github.com/heewa/servicetray/service"
)

type ListArgs struct {
	// If true, only running services are listed
	Running bool

	// If true, only temporary services are listed
	Temp bool
}

type ListResponse struct {
	Services []service.Info
}

func (s *Server) List(args *ListArgs, reply *ListResponse) error {
	for _, serv := range s.listServices() {
		if !args.Running || serv.Running() {
			reply.Services = append(reply.Services, serv.Info())
		}
	}

	return nil
}
