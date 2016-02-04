package client

import (
	"github.com/heewa/servicetray/server"
	"github.com/heewa/servicetray/service"
)

// Run calls the Run cmd on the Server
func (c *Client) Run(name, program string, runArgs []string, dir string, env map[string]string) (service.Info, error) {
	args := server.RunArgs{
		Name:    name,
		Program: program,
		Args:    runArgs,
		Dir:     dir,
		Env:     env,
	}
	reply := server.RunResponse{}
	err := c.client.Call("Server.Run", args, &reply)

	return reply.Service, err
}
