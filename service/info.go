package service

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/fatih/color"
	log "github.com/inconshreveable/log15"
	"gopkg.in/yaml.v2"

	"github.com/heewa/bento/config"
)

// Info holds info about a service
type Info struct {
	*config.Service `yaml:"config"`

	Running   bool `yaml:"running"`
	Pid       int  `yaml:"pid,omitempty"`
	Succeeded bool `yaml:"succeeded"`
	Dead      bool `yaml:"dead,omitempty"`

	StartTime time.Time     `yaml:"start-time,omitempty"`
	EndTime   time.Time     `yaml:"end-time,omitempty"`
	Runtime   time.Duration `yaml:"run-time,omitempty"`

	Tail []string `yaml:"-"`
}

var (
	stoppedNameColor = color.New(color.FgBlue).SprintfFunc()
	runningNameColor = color.New(color.FgYellow).SprintfFunc()
	statusColor      = color.New(color.FgHiWhite, color.Bold).SprintfFunc()
	pidColor         = color.New().SprintfFunc()

	unstartedBullet = "●"
	succeededBullet = color.GreenString("✔")
	failedBullet    = color.RedString("✘")
	runningBullet   = color.YellowString("⌁")

	autoStartSymbol     = color.WhiteString("↑")
	restartOnExitSymbol = color.WhiteString("↺")
)

// String gets a user friendly string about a service.
func (i Info) String() string {
	nameColor := stoppedNameColor

	var stateInfo string
	var state string
	if i.Running {
		nameColor = runningNameColor
		state = runningBullet
		stateInfo = fmt.Sprintf(
			"%s pid:%s",
			statusColor("started %s", humanize.Time(i.StartTime)),
			pidColor("%d", i.Pid))
	} else if i.Pid == 0 {
		state = unstartedBullet
		stateInfo = statusColor("unstarted")
	} else if i.Succeeded {
		state = succeededBullet
		stateInfo = fmt.Sprintf(
			"%s pid:%s",
			statusColor("ended %s", humanize.Time(i.EndTime)),
			pidColor("%d", i.Pid))
	} else {
		state = failedBullet
		stateInfo = fmt.Sprintf(
			"%s pid:%s",
			statusColor("failed %s", humanize.Time(i.EndTime)),
			pidColor("%d", i.Pid))
	}

	autoStart := " "
	if i.AutoStart {
		autoStart = autoStartSymbol
	}

	restartOnExit := " "
	if i.RestartOnExit {
		restartOnExit = restartOnExitSymbol
	}

	// For a short string, just grab the command's file part
	cmd := filepath.Base(i.Program)
	if len(i.Args) > 0 {
		cmd = fmt.Sprintf("%s %s", cmd, strings.Join(i.Args, " "))
	}
	if len(cmd) > 100 {
		cmd = fmt.Sprintf("%s…", cmd[:99])
	}

	return fmt.Sprintf(
		"  %s %s %s %s  %s cmd:'%s'",
		state,
		nameColor("%-15s", i.Name),
		autoStart, restartOnExit,
		stateInfo,
		cmd)
}

// LongString gets a more detailed description of a service
func (i Info) LongString() string {
	bytes, err := yaml.Marshal(i)
	if err != nil {
		log.Error("Failed to encode Info as yaml", "err", err, "info", i)
		return i.String()
	}

	return string(bytes)
}
