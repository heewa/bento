package client

import (
	"time"

	"github.com/heewa/bento/server"
	"github.com/heewa/bento/service"
)

// Run calls the Run cmd on the Server
func (c *Client) Run(name, program string, runArgs []string, dir string, env map[string]string, cleanAfter time.Duration) (service.Info, error) {
	args := server.RunArgs{
		Name:       name,
		Program:    program,
		Args:       runArgs,
		Dir:        dir,
		Env:        env,
		CleanAfter: cleanAfter,
	}
	reply := server.RunResponse{}
	err := c.Call("Server.Run", args, &reply)

	return reply.Service, err
}
