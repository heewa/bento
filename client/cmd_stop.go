package client

import (
	"github.com/heewa/servicetray/server"
	"github.com/heewa/servicetray/service"
)

// Stop calls the Stop cmd on the Server
func (c *Client) Stop(name string) (service.Info, error) {
	args := server.StopArgs{
		Name: name,
	}
	reply := server.StopResponse{}
	err := c.client.Call("Server.Stop", args, &reply)

	return reply.Info, err
}
