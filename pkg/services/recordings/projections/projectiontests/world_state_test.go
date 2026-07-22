package projections_test

import (
	"reflect"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestReconstructFactoryWorldState_AppliesCanonicalEventsByTick(t *testing.T) {
	t0 := time.Date(2026, 4, 16, 8, 0, 0, 0, time.UTC)
	state, err := ReconstructFactoryWorldState(canonicalCompletedDispatchProjectionEvents(t0), 3)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState: %v", err)
	}
	assertCanonicalCompletedDispatchState(t, state)
}

func int64PtrForProjectionTest(value int64) *int64 {
	return &value
}

func intPtrForProjectionTest(value int) *int {
	return &value
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
		workInputEventWithToken(1, t0.Add(time.Second), "tok-task-1", work.FactoryWorkItem{ID: "work-1", WorkTypeID: "task", TraceID: "trace-1", PlaceID: "task:init"}),
		workstationRequestEvent(2, t0.Add(2*time.Second), interfaces.WorkstationRequestPayload{
			DispatchID:   "dispatch-1",
			TransitionID: "t-review",
			Workstation:  interfaces.FactoryWorkstationRef{ID: "t-review", Name: "Review"},
			Inputs: []interfaces.WorkstationInput{{
				TokenID:  "tok-task-1",
				PlaceID:  "task:init",
				WorkItem: &work.FactoryWorkItem{ID: "work-1", WorkTypeID: "task", TraceID: "trace-1", PlaceID: "task:init"},
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
		workInputEvent(1, t0.Add(time.Second), work.FactoryWorkItem{
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
		workInputEventWithToken(1, t0.Add(time.Second), "tok-task-1", work.FactoryWorkItem{ID: "work-1", WorkTypeID: "task", TraceID: "trace-1", PlaceID: "task:init"}),
		workstationRequestEvent(2, t0.Add(2*time.Second), interfaces.WorkstationRequestPayload{
			DispatchID:   "dispatch-1",
			TransitionID: "t-review",
			Workstation:  interfaces.FactoryWorkstationRef{ID: "t-review", Name: "Review"},
			Inputs: []interfaces.WorkstationInput{{
				TokenID:  "tok-task-1",
				PlaceID:  "task:init",
				WorkItem: &work.FactoryWorkItem{ID: "work-1", WorkTypeID: "task", TraceID: "trace-1", PlaceID: "task:init"},
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
		workInputEventWithToken(1, t0.Add(time.Second), "tok-task-1", work.FactoryWorkItem{ID: "work-1", WorkTypeID: "task", TraceID: "trace-1", PlaceID: "task:init"}),
		workstationRequestEvent(2, t0.Add(2*time.Second), interfaces.WorkstationRequestPayload{
			DispatchID:   "dispatch-script-1",
			TransitionID: "t-review",
			Workstation:  interfaces.FactoryWorkstationRef{ID: "t-review", Name: "Review"},
			Inputs: []interfaces.WorkstationInput{{
				TokenID:  "tok-task-1",
				PlaceID:  "task:init",
				WorkItem: &work.FactoryWorkItem{ID: "work-1", WorkTypeID: "task", TraceID: "trace-1", PlaceID: "task:init"},
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

func TestReconstructFactoryWorldState_PreservesSafeResponseDiagnostics(t *testing.T) {
	t0 := time.Date(2026, 4, 18, 10, 0, 0, 0, time.UTC)
	state, err := ReconstructFactoryWorldState(safeResponseDiagnosticsProjectionEvents(t0), 3)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState: %v", err)
	}
	assertSafeResponseDiagnosticsState(t, state)
}

func TestReconstructFactoryWorldState_PreservesWindowsProcessFailureDetail(t *testing.T) {
	t0 := time.Date(2026, 4, 21, 20, 0, 0, 0, time.UTC)
	const message = "provider error: internal_server_error: codex exited with code 4294967295: stderr: OpenAI Codex v0.118.0 (research preview)"
	workItem := work.FactoryWorkItem{
		ID: "work-safe-windows-process-failure", WorkTypeID: "task",
		TraceID: "trace-safe-windows-process-failure", PlaceID: "task:init",
	}
	events := []factoryapi.FactoryEvent{
		initialStructureEvent(t0),
		workInputEvent(1, t0.Add(time.Second), workItem),
		workstationRequestEvent(2, t0.Add(2*time.Second), interfaces.WorkstationRequestPayload{
			DispatchID: "dispatch-windows-process-failure", TransitionID: "t-review",
			Inputs: []interfaces.WorkstationInput{{
				TokenID: "work-safe-windows-process-failure", PlaceID: "task:init", WorkItem: &workItem,
			}},
		}),
		workstationResponseEvent(3, t0.Add(3*time.Second), interfaces.WorkstationResponsePayload{
			DispatchID: "dispatch-windows-process-failure", TransitionID: "t-review",
			Result: interfaces.WorkstationResult{
				Outcome: "FAILED",
				FailureDetail: &workerexecution.FailureDetail{
					Reason: workerexecution.WorkFailureTypeInternalServerError, Message: message,
				},
			},
			Outputs: []interfaces.WorkstationOutput{{
				Type: string(interfaces.MutationMove), TokenID: workItem.ID,
				ToPlace: "task:failed",
				WorkItem: &work.FactoryWorkItem{
					ID: workItem.ID, WorkTypeID: workItem.WorkTypeID,
					TraceID: workItem.TraceID, PlaceID: "task:failed",
				},
			}},
			TerminalWork: &interfaces.FactoryTerminalWork{
				WorkItem: work.FactoryWorkItem{
					ID: workItem.ID, WorkTypeID: workItem.WorkTypeID,
					TraceID: workItem.TraceID, PlaceID: "task:failed",
				},
				Status: "FAILED",
			},
		}),
	}

	state, err := ReconstructFactoryWorldState(events, 3)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState: %v", err)
	}
	detail := state.FailureDetailsByWorkID[workItem.ID].FailureDetail
	if detail == nil || detail.Reason != workerexecution.WorkFailureTypeInternalServerError || detail.Message != message {
		t.Fatalf("failure detail = %#v, want preserved internal-server-error message", detail)
	}
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
		workInputEventWithToken(1, t0.Add(time.Second), "tok-task-1", work.FactoryWorkItem{ID: "work-1", WorkTypeID: "task", TraceID: "trace-1", PlaceID: "task:init"}),
		workstationRequestEvent(2, t0.Add(2*time.Second), interfaces.WorkstationRequestPayload{
			DispatchID:   "dispatch-1",
			TransitionID: "t-review",
			Workstation:  interfaces.FactoryWorkstationRef{ID: "t-review", Name: "Review"},
			Inputs: []interfaces.WorkstationInput{{
				TokenID:  "tok-task-1",
				PlaceID:  "task:init",
				WorkItem: &work.FactoryWorkItem{ID: "work-1", WorkTypeID: "task", TraceID: "trace-1", PlaceID: "task:init"},
			}},
		}),
		inferenceResponseEvent(3, t0.Add(2500*time.Millisecond), factoryapi.InferenceResponseEventPayload{
			InferenceRequestId: "dispatch-1/inference-request/1",
			Attempt:            1,
			Outcome:            factoryapi.InferenceOutcomeFailed,
			DurationMillis:     900,
			FailureDetail:      &factoryapi.FailureDetail{Reason: factoryapi.WorkFailureTypeTimeout, Message: "provider timed out"},
			ProviderSession: generatedProviderSessionForProjectionTest(&workerexecution.ProviderSessionMetadata{
				Provider: "codex",
				Kind:     "session_id",
				ID:       "sess-1",
			}),
		}),
		workstationResponseEvent(3, t0.Add(3*time.Second), interfaces.WorkstationResponsePayload{
			DispatchID:     "dispatch-1",
			TransitionID:   "t-review",
			Workstation:    interfaces.FactoryWorkstationRef{ID: "t-review", Name: "Review"},
			Result:         interfaces.WorkstationResult{Outcome: "FAILED", FailureMetadata: &workerexecution.WorkFailureMetadata{Family: workerexecution.WorkFailureFamilyRetryable, Type: workerexecution.WorkFailureTypeTimeout}},
			DurationMillis: 900,
			TraceData:      &interfaces.FactoryTraceData{TraceID: "trace-1", WorkIDs: []string{"work-1"}},
			ProviderSession: &workerexecution.ProviderSessionMetadata{
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
	if completion.Result.FailureMetadata == nil {
		t.Fatal("completion failure metadata is nil, want canonical metadata")
	}
	if completion.Result.FailureMetadata.Family != workerexecution.WorkFailureFamilyRetryable ||
		completion.Result.FailureMetadata.Type != workerexecution.WorkFailureTypeTimeout {
		t.Fatalf("completion failure metadata = %#v, want retryable/timeout", completion.Result.FailureMetadata)
	}
	if len(state.ProviderSessions) != 1 || state.ProviderSessions[0].ProviderSession.ID != "sess-1" {
		t.Fatalf("provider sessions = %#v, want sess-1", state.ProviderSessions)
	}
}

func TestReconstructFactoryWorldState_MapsLegacyProviderFailureOnlyWireToFailureMetadata(t *testing.T) {
	t0 := time.Date(2026, 4, 19, 11, 0, 0, 0, time.UTC)
	family := factoryapi.WorkFailureFamily(workerexecution.WorkFailureFamilyRetryable)
	failureType := factoryapi.WorkFailureType(workerexecution.WorkFailureTypeInternalServerError)
	events := []factoryapi.FactoryEvent{
		initialStructureEvent(t0),
		workInputEventWithToken(1, t0.Add(time.Second), "tok-task-1", work.FactoryWorkItem{ID: "work-1", WorkTypeID: "task", TraceID: "trace-1", PlaceID: "task:init"}),
		workstationRequestEvent(2, t0.Add(2*time.Second), interfaces.WorkstationRequestPayload{
			DispatchID:   "dispatch-legacy-wire",
			TransitionID: "t-review",
			Workstation:  interfaces.FactoryWorkstationRef{ID: "t-review", Name: "Review"},
			Inputs: []interfaces.WorkstationInput{{
				TokenID:  "tok-task-1",
				PlaceID:  "task:init",
				WorkItem: &work.FactoryWorkItem{ID: "work-1", WorkTypeID: "task", TraceID: "trace-1", PlaceID: "task:init"},
			}},
		}),
		generatedProjectionEvent(
			factoryapi.FactoryEventTypeDispatchResponse,
			"response/dispatch-legacy-wire",
			3,
			t0.Add(3*time.Second),
			factoryapi.FactoryEventContext{
				DispatchId: stringPtrForProjectionTest("dispatch-legacy-wire"),
				TraceIds:   stringSlicePtrForProjectionTest([]string{"trace-1"}),
				WorkIds:    stringSlicePtrForProjectionTest([]string{"work-1"}),
			},
			factoryapi.DispatchResponseEventPayload{
				TransitionId: "t-review",
				Outcome:      factoryapi.WorkOutcomeFailed,
				Error:        stringPtrForProjectionTest("provider error: internal_server_error"),
				ProviderFailure: &factoryapi.ProviderFailureMetadata{
					Family: &family,
					Type:   &failureType,
				},
				DurationMillis: int64PtrForProjectionTest(900),
			},
		),
	}

	state, err := ReconstructFactoryWorldState(events, 3)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState: %v", err)
	}
	if len(state.CompletedDispatches) != 1 {
		t.Fatalf("completed dispatches = %#v, want 1 completion", state.CompletedDispatches)
	}
	completion := state.CompletedDispatches[0]
	if completion.Result.FailureMetadata == nil {
		t.Fatal("completion failure metadata is nil, want ingress from wire provider_failure")
	}
	if completion.Result.FailureMetadata.Family != workerexecution.WorkFailureFamilyRetryable ||
		completion.Result.FailureMetadata.Type != workerexecution.WorkFailureTypeInternalServerError {
		t.Fatalf("completion failure metadata = %#v, want retryable/internal_server_error", completion.Result.FailureMetadata)
	}
}

func TestReconstructFactoryWorldState_WorkInputTokenIDMatchesRequestConsumption(t *testing.T) {
	t0 := time.Date(2026, 4, 16, 8, 0, 0, 0, time.UTC)
	item := work.FactoryWorkItem{ID: "work-1", WorkTypeID: "task", TraceID: "trace-1", PlaceID: "task:init"}
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
		generatedWorkForProjectionTest(work.FactoryWorkItem{ID: "work-parent", WorkTypeID: "task", DisplayName: "parent", TraceID: "trace-parent-child"}, requestID),
		generatedWorkForProjectionTest(work.FactoryWorkItem{ID: "work-prerequisite", WorkTypeID: "task", DisplayName: "prerequisite", TraceID: "trace-parent-child"}, requestID),
		generatedWorkForProjectionTest(work.FactoryWorkItem{ID: "work-child", WorkTypeID: "task", DisplayName: "child", TraceID: "trace-parent-child"}, requestID),
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
		workInputEvent(1, t0.Add(time.Second), work.FactoryWorkItem{ID: "work-failed", WorkTypeID: "task", TraceID: "trace-failed", PlaceID: "task:init"}),
		workstationRequestEvent(2, t0.Add(2*time.Second), interfaces.WorkstationRequestPayload{
			DispatchID:   "dispatch-failed",
			TransitionID: "t-review",
			Workstation:  interfaces.FactoryWorkstationRef{ID: "t-review"},
			Inputs: []interfaces.WorkstationInput{{
				TokenID:  "work-failed",
				PlaceID:  "task:init",
				WorkItem: &work.FactoryWorkItem{ID: "work-failed", WorkTypeID: "task", TraceID: "trace-failed", PlaceID: "task:init"},
			}},
		}),
		workstationResponseEvent(3, t0.Add(3*time.Second), interfaces.WorkstationResponsePayload{
			DispatchID:   "dispatch-failed",
			TransitionID: "t-review",
			Workstation:  interfaces.FactoryWorkstationRef{ID: "t-review"},
			Result:       interfaces.WorkstationResult{Outcome: "FAILED", Error: "boom"},
			TraceData:    &interfaces.FactoryTraceData{TraceID: "trace-failed", WorkIDs: []string{"work-failed"}},
		}),
		workInputEvent(4, t0.Add(4*time.Second), work.FactoryWorkItem{ID: "work-rejected", WorkTypeID: "task", TraceID: "trace-rejected", PlaceID: "task:init"}),
		workstationRequestEvent(5, t0.Add(5*time.Second), interfaces.WorkstationRequestPayload{
			DispatchID:   "dispatch-rejected",
			TransitionID: "t-review",
			Workstation:  interfaces.FactoryWorkstationRef{ID: "t-review"},
			Inputs: []interfaces.WorkstationInput{{
				TokenID:  "work-rejected",
				PlaceID:  "task:init",
				WorkItem: &work.FactoryWorkItem{ID: "work-rejected", WorkTypeID: "task", TraceID: "trace-rejected", PlaceID: "task:init"},
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
				WorkItem: &work.FactoryWorkItem{
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
		workInputEvent(1, t0.Add(time.Second), work.FactoryWorkItem{
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
				WorkItem: &work.FactoryWorkItem{ID: "work-failed", WorkTypeID: "task", DisplayName: "Blocked story", TraceID: "trace-failed", PlaceID: "task:init"},
			}},
		}),
		workstationResponseEvent(3, t0.Add(3*time.Second), interfaces.WorkstationResponsePayload{
			DispatchID:     "dispatch-failed",
			TransitionID:   "t-review",
			Workstation:    interfaces.FactoryWorkstationRef{ID: "t-review", Name: "Review"},
			Result:         interfaces.WorkstationResult{Outcome: "FAILED", Error: "provider throttled", FailureDetail: &workerexecution.FailureDetail{Reason: workerexecution.WorkFailureTypeThrottled, Message: "Provider rate limit exceeded."}},
			DurationMillis: 500,
			Outputs: []interfaces.WorkstationOutput{{
				Type:     string(interfaces.MutationMove),
				TokenID:  "work-failed-terminal",
				ToPlace:  "task:failed",
				WorkItem: &work.FactoryWorkItem{ID: "work-failed", WorkTypeID: "task", DisplayName: "Blocked story", TraceID: "trace-failed", PlaceID: "task:failed"},
			}},
			TraceData: &interfaces.FactoryTraceData{TraceID: "trace-failed", WorkIDs: []string{"work-failed"}},
			TerminalWork: &interfaces.FactoryTerminalWork{
				WorkItem: work.FactoryWorkItem{ID: "work-failed", WorkTypeID: "task", DisplayName: "Blocked story", TraceID: "trace-failed", PlaceID: "task:failed"},
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
	if detail.FailureDetail == nil || detail.FailureDetail.Reason != workerexecution.WorkFailureTypeThrottled || detail.FailureDetail.Message != "Provider rate limit exceeded." {
		t.Fatalf("failure detail = %#v, want throttled reason and provider message", detail)
	}
	if _, ok := failedState.FailedWorkItemsByID["work-failed"]; !ok {
		t.Fatalf("failed terminal work should be indexed as failed work")
	}
	if failedState.CompletedDispatches[0].Result.FailureDetail == nil || failedState.CompletedDispatches[0].Result.FailureDetail.Reason != workerexecution.WorkFailureTypeThrottled {
		t.Fatalf("completion result = %#v, want failure reason retained", failedState.CompletedDispatches[0].Result)
	}
}

func TestReconstructFactoryWorldState_OutputWorkStateReconstructsTerminalPlaceWithoutExplicitOutputPlace(t *testing.T) {
	t0 := time.Date(2026, 6, 27, 7, 0, 0, 0, time.UTC)
	events := []factoryapi.FactoryEvent{
		initialStructureEvent(t0),
		workInputEvent(1, t0.Add(time.Second), work.FactoryWorkItem{
			ID:          "work-1",
			WorkTypeID:  "task",
			DisplayName: "Classifier terminal output",
			TraceID:     "trace-1",
			PlaceID:     "task:init",
		}),
		workstationRequestEvent(2, t0.Add(2*time.Second), interfaces.WorkstationRequestPayload{
			DispatchID:   "dispatch-1",
			TransitionID: "t-review",
			Workstation:  interfaces.FactoryWorkstationRef{ID: "t-review", Name: "Review"},
			Inputs: []interfaces.WorkstationInput{{
				TokenID:  "work-1",
				PlaceID:  "task:init",
				WorkItem: &work.FactoryWorkItem{ID: "work-1", WorkTypeID: "task", DisplayName: "Classifier terminal output", TraceID: "trace-1", PlaceID: "task:init"},
			}},
		}),
		workstationResponseEvent(3, t0.Add(3*time.Second), interfaces.WorkstationResponsePayload{
			DispatchID:     "dispatch-1",
			TransitionID:   "t-review",
			Workstation:    interfaces.FactoryWorkstationRef{ID: "t-review", Name: "Review"},
			Result:         interfaces.WorkstationResult{Outcome: "ACCEPTED", SelectedClassificationLabel: "approved"},
			DurationMillis: 250,
			OutputWork: []work.FactoryWorkItem{{
				ID:          "work-1",
				WorkTypeID:  "task",
				DisplayName: "Classifier terminal output",
				TraceID:     "trace-1",
				State:       "complete",
			}},
			TraceData: &interfaces.FactoryTraceData{TraceID: "trace-1", WorkIDs: []string{"work-1"}},
		}),
	}

	state, err := ReconstructFactoryWorldState(events, 3)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState: %v", err)
	}
	terminal, ok := state.TerminalWorkByID["work-1"]
	if !ok {
		t.Fatalf("TerminalWorkByID = %#v, want work-1 indexed from outputWork state", state.TerminalWorkByID)
	}
	if terminal.WorkItem.PlaceID != "task:complete" {
		t.Fatalf("terminal place = %q, want task:complete reconstructed from task+complete state", terminal.WorkItem.PlaceID)
	}
	if terminal.Status != "TERMINAL" {
		t.Fatalf("terminal status = %q, want TERMINAL", terminal.Status)
	}
}

func TestReconstructFactoryWorldState_WorkStateChangeMovesFromFailedToInProgress(t *testing.T) {
	t0 := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	events := []factoryapi.FactoryEvent{
		initialStructureEvent(t0),
		workInputEvent(1, t0.Add(time.Second), work.FactoryWorkItem{
			ID:          "work-recover",
			WorkTypeID:  "task",
			DisplayName: "Recover me",
			TraceID:     "trace-recover",
			PlaceID:     "task:init",
		}),
		workstationRequestEvent(2, t0.Add(2*time.Second), interfaces.WorkstationRequestPayload{
			DispatchID:   "dispatch-failed",
			TransitionID: "t-review",
			Workstation:  interfaces.FactoryWorkstationRef{ID: "t-review", Name: "Review"},
			Inputs: []interfaces.WorkstationInput{{
				TokenID:  "work-recover",
				PlaceID:  "task:init",
				WorkItem: &work.FactoryWorkItem{ID: "work-recover", WorkTypeID: "task", TraceID: "trace-recover", PlaceID: "task:init"},
			}},
		}),
		workstationResponseEvent(3, t0.Add(3*time.Second), interfaces.WorkstationResponsePayload{
			DispatchID:   "dispatch-failed",
			TransitionID: "t-review",
			Workstation:  interfaces.FactoryWorkstationRef{ID: "t-review", Name: "Review"},
			Result:       interfaces.WorkstationResult{Outcome: "FAILED", Error: "boom", FailureDetail: &workerexecution.FailureDetail{Reason: workerexecution.WorkFailureTypeUnknown, Message: "boom"}},
			Outputs: []interfaces.WorkstationOutput{{
				Type:     string(interfaces.MutationMove),
				TokenID:  "work-recover",
				ToPlace:  "task:failed",
				WorkItem: &work.FactoryWorkItem{ID: "work-recover", WorkTypeID: "task", TraceID: "trace-recover", PlaceID: "task:failed", State: "failed"},
			}},
			TraceData: &interfaces.FactoryTraceData{TraceID: "trace-recover", WorkIDs: []string{"work-recover"}},
		}),
		workStateChangeEvent(4, t0.Add(4*time.Second), "work-recover", "failed", "review", "task:failed", "task:review", factoryapi.WorkStateChangeSourceCLI),
	}

	failedState, err := ReconstructFactoryWorldState(events, 3)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState failed tick: %v", err)
	}
	if _, ok := failedState.FailedWorkItemsByID["work-recover"]; !ok {
		t.Fatalf("work-recover should be failed before operator move")
	}
	if got := failedState.WorkStateChangesByWorkID["work-recover"]; len(got) != 0 {
		t.Fatalf("work-recover move history before operator move = %#v, want empty", got)
	}

	recoveredState, err := ReconstructFactoryWorldState(events, 4)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState recovered tick: %v", err)
	}
	if _, ok := recoveredState.FailedWorkItemsByID["work-recover"]; ok {
		t.Fatalf("work-recover should leave failed index after operator move")
	}
	if _, ok := recoveredState.ActiveWorkItemsByID["work-recover"]; !ok {
		t.Fatalf("work-recover should be active after move to review")
	}
	if got := recoveredState.PlaceOccupancyByID["task:review"].WorkItemIDs; len(got) != 1 || got[0] != "work-recover" {
		t.Fatalf("task:review occupancy = %#v, want work-recover", got)
	}
	if got := recoveredState.PlaceOccupancyByID["task:failed"].WorkItemIDs; len(got) != 0 {
		t.Fatalf("task:failed occupancy = %#v, want empty after move", got)
	}
	if detail, ok := recoveredState.FailureDetailsByWorkID["work-recover"]; !ok || !worldFailureDetailHasReason(detail, workerexecution.WorkFailureTypeUnknown) {
		t.Fatalf("failure details = %#v, want retained history after leaving FAILED", recoveredState.FailureDetailsByWorkID["work-recover"])
	}
	item := recoveredState.WorkItemsByID["work-recover"]
	if item.PlaceID != "task:review" || item.State != "review" {
		t.Fatalf("work item = %#v, want place task:review and state review", item)
	}
	assertWorkStateChangeRecord(t, recoveredState.WorkStateChangesByWorkID["work-recover"], interfaces.FactoryWorldWorkStateChangeRecord{
		WorkID:       "work-recover",
		WorkTypeName: "task",
		FromState:    "failed",
		ToState:      "review",
		FromPlaceID:  "task:failed",
		ToPlaceID:    "task:review",
		Source:       work.WorkStateChangeSourceCLI,
		Tick:         4,
	})
	if len(recoveredState.WorkStateChangesByWorkID) != 1 {
		t.Fatalf("move history work ids = %d, want only work-recover", len(recoveredState.WorkStateChangesByWorkID))
	}
}

func TestReconstructFactoryWorldState_WorkStateChangeMovesFromInitialToArbitraryState(t *testing.T) {
	t0 := time.Date(2026, 5, 30, 13, 0, 0, 0, time.UTC)
	events := []factoryapi.FactoryEvent{
		initialStructureEvent(t0),
		workInputEvent(1, t0.Add(time.Second), work.FactoryWorkItem{
			ID:          "work-bootstrap",
			WorkTypeID:  "task",
			DisplayName: "Bootstrap",
			TraceID:     "trace-bootstrap",
			PlaceID:     "task:init",
			State:       "init",
		}),
		workStateChangeEvent(2, t0.Add(2*time.Second), "work-bootstrap", "init", "review", "task:init", "task:review", factoryapi.WorkStateChangeSourceAPI),
	}

	state, err := ReconstructFactoryWorldState(events, 2)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState: %v", err)
	}
	if got := state.PlaceOccupancyByID["task:review"].WorkItemIDs; len(got) != 1 || got[0] != "work-bootstrap" {
		t.Fatalf("task:review occupancy = %#v, want work-bootstrap", got)
	}
	if got := state.PlaceOccupancyByID["task:init"].WorkItemIDs; len(got) != 0 {
		t.Fatalf("task:init occupancy = %#v, want empty after move", got)
	}
	item := state.WorkItemsByID["work-bootstrap"]
	if item.PlaceID != "task:review" || item.State != "review" {
		t.Fatalf("work item = %#v, want place task:review and state review", item)
	}
	assertWorkStateChangeRecord(t, state.WorkStateChangesByWorkID["work-bootstrap"], interfaces.FactoryWorldWorkStateChangeRecord{
		WorkID:       "work-bootstrap",
		WorkTypeName: "task",
		FromState:    "init",
		ToState:      "review",
		FromPlaceID:  "task:init",
		ToPlaceID:    "task:review",
		Source:       work.WorkStateChangeSourceAPI,
		Tick:         2,
	})
	if len(state.WorkStateChangesByWorkID) != 1 {
		t.Fatalf("move history work ids = %d, want only work-bootstrap", len(state.WorkStateChangesByWorkID))
	}
}

func TestReconstructFactoryWorldState_LogicalMoveCronDispatchOmitsWorkerMetadata(t *testing.T) {
	t0 := time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC)
	workstationKind := factoryapi.WorkstationKindCron
	workstationType := factoryapi.WorkstationTypeLogicalMove
	events := []factoryapi.FactoryEvent{
		generatedProjectionEvent(
			factoryapi.FactoryEventTypeInitialStructureRequest,
			"initial-logical-cron",
			0,
			t0,
			factoryapi.FactoryEventContext{},
			factoryapi.InitialStructureRequestEventPayload{
				Factory: factoryapi.Factory{
					WorkTypes: &[]factoryapi.WorkType{
						{
							Name: "task",
							States: []factoryapi.WorkState{
								{Name: "init", Type: factoryapi.WorkStateTypeINITIAL},
								{Name: "done", Type: factoryapi.WorkStateTypeTERMINAL},
							},
						},
						{
							Name: interfaces.SystemTimeWorkTypeID,
							States: []factoryapi.WorkState{
								{Name: interfaces.SystemTimePendingState, Type: factoryapi.WorkStateTypePROCESSING},
							},
						},
					},
					Workstations: &[]factoryapi.Workstation{{
						Id:       stringPtrForProjectionTest("scheduled-route"),
						Name:     "scheduled-route",
						Behavior: &workstationKind,
						Type:     &workstationType,
						Worker:   "",
						Inputs:   []factoryapi.WorkstationIO{{WorkType: interfaces.SystemTimeWorkTypeID, State: interfaces.SystemTimePendingState}},
						Outputs:  &[]factoryapi.WorkstationIO{{WorkType: "task", State: "init"}},
					}},
				},
			},
		),
		workInputEventWithToken(1, t0.Add(time.Second), "time-route", work.FactoryWorkItem{
			ID:          "time-route",
			WorkTypeID:  interfaces.SystemTimeWorkTypeID,
			DisplayName: "scheduled-route tick",
			TraceID:     "trace-time",
			PlaceID:     interfaces.SystemTimePendingPlaceID,
			Tags: map[string]string{
				interfaces.TimeWorkTagKeyCronWorkstation: "scheduled-route",
			},
		}),
		workstationRequestEvent(2, t0.Add(2*time.Second), interfaces.WorkstationRequestPayload{
			DispatchID:   "dispatch-logical-cron",
			TransitionID: "scheduled-route",
			Workstation:  interfaces.FactoryWorkstationRef{ID: "scheduled-route", Name: "scheduled-route"},
			Inputs: []interfaces.WorkstationInput{{
				TokenID:  "time-route",
				PlaceID:  interfaces.SystemTimePendingPlaceID,
				WorkItem: &work.FactoryWorkItem{ID: "time-route", WorkTypeID: interfaces.SystemTimeWorkTypeID, TraceID: "trace-time", PlaceID: interfaces.SystemTimePendingPlaceID},
			}},
		}),
	}

	state, err := ReconstructFactoryWorldState(events, 2)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState: %v", err)
	}
	dispatch := state.ActiveDispatches["dispatch-logical-cron"]
	if dispatch.Provider != "" || dispatch.Model != "" {
		t.Fatalf("dispatch worker metadata = provider %q model %q, want empty", dispatch.Provider, dispatch.Model)
	}
	if dispatch.Workstation.Name != "scheduled-route" {
		t.Fatalf("workstation = %#v, want scheduled-route", dispatch.Workstation)
	}
}
