package pool_test

import (
	"context"
	"testing"

	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"

	"github.com/portpowered/infinite-you/pkg/work"
	"github.com/portpowered/infinite-you/pkg/workers/executor"
)

func TestNoopExecutor_ReturnsAccepted(t *testing.T) {
	result, err := (&executor.NoopExecutor{}).Execute(context.Background(), work.WorkDispatch{
		DispatchID:   "d-1",
		TransitionID: "t1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.TransitionID != "t1" {
		t.Fatalf("TransitionID = %q, want %q", result.TransitionID, "t1")
	}
	if result.Outcome != workerexecution.OutcomeAccepted {
		t.Fatalf("Outcome = %q, want %q", result.Outcome, workerexecution.OutcomeAccepted)
	}
}
