package server

import (
	"fmt"

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
	Service service.Info
}

// Run will start a new, temp service
func (s *Server) Run(args *RunArgs, reply *RunResponse) error {
	if args.Name == "" {
		// Name it after the program, but avoid collisions by checking
		args.Name = args.Program
		for i := 1; i <= 50 && s.getService(args.Name) != nil; i++ {
			args.Name = fmt.Sprintf("%s (%d)", args.Program, i)
		}
	}

	serv, err := service.New(
		args.Name,
		args.Program,
		args.Args,
		args.Dir,
		args.Env)
	if err != nil {
		return err
	}

	if !s.addService(serv, false) {
		return fmt.Errorf("Service with name '%s' already exists", serv.Name)
	}

	if err := serv.Start(); err != nil {
		return err
	}

	reply.Service = serv.Info()
	return nil
}
