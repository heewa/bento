package client

import (
	"github.com/heewa/servicetray/server"
	"github.com/heewa/servicetray/service"
)

// Wait calls the Wait cmd on the Server
func (c *Client) Wait(name string) (service.Info, error) {
	args := server.WaitArgs{
		Name: name,
	}
	reply := server.WaitResponse{}
	err := c.client.Call("Server.Wait", args, &reply)

	return reply.Info, err
}
