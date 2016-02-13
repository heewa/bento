package main

import (
	"fmt"
	"os"
	"os/user"
	"sort"
	"sync"

	log "github.com/inconshreveable/log15"
	"gopkg.in/alecthomas/kingpin.v2"

	"github.com/heewa/bento/client"
	"github.com/heewa/bento/config"
	"github.com/heewa/bento/logging"
	"github.com/heewa/bento/server"
	"github.com/heewa/bento/service"
	"github.com/heewa/bento/tray"
)

var (
	// Main use-case commands

	listCmd     = kingpin.Command("list", "List services").Alias("ls")
	listRunning = listCmd.Flag("running", "List only running services").Bool()
	listTemp    = listCmd.Flag("temp", "List only temp services").Bool()
	listLong    = listCmd.Flag("long", "List more info").Short('l').Bool()

	startCmd     = kingpin.Command("start", "Start an existing service")
	startTail    = startCmd.Flag("tail", "Tail output after starting the service").Bool()
	startService = startCmd.Arg("service", "Service to start").Required().String()

	stopCmd     = kingpin.Command("stop", "Stop a running service")
	stopTail    = stopCmd.Flag("tail", "Tail output of the service while stopping").Bool()
	stopService = stopCmd.Arg("service", "Service to stop").Required().String()

	reloadCmd = kingpin.Command("reload", "Reload services conf file")

	runCmd        = kingpin.Command("run-once", "Create a new, temporary service and start it")
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

	// Other service commands

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

	// Server and management

	initCmd = kingpin.Command("init", "Start a new server").Hidden()

	shutdownCmd = kingpin.Command("shutdown", "Stop all services and shut the server down")

	versionCmd = kingpin.Command("version", "List client & server versions")

	// Function table for commands
	commandTable = map[string](func(*client.Client) error){
		"shutdown": handleShutdown,

		"version":  handleVersion,
		"list":     handleList,
		"reload":   handleReload,
		"run-once": handleRun,
		"clean":    handleClean,

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
	exitOnErr(logging.Config(cmd == "init", "-", log.LvlInfo))
	exitOnErr(config.Load(cmd == "init"))
	exitOnErr(logging.Config(cmd == "init", config.LogPath, config.LogLevel))

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
	// Since we're a server, it shouldn't matter where we were started from. So
	// change to user's home dir.
	if usr, err := user.Current(); err != nil || os.Chdir(usr.HomeDir) != nil {
		// That didn't work. Use root.
		if err = os.Chdir("/"); err != nil {
			return fmt.Errorf("Failed to set server's dir: %v", err)
		}
	}

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
		args := server.LoadServicesArgs{
			ServiceFilePath: config.ServiceConfigFile,
		}
		reply := server.LoadServicesResponse{}
		if err := srvr.LoadServices(args, &reply); err != nil {
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

func handleVersion(client *client.Client) error {
	fmt.Printf("client version: %s\n", config.Version)
	fmt.Printf("server version: %s\n", client.ServerVersion)

	if config.Version.GT(client.ServerVersion) {
		fmt.Println("Client is ahead of server - restart server to upgrade.")
	} else if config.Version.LT(client.ServerVersion) {
		fmt.Println("Server is ahead of client - maybe you're running an old client from a different path?")
	}

	return nil
}

func handleList(client *client.Client) error {
	services, err := client.List(*listRunning, *listTemp)

	// Sort short list by activity, and long list by name, cuz long list is
	// more of a clerical thing, and short list is more a status-check.
	if *listLong {
		sort.Sort(service.InfoByName(services))
	} else {
		sort.Sort(service.InfoByActivity(services))
	}

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
	// Run-once is a little different from saved services. Default to the
	// current dir of the client.
	if *runDir == "" {
		// If it doesn't work, let the server pic a default
		*runDir, _ = os.Getwd()
	}

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
	var wait sync.WaitGroup
	wait.Add(2)
	go func() {
		defer wait.Done()
		for line := range stdoutChan {
			fmt.Println(line)
		}
	}()
	go func() {
		defer wait.Done()
		for line := range stderrChan {
			fmt.Fprintln(os.Stderr, line)
		}
	}()

	wait.Wait()
	if err, ok := <-errChan; ok && err != nil {
		return err
	}
	return nil
}

func handleInfo(client *client.Client) error {
	info, err := client.Info(*infoService)
	if err == nil {
		fmt.Println(info.LongString())
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
