package server

import (
	"fmt"

	"github.com/blang/semver"
	log "github.com/inconshreveable/log15"

	"github.com/heewa/servicetray/config"
)

// VersionResponse -
type VersionResponse struct {
	Version semver.Version
}

// Version gets the version of the server
func (s *Server) Version(_ bool, reply *VersionResponse) (err error) {
	defer func() {
		if r := recover(); r != nil {
			log.Crit("panic", "msg", r)
			err = fmt.Errorf("Server error: %v", r)
		}
	}()

	if reply != nil {
		reply.Version = config.Version
	}

	return nil
}
