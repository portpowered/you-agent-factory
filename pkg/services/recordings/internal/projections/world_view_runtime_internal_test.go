package projections

import (
	"reflect"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestFactoryWorldReducerRejectsMalformedOwnerEventsAndHandlesEmptyRoutes(t *testing.T) {
	t.Parallel()

	malformedEvents := []struct {
		name      string
		eventType interfaces.FactoryEventType
		apply     func(*factoryWorldReducer, interfaces.FactoryEvent) error
	}{
		{name: "work state", eventType: interfaces.FactoryEventTypeWorkStateChange, apply: (*factoryWorldReducer).applyWorkStateChangeEvent},
		{name: "dispatch request", eventType: interfaces.FactoryEventTypeDispatchRequest, apply: (*factoryWorldReducer).applyDispatchRequestEvent},
		{name: "dispatch response", eventType: interfaces.FactoryEventTypeDispatchResponse, apply: (*factoryWorldReducer).applyDispatchResponseEvent},
		{name: "factory state", eventType: interfaces.FactoryEventTypeFactoryStateResponse, apply: (*factoryWorldReducer).applyFactoryStateResponseEvent},
	}
	for _, test := range malformedEvents {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if err := test.apply(newFactoryWorldReducer(0), interfaces.FactoryEvent{
				Type:    test.eventType,
				Payload: []byte("{"),
			}); err == nil {
				t.Fatalf("malformed %s event returned nil error", test.name)
			}
		})
	}

	structureEvents := []struct {
		name      string
		eventType interfaces.FactoryEventType
		payload   any
	}{
		{name: "run request", eventType: interfaces.FactoryEventTypeRunRequest, payload: interfaces.RunRequestEventPayload{}},
		{name: "initial structure", eventType: interfaces.FactoryEventTypeInitialStructureRequest, payload: interfaces.InitialStructureRequestEventPayload{}},
		{name: "factory change", eventType: interfaces.FactoryEventTypeFactoryChange, payload: interfaces.FactoryChangeEventPayload{}},
	}
	for _, test := range structureEvents {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if err := newFactoryWorldReducer(0).applyStructureEvent(canonicalWorldProjectionEvent(t, test.eventType, interfaces.FactoryEventContext{}, test.payload)); err == nil {
				t.Fatalf("nil Factory snapshot in %s returned nil error", test.name)
			}
		})
	}

	missingPayloadEvents := []struct {
		name      string
		eventType interfaces.FactoryEventType
		apply     func(*factoryWorldReducer, interfaces.FactoryEvent) error
	}{
		{name: "work request", eventType: interfaces.FactoryEventTypeWorkRequest, apply: (*factoryWorldReducer).applyWorkRequestEvent},
		{name: "relationship change", eventType: interfaces.FactoryEventTypeRelationshipChangeRequest, apply: (*factoryWorldReducer).applyRelationshipChangeEvent},
	}
	for _, test := range missingPayloadEvents {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if err := test.apply(newFactoryWorldReducer(0), canonicalWorldProjectionEvent(t, test.eventType, interfaces.FactoryEventContext{}, struct{}{})); err == nil {
				t.Fatalf("empty %s payload returned nil error", test.name)
			}
		})
	}

	reducer := newFactoryWorldReducer(0)
	if err := reducer.applyCanonicalFactory(nil); err == nil {
		t.Fatal("nil Factory snapshot returned nil error")
	}
	if got := reducer.outputPlaceForWork("missing", workerexecution.OutcomeContinue, "task"); got != "" {
		t.Fatalf("unknown workstation output place = %q, want empty", got)
	}
	if got := reducer.outputPlaceForOutcome(interfaces.FactoryWorkstation{}, workerexecution.OutcomeContinue, "task"); got != "" {
		t.Fatalf("empty continue route = %q, want empty", got)
	}
	if got := reducer.outputPlaceForOutcome(interfaces.FactoryWorkstation{}, workerexecution.OutcomeRejected, "task"); got != "" {
		t.Fatalf("empty rejection route = %q, want empty", got)
	}
	if got := reducer.firstAvailableResourceTokenID(""); got != "" {
		t.Fatalf("empty resource ID token = %q, want empty", got)
	}
	if got := reducer.firstAvailableResourceTokenID("missing"); got != "" {
		t.Fatalf("missing resource token = %q, want empty", got)
	}

	if got := reducer.consumeResourceUnits(nil); got != nil {
		t.Fatalf("nil resource refs = %#v, want nil", got)
	}
	emptyResources := []interfaces.DispatchResourceRef{}
	if got := reducer.consumeResourceUnits(&emptyResources); got != nil {
		t.Fatalf("empty resource refs = %#v, want nil", got)
	}
	unnamedResources := []interfaces.DispatchResourceRef{{}}
	if got := reducer.consumeResourceUnits(&unnamedResources); len(got) != 0 {
		t.Fatalf("unnamed resource refs = %#v, want no consumed units", got)
	}

	consumed := []interfaces.FactoryResourceUnit{{ResourceID: "reviewers", TokenID: "resource-token"}}
	otherResource := []workerexecution.DispatchResourceEventRef{{Name: "other"}}
	reducer.releaseResourceUnits(consumed, &otherResource)
	reducer.releaseResourceUnits(consumed, &[]workerexecution.DispatchResourceEventRef{{Name: "reviewers"}})
	if got := firstConsumedResourceIndex(consumed, []bool{true}, "reviewers"); got != -1 {
		t.Fatalf("released resource index = %d, want -1", got)
	}

	if got := firstNonEmpty("", "", "selected"); got != "selected" {
		t.Fatalf("firstNonEmpty = %q, want selected", got)
	}
	if got := firstNonEmpty("", ""); got != "" {
		t.Fatalf("empty firstNonEmpty = %q, want empty", got)
	}
	if got := removeString([]string{"work"}, ""); len(got) != 1 || got[0] != "work" {
		t.Fatalf("removeString empty target = %#v, want original value", got)
	}
	if got := removeString(nil, "work"); got != nil {
		t.Fatalf("removeString empty values = %#v, want nil", got)
	}

	// Empty identities are no-ops for the trace indexes and token bookkeeping.
	item := work.FactoryWorkItem{ID: "work-1"}
	reducer.addWorkToken("", "task:ready", item)
	reducer.addWorkToken("token-1", "", item)
	reducer.removeToken("")
	reducer.addTraceWork("", "work-1")
	reducer.addTraceDispatch("", "dispatch-1")
	reducer.addTraceTerminal("", "work-1")
	reducer.addTraceFailed("", "work-1")
	reducer.removeTraceFailed("", "work-1")
	reducer.removeTraceTerminal("", "work-1")
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
	reducer.stateValue.WorkItemsByID["work-1"] = work.FactoryWorkItem{ID: "work-1", WorkTypeID: "task", State: "ready"}
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

func TestReconstructCanonicalFactoryWorldStateProjectsPendingHumanApprovalFromReplay(t *testing.T) {
	t.Parallel()
	eventTime := time.Date(2026, time.July, 16, 8, 0, 0, 0, time.UTC)
	snapshot, err := interfaces.NewFactorySnapshot(map[string]any{
		"name": "approval-factory",
		"workstations": []any{map[string]any{
			"id":   "approval-workstation",
			"name": "Release Approval",
			"description": map[string]any{
				"type":    interfaces.NameValueTypeLocalizableAsset,
				"value":   "release-approval-description",
				"locales": []any{"en-US", "fr-FR"},
				"values":  map[string]any{"en-US": "Approve the release", "fr-FR": "Approuver la version"},
			},
		}},
	})
	if err != nil {
		t.Fatalf("NewFactorySnapshot: %v", err)
	}
	structure := canonicalWorldProjectionEvent(t, interfaces.FactoryEventTypeInitialStructureRequest,
		interfaces.FactoryEventContext{Sequence: 0, Tick: 0, EventTime: eventTime},
		interfaces.InitialStructureRequestEventPayload{Factory: snapshot})
	dispatchID, sessionID, requestID := "dispatch-approval-1", "session-approval-1", "request-approval-1"
	workIDs := []string{"work-2", "work-1"}
	traceIDs := []string{"trace-1"}
	dispatch := canonicalWorldProjectionEvent(t, interfaces.FactoryEventTypeDispatchRequest,
		interfaces.FactoryEventContext{
			Sequence: 1, Tick: 1, EventTime: eventTime.Add(time.Second), SessionID: &sessionID,
			RequestID: &requestID, DispatchID: &dispatchID, WorkIDs: &workIDs, TraceIDs: &traceIDs,
		}, interfaces.DispatchRequestEventPayload{
			TransitionID: "approval-workstation",
			Inputs: []interfaces.DispatchConsumedWorkRef{
				{WorkID: "work-2"},
				{WorkID: "work-1"},
			},
		})
	approval := canonicalWorldProjectionEvent(t, interfaces.FactoryEventTypeHumanApprovalRequested,
		interfaces.FactoryEventContext{
			Sequence: 2, Tick: 1, EventTime: eventTime.Add(2 * time.Second), SessionID: &sessionID,
			RequestID: &requestID, DispatchID: &dispatchID, WorkIDs: &workIDs, TraceIDs: &traceIDs,
		}, interfaces.HumanApprovalRequestedEventPayload{
			ApprovalID:    "approval-dispatch-approval-1",
			WorkstationID: "approval-workstation",
			Decisions: []interfaces.HumanApprovalDecision{
				interfaces.HumanApprovalDecisionApprove,
				interfaces.HumanApprovalDecisionReject,
			},
			Status: interfaces.HumanApprovalStatusPending,
		})

	state, err := ReconstructCanonicalFactoryWorldState([]interfaces.FactoryEvent{approval, dispatch, structure}, 1)
	if err != nil {
		t.Fatalf("ReconstructCanonicalFactoryWorldState: %v", err)
	}
	assertPendingHumanApprovalProjection(t, state, sessionID, requestID, dispatchID, workIDs, traceIDs)

	replayed, err := ReconstructCanonicalFactoryWorldState([]interfaces.FactoryEvent{structure, dispatch, approval}, 1)
	if err != nil {
		t.Fatalf("replay after reconnect: %v", err)
	}
	if !reflect.DeepEqual(state.PendingHumanApprovalsByID, replayed.PendingHumanApprovalsByID) {
		t.Fatalf("replayed pending approvals = %#v, want stable projection %#v", replayed.PendingHumanApprovalsByID, state.PendingHumanApprovalsByID)
	}
}

func assertPendingHumanApprovalProjection(t *testing.T, state interfaces.FactoryWorldState, sessionID, requestID, dispatchID string, workIDs, traceIDs []string) {
	t.Helper()
	if len(state.PendingHumanApprovalsByID) != 1 {
		t.Fatalf("pending approvals = %#v, want one replayed approval", state.PendingHumanApprovalsByID)
	}
	pending := state.PendingHumanApprovalsByID["approval-dispatch-approval-1"]
	if pending.SessionID != sessionID || pending.RequestID != requestID || pending.DispatchID != dispatchID ||
		pending.WorkstationID != "approval-workstation" || pending.WorkstationName != "Release Approval" ||
		pending.Status != interfaces.HumanApprovalStatusPending || !reflect.DeepEqual(pending.WorkItemIDs, workIDs) ||
		!reflect.DeepEqual(pending.TraceIDs, traceIDs) {
		t.Fatalf("pending approval = %#v, want canonical correlation and topology identity", pending)
	}
	if pending.WorkstationDescription == nil || interfaces.ResolveNameValue(*pending.WorkstationDescription, "fr-FR") != "Approuver la version" {
		t.Fatalf("pending workstation description = %#v, want localized effective-factory description", pending.WorkstationDescription)
	}
	if !reflect.DeepEqual(pending.Decisions, []interfaces.HumanApprovalDecision{
		interfaces.HumanApprovalDecisionApprove,
		interfaces.HumanApprovalDecisionReject,
	}) {
		t.Fatalf("pending decisions = %#v, want APPROVE and REJECT only", pending.Decisions)
	}
	if _, ok := state.ActiveDispatches[dispatchID]; !ok {
		t.Fatalf("active dispatches = %#v, want claimed dispatch retained while approval is pending", state.ActiveDispatches)
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
	if len(completion.OutputWorkItems) != 1 || completion.OutputWorkItems[0].State != "failed" || completion.OutputWorkItems[0].Content[0].Type != work.WorkContentPartTypeText {
		t.Fatalf("output work = %#v", completion.OutputWorkItems)
	}
	if got := reducer.tokenPlaces[resourceToken]; got != "gpu:available" {
		t.Fatalf("released resource place = %q, want gpu:available", got)
	}
	if _, active := reducer.stateValue.ActiveDispatches[dispatchID]; active {
		t.Fatal("completed dispatch remains active")
	}
}

func TestDispatchInterruptedRearmsNonTerminalInputsAndResources(t *testing.T) {
	t.Parallel()

	reducer := newFactoryWorldReducer(3)
	dispatchID := "dispatch-interrupted"
	live := work.FactoryWorkItem{ID: "work-live", WorkTypeID: "task", State: "processing", TraceID: "trace-live"}
	fallback := work.FactoryWorkItem{ID: "work-fallback", WorkTypeID: "task", TraceID: "trace-fallback"}
	terminal := work.FactoryWorkItem{ID: "work-terminal", WorkTypeID: "task", State: "done"}
	failed := work.FactoryWorkItem{ID: "work-failed", WorkTypeID: "task", State: "failed"}
	noPlace := work.FactoryWorkItem{ID: "work-no-place", WorkTypeID: "task"}
	reducer.stateValue.WorkItemsByID[live.ID] = live
	reducer.stateValue.WorkItemsByID[fallback.ID] = fallback
	reducer.stateValue.WorkItemsByID[terminal.ID] = terminal
	reducer.stateValue.WorkItemsByID[failed.ID] = failed
	reducer.stateValue.TerminalWorkByID[terminal.ID] = interfaces.FactoryTerminalWork{WorkItem: terminal, Status: "TERMINAL"}
	reducer.stateValue.FailedWorkItemsByID[failed.ID] = failed
	reducer.workPlaces[fallback.ID] = "task:ready"
	reducer.stateValue.ActiveDispatches[dispatchID] = interfaces.FactoryWorldDispatch{
		DispatchID: dispatchID,
		Inputs: []interfaces.WorkstationInput{
			{TokenID: live.ID, PlaceID: "task:processing", WorkItem: &live},
			{TokenID: fallback.ID, WorkItem: nil},
			{TokenID: terminal.ID, PlaceID: "task:done", WorkItem: &terminal},
			{TokenID: failed.ID, PlaceID: "task:failed", WorkItem: &failed},
			{TokenID: noPlace.ID, WorkItem: &noPlace},
			{TokenID: "", PlaceID: "task:processing"},
		},
		Resources: []interfaces.FactoryResourceUnit{
			{ResourceID: "gpu", TokenID: "gpu-token", PlaceID: "gpu:held"},
			{ResourceID: "cpu", TokenID: "cpu-token"},
			{ResourceID: "ignored", TokenID: ""},
		},
	}
	reason := "daemon restart interrupted process-bound attempt"
	event := canonicalWorldProjectionEvent(t, interfaces.FactoryEventTypeDispatchInterrupted, interfaces.FactoryEventContext{
		DispatchID: &dispatchID,
		EventTime:  time.Date(2026, time.July, 16, 6, 0, 0, 0, time.UTC),
		Tick:       3,
	}, interfaces.DispatchInterruptedEventPayload{Reason: reason})

	if err := reducer.applyDispatchInterruptedEvent(event); err != nil {
		t.Fatalf("applyDispatchInterruptedEvent: %v", err)
	}
	if _, active := reducer.stateValue.ActiveDispatches[dispatchID]; active {
		t.Fatal("interrupted dispatch remains active")
	}
	if got := reducer.tokenPlaces[live.ID]; got != "task:processing" {
		t.Fatalf("re-armed live Work place = %q, want task:processing", got)
	}
	if got := reducer.tokenPlaces[fallback.ID]; got != "task:ready" {
		t.Fatalf("fallback Work place = %q, want task:ready", got)
	}
	if got := reducer.tokenPlaces["gpu-token"]; got != "gpu:held" {
		t.Fatalf("explicit resource place = %q, want gpu:held", got)
	}
	if got := reducer.tokenPlaces["cpu-token"]; got != "cpu:available" {
		t.Fatalf("fallback resource place = %q, want cpu:available", got)
	}
	if _, present := reducer.tokenPlaces[terminal.ID]; present {
		t.Fatalf("terminal Work was re-armed: %#v", reducer.tokenPlaces)
	}
	if _, present := reducer.tokenPlaces[failed.ID]; present {
		t.Fatalf("failed Work was re-armed: %#v", reducer.tokenPlaces)
	}
	if _, present := reducer.tokenPlaces[noPlace.ID]; present {
		t.Fatalf("Work without a logical place was re-armed: %#v", reducer.tokenPlaces)
	}
	if reducer.stateValue.ActiveWorkItemsByID[live.ID].ID != live.ID ||
		reducer.stateValue.ActiveWorkItemsByID[fallback.ID].ID != fallback.ID {
		t.Fatalf("active Work after interruption = %#v", reducer.stateValue.ActiveWorkItemsByID)
	}
	if got := reducer.stateValue.JavaScriptRuntime.Dispatches[0].Status; got != string(interfaces.FactoryDispatchStatusInterrupted) {
		t.Fatalf("interrupted dispatch projection status = %q, want %q", got, interfaces.FactoryDispatchStatusInterrupted)
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
