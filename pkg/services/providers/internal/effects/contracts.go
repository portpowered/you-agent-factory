// Package effects defines the parent-private Providers execution effects.
package effects

import (
	"context"
	"errors"
	"time"
)

// CommandRunner is the Providers-owned subprocess effect used by private
// provider adapters. Workers may project its own command edge into this
// contract at the composition root, but Providers never consumes a Workers
// command interface directly.
type CommandRunner interface {
	Run(context.Context, CommandRequest) (CommandResult, error)
}

// StreamingCommandRunner is the optional streaming extension of
// CommandRunner. The adapter falls back to one completed stdout/stderr chunk
// when only CommandRunner is available. Implementations must preserve an
// observer error in their returned error.
type StreamingCommandRunner interface {
	CommandRunner
	RunStreaming(context.Context, CommandRequest, OutputChunkObserver) (CommandResult, error)
}

// OutputChunkObserver receives output from one provider subprocess effect and
// returns an error when the consumer cannot accept the chunk.
type OutputChunkObserver func(stream string, chunk []byte) error

const (
	OutputStreamStdout = "stdout"
	OutputStreamStderr = "stderr"
)

// CommandRequest contains policy-free process inputs plus the Providers
// attempt correlation needed by composition-owned effect adapters.
type CommandRequest struct {
	Command         string
	Args            []string
	Stdin           []byte
	Env             []string
	WorkDir         string
	AttemptID       string
	WorkerType      string
	WorkstationName string
}

// CommandResult is the observable result of one provider subprocess effect.
type CommandResult struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int
}

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
