package main

import (
	"fmt"
	"net"
	"net/rpc"
	"os"
	"os/exec"

	log "github.com/inconshreveable/log15"
	"gopkg.in/alecthomas/kingpin.v2"

	"github.com/heewa/servicetray/server"
	"github.com/heewa/servicetray/service"
)

var (
	// Global flags

	verbosity = kingpin.Flag("verbose", "Increase log verbosity, can be used multiple times").Short('v').Counter()

	fifo = kingpin.Flag("fifo", "Path to fifo used to communicate between client and server").Default("/var/run/servicetray.fifo").String()

	// Server Commands

	initCmd = kingpin.Command("init", "Start a new server").Hidden()

	exitCmd = kingpin.Command("exit", "Exit the server")

	// Client Commands

	listCmd     = kingpin.Command("list", "List services")
	listRunning = listCmd.Arg("running", "List only running services").Bool()
	listTemp    = listCmd.Arg("temp", "List only temp services").Bool()

	startCmd     = kingpin.Command("start", "Start an existing service")
	startService = startCmd.Arg("service", "Service to start").Required().String()

	stopCmd     = kingpin.Command("stop", "Stop a running service")
	stopService = stopCmd.Arg("service", "Service to stop").Required().String()

	runCmd  = kingpin.Command("run", "Run a service, but don't save it when it exits")
	runName = runCmd.Flag("name", "Set a name for the temporary service").String()
	runDir  = runCmd.Flag("dir", "Directory to run the service from").ExistingDir()
	runEnv  = runCmd.Flag("env", "Env vars to pass on to service").StringMap()
	runProg = runCmd.Arg("program", "Program to run").Required().String()
	runArgs = runCmd.Arg("args", "Args to pass to program, with -- prefix to prevent args from being processed here").Strings()

	tailCmd     = kingpin.Command("tail", "Tail stdout and/or stderr of a service")
	tailStdOut  = tailCmd.Flag("stdout", "Tail just stdout").Bool()
	tailStdErr  = tailCmd.Flag("stderr", "Tail just stderr").Bool()
	tailService = tailCmd.Arg("service", "Service to tail").Required().String()

	// Function table for commands
	commandTable = map[string](func(*rpc.Client) error){
		"init":  handleInit,
		"exit":  handleExit,
		"list":  handleList,
		"start": handleStart,
		"stop":  handleStop,
		"run":   handleRun,
		"tail":  handleTail,
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

	// If the file doesn't exist, server isn't running (there)
	_, err = os.Stat(*fifo)
	if os.IsNotExist(err) {
		// Start a server
		log.Debug("Server not running, starting one")
		cmd := exec.Command(os.Args[0], "--fifo", *fifo, "-vv", "init")
		if err := cmd.Start(); err != nil {
			return nil, fmt.Errorf("Failed to start server: %v", err)
		}
	}

	log.Debug("Connecting to server")
	client, err := rpc.Dial("unix", addr.String())
	if err != nil {
		return nil, fmt.Errorf("Failed to connect to server: %v", err)
	}

	return client, nil
}

func handleInit(_ *rpc.Client) error {
	server, err := server.New(*fifo)
	if err != nil {
		return err
	}

	var nothing bool
	return server.Start(nothing, &nothing)
}

func handleExit(client *rpc.Client) error {
	var nothing bool
	return client.Call("Server.Exit", nothing, &nothing)
}

func handleList(client *rpc.Client) error {
	services := []*service.Service{}

	args := server.ListArgs{}
	reply := server.ListResponse{}
	if err := client.Call("Server.List", args, &reply); err != nil {
		return err
	}

	if *listRunning {
		all, services := services[:], services[:0]
		for _, serv := range all {
			if serv.Running() {
				services = append(services, serv)
			}
		}
	}

	if *listTemp {
		all, services := services[:], services[:0]
		for _, serv := range all {
			if serv.Running() {
				services = append(services, serv)
			}
		}
	}

	return nil
}

func handleStart(client *rpc.Client) error {

	fmt.Printf("starting %v\n", *startService)

	return nil
}

func handleStop(client *rpc.Client) error {
	fmt.Printf("stopping %v\n", *stopService)

	return nil
}

func handleRun(client *rpc.Client) error {
	serv, err := service.New(*runProg, *runArgs)
	if err != nil {
		return err
	}
	if *runName != "" {
		serv.Name = *runName
	}
	if *runDir != "" {
		serv.Dir = *runDir
	}
	if *runEnv != nil {
		serv.Env = *runEnv
	}

	fmt.Printf("running %#v\n", serv)

	if err := serv.Start(); err != nil {
		return err
	}

	return nil
}

func handleTail(client *rpc.Client) error {
	fmt.Printf("tail stdout:%v stderr:%v %v\n", !*tailStdErr || *tailStdOut, !*tailStdOut || *tailStdErr, *tailService)

	return nil
}
