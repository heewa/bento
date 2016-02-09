package client

// Shutdown calls the Exit cmd on the Server
func (c *Client) Shutdown() error {
	return c.Call("Server.Exit", false, nil)
}
