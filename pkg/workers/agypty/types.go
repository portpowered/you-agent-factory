// Package agypty defines the minimal native Go Agy PTY boundary interfaces,
// pure argv/path helpers, and mock seams approved in
// docs/architecture/agy-pty-interface.md.
//
// Platform PTY allocation uses WindowsConPTYAllocator (ConPTY pseudo-console) and
// POSIXPTYAllocator (openpty master/slave). Unsupported OS builds return
// ErrUnsupportedPlatform and never fall back to pipe IO. Story 002+ implement
// PTYSession.Run capture, timeout, and cleanup. Tests substitute MockAllocator or
// inject mock openers for hermetic coverage without an installed Agy binary.
//
// The package does not invoke or embed the upstream Python bridge.
//
// Related documents:
//   - docs/architecture/agy-pty-boundary.md — ADR scope and gating
//   - docs/architecture/agy-pty-threat-review.md — T1/T2 security controls
package agypty

import (
	"context"
	"errors"
	"time"
)

// Default capture and timeout limits for the approved Agy PTY boundary.
// Story 17 implementation must honor these defaults unless factory config
// documents an explicit override within MaxMaxCaptureBytes.
const (
	DefaultMaxCaptureBytes = 4 * 1024 * 1024  // 4 MiB
	MaxMaxCaptureBytes     = 16 * 1024 * 1024 // 16 MiB hard ceiling
	DefaultIdleTimeout     = 30 * time.Second
	DefaultHardTimeout     = 10 * time.Minute
)

// ErrUnsupportedPlatform reports that the current OS build cannot allocate ConPTY
// or POSIX PTY APIs. Callers must fail closed; pipe IO is not a fallback.
var ErrUnsupportedPlatform = errors.New("agypty: platform PTY allocation is not supported")

// ErrPTYAllocationFailed reports that ConPTY or POSIX PTY allocation failed.
// It is distinct from ErrUnsupportedPlatform.
var ErrPTYAllocationFailed = errors.New("agypty: PTY allocation failed")

// ErrSessionTimedOut reports that idle or hard timeout canceled the PTY session.
var ErrSessionTimedOut = errors.New("agypty: session timed out")

// ErrNonzeroExit reports that the supervised Agy child exited with a nonzero status.
var ErrNonzeroExit = errors.New("agypty: process exited with nonzero status")

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

// DefaultPlatformAllocatorFactory returns the platform PTY allocator for the
// current OS build.
type DefaultPlatformAllocatorFactory struct{}

// NewDefaultPlatformAllocatorFactory constructs the default factory.
func NewDefaultPlatformAllocatorFactory() PlatformAllocatorFactory {
	return DefaultPlatformAllocatorFactory{}
}

// NewAllocator implements PlatformAllocatorFactory.
func (DefaultPlatformAllocatorFactory) NewAllocator() (PTYAllocator, error) {
	return newPlatformPTYAllocator(), nil
}
