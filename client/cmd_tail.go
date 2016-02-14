package client

import (
	"github.com/heewa/bento/server"
)

// Tail calls the Tail cmd on the Server
func (c *Client) Tail(name string, stdout, stderr bool, follow, followRestarts bool, pid, max int) (<-chan string, <-chan string, <-chan error) {
	if followRestarts {
		follow = true
	}

	stdoutChan := make(chan string, 100)
	stderrChan := make(chan string, 100)
	errChan := make(chan error, 1) // needs to be buffered cuz client might wait

	args := server.TailArgs{
		Name:     name,
		Pid:      pid,
		MaxLines: max,
		Follow:   follow,
	}

	if max > 0 {
		// Start that many from end
		args.Index = -1 * max
	} else {
		args.Index = -100000
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

			// Send lines down channels
			for _, line := range reply.Lines {
				if line.Stderr {
					stderrChan <- line.Line
				} else {
					stdoutChan <- line.Line
				}
			}

			// If there aren't any more lines from this process, stop, unless
			// we're following restarts.
			if !follow {
				// Just wanted one tail call, so stop here
				return
			} else if !followRestarts && (reply.EOF || reply.NextPid == 0) {
				// We're not following restart, so stop after end of input or
				// no next proc (EOF is never true on first call with no pid).
				return
			}

			// Set up for next fetch
			args = server.TailArgs{
				Name:     name,
				Pid:      reply.NextPid,
				MaxLines: 0,
				Index:    reply.NextIndex,
				Follow:   follow,
			}
		}
	}()

	return stdoutChan, stderrChan, errChan
}
