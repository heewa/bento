package config

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/user"
	"path"
	"time"

	"github.com/blang/semver"
	log "github.com/inconshreveable/log15"
	"gopkg.in/alecthomas/kingpin.v2"
	"gopkg.in/yaml.v2"
)

const (
	configDir         = ".bento"
	configFile        = "config.yml"
	serviceConfigFile = "services.yml"

	// Just regular constants

	// EscalationInterval is used as the default value when none is given to
	// service.Stop() as the time to wait before increasing urgency of signal
	EscalationInterval = 10 * time.Second

	defaultConfig = `# Config for Bento
# See https://github.com/heewa/bento

# Set 'log' to a path for the server to log there.
#log: "/path/to/bento.log"

# Log Level can be "crit", "error", "warn", "info", or "debug"
#log_level: "info"

# Path to the fifo file that the clients and server use to communicate
#fifo: "/path/to/bento.fifo"

# When temp services exit, after this duration (unless they are restarted),
# they are auto-removed. This can be override from the cmdline for an
# individual service when creating it.
#
# Values can be like "1s" (1 second), "1h" (1 hour), "1h15m10s" (1 hour, 15
# minutes and 10 seconds)
#clean_temp_services_after: "1h"
`
)

var (
	// Version of the package
	Version = semver.MustParse("0.1.0-alpha.1.1")

	// ServiceConfigFile is the full path to the config file that lists
	// services to be read on server startup. If the path doesn't exist,
	// this'll be empty.
	ServiceConfigFile string

	// LogLevel determines the severity of messages that are logged.
	LogLevel = log.LvlWarn

	// LogPath is the path to the server's log file.
	LogPath = "bento.log"

	// FifoPath is the path to a unix named pipe that's used to communicate
	// between clients & the server.
	FifoPath = ".fifo"

	// HeartbeatInterval is the frequency that the fifo file is touched to
	// indicate a live server.
	HeartbeatInterval = 10 * time.Second

	// CleanTempServicesAfter is the interval after which an exited temp
	// service is removed.
	CleanTempServicesAfter = 1 * time.Hour

	// Cmdline args that override conf:
	verbosity = kingpin.Flag("verbose", "Increase log verbosity, can be used multiple times").Short('v').Counter()
	fifoPath  = kingpin.Flag("fifo", "Path to fifo used to communicate between client and server").Hidden().String()
	logPath   = kingpin.Flag("log", "Path to server's log file, or '-' for stdout").Hidden().String()
)

// ConfFormat is the yaml definition of the config file
type ConfFormat struct {
	LogLevel               string `yaml:"log_level"`
	LogPath                string `yaml:"log"`
	FifoPath               string `yaml:"fifo"`
	CleanTempServicesAfter string `yaml:"clean_temp_services_after"`
}

// Load reads the config file and populates the global conf. It also handles
// creating the dir in the user's home if it doesn't exist, and populating
// an empty log file with comments, to guide the user.
func Load(isServer bool) error {
	dirPath, err := getFullConfPath()
	if err != nil {
		return fmt.Errorf("Failed to determine full config dir path: %v", err)
	}
	confPath, err := getFullConfPath(configFile)
	if err != nil {
		return fmt.Errorf("Failed to determine full config file path: %v", err)
	}

	// Create the config dir if it doesn't exist. Make with user-only
	// permissions, cuz fifo exists there, which can be used to control
	// server. Also saved service details could be sensitive.
	if os.Mkdir(dirPath, 0700); err != nil {
		return fmt.Errorf("Failed to create config dir (%s): %v", dirPath, err)
	}

	// Try opening the conf file, on most runs it'll already exist
	var confData []byte
	if f, err := os.Open(confPath); err != nil && os.IsNotExist(err) {
		// Make a default one
		if err := ioutil.WriteFile(confPath, []byte(defaultConfig), 0660); err != nil {
			return fmt.Errorf("Failed to create a default config file (%s): %v", confPath, err)
		}
		confData = []byte(defaultConfig)
	} else if err != nil {
		return fmt.Errorf("Failed to open config file (%s): %v", confPath, err)
	} else {
		defer f.Close()

		confData, err = ioutil.ReadAll(f)
		if err != nil {
			return fmt.Errorf("Failed to read conf file (%s): %v", confPath, err)
		}
	}

	conf := ConfFormat{}
	if err := yaml.Unmarshal(confData, &conf); err != nil {
		return fmt.Errorf("Failed to parse conf file (%s): %v", confPath, err)
	}

	if *verbosity > 0 {
		LogLevel = log.LvlWarn + log.Lvl(*verbosity)
	} else if level, err := log.LvlFromString(conf.LogLevel); err == nil && isServer {
		LogLevel = level
	} else if isServer {
		LogLevel = log.LvlInfo
	} else {
		LogLevel = log.LvlWarn
	}

	if *logPath != "" {
		LogPath = *logPath
	} else if conf.LogPath != "" {
		LogPath = conf.LogPath
	} else {
		if LogPath, err = getFullConfPath("log"); err != nil {
			return fmt.Errorf("Failed to build log file path: %v", err)
		}
	}

	if *fifoPath != "" {
		FifoPath = *fifoPath
	} else if conf.FifoPath != "" {
		FifoPath = conf.FifoPath
	} else {
		if FifoPath, err = getFullConfPath(FifoPath); err != nil {
			return fmt.Errorf("Failed to build fifo file path: %v", err)
		}
	}

	if conf.CleanTempServicesAfter != "" {
		dur, err := time.ParseDuration(conf.CleanTempServicesAfter)
		if err != nil {
			return fmt.Errorf("Invalid duration for cleaning temp services")
		}
		CleanTempServicesAfter = dur
	}

	// After conf file stuff is all handled, do config related to other stuff

	// Set the path to services conf file only if it exists
	path, err := getFullConfPath(serviceConfigFile)
	if err != nil {
		return fmt.Errorf("Failed to get path to services config file: %v", err)
	}
	_, err = os.Stat(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("Failed to open services config file: %v", err)
	} else if err == nil {
		ServiceConfigFile = path
	}

	log.Debug(
		"Config file loaded",
		"LogPath", LogPath,
		"FifoPath", FifoPath,
		"CleanTempServicesAfter", CleanTempServicesAfter)
	return nil
}

func getFullConfPath(pathParts ...string) (string, error) {
	usr, err := user.Current()
	if err != nil {
		return "", err
	}

	pathParts = append([]string{usr.HomeDir, configDir}, pathParts...)
	fullPath := path.Join(pathParts...)

	return fullPath, nil
}
