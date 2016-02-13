package logging

import (
	"fmt"
	"strings"

	log "github.com/inconshreveable/log15"
	"github.com/inconshreveable/log15/stack"
)

// LvlStackHandler returns a Handler that adds info about the call stack to
// log calls at or more urgent that the given level.
func LvlStackHandler(lvl log.Lvl, h log.Handler) log.Handler {
	return log.FuncHandler(func(r *log.Record) error {
		if r.Lvl <= lvl {
			s := stack.Callers().
				TrimBelow(stack.Call(r.CallPC[0])).
				TrimRuntime()

			// Format each Call twice, for fn name & line #
			formats := make([]string, 0, len(s))
			calls := make([]interface{}, 0, len(s)*2)
			for _, call := range s {
				formats = append(formats, "%n():%d")
				calls = append(calls, call, call)
			}

			r.Ctx = append(r.Ctx, "stack", fmt.Sprintf(strings.Join(formats, " "), calls...))
		}

		return h.Log(r)
	})
}
