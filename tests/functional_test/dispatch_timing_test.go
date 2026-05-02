package functional_test

import (
	"context"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil"
)

// TestDispatchTiming_HistoryRecordsDuration validates that after a dispatch
// completes, the RuntimeState.DispatchHistory contains an entry with a
// Duration >= the executor's processing time.
func TestDispatchTiming_HistoryRecordsDuration(t *testing.T) {
	dir := testutil.ScaffoldFactoryDir(t, persistTestPipelineConfig())

	// sleepyExecutor sleeps for a fixed duration before returning ACCEPTED.
	const sleepDuration = 100 * time.Millisecond
	h := testutil.NewServiceTestHarness(t, dir,
		// TODO: migrate to WithFullWorkerPoolAndScriptWrap
		testutil.WithRunAsync(),
	)
	h.SetCustomExecutor("step-worker", &sleepyExecutor{sleep: sleepDuration})

	// Submit work and run to completion.
	h.SubmitWork("task", []byte(`{"item": "timing-test"}`))
	h.RunUntilComplete(t, 10*time.Second)

	// After completion, inspect the canonical engine snapshot for dispatch history.
	rtSnap, err := h.GetEngineStateSnapshot()
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot failed: %v", err)
	}

	if len(rtSnap.DispatchHistory) == 0 {
		t.Fatal("expected at least 1 CompletedDispatch in DispatchHistory, got 0")
	}

	for _, cd := range rtSnap.DispatchHistory {
		tokenIdentities := deriveTokenIdentities(cd.ConsumedTokens, cd.OutputMutations)
		if cd.Duration < sleepDuration {
			t.Errorf("CompletedDispatch %s duration %v < expected minimum %v",
				cd.TransitionID, cd.Duration, sleepDuration)
		}
		if cd.StartTime.IsZero() {
			t.Errorf("CompletedDispatch %s has zero StartTime", cd.TransitionID)
		}
		if cd.EndTime.IsZero() {
			t.Errorf("CompletedDispatch %s has zero EndTime", cd.TransitionID)
		}
		if !cd.EndTime.After(cd.StartTime) {
			t.Errorf("CompletedDispatch %s EndTime (%v) not after StartTime (%v)",
				cd.TransitionID, cd.EndTime, cd.StartTime)
		}
		if len(tokenIdentities.WorkIDs) == 0 {
			t.Errorf("CompletedDispatch %s has no work ID", cd.TransitionID)
		}
		if len(tokenIdentities.WorkTypes) == 0 {
			t.Errorf("CompletedDispatch %s has no work type", cd.TransitionID)
		}
	}
}

// TestDispatchTiming_InFlightStartTime validates that during execution,
// EngineStateSnapshot.Dispatches has an active dispatch with StartTime before now.
func TestDispatchTiming_InFlightStartTime(t *testing.T) {
	dir := testutil.ScaffoldFactoryDir(t, persistTestPipelineConfig())

	releaseCh := make(chan struct{})
	h := testutil.NewServiceTestHarness(t, dir,
		// TODO: migrate to WithFullWorkerPoolAndScriptWrap

		testutil.WithRunAsync(),
	)
	h.SetCustomExecutor("step-worker", &channelExecutor{releaseCh: releaseCh})
	beforeSubmit := time.Now()

	// Seed work before starting the async run loop so the engine cannot
	// terminate on an empty queue before this test submits work.
	h.SubmitWork("task", []byte(`{"item": "starttime-test"}`))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	errCh := h.RunInBackground(ctx)

	// Wait for the executor to be invoked.
	time.Sleep(200 * time.Millisecond)

	rtSnap, err := h.GetEngineStateSnapshot()
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot failed: %v", err)
	}

	if len(rtSnap.Dispatches) == 0 {
		t.Fatal("expected at least 1 active dispatch, got 0")
	}

	for _, entry := range rtSnap.Dispatches {
		if entry.StartTime.Before(beforeSubmit) {
			t.Errorf("dispatch %s StartTime (%v) is before submission time (%v)",
				entry.TransitionID, entry.StartTime, beforeSubmit)
		}
		if entry.StartTime.After(time.Now()) {
			t.Errorf("dispatch %s StartTime (%v) is in the future",
				entry.TransitionID, entry.StartTime)
		}
	}

	close(releaseCh)

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

// sleepyExecutor sleeps for a fixed duration before returning ACCEPTED.
// Requires custom executor: MockProvider cannot simulate execution timing delays.
type sleepyExecutor struct {
	sleep time.Duration
}

func (e *sleepyExecutor) Execute(_ context.Context, d interfaces.WorkDispatch) (interfaces.WorkResult, error) {
	time.Sleep(e.sleep)
	return interfaces.WorkResult{
		DispatchID:   d.DispatchID,
		TransitionID: d.TransitionID,
		Outcome:      interfaces.OutcomeAccepted,
	}, nil
}

// channelExecutor blocks until releaseCh is closed, then returns ACCEPTED.
// Requires custom executor: MockProvider cannot block mid-execution for synchronization.
type channelExecutor struct {
	releaseCh <-chan struct{}
}

func (e *channelExecutor) Execute(_ context.Context, d interfaces.WorkDispatch) (interfaces.WorkResult, error) {
	<-e.releaseCh
	return interfaces.WorkResult{
		DispatchID:   d.DispatchID,
		TransitionID: d.TransitionID,
		Outcome:      interfaces.OutcomeAccepted,
	}, nil
}
