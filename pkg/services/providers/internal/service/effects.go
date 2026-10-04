package service

import (
	"context"
	"errors"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/providers"
)

// Command effect aliases retain the owner-private adapter vocabulary while
// peers consume the Providers root contract.
type CommandRunner = providers.CommandRunner
type StreamingCommandRunner = providers.StreamingCommandRunner
type CommandRequest = providers.CommandRequest
type CommandResult = providers.CommandResult
type OutputChunkObserver = providers.OutputChunkObserver

const (
	OutputStreamStdout = providers.OutputStreamStdout
	OutputStreamStderr = providers.OutputStreamStderr
)

// PTYSessionConfig carries bounded capture and timeout policy for one
// Providers-owned PTY session.
type PTYSessionConfig struct {
	MaxCaptureBytes int
	IdleTimeout     time.Duration
	HardTimeout     time.Duration
}

const (
	DefaultPTYMaxCaptureBytes = 4 * 1024 * 1024
	MaxPTYMaxCaptureBytes     = 16 * 1024 * 1024
	DefaultPTYIdleTimeout     = 30 * time.Second
	DefaultPTYHardTimeout     = 10 * time.Minute
)

// DefaultPTYSessionConfig returns the bounded native-session defaults used by
// the Providers Agy adapter.
func DefaultPTYSessionConfig() PTYSessionConfig {
	return PTYSessionConfig{
		MaxCaptureBytes: DefaultPTYMaxCaptureBytes,
		IdleTimeout:     DefaultPTYIdleTimeout,
		HardTimeout:     DefaultPTYHardTimeout,
	}
}

// PTYProcessLaunch is the typed subprocess description for one Agy PTY run.
type PTYProcessLaunch struct {
	Executable string
	Argv       []string
	WorkDir    string
	Env        []string
}

// PTYSessionResult is the observable outcome after a PTY session is cleaned
// up.
type PTYSessionResult struct {
	ExitCode    int
	RawBytes    []byte
	CleanedText string
	TimedOut    bool
	CapacityHit bool
}

// PTYKind identifies the native terminal mechanism selected by the host.
type PTYKind int

const (
	PTYKindUnknown PTYKind = iota
	PTYKindPOSIX
	PTYKindConPTY
)

func (kind PTYKind) String() string {
	switch kind {
	case PTYKindPOSIX:
		return "posix"
	case PTYKindConPTY:
		return "conpty"
	default:
		return "unknown"
	}
}

// PTYAllocator opens one native PTY session for a provider subprocess.
type PTYAllocator interface {
	Allocate(context.Context, PTYProcessLaunch, PTYSessionConfig) (PTYSession, error)
}

// PTYSession is the private seam for bounded capture, timeout signaling, and
// cleanup of one provider PTY process.
type PTYSession interface {
	Run(context.Context) (PTYSessionResult, error)
	Close() error
}

var (
	ErrPTYUnsupportedPlatform = errors.New("agypty: platform PTY allocation is not supported")
	ErrPTYAllocationFailed    = errors.New("agypty: PTY allocation failed")
	ErrPTYSessionTimedOut     = errors.New("agypty: session timed out")
	ErrPTYNonzeroExit         = errors.New("agypty: process exited with nonzero status")
	ErrPTYClockRequired       = errors.New("agypty: clock is required")
	ErrPTYHostRequired        = errors.New("agypty: native PTY host is required")
)
