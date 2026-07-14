package agypty

import (
	"context"
	"errors"
	"time"
)

// ErrUnsupportedPlatform reports that the current OS build cannot allocate ConPTY
// or POSIX PTY APIs. Callers must fail closed; pipe IO is not a fallback.
var ErrUnsupportedPlatform = errors.New("agypty: platform PTY allocation is not supported")

// SessionConfig carries bounded capture and timeout policy for one PTY session.
type SessionConfig struct {
	MaxCaptureBytes int
	IdleTimeout     time.Duration
	HardTimeout     time.Duration
}

// DefaultSessionConfig returns the documented default session limits.
func DefaultSessionConfig() SessionConfig {
	return SessionConfig{
		MaxCaptureBytes: DefaultMaxCaptureBytes,
		IdleTimeout:     DefaultIdleTimeout,
		HardTimeout:     DefaultHardTimeout,
	}
}

// ProcessLaunch is the typed subprocess description for one Agy headless run.
// Argv is passed directly to exec.Command without shell indirection.
type ProcessLaunch struct {
	Executable string
	Argv       []string
	WorkDir    string
	Env        []string
}

// SessionResult is the observable outcome of one PTY session after cleanup.
type SessionResult struct {
	ExitCode    int
	RawBytes    []byte
	CleanedText string
	TimedOut    bool
	CapacityHit bool
}

// PTYAllocator opens a platform PTY for one supervised child process.
//
// Production implementations:
//   - WindowsConPTYAllocator (windows build tag) — ConPTY pseudo-console pair
//   - POSIXPTYAllocator (linux,darwin build tags) — openpty master/slave pair
//
// Unit tests inject MockAllocator to avoid real ConPTY/PTY allocation.
type PTYAllocator interface {
	Allocate(ctx context.Context, launch ProcessLaunch, cfg SessionConfig) (PTYSession, error)
}

// PTYSession is the mockable seam for bounded capture, timeout signaling, and cleanup.
// Story 17 wires real capture loops; tests use MockSession with predetermined bytes.
type PTYSession interface {
	Run(ctx context.Context) (SessionResult, error)
	Close() error
}

// PlatformAllocatorFactory selects the platform PTY allocator at runtime.
// Story 17 provides a real factory; tests return MockAllocator directly.
type PlatformAllocatorFactory interface {
	NewAllocator() (PTYAllocator, error)
}
