package logging

import (
	log "github.com/inconshreveable/log15"
)

// Config sets up logging. It's ok to call multiple times.
func Config(isServer bool, logPath string, lvl log.Lvl) error {
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
		// Filter first, to avoid unecessary work
		log.LvlFilterHandler(lvl,
			// Add call stack to Crit calls. See log15.stack.Call.Format()
			LvlStackHandler(log.LvlCrit,
				logHandler)))

	return nil
}
