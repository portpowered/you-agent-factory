package process

import (
	"errors"
	"fmt"
	"os"
)

// ErrProcessGone means the OS affirmatively established that this PID has no
// live process. Access failures and unsupported queries never return it.
var ErrProcessGone = errors.New("process incarnation: process is gone")

// Incarnation identifies a local OS process without treating a reused PID as
// the same owner. Start is an opaque OS creation token, never a wall clock guess.
type Incarnation struct {
	Host  string `json:"host"`
	PID   int    `json:"pid"`
	Start string `json:"start"`
}

// IncarnationProbe reads OS identity; it does not choose recovery policy.
type IncarnationProbe struct{}

// CurrentProcess returns the actual process identity, or an error when the OS
// cannot establish it. Callers must preserve unknown ownership on failure.
func (probe IncarnationProbe) CurrentProcess() (Incarnation, error) {
	return probe.LookupProcess(os.Getpid())
}

// LookupProcess returns a live local process's creation identity. Recovery
// policy must compare both PID and Start with its recorded owner; a different
// creation token is a different process, even when the PID has been reused.
func (probe IncarnationProbe) LookupProcess(pid int) (Incarnation, error) {
	if pid <= 0 {
		return Incarnation{}, fmt.Errorf("process incarnation: invalid PID")
	}
	host, err := os.Hostname()
	if err != nil {
		return Incarnation{}, err
	}
	start, err := probe.processStart(pid)
	if err != nil {
		return Incarnation{}, err
	}
	return Incarnation{Host: host, PID: pid, Start: start}, nil
}
