package client

// Shutdown calls the Exit cmd on the Server
func (c *Client) Shutdown() error {
	var nothing bool
	return c.Call("Server.Exit", nothing, nothing)
}
