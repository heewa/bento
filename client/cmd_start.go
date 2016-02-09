package client

import (
	"github.com/heewa/servicetray/server"
	"github.com/heewa/servicetray/service"
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
