package client

// Shutdown calls the Exit cmd on the Server
func (c *Client) Shutdown() error {
	var nothing bool
	return c.client.Call("Server.Exit", nothing, nothing)
}
