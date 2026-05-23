package testutil

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestServiceTestHarnessMarkingFallsBackToCachedSnapshot(t *testing.T) {
	cfg := PipelineConfig(1, "processor")
	dir := ScaffoldFactoryDir(t, cfg)

	h := NewServiceTestHarness(t, dir)
	h.MockWorker("processor",
		interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted},
		interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted},
	)

	if err := h.SubmitWork("task", []byte(`{"title":"cache final marking"}`)); err != nil {
		t.Fatalf("submit work: %v", err)
	}
	h.RunUntilComplete(t, 5*time.Second)

	// Simulate the runtime disappearing after the run completes. Marking()
	// should still expose the last successful terminal snapshot.
	h.svc = nil

	snap := h.Marking()
	if snap == nil {
		t.Fatal("Marking() returned nil snapshot")
	}
	if got := len(snap.TokensInPlace("task:complete")); got != 1 {
		t.Fatalf("TokensInPlace(task:complete) = %d, want 1", got)
	}
	if got := len(snap.TokensInPlace("task:init")); got != 0 {
		t.Fatalf("TokensInPlace(task:init) = %d, want 0", got)
	}
}
