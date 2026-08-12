package workersessions

import (
	"fmt"
	"strings"
)

// ControlAction identifies the lifecycle action requested for one Worker
// Session. The service exposes one method per action so callers cannot submit
// an invalid action string; Action is retained in ControlResult as detached,
// observable evidence of what the service attempted.
type ControlAction string

const (
	ControlActionPause     ControlAction = "PAUSE"
	ControlActionResume    ControlAction = "RESUME"
	ControlActionCancel    ControlAction = "CANCEL"
	ControlActionTerminate ControlAction = "TERMINATE"
)

// ControlOutcome classifies the result of one exact Worker Session control.
// A failed Workers boundary call is returned both as OutcomeFailed and as the
// operation error so a Factory coordinator can retain per-child evidence while
// still distinguishing it from an idempotent or unsupported result.
type ControlOutcome string

const (
	ControlOutcomeApplied     ControlOutcome = "APPLIED"
	ControlOutcomeNoop        ControlOutcome = "NOOP"
	ControlOutcomeUnsupported ControlOutcome = "UNSUPPORTED"
	ControlOutcomeFailed      ControlOutcome = "FAILED"
)

// ControlRequest identifies exactly one stable Worker Session. A supervised
// session owns at most one immutable dispatch attempt at a time; the returned
// ControlResult carries that dispatch identity without requiring callers to
// reconstruct or guess it from a Worker execution request.
type ControlRequest struct {
	ID string
}

// Validate reports whether req identifies one stable Worker Session.
func (req ControlRequest) Validate() error {
	if !validSessionID(req.ID) {
		return ErrInvalidSessionID
	}
	return nil
}

// ControlResult is detached evidence for one requested control. Session is a
// snapshot at the method's linearization point or, for Terminate, after the
// associated attempt has joined. DispatchID is empty only when no dispatch was
// ever admitted for the session.
type ControlResult struct {
	Session    Session
	Action     ControlAction
	Outcome    ControlOutcome
	DispatchID string
}

// InterruptPhase identifies the authoritative boundary at which an interrupt
// was rejected. The phase is stable evidence for callers even when the
// underlying Workers or successor service returns a more specific cause.
type InterruptPhase string

const (
	InterruptPhaseValidation         InterruptPhase = "VALIDATION"
	InterruptPhaseSourceCancellation InterruptPhase = "SOURCE_CANCELLATION"
	InterruptPhaseSuccessorAdmission InterruptPhase = "SUCCESSOR_ADMISSION"
)

// InterruptRequest identifies one active source and the distinct successor
// that should receive the replacement message. The source's exact provider
// session is always resolved from Worker Sessions state; callers cannot
// substitute a provider reference or ask for a fresh session implicitly.
type InterruptRequest struct {
	RequestID                string
	SourceWorkerSessionID    string
	SuccessorWorkerSessionID string
	ReplacementMessage       string
}

// Normalize trims only request identities. ReplacementMessage is intentionally
// preserved byte-for-byte because it becomes the successor's user message.
func (req InterruptRequest) Normalize() InterruptRequest {
	req.RequestID = strings.TrimSpace(req.RequestID)
	req.SourceWorkerSessionID = strings.TrimSpace(req.SourceWorkerSessionID)
	req.SuccessorWorkerSessionID = strings.TrimSpace(req.SuccessorWorkerSessionID)
	return req
}

// Validate checks caller-owned interrupt identities and message content. It
// does not inspect Worker Sessions or cause any downstream effect.
func (req InterruptRequest) Validate() error {
	normalized := req.Normalize()
	if normalized.RequestID == "" {
		return ErrInvalidInterruptRequestID
	}
	if !validSessionID(normalized.SourceWorkerSessionID) ||
		!validSessionID(normalized.SuccessorWorkerSessionID) ||
		normalized.SourceWorkerSessionID == normalized.SuccessorWorkerSessionID {
		return ErrInvalidInterruptLineage
	}
	if strings.TrimSpace(normalized.ReplacementMessage) == "" {
		return ErrInvalidInterruptMessage
	}
	return nil
}

// InterruptResult is detached evidence for one interrupt attempt. Source and
// Successor are snapshots at the operation's authoritative completion point;
// Successor is zero when validation or source cancellation failed before a
// successor was reserved.
type InterruptResult struct {
	RequestID                string
	SourceWorkerSessionID    string
	SuccessorWorkerSessionID string
	Phase                    InterruptPhase
	Accepted                 bool
	Source                   Session
	Successor                Session
}

// Clone returns detached source and successor snapshots.
func (result InterruptResult) Clone() InterruptResult {
	result.Source = result.Source.Clone()
	result.Successor = result.Successor.Clone()
	return result
}

// InterruptError preserves the stable phase and detached snapshots alongside
// a typed operation cause. errors.Is works for both the phase sentinel and the
// underlying Workers/continuation cause through Unwrap.
type InterruptError struct {
	Phase  InterruptPhase
	Result InterruptResult
	Cause  error
}

func (err *InterruptError) Error() string {
	if err == nil {
		return "worker session: interrupt failed"
	}
	if err.Cause == nil {
		return fmt.Sprintf("worker session: interrupt failed during %s", err.Phase)
	}
	return fmt.Sprintf("worker session: interrupt failed during %s: %v", err.Phase, err.Cause)
}

func (err *InterruptError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.Cause
}

// Is allows callers to match an InterruptError by phase without depending on
// its formatted message or the concrete underlying boundary error.
func (err *InterruptError) Is(target error) bool {
	if err == nil {
		return false
	}
	phase, ok := target.(interruptPhaseSentinel)
	return ok && err.Phase == InterruptPhase(phase)
}

type interruptPhaseSentinel string

var (
	// ErrInterruptValidation marks deterministic request/source validation
	// refusal before a Workers cancellation is attempted.
	ErrInterruptValidation error = interruptPhaseSentinel(InterruptPhaseValidation)
	// ErrInterruptSourceCancellation marks failure to stop the exact source
	// dispatch through the Workers boundary.
	ErrInterruptSourceCancellation error = interruptPhaseSentinel(InterruptPhaseSourceCancellation)
	// ErrInterruptSuccessorAdmission marks failure to reserve or admit the
	// distinct successor after source cancellation committed.
	ErrInterruptSuccessorAdmission error = interruptPhaseSentinel(InterruptPhaseSuccessorAdmission)
)

func (sentinel interruptPhaseSentinel) Error() string { return string(sentinel) }
