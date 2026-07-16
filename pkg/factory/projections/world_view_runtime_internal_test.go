package projections

import (
	"reflect"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factoryresource "github.com/portpowered/infinite-you/pkg/factory/resource"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	factorytoken "github.com/portpowered/infinite-you/pkg/factory/token"
	"github.com/portpowered/infinite-you/pkg/orchestrators/petri"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/work"
	workerconfig "github.com/portpowered/infinite-you/pkg/workers/config"
	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
)

type projectionRuntimeLookupFixture struct {
	factory      *interfaces.FactoryConfig
	workers      map[string]*workerconfig.Config
	workstations map[string]*interfaces.FactoryWorkstationConfig
}

func TestFactoryWorldReducerAppliesCanonicalStructureAndStateEvents(t *testing.T) {
	t.Parallel()
	eventTime := time.Date(2026, time.July, 16, 2, 0, 0, 0, time.UTC)
	snapshot, err := interfaces.NewFactorySnapshot(map[string]any{
		"name": "canonical-factory",
		"workTypes": []any{map[string]any{"name": "task", "states": []any{
			map[string]any{"name": "ready", "type": "INITIAL"},
			map[string]any{"name": "done", "type": "TERMINAL"},
		}}},
	})
	if err != nil {
		t.Fatalf("NewFactorySnapshot: %v", err)
	}
	reducer := newFactoryWorldReducer(3)
	structure := canonicalWorldProjectionEvent(t, interfaces.FactoryEventTypeInitialStructureRequest, interfaces.FactoryEventContext{EventTime: eventTime}, interfaces.InitialStructureRequestEventPayload{Factory: snapshot})
	if err := reducer.applyStructureEvent(structure); err != nil {
		t.Fatalf("applyStructureEvent: %v", err)
	}
	reducer.stateValue.WorkItemsByID["work-1"] = work.FactoryWorkItem{ID: "work-1", WorkTypeID: "task", State: "ready", PlaceID: "task:ready"}
	reducer.addWorkToken("work-1", "task:ready", reducer.stateValue.WorkItemsByID["work-1"])
	workState := canonicalWorldProjectionEvent(t, interfaces.FactoryEventTypeWorkStateChange, interfaces.FactoryEventContext{EventTime: eventTime.Add(time.Second), Sequence: 2, Tick: 2}, interfaces.WorkStateChangeEventPayload{FromPlaceID: "task:ready", FromState: "ready", Source: work.WorkStateChangeSourceAPI, ToPlaceID: "task:done", ToState: "done", WorkID: "work-1", WorkTypeName: "task"})
	if err := reducer.applyWorkStateChangeEvent(workState); err != nil {
		t.Fatalf("applyWorkStateChangeEvent: %v", err)
	}
	previousState := interfaces.FactoryStateRunning
	factoryState := canonicalWorldProjectionEvent(t, interfaces.FactoryEventTypeFactoryStateResponse, interfaces.FactoryEventContext{EventTime: eventTime.Add(2 * time.Second)}, interfaces.FactoryStateResponseEventPayload{PreviousState: &previousState, State: interfaces.FactoryStateCompleted})
	if err := reducer.applyFactoryStateResponseEvent(factoryState); err != nil {
		t.Fatalf("applyFactoryStateResponseEvent: %v", err)
	}
	if reducer.stateValue.Factory == snapshot || reducer.stateValue.Topology.Name != "canonical-factory" || reducer.stateValue.TerminalWorkByID["work-1"].WorkItem.State != "done" {
		t.Fatalf("canonical structure/work projection = %#v", reducer.stateValue)
	}
	if reducer.stateValue.FactoryStatePrevious != string(previousState) || reducer.stateValue.FactoryState != string(interfaces.FactoryStateCompleted) || len(reducer.stateValue.WorkStateChangesByWorkID["work-1"]) != 1 {
		t.Fatalf("canonical Factory/work state projection = %#v", reducer.stateValue)
	}
}

func TestReconstructCanonicalFactoryWorldStateOrdersOwnerEvents(t *testing.T) {
	t.Parallel()
	eventTime := time.Date(2026, time.July, 16, 7, 0, 0, 0, time.UTC)
	running := canonicalWorldProjectionEvent(t, interfaces.FactoryEventTypeFactoryStateResponse, interfaces.FactoryEventContext{EventTime: eventTime, Sequence: 1, Tick: 1}, interfaces.FactoryStateResponseEventPayload{State: interfaces.FactoryStateRunning})
	completed := canonicalWorldProjectionEvent(t, interfaces.FactoryEventTypeFactoryStateResponse, interfaces.FactoryEventContext{EventTime: eventTime.Add(time.Second), Sequence: 2, Tick: 2}, interfaces.FactoryStateResponseEventPayload{State: interfaces.FactoryStateCompleted})

	state, err := ReconstructCanonicalFactoryWorldState([]interfaces.FactoryEvent{completed, running}, 2)
	if err != nil {
		t.Fatalf("ReconstructCanonicalFactoryWorldState: %v", err)
	}
	if state.FactoryState != string(interfaces.FactoryStateCompleted) || state.EventTime != eventTime.Add(time.Second) {
		t.Fatalf("canonical ordered state = %#v, want completed at final event time", state)
	}
}

func TestFactoryWorldReducerAppliesCanonicalJavaScriptAndArtifactEvents(t *testing.T) {
	t.Parallel()
	eventTime := time.Date(2026, time.July, 16, 6, 0, 0, 0, time.FixedZone("UTC+2", 2*60*60))
	hash, label, summary := "sha256:checkpoint", "after-plan", "planning complete"
	size, secrets := int64(128), int32(2)
	dispatchID, mimeType := "dispatch-1", "application/json"
	phases := []string{"plan", "review"}
	reducer := newFactoryWorldReducer(3)

	checkpoint := canonicalWorldProjectionEvent(t, interfaces.FactoryEventTypeJavaScriptCheckpointRef, interfaces.FactoryEventContext{EventTime: eventTime, Tick: 1}, interfaces.JavaScriptCheckpointRefEventPayload{
		CheckpointID: "checkpoint-1", Label: &label, Summary: &summary, Timestamp: &eventTime,
		ArtifactRef: interfaces.FactoryArtifactRef{ID: "artifact-checkpoint", Kind: "CHECKPOINT", Visibility: "INTERNAL_CHECKPOINT", ContentHash: &hash, SizeBytes: &size},
	})
	phase := canonicalWorldProjectionEvent(t, interfaces.FactoryEventTypeJavaScriptPhaseChange, interfaces.FactoryEventContext{EventTime: eventTime, Tick: 2}, interfaces.JavaScriptPhaseChangeEventPayload{
		ArgsDigest: &hash, ChildDispatchCounts: interfaces.FactorySessionChildDispatchCounts{Queued: 1, Running: 2, Completed: 3},
		Phase: "review", Phases: phases, ScriptStatus: interfaces.FactorySessionJavaScriptScriptStatusRunning,
	})
	artifact := canonicalWorldProjectionEvent(t, interfaces.FactoryEventTypeArtifactCreated, interfaces.FactoryEventContext{EventTime: eventTime, Tick: 3}, interfaces.ArtifactCreatedEventPayload{
		Artifact: interfaces.FactoryArtifact{
			ID: "artifact-result", Kind: "FINAL_RESULT", Visibility: "PUBLIC", Label: &label, Summary: &summary,
			ContentHash: &hash, SizeBytes: &size, RedactionCounts: &interfaces.FactoryArtifactRedactionCounts{Secrets: &secrets},
			CaptureMetadata: &interfaces.FactoryArtifactCaptureMetadata{CapturedAt: &eventTime, SourceDispatchID: &dispatchID, MIMEType: &mimeType},
		},
		CapturedAt: &eventTime,
	})

	for _, event := range []interfaces.FactoryEvent{checkpoint, phase, artifact} {
		handled, err := reducer.applyOrchestratorLifecycleEvent(event)
		if err != nil || !handled {
			t.Fatalf("apply canonical %s event: handled=%t err=%v", event.Type, handled, err)
		}
	}
	phases[0] = "mutated"
	assertCanonicalJavaScriptRuntime(t, reducer.stateValue.JavaScriptRuntime, hash)
	assertCanonicalArtifactProjection(t, reducer.stateValue.JavaScriptRuntime, dispatchID, mimeType)
}

func assertCanonicalJavaScriptRuntime(t *testing.T, runtime *interfaces.FactorySessionJavaScriptRuntimeState, hash string) {
	t.Helper()
	if runtime == nil || runtime.Phase != "review" || runtime.ScriptStatus != "RUNNING" || runtime.Phases[0] != "plan" {
		t.Fatalf("JavaScript runtime = %#v", runtime)
	}
	if runtime.QueuedDispatches != 1 || runtime.RunningDispatches != 2 || runtime.CompletedDispatches != 3 {
		t.Fatalf("child dispatch counts = %#v", runtime)
	}
	if len(runtime.Checkpoints) != 1 || runtime.Checkpoints[0].Timestamp.Location() != time.UTC || runtime.Checkpoints[0].ArtifactRef.ContentHash != hash {
		t.Fatalf("checkpoint projection = %#v", runtime.Checkpoints)
	}
}

func assertCanonicalArtifactProjection(t *testing.T, runtime *interfaces.FactorySessionJavaScriptRuntimeState, dispatchID, mimeType string) {
	t.Helper()
	if len(runtime.Artifacts) != 1 || runtime.Artifacts[0].CapturedAt.Location() != time.UTC || runtime.Artifacts[0].RedactionCounts["secrets"] != 2 {
		t.Fatalf("artifact projection = %#v", runtime.Artifacts)
	}
	if runtime.Artifacts[0].CaptureMetadata["sourceDispatchId"] != dispatchID || runtime.Artifacts[0].CaptureMetadata["mimeType"] != mimeType {
		t.Fatalf("artifact capture metadata = %#v", runtime.Artifacts[0].CaptureMetadata)
	}
}

func TestCanonicalDispatchResponseReconstructsCompletionAndReleasesResources(t *testing.T) {
	t.Parallel()
	eventTime := time.Date(2026, time.July, 16, 5, 0, 0, 0, time.UTC)
	dispatchID, traceID := "dispatch-1", "trace-1"
	resourceToken := resourceTokenID("gpu", 0)
	reducer := newFactoryWorldReducer(2)
	reducer.applyInitialStructure(interfaces.InitialStructurePayload{
		Places:       []interfaces.FactoryPlace{{ID: "task:failed", TypeID: "task", State: "failed", Category: "FAILED"}},
		Workstations: []interfaces.FactoryWorkstation{{ID: "review", Name: "Review", FailurePlaceIDs: []string{"task:failed"}}},
	})
	reducer.stateValue.ActiveDispatches[dispatchID] = interfaces.FactoryWorldDispatch{
		DispatchID: dispatchID, StartedTick: 1, StartedAt: eventTime.Add(-time.Second),
		Workstation: interfaces.FactoryWorkstationRef{ID: "review", Name: "Review"},
		WorkItemIDs: []string{"work-1"}, TraceIDs: []string{traceID},
		Resources: []interfaces.FactoryResourceUnit{{ResourceID: "gpu", TokenID: resourceToken, PlaceID: "gpu:available"}},
	}
	reducer.stateValue.WorkItemsByID["work-1"] = work.FactoryWorkItem{ID: "work-1", WorkTypeID: "task", TraceID: traceID}
	failure := &workerexecution.WorkFailureMetadata{Family: workerexecution.WorkFailureFamilyRetryable, Type: workerexecution.WorkFailureTypeTimeout}
	event := canonicalWorldProjectionEvent(t, interfaces.FactoryEventTypeDispatchResponse, interfaces.FactoryEventContext{
		DispatchID: &dispatchID, EventTime: eventTime, Tick: 2, TraceIDs: &[]string{traceID}, WorkIDs: &[]string{"work-1"},
	}, workerexecution.DispatchResponseEventPayload{
		TransitionID: "review", Outcome: workerexecution.OutcomeFailed,
		DurationMillis: int64PtrForProjectionTest(1000), ProviderFailure: failure,
		OutputResources: &[]workerexecution.DispatchResourceEventRef{{Name: "gpu", Capacity: 1}},
		OutputWork: &[]work.WorkRequestEventWork{{
			WorkID: "work-1", WorkTypeID: "task", TraceID: traceID,
			Content: []work.WorkContentPart{{Type: work.WorkContentPartType("TEXT"), Text: "failed output"}},
		}},
	})

	if err := reducer.applyDispatchResponseEvent(event); err != nil {
		t.Fatalf("applyDispatchResponseEvent: %v", err)
	}
	if len(reducer.stateValue.CompletedDispatches) != 1 || len(reducer.stateValue.FailedDispatches) != 1 {
		t.Fatalf("completion counts = completed %d failed %d", len(reducer.stateValue.CompletedDispatches), len(reducer.stateValue.FailedDispatches))
	}
	completion := reducer.stateValue.CompletedDispatches[0]
	if completion.DispatchID != dispatchID || completion.Result.FailureMetadata == nil || completion.Result.FailureMetadata.Type != workerexecution.WorkFailureTypeTimeout {
		t.Fatalf("completion = %#v", completion)
	}
	if len(completion.OutputWorkItems) != 1 || completion.OutputWorkItems[0].PlaceID != "task:failed" || completion.OutputWorkItems[0].Content[0].Type != work.WorkContentPartTypeText {
		t.Fatalf("output work = %#v", completion.OutputWorkItems)
	}
	if got := reducer.tokenPlaces[resourceToken]; got != "gpu:available" {
		t.Fatalf("released resource place = %q, want gpu:available", got)
	}
	if _, active := reducer.stateValue.ActiveDispatches[dispatchID]; active {
		t.Fatal("completed dispatch remains active")
	}
}

func (f projectionRuntimeLookupFixture) Worker(name string) (*workerconfig.Config, bool) {
	worker, ok := f.workers[name]
	return worker, ok
}

func (f projectionRuntimeLookupFixture) Workstation(name string) (*interfaces.FactoryWorkstationConfig, bool) {
	workstation, ok := f.workstations[name]
	return workstation, ok
}

func (f projectionRuntimeLookupFixture) FactoryConfig() *interfaces.FactoryConfig {
	return f.factory
}

type stubGuard struct{}

func (stubGuard) Evaluate(
	_ []factorytoken.Token,
	_ map[string]*factorytoken.Token,
	_ *petri.MarkingSnapshot,
) ([]factorytoken.Token, bool) {
	return nil, false
}

type worldViewProjectionFixture struct {
	factory   *factoryapi.Factory
	state     interfaces.FactoryWorldState
	dashboard SimpleDashboardProjection
	view      interfaces.FactoryWorldView
}

func buildWorldViewProjectionState() (*factoryapi.Factory, interfaces.FactoryWorldState) {
	t0 := time.Date(2026, 6, 17, 10, 0, 0, 0, time.UTC)
	lineage := work.WorkPayloadLineageProjection{}
	lineage.RecordWorkRequestSnapshot(1, "request-queued", work.FactoryWorkItem{
		ID:          "work-queued",
		WorkTypeID:  "task",
		DisplayName: "Queued task",
		State:       "init",
		TraceID:     "trace-queued",
	})
	factory := &factoryapi.Factory{Name: "factory-canonical"}
	factorySnapshot, err := interfaces.NewFactorySnapshot(factory)
	if err != nil {
		panic(err)
	}
	return factory, interfaces.FactoryWorldState{
		Factory:        factorySnapshot,
		Topology:       buildWorldViewProjectionTopology(),
		PayloadLineage: lineage,
		WorkItemsByID:  buildWorldViewWorkItems(),
		ActiveWorkItemsByID: map[string]work.FactoryWorkItem{
			"work-queued": {ID: "work-queued", WorkTypeID: "task", DisplayName: "Queued task", State: "init", TraceID: "trace-queued"},
		},
		TerminalWorkByID:    buildWorldViewTerminalWork(),
		FailedWorkItemsByID: buildWorldViewFailedWorkItems(),
		PlaceOccupancyByID:  buildWorldViewPlaceOccupancy(),
		ActiveDispatches:    buildWorldViewActiveDispatches(t0),
		CompletedDispatches: buildWorldViewCompletedDispatches(t0),
		ProviderSessions:    buildWorldViewProviderSessions(),
		InferenceAttemptsByDispatchID: map[string]map[string]interfaces.FactoryWorldInferenceAttempt{
			"dispatch-customer": {"attempt-1": {DispatchID: "dispatch-customer", InferenceRequestID: "attempt-1", Outcome: "SUCCESS"}},
		},
		WorkStateChangesByWorkID: map[string][]interfaces.FactoryWorldWorkStateChangeRecord{
			"work-queued": {{WorkID: "work-queued", FromState: "init", ToState: "review", FromPlaceID: "task:init", ToPlaceID: "task:done", Tick: 2}},
			"empty":       nil,
		},
		JavaScriptRuntime:     buildWorldViewJavaScriptRuntime(),
		JavaScriptCheckpoints: []interfaces.FactorySessionJavaScriptCheckpointRef{{ID: "checkpoint-1", Label: "checkpoint"}},
		Artifacts:             []interfaces.FactorySessionArtifactState{{ID: "artifact-1", Label: "artifact"}},
		SessionBracket:        buildWorldViewSessionBracket(t0),
	}
}

func buildWorldViewProjectionTopology() interfaces.InitialStructurePayload {
	return interfaces.InitialStructurePayload{
		Resources: []interfaces.FactoryResource{{ID: "cpu", Name: "CPU", Capacity: 2}},
		WorkTypes: []interfaces.FactoryWorkType{
			{ID: "task", Name: "Task", States: []interfaces.FactoryStateDefinition{{Value: "init", Category: "INITIAL"}, {Value: "done", Category: "TERMINAL"}, {Value: "failed", Category: "FAILED"}}},
			{ID: interfaces.SystemTimeWorkTypeID, States: []interfaces.FactoryStateDefinition{{Value: "pending", Category: "PROCESSING"}}},
		},
		Places: []interfaces.FactoryPlace{
			{ID: "task:init", TypeID: "task", State: "init", Category: "INITIAL"},
			{ID: "task:done", TypeID: "task", State: "done", Category: "TERMINAL"},
			{ID: "task:failed", TypeID: "task", State: "failed", Category: "FAILED"},
			{ID: "cpu:available", TypeID: "cpu", State: "available", Category: "PROCESSING"},
			{ID: interfaces.SystemTimePendingPlaceID, TypeID: interfaces.SystemTimeWorkTypeID, State: "pending", Category: "PROCESSING"},
		},
		Workstations: []interfaces.FactoryWorkstation{
			{ID: "t-review", Name: "Review", WorkerID: "worker-review", Kind: string(interfaces.WorkstationKindStandard), InputPlaceIDs: []string{"task:init", "cpu:available"}, OutputPlaceIDs: []string{"task:done"}, ContinuePlaceIDs: []string{"task:init"}, RejectionPlaceIDs: []string{"task:init"}, FailurePlaceIDs: []string{"task:failed"}},
			{ID: interfaces.SystemTimeExpiryTransitionID, Name: "Expire time work", WorkerID: "worker-time", InputPlaceIDs: []string{interfaces.SystemTimePendingPlaceID}},
		},
	}
}

func buildWorldViewWorkItems() map[string]work.FactoryWorkItem {
	return map[string]work.FactoryWorkItem{
		"work-queued": {ID: "work-queued", WorkTypeID: "task", DisplayName: "Queued task", State: "init", TraceID: "trace-queued"},
		"work-active": {ID: "work-active", WorkTypeID: "task", DisplayName: "Active task", State: "review", TraceID: "trace-active", CurrentChainingTraceID: "chain-active"},
		"time-work":   {ID: "time-work", WorkTypeID: interfaces.SystemTimeWorkTypeID, DisplayName: "Clock tick", State: "pending", TraceID: "trace-time"},
	}
}

func buildWorldViewTerminalWork() map[string]interfaces.FactoryTerminalWork {
	return map[string]interfaces.FactoryTerminalWork{
		"term-success": {Status: "COMPLETED", WorkItem: work.FactoryWorkItem{ID: "work-success", WorkTypeID: "task"}},
		"term-failed":  {Status: "FAILED", WorkItem: work.FactoryWorkItem{ID: "work-failed", WorkTypeID: "task"}},
		"term-system":  {Status: "COMPLETED", WorkItem: work.FactoryWorkItem{ID: "time-finished", WorkTypeID: interfaces.SystemTimeWorkTypeID}},
	}
}

func buildWorldViewFailedWorkItems() map[string]work.FactoryWorkItem {
	return map[string]work.FactoryWorkItem{
		"failed-customer": {ID: "failed-customer", WorkTypeID: "task"},
		"failed-system":   {ID: "failed-system", WorkTypeID: interfaces.SystemTimeWorkTypeID},
	}
}

func buildWorldViewPlaceOccupancy() map[string]interfaces.FactoryPlaceOccupancy {
	return map[string]interfaces.FactoryPlaceOccupancy{
		"task:init":                         {PlaceID: "task:init", WorkItemIDs: []string{"work-queued"}, TokenCount: 1},
		"cpu:available":                     {PlaceID: "cpu:available", ResourceTokenIDs: []string{"cpu:0"}, TokenCount: 1},
		interfaces.SystemTimePendingPlaceID: {PlaceID: interfaces.SystemTimePendingPlaceID, WorkItemIDs: []string{"time-work"}, TokenCount: 1},
	}
}

func buildWorldViewActiveDispatches(t0 time.Time) map[string]interfaces.FactoryWorldDispatch {
	return map[string]interfaces.FactoryWorldDispatch{
		"dispatch-customer": {
			DispatchID:               "dispatch-customer",
			TransitionID:             "t-review",
			Workstation:              interfaces.FactoryWorkstationRef{ID: "t-review", Name: "Review"},
			StartedAt:                t0,
			WorkItemIDs:              []string{"work-active"},
			CurrentChainingTraceID:   "chain-active",
			PreviousChainingTraceIDs: []string{"chain-parent"},
			TraceIDs:                 []string{"trace-active", "trace-active", "trace-other"},
			Inputs: []interfaces.WorkstationInput{{
				TokenID: "tok-active",
				PlaceID: "task:init",
				WorkItem: &work.FactoryWorkItem{
					ID: "work-active", WorkTypeID: "task", TraceID: "trace-active",
				},
			}},
		},
		"dispatch-system": {DispatchID: "dispatch-system", TransitionID: interfaces.SystemTimeExpiryTransitionID, WorkItemIDs: []string{"time-work"}},
	}
}

func buildWorldViewCompletedDispatches(t0 time.Time) []interfaces.FactoryWorldDispatchCompletion {
	return []interfaces.FactoryWorldDispatchCompletion{
		{DispatchID: "dispatch-completed", TransitionID: "t-review", CompletedAt: t0.Add(time.Minute), Result: interfaces.WorkstationResult{Outcome: "ACCEPTED"}, WorkItemIDs: []string{"work-active"}},
		{DispatchID: "dispatch-expiry", TransitionID: interfaces.SystemTimeExpiryTransitionID, CompletedAt: t0.Add(2 * time.Minute), Result: interfaces.WorkstationResult{Outcome: "ACCEPTED"}, WorkItemIDs: []string{"time-work"}},
	}
}

func buildWorldViewProviderSessions() []interfaces.FactoryWorldProviderSessionRecord {
	return []interfaces.FactoryWorldProviderSessionRecord{
		{
			DispatchID:      "dispatch-provider",
			TransitionID:    "t-review",
			WorkItemIDs:     []string{"work-active"},
			ConsumedInputs:  []interfaces.WorkstationInput{{WorkItem: &work.FactoryWorkItem{ID: "work-active", WorkTypeID: "task"}}},
			ProviderSession: workerexecution.ProviderSessionMetadata{ID: "provider-session"},
		},
		{
			DispatchID:      "dispatch-provider-system",
			TransitionID:    interfaces.SystemTimeExpiryTransitionID,
			WorkItemIDs:     []string{"time-work"},
			ProviderSession: workerexecution.ProviderSessionMetadata{ID: "provider-system"},
		},
	}
}

func buildWorldViewJavaScriptRuntime() *interfaces.FactorySessionJavaScriptRuntimeState {
	return &interfaces.FactorySessionJavaScriptRuntimeState{
		Phase:               "plan",
		Phases:              []string{"bootstrap", "plan"},
		ArgsDigest:          "digest-1",
		ScriptStatus:        "RUNNING",
		QueuedDispatches:    1,
		RunningDispatches:   2,
		CompletedDispatches: 3,
	}
}

func buildWorldViewSessionBracket(t0 time.Time) *interfaces.FactoryWorldSessionBracketState {
	return &interfaces.FactoryWorldSessionBracketState{
		SessionID:     "session-1",
		StartedAt:     t0,
		ResultStatus:  "running",
		ResultSummary: []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "summary"}},
		ArtifactIDs:   []string{"artifact-1"},
		Terminal:      true,
		FinalStatus:   "SUCCESS",
		CompletedAt:   t0.Add(3 * time.Minute),
		FailureDetail: &workerexecution.FailureDetail{Reason: "none", Message: "none"},
	}
}

func newWorldViewProjectionFixture() worldViewProjectionFixture {
	factory, state := buildWorldViewProjectionState()
	return worldViewProjectionFixture{
		factory:   factory,
		state:     state,
		dashboard: BuildSimpleDashboardProjection(state),
		view:      BuildFactoryWorldView(state),
	}
}

func TestBuildFactoryWorldViewAndDashboardProjection_FilterCustomerFacingRuntimeData(t *testing.T) {
	t.Run("dashboard projection", testDashboardProjectionFiltersCustomerFacingRuntimeData)
	t.Run("world view projection", testWorldViewProjectionFiltersCustomerFacingRuntimeData)
	t.Run("topology projection", testWorldViewTopologyProjectionFiltersCustomerFacingRuntimeData)
}

func testDashboardProjectionFiltersCustomerFacingRuntimeData(t *testing.T) {
	t.Helper()

	fixture := newWorldViewProjectionFixture()
	dashboard := fixture.dashboard

	assertDashboardProjectionRuntimeCounts(t, dashboard)
	assertDashboardProjectionWorkstationState(t, dashboard)
	assertDashboardProjectionSessionState(t, dashboard)
}

func assertDashboardProjectionRuntimeCounts(t *testing.T, dashboard SimpleDashboardProjection) {
	t.Helper()

	if dashboard.Runtime.InFlightDispatchCount != 1 {
		t.Fatalf("InFlightDispatchCount = %d, want 1", dashboard.Runtime.InFlightDispatchCount)
	}
	if got := dashboard.Runtime.PlaceTokenCounts["task:init"]; got != 1 {
		t.Fatalf("task:init token count = %d, want 1", got)
	}
	if _, ok := dashboard.Runtime.PlaceTokenCounts[interfaces.SystemTimePendingPlaceID]; ok {
		t.Fatalf("system time place should be hidden from place counts: %#v", dashboard.Runtime.PlaceTokenCounts)
	}
	if got := dashboard.Runtime.CurrentWorkItemsByPlaceID["task:init"]; len(got) != 1 || got[0].WorkID != "work-queued" {
		t.Fatalf("current work items at task:init = %#v, want queued customer work", got)
	}
	if got := dashboard.Runtime.PlaceOccupancyWorkItemsByPlaceID["task:init"]; len(got) != 1 || got[0].PayloadStatus != string(work.WorkPayloadResolutionResolved) {
		t.Fatalf("place occupancy work items = %#v, want resolved queued work", got)
	}
	if got := dashboard.Runtime.WorkMoveOperationsByWorkID["work-queued"]; len(got) != 1 || got[0].ToState != "review" {
		t.Fatalf("work move operations = %#v, want cloned review move", got)
	}
}

func assertDashboardProjectionWorkstationState(t *testing.T, dashboard SimpleDashboardProjection) {
	t.Helper()

	if got := dashboard.WorkstationNodesByID["t-review"]; got.WorkstationName != "Review" || len(got.OutputPlaces) != 3 {
		t.Fatalf("dashboard workstation node = %#v, want review node with deduped merged outputs", got)
	}
}

func assertDashboardProjectionSessionState(t *testing.T, dashboard SimpleDashboardProjection) {
	t.Helper()

	if dashboard.Runtime.Session.DispatchedCount != 2 || dashboard.Runtime.Session.CompletedCount != 1 || dashboard.Runtime.Session.FailedCount != 1 {
		t.Fatalf("session counts = %#v, want dispatched=2 completed=1 failed=1", dashboard.Runtime.Session)
	}
	if !reflect.DeepEqual(dashboard.Runtime.Session.CompletedByWorkType, map[string]int{"task": 1}) {
		t.Fatalf("completed by work type = %#v, want task=1", dashboard.Runtime.Session.CompletedByWorkType)
	}
	if !reflect.DeepEqual(dashboard.Runtime.Session.FailedByWorkType, map[string]int{"task": 1}) {
		t.Fatalf("failed by work type = %#v, want task=1", dashboard.Runtime.Session.FailedByWorkType)
	}
	if !reflect.DeepEqual(dashboard.Runtime.Session.DispatchedByWorkType, map[string]int{"task": 2}) {
		t.Fatalf("dispatched by work type = %#v, want task=2", dashboard.Runtime.Session.DispatchedByWorkType)
	}
	if len(dashboard.Runtime.Session.ProviderSessions) != 1 || dashboard.Runtime.Session.ProviderSessions[0].ProviderSession.ID != "provider-session" {
		t.Fatalf("provider sessions = %#v, want only customer session", dashboard.Runtime.Session.ProviderSessions)
	}
	if dashboard.Runtime.Session.Bracket == nil || dashboard.Runtime.Session.Bracket.SessionID != "session-1" {
		t.Fatalf("session bracket = %#v, want projected session bracket", dashboard.Runtime.Session.Bracket)
	}
}

func testWorldViewProjectionFiltersCustomerFacingRuntimeData(t *testing.T) {
	t.Helper()

	fixture := newWorldViewProjectionFixture()
	view := fixture.view

	if view.Factory == nil {
		t.Fatal("Factory is nil, want cloned canonical factory")
	}
	var projectedFactory factoryapi.Factory
	if err := view.Factory.Decode(&projectedFactory); err != nil {
		t.Fatalf("decode Factory: %v", err)
	}
	if projectedFactory.Name != "factory-canonical" {
		t.Fatalf("Factory name = %q, want factory-canonical", projectedFactory.Name)
	}
	if !reflect.DeepEqual(view.Runtime.ActiveDispatchIDs, []string{"dispatch-customer"}) {
		t.Fatalf("ActiveDispatchIDs = %#v, want only customer dispatch", view.Runtime.ActiveDispatchIDs)
	}
	if got := view.Runtime.ActiveExecutionsByDispatchID["dispatch-customer"]; got.DispatchID != "dispatch-customer" || !reflect.DeepEqual(got.TraceIDs, []string{"trace-active", "trace-other"}) {
		t.Fatalf("active execution = %#v, want canonicalized trace ids", got)
	}
	if _, ok := view.Runtime.ActiveExecutionsByDispatchID["dispatch-system"]; ok {
		t.Fatalf("system-only dispatch should be hidden from active executions")
	}
	if view.Runtime.JavaScript == nil || view.Runtime.JavaScript.Phase != "plan" || len(view.Runtime.JavaScript.Checkpoints) != 1 || len(view.Runtime.JavaScript.Artifacts) != 1 {
		t.Fatalf("javascript projection = %#v, want merged runtime/checkpoint/artifact state", view.Runtime.JavaScript)
	}
}

func testWorldViewTopologyProjectionFiltersCustomerFacingRuntimeData(t *testing.T) {
	t.Helper()

	view := newWorldViewProjectionFixture().view

	if len(view.Topology.WorkstationNodeIDs) != 1 || view.Topology.WorkstationNodeIDs[0] != "t-review" {
		t.Fatalf("WorkstationNodeIDs = %#v, want only customer workstation", view.Topology.WorkstationNodeIDs)
	}
	if got := view.Topology.WorkstationNodesByID["t-review"]; !reflect.DeepEqual(got.InputPlaceIDs, []string{"cpu:available", "task:init"}) {
		t.Fatalf("topology node input places = %#v, want sorted customer-visible inputs", got.InputPlaceIDs)
	}
}

func TestWorkItemReferenceHelpers_FilterDeduplicateAndStabilize(t *testing.T) {
	lineage := work.WorkPayloadLineageProjection{}
	lineage.RecordWorkRequestSnapshot(1, "request-a", work.FactoryWorkItem{
		ID: "work-a", WorkTypeID: "task", State: "init", TraceID: "trace-a",
	})
	items := map[string]work.FactoryWorkItem{
		"work-a":    {ID: "work-a", WorkTypeID: "task", State: "init", TraceID: "trace-a"},
		"work-b":    {ID: "work-b", WorkTypeID: "task", TraceID: "trace-b"},
		"time-only": {ID: "time-only", WorkTypeID: interfaces.SystemTimeWorkTypeID, TraceID: "trace-time"},
	}

	refs := workItemRefsForIDs(lineage, []string{"work-b", "time-only", "work-a", "missing"}, items)
	if !reflect.DeepEqual([]string{refs[0].WorkID, refs[1].WorkID}, []string{"work-a", "work-b"}) {
		t.Fatalf("workItemRefsForIDs = %#v, want sorted customer refs", refs)
	}
	if refs[0].PayloadStatus != string(work.WorkPayloadResolutionResolved) {
		t.Fatalf("resolved ref payload status = %q, want RESOLVED", refs[0].PayloadStatus)
	}
	if refs[1].PayloadStatus != string(work.WorkPayloadResolutionUnavailable) || refs[1].PayloadUnavailableReason == "" {
		t.Fatalf("unresolved ref = %#v, want unavailable reason", refs[1])
	}

	activeRefs := workRefsForActiveIDs(work.WorkPayloadLineageProjection{}, []string{"missing"}, items)
	if activeRefs == nil || len(activeRefs) != 0 {
		t.Fatalf("workRefsForActiveIDs = %#v, want empty non-nil slice", activeRefs)
	}

	itemRefs := workItemRefsForItems(lineage, []work.FactoryWorkItem{
		items["work-b"],
		items["work-a"],
		items["work-b"],
		items["time-only"],
		{},
	})
	if !reflect.DeepEqual([]string{itemRefs[0].WorkID, itemRefs[1].WorkID}, []string{"work-b", "work-a"}) {
		t.Fatalf("workItemRefsForItems = %#v, want original-order deduped refs", itemRefs)
	}

	inputRefs := workItemRefsForInputs(lineage, []interfaces.WorkstationInput{
		{WorkItem: &work.FactoryWorkItem{ID: "work-b", WorkTypeID: "task"}},
		{WorkItem: &work.FactoryWorkItem{ID: "work-b", WorkTypeID: "task"}},
		{WorkItem: &work.FactoryWorkItem{ID: "time-only", WorkTypeID: interfaces.SystemTimeWorkTypeID}},
		{},
	})
	if len(inputRefs) != 1 || inputRefs[0].WorkID != "work-b" {
		t.Fatalf("workItemRefsForInputs = %#v, want one customer ref", inputRefs)
	}

	sessionRefs := providerSessionWorkItemRefs(lineage, interfaces.FactoryWorldProviderSessionRecord{
		ConsumedInputs: []interfaces.WorkstationInput{{WorkItem: &work.FactoryWorkItem{ID: "work-b", WorkTypeID: "task"}}},
		WorkItemIDs:    []string{"work-a", "work-b", ""},
	})
	if !reflect.DeepEqual([]string{sessionRefs[0].WorkID, sessionRefs[1].WorkID}, []string{"work-b", "work-a"}) {
		t.Fatalf("providerSessionWorkItemRefs = %#v, want input-first deduped refs", sessionRefs)
	}
	if providerSessionWorkItemRefs(work.WorkPayloadLineageProjection{}, interfaces.FactoryWorldProviderSessionRecord{}) != nil {
		t.Fatal("providerSessionWorkItemRefs(empty) should return nil")
	}

	merged := mergeWorkRefs(
		[]interfaces.FactoryWorldWorkItemRef{{WorkID: "work-b", WorkTypeID: "task"}, {WorkID: "work-a", WorkTypeID: "task"}},
		[]interfaces.FactoryWorldWorkItemRef{{WorkID: "work-c", WorkTypeID: "task"}, {WorkID: "work-a", WorkTypeID: "task-override"}},
	)
	if !reflect.DeepEqual([]string{merged[0].WorkID, merged[1].WorkID, merged[2].WorkID}, []string{"work-a", "work-b", "work-c"}) || merged[0].WorkTypeID != "task-override" {
		t.Fatalf("mergeWorkRefs = %#v, want sorted merged refs with override", merged)
	}
}

func TestProjectActiveThrottlePauses_UsesProviderFallbackAndFiltersAffectedTopology(t *testing.T) {
	pausedUntil := time.Date(2026, 6, 17, 10, 5, 0, 0, time.UTC)
	topology := interfaces.InitialStructurePayload{
		Workers: []interfaces.FactoryWorker{
			{ID: "worker-provider-fallback", Provider: "openai", Model: "gpt-5"},
			{ID: "worker-explicit-model-provider", ModelProvider: "anthropic", Model: "claude"},
		},
		Workstations: []interfaces.FactoryWorkstation{
			{ID: "review-b", Name: "Review B", WorkerID: "worker-provider-fallback", InputPlaceIDs: []string{"task:init", interfaces.SystemTimePendingPlaceID}},
			{ID: "review-a", Name: "Review A", WorkerID: "worker-provider-fallback", InputPlaceIDs: []string{"task:init", "report:init"}},
			{ID: "review-c", Name: "Review C", WorkerID: "worker-explicit-model-provider", InputPlaceIDs: []string{"report:init"}},
			{ID: "no-worker", Name: "No Worker"},
		},
		Places: []interfaces.FactoryPlace{
			{ID: "task:init", TypeID: "task"},
			{ID: "report:init", TypeID: "report"},
			{ID: interfaces.SystemTimePendingPlaceID, TypeID: interfaces.SystemTimeWorkTypeID},
		},
	}

	projected := ProjectActiveThrottlePauses(topology, []interfaces.ActiveThrottlePause{{
		LaneID:      "openai/gpt-5",
		Provider:    "OPENAI",
		Model:       "gpt-5",
		PausedAt:    pausedUntil.Add(-time.Minute),
		PausedUntil: pausedUntil,
	}})
	if len(projected) != 1 {
		t.Fatalf("ProjectActiveThrottlePauses() = %#v, want one pause", projected)
	}
	if !reflect.DeepEqual(projected[0].AffectedTransitionIDs, []string{"review-a", "review-b"}) {
		t.Fatalf("AffectedTransitionIDs = %#v, want sorted matching workstations", projected[0].AffectedTransitionIDs)
	}
	if !reflect.DeepEqual(projected[0].AffectedWorkstationNames, []string{"Review A", "Review B"}) {
		t.Fatalf("AffectedWorkstationNames = %#v, want sorted names", projected[0].AffectedWorkstationNames)
	}
	if !reflect.DeepEqual(projected[0].AffectedWorkerTypes, []string{"worker-provider-fallback"}) {
		t.Fatalf("AffectedWorkerTypes = %#v, want deduped worker type", projected[0].AffectedWorkerTypes)
	}
	if !reflect.DeepEqual(projected[0].AffectedWorkTypeIDs, []string{"report", "task"}) {
		t.Fatalf("AffectedWorkTypeIDs = %#v, want filtered non-system work types", projected[0].AffectedWorkTypeIDs)
	}
	if ProjectActiveThrottlePauses(topology, nil) != nil {
		t.Fatal("ProjectActiveThrottlePauses(nil) should return nil")
	}
}

func TestTopologyProjectionHelpers_ProjectRuntimeMetadataAndConstraints(t *testing.T) {
	t.Run("runtime factory metadata helpers", testRuntimeFactoryMetadataHelpers)
	t.Run("transition and worker metadata helpers", testTransitionAndWorkerMetadataHelpers)
	t.Run("guard and constraint helpers", testGuardAndConstraintHelpers)
}

func newRuntimeLookupFixture() projectionRuntimeLookupFixture {
	locked := true
	parentID := "group-parent"
	return projectionRuntimeLookupFixture{
		factory: &interfaces.FactoryConfig{
			Name: "factory-runtime",
			Version: &interfaces.FactoryVersion{
				Logical: 7, Physical: time.Date(2026, 6, 17, 11, 0, 0, 0, time.UTC),
			},
			ResourceManifest: &interfaces.PortableResourceManifestConfig{
				RequiredTools: []interfaces.RequiredToolConfig{{Name: "go", Command: "go"}},
			},
			Layout: &interfaces.FactoryLayoutConfig{
				SchemaVersion: 1,
				Nodes:         []interfaces.FactoryLayoutNodeConfig{{ID: "review", Position: interfaces.FactoryLayoutPointConfig{X: 1, Y: 2}, Locked: &locked}},
				Edges:         []interfaces.FactoryLayoutEdgeConfig{{ID: "edge-1", Waypoints: []interfaces.FactoryLayoutPointConfig{{X: 3, Y: 4}}}},
				Groups:        []interfaces.FactoryLayoutGroupConfig{{ID: "group-1", Bounds: interfaces.FactoryLayoutBoundsConfig{Width: 5, Height: 6}, NodeIDs: []string{"review"}, ParentGroupID: &parentID}},
				Viewport:      &interfaces.FactoryLayoutViewportConfig{X: 7, Y: 8, Zoom: 0.9},
				Preferences:   &interfaces.FactoryLayoutPreferencesConfig{Direction: "LR"},
			},
			Workstations: []interfaces.FactoryWorkstationConfig{{
				Name:           "Review",
				WorkerTypeName: "worker-review",
			}},
		},
		workers: map[string]*workerconfig.Config{
			"worker-review": {Type: interfaces.WorkerTypeModel, Concurrency: 2, Timeout: "30s"},
		},
		workstations: map[string]*interfaces.FactoryWorkstationConfig{
			"Review": {
				Name:           "Review",
				Kind:           interfaces.WorkstationKindCron,
				WorkerTypeName: "worker-review",
				Resources:      []factoryresource.Config{{Name: "gpu", Capacity: 2}, {Name: "", Capacity: 4}},
				Guards:         []interfaces.GuardConfig{{Type: interfaces.GuardTypeVisitCount, Workstation: "other", MaxVisits: 3}, {}},
				Cron:           &interfaces.CronConfig{Schedule: "* * * * *", TriggerAtStart: true, Jitter: "5s", ExpiryWindow: "1m"},
				StopWords:      []string{"stop", "pause"},
				Limits:         interfaces.WorkstationLimits{MaxRetries: 4, MaxExecutionTime: "2m"},
				Timeout:        "legacy-timeout",
			},
		},
	}
}

func testRuntimeFactoryMetadataHelpers(t *testing.T) {
	t.Helper()

	fixture := newRuntimeLookupFixture()
	version := runtimeFactoryVersion(fixture)
	manifest := runtimeFactoryResourceManifest(fixture)
	layout := runtimeFactoryLayout(fixture)
	if runtimeFactoryName(fixture) != "factory-runtime" {
		t.Fatalf("runtimeFactoryName() = %q, want factory-runtime", runtimeFactoryName(fixture))
	}
	version.Logical = 99
	manifest.RequiredTools[0].Name = "mutated"
	layout.Nodes[0].ID = "mutated"
	if fixture.factory.Version.Logical != 7 || fixture.factory.ResourceManifest.RequiredTools[0].Name != "go" || fixture.factory.Layout.Nodes[0].ID != "review" {
		t.Fatalf("runtime factory helpers should clone config values: %#v", fixture.factory)
	}
}

func testTransitionAndWorkerMetadataHelpers(t *testing.T) {
	t.Helper()

	if got := transitionWorkerIDs(map[string]*petri.Transition{
		"b": {WorkerType: "worker-review"},
		"a": {WorkerType: "worker-review"},
		"c": {WorkerType: "worker-other"},
		"d": nil,
	}); !reflect.DeepEqual(got, []string{"worker-other", "worker-review"}) {
		t.Fatalf("transitionWorkerIDs() = %#v, want sorted unique ids", got)
	}

	if got := workerConfigWithUsage(&workerconfig.Config{Type: interfaces.WorkerTypeModel}, nil); !reflect.DeepEqual(got, map[string]string{"type": interfaces.WorkerTypeModel}) {
		t.Fatalf("workerConfigWithUsage() = %#v, want type", got)
	}
	if workerConfigWithUsage(nil, nil) != nil {
		t.Fatal("workerConfigWithUsage(nil) should return nil")
	}
}

func testGuardAndConstraintHelpers(t *testing.T) {
	t.Helper()

	fixture := newRuntimeLookupFixture()

	if got := guardConstraintType(&petri.VisitCountGuard{}); got != "visit_count_guard" {
		t.Fatalf("guardConstraintType(VisitCountGuard) = %q", got)
	}
	if got := guardConstraintType(&petri.CronTimeWindowGuard{}); got != "cron_time_window_guard" {
		t.Fatalf("guardConstraintType(CronTimeWindowGuard) = %q", got)
	}
	if got := guardConstraintType(stubGuard{}); got != "guard" {
		t.Fatalf("guardConstraintType(stubGuard) = %q, want guard", got)
	}

	values := guardConstraintValues(petri.Arc{
		Name:        "work",
		PlaceID:     "task:init",
		Mode:        interfaces.ArcModeObserve,
		Cardinality: petri.ArcCardinality{Mode: petri.CardinalityN, Count: 2},
		Guard:       &petri.MatchColorGuard{Field: "kind", MatchBinding: "work", MatchField: "parent"},
	}, "input")
	if values["arc_set"] != "input" || values["binding"] != "work" || values["mode"] == "" || values["cardinality"] == "" || values["field"] != "kind" || values["match_binding"] != "work" || values["match_field"] != "parent" {
		t.Fatalf("guardConstraintValues(MatchColorGuard) = %#v, want populated guard metadata", values)
	}

	net := &state.Net{
		Transitions: map[string]*petri.Transition{
			"review": {
				ID:         "review",
				Name:       "Review",
				WorkerType: "worker-review",
				InputArcs: []petri.Arc{{
					Name:        "task",
					PlaceID:     "task:init",
					Guard:       &petri.VisitCountGuard{TransitionID: "review", MaxVisits: 2},
					Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne},
				}},
			},
		},
		Limits: state.GlobalLimits{MaxTokenAge: time.Minute, MaxTotalVisits: 9},
	}
	constraints := factoryConstraints(net, fixture)
	wantConstraintTypes := map[string]bool{
		"global_limit":       true,
		"visit_count_guard":  true,
		"worker_concurrency": true,
		"worker_timeout":     true,
		"resource_usage":     true,
		"configured_guard":   true,
		"cron_trigger":       true,
		"stop_words":         true,
		"workstation_limit":  true,
	}
	for _, constraint := range constraints {
		delete(wantConstraintTypes, constraint.Type)
	}
	if len(wantConstraintTypes) != 0 {
		t.Fatalf("factoryConstraints() missing types: %#v from %#v", wantConstraintTypes, constraints)
	}
}

func TestSessionLifecycleHelperFunctions_ProjectStableCopies(t *testing.T) {
	emptyState := interfaces.FactoryWorldState{SessionBracket: &interfaces.FactoryWorldSessionBracketState{}}
	if buildFactoryWorldSessionBracketProjection(emptyState) != nil {
		t.Fatal("empty non-terminal bracket should stay hidden")
	}

	parts := []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "summary"}}
	state := interfaces.FactoryWorldState{
		SessionBracket: &interfaces.FactoryWorldSessionBracketState{
			SessionID:      "session-2",
			StartedAt:      time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC),
			ResultStatus:   "running",
			ResultSummary:  parts,
			ArtifactIDs:    []string{"artifact-1"},
			Terminal:       true,
			FinalStatus:    "FAILED",
			CompletedAt:    time.Date(2026, 6, 17, 12, 1, 0, 0, time.UTC),
			DurationMillis: 60000,
			FailureDetail:  &workerexecution.FailureDetail{Reason: "timeout", Message: "timed out"},
		},
	}
	projected := buildFactoryWorldSessionBracketProjection(state)
	if projected == nil || projected.SessionID != "session-2" || projected.FinalStatus != "FAILED" {
		t.Fatalf("buildFactoryWorldSessionBracketProjection() = %#v, want projected terminal bracket", projected)
	}
	projected.ResultSummary[0].Text = "mutated"
	projected.ArtifactIDs[0] = "mutated"
	if state.SessionBracket.ResultSummary[0].Text != "summary" || state.SessionBracket.ArtifactIDs[0] != "artifact-1" {
		t.Fatalf("projected bracket should clone slices: %#v", state.SessionBracket)
	}

	artifacts := []interfaces.FactorySessionArtifactState{{ID: " artifact-1 "}}
	if artifact, ok := findArtifactStateByID(artifacts, "artifact-1"); !ok || artifact.ID != " artifact-1 " {
		t.Fatalf("findArtifactStateByID() = %#v, %v; want trimmed lookup match", artifact, ok)
	}

	appendUniqueArtifactState(&artifacts, interfaces.FactorySessionArtifactState{ID: "artifact-1"})
	appendUniqueArtifactState(&artifacts, interfaces.FactorySessionArtifactState{ID: "artifact-2"})
	if !reflect.DeepEqual([]string{artifacts[0].ID, artifacts[1].ID}, []string{" artifact-1 ", "artifact-2"}) {
		t.Fatalf("appendUniqueArtifactState() = %#v, want trimmed dedupe", artifacts)
	}

	clonedParts := cloneWorkContentParts(parts)
	clonedParts[0].Text = "changed"
	if parts[0].Text != "summary" {
		t.Fatalf("cloneWorkContentParts() should deep copy top-level slice: %#v", parts)
	}
}
