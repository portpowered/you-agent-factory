//go:build functionallong

package runtime_api

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/agent-factory/pkg/factory/state"
	"github.com/portpowered/agent-factory/pkg/interfaces"
	"github.com/portpowered/agent-factory/pkg/petri"
	"github.com/portpowered/agent-factory/pkg/testutil"
	"github.com/portpowered/agent-factory/tests/functional/internal/support"
)

func TestRuntimeState_ThreeStagePipeline(t *testing.T) {
	support.SkipLongFunctional(t, "slow runtime-state three-stage pipeline sweep")

	cfg := threeStageConfig()
	dir := testutil.ScaffoldFactoryDir(t, cfg)

	const sleepDuration = 10 * time.Millisecond
	h := testutil.NewServiceTestHarness(t, dir, testutil.WithRunAsync())
	h.SetCustomExecutor("step-worker", &sleepyExecutor{sleep: sleepDuration})

	const numItems = 5
	for i := 1; i <= numItems; i++ {
		h.SubmitWork("task", []byte(fmt.Sprintf(`{"item": "w%d"}`, i)))
	}
	h.RunUntilComplete(t, 30*time.Second)

	rtSnap, err := h.GetEngineStateSnapshot()
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot failed: %v", err)
	}
	if rtSnap.RuntimeStatus != interfaces.RuntimeStatusFinished {
		t.Fatalf("RuntimeStatus = %q, want %q", rtSnap.RuntimeStatus, interfaces.RuntimeStatusFinished)
	}

	terminalCount := 0
	for _, tok := range rtSnap.Marking.Tokens {
		if tok.PlaceID == state.PlaceID("task", "complete") {
			terminalCount++
		}
	}
	if terminalCount != numItems {
		t.Errorf("expected %d terminal tokens, got %d", numItems, terminalCount)
	}

	if len(rtSnap.DispatchHistory) < 3 {
		t.Errorf("expected at least 3 dispatch history entries (one per stage), got %d", len(rtSnap.DispatchHistory))
	}
	for _, cd := range rtSnap.DispatchHistory {
		tokenIdentities := support.DeriveTokenIdentities(cd.ConsumedTokens, cd.OutputMutations)
		if cd.Duration < sleepDuration {
			t.Errorf("dispatch %s duration %v < expected minimum %v", cd.TransitionID, cd.Duration, sleepDuration)
		}
		if cd.StartTime.IsZero() {
			t.Errorf("dispatch %s has zero StartTime", cd.TransitionID)
		}
		if cd.EndTime.IsZero() {
			t.Errorf("dispatch %s has zero EndTime", cd.TransitionID)
		}
		if len(tokenIdentities.WorkIDs) == 0 {
			t.Errorf("dispatch %s has no work ID", cd.TransitionID)
		}
		if len(tokenIdentities.WorkTypes) == 0 {
			t.Errorf("dispatch %s has no work type", cd.TransitionID)
		}
	}
}

func TestRuntimeState_MidExecutionConsistency(t *testing.T) {
	support.SkipLongFunctional(t, "slow runtime-state mid-execution consistency sweep")

	dir := testutil.ScaffoldFactoryDir(t, midExecutionConsistencyConfig())
	blockExec, releaseCh := newMidExecutionBlockingExecutor()
	h := testutil.NewServiceTestHarness(t, dir, testutil.WithRunAsync())
	h.SetCustomExecutor("step-worker", blockExec)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	errCh := h.RunInBackground(ctx)

	h.SubmitWork("task", []byte(`{"item": "mid-exec"}`))
	assertMidExecutionSnapshot(t, waitForMidExecutionSnapshot(t, h, 2*time.Second))
	close(releaseCh)
	waitForMidExecutionHarnessCompletion(t, h, errCh, cancel, ctx)
}

func newMidExecutionBlockingExecutor() (*blockingExecutor, chan struct{}) {
	releaseCh := make(chan struct{})
	var mu sync.Mutex
	calls := 0
	blockExec := &blockingExecutor{releaseCh: releaseCh, mu: &mu, calls: &calls}
	return blockExec, releaseCh
}

func midExecutionConsistencyConfig() *interfaces.FactoryConfig {
	return &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{{Name: "task", States: []interfaces.StateConfig{
			{Name: "init", Type: interfaces.StateTypeInitial},
			{Name: "complete", Type: interfaces.StateTypeTerminal},
			{Name: "failed", Type: interfaces.StateTypeFailed},
		}}},
		Workers: []interfaces.WorkerConfig{{Name: "step-worker"}},
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name: "process", WorkerTypeName: "step-worker",
			Inputs:  []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}},
			Outputs: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "complete"}},
		}},
	}
}

func waitForMidExecutionSnapshot(
	t *testing.T,
	h *testutil.ServiceTestHarness,
	timeout time.Duration,
) *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net] {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		rtSnap, err := h.GetEngineStateSnapshot()
		if err != nil {
			t.Fatalf("GetEngineStateSnapshot failed: %v", err)
		}
		if rtSnap.RuntimeStatus == interfaces.RuntimeStatusActive && rtSnap.InFlightCount > 0 {
			return rtSnap
		}
		if time.Now().After(deadline) {
			t.Fatalf("RuntimeSnapshot = %#v, want active state with in-flight dispatch", rtSnap)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func assertMidExecutionSnapshot(
	t *testing.T,
	rtSnap *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net],
) {
	t.Helper()
	for _, tok := range rtSnap.Marking.Tokens {
		if tok.PlaceID == state.PlaceID("task", "init") {
			t.Errorf("token %s still in init place during dispatch; should be consumed", tok.ID)
		}
	}
	if rtSnap.InFlightCount == 0 {
		t.Error("expected InFlightCount > 0 during blocking dispatch")
	}
	if len(rtSnap.Dispatches) == 0 {
		t.Error("expected at least 1 entry in Dispatches during blocking dispatch")
	}
}

func waitForMidExecutionHarnessCompletion(
	t *testing.T,
	h *testutil.ServiceTestHarness,
	errCh <-chan error,
	cancel context.CancelFunc,
	ctx context.Context,
) {
	t.Helper()
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

func threeStageConfig() *interfaces.FactoryConfig {
	return &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{{Name: "task", States: []interfaces.StateConfig{
			{Name: "init", Type: interfaces.StateTypeInitial},
			{Name: "stage1", Type: interfaces.StateTypeProcessing},
			{Name: "stage2", Type: interfaces.StateTypeProcessing},
			{Name: "complete", Type: interfaces.StateTypeTerminal},
			{Name: "failed", Type: interfaces.StateTypeFailed},
		}}},
		Workers: []interfaces.WorkerConfig{{Name: "step-worker"}},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{Name: "step1", WorkerTypeName: "step-worker", Inputs: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}}, Outputs: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "stage1"}}},
			{Name: "step2", WorkerTypeName: "step-worker", Inputs: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "stage1"}}, Outputs: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "stage2"}}},
			{Name: "finish", WorkerTypeName: "step-worker", Inputs: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "stage2"}}, Outputs: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "complete"}}},
		},
	}
}
