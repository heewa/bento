package server

import (
	"fmt"
	"path/filepath"
	"time"

	log "github.com/inconshreveable/log15"

	"github.com/heewa/bento/service"
)

// CleanArgs -
type CleanArgs struct {
	NamePattern string
	Age         time.Duration
}

// RemoveFailure -
type RemoveFailure struct {
	Service service.Info
	Err     string
}

// CleanResponse -
type CleanResponse struct {
	Cleaned []service.Info
	Failed  []RemoveFailure
}

// Clean will start a new, temp service
func (s *Server) Clean(args CleanArgs, reply *CleanResponse) (err error) {
	defer func() {
		if r := recover(); r != nil {
			log.Crit("panic", "msg", r)
			err = fmt.Errorf("Server error: %v", r)
		}
	}()

	now := time.Now()

	// Precheck pattern
	if args.NamePattern == "" {
		args.NamePattern = "*"
	}
	if _, err := filepath.Match(args.NamePattern, ""); err != nil {
		return fmt.Errorf("Bad service name pattern: %v", err)
	}

	log.Info("Cleanning services", "pattern", args.NamePattern, "age", args.Age)
	for _, srvc := range s.listServices() {
		info := srvc.Info()
		matches, _ := filepath.Match(args.NamePattern, info.Name)

		if info.Temp && !info.Running && matches && (args.Age == 0 || now.Sub(info.EndTime) >= args.Age) {
			if err := s.removeService(info.Name); err != nil {
				log.Warn("Failed to remove a service", "name", info.Name, "err", err)
				reply.Failed = append(reply.Failed, RemoveFailure{info, err.Error()})
			} else {
				reply.Cleaned = append(reply.Cleaned, info)
			}
		}
	}

	return nil
}
