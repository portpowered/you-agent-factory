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

import providerservice "github.com/portpowered/infinite-you/pkg/services/providers/internal/service"

// Default capture and timeout limits for the approved Agy PTY boundary.
// Story 17 implementation must honor these defaults unless factory config
// documents an explicit override within MaxMaxCaptureBytes.
const (
	DefaultMaxCaptureBytes = providerservice.DefaultPTYMaxCaptureBytes
	MaxMaxCaptureBytes     = providerservice.MaxPTYMaxCaptureBytes
	DefaultIdleTimeout     = providerservice.DefaultPTYIdleTimeout
	DefaultHardTimeout     = providerservice.DefaultPTYHardTimeout
)

// ErrUnsupportedPlatform reports that the current OS build cannot allocate ConPTY
// or POSIX PTY APIs. Callers must fail closed; pipe IO is not a fallback.
var ErrUnsupportedPlatform = providerservice.ErrPTYUnsupportedPlatform

// ErrPTYAllocationFailed reports that ConPTY or POSIX PTY allocation failed.
// It is distinct from ErrUnsupportedPlatform.
var ErrPTYAllocationFailed = providerservice.ErrPTYAllocationFailed

// ErrSessionTimedOut reports that idle or hard timeout canceled the PTY session.
var ErrSessionTimedOut = providerservice.ErrPTYSessionTimedOut

// ErrNonzeroExit reports that the supervised Agy child exited with a nonzero status.
var ErrNonzeroExit = providerservice.ErrPTYNonzeroExit

// ErrClockRequired reports that PTY session timing was not injected.
var ErrClockRequired = providerservice.ErrPTYClockRequired

// ErrHostRequired reports that the native PTY/process effect was not injected.
var ErrHostRequired = providerservice.ErrPTYHostRequired

// SessionConfig carries bounded capture and timeout policy for one PTY session.
type SessionConfig = providerservice.PTYSessionConfig

// DefaultSessionConfig returns the documented default session limits.
func DefaultSessionConfig() SessionConfig {
	return providerservice.DefaultPTYSessionConfig()
}

// ProcessLaunch is the typed subprocess description for one Agy headless run.
// Argv is passed directly to exec.Command without shell indirection.
type ProcessLaunch = providerservice.PTYProcessLaunch

// SessionResult is the observable outcome of one PTY session after cleanup.
type SessionResult = providerservice.PTYSessionResult

// PTYKind identifies the native terminal mechanism selected by the host
// adapter. It is observable session metadata, not construction policy.
type PTYKind = providerservice.PTYKind

const (
	PTYKindUnknown = providerservice.PTYKindUnknown
	PTYKindPOSIX   = providerservice.PTYKindPOSIX
	PTYKindConPTY  = providerservice.PTYKindConPTY
)

// PTYAllocator opens a platform PTY for one supervised child process.
//
// The concrete host adapter is selected in Wire from pkg/platform/pty.
//
// Unit tests inject MockAllocator to avoid real ConPTY/PTY allocation.
type PTYAllocator = providerservice.PTYAllocator

// PTYSession is the mockable seam for bounded capture, timeout signaling, and cleanup.
// Story 17 wires real capture loops; tests use MockSession with predetermined bytes.
type PTYSession = providerservice.PTYSession
