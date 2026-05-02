package workers

import (
	"context"
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestNoopExecutor_ReturnsAccepted(t *testing.T) {
	result, err := (&NoopExecutor{}).Execute(context.Background(), interfaces.WorkDispatch{
		DispatchID:   "d-1",
		TransitionID: "t1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.TransitionID != "t1" {
		t.Fatalf("TransitionID = %q, want %q", result.TransitionID, "t1")
	}
	if result.Outcome != interfaces.OutcomeAccepted {
		t.Fatalf("Outcome = %q, want %q", result.Outcome, interfaces.OutcomeAccepted)
	}
}
