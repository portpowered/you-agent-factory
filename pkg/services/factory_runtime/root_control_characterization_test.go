package factory_test

import (
	"context"
	"errors"
	"testing"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

// peerControlService extends the singular root Service fake with control-slice
// outcomes. It depends only on published root types plus the approved Work peer
// contract and never imports factory_runtime/internal.
type peerControlService struct {
	peerRootService

	pauseErr      error
	resumeErr     error
	terminateErr  error
	terminateOut  factoryruntime.TerminateResult
	moveErr       error
	moveResult    factoryruntime.MoveWorkResult
	waitDone      <-chan struct{}
	pauseAccepted bool
	resumeCalls   int
}

var _ factoryruntime.Service = (*peerControlService)(nil)

func (s *peerControlService) Pause(context.Context) error {
	if s.pauseErr != nil {
		return s.pauseErr
	}
	s.pauseAccepted = true
	return nil
}

func (s *peerControlService) Resume(context.Context) error {
	s.resumeCalls++
	return s.resumeErr
}

func (s *peerControlService) Terminate(context.Context, factoryruntime.TerminateRequest) (factoryruntime.TerminateResult, error) {
	if s.terminateErr != nil {
		return factoryruntime.TerminateResult{}, s.terminateErr
	}
	if s.terminateOut.Outcome == "" {
		return factoryruntime.TerminateResult{Outcome: factoryruntime.ControlOutcomeAccepted}, nil
	}
	return s.terminateOut, nil
}

func (s *peerControlService) WaitToComplete() <-chan struct{} {
	if s.waitDone != nil {
		return s.waitDone
	}
	done := make(chan struct{})
	close(done)
	return done
}

func (s *peerControlService) MoveWork(
	_ context.Context,
	workID string,
	stateName string,
	_ work.WorkStateChangeSource,
	_ string,
) (work.OperatorMoveResult, error) {
	if s.moveErr != nil {
		return work.OperatorMoveResult{}, s.moveErr
	}
	if s.moveResult.WorkID != "" {
		return work.OperatorMoveResult{
			WorkID:     s.moveResult.WorkID,
			WorkTypeID: s.moveResult.WorkTypeID,
			FromState:  s.moveResult.FromState,
			ToState:    s.moveResult.ToState,
		}, nil
	}
	return work.OperatorMoveResult{WorkID: workID, FromState: "inbox", ToState: stateName}, nil
}

func TestRootControl_FakePeerPauseAndWaitSuccessShapes(t *testing.T) {
	t.Parallel()

	done := make(chan struct{})
	close(done)
	var runtime factoryruntime.Service = &peerControlService{waitDone: done}

	pauseResult, err := factoryruntime.ApplyPause(context.Background(), runtime, factoryruntime.PauseRequest{})
	if err != nil {
		t.Fatalf("ApplyPause error = %v, want nil", err)
	}
	if pauseResult.Outcome != factoryruntime.ControlOutcomeAccepted {
		t.Fatalf("PauseResult.Outcome = %q, want %q", pauseResult.Outcome, factoryruntime.ControlOutcomeAccepted)
	}

	waitResult := factoryruntime.ApplyWaitToComplete(runtime, factoryruntime.WaitToCompleteRequest{})
	select {
	case <-waitResult.Done:
	default:
		t.Fatal("WaitToCompleteResult.Done was not closed")
	}
}

func TestRootControl_FakePeerTypedLifecycleFailures(t *testing.T) {
	t.Parallel()

	t.Run("not running", func(t *testing.T) {
		t.Parallel()
		var runtime factoryruntime.Service = &peerControlService{pauseErr: factoryruntime.ErrNotRunning}
		_, err := factoryruntime.ApplyPause(context.Background(), runtime, factoryruntime.PauseRequest{})
		if !errors.Is(err, factoryruntime.ErrNotRunning) {
			t.Fatalf("ApplyPause error = %v, want ErrNotRunning", err)
		}
	})

	t.Run("already stopped", func(t *testing.T) {
		t.Parallel()
		var runtime factoryruntime.Service = &peerControlService{terminateErr: factoryruntime.ErrAlreadyStopped}
		_, err := factoryruntime.ApplyTerminate(context.Background(), runtime, factoryruntime.TerminateRequest{Reason: "operator stop"})
		if !errors.Is(err, factoryruntime.ErrAlreadyStopped) {
			t.Fatalf("ApplyTerminate error = %v, want ErrAlreadyStopped", err)
		}
	})

	t.Run("invalid lifecycle transition", func(t *testing.T) {
		t.Parallel()
		var runtime factoryruntime.Service = &peerControlService{resumeErr: factoryruntime.ErrInvalidLifecycleTransition}
		_, err := factoryruntime.ApplyResume(context.Background(), runtime, factoryruntime.ResumeRequest{})
		if !errors.Is(err, factoryruntime.ErrInvalidLifecycleTransition) {
			t.Fatalf("ApplyResume error = %v, want ErrInvalidLifecycleTransition", err)
		}
	})
}

func TestRootControl_FakePeerMoveWorkSuccessAndTypedFailures(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		var runtime factoryruntime.Service = &peerControlService{
			moveResult: factoryruntime.MoveWorkResult{
				WorkID:     "work-1",
				WorkTypeID: "task",
				FromState:  "inbox",
				ToState:    "done",
			},
		}
		got, err := factoryruntime.ApplyMoveWork(context.Background(), runtime, factoryruntime.MoveWorkRequest{
			WorkID:    "work-1",
			StateName: "done",
			Source:    work.WorkStateChangeSourceAPI,
			RequestID: "req-1",
		})
		if err != nil {
			t.Fatalf("ApplyMoveWork error = %v, want nil", err)
		}
		if got != (factoryruntime.MoveWorkResult{
			WorkID:     "work-1",
			WorkTypeID: "task",
			FromState:  "inbox",
			ToState:    "done",
		}) {
			t.Fatalf("MoveWorkResult = %#v, want plain root success shape", got)
		}
	})

	t.Run("missing work", func(t *testing.T) {
		t.Parallel()
		var runtime factoryruntime.Service = &peerControlService{moveErr: factoryruntime.ErrMoveWorkNotFound}
		_, err := factoryruntime.ApplyMoveWork(context.Background(), runtime, factoryruntime.MoveWorkRequest{
			WorkID:    "missing",
			StateName: "done",
			Source:    work.WorkStateChangeSourceAPI,
			RequestID: "req-missing",
		})
		if !errors.Is(err, factoryruntime.ErrMoveWorkNotFound) {
			t.Fatalf("ApplyMoveWork error = %v, want ErrMoveWorkNotFound", err)
		}
	})

	t.Run("conflict in-flight dispatch", func(t *testing.T) {
		t.Parallel()
		var runtime factoryruntime.Service = &peerControlService{moveErr: factoryruntime.ErrMoveWorkInFlightDispatch}
		_, err := factoryruntime.ApplyMoveWork(context.Background(), runtime, factoryruntime.MoveWorkRequest{
			WorkID:    "work-busy",
			StateName: "done",
			Source:    work.WorkStateChangeSourceAPI,
			RequestID: "req-busy",
		})
		if !errors.Is(err, factoryruntime.ErrMoveWorkInFlightDispatch) {
			t.Fatalf("ApplyMoveWork error = %v, want ErrMoveWorkInFlightDispatch", err)
		}
	})
}
