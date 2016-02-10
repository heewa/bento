package client

import (
	"time"

	"github.com/heewa/bento/server"
	"github.com/heewa/bento/service"
)

// Clean calls the Clean cmd on the Server
func (c *Client) Clean(pattern string, age time.Duration) ([]service.Info, []server.RemoveFailure, error) {
	args := server.CleanArgs{
		NamePattern: pattern,
		Age:         age,
	}
	reply := server.CleanResponse{}
	err := c.Call("Server.Clean", args, &reply)

	return reply.Cleaned, reply.Failed, err
}
