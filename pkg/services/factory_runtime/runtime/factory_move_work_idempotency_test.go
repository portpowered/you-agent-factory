package runtime

import (
	"context"
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
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
