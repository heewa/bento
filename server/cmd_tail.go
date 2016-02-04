package server

import (
	"fmt"
)

// TailArgs -
type TailArgs struct {
	// Name & pid included to detect restarts between calls
	Name string
	Pid  int

	Stdout          bool
	StdoutSinceLine int

	Stderr          bool
	StderrSinceLine int
}

// TailResponse -
type TailResponse struct {
	Pid int

	StdoutLines     []string
	StdoutSinceLine int

	StderrLines     []string
	StderrSinceLine int
}

// Tail gets lines of output since a line index for stdout and/or stderr
func (s *Server) Tail(args *TailArgs, reply *TailResponse) error {
	serv := s.getService(args.Name)
	if serv == nil {
		return fmt.Errorf("Service '%s' not found.", args.Name)
	}

	if args.Stdout {
		reply.StdoutLines, reply.StdoutSinceLine, reply.Pid = serv.Stdout(args.Pid, args.StdoutSinceLine)
	}

	if args.Stderr {
		reply.StderrLines, reply.StderrSinceLine, reply.Pid = serv.Stderr(args.Pid, args.StderrSinceLine)
	}

	return nil
}
