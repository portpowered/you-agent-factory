package process

import (
	"os"
)

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
	host, err := os.Hostname()
	if err != nil {
		return Incarnation{}, err
	}
	start, err := probe.processStart(os.Getpid())
	if err != nil {
		return Incarnation{}, err
	}
	return Incarnation{Host: host, PID: os.Getpid(), Start: start}, nil
}
