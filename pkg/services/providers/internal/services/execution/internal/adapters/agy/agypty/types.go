// Package agypty defines the minimal native Go Agy PTY boundary interfaces,
// pure argv/path helpers, and mock seams approved in
// docs/architecture/agy-pty-interface.md.
//
// Wire injects the policy-free native host adapter from pkg/platform/pty.
// Unsupported OS builds return ErrUnsupportedPlatform and never fall back to
// pipe IO. Story 002+ implement
// PTYSession.Run capture, timeout, and cleanup. Tests substitute MockAllocator or
// inject mock openers for hermetic coverage without an installed Agy binary.
//
// The package does not invoke or embed the upstream Python bridge.
//
// Related documents:
//   - docs/architecture/agy-pty-boundary.md — ADR scope and gating
//   - docs/architecture/agy-pty-threat-review.md — T1/T2 security controls
package agypty

import "github.com/portpowered/infinite-you/pkg/services/workers"

// Default capture and timeout limits for the approved Agy PTY boundary.
// Story 17 implementation must honor these defaults unless factory config
// documents an explicit override within MaxMaxCaptureBytes.
const (
	DefaultMaxCaptureBytes = workers.DefaultPTYMaxCaptureBytes
	MaxMaxCaptureBytes     = workers.MaxPTYMaxCaptureBytes
	DefaultIdleTimeout     = workers.DefaultPTYIdleTimeout
	DefaultHardTimeout     = workers.DefaultPTYHardTimeout
)

// ErrUnsupportedPlatform reports that the current OS build cannot allocate ConPTY
// or POSIX PTY APIs. Callers must fail closed; pipe IO is not a fallback.
var ErrUnsupportedPlatform = workers.ErrPTYUnsupportedPlatform

// ErrPTYAllocationFailed reports that ConPTY or POSIX PTY allocation failed.
// It is distinct from ErrUnsupportedPlatform.
var ErrPTYAllocationFailed = workers.ErrPTYAllocationFailed

// ErrSessionTimedOut reports that idle or hard timeout canceled the PTY session.
var ErrSessionTimedOut = workers.ErrPTYSessionTimedOut

// ErrNonzeroExit reports that the supervised Agy child exited with a nonzero status.
var ErrNonzeroExit = workers.ErrPTYNonzeroExit

// ErrClockRequired reports that PTY session timing was not injected.
var ErrClockRequired = workers.ErrPTYClockRequired

// ErrHostRequired reports that the native PTY/process effect was not injected.
var ErrHostRequired = workers.ErrPTYHostRequired

// SessionConfig carries bounded capture and timeout policy for one PTY session.
type SessionConfig = workers.PTYSessionConfig

// DefaultSessionConfig returns the documented default session limits.
func DefaultSessionConfig() SessionConfig {
	return workers.DefaultPTYSessionConfig()
}

// ProcessLaunch is the typed subprocess description for one Agy headless run.
// Argv is passed directly to exec.Command without shell indirection.
type ProcessLaunch = workers.PTYProcessLaunch

// SessionResult is the observable outcome of one PTY session after cleanup.
type SessionResult = workers.PTYSessionResult

// PTYKind identifies the native terminal mechanism selected by the host
// adapter. It is observable session metadata, not construction policy.
type PTYKind = workers.PTYKind

const (
	PTYKindUnknown = workers.PTYKindUnknown
	PTYKindPOSIX   = workers.PTYKindPOSIX
	PTYKindConPTY  = workers.PTYKindConPTY
)

// PTYAllocator opens a platform PTY for one supervised child process.
//
// The concrete host adapter is selected in Wire from pkg/platform/pty.
//
// Unit tests inject MockAllocator to avoid real ConPTY/PTY allocation.
type PTYAllocator = workers.PTYAllocator

// PTYSession is the mockable seam for bounded capture, timeout signaling, and cleanup.
// Story 17 wires real capture loops; tests use MockSession with predetermined bytes.
type PTYSession = workers.PTYSession
