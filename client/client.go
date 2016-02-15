package client

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/rpc"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/blang/semver"
	log "github.com/inconshreveable/log15"

	"github.com/heewa/bento/config"
	"github.com/heewa/bento/server"
)

// Client handles communicating with a Server
type Client struct {
	client *rpc.Client

	// ServerVersion is reported by the server from an RPC call right after
	// connect
	ServerVersion semver.Version
}

// New creates a new Client
func New() (*Client, error) {
	// Resolve the net address to make sure it's valid
	_, err := net.ResolveUnixAddr("unix", config.FifoPath)
	if err != nil {
		return nil, fmt.Errorf("Bad fifo path: %v", err)
	}

	return &Client{}, nil
}

// Connect tries to connect to a server. If startServer is true, and
// connection fails, try to start a new server.
func (c *Client) Connect(startServer bool) error {
	c.Close()

	// Wait a bit for the service to start, but not forever. Since net calls
	// block, do it in a goroutine for correct timeout behavior
	clientChan := make(chan *rpc.Client)

	log.Debug("Connecting to server")
	go func() {
		// Try to connect if fifo exists
		if _, err := os.Stat(config.FifoPath); err == nil {
			client, err := rpc.Dial("unix", config.FifoPath)
			if err == nil {
				clientChan <- client
				return
			}
			log.Debug("Error connecting to server", "err", err)
		} else if !os.IsNotExist(err) {
			log.Error("Problem with fifo", "err", err)
			clientChan <- nil
			return
		} else if !startServer {
			clientChan <- nil
			return
		}

		// Pass args for config, which could have overriden file values
		cmd := exec.Command(
			os.Args[0],
			"--fifo", config.FifoPath,
			"--log", config.LogPath,
			"init")
		log.Debug("Server might not running, starting one", "args", strings.Join(cmd.Args, " "))

		// Set the process group ID to 0, so it'll create a new one, and
		// not get interrupt signals sent to this process.
		cmd.SysProcAttr = &syscall.SysProcAttr{
			Pgid:    0,
			Setpgid: true,
		}

		// Watch stdout & stderr output for server for a bit
		stdoutPipe, err := cmd.StdoutPipe()
		if err != nil {
			log.Debug("Failed to get stdout pipe for server", "err", err)
		}
		stdout := bufio.NewScanner(stdoutPipe)

		stderrPipe, err := cmd.StderrPipe()
		if err != nil {
			log.Debug("Failed to get stderr pipe for server", "err", err)
		}
		stderr := bufio.NewScanner(stderrPipe)

		if err := cmd.Start(); err != nil {
			log.Error("Failed to start server", "err", err)
			clientChan <- nil
			return
		}

		go func() {
			outDone := make(chan interface{})

			go func() {
				for stdout.Scan() {
					fmt.Println("Server: " + stdout.Text())
				}
				outDone <- struct{}{}
			}()

			go func() {
				for stderr.Scan() {
					fmt.Fprintln(os.Stderr, "Server: "+stderr.Text())
				}
				outDone <- struct{}{}
			}()

			// If stdout/stderr are done, server exited
			<-outDone
			<-outDone
			cmd.Wait()

			clientChan <- nil
			return
		}()

		// Keep trying to connect, it might take some time
		for {
			time.Sleep(500 * time.Millisecond)

			// Only attemp if fifo even exists
			if _, err = os.Stat(config.FifoPath); err == nil {
				client, err := rpc.Dial("unix", config.FifoPath)
				if err != nil {
					log.Debug("Error connecting to server", "err", err)
					return
				}

				clientChan <- client
			}
		}
	}()

	select {
	case client := <-clientChan:
		if client != nil {
			// Check that the server's version is close enough to ours
			versionReply := server.VersionResponse{}
			if err := client.Call("Server.Version", false, &versionReply); err != nil {
				return fmt.Errorf("Failed to get server version: %v", err)
			}
			c.ServerVersion = versionReply.Version

			c.client = client
			return nil
		}
	case <-time.After(5 * time.Second):
		return fmt.Errorf("Failed to connect to server: timed out")
	}

	return fmt.Errorf("Failed to connect to server")
}

// Close will end the RPC connection
func (c *Client) Close() {
	if c != nil && c.client != nil {
		c.client.Close()
		c.client = nil
	}
}

// Call wraps a regular rpc.Call to give more user-friendly error messages in
// some cases.
func (c *Client) Call(method string, args interface{}, reply interface{}) error {
	if c == nil {
		return fmt.Errorf("Failed to initialize server connection")
	}

	// Notify user about version mismatches
	if config.Version.LT(c.ServerVersion) {
		fmt.Fprintf(os.Stderr, "Note: client version (%s) is behind server version (%s). Upgrade client.\n", config.Version, c.ServerVersion)
	} else if config.Version.GT(c.ServerVersion) {
		fmt.Fprintf(os.Stderr, "Note: client version (%s) is ahead of server version (%s). Update server by restarting it.\n", config.Version, c.ServerVersion)
	}

	// Outright refuse to use a server that's too far ahead/behind.
	if c.ServerVersion.Major != config.Version.Major || c.ServerVersion.Minor != config.Version.Minor {
		return fmt.Errorf("Client & Server versions are incompatible.")
	}

	// On pre-release builds, refuse any mismatch - things are changing too fast
	if !config.Version.Equals(c.ServerVersion) && (len(config.Version.Pre) > 0 || len(c.ServerVersion.Pre) > 0) {
		return fmt.Errorf("Client & Server versions are incompatible.")
	}

	err := c.client.Call(method, args, reply)
	if err == io.EOF || err == io.ErrUnexpectedEOF {
		err = fmt.Errorf("Lost connection to backend server during a call to %s", method)
	}

	return err
}
