package projections

import (
	"reflect"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestReconstructFactoryWorldState_AppliesCanonicalEventsByTick(t *testing.T) {
	t0 := time.Date(2026, 4, 16, 8, 0, 0, 0, time.UTC)
	state, err := ReconstructFactoryWorldState(canonicalCompletedDispatchProjectionEvents(t0), 3)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState: %v", err)
	}
	assertCanonicalCompletedDispatchState(t, state)
}

func TestReconstructFactoryWorldState_SeedsTopologyFromRunRequestBeforeInitialStructure(t *testing.T) {
	t0 := time.Date(2026, 4, 21, 12, 0, 0, 0, time.UTC)
	state, err := ReconstructFactoryWorldState([]factoryapi.FactoryEvent{runRequestEvent(t0)}, 0)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState: %v", err)
	}

	if len(state.Topology.WorkTypes) != 1 || state.Topology.WorkTypes[0].ID != "task" {
		t.Fatalf("work types = %#v, want task topology from RUN_REQUEST", state.Topology.WorkTypes)
	}
	if len(state.Topology.Workstations) != 1 || state.Topology.Workstations[0].ID != "t-review" {
		t.Fatalf("workstations = %#v, want review topology from RUN_REQUEST", state.Topology.Workstations)
	}
	if got := state.PlaceOccupancyByID["agent-slot:available"].TokenCount; got != 2 {
		t.Fatalf("agent-slot:available token count = %d, want 2 seeded from RUN_REQUEST", got)
	}
}

func TestReconstructFactoryWorldState_InitialStructurePreservesNonSuccessRouteArrays(t *testing.T) {
	t0 := time.Date(2026, 4, 21, 12, 0, 0, 0, time.UTC)
	state, err := ReconstructFactoryWorldState([]factoryapi.FactoryEvent{initialStructureEventWithNonSuccessRouteArrays(t0)}, 0)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState: %v", err)
	}
	if len(state.Topology.Workstations) != 1 {
		t.Fatalf("topology workstations = %#v, want one projected workstation", state.Topology.Workstations)
	}

	workstation := state.Topology.Workstations[0]
	if !reflect.DeepEqual(workstation.ContinuePlaceIDs, []string{"task:retry", "task:init"}) {
		t.Fatalf("continue routes = %#v, want authored order", workstation.ContinuePlaceIDs)
	}
	if !reflect.DeepEqual(workstation.RejectionPlaceIDs, []string{"task:triage", "task:init"}) {
		t.Fatalf("rejection routes = %#v, want authored order", workstation.RejectionPlaceIDs)
	}
	if !reflect.DeepEqual(workstation.FailurePlaceIDs, []string{"task:failed", "task:abandoned"}) {
		t.Fatalf("failure routes = %#v, want authored order", workstation.FailurePlaceIDs)
	}
}

func TestReconstructFactoryWorldState_FactoryChangeReplacesProjectedTopology(t *testing.T) {
	t0 := time.Date(2026, 4, 22, 9, 0, 0, 0, time.UTC)
	events := []factoryapi.FactoryEvent{
		initialStructureEvent(t0),
		factoryChangeEvent(t0.Add(time.Second)),
	}

	state, err := ReconstructFactoryWorldState(events, 1)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState: %v", err)
	}

	if len(state.Topology.WorkTypes) != 1 || state.Topology.WorkTypes[0].ID != "story" {
		t.Fatalf("work types after factory-change = %#v, want story replacement topology", state.Topology.WorkTypes)
	}
	if len(state.Topology.Workstations) != 1 || state.Topology.Workstations[0].ID != "t-plan" {
		t.Fatalf("workstations after factory-change = %#v, want t-plan replacement topology", state.Topology.Workstations)
	}
	if _, ok := state.PlaceOccupancyByID["task:init"]; ok {
		t.Fatalf("task:init occupancy = %#v, want old topology removed after factory-change", state.PlaceOccupancyByID["task:init"])
	}
}

func TestReconstructFactoryWorldState_ActiveRequestAtSelectedTick(t *testing.T) {
	t0 := time.Date(2026, 4, 16, 8, 0, 0, 0, time.UTC)
	events := []factoryapi.FactoryEvent{
		initialStructureEvent(t0),
		workInputEventWithToken(1, t0.Add(time.Second), "tok-task-1", interfaces.FactoryWorkItem{ID: "work-1", WorkTypeID: "task", TraceID: "trace-1", PlaceID: "task:init"}),
		workstationRequestEvent(2, t0.Add(2*time.Second), interfaces.WorkstationRequestPayload{
			DispatchID:   "dispatch-1",
			TransitionID: "t-review",
			Workstation:  interfaces.FactoryWorkstationRef{ID: "t-review", Name: "Review"},
			Inputs: []interfaces.WorkstationInput{{
				TokenID:  "tok-task-1",
				PlaceID:  "task:init",
				WorkItem: &interfaces.FactoryWorkItem{ID: "work-1", WorkTypeID: "task", TraceID: "trace-1", PlaceID: "task:init"},
			}},
		}),
	}

	state, err := ReconstructFactoryWorldState(events, 2)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState: %v", err)
	}

	if len(state.ActiveDispatches) != 1 {
		t.Fatalf("active dispatches = %d, want 1", len(state.ActiveDispatches))
	}
	dispatch := state.ActiveDispatches["dispatch-1"]
	if dispatch.StartedTick != 2 || len(dispatch.WorkItemIDs) != 1 || dispatch.WorkItemIDs[0] != "work-1" {
		t.Fatalf("active dispatch = %#v, want work-1 at tick 2", dispatch)
	}
	if got, ok := state.PlaceOccupancyByID["task:init"]; ok {
		t.Fatalf("task:init occupancy = %#v, want no occupancy after request consumed runtime token", got)
	}
	if _, ok := state.ActiveWorkItemsByID["work-1"]; !ok {
		t.Fatalf("work-1 should remain active while dispatch is in flight")
	}
}

func TestReconstructFactoryWorldState_PreservesExplicitDispatchChainingLineage(t *testing.T) {
	events := chainingTraceProjectionEvents()
	activeState, err := ReconstructFactoryWorldState(events, 2)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState active tick: %v", err)
	}
	assertChainingTraceProjectionActiveState(t, activeState)

	completedState, err := ReconstructFactoryWorldState(events, 3)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState completed tick: %v", err)
	}
	assertChainingTraceProjectionCompletedState(t, completedState)
}

func TestReconstructFactoryWorldState_FallsBackToPayloadDispatchChainingLineageForLegacyEvents(t *testing.T) {
	t0 := time.Date(2026, 4, 22, 13, 0, 0, 0, time.UTC)
	events := []factoryapi.FactoryEvent{
		initialStructureEvent(t0),
		workInputEvent(1, t0.Add(time.Second), interfaces.FactoryWorkItem{
			ID:                     "work-1",
			WorkTypeID:             "task",
			DisplayName:            "Input",
			CurrentChainingTraceID: "chain-input",
			TraceID:                "trace-input",
			PlaceID:                "task:init",
		}),
		generatedProjectionEvent(
			factoryapi.FactoryEventTypeDispatchRequest,
			"request/dispatch-legacy",
			2,
			t0.Add(2*time.Second),
			factoryapi.FactoryEventContext{
				DispatchId: stringPtrForProjectionTest("dispatch-legacy"),
				TraceIds:   stringSlicePtrForProjectionTest([]string{"trace-input"}),
				WorkIds:    stringSlicePtrForProjectionTest([]string{"work-1"}),
			},
			factoryapi.DispatchRequestEventPayload{
				TransitionId:             "t-review",
				CurrentChainingTraceId:   stringPtrForProjectionTest("payload-current"),
				PreviousChainingTraceIds: stringSlicePtrForProjectionTest([]string{"payload-a", "payload-z"}),
				Inputs:                   []factoryapi.DispatchConsumedWorkRef{{WorkId: "work-1"}},
			},
		),
	}

	state, err := ReconstructFactoryWorldState(events, 2)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState: %v", err)
	}

	dispatch := state.ActiveDispatches["dispatch-legacy"]
	if dispatch.CurrentChainingTraceID != "payload-current" {
		t.Fatalf("active dispatch current chaining trace ID = %q, want payload-current", dispatch.CurrentChainingTraceID)
	}
	if got := dispatch.PreviousChainingTraceIDs; len(got) != 2 || got[0] != "payload-a" || got[1] != "payload-z" {
		t.Fatalf("active dispatch previous chaining trace IDs = %#v, want [payload-a payload-z]", got)
	}
}

func TestReconstructFactoryWorldState_RetainsInferenceAttemptsByDispatchID(t *testing.T) {
	t0 := time.Date(2026, 4, 18, 12, 0, 0, 0, time.UTC)
	events := []factoryapi.FactoryEvent{
		initialStructureEvent(t0),
		workInputEventWithToken(1, t0.Add(time.Second), "tok-task-1", interfaces.FactoryWorkItem{ID: "work-1", WorkTypeID: "task", TraceID: "trace-1", PlaceID: "task:init"}),
		workstationRequestEvent(2, t0.Add(2*time.Second), interfaces.WorkstationRequestPayload{
			DispatchID:   "dispatch-1",
			TransitionID: "t-review",
			Workstation:  interfaces.FactoryWorkstationRef{ID: "t-review", Name: "Review"},
			Inputs: []interfaces.WorkstationInput{{
				TokenID:  "tok-task-1",
				PlaceID:  "task:init",
				WorkItem: &interfaces.FactoryWorkItem{ID: "work-1", WorkTypeID: "task", TraceID: "trace-1", PlaceID: "task:init"},
			}},
		}),
		inferenceRequestEvent(3, t0.Add(3*time.Second), factoryapi.InferenceRequestEventPayload{
			InferenceRequestId: "dispatch-1/inference-request/1",
			Attempt:            1,
			WorkingDirectory:   "/work/project",
			Worktree:           "/work/project/.worktrees/story",
			Prompt:             "Summarize the current story.",
		}),
		inferenceResponseEvent(4, t0.Add(4*time.Second), factoryapi.InferenceResponseEventPayload{
			InferenceRequestId: "dispatch-1/inference-request/1",
			Attempt:            1,
			Outcome:            factoryapi.InferenceOutcomeSucceeded,
			Response:           stringPtrForProjectionTest("Story is ready for review."),
			DurationMillis:     1250,
		}),
	}

	pendingState, err := ReconstructFactoryWorldState(events, 3)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState pending tick: %v", err)
	}
	pendingAttempt := pendingState.InferenceAttemptsByDispatchID["dispatch-1"]["dispatch-1/inference-request/1"]
	if pendingAttempt.InferenceRequestID == "" || pendingAttempt.Outcome != "" {
		t.Fatalf("pending inference attempt = %#v, want request fields without outcome", pendingAttempt)
	}
	if pendingAttempt.Prompt != "Summarize the current story." || pendingAttempt.RequestTime.IsZero() {
		t.Fatalf("pending inference request fields = %#v, want prompt and request time", pendingAttempt)
	}

	completedState, err := ReconstructFactoryWorldState(events, 4)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState completed tick: %v", err)
	}
	completedAttempt := completedState.InferenceAttemptsByDispatchID["dispatch-1"]["dispatch-1/inference-request/1"]
	if completedAttempt.Outcome != string(factoryapi.InferenceOutcomeSucceeded) ||
		completedAttempt.Response != "Story is ready for review." ||
		completedAttempt.DurationMillis != 1250 ||
		completedAttempt.ResponseTime.IsZero() {
		t.Fatalf("completed inference attempt = %#v, want response details", completedAttempt)
	}
}

func TestReconstructFactoryWorldState_RetainsScriptAttemptsByDispatchID(t *testing.T) {
	t0 := time.Date(2026, 4, 22, 12, 0, 0, 0, time.UTC)
	events := []factoryapi.FactoryEvent{
		initialStructureEvent(t0),
		workInputEventWithToken(1, t0.Add(time.Second), "tok-task-1", interfaces.FactoryWorkItem{ID: "work-1", WorkTypeID: "task", TraceID: "trace-1", PlaceID: "task:init"}),
		workstationRequestEvent(2, t0.Add(2*time.Second), interfaces.WorkstationRequestPayload{
			DispatchID:   "dispatch-script-1",
			TransitionID: "t-review",
			Workstation:  interfaces.FactoryWorkstationRef{ID: "t-review", Name: "Review"},
			Inputs: []interfaces.WorkstationInput{{
				TokenID:  "tok-task-1",
				PlaceID:  "task:init",
				WorkItem: &interfaces.FactoryWorkItem{ID: "work-1", WorkTypeID: "task", TraceID: "trace-1", PlaceID: "task:init"},
			}},
		}),
		scriptRequestEvent(3, t0.Add(3*time.Second), factoryapi.ScriptRequestEventPayload{
			Args:            []string{"--work", "work-1", "--project", "docs"},
			Attempt:         1,
			Command:         "script-tool",
			DispatchId:      "dispatch-script-1",
			ScriptRequestId: "dispatch-script-1/script-request/1",
			TransitionId:    "t-review",
		}),
		scriptResponseEvent(4, t0.Add(4*time.Second), factoryapi.ScriptResponseEventPayload{
			Attempt:         1,
			DispatchId:      "dispatch-script-1",
			DurationMillis:  238,
			ExitCode:        intPtrForProjectionTest(3),
			Outcome:         factoryapi.ScriptExecutionOutcomeFailedExitCode,
			ScriptRequestId: "dispatch-script-1/script-request/1",
			Stderr:          "script stderr\n",
			Stdout:          "script stdout\n",
			TransitionId:    "t-review",
		}),
	}

	pendingState, err := ReconstructFactoryWorldState(events, 3)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState pending tick: %v", err)
	}
	pendingRequest := pendingState.ScriptRequestsByDispatchID["dispatch-script-1"]["dispatch-script-1/script-request/1"]
	if pendingRequest.ScriptRequestID == "" || pendingRequest.Command != "script-tool" {
		t.Fatalf("pending script request = %#v, want retained request fields", pendingRequest)
	}
	if len(pendingRequest.Args) != 4 || pendingRequest.RequestTime.IsZero() {
		t.Fatalf("pending script request fields = %#v, want args and request time", pendingRequest)
	}
	if len(pendingState.ScriptResponsesByDispatchID["dispatch-script-1"]) != 0 {
		t.Fatalf("pending script responses = %#v, want none before response tick", pendingState.ScriptResponsesByDispatchID["dispatch-script-1"])
	}

	completedState, err := ReconstructFactoryWorldState(events, 4)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState completed tick: %v", err)
	}
	completedResponse := completedState.ScriptResponsesByDispatchID["dispatch-script-1"]["dispatch-script-1/script-request/1"]
	if completedResponse.Outcome != string(factoryapi.ScriptExecutionOutcomeFailedExitCode) ||
		completedResponse.Stdout != "script stdout\n" ||
		completedResponse.Stderr != "script stderr\n" ||
		completedResponse.DurationMillis != 238 ||
		completedResponse.ExitCode == nil || *completedResponse.ExitCode != 3 ||
		completedResponse.ResponseTime.IsZero() {
		t.Fatalf("completed script response = %#v, want response details", completedResponse)
	}
}

func TestFactoryWorldReducer_RemoveTokenCleansWorkIndexes(t *testing.T) {
	reducer := newFactoryWorldReducer(0)
	firstItem := interfaces.FactoryWorkItem{ID: "work-1", WorkTypeID: "task", TraceID: "trace-1", PlaceID: "task:init"}
	secondItem := interfaces.FactoryWorkItem{ID: "work-2", WorkTypeID: "task", TraceID: "trace-2", PlaceID: "task:init"}

	reducer.addWorkToken("tok-work-1", "task:init", firstItem)
	reducer.addWorkToken("tok-work-2", "task:init", secondItem)

	reducer.removeToken("tok-work-1")

	if _, ok := reducer.tokenPlaces["tok-work-1"]; ok {
		t.Fatalf("token place for removed work token should be deleted")
	}
	if _, ok := reducer.tokenKinds["tok-work-1"]; ok {
		t.Fatalf("token kind for removed work token should be deleted")
	}
	if _, ok := reducer.tokenWorkIDs["tok-work-1"]; ok {
		t.Fatalf("token work ID for removed work token should be deleted")
	}
	if len(reducer.placeTokens["task:init"]) != 1 {
		t.Fatalf("task:init token count = %d, want 1 remaining token", len(reducer.placeTokens["task:init"]))
	}
	if _, ok := reducer.placeTokens["task:init"]["tok-work-2"]; !ok {
		t.Fatalf("task:init should retain tok-work-2 after removing tok-work-1")
	}

	reducer.removeToken("tok-work-2")

	if _, ok := reducer.placeTokens["task:init"]; ok {
		t.Fatalf("task:init place index should be deleted after final work token removal")
	}
}

func TestFactoryWorldReducer_RemoveTokenCleansResourceIndexes(t *testing.T) {
	reducer := newFactoryWorldReducer(0)
	resource := interfaces.FactoryResource{ID: "agent-slot", Capacity: 1}

	reducer.seedResourceTokens(resource)

	tokenID := resourceTokenID(resource.ID, 0)
	reducer.removeToken(tokenID)

	if _, ok := reducer.tokenPlaces[tokenID]; ok {
		t.Fatalf("token place for removed resource token should be deleted")
	}
	if _, ok := reducer.tokenKinds[tokenID]; ok {
		t.Fatalf("token kind for removed resource token should be deleted")
	}
	if _, ok := reducer.placeTokens[resourceAvailablePlaceID(resource.ID)]; ok {
		t.Fatalf("resource available place index should be deleted after final token removal")
	}
}

func TestReconstructFactoryWorldState_PreservesSafeResponseDiagnostics(t *testing.T) {
	t0 := time.Date(2026, 4, 18, 10, 0, 0, 0, time.UTC)
	state, err := ReconstructFactoryWorldState(safeResponseDiagnosticsProjectionEvents(t0), 3)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState: %v", err)
	}
	assertSafeResponseDiagnosticsState(t, state)
}

// pkgmaintcheck:ignore-cyclomatic-complexity this reducer regression test keeps the safe inference diagnostics matrix inline to preserve world-state intent.
func TestReconstructFactoryWorldState_PreservesSafeInferenceAttemptDiagnostics(t *testing.T) {
	t0 := time.Date(2026, 4, 18, 10, 0, 0, 0, time.UTC)
	state, err := ReconstructFactoryWorldState(safeResponseDiagnosticsProjectionEvents(t0), 3)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState: %v", err)
	}

	attempts := state.InferenceAttemptsByDispatchID["dispatch-1"]
	if len(attempts) != 1 {
		t.Fatalf("inference attempts = %#v, want one attempt for dispatch-1", attempts)
	}
	attempt := attempts["dispatch-1/inference-request/1"]
	if attempt.ProviderSession == nil || attempt.ProviderSession.ID != "resp-1" {
		t.Fatalf("attempt provider session = %#v, want resp-1", attempt.ProviderSession)
	}
	if attempt.Diagnostics == nil || attempt.Diagnostics.Provider == nil || attempt.Diagnostics.RenderedPrompt == nil {
		t.Fatalf("attempt diagnostics = %#v, want provider and rendered prompt", attempt.Diagnostics)
	}
	if attempt.Diagnostics.Provider.Provider != "codex" || attempt.Diagnostics.Provider.Model != "gpt-5.4" {
		t.Fatalf("attempt provider diagnostics = %#v, want codex/gpt-5.4", attempt.Diagnostics.Provider)
	}
	if got := attempt.Diagnostics.Provider.RequestMetadata["worker_type"]; got != "builder" {
		t.Fatalf("attempt request metadata worker_type = %q, want builder", got)
	}
	if got := attempt.Diagnostics.Provider.ResponseMetadata["provider_session_id"]; got != "resp-1" {
		t.Fatalf("attempt response metadata provider_session_id = %q, want resp-1", got)
	}
	if got := attempt.Diagnostics.RenderedPrompt.Variables["work_type_name"]; got != "task" {
		t.Fatalf("attempt rendered prompt work_type_name = %q, want task", got)
	}
	if len(attempt.Diagnostics.RenderedPrompt.Variables) != 2 {
		t.Fatalf("attempt rendered prompt variables = %#v, want only safe allowlisted keys", attempt.Diagnostics.RenderedPrompt.Variables)
	}
	if len(attempt.Diagnostics.Provider.RequestMetadata) != 1 {
		t.Fatalf("attempt request metadata = %#v, want only allowlisted keys", attempt.Diagnostics.Provider.RequestMetadata)
	}
	if len(attempt.Diagnostics.Provider.ResponseMetadata) != 2 {
		t.Fatalf("attempt response metadata = %#v, want only allowlisted keys", attempt.Diagnostics.Provider.ResponseMetadata)
	}
}

func TestReconstructFactoryWorldState_PreservesCanonicalProviderMetadata(t *testing.T) {
	t0 := time.Date(2026, 4, 18, 10, 30, 0, 0, time.UTC)
	events := []factoryapi.FactoryEvent{
		initialStructureEvent(t0),
		workInputEventWithToken(1, t0.Add(time.Second), "tok-task-1", interfaces.FactoryWorkItem{ID: "work-1", WorkTypeID: "task", TraceID: "trace-1", PlaceID: "task:init"}),
		workstationRequestEvent(2, t0.Add(2*time.Second), interfaces.WorkstationRequestPayload{
			DispatchID:   "dispatch-1",
			TransitionID: "t-review",
			Workstation:  interfaces.FactoryWorkstationRef{ID: "t-review", Name: "Review"},
			Inputs: []interfaces.WorkstationInput{{
				TokenID:  "tok-task-1",
				PlaceID:  "task:init",
				WorkItem: &interfaces.FactoryWorkItem{ID: "work-1", WorkTypeID: "task", TraceID: "trace-1", PlaceID: "task:init"},
			}},
		}),
		inferenceResponseEvent(3, t0.Add(2500*time.Millisecond), factoryapi.InferenceResponseEventPayload{
			InferenceRequestId: "dispatch-1/inference-request/1",
			Attempt:            1,
			Outcome:            factoryapi.InferenceOutcomeFailed,
			DurationMillis:     900,
			ErrorClass:         stringPtrForProjectionTest(string(interfaces.ProviderErrorTypeTimeout)),
			ProviderSession: generatedProviderSessionForProjectionTest(&interfaces.ProviderSessionMetadata{
				Provider: "codex",
				Kind:     "session_id",
				ID:       "sess-1",
			}),
		}),
		workstationResponseEvent(3, t0.Add(3*time.Second), interfaces.WorkstationResponsePayload{
			DispatchID:     "dispatch-1",
			TransitionID:   "t-review",
			Workstation:    interfaces.FactoryWorkstationRef{ID: "t-review", Name: "Review"},
			Result:         interfaces.WorkstationResult{Outcome: "FAILED", ProviderFailure: &interfaces.ProviderFailureMetadata{Family: interfaces.ProviderErrorFamilyRetryable, Type: interfaces.ProviderErrorTypeTimeout}},
			DurationMillis: 900,
			TraceData:      &interfaces.FactoryTraceData{TraceID: "trace-1", WorkIDs: []string{"work-1"}},
			ProviderSession: &interfaces.ProviderSessionMetadata{
				Provider: "codex",
				Kind:     "session_id",
				ID:       "sess-1",
			},
		}),
	}

	state, err := ReconstructFactoryWorldState(events, 3)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState: %v", err)
	}

	if len(state.CompletedDispatches) != 1 {
		t.Fatalf("completed dispatches = %#v, want 1 completion", state.CompletedDispatches)
	}
	completion := state.CompletedDispatches[0]
	if completion.ProviderSession == nil || completion.ProviderSession.ID != "sess-1" {
		t.Fatalf("completion provider session = %#v, want sess-1", completion.ProviderSession)
	}
	if completion.Result.ProviderFailure == nil {
		t.Fatal("completion provider failure is nil, want canonical metadata")
	}
	if completion.Result.ProviderFailure.Family != interfaces.ProviderErrorFamilyRetryable ||
		completion.Result.ProviderFailure.Type != interfaces.ProviderErrorTypeTimeout {
		t.Fatalf("completion provider failure = %#v, want retryable/timeout", completion.Result.ProviderFailure)
	}
	if len(state.ProviderSessions) != 1 || state.ProviderSessions[0].ProviderSession.ID != "sess-1" {
		t.Fatalf("provider sessions = %#v, want sess-1", state.ProviderSessions)
	}
}

func TestFactoryWorldReducer_DetachesCompletedConsumedInputsFromDispatchSource(t *testing.T) {
	t0 := time.Date(2026, 5, 19, 9, 0, 0, 0, time.UTC)
	input := interfaces.FactoryWorkItem{
		ID:                       "work-1",
		WorkTypeID:               "task",
		DisplayName:              "Draft",
		TraceID:                  "trace-1",
		PlaceID:                  "task:init",
		PreviousChainingTraceIDs: []string{"chain-a", "chain-b"},
		Tags:                     map[string]string{"priority": "high"},
	}
	request := workstationRequestEvent(2, t0.Add(2*time.Second), interfaces.WorkstationRequestPayload{
		DispatchID:   "dispatch-1",
		TransitionID: "t-review",
		Workstation:  interfaces.FactoryWorkstationRef{ID: "t-review", Name: "Review"},
		Inputs: []interfaces.WorkstationInput{{
			TokenID:  "tok-task-1",
			PlaceID:  "task:init",
			WorkItem: &input,
		}},
	})
	response := workstationResponseEvent(3, t0.Add(3*time.Second), interfaces.WorkstationResponsePayload{
		DispatchID:      "dispatch-1",
		TransitionID:    "t-review",
		Workstation:     interfaces.FactoryWorkstationRef{ID: "t-review", Name: "Review"},
		Result:          interfaces.WorkstationResult{Outcome: "ACCEPTED"},
		DurationMillis:  800,
		TraceData:       &interfaces.FactoryTraceData{TraceID: "trace-1", WorkIDs: []string{"work-1"}},
		ProviderSession: &interfaces.ProviderSessionMetadata{Provider: "codex", Kind: "session_id", ID: "sess-1"},
	})
	inference := inferenceResponseEvent(3, t0.Add(2500*time.Millisecond), factoryapi.InferenceResponseEventPayload{
		InferenceRequestId: "dispatch-1/inference-request/1",
		Attempt:            1,
		Outcome:            factoryapi.InferenceOutcomeSucceeded,
		DurationMillis:     700,
		ProviderSession:    generatedProviderSessionForProjectionTest(&interfaces.ProviderSessionMetadata{Provider: "codex", Kind: "session_id", ID: "sess-1"}),
	})

	reducer := newFactoryWorldReducer(3)
	for _, event := range []factoryapi.FactoryEvent{
		initialStructureEvent(t0),
		workInputEventWithToken(1, t0.Add(time.Second), "tok-task-1", input),
		request,
	} {
		if err := reducer.apply(event); err != nil {
			t.Fatalf("apply pre-completion event %q: %v", event.Type, err)
		}
	}

	dispatchSource := reducer.stateValue.ActiveDispatches["dispatch-1"]
	if len(dispatchSource.Inputs) != 1 || dispatchSource.Inputs[0].WorkItem == nil {
		t.Fatalf("dispatch source inputs = %#v, want one traced work item", dispatchSource.Inputs)
	}

	if err := reducer.apply(inference); err != nil {
		t.Fatalf("apply inference event: %v", err)
	}
	if err := reducer.apply(response); err != nil {
		t.Fatalf("apply response event: %v", err)
	}

	dispatchSource.Inputs[0].WorkItem.PreviousChainingTraceIDs[0] = "chain-z"
	dispatchSource.Inputs[0].WorkItem.Tags["priority"] = "low"

	state := reducer.state()
	if len(state.CompletedDispatches) != 1 {
		t.Fatalf("completed dispatches = %#v, want one completion", state.CompletedDispatches)
	}
	if len(state.ProviderSessions) != 1 {
		t.Fatalf("provider sessions = %#v, want one provider session", state.ProviderSessions)
	}

	completionInput := state.CompletedDispatches[0].ConsumedInputs[0].WorkItem
	if len(completionInput.PreviousChainingTraceIDs) != 2 || completionInput.PreviousChainingTraceIDs[0] != "chain-a" {
		t.Fatalf("completed consumed input previous chaining trace IDs = %#v, want [chain-a chain-b]", completionInput.PreviousChainingTraceIDs)
	}
	if completionInput.Tags["priority"] != "high" {
		t.Fatalf("completed consumed input tags = %#v, want priority high", completionInput.Tags)
	}

	providerInput := state.ProviderSessions[0].ConsumedInputs[0].WorkItem
	if len(providerInput.PreviousChainingTraceIDs) != 2 || providerInput.PreviousChainingTraceIDs[0] != "chain-a" {
		t.Fatalf("provider-session consumed input previous chaining trace IDs = %#v, want [chain-a chain-b]", providerInput.PreviousChainingTraceIDs)
	}
	if providerInput.Tags["priority"] != "high" {
		t.Fatalf("provider-session consumed input tags = %#v, want priority high", providerInput.Tags)
	}
}

func TestReconstructFactoryWorldState_WorkInputTokenIDMatchesRequestConsumption(t *testing.T) {
	t0 := time.Date(2026, 4, 16, 8, 0, 0, 0, time.UTC)
	item := interfaces.FactoryWorkItem{ID: "work-1", WorkTypeID: "task", TraceID: "trace-1", PlaceID: "task:init"}
	events := []factoryapi.FactoryEvent{
		initialStructureEvent(t0),
		workInputEventWithToken(1, t0.Add(time.Second), "tok-task-1", item),
		workstationRequestEvent(2, t0.Add(2*time.Second), interfaces.WorkstationRequestPayload{
			DispatchID:   "dispatch-1",
			TransitionID: "t-review",
			Workstation:  interfaces.FactoryWorkstationRef{ID: "t-review", Name: "Review"},
			Inputs: []interfaces.WorkstationInput{{
				TokenID:  "tok-task-1",
				PlaceID:  "task:init",
				WorkItem: &item,
			}},
		}),
	}

	submitted, err := ReconstructFactoryWorldState(events, 1)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState submitted tick: %v", err)
	}
	if got := submitted.PlaceOccupancyByID["task:init"].TokenCount; got != 1 {
		t.Fatalf("submitted task:init token count = %d, want 1", got)
	}
	if got := submitted.PlaceOccupancyByID["task:init"].WorkItemIDs; len(got) != 1 || got[0] != "work-1" {
		t.Fatalf("submitted task:init work IDs = %#v, want work-1", got)
	}

	active, err := ReconstructFactoryWorldState(events, 2)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState active tick: %v", err)
	}
	if got, ok := active.PlaceOccupancyByID["task:init"]; ok {
		t.Fatalf("active task:init occupancy = %#v, want consumed runtime token removed", got)
	}
	if len(active.ActiveDispatches) != 1 || active.ActiveDispatches["dispatch-1"].WorkItemIDs[0] != "work-1" {
		t.Fatalf("active dispatches = %#v, want dispatch-1 to retain work-1 while task:init occupancy is cleared", active.ActiveDispatches)
	}
}

func TestReconstructFactoryWorldState_ResolvesBatchRelationSourcesByWorkName(t *testing.T) {
	t0 := time.Date(2026, 4, 20, 20, 0, 0, 0, time.UTC)
	requestID := "request-parent-child"
	works := []factoryapi.Work{
		generatedWorkForProjectionTest(interfaces.FactoryWorkItem{ID: "work-parent", WorkTypeID: "task", DisplayName: "parent", TraceID: "trace-parent-child"}, requestID),
		generatedWorkForProjectionTest(interfaces.FactoryWorkItem{ID: "work-prerequisite", WorkTypeID: "task", DisplayName: "prerequisite", TraceID: "trace-parent-child"}, requestID),
		generatedWorkForProjectionTest(interfaces.FactoryWorkItem{ID: "work-child", WorkTypeID: "task", DisplayName: "child", TraceID: "trace-parent-child"}, requestID),
	}
	relations := []factoryapi.Relation{
		{
			Type:           factoryapi.RelationTypeParentChild,
			SourceWorkName: "child",
			TargetWorkName: "parent",
			TargetWorkId:   stringPtrForProjectionTest("work-parent"),
		},
		{
			Type:           factoryapi.RelationTypeDependsOn,
			SourceWorkName: "child",
			TargetWorkName: "prerequisite",
			TargetWorkId:   stringPtrForProjectionTest("work-prerequisite"),
			RequiredState:  stringPtrForProjectionTest("complete"),
		},
	}
	events := []factoryapi.FactoryEvent{
		initialStructureEvent(t0),
		generatedProjectionEvent(factoryapi.FactoryEventTypeWorkRequest, "work-request/request-parent-child", 1, t0.Add(time.Second), factoryapi.FactoryEventContext{
			RequestId: stringPtrForProjectionTest(requestID),
			TraceIds:  &[]string{"trace-parent-child"},
			WorkIds:   &[]string{"work-parent", "work-prerequisite", "work-child"},
		}, factoryapi.WorkRequestEventPayload{
			Type:      factoryapi.WorkRequestTypeFactoryRequestBatch,
			Works:     &works,
			Relations: &relations,
		}),
		relationshipChangeEvent(1, t0.Add(2*time.Second), requestID, "trace-parent-child", []string{"work-child", "work-parent"}, relations[0]),
		relationshipChangeEvent(1, t0.Add(3*time.Second), requestID, "trace-parent-child", []string{"work-child", "work-prerequisite"}, relations[1]),
	}

	state, err := ReconstructFactoryWorldState(events, 1)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState: %v", err)
	}

	childRelations := state.RelationsByWorkID["work-child"]
	if len(childRelations) != 2 {
		t.Fatalf("child relations = %#v, want 2 relations on work-child", childRelations)
	}
	if got := childRelations[0]; got.Type != string(factoryapi.RelationTypeParentChild) || got.TargetWorkID != "work-parent" {
		t.Fatalf("first child relation = %#v, want parent-child -> work-parent", got)
	}
	if got := childRelations[1]; got.Type != string(factoryapi.RelationTypeDependsOn) || got.TargetWorkID != "work-prerequisite" || got.RequiredState != "complete" {
		t.Fatalf("second child relation = %#v, want depends_on -> work-prerequisite in complete", got)
	}
	if got := state.RelationsByWorkID["work-parent"]; len(got) != 0 {
		t.Fatalf("parent relations = %#v, want no source relations on work-parent", got)
	}
	if got := state.RelationsByWorkID["work-prerequisite"]; len(got) != 0 {
		t.Fatalf("prerequisite relations = %#v, want no source relations on work-prerequisite", got)
	}
}

func TestReconstructFactoryWorldState_FailedAndRejectedResponses(t *testing.T) {
	t0 := time.Date(2026, 4, 16, 8, 0, 0, 0, time.UTC)
	events := []factoryapi.FactoryEvent{
		initialStructureEvent(t0),
		workInputEvent(1, t0.Add(time.Second), interfaces.FactoryWorkItem{ID: "work-failed", WorkTypeID: "task", TraceID: "trace-failed", PlaceID: "task:init"}),
		workstationRequestEvent(2, t0.Add(2*time.Second), interfaces.WorkstationRequestPayload{
			DispatchID:   "dispatch-failed",
			TransitionID: "t-review",
			Workstation:  interfaces.FactoryWorkstationRef{ID: "t-review"},
			Inputs: []interfaces.WorkstationInput{{
				TokenID:  "work-failed",
				PlaceID:  "task:init",
				WorkItem: &interfaces.FactoryWorkItem{ID: "work-failed", WorkTypeID: "task", TraceID: "trace-failed", PlaceID: "task:init"},
			}},
		}),
		workstationResponseEvent(3, t0.Add(3*time.Second), interfaces.WorkstationResponsePayload{
			DispatchID:   "dispatch-failed",
			TransitionID: "t-review",
			Workstation:  interfaces.FactoryWorkstationRef{ID: "t-review"},
			Result:       interfaces.WorkstationResult{Outcome: "FAILED", Error: "boom"},
			TraceData:    &interfaces.FactoryTraceData{TraceID: "trace-failed", WorkIDs: []string{"work-failed"}},
		}),
		workInputEvent(4, t0.Add(4*time.Second), interfaces.FactoryWorkItem{ID: "work-rejected", WorkTypeID: "task", TraceID: "trace-rejected", PlaceID: "task:init"}),
		workstationRequestEvent(5, t0.Add(5*time.Second), interfaces.WorkstationRequestPayload{
			DispatchID:   "dispatch-rejected",
			TransitionID: "t-review",
			Workstation:  interfaces.FactoryWorkstationRef{ID: "t-review"},
			Inputs: []interfaces.WorkstationInput{{
				TokenID:  "work-rejected",
				PlaceID:  "task:init",
				WorkItem: &interfaces.FactoryWorkItem{ID: "work-rejected", WorkTypeID: "task", TraceID: "trace-rejected", PlaceID: "task:init"},
			}},
		}),
		workstationResponseEvent(6, t0.Add(6*time.Second), interfaces.WorkstationResponsePayload{
			DispatchID:   "dispatch-rejected",
			TransitionID: "t-review",
			Workstation:  interfaces.FactoryWorkstationRef{ID: "t-review"},
			Result:       interfaces.WorkstationResult{Outcome: "REJECTED", Feedback: "retry"},
			Outputs: []interfaces.WorkstationOutput{{
				Type:    string(interfaces.MutationMove),
				TokenID: "work-rejected",
				ToPlace: "task:init",
				WorkItem: &interfaces.FactoryWorkItem{
					ID:         "work-rejected",
					WorkTypeID: "task",
					TraceID:    "trace-rejected",
					PlaceID:    "task:init",
				},
			}},
			TraceData: &interfaces.FactoryTraceData{TraceID: "trace-rejected", WorkIDs: []string{"work-rejected"}},
		}),
	}

	state, err := ReconstructFactoryWorldState(events, 6)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState: %v", err)
	}

	if len(state.FailedDispatches) != 1 || state.FailedDispatches[0].DispatchID != "dispatch-failed" {
		t.Fatalf("failed dispatches = %#v, want dispatch-failed", state.FailedDispatches)
	}
	if _, ok := state.FailedWorkItemsByID["work-failed"]; !ok {
		t.Fatalf("work-failed should be marked failed")
	}
	if _, ok := state.ActiveWorkItemsByID["work-rejected"]; !ok {
		t.Fatalf("work-rejected should remain active after rejected response output")
	}
	if got := state.PlaceOccupancyByID["task:init"].WorkItemIDs; len(got) != 1 || got[0] != "work-rejected" {
		t.Fatalf("task:init work IDs = %#v, want work-rejected", got)
	}
}

func TestReconstructFactoryWorldState_FailedTerminalWorkRetainsFailureDetails(t *testing.T) {
	t0 := time.Date(2026, 4, 16, 8, 0, 0, 0, time.UTC)
	events := []factoryapi.FactoryEvent{
		initialStructureEvent(t0),
		workInputEvent(1, t0.Add(time.Second), interfaces.FactoryWorkItem{
			ID:          "work-failed",
			WorkTypeID:  "task",
			DisplayName: "Blocked story",
			TraceID:     "trace-failed",
			PlaceID:     "task:init",
		}),
		workstationRequestEvent(2, t0.Add(2*time.Second), interfaces.WorkstationRequestPayload{
			DispatchID:   "dispatch-failed",
			TransitionID: "t-review",
			Workstation:  interfaces.FactoryWorkstationRef{ID: "t-review", Name: "Review"},
			Inputs: []interfaces.WorkstationInput{{
				TokenID:  "work-failed",
				PlaceID:  "task:init",
				WorkItem: &interfaces.FactoryWorkItem{ID: "work-failed", WorkTypeID: "task", DisplayName: "Blocked story", TraceID: "trace-failed", PlaceID: "task:init"},
			}},
		}),
		workstationResponseEvent(3, t0.Add(3*time.Second), interfaces.WorkstationResponsePayload{
			DispatchID:     "dispatch-failed",
			TransitionID:   "t-review",
			Workstation:    interfaces.FactoryWorkstationRef{ID: "t-review", Name: "Review"},
			Result:         interfaces.WorkstationResult{Outcome: "FAILED", Error: "provider throttled", FailureReason: "throttled", FailureMessage: "Provider rate limit exceeded."},
			DurationMillis: 500,
			Outputs: []interfaces.WorkstationOutput{{
				Type:     string(interfaces.MutationMove),
				TokenID:  "work-failed-terminal",
				ToPlace:  "task:failed",
				WorkItem: &interfaces.FactoryWorkItem{ID: "work-failed", WorkTypeID: "task", DisplayName: "Blocked story", TraceID: "trace-failed", PlaceID: "task:failed"},
			}},
			TraceData: &interfaces.FactoryTraceData{TraceID: "trace-failed", WorkIDs: []string{"work-failed"}},
			TerminalWork: &interfaces.FactoryTerminalWork{
				WorkItem: interfaces.FactoryWorkItem{ID: "work-failed", WorkTypeID: "task", DisplayName: "Blocked story", TraceID: "trace-failed", PlaceID: "task:failed"},
				Status:   "FAILED",
			},
		}),
	}

	activeState, err := ReconstructFactoryWorldState(events, 2)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState active tick: %v", err)
	}
	if _, ok := activeState.FailureDetailsByWorkID["work-failed"]; ok {
		t.Fatalf("failure details should not exist before failed response")
	}

	failedState, err := ReconstructFactoryWorldState(events, 3)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState failed tick: %v", err)
	}

	detail := failedState.FailureDetailsByWorkID["work-failed"]
	if detail.DispatchID != "dispatch-failed" || detail.WorkstationName != "Review" {
		t.Fatalf("failure detail dispatch = %#v, want dispatch-failed from Review", detail)
	}
	if detail.FailureReason != "throttled" || detail.FailureMessage != "Provider rate limit exceeded." {
		t.Fatalf("failure detail = %#v, want throttled reason and provider message", detail)
	}
	if _, ok := failedState.FailedWorkItemsByID["work-failed"]; !ok {
		t.Fatalf("failed terminal work should be indexed as failed work")
	}
	if failedState.CompletedDispatches[0].Result.FailureReason != "throttled" {
		t.Fatalf("completion result = %#v, want failure reason retained", failedState.CompletedDispatches[0].Result)
	}
}
