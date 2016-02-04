package main

import (
	"bufio"
	"fmt"
	"net"
	"net/rpc"
	"os"
	"os/exec"
	"os/user"
	"strings"
	"time"

	log "github.com/inconshreveable/log15"
	"gopkg.in/alecthomas/kingpin.v2"

	"github.com/heewa/servicetray/server"
)

var (
	// Global flags

	verbosity = kingpin.Flag("verbose", "Increase log verbosity, can be used multiple times").Short('v').Counter()

	fifo = kingpin.Flag("fifo", "Path to fifo used to communicate between client and server").Default("~/.servicetray.fifo").String()

	// Server Commands

	initCmd = kingpin.Command("init", "Start a new server").Hidden()

	shutdownCmd = kingpin.Command("shutdown", "Stop all services and shut the server down")

	// Commands on nothing

	listCmd     = kingpin.Command("list", "List services")
	listRunning = listCmd.Arg("running", "List only running services").Bool()
	listTemp    = listCmd.Arg("temp", "List only temp services").Bool()

	runCmd  = kingpin.Command("run", "Run command as a new service")
	runName = runCmd.Flag("name", "Set a name for the service").String()
	runDir  = runCmd.Flag("dir", "Directory to run the service from").ExistingDir()
	runEnv  = runCmd.Flag("env", "Env vars to pass on to service").StringMap()
	runProg = runCmd.Arg("program", "Program to run").Required().String()
	runArgs = runCmd.Arg("args", "Args to pass to program, with -- prefix to prevent args from being processed here").Strings()

	// Service commands

	startCmd     = kingpin.Command("start", "Start an existing service")
	startService = startCmd.Arg("service", "Service to start").Required().String()

	stopCmd     = kingpin.Command("stop", "Stop a running service")
	stopService = stopCmd.Arg("service", "Service to stop").Required().String()

	tailCmd     = kingpin.Command("tail", "Tail stdout and/or stderr of a service")
	tailStdOut  = tailCmd.Flag("stdout", "Tail just stdout").Bool()
	tailStdErr  = tailCmd.Flag("stderr", "Tail just stderr").Bool()
	tailService = tailCmd.Arg("service", "Service to tail").Required().String()

	infoCmd     = kingpin.Command("info", "Output info on a service")
	infoService = infoCmd.Arg("service", "Service to get info about").Required().String()

	pidCmd     = kingpin.Command("pid", "Output the process id for a running service")
	pidService = pidCmd.Arg("service", "Service to get pid of").Required().String()

	// Function table for commands
	commandTable = map[string](func(*rpc.Client) error){
		"init":     handleInit,
		"shutdown": handleShutdown,

		"list": handleList,
		"run":  handleRun,

		"start": handleStart,
		"stop":  handleStop,
		"tail":  handleTail,
		"info":  handleInfo,
		"pid":   handlePid,
	}
)

func main() {
	cmd := kingpin.Parse()

	logLevel := log.Lvl(*verbosity) + log.LvlWarn
	if logLevel > log.LvlDebug {
		logLevel = log.LvlDebug
	}

	log.Root().SetHandler(
		log.LvlFilterHandler(logLevel,
			log.StdoutHandler))

	// Fix up fifo path if it contains a ~
	if len(*fifo) > 2 && (*fifo)[:2] == "~/" {
		if usr, err := user.Current(); err == nil {
			*fifo = fmt.Sprintf("%s/%s", usr.HomeDir, (*fifo)[2:])
		}
	}

	// All other command besides init require a connection to the server
	var client *rpc.Client
	var err error

	if cmd != "init" {
		client, err = getClient()
		if err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(1)
		}
		defer client.Close()
	}

	if fn, ok := commandTable[cmd]; !ok {
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", cmd)
		os.Exit(1)
	} else {
		if err := fn(client); err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(1)
		}
	}
}

func getClient() (*rpc.Client, error) {
	// Resolve the net address to make sure it's valid
	addr, err := net.ResolveUnixAddr("unix", *fifo)
	if err != nil {
		return nil, fmt.Errorf("Bad fifo path: %v", err)
	}

	// Wait a bit for the service to start, but not forever. Since net calls
	// block, do it in a goroutine for correct timeout behavior
	timeout := time.After(5 * time.Second)
	clientChan := make(chan *rpc.Client)

	log.Debug("Connecting to server")
	go func() {
		// Try to connect if fifo exists
		if _, err = os.Stat(*fifo); err == nil {
			client, err := rpc.Dial("unix", addr.String())
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

		cmd := exec.Command(os.Args[0], "--fifo", *fifo, "init")
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
			if _, err = os.Stat(*fifo); err == nil {
				client, err := rpc.Dial("unix", addr.String())
				if err == nil {
					clientChan <- client
					return
				}
				log.Debug("Error connecting to server", "err", err)
			}
		}
	}()

	var client *rpc.Client
	select {
	case client = <-clientChan:
	case <-timeout:
	}

	if client == nil {
		// Try to be helpful with the message. If fifo exists and we still
		// couldn't connect, maybe the server died before cleaning it up.
		if _, err = os.Stat(*fifo); err == nil {
			return nil, fmt.Errorf("Failed to connect to server: timed out. It's possible the server died before cleaning up. Try removing %s and trying again", *fifo)
		}

		return nil, fmt.Errorf("Failed to connect to server: timed out")
	}
	return client, nil
}

func handleInit(_ *rpc.Client) error {
	server, err := server.New(*fifo)
	if err != nil {
		return err
	}

	var nothing bool
	return server.Init(nothing, &nothing)
}

func handleShutdown(client *rpc.Client) error {
	var nothing bool
	return client.Call("Server.Exit", nothing, &nothing)
}

func handleList(client *rpc.Client) error {
	args := server.ListArgs{
		Running: *listRunning,
		Temp:    *listTemp,
	}
	reply := server.ListResponse{}
	if err := client.Call("Server.List", args, &reply); err != nil {
		return err
	}

	fmt.Printf("%d services\n", len(reply.Services))
	for _, serv := range reply.Services {
		fmt.Printf("\n  %s\n  %#v\n", serv.Name, serv)
	}

	return nil
}

func handleRun(client *rpc.Client) error {
	args := server.RunArgs{
		Name:    *runName,
		Program: *runProg,
		Args:    *runArgs,
		Dir:     *runDir,
		Env:     *runEnv,
	}
	reply := server.RunResponse{}

	if err := client.Call("Server.Run", args, &reply); err != nil {
		return err
	}

	fmt.Printf("running %#v\n", reply.Service)
	return nil
}

func handleStart(client *rpc.Client) error {
	args := server.StartArgs{
		Name: *startService,
	}
	reply := server.StartResponse{}

	if err := client.Call("Server.Start", args, &reply); err != nil {
		return err
	}

	fmt.Printf("started: %#v\n", reply.Info)
	return nil
}

func handleStop(client *rpc.Client) error {
	args := server.StopArgs{
		Name: *stopService,
	}
	reply := server.StopResponse{}

	if err := client.Call("Server.Stop", args, &reply); err != nil {
		return err
	}

	fmt.Printf("stopped: %#v\n", reply.Info)
	return nil
}

func handleTail(client *rpc.Client) error {
	return fmt.Errorf("Functionality not implemented")
}

func handleInfo(client *rpc.Client) error {
	args := server.InfoArgs{
		Name: *infoService,
	}
	reply := server.InfoResponse{}

	if err := client.Call("Server.Info", args, &reply); err != nil {
		return err
	}

	fmt.Printf("%#v\n", reply.Info)
	return nil
}

func handlePid(client *rpc.Client) error {
	args := server.InfoArgs{
		Name: *pidService,
	}
	reply := server.InfoResponse{}

	if err := client.Call("Server.Info", args, &reply); err != nil {
		return err
	}

	fmt.Println(reply.Info.Pid)
	return nil
}
