package service

import (
	"bufio"
	"sync"
)

// OutputLine is a line of output, eithet to stdout or stderr
type OutputLine struct {
	// Pid of process that outputted this line
	Pid int

	// True if output to stderr, otherwise it was to stdout
	Stderr bool

	// The output line
	Line string
}

// output manages output from a service
type output struct {
	lock sync.RWMutex

	// Output lines from all the processes related to a service, across restarts
	lines       []OutputLine
	indexOffset int

	// Pid of the streams currently being watched. If both streams are closed,
	// this will be set to 0, even if the process itself is still going. That
	// doesn't concern this struct.
	pid int

	// Used internally to cancel output watchers if
	cancel chan interface{}
}

func (out *output) followNewProcess(pid int, stdout, stderr *bufio.Scanner) *sync.WaitGroup {
	out.lock.Lock()
	defer out.lock.Unlock()

	// Cancel outputWatchers if they're still going.
	if out.cancel != nil {
		close(out.cancel)
	}
	out.cancel = make(chan interface{})

	// It's ok if we race with the previous watchPid(), the lock & its check
	// should be safe
	out.pid = pid

	// Spin up watchers, 2 that use the sync group to indicate when they're
	// done, and one that waits on those two.
	outputDone := new(sync.WaitGroup)
	outputDone.Add(2)
	go out.watchOutput(stdout, false, pid, outputDone)
	go out.watchOutput(stderr, true, pid, outputDone)
	go out.watchPid(pid, outputDone)

	return outputDone
}

// GetTail is a convenience wrapper aroung Get().
func (out *output) GetTail(pid, num int) (lines []OutputLine, eof bool, nextIndex, nextPid int) {
	return out.Get(-1*num, pid, num)
}

// Get gets lines of output.
//   index: If >= 0, the line # to start from. If < 0, that # of lines from
//	        the end of output
//	 pid: If 0, lines are from any process, otherwise restricted to this pid's
//   max: If > 0, limit # lines returned
// Returns:
//   lines: A slice of lines
//   eof: True if pid != 0 && that process has no more output & never will
//   nextIndex: An index that can be used on a subsequent call to continue from
//              where this Get() call left off
//   nextPid: A pid that can be used on a subsequent call to continue from where
//            this Get() call left off
func (out *output) Get(index, pid, max int) (lines []OutputLine, eof bool, nextIndex, nextPid int) {
	out.lock.RLock()
	defer out.lock.RUnlock()

	// Translate a global or reverse index to an index into the window we have in out.lines
	if index >= 0 {
		// Global index
		index = index - out.indexOffset
	} else {
		// Negative index means that many from end
		end := len(out.lines)

		// If they're asking for a specific pid, and it's not the current proc,
		// find that proc's end, otherwise if it's the current proc, and it
		// hasn't yet outputted anything, we'll skip where it would go, and
		// think it's done.
		if pid > 0 && pid != out.pid {
			for end > 0 && pid != out.lines[end-1].Pid {
				end--
			}
		}

		if end > 0 {
			// Find the start by scanning back from end
			num := -1 * index
			index = end
			for index > 0 && end-index < num && (pid == 0 || out.lines[index-1].Pid == pid) {
				index--
			}
		}
	}

	// If the caller falls behind, just clamp them to what we have. If they
	// care about a particular pid, that'll be handled regardless.
	if index < 0 {
		index = 0
	}

	// Scan for how many lines are from the same process, up to requested max
	end := index
	for end < len(out.lines) && (max == 0 || end-index < max) && (pid == 0 || pid == out.lines[end].Pid) {
		end++
	}

	// Copy
	if end-index > 0 {
		lines = out.lines[index:end]
	}

	// Set the returned global next index
	nextIndex = out.indexOffset + end

	// Next pid from next line, if there is one
	if end < len(out.lines) {
		nextPid = out.lines[end].Pid
	} else {
		// No more lines, so use what's going to output next, even if that's 0
		nextPid = out.pid
	}

	// EOF if next output will be from a diff proc. In the case that user
	// doesn't care about pid, eof will never be false (there's always a
	// possibility of a new proc to output more).
	eof = pid != 0 && pid != nextPid

	return
}

// watchOutput reads from stdout or stderr & puts lines on a capped slice
func (out *output) watchOutput(outScanner *bufio.Scanner, isStderr bool, pid int, done *sync.WaitGroup) {
	defer done.Done()

	size := 0

	for outScanner.Scan() {
		// Checking cancel here is not really that responsive, since the Scan()
		// call above blocks. But that's the interface we have to the output
		// stream ¯\_(ツ)_/¯ But we do need it, so we don't interleave lines
		// from different procs, and mess up the EOF logic, or what a tailer
		// expects.
		select {
		case <-out.cancel:
			return
		default:
		}

		func(line string) {
			out.lock.Lock()
			defer out.lock.Unlock()

			// Don't write lines if process has already been replaced
			if pid != out.pid {
				return
			}

			size += len(line)
			out.lines = append(out.lines, OutputLine{
				Pid:    pid,
				Stderr: isStderr,
				Line:   line,
			})

			// Cut down by total size, cuz output could be a binary stream, and we
			// care about size more than # lines anyway.
			for len(out.lines) > 1 && size > maxOutputSize {
				size -= len(out.lines[0].Line)
				out.lines = out.lines[1:]
				out.indexOffset++
			}
		}(outScanner.Text())
	}
}

// watchPid waits for output to finish, and clears the pid, if it's not been
// changed already.
func (out *output) watchPid(currentPid int, outputDone *sync.WaitGroup) {
	outputDone.Wait()

	out.lock.Lock()
	defer out.lock.Unlock()

	// Only clear if it's the one we started with (can race between done
	// & lock).
	if out.pid == currentPid {
		out.pid = 0
	}
}
