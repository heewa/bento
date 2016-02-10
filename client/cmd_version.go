package client

import (
	"github.com/heewa/bento/server"

	"github.com/blang/semver"
)

// Version calls the Version cmd on the Server
func (c *Client) Version() (semver.Version, error) {
	reply := server.VersionResponse{}
	err := c.Call("Server.Version", false, &reply)

	return reply.Version, err
}
