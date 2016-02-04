package client

import (
	"github.com/heewa/servicetray/server"
	"github.com/heewa/servicetray/service"
)

// Info calls the Info cmd on the Server
func (c *Client) Info(name string) (service.Info, error) {
	args := server.InfoArgs{
		Name: name,
	}
	reply := server.InfoResponse{}
	err := c.client.Call("Server.Info", args, &reply)

	return reply.Info, err
}
