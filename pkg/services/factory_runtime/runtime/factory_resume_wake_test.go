package runtime

import (
	"context"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

type gatedMultiWorkerExecutor struct {
	started chan struct{}
	release chan struct{}
}

func (e *gatedMultiWorkerExecutor) Execute(_ context.Context, dispatch work.WorkDispatch) (workerexecution.WorkResult, error) {
	e.started <- struct{}{}
	<-e.release
	return workerexecution.WorkResult{
		DispatchID:   dispatch.DispatchID,
		TransitionID: dispatch.TransitionID,
		Outcome:      workerexecution.OutcomeAccepted,
		Output:       "done",
	}, nil
}

func waitForWorkerStarts(t *testing.T, started <-chan struct{}, count int) {
	t.Helper()
	for i := 0; i < count; i++ {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for worker start %d/%d", i+1, count)
		}
	}
}

func workIDsFromDispatchHistory(history []interfaces.CompletedDispatch) []string {
	workIDs := make([]string, 0, len(history))
	for _, completed := range history {
		for _, token := range completed.ConsumedTokens {
			if token.Color.WorkID != "" {
				workIDs = append(workIDs, token.Color.WorkID)
				break
			}
		}
	}
	return workIDs
}

func dispatchHistoryContainsWorkIDs(t *testing.T, history []interfaces.CompletedDispatch, wantWorkIDs []string) {
	t.Helper()
	got := workIDsFromDispatchHistory(history)
	if len(got) != len(wantWorkIDs) {
		t.Fatalf("dispatch history count = %d, want %d: %#v", len(got), len(wantWorkIDs), history)
	}
	seen := make(map[string]int, len(got))
	for _, workID := range got {
		seen[workID]++
	}
	for _, workID := range wantWorkIDs {
		if seen[workID] != 1 {
			t.Fatalf("dispatch history work IDs = %v, want each of %v exactly once", got, wantWorkIDs)
		}
	}
}

func allWorksAtDonePlace(marking *petri.MarkingSnapshot, workIDs []string) bool {
	for _, workID := range workIDs {
		if !markingContainsWorkAtPlace(marking, workID, "task:done") {
			return false
		}
	}
	return true
}

func TestResumeWakesOneBufferedSubmissionWhilePaused(t *testing.T) {
	h := startServiceModeRunHarness(t,
		withNet(buildSimpleNet()),
		withServiceMode(),
		withInlineDispatch(),
		withWorkerExecutor("mock", &passExecutor{}),
		withLogger(logging.NoopLogger{}),
	)
	defer h.stop()

	h.pauseAndWait()
	submitTaskWithWorkID(t, h.Factory, "work-resume-wake", "trace-resume-wake")
	assertWorkNotAtDonePlace(t, h.Factory, "work-resume-wake")

	resumeFactory(t, h.Factory)
	waitForWorkDoneAfterResume(t, h.Factory, "work-resume-wake")
}

func TestResumeWakesOneBufferedWorkerResultWhilePaused(t *testing.T) {
	executor := &blockingExecutor{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	h := startServiceModeRunHarness(t,
		withNet(buildSimpleNet()),
		withServiceMode(),
		withWorkerExecutor("mock", executor),
		withLogger(logging.NoopLogger{}),
	)
	defer h.stop()

	submitTaskWithWorkID(t, h.Factory, "work-resume-result", "trace-resume-result")
	waitForBlockingWorkerStart(t, executor, h.errCh)
	h.pauseAndWait()
	close(executor.release)
	assertWorkNotAtDonePlace(t, h.Factory, "work-resume-result")

	resumeFactory(t, h.Factory)
	waitForWorkDoneAfterResume(t, h.Factory, "work-resume-result")
}

func TestResumeDrainsMultipleBufferedSubmissionsToQuiescenceWhilePaused(t *testing.T) {
	workIDs := []string{"work-resume-drain-a", "work-resume-drain-b", "work-resume-drain-c"}
	h := startServiceModeRunHarness(t,
		withNet(buildSimpleNet()),
		withServiceMode(),
		withInlineDispatch(),
		withWorkerExecutor("mock", &passExecutor{}),
		withLogger(logging.NoopLogger{}),
	)
	defer h.stop()

	h.pauseAndWait()
	for _, workID := range workIDs {
		submitTaskWithWorkID(t, h.Factory, workID, "trace-"+workID)
	}
	assertWorksNotAtDonePlace(t, h.Factory, workIDs)

	resumeFactory(t, h.Factory)
	snap := waitForQuiescentWorksAtDone(t, h.Factory, workIDs)
	assertDispatchOrder(t, snap.DispatchHistory, workIDs)
}

func TestResumeDrainsMultipleBufferedWorkerResultsToQuiescenceWhilePaused(t *testing.T) {
	workIDs := []string{"work-resume-result-a", "work-resume-result-b", "work-resume-result-c"}
	executor := &gatedMultiWorkerExecutor{
		started: make(chan struct{}, len(workIDs)),
		release: make(chan struct{}),
	}
	h := startServiceModeRunHarness(t,
		withNet(buildSimpleNet()),
		withServiceMode(),
		withWorkerExecutor("mock", executor),
		withLogger(logging.NoopLogger{}),
	)
	defer h.stop()

	for _, workID := range workIDs {
		submitTaskWithWorkID(t, h.Factory, workID, "trace-"+workID)
	}
	waitForWorkerStarts(t, executor.started, len(workIDs))
	h.pauseAndWait()
	close(executor.release)
	assertWorksNotAtDonePlace(t, h.Factory, workIDs)

	resumeFactory(t, h.Factory)
	snap := waitForQuiescentWorksAtDone(t, h.Factory, workIDs)
	dispatchHistoryContainsWorkIDs(t, snap.DispatchHistory, workIDs)
}

func TestResumeOnRunningFactoryIsAcceptedNoOp(t *testing.T) {
	h := startServiceModeRunHarness(t,
		withNet(buildSimpleNet()),
		withServiceMode(),
		withInlineDispatch(),
		withWorkerExecutor("mock", &passExecutor{}),
		withLogger(logging.NoopLogger{}),
	)
	defer h.stop()

	snapBefore, err := h.Factory.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot before resume: %v", err)
	}
	if snapBefore.FactoryState != string(interfaces.FactoryStateRunning) {
		t.Fatalf("factory state = %q, want %q before resume no-op", snapBefore.FactoryState, interfaces.FactoryStateRunning)
	}

	resumeFactory(t, h.Factory)

	snapAfter, err := h.Factory.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot after resume: %v", err)
	}
	if snapAfter.FactoryState != string(interfaces.FactoryStateRunning) {
		t.Fatalf("factory state = %q, want %q after resume no-op", snapAfter.FactoryState, interfaces.FactoryStateRunning)
	}
	if len(snapAfter.DispatchHistory) != len(snapBefore.DispatchHistory) {
		t.Fatalf("dispatch history count = %d, want unchanged %d after resume no-op", len(snapAfter.DispatchHistory), len(snapBefore.DispatchHistory))
	}
	if snapAfter.InFlightCount != snapBefore.InFlightCount {
		t.Fatalf("in-flight count = %d, want unchanged %d after resume no-op", snapAfter.InFlightCount, snapBefore.InFlightCount)
	}
}

func TestResumeRepeatedAfterBufferedDrainDoesNotReprocessWork(t *testing.T) {
	h := startServiceModeRunHarness(t,
		withNet(buildSimpleNet()),
		withServiceMode(),
		withInlineDispatch(),
		withWorkerExecutor("mock", &passExecutor{}),
		withLogger(logging.NoopLogger{}),
	)
	defer h.stop()

	h.pauseAndWait()
	submitTaskWithWorkID(t, h.Factory, "work-resume-repeat", "trace-resume-repeat")
	resumeFactory(t, h.Factory)

	snap := waitForQuiescentWorksAtDone(t, h.Factory, []string{"work-resume-repeat"})
	drainedDispatchCount := len(snap.DispatchHistory)

	resumeFactory(t, h.Factory)
	resumeFactory(t, h.Factory)

	snapAfter, err := h.Factory.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot after repeated resume: %v", err)
	}
	if snapAfter.FactoryState != string(interfaces.FactoryStateRunning) {
		t.Fatalf("factory state = %q, want %q after repeated resume", snapAfter.FactoryState, interfaces.FactoryStateRunning)
	}
	if len(snapAfter.DispatchHistory) != drainedDispatchCount {
		t.Fatalf("dispatch history count = %d, want unchanged %d after repeated resume", len(snapAfter.DispatchHistory), drainedDispatchCount)
	}
	dispatchHistoryContainsWorkIDs(t, snapAfter.DispatchHistory, []string{"work-resume-repeat"})
	if !markingContainsWorkAtPlace(&snapAfter.Marking, "work-resume-repeat", "task:done") {
		t.Fatalf("marking = %#v, want work-resume-repeat at task:done after repeated resume", snapAfter.Marking.Tokens)
	}
	if snapAfter.InFlightCount != 0 {
		t.Fatalf("in-flight count = %d, want 0 after repeated resume", snapAfter.InFlightCount)
	}
}

func TestResumeRepeatedWhilePausedWakesBufferedWorkOnce(t *testing.T) {
	h := startServiceModeRunHarness(t,
		withNet(buildSimpleNet()),
		withServiceMode(),
		withInlineDispatch(),
		withWorkerExecutor("mock", &passExecutor{}),
		withLogger(logging.NoopLogger{}),
	)
	defer h.stop()

	h.pauseAndWait()
	submitTaskWithWorkID(t, h.Factory, "work-resume-double", "trace-resume-double")
	resumeFactory(t, h.Factory)
	resumeFactory(t, h.Factory)

	snap := waitForQuiescentWorksAtDone(t, h.Factory, []string{"work-resume-double"})
	dispatchHistoryContainsWorkIDs(t, snap.DispatchHistory, []string{"work-resume-double"})
}
