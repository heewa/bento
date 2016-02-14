package server

import (
	"fmt"
	"path/filepath"
	"time"

	log "github.com/inconshreveable/log15"

	"github.com/heewa/bento/config"
	"github.com/heewa/bento/service"
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
		// Name it after the program, but avoid collisions by checking.
		prog := filepath.Base(args.Program)
		if srvc := s.getService(prog); srvc == nil {
			args.Name = prog
		} else if srvc.Conf.Temp && !srvc.Running() {
			// Colliding with an ended temporary service, just replace it.
			if err := s.removeService(prog); err == nil {
				args.Name = prog
			}
		}

		// If either that didn't work, append a number to name
		if args.Name == "" {
			for i := 1; i <= 50 && s.getService(prog) != nil; i++ {
				prog = fmt.Sprintf("%s-%d", prog, i)
			}
			if s.getService(prog) != nil {
				return fmt.Errorf("Failed to name the service")
			}
			args.Name = prog
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

	log.Debug("Running service", "service", serv.Conf.Name)
	if err := serv.Start(s.serviceUpdates); err != nil {
		return err
	}

	reply.Service = serv.Info()
	return nil
}
