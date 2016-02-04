package client

import (
	"github.com/heewa/servicetray/server"
)

// Tail calls the Tail cmd on the Server
func (c *Client) Tail(name string, stdout, stderr bool, follow, followRestarts bool) (<-chan string, <-chan string, <-chan error) {
	if followRestarts {
		follow = true
	}

	stdoutChan := make(chan string)
	stderrChan := make(chan string)
	errChan := make(chan error)

	args := server.TailArgs{
		Name:   name,
		Stdout: stdout,
		Stderr: stderr,
	}
	reply := server.TailResponse{}

	go func() {
		defer func() {
			close(stderrChan)
			close(stdoutChan)
			close(errChan)
		}()

		// Make sure service exists, and get pid
		info, err := c.Info(name)
		if err != nil {
			errChan <- err
			return
		}
		args.Pid = info.Pid

		for {
			// Need to make a new reply struct, otherwise we'll get the same
			// reply as last time. Not sure why, some rpc quirk.
			reply = server.TailResponse{}

			if err := c.client.Call("Server.Tail", args, &reply); err != nil {
				errChan <- err
				return
			}

			if reply.Pid != args.Pid && !followRestarts {
				// These lines are already from a diff process, so we're done
				return
			}

			// Send lines down channels
			for _, line := range reply.StderrLines {
				stderrChan <- line
			}
			for _, line := range reply.StdoutLines {
				stdoutChan <- line
			}

			// Set up for next fetch
			args.StdoutSinceLine = reply.StdoutSinceLine
			args.StderrSinceLine = reply.StderrSinceLine
			args.Pid = reply.Pid

			// Look forever if following, or just once if not
			if !follow {
				break
			}
		}
	}()

	return stdoutChan, stderrChan, errChan
}
