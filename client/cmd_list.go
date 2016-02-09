package client

import (
	"github.com/heewa/servicetray/server"
	"github.com/heewa/servicetray/service"
)

// List calls the List cmd on the Server
func (c *Client) List(running bool, temp bool) ([]service.Info, error) {
	args := server.ListArgs{
		Running: running,
		Temp:    temp,
	}
	reply := server.ListResponse{}
	if err := c.Call("Server.List", args, &reply); err != nil {
		return nil, err
	}

	return reply.Services, nil
}
