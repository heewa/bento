package client

import (
	"github.com/heewa/servicetray/server"
)

// LoadServices calls the LoadServices cmd on the Server
func (c *Client) LoadServices(serviceFilePath string) (server.LoadServicesResponse, error) {
	args := server.LoadServicesArgs{
		ServiceFilePath: serviceFilePath,
	}
	reply := server.LoadServicesResponse{}
	err := c.client.Call("Server.LoadServices", args, &reply)

	return reply, err
}
