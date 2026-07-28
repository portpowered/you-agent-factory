package runtime

import (
	"context"
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

func TestMoveWork_RejectsDuplicateRequestId(t *testing.T) {
	f, err := newTestFactory(
		withNet(buildMoveControlNet()),
		withInlineDispatch(),
		withLogger(logging.NoopLogger{}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

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
	if _, err := f.MoveWork(ctx, "work-idempotent", "complete", work.WorkStateChangeSourceAPI, "move-req-dup"); err != nil {
		if !errors.Is(err, work.ErrMoveWorkRequestAlreadyApplied) {
			t.Fatalf("second MoveWork error = %v, want %v", err, work.ErrMoveWorkRequestAlreadyApplied)
		}
	} else {
		t.Fatal("second MoveWork succeeded, want duplicate requestId conflict")
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
