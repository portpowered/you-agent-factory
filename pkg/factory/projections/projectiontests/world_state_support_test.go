package projections_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	. "github.com/portpowered/infinite-you/pkg/factory/projections"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

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
	t0 := time.Date(2026, 4, 23, 11, 0, 0, 0, time.UTC)
	requestID := "request-json-relations"
	works := []factoryapi.Work{
		generatedWorkForProjectionTest(interfaces.FactoryWorkItem{ID: "work-draft", WorkTypeID: "task", DisplayName: "draft", TraceID: "trace-json-relations"}, requestID),
		generatedWorkForProjectionTest(interfaces.FactoryWorkItem{ID: "work-review", WorkTypeID: "task", DisplayName: "review", TraceID: "trace-json-relations"}, requestID),
	}
	relation := factoryapi.Relation{
		Type:           factoryapi.RelationTypeDependsOn,
		SourceWorkName: "review",
		TargetWorkName: "draft",
		TargetWorkId:   stringPtrForProjectionTest("work-draft"),
		RequiredState:  stringPtrForProjectionTest("done"),
	}
	events := []factoryapi.FactoryEvent{
		initialStructureEvent(t0),
		generatedProjectionEvent(factoryapi.FactoryEventTypeWorkRequest, "work-request/request-json-relations", 1, t0.Add(time.Second), factoryapi.FactoryEventContext{
			RequestId: stringPtrForProjectionTest(requestID),
			TraceIds:  &[]string{"trace-json-relations"},
			WorkIds:   &[]string{"work-draft", "work-review"},
		}, factoryapi.WorkRequestEventPayload{
			Type:      factoryapi.WorkRequestTypeFactoryRequestBatch,
			Works:     &works,
			Relations: &[]factoryapi.Relation{relation},
		}),
		relationshipChangeEvent(1, t0.Add(2*time.Second), requestID, "trace-json-relations", []string{"work-review", "work-draft"}, relation),
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
	request := state.WorkRequestsByID[requestID]
	if request.RequestID != requestID || request.Type != interfaces.WorkRequestTypeFactoryRequestBatch {
		t.Fatalf("decoded request = %#v, want canonical work-request metadata", request)
	}
	if len(request.WorkItems) != 2 || request.WorkItems[0].ID != "work-draft" || request.WorkItems[1].ID != "work-review" {
		t.Fatalf("decoded request work items = %#v, want draft and review preserved", request.WorkItems)
	}
	relations := state.RelationsByWorkID["work-review"]
	if len(relations) != 1 ||
		relations[0].RequestID != requestID ||
		relations[0].TraceID != "trace-json-relations" ||
		relations[0].TargetWorkID != "work-draft" ||
		relations[0].TargetWorkName != "draft" ||
		relations[0].RequiredState != "done" {
		t.Fatalf("decoded relations = %#v, want replayed canonical request dependency", relations)
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

func canonicalCompletedDispatchProjectionEvents(t0 time.Time) []factoryapi.FactoryEvent {
	return []factoryapi.FactoryEvent{
		factoryStateEvent(4, t0.Add(4*time.Second), "RUNNING", "COMPLETED"),
		workstationResponseEvent(3, t0.Add(3*time.Second), interfaces.WorkstationResponsePayload{
			DispatchID:     "dispatch-1",
			TransitionID:   "t-review",
			Workstation:    interfaces.FactoryWorkstationRef{ID: "t-review", Name: "Review"},
			Result:         interfaces.WorkstationResult{Outcome: "ACCEPTED", SelectedClassificationLabel: "approved"},
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
	assertCanonicalCompletedDispatchRecord(t, state.CompletedDispatches[0])
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

func assertCanonicalCompletedDispatchRecord(t *testing.T, dispatch interfaces.FactoryWorldDispatchCompletion) {
	t.Helper()

	if dispatch.DispatchID != "dispatch-1" || dispatch.StartedTick != 2 {
		t.Fatalf("completion = %#v, want dispatch-1 started at tick 2", dispatch)
	}
	if got := dispatch.Result.SelectedClassificationLabel; got != "approved" {
		t.Fatalf("completion selected classification label = %q, want approved", got)
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
			Variables: map[string]string{
				"prompt_source":  "factory-renderer",
				"work_type_name": "task",
			},
		},
		Provider: &interfaces.SafeProviderDiagnostic{
			Provider: "codex",
			Model:    "gpt-5.4",
			RequestMetadata: map[string]string{
				"worker_type": "builder",
			},
			ResponseMetadata: map[string]string{
				"provider_session_id": "resp-1",
				"retry_count":         "1",
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
				Outputs:   &[]factoryapi.WorkstationIO{{WorkType: "task", State: "complete"}},
				OnFailure: &[]factoryapi.WorkstationIO{{WorkType: "task", State: "failed"}},
			}},
		},
	}
	return generatedProjectionEvent(factoryapi.FactoryEventTypeInitialStructureRequest, "initial", 0, eventTime, factoryapi.FactoryEventContext{}, payload)
}

func initialStructureEventWithNonSuccessRouteArrays(eventTime time.Time) factoryapi.FactoryEvent {
	payload := factoryapi.InitialStructureRequestEventPayload{
		Factory: factoryapi.Factory{
			WorkTypes: &[]factoryapi.WorkType{{
				Name: "task",
				States: []factoryapi.WorkState{
					{Name: "init", Type: factoryapi.WorkStateTypeINITIAL},
					{Name: "retry", Type: factoryapi.WorkStateTypePROCESSING},
					{Name: "triage", Type: factoryapi.WorkStateTypePROCESSING},
					{Name: "complete", Type: factoryapi.WorkStateTypeTERMINAL},
					{Name: "failed", Type: factoryapi.WorkStateTypeFAILED},
					{Name: "abandoned", Type: factoryapi.WorkStateTypeFAILED},
				},
			}},
			Workstations: &[]factoryapi.Workstation{{
				Id:          stringPtrForProjectionTest("t-review"),
				Name:        "Review",
				Worker:      "reviewer",
				Inputs:      []factoryapi.WorkstationIO{{WorkType: "task", State: "init"}},
				Outputs:     &[]factoryapi.WorkstationIO{{WorkType: "task", State: "complete"}},
				OnContinue:  &[]factoryapi.WorkstationIO{{WorkType: "task", State: "retry"}, {WorkType: "task", State: "init"}},
				OnRejection: &[]factoryapi.WorkstationIO{{WorkType: "task", State: "triage"}, {WorkType: "task", State: "init"}},
				OnFailure:   &[]factoryapi.WorkstationIO{{WorkType: "task", State: "failed"}, {WorkType: "task", State: "abandoned"}},
			}},
		},
	}
	return generatedProjectionEvent(factoryapi.FactoryEventTypeInitialStructureRequest, "initial-non-success-routes", 0, eventTime, factoryapi.FactoryEventContext{}, payload)
}

func factoryChangeEvent(eventTime time.Time) factoryapi.FactoryEvent {
	payload := factoryapi.FactoryChangeEventPayload{
		Factory: factoryapi.Factory{
			WorkTypes: &[]factoryapi.WorkType{{
				Name: "story",
				States: []factoryapi.WorkState{
					{Name: "new", Type: factoryapi.WorkStateTypeINITIAL},
					{Name: "done", Type: factoryapi.WorkStateTypeTERMINAL},
				},
			}},
			Workstations: &[]factoryapi.Workstation{{
				Id:      stringPtrForProjectionTest("t-plan"),
				Name:    "Plan",
				Worker:  "planner",
				Inputs:  []factoryapi.WorkstationIO{{WorkType: "story", State: "new"}},
				Outputs: &[]factoryapi.WorkstationIO{{WorkType: "story", State: "done"}},
			}},
		},
	}
	return generatedProjectionEvent(factoryapi.FactoryEventTypeFactoryChange, "factory-change", 1, eventTime, factoryapi.FactoryEventContext{}, payload)
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
				Outputs:   &[]factoryapi.WorkstationIO{{WorkType: "task", State: "complete"}},
				OnFailure: &[]factoryapi.WorkstationIO{{WorkType: "task", State: "failed"}},
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
		TransitionId:                payload.TransitionID,
		Outcome:                     outcome,
		Output:                      stringPtrForProjectionTest(payload.Result.Output),
		Error:                       stringPtrForProjectionTest(payload.Result.Error),
		Feedback:                    stringPtrForProjectionTest(payload.Result.Feedback),
		SelectedClassificationLabel: stringPtrForProjectionTest(payload.Result.SelectedClassificationLabel),
		FailureReason:               stringPtrForProjectionTest(payload.Result.FailureReason),
		FailureMessage:              stringPtrForProjectionTest(payload.Result.FailureMessage),
		ProviderFailure:             generatedProviderFailureForProjectionTest(payload.Result.ProviderFailure),
		DurationMillis:              int64PtrForProjectionTest(payload.DurationMillis),
		OutputWork:                  &outputWork,
		OutputResources:             generatedResourcesForProjectionTest(payload.OutputResources),
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

// pkgmaintcheck:ignore-cyclomatic-complexity this event fixture builder keeps the supported replay payload variants on one canonical helper.
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
	case factoryapi.FactoryChangeEventPayload:
		if err := event.Payload.FromFactoryChangeEventPayload(typed); err != nil {
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
		ChainingTraceDepth:       intPtrForProjectionTest(item.ChainingTraceDepth),
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
		Family: workFailureFamilyPtrForProjectionTest(failure.Family),
		Type:   workFailureTypePtrForProjectionTest(failure.Type),
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

func workFailureFamilyPtrForProjectionTest(value interfaces.WorkFailureFamily) *factoryapi.WorkFailureFamily {
	if value == "" {
		return nil
	}
	typed := factoryapi.WorkFailureFamily(value)
	return &typed
}

func workFailureTypePtrForProjectionTest(value interfaces.WorkFailureType) *factoryapi.WorkFailureType {
	if value == "" {
		return nil
	}
	typed := factoryapi.WorkFailureType(value)
	return &typed
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
