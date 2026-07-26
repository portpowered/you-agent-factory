package factory

import "errors"

var (
	ErrMoveWorkNotFound         = errors.New("work not found")
	ErrMoveWorkInvalidState     = errors.New("invalid target state for work type")
	ErrMoveWorkInFlightDispatch = errors.New("work is in an active dispatch")
	ErrMoveWorkEngineTerminated = errors.New("engine has terminated")
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
type PauseRequest struct{}

// PauseResult is the plain pause control success shape published at the Runtime root.
type PauseResult struct {
	Outcome ControlOutcome
}

// ResumeRequest is the plain resume control input published at the Runtime root.
type ResumeRequest struct{}

// ResumeResult is the plain resume control success shape published at the Runtime root.
type ResumeResult struct {
	Outcome ControlOutcome
}

// TerminateRequest is the plain terminate/stop control input published at the
// Runtime root. Nested IMP-RUN packets own durable stop wiring; this type is
// the peer-facing request vocabulary.
type TerminateRequest struct {
	Reason string
}

// TerminateResult is the plain terminate/stop control success shape published
// at the Runtime root.
type TerminateResult struct {
	Outcome ControlOutcome
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
