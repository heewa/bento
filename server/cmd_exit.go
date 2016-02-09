package server

import (
	"fmt"
	"net"

	log "github.com/inconshreveable/log15"
)

// Exit casues server to exit
func (s *Server) Exit(_ bool, _ *bool) (err error) {
	defer func() {
		if r := recover(); r != nil {
			log.Crit("panic", "msg", r)
			err = fmt.Errorf("Server error: %v", r)
		}
	}()

	log.Info("Exiting server")
	select {
	case s.stop <- struct{}{}:
	default:
		// Someone already asked to stop, so it's fine
	}

	log.Debug("Connecting to server to break out of listen loop")
	var conn *net.UnixConn
	conn, err = net.DialUnix("unix", nil, s.fifoAddr)
	if err != nil {
		return
	}

	// Do this in a goroutine so we can reply to RPC call before exiting
	go func() {
		conn.Close()
		log.Debug("Connected and closed")
	}()

	return
}
