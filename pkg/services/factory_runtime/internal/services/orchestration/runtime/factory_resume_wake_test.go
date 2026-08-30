package runtime

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil/recordingfixtures"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	factory_context "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/context"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
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

func TestNew_RestoresOtherWorkWhenCompletedCronWorkHasNoOccupancy(t *testing.T) {
	base := time.Date(2026, time.August, 26, 12, 0, 0, 0, time.UTC)
	cronWork := work.FactoryWorkItem{
		ID: "automation-completed-without-time-prefix", WorkTypeID: interfaces.SystemTimeWorkTypeID,
		State: interfaces.SystemTimePendingState, TraceID: "trace-cron-recovery",
		Tags: map[string]string{
			interfaces.TimeWorkTagKeySource:          interfaces.TimeWorkSourceCron,
			interfaces.TimeWorkTagKeyCronWorkstation: "cron-refresh",
		},
	}
	recoverableWork := work.FactoryWorkItem{ID: "recoverable-work", WorkTypeID: "task", State: "init", TraceID: "trace-recoverable"}
	restored := &interfaces.FactoryWorldState{
		WorkItemsByID:       map[string]work.FactoryWorkItem{cronWork.ID: cronWork, recoverableWork.ID: recoverableWork},
		ActiveWorkItemsByID: map[string]work.FactoryWorkItem{cronWork.ID: cronWork, recoverableWork.ID: recoverableWork},
		WorkRequestsByID: map[string]interfaces.WorkRequestPayload{
			"request-recovery": {RequestID: "request-recovery", WorkItems: []work.FactoryWorkItem{cronWork, recoverableWork}},
		},
		// Legacy recordings can omit an empty occupancy projection during JSON serialization.
		PlaceOccupancyByID: nil,
		CompletedDispatches: []interfaces.FactoryWorldDispatchCompletion{{
			DispatchID: "dispatch-cron-recovery", TransitionID: "cron-refresh", WorkItemIDs: []string{cronWork.ID},
			ConsumedInputs: []interfaces.WorkstationInput{{TokenID: cronWork.ID, PlaceID: interfaces.SystemTimePendingPlaceID, WorkItem: &cronWork}},
			InputWorkItems: []work.FactoryWorkItem{cronWork}, Result: interfaces.WorkstationResult{Outcome: string(workerexecution.OutcomeAccepted)},
		}},
	}
	ledger := &recordingfixtures.ScriptedRuntimeLedger{}
	logger := &recordingLogger{}
	f, err := newTestFactory(
		withNet(buildCronRestoreNet()), withClock(platformclock.NewDeterministic(base, time.Second)),
		withWorkflowContext(&factory_context.FactoryContext{SessionID: "session-cron-recovery"}),
		withFactoryEventHistory(ledger), withLogger(logger), withRestoredWorldState(restored),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	snapshot, err := f.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	if !markingContainsWorkAtPlace(&snapshot.Marking, recoverableWork.ID, "task:init") {
		t.Fatalf("recoverable Work marking = %#v, want task:init", snapshot.Marking.PlaceTokens)
	}
	if markingContainsWorkAtPlace(&snapshot.Marking, cronWork.ID, interfaces.SystemTimePendingPlaceID) {
		t.Fatalf("completed cron Work marking = %#v, want no pending time token", snapshot.Marking.PlaceTokens)
	}
	if count := len(restoreWarnings(logger)); count != 1 {
		t.Fatalf("restore warnings = %d, want one structured legacy recovery warning", count)
	}
	warning := restoreWarnings(logger)[0]
	if warning.fields["session_id"] != "session-cron-recovery" || warning.fields["recording_id"] != "runtime-test-recording-id" ||
		warning.fields["work_id"] != cronWork.ID || warning.fields["disposition"] != restoredLegacyAutomationDisposition {
		t.Fatalf("restore warning fields = %#v, want safe session/recording/work/disposition context", warning.fields)
	}
	tickable := tickableFactory(t, f)
	if err := tickable.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	for _, event := range ledger.CanonicalEvents() {
		if event.Type != interfaces.FactoryEventTypeDispatchRequest {
			continue
		}
		var payload interfaces.DispatchRequestEventPayload
		if err := event.DecodePayload(&payload); err != nil {
			t.Fatalf("decode dispatch request: %v", err)
		}
		if payload.TransitionID == "cron-refresh" {
			t.Fatalf("completed cron Work was dispatched again: %#v", payload)
		}
	}
}

func TestNew_RestoresCompletedWorkFromCanonicalDispatchOutputWhenOccupancyIsMissing(t *testing.T) {
	completed := work.FactoryWorkItem{ID: "work-completed-from-output", WorkTypeID: "task", State: "done", TraceID: "trace-completed-from-output"}
	restored := &interfaces.FactoryWorldState{
		WorkItemsByID:      map[string]work.FactoryWorkItem{completed.ID: completed},
		TerminalWorkByID:   map[string]interfaces.FactoryTerminalWork{completed.ID: {WorkItem: completed, Status: "TERMINAL"}},
		PlaceOccupancyByID: map[string]interfaces.FactoryPlaceOccupancy{},
		CompletedDispatches: []interfaces.FactoryWorldDispatchCompletion{{
			DispatchID: "dispatch-completed-from-output", TransitionID: "complete-task", WorkItemIDs: []string{completed.ID},
			OutputWorkItems: []work.FactoryWorkItem{completed}, Result: interfaces.WorkstationResult{Outcome: string(workerexecution.OutcomeAccepted)},
		}},
	}
	f, err := newTestFactory(withNet(buildSimpleNet()), withClock(platformclock.NewDeterministic(time.Unix(0, 0).UTC(), time.Second)), withRestoredWorldState(restored))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	snapshot, err := f.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	if !markingContainsWorkAtPlace(&snapshot.Marking, completed.ID, "task:done") {
		t.Fatalf("completed Work marking = %#v, want task:done from completed dispatch output", snapshot.Marking.PlaceTokens)
	}
}

func TestNew_RestoredWorkStateChangeSupersedesCompletedDispatchOutputPlacement(t *testing.T) {
	completed := work.FactoryWorkItem{ID: "work-state-change-current", WorkTypeID: "task", State: "done", TraceID: "trace-state-change-current"}
	intermediate := completed
	intermediate.State = "init"
	restored := &interfaces.FactoryWorldState{
		WorkItemsByID:      map[string]work.FactoryWorkItem{completed.ID: completed},
		TerminalWorkByID:   map[string]interfaces.FactoryTerminalWork{completed.ID: {WorkItem: completed, Status: "TERMINAL"}},
		PlaceOccupancyByID: map[string]interfaces.FactoryPlaceOccupancy{},
		CompletedDispatches: []interfaces.FactoryWorldDispatchCompletion{{
			DispatchID: "dispatch-intermediate-output", TransitionID: "process-task", WorkItemIDs: []string{completed.ID},
			OutputWorkItems: []work.FactoryWorkItem{intermediate}, Result: interfaces.WorkstationResult{Outcome: string(workerexecution.OutcomeAccepted)},
		}},
		WorkStateChangesByWorkID: map[string][]interfaces.FactoryWorldWorkStateChangeRecord{
			completed.ID: {{
				WorkID: completed.ID, WorkTypeName: "task", FromState: "init", ToState: "done",
				FromPlaceID: "task:init", ToPlaceID: "task:done", Source: work.WorkStateChangeSourceAPI,
				Sequence: 2,
			}},
		},
	}
	f, err := newTestFactory(withNet(buildSimpleNet()), withClock(platformclock.NewDeterministic(time.Unix(0, 0).UTC(), time.Second)), withRestoredWorldState(restored))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	snapshot, err := f.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	if !markingContainsWorkAtPlace(&snapshot.Marking, completed.ID, "task:done") {
		t.Fatalf("restored Work marking = %#v, want task:done from the later canonical state change", snapshot.Marking.PlaceTokens)
	}
	if markingContainsWorkAtPlace(&snapshot.Marking, completed.ID, "task:init") {
		t.Fatalf("restored Work retained intermediate dispatch-output place: %#v", snapshot.Marking.PlaceTokens)
	}
}

func TestNew_WithRestoredTerminalWorkSkipsStaleActiveDispatchPlacement(t *testing.T) {
	terminal := work.FactoryWorkItem{ID: "work-terminal", WorkTypeID: "task", State: "done"}
	restored := &interfaces.FactoryWorldState{
		WorkItemsByID: map[string]work.FactoryWorkItem{
			terminal.ID: terminal,
		},
		TerminalWorkByID: map[string]interfaces.FactoryTerminalWork{
			terminal.ID: {WorkItem: terminal, Status: "TERMINAL"},
		},
		ActiveDispatches: map[string]interfaces.FactoryWorldDispatch{
			"dispatch-late": {
				DispatchID:  "dispatch-late",
				WorkItemIDs: []string{terminal.ID},
				Inputs: []interfaces.WorkstationInput{{
					TokenID:  terminal.ID,
					PlaceID:  "task:init",
					WorkItem: &work.FactoryWorkItem{ID: terminal.ID},
				}},
			},
		},
		PlaceOccupancyByID: map[string]interfaces.FactoryPlaceOccupancy{
			"task:done": {PlaceID: "task:done", WorkItemIDs: []string{terminal.ID}, TokenCount: 1},
		},
	}

	f, err := newTestFactory(
		withNet(buildSimpleNet()),
		withClock(platformclock.NewDeterministic(time.Unix(0, 0).UTC(), time.Second)),
		withRestoredWorldState(restored),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	snapshot, err := f.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	if tokens := snapshot.Marking.TokensInPlace("task:init"); len(tokens) != 0 {
		t.Fatalf("stale active-dispatch Work tokens = %#v, want no token at task:init", tokens)
	}
	if tokens := snapshot.Marking.TokensInPlace("task:done"); len(tokens) != 1 {
		t.Fatalf("terminal Work tokens = %#v, want one token at task:done", tokens)
	}
}

func TestNew_DoesNotRedispatchCompletedAutomationWithCanonicalCompletionPlacement(t *testing.T) {
	cronWork := work.FactoryWorkItem{
		ID: "completed-cron-with-recovered-place", WorkTypeID: interfaces.SystemTimeWorkTypeID, State: interfaces.SystemTimePendingState,
		Tags: map[string]string{interfaces.TimeWorkTagKeySource: interfaces.TimeWorkSourceCron, interfaces.TimeWorkTagKeyCronWorkstation: "cron-refresh"},
	}
	terminal := interfaces.FactoryTerminalWork{WorkItem: cronWork, Status: restoredCompletedAutomationStatus}
	restored := &interfaces.FactoryWorldState{
		WorkItemsByID: map[string]work.FactoryWorkItem{
			cronWork.ID: cronWork, "downstream-output": {ID: "downstream-output", WorkTypeID: "task", State: "done"},
		},
		TerminalWorkByID: map[string]interfaces.FactoryTerminalWork{cronWork.ID: terminal}, PlaceOccupancyByID: map[string]interfaces.FactoryPlaceOccupancy{},
		CompletedDispatches: []interfaces.FactoryWorldDispatchCompletion{{
			DispatchID: "dispatch-completed-cron-with-recovered-place", TransitionID: "cron-refresh", WorkItemIDs: []string{cronWork.ID},
			OutputWorkItems: []work.FactoryWorkItem{{ID: "downstream-output", WorkTypeID: "task", State: "done"}}, TerminalWork: &terminal,
			Result: interfaces.WorkstationResult{Outcome: string(workerexecution.OutcomeAccepted)},
		}},
	}
	ledger := &recordingfixtures.ScriptedRuntimeLedger{}
	f, err := newTestFactory(withNet(buildCronRestoreNet()), withClock(platformclock.NewDeterministic(time.Unix(0, 0).UTC(), time.Second)), withFactoryEventHistory(ledger), withRestoredWorldState(restored))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	snapshot, err := f.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	if markingContainsWorkAtPlace(&snapshot.Marking, cronWork.ID, interfaces.SystemTimePendingPlaceID) {
		t.Fatalf("completed cron Work marking = %#v, want no live pending-time token", snapshot.Marking.PlaceTokens)
	}
	tickable := tickableFactory(t, f)
	if err := tickable.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	for _, event := range ledger.CanonicalEvents() {
		if event.Type != interfaces.FactoryEventTypeDispatchRequest {
			continue
		}
		var payload interfaces.DispatchRequestEventPayload
		if err := event.DecodePayload(&payload); err != nil {
			t.Fatalf("decode dispatch request: %v", err)
		}
		if payload.TransitionID == "cron-refresh" {
			t.Fatalf("completed cron Work was dispatched again: %#v", payload)
		}
	}
}

func TestNew_RejectsConflictingCompletedDispatchPlaces(t *testing.T) {
	completed := work.FactoryWorkItem{ID: "work-conflicting-completed-places", WorkTypeID: "task", State: "done"}
	restored := &interfaces.FactoryWorldState{
		WorkItemsByID:      map[string]work.FactoryWorkItem{completed.ID: completed},
		TerminalWorkByID:   map[string]interfaces.FactoryTerminalWork{completed.ID: {WorkItem: completed, Status: "TERMINAL"}},
		PlaceOccupancyByID: map[string]interfaces.FactoryPlaceOccupancy{},
		CompletedDispatches: []interfaces.FactoryWorldDispatchCompletion{{
			DispatchID: "dispatch-conflicting-places", TransitionID: "complete-task", WorkItemIDs: []string{completed.ID},
			OutputWorkItems: []work.FactoryWorkItem{{ID: completed.ID, WorkTypeID: "task", State: "done"}, {ID: completed.ID, WorkTypeID: "task", State: "init"}},
			Result:          interfaces.WorkstationResult{Outcome: string(workerexecution.OutcomeAccepted)},
		}},
	}
	_, err := newTestFactory(withNet(buildSimpleNet()), withRestoredWorldState(restored))
	if err == nil || !strings.Contains(err.Error(), "conflicting completed-dispatch places") {
		t.Fatalf("New error = %v, want fail-closed conflicting completed-dispatch places error", err)
	}
}

func TestNew_RejectsConflictingCurrentWorkPlacements(t *testing.T) {
	workItem := work.FactoryWorkItem{ID: "work-conflicting-current-places", WorkTypeID: "task"}
	restored := &interfaces.FactoryWorldState{
		WorkItemsByID: map[string]work.FactoryWorkItem{workItem.ID: workItem},
		PlaceOccupancyByID: map[string]interfaces.FactoryPlaceOccupancy{
			"task:init": {PlaceID: "task:init", WorkItemIDs: []string{workItem.ID}},
		},
		WorkStateChangesByWorkID: map[string][]interfaces.FactoryWorldWorkStateChangeRecord{
			workItem.ID: {{
				WorkID: workItem.ID, WorkTypeName: "task", ToState: "done",
				FromPlaceID: "task:init", ToPlaceID: "task:done", Source: work.WorkStateChangeSourceAPI,
			}},
		},
	}
	_, err := newTestFactory(withNet(buildSimpleNet()), withRestoredWorldState(restored))
	if err == nil || !strings.Contains(err.Error(), "conflicting current places") {
		t.Fatalf("New error = %v, want fail-closed conflicting-current-places error", err)
	}
}

func TestNew_RestoresFailedDispatchReferencesWithoutDroppingFailedWork(t *testing.T) {
	failed := work.FactoryWorkItem{ID: "work-failed-dispatch", WorkTypeID: "task", State: "failed"}
	restored := &interfaces.FactoryWorldState{
		WorkItemsByID:       map[string]work.FactoryWorkItem{failed.ID: failed},
		FailedWorkItemsByID: map[string]work.FactoryWorkItem{failed.ID: failed},
		PlaceOccupancyByID: map[string]interfaces.FactoryPlaceOccupancy{
			"task:failed": {PlaceID: "task:failed", WorkItemIDs: []string{failed.ID}},
		},
		FailedDispatches: []interfaces.FactoryWorldDispatchCompletion{{
			DispatchID: "dispatch-failed-work", WorkItemIDs: []string{failed.ID},
		}},
	}
	f, err := newTestFactory(withNet(buildSimpleNet()), withRestoredWorldState(restored))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	snapshot, err := f.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	if !markingContainsWorkAtPlace(&snapshot.Marking, failed.ID, "task:failed") {
		t.Fatalf("failed Work marking = %#v, want task:failed", snapshot.Marking.PlaceTokens)
	}
}

func TestNew_RejectsMalformedFailedDispatchReferences(t *testing.T) {
	failed := work.FactoryWorkItem{ID: "work-failed-dispatch-validation", WorkTypeID: "task", State: "failed"}
	tests := []struct {
		name     string
		restored *interfaces.FactoryWorldState
		want     string
	}{
		{
			name: "unknown Work",
			restored: &interfaces.FactoryWorldState{
				FailedDispatches: []interfaces.FactoryWorldDispatchCompletion{{
					DispatchID: "dispatch-unknown-work", WorkItemIDs: []string{"work-not-recorded"},
				}},
			},
			want: `failed dispatch "dispatch-unknown-work" references unknown Work "work-not-recorded"`,
		},
		{
			name: "missing dispatch identity",
			restored: &interfaces.FactoryWorldState{
				WorkItemsByID:       map[string]work.FactoryWorkItem{failed.ID: failed},
				FailedWorkItemsByID: map[string]work.FactoryWorkItem{failed.ID: failed},
				PlaceOccupancyByID: map[string]interfaces.FactoryPlaceOccupancy{
					"task:failed": {PlaceID: "task:failed", WorkItemIDs: []string{failed.ID}},
				},
				FailedDispatches: []interfaces.FactoryWorldDispatchCompletion{{WorkItemIDs: []string{failed.ID}}},
			},
			want: "failed dispatch at index 0 has no dispatch identity",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := newTestFactory(withNet(buildSimpleNet()), withRestoredWorldState(test.restored))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("New error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestNew_RejectsCompletedAutomationWithIncompatibleTopology(t *testing.T) {
	cronWork := work.FactoryWorkItem{
		ID: "cron-incompatible-topology", WorkTypeID: interfaces.SystemTimeWorkTypeID, State: interfaces.SystemTimePendingState,
		Tags: map[string]string{interfaces.TimeWorkTagKeySource: interfaces.TimeWorkSourceCron, interfaces.TimeWorkTagKeyCronWorkstation: "cron-refresh"},
	}
	restored := &interfaces.FactoryWorldState{
		WorkItemsByID:       map[string]work.FactoryWorkItem{cronWork.ID: cronWork},
		TerminalWorkByID:    map[string]interfaces.FactoryTerminalWork{cronWork.ID: {WorkItem: cronWork, Status: restoredCompletedAutomationStatus}},
		PlaceOccupancyByID:  map[string]interfaces.FactoryPlaceOccupancy{},
		CompletedDispatches: []interfaces.FactoryWorldDispatchCompletion{{DispatchID: "dispatch-incompatible-topology", WorkItemIDs: []string{cronWork.ID}, Result: interfaces.WorkstationResult{Outcome: string(workerexecution.OutcomeAccepted)}}},
	}
	_, err := newTestFactory(withNet(buildSimpleNet()), withRestoredWorldState(restored))
	if err == nil || !strings.Contains(err.Error(), "has no current place occupancy") {
		t.Fatalf("New error = %v, want fail-closed incompatible automation topology error", err)
	}
}

func TestNew_RestoredAutomationRecoveryRequiresCanonicalCompletionFacts(t *testing.T) {
	base := time.Date(2026, time.August, 26, 13, 0, 0, 0, time.UTC)
	cronWork := func(id string) work.FactoryWorkItem {
		return work.FactoryWorkItem{
			ID: id, WorkTypeID: interfaces.SystemTimeWorkTypeID, State: interfaces.SystemTimePendingState,
			Tags: map[string]string{interfaces.TimeWorkTagKeySource: interfaces.TimeWorkSourceCron, interfaces.TimeWorkTagKeyCronWorkstation: "cron-refresh"},
		}
	}
	tests := []struct {
		name     string
		restored *interfaces.FactoryWorldState
		net      *state.Net
	}{
		{
			name: "incomplete dispatch history",
			restored: &interfaces.FactoryWorldState{
				WorkItemsByID:       map[string]work.FactoryWorkItem{"cron-work": cronWork("cron-work")},
				ActiveWorkItemsByID: map[string]work.FactoryWorkItem{"cron-work": cronWork("cron-work")}, PlaceOccupancyByID: map[string]interfaces.FactoryPlaceOccupancy{},
			}, net: buildCronRestoreNet(),
		},
		{
			name: "time prefix without canonical automation identity",
			restored: &interfaces.FactoryWorldState{
				WorkItemsByID:       map[string]work.FactoryWorkItem{"time-lookalike": {ID: "time-lookalike", WorkTypeID: "task", State: "init"}},
				ActiveWorkItemsByID: map[string]work.FactoryWorkItem{"time-lookalike": {ID: "time-lookalike", WorkTypeID: "task", State: "init"}}, PlaceOccupancyByID: map[string]interfaces.FactoryPlaceOccupancy{},
				CompletedDispatches: []interfaces.FactoryWorldDispatchCompletion{{DispatchID: "dispatch-lookalike", WorkItemIDs: []string{"time-lookalike"}, Result: interfaces.WorkstationResult{Outcome: string(workerexecution.OutcomeAccepted)}}},
			}, net: buildSimpleNet(),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := newTestFactory(withNet(test.net), withClock(platformclock.NewDeterministic(base, time.Second)), withRestoredWorldState(test.restored))
			if err == nil || !strings.Contains(err.Error(), "has no current place occupancy") {
				t.Fatalf("New error = %v, want fail-closed missing occupancy error", err)
			}
		})
	}
}

func TestNew_RestoredAutomationRequiresOneAcceptedCompletion(t *testing.T) {
	cronWork := work.FactoryWorkItem{
		ID: "cron-ambiguous-completion", WorkTypeID: interfaces.SystemTimeWorkTypeID, State: interfaces.SystemTimePendingState,
		Tags: map[string]string{interfaces.TimeWorkTagKeySource: interfaces.TimeWorkSourceCron, interfaces.TimeWorkTagKeyCronWorkstation: "cron-refresh"},
	}
	accepted := interfaces.FactoryWorldDispatchCompletion{
		DispatchID: "dispatch-accepted", WorkItemIDs: []string{cronWork.ID},
		Result: interfaces.WorkstationResult{Outcome: string(workerexecution.OutcomeAccepted)},
	}
	tests := []struct {
		name   string
		mutate func(*interfaces.FactoryWorldState)
	}{
		{
			name: "failed dispatch also references Work",
			mutate: func(restored *interfaces.FactoryWorldState) {
				restored.FailedDispatches = []interfaces.FactoryWorldDispatchCompletion{{
					DispatchID: "dispatch-failed", WorkItemIDs: []string{cronWork.ID},
				}}
			},
		},
		{
			name: "active dispatch also references Work",
			mutate: func(restored *interfaces.FactoryWorldState) {
				restored.ActiveDispatches = map[string]interfaces.FactoryWorldDispatch{
					"dispatch-active": {DispatchID: "dispatch-active", WorkItemIDs: []string{cronWork.ID}},
				}
			},
		},
		{
			name: "multiple accepted completions",
			mutate: func(restored *interfaces.FactoryWorldState) {
				restored.CompletedDispatches = append(restored.CompletedDispatches, accepted)
			},
		},
		{
			name: "non-accepted completion",
			mutate: func(restored *interfaces.FactoryWorldState) {
				restored.CompletedDispatches[0].Result.Outcome = string(workerexecution.OutcomeFailed)
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			restored := &interfaces.FactoryWorldState{
				WorkItemsByID:       map[string]work.FactoryWorkItem{cronWork.ID: cronWork},
				ActiveWorkItemsByID: map[string]work.FactoryWorkItem{cronWork.ID: cronWork},
				PlaceOccupancyByID:  map[string]interfaces.FactoryPlaceOccupancy{},
				CompletedDispatches: []interfaces.FactoryWorldDispatchCompletion{accepted},
			}
			test.mutate(restored)
			_, err := newTestFactory(withNet(buildCronRestoreNet()), withRestoredWorldState(restored))
			if err == nil || !strings.Contains(err.Error(), "has no current place occupancy") {
				t.Fatalf("New error = %v, want fail-closed missing occupancy error", err)
			}
		})
	}
}

func buildCronRestoreNet() *state.Net {
	net := buildSimpleNet()
	net.Places[interfaces.SystemTimePendingPlaceID] = &petri.Place{ID: interfaces.SystemTimePendingPlaceID, TypeID: interfaces.SystemTimeWorkTypeID, State: interfaces.SystemTimePendingState}
	net.WorkTypes[interfaces.SystemTimeWorkTypeID] = &state.WorkType{
		ID: interfaces.SystemTimeWorkTypeID, Name: interfaces.SystemTimeWorkTypeID,
		States: []state.StateDefinition{{Value: interfaces.SystemTimePendingState, Category: state.StateCategoryProcessing}},
	}
	net.Transitions["cron-refresh"] = &petri.Transition{
		ID: "cron-refresh", Name: "cron-refresh", Type: petri.TransitionNormal, WorkerType: "mock",
		InputArcs: []petri.Arc{{ID: "cron-input", Name: "cron-input", PlaceID: interfaces.SystemTimePendingPlaceID, Direction: petri.ArcInput, Mode: interfaces.ArcModeConsume, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne}}},
	}
	return net
}

func restoreWarnings(logger *recordingLogger) []logEntry {
	if logger == nil {
		return nil
	}
	warnings := make([]logEntry, 0)
	for _, entry := range logger.entries {
		if entry.level == "warn" && strings.Contains(entry.message, "skipping completed automation Work") {
			warnings = append(warnings, entry)
		}
	}
	return warnings
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
