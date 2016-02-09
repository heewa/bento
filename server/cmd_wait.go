package server

import (
	"fmt"

	log "github.com/inconshreveable/log15"

	"github.com/heewa/servicetray/service"
)

// WaitArgs -
type WaitArgs struct {
	Name string
}

// WaitResponse -
type WaitResponse struct {
	Info service.Info
}

// Wait blocks until a service stops running
func (s *Server) Wait(args *WaitArgs, reply *WaitResponse) (err error) {
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

	if err = serv.Wait(); err != nil {
		return err
	}

	reply.Info = serv.Info()
	return nil
}
