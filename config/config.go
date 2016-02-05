package config

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/user"
	"path"

	log "github.com/inconshreveable/log15"
	"gopkg.in/yaml.v2"
)

const (
	configDir  = ".servicetray"
	configFile = "config.yml"

	defaultConfig = `# Config for ServiceTray
# See https://github.com/heewa/servicetray

# Set 'log' to a path for the server to log there. 
#log: "/path/to/servicetray.log"

# Path to the fifo file that the clients and server use to communicate
#fifo: "/path/to/servicetray.fifo"
`
)

var (
	// LogPath -
	LogPath = "servicetray.log"

	// FifoPath -
	FifoPath = ".fifo"
)

// ConfFormat is the yaml definition of the config file
type ConfFormat struct {
	LogPath  string `yaml:"log"`
	FifoPath string `yaml:"fifo"`
}

// Load reads the config file and populates the global conf. It also handles
// creating the dir in the user's home if it doesn't exist, and populating
// an empty log file with comments, to guide the user.
func Load() error {
	log.Debug("Loading config")

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
	log.Debug("Checking or making config dir", "dir", dirPath)
	if os.Mkdir(dirPath, 0700); err != nil {
		return fmt.Errorf("Failed to create config dir (%s): %v", dirPath, err)
	}

	// Try opening the conf file, on most runs it'll already exist
	var confData []byte
	if f, err := os.Open(confPath); err != nil && os.IsNotExist(err) {
		log.Debug("Creating default conf file", "path", confPath)
		if err := ioutil.WriteFile(confPath, []byte(defaultConfig), 0660); err != nil {
			return fmt.Errorf("Failed to create a default config file (%s): %v", confPath, err)
		}
		confData = []byte(defaultConfig)
	} else if err != nil {
		return fmt.Errorf("Failed to open config file (%s): %v", confPath, err)
	} else {
		defer f.Close()

		log.Debug("Reading conf file", "path", confPath)
		confData, err = ioutil.ReadAll(f)
		if err != nil {
			return fmt.Errorf("Failed to read conf file (%s): %v", confPath, err)
		}
	}

	conf := ConfFormat{}

	log.Debug("Parsing log file")
	if err := yaml.Unmarshal(confData, &conf); err != nil {
		return fmt.Errorf("Failed to parse conf file (%s): %v", confPath, err)
	}

	if conf.LogPath != "" {
		LogPath = conf.LogPath
	} else {
		if LogPath, err = getFullConfPath("log"); err != nil {
			return fmt.Errorf("Failed to build log file path", "err", err)
		}
	}

	if conf.FifoPath != "" {
		FifoPath = conf.FifoPath
	} else {
		if FifoPath, err = getFullConfPath(".fifo"); err != nil {
			return fmt.Errorf("Failed to build fifo file path", "err", err)
		}
	}

	log.Debug("Config file loaded", "LogPath", LogPath, "FifoPath", FifoPath)
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
