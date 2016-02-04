package main

import (
	"fmt"
	"os"
	"os/user"

	log "github.com/inconshreveable/log15"
	"gopkg.in/alecthomas/kingpin.v2"

	"github.com/heewa/servicetray/client"
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

	waitCmd     = kingpin.Command("wait", "Waits for a service to stop and exits with 0 if succeeded, != 0 otherwise")
	waitService = waitCmd.Arg("service", "Service to wait for").Required().String()

	pidCmd     = kingpin.Command("pid", "Output the process id for a running service")
	pidService = pidCmd.Arg("service", "Service to get pid of").Required().String()

	// Function table for commands
	commandTable = map[string](func(*client.Client) error){
		"shutdown": handleShutdown,

		"list": handleList,
		"run":  handleRun,

		"start": handleStart,
		"stop":  handleStop,
		"tail":  handleTail,
		"info":  handleInfo,
		"wait":  handleWait,
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
	if cmd == "init" {
		if err := handleInit(); err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(1)
		}
	} else {
		clnt, err := client.New(*fifo)
		if err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(1)
		}
		defer clnt.Close()

		if err := clnt.Connect(); err != nil {
			// Try to be helpful with the message. If fifo exists and we still
			// couldn't connect, maybe the server died before cleaning it up.
			if _, err = os.Stat(*fifo); err == nil {
				err = fmt.Errorf("Failed to connect to server: timed out. It's possible the server died before cleaning up. Try removing %s and trying again", *fifo)
			}
			fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(1)
		}

		if fn, ok := commandTable[cmd]; !ok {
			fmt.Fprintf(os.Stderr, "Unknown command: %s\n", cmd)
			os.Exit(1)
		} else {
			if err := fn(clnt); err != nil {
				fmt.Fprintln(os.Stderr, err.Error())
				os.Exit(1)
			}
		}
	}
}

func handleInit() error {
	server, err := server.New(*fifo)
	if err != nil {
		return err
	}

	var nothing bool
	return server.Init(nothing, &nothing)
}

func handleShutdown(client *client.Client) error {
	return client.Shutdown()
}

func handleList(client *client.Client) error {
	services, err := client.List(*listRunning, *listTemp)

	for _, serv := range services {
		fmt.Println(serv)
	}

	return err
}

func handleRun(client *client.Client) error {
	info, err := client.Run(*runName, *runProg, *runArgs, *runDir, *runEnv)
	if err == nil {
		fmt.Println(info)
	}
	return err
}

func handleStart(client *client.Client) error {
	info, err := client.Start(*startService)
	if err == nil {
		fmt.Println(info)
	}
	return err
}

func handleStop(client *client.Client) error {
	info, err := client.Stop(*stopService)
	if err == nil {
		fmt.Println(info)
	}
	return err
}

func handleTail(client *client.Client) error {
	return fmt.Errorf("Functionality not implemented")
}

func handleInfo(client *client.Client) error {
	info, err := client.Info(*infoService)
	if err == nil {
		fmt.Println(info)
	}
	return err
}

func handleWait(client *client.Client) error {
	info, err := client.Wait(*waitService)
	if err != nil {
		return err
	}

	if info.Succeeded {
		os.Exit(0)
	}
	os.Exit(1)

	return nil
}

func handlePid(client *client.Client) error {
	info, err := client.Info(*infoService)
	if err == nil {
		fmt.Println(info.Pid)
	}
	return err
}
