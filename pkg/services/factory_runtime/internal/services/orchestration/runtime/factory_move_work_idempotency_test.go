package runtime

import (
	"context"
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	dispatchplanning "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/dispatch_planning"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

type invalidationCallSpy struct {
	dispatchplanning.Service
	calls []string
	err   error
}

func (s *invalidationCallSpy) InvalidateWork(
	ctx context.Context,
	workID string,
) (dispatchplanning.WorkInvalidationResult, error) {
	s.calls = append(s.calls, workID)
	if s.err != nil {
		return dispatchplanning.WorkInvalidationResult{WorkID: workID}, s.err
	}
	return s.Service.InvalidateWork(ctx, workID)
}

func TestMoveWork_RejectsDuplicateRequestId(t *testing.T) {
	f, err := newTestFactory(
		withNet(buildMoveControlNet()),
		withInlineDispatch(),
		withLogger(logging.NoopLogger{}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	impl := f.(*factoryImpl)
	spy := &invalidationCallSpy{Service: impl.dispatchPlan}
	impl.dispatchPlan = spy

	ctx := context.Background()
	if _, err := submitWorkRequests(ctx, f, []work.SubmitRequest{{
		WorkID:     "work-idempotent",
		WorkTypeID: "task",
		TraceID:    "trace-idempotent",
	}}); err != nil {
		t.Fatalf("SubmitWorkRequest: %v", err)
	}
	if err := tickableFactory(t, f).Tick(ctx); err != nil {
		t.Fatalf("Tick: %v", err)
	}

	if _, err := f.MoveWork(ctx, "work-idempotent", "complete", work.WorkStateChangeSourceAPI, "move-req-dup"); err != nil {
		t.Fatalf("first MoveWork: %v", err)
	}
	if len(spy.calls) != 1 || spy.calls[0] != "work-idempotent" {
		t.Fatalf("successful move invalidation calls = %#v, want one target call", spy.calls)
	}
	if _, err := f.MoveWork(ctx, "work-idempotent", "complete", work.WorkStateChangeSourceAPI, "move-req-dup"); err != nil {
		if !errors.Is(err, work.ErrMoveWorkRequestAlreadyApplied) {
			t.Fatalf("second MoveWork error = %v, want %v", err, work.ErrMoveWorkRequestAlreadyApplied)
		}
	} else {
		t.Fatal("second MoveWork succeeded, want duplicate requestId conflict")
	}
	if len(spy.calls) != 1 {
		t.Fatalf("duplicate move invalidation calls = %#v, want no additional call", spy.calls)
	}
}

func TestMoveWork_ErrorsDoNotInvalidateDispatches(t *testing.T) {
	f, err := newTestFactory(
		withNet(buildMoveControlNet()),
		withInlineDispatch(),
		withLogger(logging.NoopLogger{}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	impl := f.(*factoryImpl)
	spy := &invalidationCallSpy{Service: impl.dispatchPlan}
	impl.dispatchPlan = spy

	ctx := context.Background()
	if _, err := submitWorkRequests(ctx, f, []work.SubmitRequest{
		{WorkID: "work-move-errors", WorkTypeID: "task", TraceID: "trace-move-errors"},
	}); err != nil {
		t.Fatalf("SubmitWorkRequest: %v", err)
	}
	if err := tickableFactory(t, f).Tick(ctx); err != nil {
		t.Fatalf("Tick: %v", err)
	}

	cancelledCtx, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := f.MoveWork(
		cancelledCtx, "work-move-errors", "complete", work.WorkStateChangeSourceAPI, "move-cancelled",
	); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled MoveWork error = %v, want context.Canceled", err)
	}
	if _, err := f.MoveWork(
		ctx, "work-move-errors", "not-a-state", work.WorkStateChangeSourceAPI, "move-invalid-state",
	); !errors.Is(err, factory.ErrMoveWorkInvalidState) {
		t.Fatalf("invalid-state MoveWork error = %v, want %v", err, factory.ErrMoveWorkInvalidState)
	}
	if _, err := f.MoveWork(
		ctx, "missing-work", "complete", work.WorkStateChangeSourceAPI, "move-missing",
	); !errors.Is(err, factory.ErrMoveWorkNotFound) {
		t.Fatalf("missing MoveWork error = %v, want %v", err, factory.ErrMoveWorkNotFound)
	}
	if len(spy.calls) != 0 {
		t.Fatalf("unsuccessful move invalidation calls = %#v, want none", spy.calls)
	}
}

func TestMoveWork_CommitsWhenDispatchInvalidationFails(t *testing.T) {
	f, err := newTestFactory(
		withNet(buildMoveControlNet()),
		withInlineDispatch(),
		withLogger(logging.NoopLogger{}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	impl := f.(*factoryImpl)
	cancelErr := errors.New("Workers cancellation unavailable")
	spy := &invalidationCallSpy{Service: impl.dispatchPlan, err: cancelErr}
	impl.dispatchPlan = spy

	ctx := context.Background()
	if _, err := submitWorkRequests(ctx, f, []work.SubmitRequest{
		{WorkID: "work-cancel-failure", WorkTypeID: "task", TraceID: "trace-cancel-failure"},
	}); err != nil {
		t.Fatalf("SubmitWorkRequest: %v", err)
	}
	if err := tickableFactory(t, f).Tick(ctx); err != nil {
		t.Fatalf("Tick: %v", err)
	}

	result, err := f.MoveWork(
		ctx, "work-cancel-failure", "complete", work.WorkStateChangeSourceAPI, "move-cancel-failure",
	)
	if err != nil {
		t.Fatalf("MoveWork with cancellation failure: %v", err)
	}
	if result.FromState != "init" || result.ToState != "complete" || len(spy.calls) != 1 {
		t.Fatalf("move = %#v, invalidation calls = %#v, want committed move and one target call", result, spy.calls)
	}
	snapshot, err := f.GetEngineStateSnapshot(ctx)
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	if !markingContainsWorkAtPlace(&snapshot.Marking, "work-cancel-failure", "task:complete") {
		t.Fatalf("marking after cancellation failure = %#v, want committed terminal move", snapshot.Marking.Tokens)
	}
}

func TestControlMoveWork_MapsDuplicateRequestIDToRootConflict(t *testing.T) {
	f, err := newTestFactory(
		withNet(buildMoveControlNet()),
		withInlineDispatch(),
		withLogger(logging.NoopLogger{}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	impl := f.(*factoryImpl)

	ctx := context.Background()
	if _, err := submitWorkRequests(ctx, f, []work.SubmitRequest{{
		WorkID: "work-root-conflict", WorkTypeID: "task", TraceID: "trace-root-conflict",
	}}); err != nil {
		t.Fatalf("SubmitWorkRequest: %v", err)
	}
	if err := tickableFactory(t, f).Tick(ctx); err != nil {
		t.Fatalf("Tick: %v", err)
	}

	request := factory.MoveWorkRequest{
		WorkID: "work-root-conflict", StateName: "complete",
		Source: factory.WorkMoveSourceAPI, RequestID: "move-root-dup",
	}
	if _, err := impl.ControlMoveWork(ctx, request); err != nil {
		t.Fatalf("first ControlMoveWork: %v", err)
	}
	if _, err := impl.ControlMoveWork(ctx, request); !errors.Is(err, factory.ErrMoveWorkRequestConflict) {
		t.Fatalf("second ControlMoveWork error = %v, want %v", err, factory.ErrMoveWorkRequestConflict)
	}
}
