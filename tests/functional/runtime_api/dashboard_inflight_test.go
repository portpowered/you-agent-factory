package runtime_api

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/agent-factory/pkg/testutil"
	"github.com/portpowered/agent-factory/tests/functional/internal/support"
)

// TestDashboard_InFlightDispatches validates that the runtime state shows
// dispatched work as in-flight when a worker executor is blocking.
// A slow executor blocks until released; the runtime state snapshot is taken
// during that window and must show the dispatch with Duration > 0.
func TestDashboard_InFlightDispatches(t *testing.T) {
	dir := testutil.ScaffoldFactoryDir(t, persistTestPipelineConfig())

	// blockingExecutor blocks until released via a channel.
	releaseCh := make(chan struct{})
	var mu sync.Mutex
	calls := 0
	blockExec := &blockingExecutor{
		releaseCh: releaseCh,
		mu:        &mu,
		calls:     &calls,
	}

	h := testutil.NewServiceTestHarness(t, dir,
		// TODO: fix me - this test should not require async mode, but currently does because the mock executor is registered after construction. Refactor to allow pre-construction registration of custom executors, which would let us run this test in sync mode for more deterministic timing.
		testutil.WithRunAsync(),
	)
	h.SetCustomExecutor("step-worker", blockExec)

	// Start the factory in the background.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	errCh := h.RunInBackground(ctx)

	// Submit one work item.
	h.SubmitWork("task", []byte(`{"item": "inflight-test"}`))

	// Wait a short time for the dispatcher to fire and the executor to block.
	time.Sleep(200 * time.Millisecond)

	// Take a canonical engine snapshot while the executor is blocking.
	rt, err := h.GetEngineStateSnapshot()
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot failed: %v", err)
	}

	// Verify in-flight dispatches.
	if len(rt.Dispatches) == 0 {
		t.Errorf("expected at least 1 in-flight dispatch, got 0 (marking tokens: %d, state: %s)",
			len(rt.Marking.Tokens), rt.FactoryState)
	}
	now := time.Now()
	for _, d := range rt.Dispatches {
		dur := now.Sub(d.StartTime)
		tokenIdentities := support.DeriveTokenIdentities(d.ConsumedTokens, nil)
		dispatchLabel := d.TransitionID
		if len(tokenIdentities.WorkIDs) > 0 {
			dispatchLabel = tokenIdentities.WorkIDs[0]
		}
		if dur <= 0 {
			t.Errorf("expected Duration > 0 for in-flight dispatch %s, got %v", dispatchLabel, dur)
		}
		if d.TransitionID == "" {
			t.Error("expected non-empty TransitionID for in-flight dispatch")
		}
	}

	// Release the executor to let the pipeline complete.
	close(releaseCh)

	// Wait for completion.
	select {
	case <-h.WaitToComplete():
		cancel()
	case <-ctx.Done():
		t.Fatal("timed out waiting for factory to complete")
	}
	if err := <-errCh; err != nil && err != context.Canceled {
		t.Fatalf("factory run error: %v", err)
	}
}
