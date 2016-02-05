package service

import (
	"fmt"
	"strings"
	"time"
)

// Info holds info about a service
type Info struct {
	Service

	Running   bool
	Pid       int
	Succeeded bool

	StartTime time.Time
	EndTime   time.Time
	Runtime   time.Duration

	Tail []string
}

func (i Info) String() string {
	var state string
	if i.Running {
		state = fmt.Sprintf(
			"<running> uptime:%v pid:%d started:%s",
			i.Runtime, i.Pid, i.StartTime.Format(time.UnixDate))
	} else if i.Pid == 0 {
		state = "<unstarted>"
	} else {
		result := "failed"
		if i.Succeeded {
			result = "succeeded"
		}

		state = fmt.Sprintf(
			"<%s> runtime:%v pid:%d started:%s ended:%s",
			result, i.Runtime, i.Pid,
			i.StartTime.Format(time.UnixDate), i.EndTime.Format(time.UnixDate))
	}

	cmd := i.Program
	if len(i.Args) > 0 {
		cmd = fmt.Sprintf("%s %s", i.Program, strings.Join(i.Args, " "))
	}

	return fmt.Sprintf(
		"[%s] %s cmd:'%s' dir:%s env:%v",
		i.Name, state, cmd, i.Dir, i.Env)
}
