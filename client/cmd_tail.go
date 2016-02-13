package client

import (
	"github.com/heewa/bento/server"
)

// Tail calls the Tail cmd on the Server
func (c *Client) Tail(name string, stdout, stderr bool, follow, followRestarts bool, max int) (<-chan string, <-chan string, <-chan error) {
	if followRestarts {
		follow = true
	}

	stdoutChan := make(chan string)
	stderrChan := make(chan string)
	errChan := make(chan error)

	args := server.TailArgs{
		Name:     name,
		Stdout:   stdout,
		Stderr:   stderr,
		MaxLines: max,
	}
	reply := server.TailResponse{}

	go func() {
		defer func() {
			close(stderrChan)
			close(stdoutChan)
			close(errChan)
		}()

		for {
			// Need to make a new reply struct, otherwise we'll get the same
			// reply as last time. Not sure why, some rpc quirk.
			reply = server.TailResponse{}

			if err := c.Call("Server.Tail", args, &reply); err != nil {
				errChan <- err
				return
			}

			// Set pid if called without one (whatever's latest)
			if args.Pid == 0 {
				args.Pid = reply.Pid
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
			args.MaxLines = 0

			// Look forever if following, or just once if not
			if !follow {
				break
			}
		}
	}()

	return stdoutChan, stderrChan, errChan
}
