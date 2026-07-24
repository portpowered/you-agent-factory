package factory

import (
	"context"

	"github.com/portpowered/infinite-you/pkg/services/work"
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
// the Runtime root. Source uses the approved Work peer vocabulary.
type MoveWorkRequest struct {
	WorkID    string
	StateName string
	Source    work.WorkStateChangeSource
	RequestID string
}

// MoveWorkResult is the orchestration-neutral operator work-move success shape.
// Peers consume work identity and state names; Petri place/token identifiers
// are not part of this published root vocabulary.
type MoveWorkResult struct {
	WorkID     string
	WorkTypeID string
	FromState  string
	ToState    string
}

// ApplyPause exercises the published pause control contract against Service.
func ApplyPause(ctx context.Context, runtime Service, _ PauseRequest) (PauseResult, error) {
	if runtime == nil {
		return PauseResult{}, ErrNotFound
	}
	if err := runtime.Pause(ctx); err != nil {
		return PauseResult{}, err
	}
	return PauseResult{Outcome: ControlOutcomeAccepted}, nil
}

// ApplyResume exercises the published resume control contract against Service.
func ApplyResume(ctx context.Context, runtime Service, _ ResumeRequest) (ResumeResult, error) {
	if runtime == nil {
		return ResumeResult{}, ErrNotFound
	}
	if err := runtime.Resume(ctx); err != nil {
		return ResumeResult{}, err
	}
	return ResumeResult{Outcome: ControlOutcomeAccepted}, nil
}

// ApplyTerminate exercises the published terminate/stop control contract against Service.
func ApplyTerminate(ctx context.Context, runtime Service, req TerminateRequest) (TerminateResult, error) {
	if runtime == nil {
		return TerminateResult{}, ErrNotFound
	}
	return runtime.Terminate(ctx, req)
}

// ApplyWaitToComplete exercises the published wait-to-complete control contract
// against Service.
func ApplyWaitToComplete(runtime Service, _ WaitToCompleteRequest) WaitToCompleteResult {
	if runtime == nil {
		done := make(chan struct{})
		close(done)
		return WaitToCompleteResult{Done: done}
	}
	return WaitToCompleteResult{Done: runtime.WaitToComplete()}
}

// ApplyMoveWork exercises the published operator work-move control contract
// against Service and projects the plain root success shape.
func ApplyMoveWork(ctx context.Context, runtime Service, req MoveWorkRequest) (MoveWorkResult, error) {
	if runtime == nil {
		return MoveWorkResult{}, ErrNotFound
	}
	got, err := runtime.MoveWork(ctx, req.WorkID, req.StateName, req.Source, req.RequestID)
	if err != nil {
		return MoveWorkResult{}, err
	}
	return MoveWorkResult{
		WorkID:     got.WorkID,
		WorkTypeID: got.WorkTypeID,
		FromState:  got.FromState,
		ToState:    got.ToState,
	}, nil
}
