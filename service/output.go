package service

import (
	"bufio"
	"fmt"
	"sync"
)

// output manages output from a service
type output struct {
	lock sync.RWMutex

	stdout       []string
	stdoutShifts int

	stderr       []string
	stderrShifts int

	// TODO: fold into regular outputs, with timestamps to order
	shortTail []string
}

func newOutput(stdout, stderr *bufio.Scanner) (*output, <-chan interface{}) {
	out := output{}

	done := make(chan interface{})

	go out.watchOutput(stdout, &(out.stdout), &(out.stdoutShifts), done)
	go out.watchOutput(stderr, &(out.stderr), &(out.stderrShifts), done)

	return &out, done
}

func (out *output) copyShortTail() []string {
	if out == nil {
		return nil
	}

	out.lock.RLock()
	defer out.lock.RUnlock()

	short := make([]string, len(out.shortTail))
	copy(short, out.shortTail)

	return short
}

func (out *output) getStdout(requestedPid, currentPid, since, max int) (lines []string, newSince int, newPid int) {
	if out == nil {
		newPid = -1
		return
	}

	return out.get(requestedPid, currentPid, since, out.stdoutShifts, max, out.stdout)
}

func (out *output) getStderr(requestedPid, currentPid, since, max int) (lines []string, newSince int, newPid int) {
	if out == nil {
		newPid = -1
		return
	}

	return out.get(requestedPid, currentPid, since, out.stderrShifts, max, out.stderr)
}

func (out *output) get(pid, currentPid, since, shifts, max int, outLines []string) ([]string, int, int) {
	out.lock.RLock()
	defer out.lock.RUnlock()

	// If pid doesn't match, there's been a restart since the last call, so
	// reset since
	if pid != currentPid {
		since = 0
	}

	// Look up where in the current buffer they are
	index := since - shifts
	if index < 0 {
		// They've fallen behind, just start at the earliest
		index = 0
	}

	numLines := len(outLines) - index

	// Max counts in reverse from end
	if max > 0 && numLines > max {
		index += numLines - max
		numLines = max
	}

	lines := []string{}
	if numLines > 0 {
		lines = append([]string{}, outLines[index:]...)
	} else {
		// They're caught up
		numLines = 0
	}

	return lines, index + shifts + numLines, currentPid
}

// watchOutput reads from stdout or stderr & puts lines on a capped slice
func (out *output) watchOutput(outScanner *bufio.Scanner, tail *[]string, shifts *int, done chan<- interface{}) {
	size := 0

	for outScanner.Scan() {
		func(line string) {
			fmt.Println(line)

			out.lock.Lock()
			defer out.lock.Unlock()

			if len(out.shortTail) >= shortTailLen {
				out.shortTail = append(out.shortTail[len(out.shortTail)-shortTailLen:], line)
			} else {
				out.shortTail = append(out.shortTail, line)
			}

			size += len(line)
			*tail = append(*tail, line)

			// Cut down by total size, cuz output could be a binary stream, and we
			// care about size more than # lines anyway.
			for len(*tail) > 1 && size > maxOutputSize {
				size -= len((*tail)[0])
				*tail = (*tail)[1:]
				*shifts++
			}

		}(outScanner.Text())
	}

	done <- struct{}{}
}
