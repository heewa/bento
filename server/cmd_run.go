package server

import (
	"github.com/heewa/servicetray/service"
)

// RunArgs are arguments to the Run cmd
type RunArgs struct {
	Name    string
	Program string
	Args    []string
	Dir     string
	Env     map[string]string
}

// RunResponse is the response from the Run cmd
type RunResponse struct {
	Service *service.Service
}

// Run will start a new, temp service
func (s *Server) Run(args *RunArgs, reply *RunResponse) error {
	serv, err := service.New(
		args.Name,
		args.Program,
		args.Args,
		args.Dir,
		args.Env)
	if err != nil {
		return err
	}

	reply.Service = serv

	return serv.Start()
}
