package projections

import (
	"encoding/json"
	"strings"
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

func canonicalCompletedDispatchProjectionEvents(t0 time.Time) []factoryapi.FactoryEvent {
	return []factoryapi.FactoryEvent{
		factoryStateEvent(4, t0.Add(4*time.Second), "RUNNING", "COMPLETED"),
		workstationResponseEvent(3, t0.Add(3*time.Second), interfaces.WorkstationResponsePayload{
			DispatchID:     "dispatch-1",
			TransitionID:   "t-review",
			Workstation:    interfaces.FactoryWorkstationRef{ID: "t-review", Name: "Review"},
			Result:         interfaces.WorkstationResult{Outcome: "ACCEPTED"},
			DurationMillis: 2500,
			Outputs: []interfaces.WorkstationOutput{{
				Type:    string(interfaces.MutationMove),
				TokenID: "work-1",
				ToPlace: "task:complete",
				WorkItem: &interfaces.FactoryWorkItem{
					ID:          "work-1",
					WorkTypeID:  "task",
					DisplayName: "Write docs",
					TraceID:     "trace-1",
					PlaceID:     "task:complete",
				},
			}},
			TraceData:       &interfaces.FactoryTraceData{TraceID: "trace-1", WorkIDs: []string{"work-1"}},
			ProviderSession: &interfaces.ProviderSessionMetadata{Provider: "openai", Kind: "responses", ID: "sess-1"},
			TerminalWork: &interfaces.FactoryTerminalWork{
				WorkItem: interfaces.FactoryWorkItem{ID: "work-1", WorkTypeID: "task", DisplayName: "Write docs", TraceID: "trace-1", PlaceID: "task:complete"},
				Status:   "TERMINAL",
			},
		}),
		workInputEvent(1, t0.Add(time.Second), interfaces.FactoryWorkItem{ID: "work-1", WorkTypeID: "task", DisplayName: "Write docs", TraceID: "trace-1", PlaceID: "task:init"}),
		initialStructureEvent(t0),
		workstationRequestEvent(2, t0.Add(2*time.Second), interfaces.WorkstationRequestPayload{
			DispatchID:   "dispatch-1",
			TransitionID: "t-review",
			Workstation:  interfaces.FactoryWorkstationRef{ID: "t-review", Name: "Review"},
			Inputs: []interfaces.WorkstationInput{{
				TokenID:  "work-1",
				PlaceID:  "task:init",
				WorkItem: &interfaces.FactoryWorkItem{ID: "work-1", WorkTypeID: "task", DisplayName: "Write docs", TraceID: "trace-1", PlaceID: "task:init"},
			}},
		}),
		inferenceResponseEvent(3, t0.Add(2500*time.Millisecond), factoryapi.InferenceResponseEventPayload{
			InferenceRequestId: "dispatch-1/inference-request/1",
			Attempt:            1,
			Outcome:            factoryapi.InferenceOutcomeSucceeded,
			DurationMillis:     2500,
			ProviderSession:    generatedProviderSessionForProjectionTest(&interfaces.ProviderSessionMetadata{Provider: "openai", Kind: "responses", ID: "sess-1"}),
		}),
	}
}

func assertCanonicalCompletedDispatchState(t *testing.T, state interfaces.FactoryWorldState) {
	t.Helper()

	if state.Tick != 3 {
		t.Fatalf("Tick = %d, want 3", state.Tick)
	}
	if len(state.ActiveDispatches) != 0 {
		t.Fatalf("active dispatches = %#v, want none", state.ActiveDispatches)
	}
	if len(state.CompletedDispatches) != 1 {
		t.Fatalf("completed dispatches = %d, want 1", len(state.CompletedDispatches))
	}
	if state.CompletedDispatches[0].DispatchID != "dispatch-1" || state.CompletedDispatches[0].StartedTick != 2 {
		t.Fatalf("completion = %#v, want dispatch-1 started at tick 2", state.CompletedDispatches[0])
	}
	if _, ok := state.ActiveWorkItemsByID["work-1"]; ok {
		t.Fatalf("work-1 should not remain active after terminal response")
	}
	if terminal := state.TerminalWorkByID["work-1"]; terminal.Status != "TERMINAL" {
		t.Fatalf("terminal work = %#v, want TERMINAL", terminal)
	}
	if got := state.PlaceOccupancyByID["task:complete"].WorkItemIDs; len(got) != 1 || got[0] != "work-1" {
		t.Fatalf("task:complete work IDs = %#v, want work-1", got)
	}
	if got := state.TracesByID["trace-1"].DispatchIDs; len(got) != 1 || got[0] != "dispatch-1" {
		t.Fatalf("trace dispatch IDs = %#v, want dispatch-1", got)
	}
	if len(state.ProviderSessions) != 1 || state.ProviderSessions[0].ProviderSession.ID != "sess-1" {
		t.Fatalf("provider sessions = %#v, want sess-1", state.ProviderSessions)
	}
	if state.FactoryState != "" {
		t.Fatalf("FactoryState = %q, want empty before tick 4", state.FactoryState)
	}
}

func safeResponseDiagnosticsProjectionEvents(t0 time.Time) []factoryapi.FactoryEvent {
	diagnostics := projectionSafeResponseDiagnostics()
	return []factoryapi.FactoryEvent{
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
			Outcome:            factoryapi.InferenceOutcomeSucceeded,
			DurationMillis:     1500,
			ProviderSession:    generatedProviderSessionForProjectionTest(&interfaces.ProviderSessionMetadata{Provider: "codex", Kind: "response_id", ID: "resp-1"}),
			Diagnostics:        generatedWorkDiagnosticsForProjectionTest(diagnostics),
		}),
		workstationResponseEvent(3, t0.Add(3*time.Second), interfaces.WorkstationResponsePayload{
			DispatchID:      "dispatch-1",
			TransitionID:    "t-review",
			Workstation:     interfaces.FactoryWorkstationRef{ID: "t-review", Name: "Review"},
			Result:          interfaces.WorkstationResult{Outcome: "ACCEPTED"},
			DurationMillis:  1500,
			TraceData:       &interfaces.FactoryTraceData{TraceID: "trace-1", WorkIDs: []string{"work-1"}},
			ProviderSession: &interfaces.ProviderSessionMetadata{Provider: "codex", Kind: "response_id", ID: "resp-1"},
			Diagnostics:     diagnostics,
		}),
	}
}

func projectionSafeResponseDiagnostics() *interfaces.SafeWorkDiagnostics {
	return &interfaces.SafeWorkDiagnostics{
		RenderedPrompt: &interfaces.SafeRenderedPromptDiagnostic{
			SystemPromptHash: "system-hash",
			UserMessageHash:  "user-hash",
		},
		Provider: &interfaces.SafeProviderDiagnostic{
			Provider: "codex",
			Model:    "gpt-5.4",
			ResponseMetadata: map[string]string{
				"retry_count": "1",
			},
		},
	}
}

func assertSafeResponseDiagnosticsState(t *testing.T, state interfaces.FactoryWorldState) {
	t.Helper()

	if len(state.CompletedDispatches) != 1 {
		t.Fatalf("completed dispatches = %d, want 1", len(state.CompletedDispatches))
	}
	diagnostics := state.CompletedDispatches[0].Diagnostics
	if diagnostics == nil || diagnostics.Provider == nil || diagnostics.RenderedPrompt == nil {
		t.Fatalf("completion diagnostics = %#v, want provider and rendered prompt", diagnostics)
	}
	if diagnostics.Provider.Provider != "codex" || diagnostics.Provider.Model != "gpt-5.4" {
		t.Fatalf("provider diagnostics = %#v, want codex/gpt-5.4", diagnostics.Provider)
	}
	if diagnostics.RenderedPrompt.SystemPromptHash != "system-hash" || diagnostics.RenderedPrompt.UserMessageHash != "user-hash" {
		t.Fatalf("rendered prompt diagnostics = %#v, want hashes", diagnostics.RenderedPrompt)
	}
	if len(state.ProviderSessions) != 1 || state.ProviderSessions[0].Diagnostics == nil {
		t.Fatalf("provider sessions = %#v, want diagnostics copied into provider attempt", state.ProviderSessions)
	}
	if state.ProviderSessions[0].Diagnostics.Provider.ResponseMetadata["retry_count"] != "1" {
		t.Fatalf("provider session diagnostics = %#v, want retry_count", state.ProviderSessions[0].Diagnostics)
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

func TestReconstructFactoryWorldState_BeforeFirstEventReturnsEmptyState(t *testing.T) {
	state, err := ReconstructFactoryWorldState([]factoryapi.FactoryEvent{initialStructureEvent(time.Now())}, -1)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState: %v", err)
	}
	if len(state.Topology.Places) != 0 || len(state.WorkItemsByID) != 0 {
		t.Fatalf("state before first event = %#v, want empty", state)
	}
}

func TestReconstructFactoryWorldState_AcceptsJSONDecodedPayloads(t *testing.T) {
	events := []factoryapi.FactoryEvent{
		initialStructureEvent(time.Date(2026, 4, 16, 8, 0, 0, 0, time.UTC)),
		workInputEvent(1, time.Date(2026, 4, 16, 8, 0, 1, 0, time.UTC), interfaces.FactoryWorkItem{ID: "work-1", WorkTypeID: "task", PlaceID: "task:init"}),
	}
	raw, err := json.Marshal(events)
	if err != nil {
		t.Fatalf("Marshal events: %v", err)
	}
	var decoded []factoryapi.FactoryEvent
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("Unmarshal events: %v", err)
	}

	state, err := ReconstructFactoryWorldState(decoded, 1)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState: %v", err)
	}
	if _, ok := state.ActiveWorkItemsByID["work-1"]; !ok {
		t.Fatalf("decoded work input should reconstruct active work")
	}
}

func TestReconstructFactoryWorldState_SeedsResourceOccupancyFromInitialStructure(t *testing.T) {
	t0 := time.Date(2026, 4, 18, 14, 0, 0, 0, time.UTC)
	events := []factoryapi.FactoryEvent{
		initialStructureEventWithResources(t0, []factoryapi.Resource{
			{Name: "agent-slot", Capacity: 2},
			{Name: "gpu", Capacity: 1},
			{Name: "empty-slot", Capacity: 0},
		}),
	}

	state, err := ReconstructFactoryWorldState(events, 0)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState: %v", err)
	}

	agentSlot := state.PlaceOccupancyByID["agent-slot:available"]
	if agentSlot.TokenCount != 2 {
		t.Fatalf("agent-slot:available token count = %d, want 2", agentSlot.TokenCount)
	}
	if got := agentSlot.ResourceTokenIDs; len(got) != 2 || got[0] != "agent-slot:resource:0" || got[1] != "agent-slot:resource:1" {
		t.Fatalf("agent-slot resource tokens = %#v, want deterministic capacity tokens", got)
	}
	if got := state.PlaceOccupancyByID["gpu:available"].TokenCount; got != 1 {
		t.Fatalf("gpu:available token count = %d, want 1", got)
	}
	if _, ok := state.PlaceOccupancyByID["empty-slot:available"]; ok {
		t.Fatalf("empty-slot:available should not have resource occupancy for zero capacity")
	}
}

func TestReconstructFactoryWorldState_AppliesResourceDispatchDeltas(t *testing.T) {
	t0 := time.Date(2026, 4, 18, 15, 0, 0, 0, time.UTC)
	events := []factoryapi.FactoryEvent{
		initialStructureEventWithResources(t0, []factoryapi.Resource{{Name: "agent-slot", Capacity: 2}}),
		workstationRequestEvent(1, t0.Add(time.Second), interfaces.WorkstationRequestPayload{
			DispatchID:   "dispatch-1",
			TransitionID: "t-review",
			Workstation:  interfaces.FactoryWorkstationRef{ID: "t-review", Name: "Review"},
			Resources:    []interfaces.FactoryResourceUnit{{ResourceID: "agent-slot"}},
		}),
		workstationResponseEvent(2, t0.Add(2*time.Second), interfaces.WorkstationResponsePayload{
			DispatchID:      "dispatch-1",
			TransitionID:    "t-review",
			Workstation:     interfaces.FactoryWorkstationRef{ID: "t-review", Name: "Review"},
			Result:          interfaces.WorkstationResult{Outcome: "ACCEPTED"},
			OutputResources: []interfaces.FactoryResourceUnit{{ResourceID: "agent-slot"}},
		}),
	}

	idle, err := ReconstructFactoryWorldState(events, 0)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState idle tick: %v", err)
	}
	if got := idle.PlaceOccupancyByID["agent-slot:available"].TokenCount; got != 2 {
		t.Fatalf("idle agent-slot:available token count = %d, want 2", got)
	}

	active, err := ReconstructFactoryWorldState(events, 1)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState active tick: %v", err)
	}
	if got := active.PlaceOccupancyByID["agent-slot:available"].TokenCount; got != 1 {
		t.Fatalf("active agent-slot:available token count = %d, want 1", got)
	}
	if got := active.PlaceOccupancyByID["agent-slot:available"].ResourceTokenIDs; len(got) != 1 || got[0] != "agent-slot:resource:1" {
		t.Fatalf("active resource token IDs = %#v, want only unconsumed token", got)
	}
	activeDispatch := active.ActiveDispatches["dispatch-1"]
	if len(activeDispatch.Resources) != 1 || activeDispatch.Resources[0].TokenID != "agent-slot:resource:0" {
		t.Fatalf("active dispatch resources = %#v, want consumed resource token identity", activeDispatch.Resources)
	}

	released, err := ReconstructFactoryWorldState(events, 2)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState released tick: %v", err)
	}
	if got := released.PlaceOccupancyByID["agent-slot:available"].TokenCount; got != 2 {
		t.Fatalf("released agent-slot:available token count = %d, want 2", got)
	}
	if got := released.PlaceOccupancyByID["agent-slot:available"].ResourceTokenIDs; len(got) != 2 || got[0] != "agent-slot:resource:0" || got[1] != "agent-slot:resource:1" {
		t.Fatalf("released resource token IDs = %#v, want consumed token restored", got)
	}
}

func initialStructureEvent(eventTime time.Time) factoryapi.FactoryEvent {
	payload := factoryapi.InitialStructureRequestEventPayload{
		Factory: factoryapi.Factory{
			WorkTypes: &[]factoryapi.WorkType{{
				Name: "task",
				States: []factoryapi.WorkState{
					{Name: "init", Type: factoryapi.WorkStateTypeINITIAL},
					{Name: "review", Type: factoryapi.WorkStateTypePROCESSING},
					{Name: "complete", Type: factoryapi.WorkStateTypeTERMINAL},
					{Name: "failed", Type: factoryapi.WorkStateTypeFAILED},
				},
			}},
			Workstations: &[]factoryapi.Workstation{{
				Id:        stringPtrForProjectionTest("t-review"),
				Name:      "Review",
				Worker:    "reviewer",
				Inputs:    []factoryapi.WorkstationIO{{WorkType: "task", State: "init"}},
				Outputs:   []factoryapi.WorkstationIO{{WorkType: "task", State: "complete"}},
				OnFailure: &factoryapi.WorkstationIO{WorkType: "task", State: "failed"},
			}},
		},
	}
	return generatedProjectionEvent(factoryapi.FactoryEventTypeInitialStructureRequest, "initial", 0, eventTime, factoryapi.FactoryEventContext{}, payload)
}

func runRequestEvent(eventTime time.Time) factoryapi.FactoryEvent {
	payload := factoryapi.RunRequestEventPayload{
		RecordedAt: eventTime,
		Factory: factoryapi.Factory{
			Resources: &[]factoryapi.Resource{{
				Name:     "agent-slot",
				Capacity: 2,
			}},
			WorkTypes: &[]factoryapi.WorkType{{
				Name: "task",
				States: []factoryapi.WorkState{
					{Name: "init", Type: factoryapi.WorkStateTypeINITIAL},
					{Name: "review", Type: factoryapi.WorkStateTypePROCESSING},
					{Name: "complete", Type: factoryapi.WorkStateTypeTERMINAL},
					{Name: "failed", Type: factoryapi.WorkStateTypeFAILED},
				},
			}},
			Workstations: &[]factoryapi.Workstation{{
				Id:        stringPtrForProjectionTest("t-review"),
				Name:      "Review",
				Worker:    "reviewer",
				Inputs:    []factoryapi.WorkstationIO{{WorkType: "task", State: "init"}},
				Outputs:   []factoryapi.WorkstationIO{{WorkType: "task", State: "complete"}},
				OnFailure: &factoryapi.WorkstationIO{WorkType: "task", State: "failed"},
			}},
		},
	}
	return generatedProjectionEvent(factoryapi.FactoryEventTypeRunRequest, "run-request", 0, eventTime, factoryapi.FactoryEventContext{}, payload)
}

func initialStructureEventWithResources(eventTime time.Time, resources []factoryapi.Resource) factoryapi.FactoryEvent {
	payload := factoryapi.InitialStructureRequestEventPayload{
		Factory: factoryapi.Factory{
			Resources: &resources,
		},
	}
	return generatedProjectionEvent(factoryapi.FactoryEventTypeInitialStructureRequest, "initial-resources", 0, eventTime, factoryapi.FactoryEventContext{}, payload)
}

func workInputEvent(tick int, eventTime time.Time, item interfaces.FactoryWorkItem) factoryapi.FactoryEvent {
	return workInputEventWithToken(tick, eventTime, item.ID, item)
}

func workInputEventWithToken(tick int, eventTime time.Time, _ string, item interfaces.FactoryWorkItem) factoryapi.FactoryEvent {
	requestID := "request/" + item.ID
	context := factoryapi.FactoryEventContext{
		RequestId: stringPtrForProjectionTest(requestID),
		TraceIds:  &[]string{item.TraceID},
		WorkIds:   &[]string{item.ID},
	}
	payload := factoryapi.WorkRequestEventPayload{
		Type:  factoryapi.WorkRequestTypeFactoryRequestBatch,
		Works: &[]factoryapi.Work{generatedWorkForProjectionTest(item, requestID)},
	}
	return generatedProjectionEvent(factoryapi.FactoryEventTypeWorkRequest, "work-input/"+item.ID, tick, eventTime, context, payload)
}

func workstationRequestEvent(tick int, eventTime time.Time, payload interfaces.WorkstationRequestPayload) factoryapi.FactoryEvent {
	works := make([]factoryapi.Work, 0, len(payload.Inputs))
	inputRefs := make([]factoryapi.DispatchConsumedWorkRef, 0, len(payload.Inputs))
	inputWorkItems := make([]interfaces.FactoryWorkItem, 0, len(payload.Inputs))
	for _, input := range payload.Inputs {
		if input.WorkItem != nil {
			inputWorkItems = append(inputWorkItems, *input.WorkItem)
			works = append(works, generatedWorkForProjectionTest(*input.WorkItem, ""))
			inputRefs = append(inputRefs, factoryapi.DispatchConsumedWorkRef{WorkId: input.WorkItem.ID})
		}
	}
	context := factoryapi.FactoryEventContext{
		DispatchId:               stringPtrForProjectionTest(payload.DispatchID),
		CurrentChainingTraceId:   stringPtrForProjectionTest(interfaces.CurrentChainingTraceIDFromWorkItems(inputWorkItems)),
		PreviousChainingTraceIds: stringSlicePtrForProjectionTest(interfaces.PreviousChainingTraceIDsFromWorkItems(inputWorkItems)),
		TraceIds:                 stringSlicePtrForProjectionTest(traceIDsForProjectionTest(works)),
		WorkIds:                  stringSlicePtrForProjectionTest(workIDsForProjectionTest(works)),
	}
	generatedPayload := factoryapi.DispatchRequestEventPayload{
		TransitionId:             payload.TransitionID,
		CurrentChainingTraceId:   stringPtrForProjectionTest(interfaces.CurrentChainingTraceIDFromWorkItems(inputWorkItems)),
		PreviousChainingTraceIds: stringSlicePtrForProjectionTest(interfaces.PreviousChainingTraceIDsFromWorkItems(inputWorkItems)),
		Inputs:                   inputRefs,
		Resources:                generatedResourcesForProjectionTest(payload.Resources),
	}
	return generatedProjectionEvent(factoryapi.FactoryEventTypeDispatchRequest, "request/"+payload.DispatchID, tick, eventTime, context, generatedPayload)
}

func workstationResponseEvent(tick int, eventTime time.Time, payload interfaces.WorkstationResponsePayload) factoryapi.FactoryEvent {
	outputWork := generatedOutputWorkForProjectionTest(payload)
	outcome := factoryapi.WorkOutcome(payload.Result.Outcome)
	context := factoryapi.FactoryEventContext{
		DispatchId: stringPtrForProjectionTest(payload.DispatchID),
		TraceIds:   stringSlicePtrForProjectionTest(traceIDsForProjectionTest(outputWork)),
		WorkIds:    stringSlicePtrForProjectionTest(workIDsForProjectionTest(outputWork)),
	}
	if payload.TraceData != nil {
		context.TraceIds = stringSlicePtrForProjectionTest([]string{payload.TraceData.TraceID})
		context.WorkIds = stringSlicePtrForProjectionTest(payload.TraceData.WorkIDs)
	}
	generatedPayload := factoryapi.DispatchResponseEventPayload{
		TransitionId:    payload.TransitionID,
		Outcome:         outcome,
		Output:          stringPtrForProjectionTest(payload.Result.Output),
		Error:           stringPtrForProjectionTest(payload.Result.Error),
		Feedback:        stringPtrForProjectionTest(payload.Result.Feedback),
		FailureReason:   stringPtrForProjectionTest(payload.Result.FailureReason),
		FailureMessage:  stringPtrForProjectionTest(payload.Result.FailureMessage),
		ProviderFailure: generatedProviderFailureForProjectionTest(payload.Result.ProviderFailure),
		DurationMillis:  int64PtrForProjectionTest(payload.DurationMillis),
		OutputWork:      &outputWork,
		OutputResources: generatedResourcesForProjectionTest(payload.OutputResources),
	}
	return generatedProjectionEvent(factoryapi.FactoryEventTypeDispatchResponse, "response/"+payload.DispatchID, tick, eventTime, context, generatedPayload)
}

func relationshipChangeEvent(tick int, eventTime time.Time, requestID string, traceID string, workIDs []string, relation factoryapi.Relation) factoryapi.FactoryEvent {
	return generatedProjectionEvent(factoryapi.FactoryEventTypeRelationshipChangeRequest, "relationship/"+requestID+"/"+relation.SourceWorkName+"/"+relation.TargetWorkName, tick, eventTime, factoryapi.FactoryEventContext{
		RequestId: stringPtrForProjectionTest(requestID),
		TraceIds:  stringSlicePtrForProjectionTest([]string{traceID}),
		WorkIds:   stringSlicePtrForProjectionTest(workIDs),
	}, factoryapi.RelationshipChangeRequestEventPayload{Relation: relation})
}

func factoryStateEvent(tick int, eventTime time.Time, previous string, next string) factoryapi.FactoryEvent {
	prev := factoryapi.FactoryState(previous)
	payload := factoryapi.FactoryStateResponseEventPayload{
		PreviousState: &prev,
		State:         factoryapi.FactoryState(next),
		Reason:        stringPtrForProjectionTest("test"),
	}
	return generatedProjectionEvent(factoryapi.FactoryEventTypeFactoryStateResponse, "state/"+next, tick, eventTime, factoryapi.FactoryEventContext{}, payload)
}

func generatedProjectionEvent(eventType factoryapi.FactoryEventType, id string, tick int, eventTime time.Time, context factoryapi.FactoryEventContext, payload any) factoryapi.FactoryEvent {
	context.Tick = tick
	context.EventTime = eventTime
	event := factoryapi.FactoryEvent{
		Context:       context,
		Id:            id,
		SchemaVersion: factoryapi.AgentFactoryEventV1,
		Type:          eventType,
	}
	switch typed := payload.(type) {
	case factoryapi.RunRequestEventPayload:
		if err := event.Payload.FromRunRequestEventPayload(typed); err != nil {
			panic(err)
		}
	case factoryapi.InitialStructureRequestEventPayload:
		if err := event.Payload.FromInitialStructureRequestEventPayload(typed); err != nil {
			panic(err)
		}
	case factoryapi.WorkRequestEventPayload:
		if err := event.Payload.FromWorkRequestEventPayload(typed); err != nil {
			panic(err)
		}
	case factoryapi.DispatchRequestEventPayload:
		if err := event.Payload.FromDispatchRequestEventPayload(typed); err != nil {
			panic(err)
		}
	case factoryapi.InferenceRequestEventPayload:
		if err := event.Payload.FromInferenceRequestEventPayload(typed); err != nil {
			panic(err)
		}
	case factoryapi.InferenceResponseEventPayload:
		if err := event.Payload.FromInferenceResponseEventPayload(typed); err != nil {
			panic(err)
		}
	case factoryapi.ScriptRequestEventPayload:
		if err := event.Payload.FromScriptRequestEventPayload(typed); err != nil {
			panic(err)
		}
	case factoryapi.ScriptResponseEventPayload:
		if err := event.Payload.FromScriptResponseEventPayload(typed); err != nil {
			panic(err)
		}
	case factoryapi.DispatchResponseEventPayload:
		if err := event.Payload.FromDispatchResponseEventPayload(typed); err != nil {
			panic(err)
		}
	case factoryapi.RelationshipChangeRequestEventPayload:
		if err := event.Payload.FromRelationshipChangeRequestEventPayload(typed); err != nil {
			panic(err)
		}
	case factoryapi.FactoryStateResponseEventPayload:
		if err := event.Payload.FromFactoryStateResponseEventPayload(typed); err != nil {
			panic(err)
		}
	case factoryapi.RunResponseEventPayload:
		if err := event.Payload.FromRunResponseEventPayload(typed); err != nil {
			panic(err)
		}
	default:
		panic("unsupported projection test payload")
	}
	return event
}

func inferenceRequestEvent(tick int, eventTime time.Time, payload factoryapi.InferenceRequestEventPayload) factoryapi.FactoryEvent {
	context := factoryapi.FactoryEventContext{
		DispatchId: stringPtrForProjectionTest(dispatchIDForInferenceRequest(payload.InferenceRequestId)),
	}
	return generatedProjectionEvent(factoryapi.FactoryEventTypeInferenceRequest, "inference-request/"+payload.InferenceRequestId, tick, eventTime, context, payload)
}

func inferenceResponseEvent(tick int, eventTime time.Time, payload factoryapi.InferenceResponseEventPayload) factoryapi.FactoryEvent {
	context := factoryapi.FactoryEventContext{
		DispatchId: stringPtrForProjectionTest(dispatchIDForInferenceRequest(payload.InferenceRequestId)),
	}
	return generatedProjectionEvent(factoryapi.FactoryEventTypeInferenceResponse, "inference-response/"+payload.InferenceRequestId, tick, eventTime, context, payload)
}

func dispatchIDForInferenceRequest(inferenceRequestID string) string {
	if idx := strings.Index(inferenceRequestID, "/inference-request/"); idx > 0 {
		return inferenceRequestID[:idx]
	}
	return ""
}

func scriptRequestEvent(tick int, eventTime time.Time, payload factoryapi.ScriptRequestEventPayload) factoryapi.FactoryEvent {
	context := factoryapi.FactoryEventContext{
		DispatchId: stringPtrForProjectionTest(payload.DispatchId),
	}
	return generatedProjectionEvent(factoryapi.FactoryEventTypeScriptRequest, "script-request/"+payload.ScriptRequestId, tick, eventTime, context, payload)
}

func scriptResponseEvent(tick int, eventTime time.Time, payload factoryapi.ScriptResponseEventPayload) factoryapi.FactoryEvent {
	context := factoryapi.FactoryEventContext{
		DispatchId: stringPtrForProjectionTest(payload.DispatchId),
	}
	return generatedProjectionEvent(factoryapi.FactoryEventTypeScriptResponse, "script-response/"+payload.ScriptRequestId, tick, eventTime, context, payload)
}

func generatedWorkForProjectionTest(item interfaces.FactoryWorkItem, requestID string) factoryapi.Work {
	return factoryapi.Work{
		Name:                     item.DisplayName,
		RequestId:                stringPtrForProjectionTest(requestID),
		Tags:                     generatedStringMapForProjectionTest(item.Tags),
		CurrentChainingTraceId:   stringPtrForProjectionTest(item.CurrentChainingTraceID),
		PreviousChainingTraceIds: stringSlicePtrForProjectionTest(item.PreviousChainingTraceIDs),
		TraceId:                  stringPtrForProjectionTest(item.TraceID),
		WorkId:                   stringPtrForProjectionTest(item.ID),
		WorkTypeName:             stringPtrForProjectionTest(item.WorkTypeID),
	}
}

func generatedOutputWorkForProjectionTest(payload interfaces.WorkstationResponsePayload) []factoryapi.Work {
	works := make([]factoryapi.Work, 0, len(payload.OutputWork)+len(payload.Outputs))
	for _, item := range payload.OutputWork {
		works = append(works, generatedWorkForProjectionTest(item, ""))
	}
	for _, output := range payload.Outputs {
		if output.WorkItem != nil {
			works = append(works, generatedWorkForProjectionTest(*output.WorkItem, ""))
		}
	}
	if payload.TerminalWork != nil {
		works = append(works, generatedWorkForProjectionTest(payload.TerminalWork.WorkItem, ""))
	}
	return works
}

func generatedResourcesForProjectionTest(resources []interfaces.FactoryResourceUnit) *[]factoryapi.Resource {
	if len(resources) == 0 {
		return nil
	}
	out := make([]factoryapi.Resource, 0, len(resources))
	for _, resource := range resources {
		if resource.ResourceID == "" {
			continue
		}
		out = append(out, factoryapi.Resource{Name: resource.ResourceID})
	}
	if len(out) == 0 {
		return nil
	}
	return &out
}

func generatedProviderSessionForProjectionTest(session *interfaces.ProviderSessionMetadata) *factoryapi.ProviderSessionMetadata {
	if session == nil {
		return nil
	}
	return &factoryapi.ProviderSessionMetadata{
		Id:       stringPtrForProjectionTest(session.ID),
		Kind:     stringPtrForProjectionTest(session.Kind),
		Provider: stringPtrForProjectionTest(session.Provider),
	}
}

func generatedProviderFailureForProjectionTest(failure *interfaces.ProviderFailureMetadata) *factoryapi.ProviderFailureMetadata {
	if failure == nil {
		return nil
	}
	return &factoryapi.ProviderFailureMetadata{
		Family: stringPtrForProjectionTest(string(failure.Family)),
		Type:   stringPtrForProjectionTest(string(failure.Type)),
	}
}

func generatedWorkDiagnosticsForProjectionTest(diagnostics *interfaces.SafeWorkDiagnostics) *factoryapi.SafeWorkDiagnostics {
	return interfaces.GeneratedSafeWorkDiagnostics(diagnostics)
}

func generatedStringMapForProjectionTest(values map[string]string) *factoryapi.StringMap {
	if len(values) == 0 {
		return nil
	}
	converted := factoryapi.StringMap(values)
	return &converted
}

func traceIDsForProjectionTest(works []factoryapi.Work) []string {
	values := make([]string, 0, len(works))
	for _, work := range works {
		if work.TraceId != nil {
			values = append(values, *work.TraceId)
		}
	}
	return values
}

func workIDsForProjectionTest(works []factoryapi.Work) []string {
	values := make([]string, 0, len(works))
	for _, work := range works {
		if work.WorkId != nil {
			values = append(values, *work.WorkId)
		}
	}
	return values
}

func stringPtrForProjectionTest(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func stringValueForProjectionTest(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func stringSlicePtrForProjectionTest(values []string) *[]string {
	if len(values) == 0 {
		return nil
	}
	return &values
}

func int64PtrForProjectionTest(value int64) *int64 {
	return &value
}

func intPtrForProjectionTest(value int) *int {
	return &value
}
