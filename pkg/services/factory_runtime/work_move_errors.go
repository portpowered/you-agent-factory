package factory

import "errors"

var (
	ErrMoveWorkNotFound         = errors.New("work not found")
	ErrMoveWorkInvalidState     = errors.New("invalid target state for work type")
	ErrMoveWorkInFlightDispatch = errors.New("work is in an active dispatch")
	ErrMoveWorkEngineTerminated = errors.New("engine has terminated")
	// ErrMoveWorkRequestConflict indicates that an operator move request ID was
	// already applied and cannot be accepted again.
	ErrMoveWorkRequestConflict = errors.New("factory runtime move work request conflict")
)

// ControlOutcome is the plain success vocabulary for Factory Runtime root
// control operations. Peers branch on these values without hosting or
// orchestration-strategy types.
type ControlOutcome string

const (
	// ControlOutcomeAccepted indicates the control request was applied.
	ControlOutcomeAccepted ControlOutcome = "ACCEPTED"
	// ControlOutcomeNoOp indicates the control request was already satisfied.
	ControlOutcomeNoOp ControlOutcome = "NO_OP"
)

// PauseRequest is the plain pause control input published at the Runtime root.
// TurnID is the immutable Factory invocation correlation captured by the
// upstream control owner. A blank TurnID preserves the historical
// factory-wide pause behavior and selects no Worker Sessions.
type PauseRequest struct {
	TurnID string
	// ControlID is the upstream committed-control identity. Reusing it returns
	// the original captured child evidence instead of selecting a later ledger
	// snapshot. It is optional for compatibility with existing factory-wide
	// controls that do not address a turn.
	ControlID string
}

// PauseResult is the plain pause control success shape published at the Runtime root.
type PauseResult struct {
	Outcome              ControlOutcome
	WorkerSessionControl WorkerSessionControlResult
}

// ResumeRequest is the plain resume control input published at the Runtime root.
// TurnID has the same immutable captured-turn meaning as PauseRequest.TurnID.
type ResumeRequest struct {
	TurnID string
	// ControlID has the same committed-control identity meaning as
	// PauseRequest.ControlID.
	ControlID string
}

// ResumeResult is the plain resume control success shape published at the Runtime root.
type ResumeResult struct {
	Outcome              ControlOutcome
	WorkerSessionControl WorkerSessionControlResult
}

// TerminateRequest is the plain stop control input published at the Runtime
// root. Nested IMP-RUN packets own durable stop wiring; this type is the
// peer-facing request vocabulary. WorkerSessionAction defaults to TERMINATE;
// a captured upstream CANCEL intent sets it to CANCEL so descendants retain
// their distinct Worker Sessions lifecycle outcome.
type TerminateRequest struct {
	Reason string
	// TurnID is the immutable Factory invocation correlation captured by the
	// upstream control owner. A blank TurnID preserves historical factory-wide
	// termination behavior and selects no Worker Sessions.
	TurnID string
	// ControlID is the upstream committed-control identity. Reusing it returns
	// the original captured child evidence instead of selecting a later ledger
	// snapshot or a replacement turn.
	ControlID string
	// WorkerSessionAction is CANCEL or TERMINATE for a turn-scoped stop. The
	// zero value is TERMINATE for compatibility with existing stop callers.
	WorkerSessionAction WorkerSessionControlAction
}

// TerminateResult is the plain terminate/stop control success shape published
// at the Runtime root.
type TerminateResult struct {
	Outcome              ControlOutcome
	WorkerSessionControl WorkerSessionControlResult
}

// WaitToCompleteRequest is the plain wait-to-complete control input published
// at the Runtime root.
type WaitToCompleteRequest struct{}

// WaitToCompleteResult is the plain wait-to-complete success shape. Done is
// closed when the instance has finished all work with no in-flight dispatches.
type WaitToCompleteResult struct {
	Done <-chan struct{}
}

// MoveWorkRequest is the plain operator work-move control input published at
// the Runtime root.
type MoveWorkRequest struct {
	WorkID    string
	StateName string
	Source    WorkMoveSource
	RequestID string
}

// WorkMoveSource identifies the peer boundary requesting an operator move.
// Runtime owns this vocabulary so Service implementers need no Work import.
type WorkMoveSource string

const (
	WorkMoveSourceAPI WorkMoveSource = "api"
	WorkMoveSourceCLI WorkMoveSource = "cli"
)

// MoveWorkResult is the orchestration-neutral operator work-move success shape.
// Peers consume work identity and state names; Petri place/token identifiers
// are not part of this published root vocabulary.
type MoveWorkResult struct {
	WorkID     string
	WorkTypeID string
	FromState  string
	ToState    string
}
