package runtime_api

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/factory/scheduler"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
	"github.com/portpowered/infinite-you/pkg/testutil"
	"github.com/portpowered/infinite-you/pkg/workers"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestWorkInQueueScheduler_BatchSubmissionPrioritizesWorkInProgressState(t *testing.T) {
	support.SkipLongFunctional(t, "slow work-in-queue scheduler state-priority sweep")
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
	support.SkipLongFunctional(t, "slow work-in-queue scheduler ordering sweep")
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
	support.SkipLongFunctional(t, "slow work-in-queue scheduler runtime-config sweep")
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

func workInProgressPriorityFixture(t *testing.T) string {
	t.Helper()

	dir := support.ScaffoldFactory(t, map[string]any{})
	factoryJSON := `{
  "name": "work-in-progress-priority-factory",
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
      "behavior": "STANDARD",
      "worker": "processor",
      "inputs": [{ "workType": "task", "state": "init" }],
      "outputs": [{ "workType": "task", "state": "processing" }],
      "onFailure": { "workType": "task", "state": "failed" }
    },
    {
      "name": "zzz-finish",
      "behavior": "STANDARD",
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
	support.WriteAgentConfig(t, dir, "processor", priorityFixtureAgentConfig("Process the task."))
	support.WriteAgentConfig(t, dir, "finisher", priorityFixtureAgentConfig("Finish the task."))
	return dir
}

func schedulerPrioritySmokeFixture(t *testing.T) string {
	t.Helper()

	dir := support.ScaffoldFactory(t, schedulerPrioritySmokeConfig())
	support.WriteAgentConfig(t, dir, "finisher", priorityFixtureAgentConfig("Finish processing work."))
	support.WriteAgentConfig(t, dir, "starter", priorityFixtureAgentConfig("Complete initial work."))
	support.WriteAgentConfig(t, dir, "cron-worker", priorityFixtureAgentConfig("Complete cron-gated work."))
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
				"behavior":  "STANDARD",
				"worker":    "starter",
				"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
				"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
				"onFailure": map[string]string{"workType": "task", "state": "failed"},
			},
			{
				"name":      "bbb-finish-processing",
				"behavior":  "STANDARD",
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

func dispatchCreatedTick(dispatch interfaces.FactoryDispatchRecord) int {
	return dispatch.Dispatch.Execution.DispatchCreatedTick
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
