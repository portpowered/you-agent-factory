package replay_contracts

import (
	"path/filepath"
	"sort"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/factory/scheduler"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
	"github.com/portpowered/infinite-you/pkg/testutil"
	"github.com/portpowered/infinite-you/pkg/workers"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestReplayAdhocWorkInQueueScheduler_PrioritizesInitializedTraceProgressOverFIFO(t *testing.T) {
	artifactPath := support.AgentFactoryPath(t, filepath.Join("tests", "functional_test", "testdata", "adhoc-recording-batch-event-log.json"))
	artifact := testutil.LoadReplayArtifact(t, artifactPath)
	assertRecordedAdhocReplayIsUnary(t, artifact)

	fifoDispatches, fifoHarness := runRecordedAdhocReplay(t, artifactPath, scheduler.NewFIFOScheduler())
	workInQueueDispatches, workInQueueHarness := runRecordedAdhocReplay(t, artifactPath, scheduler.NewWorkInQueueScheduler(8))

	assertReplayWorkInQueueAdvancesInitializedTracesEarlierThanFIFO(t, fifoDispatches, workInQueueDispatches)
	assertInitializedTraceProgressIsPrioritized(t, workInQueueDispatches)
	assertReplayFinishedWithoutCompletedOrActiveDispatches(t, fifoHarness)
	assertReplayFinishedWithoutCompletedOrActiveDispatches(t, workInQueueHarness)
}

func runRecordedAdhocReplay(t *testing.T, artifactPath string, replayScheduler scheduler.Scheduler) ([]interfaces.FactoryDispatchRecord, *testutil.ReplayHarness) {
	t.Helper()

	var dispatches []interfaces.FactoryDispatchRecord
	h := testutil.AssertReplaySucceeds(t, artifactPath, 30*time.Second,
		testutil.WithReplayHarnessServiceOptions(
			testutil.WithExtraOptions(
				factory.WithScheduler(replayScheduler),
				factory.WithDispatchRecorder(func(record interfaces.FactoryDispatchRecord) {
					dispatches = append(dispatches, record)
				}),
			),
		),
	)

	return dispatches, h
}

func assertRecordedAdhocReplayIsUnary(t *testing.T, artifact *interfaces.ReplayArtifact) {
	t.Helper()

	if replayEventCount(artifact, factoryapi.FactoryEventTypeWorkRequest) == 0 {
		t.Fatal("expected adhoc replay artifact to contain submissions")
	}
	if replayEventCount(artifact, factoryapi.FactoryEventTypeDispatchRequest) == 0 {
		t.Fatal("expected adhoc replay artifact to contain dispatches")
	}
	if got := submissionsObservedAtTick(artifact.Events, 1); got < 8 {
		t.Fatalf("adhoc replay submissions observed at tick 1 = %d, want at least 8", got)
	}
	if maxRecorded := maxRecordedDispatchesPerTick(artifact.Events); maxRecorded != 1 {
		t.Fatalf("recorded adhoc replay max dispatches per tick = %d, want unary recording", maxRecorded)
	}
}

func submissionsObservedAtTick(events []factoryapi.FactoryEvent, tick int) int {
	count := 0
	for _, event := range events {
		if event.Type == factoryapi.FactoryEventTypeWorkRequest && event.Context.Tick == tick {
			count++
		}
	}
	return count
}

func maxRecordedDispatchesPerTick(events []factoryapi.FactoryEvent) int {
	byTick := make(map[int]int)
	for _, event := range events {
		if event.Type == factoryapi.FactoryEventTypeDispatchRequest {
			byTick[event.Context.Tick]++
		}
	}
	maxCount := 0
	for _, count := range byTick {
		if count > maxCount {
			maxCount = count
		}
	}
	return maxCount
}

func assertReplayWorkInQueueAdvancesInitializedTracesEarlierThanFIFO(t *testing.T, fifoDispatches, workInQueueDispatches []interfaces.FactoryDispatchRecord) {
	t.Helper()

	fifoFirstReviewTick := firstTransitionTick(fifoDispatches, "review")
	workInQueueFirstReviewTick := firstTransitionTick(workInQueueDispatches, "review")
	if fifoFirstReviewTick < 0 || workInQueueFirstReviewTick < 0 {
		t.Fatalf("expected both replays to dispatch review transitions; fifo=%v work-in-queue=%v",
			dispatchTransitionIDs(fifoDispatches), dispatchTransitionIDs(workInQueueDispatches))
	}
	if workInQueueFirstReviewTick >= fifoFirstReviewTick {
		t.Fatalf("work-in-queue first review tick = %d, want earlier than FIFO first review tick = %d",
			workInQueueFirstReviewTick, fifoFirstReviewTick)
	}
}

func firstTransitionTick(dispatches []interfaces.FactoryDispatchRecord, transitionID string) int {
	for _, dispatch := range dispatches {
		if dispatch.Dispatch.TransitionID == transitionID {
			return dispatchCreatedTick(dispatch)
		}
	}
	return -1
}

func assertInitializedTraceProgressIsPrioritized(t *testing.T, dispatches []interfaces.FactoryDispatchRecord) {
	t.Helper()

	for i, dispatch := range dispatches {
		if dispatch.Dispatch.TransitionID != "review" {
			continue
		}
		if !hasEarlierDispatchForTrace(dispatches[:i], traceIDFromDispatch(dispatch)) {
			t.Fatalf("review dispatch %s was not preceded by initialized trace history", dispatchID(dispatch))
		}
		if earlierUninitialized := earlierUninitializedDispatchAtSameTick(dispatches, i); earlierUninitialized != "" {
			t.Fatalf("review dispatch for initialized trace was ordered after uninitialized dispatch %s in the same tick", earlierUninitialized)
		}
		return
	}
	t.Fatal("expected replay to dispatch at least one review transition for initialized trace progress")
}

func earlierUninitializedDispatchAtSameTick(dispatches []interfaces.FactoryDispatchRecord, reviewIndex int) string {
	review := dispatches[reviewIndex]
	for i := 0; i < reviewIndex; i++ {
		candidate := dispatches[i]
		if dispatchCreatedTick(candidate) != dispatchCreatedTick(review) {
			continue
		}
		if hasEarlierDispatchForTrace(dispatches[:i], traceIDFromDispatch(candidate)) {
			continue
		}
		return dispatchID(candidate)
	}
	return ""
}

func hasEarlierDispatchForTrace(dispatches []interfaces.FactoryDispatchRecord, traceID string) bool {
	if traceID == "" {
		return false
	}
	for _, dispatch := range dispatches {
		if traceIDFromDispatch(dispatch) == traceID {
			return true
		}
	}
	return false
}

func traceIDFromDispatch(dispatch interfaces.FactoryDispatchRecord) string {
	for _, token := range workers.WorkDispatchInputTokens(dispatch.Dispatch) {
		if token.Color.DataType != interfaces.DataTypeResource && token.Color.TraceID != "" {
			return token.Color.TraceID
		}
	}
	return ""
}

func dispatchTransitionIDs(dispatches []interfaces.FactoryDispatchRecord) []string {
	ids := make([]string, len(dispatches))
	for i := range dispatches {
		ids[i] = dispatches[i].Dispatch.TransitionID
	}
	return ids
}

func dispatchCreatedTick(dispatch interfaces.FactoryDispatchRecord) int {
	return dispatch.Dispatch.Execution.DispatchCreatedTick
}

func dispatchID(dispatch interfaces.FactoryDispatchRecord) string {
	return dispatch.Dispatch.DispatchID
}

func assertReplayFinishedWithoutCompletedOrActiveDispatches(t *testing.T, h *testutil.ReplayHarness) {
	t.Helper()

	snapshot, err := h.Service.GetEngineStateSnapshot()
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot after work-in-queue replay: %v", err)
	}
	if snapshot.RuntimeStatus != interfaces.RuntimeStatusFinished {
		t.Fatalf("work-in-queue replay runtime status = %q, want %q", snapshot.RuntimeStatus, interfaces.RuntimeStatusFinished)
	}
	if len(snapshot.Dispatches) != 0 || snapshot.InFlightCount != 0 {
		t.Fatalf("work-in-queue replay left active dispatch state: dispatches=%d in_flight=%d", len(snapshot.Dispatches), snapshot.InFlightCount)
	}
	if stuck := nonTerminalWorkItemsInSnapshot(snapshot); len(stuck) > 0 {
		t.Fatalf("work-in-queue replay left non-terminal work item(s): %v", stuck)
	}
}

func nonTerminalWorkItemsInSnapshot(snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) []string {
	if snapshot == nil || snapshot.Topology == nil {
		return nil
	}

	var items []string
	for _, token := range snapshot.Marking.Tokens {
		if token == nil || token.Color.DataType != interfaces.DataTypeWork || token.Color.WorkID == "" {
			continue
		}
		category := snapshot.Topology.StateCategoryForPlace(token.PlaceID)
		if category == state.StateCategoryTerminal || category == state.StateCategoryFailed {
			continue
		}
		items = append(items, token.Color.WorkID+"@"+token.PlaceID)
	}
	sort.Strings(items)
	return items
}
