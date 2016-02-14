package service

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/fatih/color"
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

// InfoByName implements the sort interface
type InfoByName []Info

func (i InfoByName) Len() int           { return len(i) }
func (i InfoByName) Swap(a, b int)      { i[b], i[a] = i[a], i[b] }
func (i InfoByName) Less(a, b int) bool { return strings.Compare(i[a].Name, i[b].Name) < 0 }

// InfoByActivity implements the sort interface, so that the order is:
//   - running & recently started
//   - running & started longer ago than above
//   - stopped & rececently exitted
//   - stopped & exitted longer ago than above
type InfoByActivity []Info

func (i InfoByActivity) Len() int      { return len(i) }
func (i InfoByActivity) Swap(a, b int) { i[b], i[a] = i[a], i[b] }
func (i InfoByActivity) Less(a, b int) bool {
	return (
	// [a] running
	(i[a].Running && (!i[b].Running || i[a].StartTime.After(i[b].StartTime))) ||
		// [a] stopped
		(!i[a].Running && !i[b].Running && i[a].EndTime.After(i[b].EndTime)))
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

	colorPattern      = regexp.MustCompile("\x1b[^m]*m")
	multiSpacePattern = regexp.MustCompile("   *")
)

// PlainString gets an uncolored string
func (i Info) PlainString() string {
	return multiSpacePattern.ReplaceAllString(colorPattern.ReplaceAllString(i.String(), ""), " ")
}

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
	stateColor := stoppedNameColor
	state := "stopped"
	stateBullet := unstartedBullet
	if i.Running {
		stateColor = runningNameColor
		state = fmt.Sprintf("%s, pid:%v", stateColor("running"), i.Pid)
		stateBullet = runningBullet
	}

	startTime := "(hasn't started yet)"
	if !i.StartTime.IsZero() {
		startTime = fmt.Sprintf("%s, %v", humanize.Time(i.StartTime), i.StartTime)
	}

	exitTime := fmt.Sprintf("%s, %v", humanize.Time(i.EndTime), i.EndTime)
	exitStatus := "(hasn't exitted yet)"
	exitBullet := unstartedBullet
	if i.Succeeded {
		exitStatus = color.GreenString("succeeded")
		exitBullet = succeededBullet
	} else if !i.EndTime.IsZero() {
		exitStatus = color.RedString("failed")
		exitBullet = failedBullet
	} else {
		exitTime = "-"
	}

	runTime := "(hasn't run yet)"
	if !i.EndTime.IsZero() {
		runTime = i.EndTime.Sub(i.StartTime).String()
	} else if i.Running {
		runTime = time.Since(i.StartTime).String()
	}

	autoStart := "-"
	if i.AutoStart {
		autoStart = autoStartSymbol
	}

	restartOnExit := "-"
	if i.RestartOnExit {
		restartOnExit = restartOnExitSymbol
	}

	var conf string
	if bytes, err := yaml.Marshal(i.Service); err != nil {
		conf = color.RedString(" %v", err)
	} else {
		for _, line := range strings.Split(string(bytes), "\n") {
			conf = fmt.Sprintf("%s\n      %s", conf, line)
		}
	}

	return fmt.Sprintf(
		"[%s]\n"+
			"  %s %s\n"+
			"  %s last exit status: %s\n"+
			"  - last exit time: %s\n"+
			"  - last start time: %s\n"+
			"  - run time: %s\n"+
			"  %s auto-start: %v\n"+
			"  %s restart-on-exit: %v\n"+
			"  - config:%s",
		stateColor(i.Name),
		stateBullet, state,
		exitBullet, exitStatus,
		exitTime,
		startTime,
		runTime,
		autoStart, i.AutoStart,
		restartOnExit, i.RestartOnExit,
		conf)
}
