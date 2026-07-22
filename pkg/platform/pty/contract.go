// Package pty provides policy-free native pseudo-terminal and attached-process
// mechanics. Domain packages supply validated launch descriptions and own
// capture, timeout, output, and failure policy.
package pty

import (
	"context"
	"errors"
	"io"
)

// ErrUnsupportedPlatform reports that the current OS has no supported native
// PTY implementation.
var ErrUnsupportedPlatform = errors.New("pty: platform PTY allocation is not supported")

// Kind identifies the native terminal mechanism selected by the host.
type Kind int

const (
	KindUnknown Kind = iota
	KindPOSIX
	KindConPTY
)

func (k Kind) String() string {
	switch k {
	case KindPOSIX:
		return "posix"
	case KindConPTY:
		return "conpty"
	default:
		return "unknown"
	}
}

// ProcessLaunch describes a subprocess without applying provider or shell
// policy. Argv is passed directly to the operating-system process API.
type ProcessLaunch struct {
	Executable string
	Argv       []string
	WorkDir    string
	Env        []string
}

// Host owns native PTY handles and attached-process mechanics.
type Host interface {
	Allocate(context.Context) (Allocation, error)
	Start(ProcessLaunch, Allocation) (Process, io.ReadCloser, error)
}

// Allocation is an opaque native PTY allocation returned by Host.
type Allocation interface {
	Kind() Kind
	Close() error
}

// Process is a child process attached to a native PTY.
type Process interface {
	Wait() error
	Terminate() error
	Close()
	PID() int
	ExitCode() int
}
