package client

import (
	"github.com/heewa/bento/server"
	"github.com/heewa/bento/service"
)

// Stop calls the Stop cmd on the Server
func (c *Client) Stop(name string) (service.Info, error) {
	args := server.StopArgs{
		Name: name,
	}
	reply := server.StopResponse{}
	err := c.Call("Server.Stop", args, &reply)

	return reply.Info, err
}
