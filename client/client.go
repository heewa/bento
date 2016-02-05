package client

import (
	"bufio"
	"fmt"
	"net"
	"net/rpc"
	"os"
	"os/exec"
	"strings"
	"time"

	log "github.com/inconshreveable/log15"

	"github.com/heewa/servicetray/config"
)

// Client handles communicating with a Server
type Client struct {
	client *rpc.Client
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

// Connect tries to connect to a server, running a new one if necessary
func (c *Client) Connect() error {
	c.Close()

	// Wait a bit for the service to start, but not forever. Since net calls
	// block, do it in a goroutine for correct timeout behavior
	timeout := time.After(5 * time.Second)
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
		}

		// Pass args for config, which could have overriden file values
		cmd := exec.Command(
			os.Args[0],
			"--fifo", config.FifoPath,
			"--log", config.LogPath,
			"init")
		log.Debug("Server might not running, starting one", "args", strings.Join(cmd.Args, " "))

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

		go func() {
			for {
				if stdout.Scan() {
					log.Debug("Server Log", "line", stdout.Text())
				} else if stderr.Scan() {
					log.Debug("Server Error", "line", stderr.Text())
				} else {
					break
				}
			}
		}()

		if err := cmd.Start(); err != nil {
			log.Error("Failed to start server", "err", err)
			clientChan <- nil
		}

		// Keep trying to connect, it might take some time
		for {
			time.Sleep(500 * time.Millisecond)

			// Only attemp if fifo even exists
			if _, err = os.Stat(config.FifoPath); err == nil {
				client, err := rpc.Dial("unix", config.FifoPath)
				if err == nil {
					clientChan <- client
					return
				}
				log.Debug("Error connecting to server", "err", err)
			}
		}
	}()

	select {
	case client := <-clientChan:
		c.client = client
		return nil
	case <-timeout:
	}

	return fmt.Errorf("Failed to connect to client: timed out")
}

func (c *Client) Close() {
	if c.client != nil {
		c.client.Close()
		c.client = nil
	}
}
