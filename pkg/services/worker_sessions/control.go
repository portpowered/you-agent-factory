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
	ControlActionInterrupt ControlAction = "INTERRUPT"
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

// ControlRequest identifies exactly one stable Worker Session. RequestID is an
// optional caller-owned control identity used to correlate durable request and
// outcome records; the service derives a stable fallback when it is blank. A
// supervised session owns at most one immutable dispatch attempt at a time;
// the returned ControlResult carries that dispatch identity without requiring
// callers to reconstruct or guess it from a Worker execution request.
type ControlRequest struct {
	ID        string
	RequestID string
}

// Validate reports whether req identifies one stable Worker Session.
func (req ControlRequest) Validate() error {
	if !validSessionID(req.ID) {
		return ErrInvalidSessionID
	}
	return nil
}

// ControlRecordType distinguishes the two source-native records that bracket
// one lifecycle control. The request is committed before Worker Sessions
// invokes a Workers boundary; the outcome is committed by the owning
// supervision path before any resulting terminal record.
type ControlRecordType string

const (
	ControlRecordTypeRequest ControlRecordType = "REQUEST"
	ControlRecordTypeOutcome ControlRecordType = "OUTCOME"
)

// ControlRecordPayload is the detached payload used by Worker Session
// control history. It deliberately carries both the caller correlation and
// the server-owned attempt identity so durable replay never has to infer what
// a control targeted from a later terminal record.
type ControlRecordPayload struct {
	RecordType      ControlRecordType `json:"recordType"`
	Action          ControlAction     `json:"action"`
	Outcome         ControlOutcome    `json:"outcome,omitempty"`
	RequestID       string            `json:"requestId"`
	CorrelationID   string            `json:"correlationId"`
	WorkerSessionID string            `json:"workerSessionId"`
	DispatchID      string            `json:"dispatchId,omitempty"`
	AttemptID       string            `json:"attemptId,omitempty"`
	State           State             `json:"state,omitempty"`
}

// Validate reports whether the payload is a complete request or outcome
// bracket. It is intentionally independent of registry state so retained and
// portable records can be checked without Workers or Providers.
func (payload ControlRecordPayload) Validate() error {
	if payload.RecordType != ControlRecordTypeRequest && payload.RecordType != ControlRecordTypeOutcome {
		return fmt.Errorf("worker session: invalid control record type %q", payload.RecordType)
	}
	switch payload.Action {
	case ControlActionPause, ControlActionResume, ControlActionCancel, ControlActionTerminate, ControlActionInterrupt:
	default:
		return fmt.Errorf("worker session: invalid control record action %q", payload.Action)
	}
	if !validSessionID(payload.WorkerSessionID) || strings.TrimSpace(payload.RequestID) == "" || strings.TrimSpace(payload.CorrelationID) == "" {
		return ErrInvalidControlRecord
	}
	if payload.RecordType == ControlRecordTypeRequest && payload.Outcome != "" {
		return ErrInvalidControlRecord
	}
	if payload.RecordType == ControlRecordTypeOutcome {
		switch payload.Outcome {
		case ControlOutcomeApplied, ControlOutcomeNoop, ControlOutcomeUnsupported, ControlOutcomeFailed:
		default:
			return ErrInvalidControlRecord
		}
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
