package client

import (
	"github.com/heewa/bento/server"
	"github.com/heewa/bento/service"
)

// Start calls the Start cmd on the Server
func (c *Client) Start(name string) (service.Info, error) {
	args := server.StartArgs{
		Name: name,
	}
	reply := server.StartResponse{}
	err := c.Call("Server.Start", args, &reply)

	return reply.Info, err
}
