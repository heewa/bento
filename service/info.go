package service

import (
	"fmt"
	"strings"
	"time"

	log "github.com/inconshreveable/log15"
	"gopkg.in/yaml.v2"

	"github.com/heewa/servicetray/config"
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

	Tail []string `yaml:"tail,omitempty"`
}

// String gets a user friendly string about a service.
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
			"<%s> runtime:%v ended:%s ago pid:%d started:%s",
			result,
			i.Runtime,
			time.Since(i.EndTime).String(),
			i.Pid,
			i.StartTime.Format(time.UnixDate))
	}

	var behaviors []string
	if i.AutoStart {
		behaviors = append(behaviors, "(auto-start) ")
	}
	if i.RestartOnExit {
		behaviors = append(behaviors, "(restart-on-exit) ")
	}

	cmd := i.Program
	if len(i.Args) > 0 {
		cmd = fmt.Sprintf("%s %s", i.Program, strings.Join(i.Args, " "))
	}

	return fmt.Sprintf(
		"[%s] %s %scmd:'%s' dir:%s env:%v",
		i.Name,
		state,
		strings.Join(behaviors, ""),
		cmd, i.Dir, i.Env)
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
