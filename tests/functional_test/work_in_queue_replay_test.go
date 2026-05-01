package functional_test

import (
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/agent-factory/pkg/api/generated"
	"github.com/portpowered/agent-factory/pkg/factory"
	"github.com/portpowered/agent-factory/pkg/factory/scheduler"
	"github.com/portpowered/agent-factory/pkg/factory/state"
	"github.com/portpowered/agent-factory/pkg/interfaces"
	"github.com/portpowered/agent-factory/pkg/petri"
	"github.com/portpowered/agent-factory/pkg/testutil"
	"github.com/portpowered/agent-factory/pkg/workers"
)

const adhocReplayMaxDispatchesPerTick = 8

func TestWorkInQueueScheduler_AdhocBatchReplayDispatchesMultipleItemsAndPrioritizesInitializedTraces(t *testing.T) {
	skipSlowFunctionalSmokeInShort(t, "slow batch replay scheduler smoke")
	artifactPath := adhocBatchReplayArtifactPath(t)
	artifact := testutil.LoadReplayArtifact(t, artifactPath)
	assertRecordedAdhocReplayWasUnary(t, artifact)

	var fifoDispatches []interfaces.FactoryDispatchRecord
	fifoHarness := testutil.AssertReplaySucceeds(t, artifactPath, 30*time.Second,
		testutil.WithReplayHarnessServiceOptions(
			testutil.WithExtraOptions(
				factory.WithScheduler(scheduler.NewFIFOScheduler()),
				factory.WithDispatchRecorder(func(record interfaces.FactoryDispatchRecord) {
					fifoDispatches = append(fifoDispatches, record)
				}),
			),
		),
	)
	assertReplayFinishedWithoutCompletedOrActiveDispatches(t, fifoHarness)

	var workInQueueDispatches []interfaces.FactoryDispatchRecord
	recordingScheduler := &recordingWorkInQueueScheduler{
		inner: scheduler.NewWorkInQueueScheduler(adhocReplayMaxDispatchesPerTick),
	}
	workInQueueHarness := testutil.AssertReplaySucceeds(t, artifactPath, 30*time.Second,
		testutil.WithReplayHarnessServiceOptions(
			testutil.WithExtraOptions(
				factory.WithScheduler(recordingScheduler),
				factory.WithDispatchRecorder(func(record interfaces.FactoryDispatchRecord) {
					workInQueueDispatches = append(workInQueueDispatches, record)
				}),
			),
		),
	)

	workInQueueFirstTickDispatches := dispatchesCreatedAtTick(workInQueueDispatches, 1)
	if got := len(workInQueueFirstTickDispatches); got != adhocReplayMaxDispatchesPerTick {
		t.Fatalf("work-in-queue first replay tick dispatch count = %d, want %d; first enabled=%d first decisions=%d first bindings=%v",
			got, adhocReplayMaxDispatchesPerTick, recordingScheduler.firstEnabled, recordingScheduler.firstDecisions, recordingScheduler.firstBindings)
	}
	if maxRecorded := maxRecordedDispatchesPerTick(artifact.Events); maxRecorded != 1 {
		t.Fatalf("recorded adhoc replay max dispatches per tick = %d, want unary recording", maxRecorded)
	}

	assertWorkInQueueImprovesThroughputOverFIFO(t, fifoDispatches, workInQueueDispatches)
	assertInitializedTraceProgressIsPrioritized(t, workInQueueDispatches)
	assertReplayFinishedWithoutCompletedOrActiveDispatches(t, workInQueueHarness)
}

type recordingWorkInQueueScheduler struct {
	inner          scheduler.Scheduler
	firstEnabled   int
	firstDecisions int
	firstBindings  []string
}

func (s *recordingWorkInQueueScheduler) Select(enabled []interfaces.EnabledTransition, snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) []interfaces.FiringDecision {
	decisions := s.inner.Select(enabled, snapshot)
	if s.firstEnabled == 0 && len(enabled) > 0 {
		s.firstEnabled = len(enabled)
		s.firstDecisions = len(decisions)
		for _, candidate := range enabled {
			for name, tokens := range candidate.Bindings {
				s.firstBindings = append(s.firstBindings, name+"="+tokenIDList(tokens))
			}
		}
	}
	return decisions
}

func (s *recordingWorkInQueueScheduler) SupportsRepeatedTransitionBindings() bool {
	if s == nil {
		return false
	}
	return scheduler.SupportsRepeatedTransitionBindings(s.inner)
}

func (s *recordingWorkInQueueScheduler) SetRuntimeConfig(runtimeConfig interfaces.RuntimeWorkstationLookup) {
	if s == nil {
		return
	}
	scheduler.ApplyRuntimeConfig(s.inner, runtimeConfig)
}

func tokenIDList(tokens []interfaces.Token) string {
	ids := make([]string, len(tokens))
	for i := range tokens {
		ids[i] = tokens[i].ID + ":" + string(tokens[i].Color.DataType)
	}
	sort.Strings(ids)
	return strings.Join(ids, "|")
}

func adhocBatchReplayArtifactPath(t *testing.T) string {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot determine work-in-queue replay test path")
	}
	return filepath.Join(filepath.Dir(thisFile), "testdata", "adhoc-recording-batch-event-log.json")
}

func assertRecordedAdhocReplayWasUnary(t *testing.T, artifact *interfaces.ReplayArtifact) {
	t.Helper()

	if replayEventCount(artifact, factoryapi.FactoryEventTypeWorkRequest) == 0 {
		t.Fatal("expected adhoc replay artifact to contain submissions")
	}
	if replayEventCount(artifact, factoryapi.FactoryEventTypeDispatchRequest) == 0 {
		t.Fatal("expected adhoc replay artifact to contain dispatches")
	}
	if got := submissionsObservedAtTick(artifact.Events, 1); got < adhocReplayMaxDispatchesPerTick {
		t.Fatalf("adhoc replay submissions observed at tick 1 = %d, want at least %d", got, adhocReplayMaxDispatchesPerTick)
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

func dispatchesCreatedAtTick(dispatches []interfaces.FactoryDispatchRecord, tick int) []interfaces.FactoryDispatchRecord {
	var records []interfaces.FactoryDispatchRecord
	for _, dispatch := range dispatches {
		if dispatchCreatedTick(dispatch) == tick {
			records = append(records, dispatch)
		}
	}
	return records
}

func assertWorkInQueueImprovesThroughputOverFIFO(t *testing.T, fifoDispatches, workInQueueDispatches []interfaces.FactoryDispatchRecord) {
	t.Helper()

	fifoFirstTickDispatches := dispatchesCreatedAtTick(fifoDispatches, 1)
	workInQueueFirstTickDispatches := dispatchesCreatedAtTick(workInQueueDispatches, 1)
	if len(fifoFirstTickDispatches) == 0 {
		t.Fatal("FIFO replay produced no first-tick dispatches")
	}
	if maxFIFO := maxFactoryDispatchesPerTick(fifoDispatches); maxFIFO != 1 {
		t.Fatalf("FIFO replay max dispatches per tick = %d, want current unary fallback behavior", maxFIFO)
	}
	if len(workInQueueFirstTickDispatches) <= len(fifoFirstTickDispatches) {
		t.Fatalf("work-in-queue first-tick dispatches = %d, want more than FIFO first-tick dispatches = %d",
			len(workInQueueFirstTickDispatches), len(fifoFirstTickDispatches))
	}
}

func maxFactoryDispatchesPerTick(dispatches []interfaces.FactoryDispatchRecord) int {
	byTick := make(map[int]int)
	for _, dispatch := range dispatches {
		byTick[dispatchCreatedTick(dispatch)]++
	}
	maxCount := 0
	for _, count := range byTick {
		if count > maxCount {
			maxCount = count
		}
	}
	return maxCount
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
