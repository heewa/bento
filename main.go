package main

import (
	"fmt"
	"os"

	log "github.com/inconshreveable/log15"
	"gopkg.in/alecthomas/kingpin.v2"

	"github.com/heewa/servicetray/client"
	"github.com/heewa/servicetray/config"
	"github.com/heewa/servicetray/server"
	"github.com/heewa/servicetray/tray"
)

var (
	// Server Commands

	initCmd = kingpin.Command("init", "Start a new server").Hidden()

	shutdownCmd = kingpin.Command("shutdown", "Stop all services and shut the server down")

	// Commands on nothing

	listCmd     = kingpin.Command("list", "List services")
	listRunning = listCmd.Arg("running", "List only running services").Bool()
	listTemp    = listCmd.Arg("temp", "List only temp services").Bool()

	runCmd        = kingpin.Command("run", "Run command as a new service")
	runCleanAfter = runCmd.Flag("clean-after", "Remove service after it's finished running for this long. Overrides config value for this service.").Duration()
	runWait       = runCmd.Flag("wait", "Wait for it exit").Bool()
	runName       = runCmd.Flag("name", "Set a name for the service").String()
	runDir        = runCmd.Flag("dir", "Directory to run the service from").ExistingDir()
	runEnv        = runCmd.Flag("env", "Env vars to pass on to service").StringMap()
	runProg       = runCmd.Arg("program", "Program to run").Required().String()
	runArgs       = runCmd.Arg("args", "Args to pass to program, with -- prefix to prevent args from being processed here").Strings()

	cleanCmd     = kingpin.Command("clean", "Remove one or multiple stopped temporary services")
	cleanAge     = cleanCmd.Flag("age", "Only remove temp services that have been stopped for at least this long. Specify like '10s' or '5m'").Default("0s").Duration()
	cleanService = cleanCmd.Arg("service", "Service name or pattern").String()

	// Service commands

	startCmd     = kingpin.Command("start", "Start an existing service")
	startService = startCmd.Arg("service", "Service to start").Required().String()

	stopCmd     = kingpin.Command("stop", "Stop a running service")
	stopService = stopCmd.Arg("service", "Service to stop").Required().String()

	tailCmd            = kingpin.Command("tail", "Tail stdout and/or stderr of a service")
	tailNum            = tailCmd.Flag("num", "Number of lines from end to output").Short('n').Default("10").Int()
	tailFollow         = tailCmd.Flag("follow", "Continuously output new lines from service").Short('f').Bool()
	tailFollowRestarts = tailCmd.Flag("follow-restarts", "Continuously output new lines from service, even after it exits and starts again").Short('F').Bool()
	tailStdout         = tailCmd.Flag("stdout", "Tail just stdout").Bool()
	tailStderr         = tailCmd.Flag("stderr", "Tail just stderr").Bool()
	tailService        = tailCmd.Arg("service", "Service to tail").Required().String()

	infoCmd     = kingpin.Command("info", "Output info on a service")
	infoService = infoCmd.Arg("service", "Service to get info about").Required().String()

	waitCmd     = kingpin.Command("wait", "Waits for a service to stop and exits with 0 if succeeded, != 0 otherwise")
	waitService = waitCmd.Arg("service", "Service to wait for").Required().String()

	pidCmd     = kingpin.Command("pid", "Output the process id for a running service")
	pidService = pidCmd.Arg("service", "Service to get pid of").Required().String()

	// Function table for commands
	commandTable = map[string](func(*client.Client) error){
		"shutdown": handleShutdown,

		"list":  handleList,
		"run":   handleRun,
		"clean": handleClean,

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

	// Set up logging twice, cuz conf might change it, but it also logs
	setupLogging(cmd)
	if err := config.Load(cmd == "init"); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
	setupLogging(cmd)

	// All other command besides init require a connection to the server
	if cmd == "init" {
		if err := handleInit(); err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(1)
		}
	} else {
		clnt, err := client.New()
		if err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(1)
		}
		defer clnt.Close()

		if err := clnt.Connect(); err != nil {
			// Try to be helpful with the message. If fifo exists and we still
			// couldn't connect, maybe the server died before cleaning it up.
			if _, err = os.Stat(config.FifoPath); err == nil {
				err = fmt.Errorf("Failed to connect to server: timed out. It's possible the server died before cleaning up. Try removing %s and trying again", config.FifoPath)
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

// handleInit is the main entry point into the UI & server backends. This is how
// the app really "starts" up.
func handleInit() error {
	// Because the app needs to (usually) start without user explicitly driving
	// it, like on user login, we need to start the UI portion first, so if
	// there are any problems, the user can be notified.
	tray.Init()
	defer tray.Quit()

	// Start the server, so even if the services config fails to load, the
	// app is still usable.
	// TODO: take an errors channel for the tray
	server, serviceUpdates, err := server.New()
	if err != nil {
		// TODO: send error to Tray
		return err
	}

	// Hook Tray and Server together
	if err := tray.SetServer(server, serviceUpdates); err != nil {
		// TODO: send error to Tray
		return err
	}

	// Finally start the server
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
	info, err := client.Run(*runName, *runProg, *runArgs, *runDir, *runEnv, *runCleanAfter)
	if err == nil && !*runWait {
		fmt.Println(info)
	} else if err == nil {
		info, err = client.Wait(info.Name)
		if err == nil {
			fmt.Println(info)
			if info.Succeeded {
				os.Exit(0)
			}
			os.Exit(1)
		}
	}
	return err
}

func handleClean(client *client.Client) error {
	cleaned, failed, err := client.Clean(*cleanService, *cleanAge)

	if len(cleaned) > 0 {
		fmt.Printf("Removed %d services:\n", len(cleaned))
		for _, cleaned := range cleaned {
			fmt.Printf("    %s\n", cleaned)
		}
	}

	if len(failed) > 0 {
		fmt.Printf("Failed to remove %d services:\n", len(failed))
		for _, failed := range failed {
			fmt.Printf("    [%s] -- %s\n", failed.Service.Name, failed.Err)
		}
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
	stdoutChan, stderrChan, errChan := client.Tail(
		*tailService,
		*tailStdout || !*tailStderr,
		*tailStderr || !*tailStdout,
		*tailFollow,
		*tailFollowRestarts,
		*tailNum)

	// Keep outputting until done
	done := make(chan interface{})
	go func() {
		for line := range stdoutChan {
			fmt.Println(line)
		}
		done <- struct{}{}
	}()
	go func() {
		for line := range stderrChan {
			fmt.Fprintln(os.Stderr, line)
		}
		done <- struct{}{}
	}()

	// Wait for both to finish, which they will do on err also
	<-done
	<-done

	// Check err
	if err, ok := <-errChan; ok && err != nil {
		return err
	}
	return nil
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

func setupLogging(cmd string) {
	// Set client's logging to stdout, and server's if no path, or path of '-'
	logHandler := log.StdoutHandler
	if cmd == "init" && config.LogPath != "" && config.LogPath != "-" {
		var err error
		logHandler, err = log.FileHandler(config.LogPath, log.LogfmtFormat())
		if err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(1)
		}
	}

	log.Root().SetHandler(
		log.LvlFilterHandler(config.LogLevel,
			logHandler))
}
