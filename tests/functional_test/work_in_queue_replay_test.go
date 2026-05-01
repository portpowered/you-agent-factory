package functional_test

import (
	"context"
	"os"
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

func TestWorkInQueueScheduler_BatchSubmissionPrioritizesWorkInProgressState(t *testing.T) {
	dir := workInProgressPriorityFixture(t)

	var dispatches []interfaces.FactoryDispatchRecord
	provider := testutil.NewMockWorkerMapProvider(map[string][]interfaces.InferenceResponse{
		"processor": {{Content: "processing COMPLETE"}},
		"finisher":  {{Content: "finish existing COMPLETE"}, {Content: "finish initial COMPLETE"}},
	})
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
		testutil.WithExtraOptions(
			factory.WithScheduler(scheduler.NewWorkInQueueScheduler(2)),
			factory.WithDispatchRecorder(func(record interfaces.FactoryDispatchRecord) {
				dispatches = append(dispatches, record)
			}),
		),
	)

	h.SubmitFull(context.Background(), []interfaces.SubmitRequest{
		{
			RequestID:  "request-state-priority",
			WorkID:     "work-initial",
			Name:       "initial",
			WorkTypeID: "task",
			TraceID:    "trace-state-priority-initial",
			Payload:    []byte("initial"),
		},
		{
			RequestID:   "request-state-priority",
			WorkID:      "work-processing",
			Name:        "processing",
			WorkTypeID:  "task",
			TargetState: "processing",
			TraceID:     "trace-state-priority-processing",
			Payload:     []byte("processing"),
		},
	})
	h.RunUntilComplete(t, 10*time.Second)

	assertWorkInProgressStateProgressIsPrioritized(t, h, dispatches)
}

func TestWorkInQueueScheduler_RuntimeSmokeOrdersProcessingThenInitialThenCron(t *testing.T) {
	dir := schedulerPrioritySmokeFixture(t)
	dueAt := time.Now().UTC().Add(-time.Second)
	expiresAt := dueAt.Add(time.Hour)

	var dispatches []interfaces.FactoryDispatchRecord
	provider := testutil.NewMockWorkerMapProvider(map[string][]interfaces.InferenceResponse{
		"finisher":    {{Content: "finish processing COMPLETE"}},
		"starter":     {{Content: "start initial COMPLETE"}},
		"cron-worker": {{Content: "cron initial COMPLETE"}},
	})
	recordingScheduler := &recordingWorkInQueueScheduler{
		inner: scheduler.NewWorkInQueueScheduler(2),
	}
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
		testutil.WithExtraOptions(
			factory.WithScheduler(recordingScheduler),
			factory.WithDispatchRecorder(func(record interfaces.FactoryDispatchRecord) {
				dispatches = append(dispatches, record)
			}),
		),
	)

	h.SubmitFull(context.Background(), []interfaces.SubmitRequest{
		schedulerPriorityWorkRequest("request-runtime-priority", "work-processing", "processing", "task", "processing"),
		schedulerPriorityWorkRequest("request-runtime-priority", "work-initial", "initial", "task", ""),
		schedulerPriorityWorkRequest("request-runtime-priority", "work-cron-input", "cron-input", "scheduled", ""),
		schedulerPriorityCronTimeRequest("time-cron-priority", "zzz-cron-initial", dueAt, expiresAt),
	})
	h.RunUntilComplete(t, 10*time.Second)

	assertRuntimePrioritySmokeFirstBatchWasConstrained(t, recordingScheduler)
	assertRuntimePrioritySmokeOrder(t, h, dispatches)
}

func TestWorkInQueueScheduler_RuntimeSmokeCustomSchedulerReceivesRuntimeConfig(t *testing.T) {
	dir := schedulerPrioritySmokeFixture(t)
	dueAt := time.Now().UTC().Add(-time.Second)
	expiresAt := dueAt.Add(time.Hour)

	var dispatches []interfaces.FactoryDispatchRecord
	provider := testutil.NewMockWorkerMapProvider(map[string][]interfaces.InferenceResponse{
		"finisher":    {{Content: "finish processing COMPLETE"}},
		"starter":     {{Content: "start initial COMPLETE"}},
		"cron-worker": {{Content: "cron initial COMPLETE"}},
	})
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
		testutil.WithExtraOptions(
			factory.WithScheduler(scheduler.NewWorkInQueueScheduler(2)),
			factory.WithDispatchRecorder(func(record interfaces.FactoryDispatchRecord) {
				dispatches = append(dispatches, record)
			}),
		),
	)

	h.SubmitFull(context.Background(), []interfaces.SubmitRequest{
		schedulerPriorityWorkRequest("request-runtime-priority-custom", "work-processing", "processing", "task", "processing"),
		schedulerPriorityWorkRequest("request-runtime-priority-custom", "work-initial", "initial", "task", ""),
		schedulerPriorityWorkRequest("request-runtime-priority-custom", "work-cron-input", "cron-input", "scheduled", ""),
		schedulerPriorityCronTimeRequest("time-cron-priority-custom", "zzz-cron-initial", dueAt, expiresAt),
	})
	h.RunUntilComplete(t, 10*time.Second)

	assertRuntimePrioritySmokeOrder(t, h, dispatches)
}

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

func workInProgressPriorityFixture(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	factoryJSON := `{
  "name": "work-in-progress-priority-fixture",
  "workTypes": [
    {
      "name": "task",
      "states": [
        { "name": "init", "type": "INITIAL" },
        { "name": "processing", "type": "PROCESSING" },
        { "name": "complete", "type": "TERMINAL" },
        { "name": "failed", "type": "FAILED" }
      ]
    }
  ],
  "workers": [
    { "name": "processor" },
    { "name": "finisher" }
  ],
  "workstations": [
    {
      "name": "aaa-process",
      "worker": "processor",
      "inputs": [{ "workType": "task", "state": "init" }],
      "outputs": [{ "workType": "task", "state": "processing" }],
      "onFailure": { "workType": "task", "state": "failed" }
    },
    {
      "name": "zzz-finish",
      "worker": "finisher",
      "inputs": [{ "workType": "task", "state": "processing" }],
      "outputs": [{ "workType": "task", "state": "complete" }],
      "onFailure": { "workType": "task", "state": "failed" }
    }
  ]
}`
	if err := os.WriteFile(filepath.Join(dir, "factory.json"), []byte(factoryJSON), 0o644); err != nil {
		t.Fatalf("write priority fixture factory.json: %v", err)
	}
	writeAgentConfig(t, dir, "processor", priorityFixtureAgentConfig("Process the task."))
	writeAgentConfig(t, dir, "finisher", priorityFixtureAgentConfig("Finish the task."))
	return dir
}

func schedulerPrioritySmokeFixture(t *testing.T) string {
	t.Helper()

	dir := scaffoldFactory(t, schedulerPrioritySmokeConfig())
	writeAgentConfig(t, dir, "finisher", priorityFixtureAgentConfig("Finish processing work."))
	writeAgentConfig(t, dir, "starter", priorityFixtureAgentConfig("Complete initial work."))
	writeAgentConfig(t, dir, "cron-worker", priorityFixtureAgentConfig("Complete cron-gated work."))
	return dir
}

func schedulerPrioritySmokeConfig() map[string]any {
	return map[string]any{
		"workTypes": []map[string]any{
			{"name": "task", "states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "processing", "type": "PROCESSING"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			}},
			{"name": "scheduled", "states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			}},
		},
		"workers": []map[string]string{
			{"name": "finisher"},
			{"name": "starter"},
			{"name": "cron-worker"},
		},
		"workstations": []map[string]any{
			{
				"name":      "aaa-start-initial",
				"worker":    "starter",
				"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
				"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
				"onFailure": map[string]string{"workType": "task", "state": "failed"},
			},
			{
				"name":      "bbb-finish-processing",
				"worker":    "finisher",
				"inputs":    []map[string]string{{"workType": "task", "state": "processing"}},
				"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
				"onFailure": map[string]string{"workType": "task", "state": "failed"},
			},
			{
				"name":      "zzz-cron-initial",
				"behavior":  "CRON",
				"worker":    "cron-worker",
				"cron":      map[string]string{"schedule": "0 * * * *", "expiryWindow": "2h"},
				"inputs":    []map[string]string{{"workType": "scheduled", "state": "init"}},
				"outputs":   []map[string]string{{"workType": "scheduled", "state": "complete"}},
				"onFailure": map[string]string{"workType": "scheduled", "state": "failed"},
			},
		},
	}
}

func priorityFixtureAgentConfig(prompt string) string {
	return `---
type: MODEL_WORKER
model: test-model
modelProvider: claude
stopToken: COMPLETE
---
` + prompt + `
`
}

func schedulerPriorityWorkRequest(requestID, workID, name, workTypeID, targetState string) interfaces.SubmitRequest {
	return interfaces.SubmitRequest{
		RequestID:   requestID,
		WorkID:      workID,
		Name:        name,
		WorkTypeID:  workTypeID,
		TargetState: targetState,
		TraceID:     "trace-" + workID,
		Payload:     []byte(name),
	}
}

func schedulerPriorityCronTimeRequest(workID, workstation string, dueAt, expiresAt time.Time) interfaces.SubmitRequest {
	return interfaces.SubmitRequest{
		RequestID:   "request-runtime-priority",
		WorkID:      workID,
		Name:        "cron:" + workstation,
		WorkTypeID:  interfaces.SystemTimeWorkTypeID,
		TargetState: interfaces.SystemTimePendingState,
		TraceID:     "trace-" + workID,
		Payload:     []byte(`{"source":"cron"}`),
		Tags: map[string]string{
			interfaces.TimeWorkTagKeySource:          interfaces.TimeWorkSourceCron,
			interfaces.TimeWorkTagKeyCronWorkstation: workstation,
			interfaces.TimeWorkTagKeyNominalAt:       dueAt.Format(time.RFC3339Nano),
			interfaces.TimeWorkTagKeyDueAt:           dueAt.Format(time.RFC3339Nano),
			interfaces.TimeWorkTagKeyExpiresAt:       expiresAt.Format(time.RFC3339Nano),
			interfaces.TimeWorkTagKeyJitter:          "0s",
		},
	}
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

func assertWorkInProgressStateProgressIsPrioritized(t *testing.T, h *testutil.ServiceTestHarness, dispatches []interfaces.FactoryDispatchRecord) {
	t.Helper()

	snapshot, err := h.GetEngineStateSnapshot()
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot for work-in-progress priority assertion: %v", err)
	}
	if snapshot.Topology == nil {
		t.Fatal("expected replay snapshot topology for work-in-progress priority assertion")
	}

	seenInitialByTick := make(map[int]string)
	initialTicks := make(map[int]bool)
	processingTicks := make(map[int]bool)
	for _, dispatch := range dispatches {
		category, ok := dispatchWorkStateCategory(dispatch, snapshot.Topology)
		if !ok {
			continue
		}
		tick := dispatch.Dispatch.Execution.DispatchCreatedTick
		switch category {
		case state.StateCategoryInitial:
			initialTicks[tick] = true
			if seenInitialByTick[tick] == "" {
				seenInitialByTick[tick] = dispatch.Dispatch.DispatchID
			}
		case state.StateCategoryProcessing:
			processingTicks[tick] = true
			if earlierInitial := seenInitialByTick[tick]; earlierInitial != "" {
				t.Fatalf("processing-state dispatch %s was ordered after initial-state dispatch %s in tick %d",
					dispatch.Dispatch.DispatchID, earlierInitial, tick)
			}
		}
	}

	for tick := range processingTicks {
		if initialTicks[tick] {
			return
		}
	}
	t.Fatal("expected run to include a mixed tick with processing-state and initial-state dispatches")
}

func assertRuntimePrioritySmokeOrder(t *testing.T, h *testutil.ServiceTestHarness, dispatches []interfaces.FactoryDispatchRecord) {
	t.Helper()

	processingIndex := dispatchIndexByTransition(dispatches, "bbb-finish-processing")
	initialIndex := dispatchIndexByTransition(dispatches, "aaa-start-initial")
	cronIndex := dispatchIndexByTransition(dispatches, "zzz-cron-initial")
	if processingIndex < 0 || initialIndex < 0 || cronIndex < 0 {
		t.Fatalf("priority smoke dispatches missing expected transitions: %v", dispatchTransitionIDs(dispatches))
	}

	processingTick := dispatchCreatedTick(dispatches[processingIndex])
	initialTick := dispatchCreatedTick(dispatches[initialIndex])
	cronTick := dispatchCreatedTick(dispatches[cronIndex])
	if processingTick != initialTick {
		t.Fatalf("processing and initial dispatches ran in different ticks: processing=%d initial=%d", processingTick, initialTick)
	}
	if processingIndex > initialIndex {
		t.Fatalf("processing dispatch was ordered after initial dispatch in constrained tick: %v", dispatchTransitionIDs(dispatches))
	}
	if got := countDispatchesCreatedAtTick(dispatches, initialTick); got != 2 {
		t.Fatalf("constrained priority tick dispatched %d transitions, want 2; order=%v", got, dispatchTransitionIDs(dispatches))
	}
	if cronTick <= initialTick {
		t.Fatalf("cron dispatch tick = %d, want after standard initial tick %d; order=%v", cronTick, initialTick, dispatchTransitionIDs(dispatches))
	}

	snapshot, err := h.GetEngineStateSnapshot()
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot for runtime priority smoke: %v", err)
	}
	if stuck := nonTerminalWorkItemsInSnapshot(snapshot); len(stuck) > 0 {
		t.Fatalf("runtime priority smoke left non-terminal work item(s): %v", stuck)
	}
}

func assertRuntimePrioritySmokeFirstBatchWasConstrained(t *testing.T, recordingScheduler *recordingWorkInQueueScheduler) {
	t.Helper()

	if recordingScheduler.firstEnabled != 3 || recordingScheduler.firstDecisions != 2 {
		t.Fatalf("first priority smoke batch enabled=%d decisions=%d bindings=%v, want 3 enabled and 2 decisions",
			recordingScheduler.firstEnabled, recordingScheduler.firstDecisions, recordingScheduler.firstBindings)
	}
	if !strings.Contains(strings.Join(recordingScheduler.firstBindings, ","), interfaces.SystemTimePendingPlaceID+":to:zzz-cron-initial") {
		t.Fatalf("first priority smoke batch did not include cron time binding: %v", recordingScheduler.firstBindings)
	}
}

func dispatchWorkStateCategory(dispatch interfaces.FactoryDispatchRecord, topology *state.Net) (state.StateCategory, bool) {
	category := state.StateCategoryProcessing
	found := false
	for _, token := range workers.WorkDispatchInputTokens(dispatch.Dispatch) {
		if token.Color.DataType == interfaces.DataTypeResource {
			continue
		}
		tokenCategory := topology.StateCategoryForPlace(token.PlaceID)
		if tokenCategory == state.StateCategoryProcessing {
			return tokenCategory, true
		}
		if !found {
			category = tokenCategory
			found = true
		}
	}
	return category, found
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

func dispatchIndexByTransition(dispatches []interfaces.FactoryDispatchRecord, transitionID string) int {
	for i, dispatch := range dispatches {
		if dispatch.Dispatch.TransitionID == transitionID {
			return i
		}
	}
	return -1
}

func dispatchTransitionIDs(dispatches []interfaces.FactoryDispatchRecord) []string {
	ids := make([]string, len(dispatches))
	for i := range dispatches {
		ids[i] = dispatches[i].Dispatch.TransitionID
	}
	return ids
}

func countDispatchesCreatedAtTick(dispatches []interfaces.FactoryDispatchRecord, tick int) int {
	count := 0
	for _, dispatch := range dispatches {
		if dispatchCreatedTick(dispatch) == tick {
			count++
		}
	}
	return count
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
