package server

import (
	"fmt"
	"path/filepath"
	"time"

	log "github.com/inconshreveable/log15"

	"github.com/heewa/servicetray/config"
	"github.com/heewa/servicetray/service"
)

// RunArgs -
type RunArgs struct {
	Name       string
	Program    string
	Args       []string
	Dir        string
	Env        map[string]string
	CleanAfter time.Duration
}

// RunResponse -
type RunResponse struct {
	Service service.Info
}

// Run will start a new, temp service
func (s *Server) Run(args *RunArgs, reply *RunResponse) (err error) {
	defer func() {
		if r := recover(); r != nil {
			log.Crit("panic", "msg", r)
			err = fmt.Errorf("Server error: %v", r)
		}
	}()

	if args.Name == "" {
		// Name it after the program, but avoid collisions by checking
		args.Name = filepath.Base(args.Program)
		for i := 1; i <= 50 && s.getService(args.Name) != nil; i++ {
			args.Name = fmt.Sprintf("%s-%d", args.Program, i)
		}
	}

	conf := config.Service{
		Name:    args.Name,
		Program: args.Program,
		Args:    args.Args,
		Dir:     args.Dir,
		Env:     args.Env,

		Temp:       true,
		CleanAfter: args.CleanAfter,
	}
	if err := conf.Sanitize(); err != nil {
		return err
	}

	serv, err := service.New(conf)
	if err != nil {
		return err
	}

	if err := s.addService(serv, false); err != nil {
		return fmt.Errorf("Failed to add service (%s): %v", conf.Name, err)
	}

	// Update after creating, but before changing its state
	select {
	case s.serviceUpdates <- serv.Info():
	default:
	}

	log.Info("Running service", "service", serv.Conf.Name)
	if err := serv.Start(s.serviceUpdates); err != nil {
		return err
	}

	reply.Service = serv.Info()
	return nil
}
