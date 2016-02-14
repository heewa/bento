package server

import (
	"fmt"
	"time"

	log "github.com/inconshreveable/log15"

	"github.com/heewa/bento/service"
)

// TailArgs -
type TailArgs struct {
	// Name of service to get output from
	Name string

	// If specified, restrict output to this pid
	Pid int

	// Max num lines to include
	MaxLines int

	// Index to start at, from a previous call, or a negative index to mean
	// that may from the end in reverse.
	Index int

	// If false, whatever output is available will be returned. Otherwise,
	// the call will wait for some output before returning. If pid != 0 and
	// that process is done with output, the call will return, even if there
	// isn't any output, and EOF will be true.
	Follow bool
}

// TailResponse -
type TailResponse struct {
	// Output lines
	Lines []service.OutputLine

	// True if the pid asked for is done outputting. If no pid was given,
	// true if tail has reached end of whatever is currently available.
	EOF bool

	// Index & pid to use for a followup call to resume from the next line
	// of output.
	NextIndex int
	NextPid   int
}

// Tail gets lines of output since a line index for stdout and/or stderr
func (s *Server) Tail(args *TailArgs, reply *TailResponse) (err error) {
	defer func() {
		if r := recover(); r != nil {
			log.Crit("panic", "msg", r)
			err = fmt.Errorf("Server error: %v", r)
		}
	}()

	serv := s.getService(args.Name)
	if serv == nil {
		return fmt.Errorf("Service '%s' not found.", args.Name)
	}

	reply.Lines, reply.EOF, reply.NextIndex, reply.NextPid = serv.Output.Get(args.Index, args.Pid, args.MaxLines)

	// If following output, wait for some output for a bit.
	// TODO: use a channel for a no-sleep solution
	deadline := time.After(10 * time.Second)
	for args.Follow && !reply.EOF && len(reply.Lines) == 0 {
		select {
		case <-deadline:
			return nil
		case <-time.After(500 * time.Millisecond):
		}

		reply.Lines, reply.EOF, reply.NextIndex, reply.NextPid = serv.Output.Get(reply.NextIndex, reply.NextPid, args.MaxLines)
	}

	return nil
}
