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
	Services []*service.Service
}

func (s *Server) List(args *ListArgs, reply *ListResponse) error {
	return nil
}
