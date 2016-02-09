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
	listRunning = listCmd.Flag("running", "List only running services").Bool()
	listTemp    = listCmd.Flag("temp", "List only temp services").Bool()
	listLong    = listCmd.Flag("long", "List more info").Short('l').Bool()

	reloadCmd = kingpin.Command("reload", "Reload services conf file")

	runCmd        = kingpin.Command("run", "Run command as a new service")
	runCleanAfter = runCmd.Flag("clean-after", "Remove service after it's finished running for this long. Overrides config value for this service.").Duration()
	runName       = runCmd.Flag("name", "Set a name for the service").String()
	runDir        = runCmd.Flag("dir", "Directory to run the service from").ExistingDir()
	runEnv        = runCmd.Flag("env", "Env vars to pass on to service").StringMap()
	runProg       = runCmd.Arg("program", "Program to run").Required().String()
	runTail       = runCmd.Flag("tail", "Tail output after starting the service").Bool()
	runArgs       = runCmd.Arg("args", "Args to pass to program, with -- prefix to prevent args from being processed here").Strings()

	cleanCmd     = kingpin.Command("clean", "Remove one or multiple stopped temporary services")
	cleanAge     = cleanCmd.Flag("age", "Only remove temp services that have been stopped for at least this long. Specify like '10s' or '5m'").Default("0s").Duration()
	cleanService = cleanCmd.Arg("service", "Service name or pattern").String()

	// Service commands

	startCmd     = kingpin.Command("start", "Start an existing service")
	startTail    = startCmd.Flag("tail", "Tail output after starting the service").Bool()
	startService = startCmd.Arg("service", "Service to start").Required().String()

	stopCmd     = kingpin.Command("stop", "Stop a running service")
	stopTail    = stopCmd.Flag("tail", "Tail output of the service while stopping").Bool()
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

		"list":   handleList,
		"reload": handleReload,
		"run":    handleRun,
		"clean":  handleClean,

		"start": handleStart,
		"stop":  handleStop,
		"tail":  handleTail,
		"info":  handleInfo,
		"wait":  handleWait,
		"pid":   handlePid,
	}
)

func exitOnErr(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

func main() {
	cmd := kingpin.Parse()

	// Set up logging twice, cuz conf might change it, but it also logs
	exitOnErr(setupLogging(cmd == "init", "-"))
	exitOnErr(config.Load(cmd == "init"))
	exitOnErr(setupLogging(cmd == "init", config.LogPath))

	// All other command besides init require a connection to the server
	if cmd == "init" {
		exitOnErr(handleInit())
	} else {
		clnt, err := client.New()
		exitOnErr(err)
		defer clnt.Close()

		exitOnErr(clnt.Connect())

		if fn, ok := commandTable[cmd]; !ok {
			fmt.Fprintf(os.Stderr, "Unknown command: %s\n", cmd)
			os.Exit(1)
		} else {
			exitOnErr(fn(clnt))
		}
	}
}

// handleInit is the main entry point into the UI & server backends. This is how
// the app really "starts" up.
func handleInit() error {
	// Start the UI
	tray.Init()
	defer tray.Quit()

	// Create a Server
	srvr, serviceUpdates, err := server.New()
	if err != nil {
		return err
	}

	// Hook Tray and Server together
	if err := tray.SetServer(srvr, serviceUpdates); err != nil {
		return err
	}

	// Start the server
	errChan := make(chan error)
	go func() {
		errChan <- srvr.Init(false, nil)
	}()

	// Load services config
	if config.ServiceConfigFile != "" {
		reply := server.LoadServicesResponse{}
		if err := srvr.LoadServices(server.LoadServicesArgs{config.ServiceConfigFile}, &reply); err != nil {
			// Shut the server down before leaving
			if shutdownErr := srvr.Exit(false, nil); shutdownErr != nil {
				log.Error("Failed to shut down server", "err", shutdownErr)
			}

			return err
		}
	}

	// Block on server exit
	return <-errChan
}

func handleShutdown(client *client.Client) error {
	return client.Shutdown()
}

func handleList(client *client.Client) error {
	services, err := client.List(*listRunning, *listTemp)

	for _, serv := range services {
		if *listLong {
			fmt.Println(serv.LongString())
		} else {
			fmt.Println(serv)
		}
	}

	return err
}

func handleReload(client *client.Client) error {
	reply, err := client.LoadServices(config.ServiceConfigFile)

	if len(reply.NewServices) > 0 {
		fmt.Printf("Added %d new services:\n", len(reply.NewServices))
		for _, srvc := range reply.NewServices {
			fmt.Printf("    %s\n", srvc)
		}
		fmt.Println("")
	}

	if len(reply.UpdatedServices) > 0 {
		fmt.Printf("Updated %d existing services:\n", len(reply.UpdatedServices))
		for _, srvc := range reply.UpdatedServices {
			fmt.Printf("    %s\n", srvc)
		}
		fmt.Println("")
	}

	if len(reply.DeprecatedServices) > 0 {
		fmt.Printf("Marked %d running, but removed services for removal after exit:\n", len(reply.DeprecatedServices))
		for _, srvc := range reply.DeprecatedServices {
			fmt.Printf("    %s\n", srvc)
		}
		fmt.Println("")
	}

	if len(reply.RemovedServices) > 0 {
		fmt.Printf("Removed %d services:\n", len(reply.RemovedServices))
		for _, name := range reply.RemovedServices {
			fmt.Printf("    %s\n", name)
		}
		fmt.Println("")
	}

	return err
}

func handleRun(client *client.Client) error {
	info, err := client.Run(*runName, *runProg, *runArgs, *runDir, *runEnv, *runCleanAfter)
	if err == nil && !*runTail {
		fmt.Println(info)
	} else if err == nil {
		*tailService = info.Name
		*tailFollow = true
		err = handleTail(client)
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

		if *startTail {
			*tailService = info.Name
			*tailFollow = true
			err = handleTail(client)
		}
	}
	return err
}

func handleStop(client *client.Client) error {
	if *stopTail {
		*tailService = *stopService
		*tailFollow = true
		go handleTail(client)
	}

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
	info, err := client.Info(*pidService)
	if err == nil {
		fmt.Println(info.Pid)
	}
	return err
}

func setupLogging(isServer bool, logPath string) error {
	// Set client's logging to stdout, and server's if no path, or path of '-'
	logHandler := log.StdoutHandler
	if isServer && logPath != "" && logPath != "-" {
		var err error
		logHandler, err = log.FileHandler(logPath, log.LogfmtFormat())
		if err != nil {
			return err
		}
	}

	log.Root().SetHandler(
		log.LvlFilterHandler(config.LogLevel,
			logHandler))

	return nil
}
