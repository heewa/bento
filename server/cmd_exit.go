package server

import (
	"net"

	log "github.com/inconshreveable/log15"
)

// Exit casues server to exit
func (s *Server) Exit(_ bool, _ *bool) error {
	log.Info("Exiting server")
	close(s.stop)

	log.Debug("Connecting to server to break out of listen loop")
	conn, err := net.DialUnix("unix", nil, s.fifoAddr)
	if err != nil {
		return err
	}

	// Do this in a goroutine so we can reply to RPC call before exiting
	go func() {
		conn.Close()
		log.Debug("Connected and closed")
	}()

	return nil
}
