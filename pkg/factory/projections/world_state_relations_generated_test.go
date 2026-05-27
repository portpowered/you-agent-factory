package projections

import (
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestFactoryRelationsFromGenerated_PreservesRequestNameAndContextResolution(t *testing.T) {
	reducer := newFactoryWorldReducer(1)
	reducer.stateValue.WorkRequestsByID["request-1"] = interfaces.WorkRequestPayload{
		RequestID: "request-1",
		WorkItems: []interfaces.FactoryWorkItem{
			{ID: "work-parent", DisplayName: "parent"},
			{ID: "work-child", DisplayName: "child"},
			{ID: "work-prerequisite", DisplayName: "prerequisite"},
		},
	}

	relations := []factoryapi.Relation{
		{
			Type:           factoryapi.RelationTypeParentChild,
			SourceWorkName: "child",
			TargetWorkName: "parent",
		},
		{
			Type:           factoryapi.RelationTypeDependsOn,
			SourceWorkName: "",
			TargetWorkName: "prerequisite",
			RequiredState:  stringPtrForProjectionTest("complete"),
		},
		{
			Type:           factoryapi.RelationTypeDependsOn,
			SourceWorkName: "child",
			TargetWorkName: "missing",
		},
	}

	got := reducer.factoryRelationsFromGenerated(&relations, factoryapi.FactoryEventContext{
		RequestId: stringPtrForProjectionTest("request-1"),
		TraceIds:  &[]string{"trace-1"},
		WorkIds:   &[]string{"work-child", "work-prerequisite"},
	})

	if len(got) != 2 {
		t.Fatalf("converted relation count = %d, want 2 (%#v)", len(got), got)
	}
	if got[0].SourceWorkID != "work-child" || got[0].TargetWorkID != "work-parent" {
		t.Fatalf("first relation = %#v, want child -> parent resolved by request names", got[0])
	}
	if got[1].SourceWorkID != "work-child" || got[1].TargetWorkID != "work-prerequisite" || got[1].RequiredState != "complete" {
		t.Fatalf("second relation = %#v, want context fallback source and preserved required state", got[1])
	}
}

func TestFactoryRelationsFromGenerated_PreservesNilInput(t *testing.T) {
	reducer := newFactoryWorldReducer(1)

	if got := reducer.factoryRelationsFromGenerated(nil, factoryapi.FactoryEventContext{}); got != nil {
		t.Fatalf("nil relations = %#v, want nil", got)
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
	request := projectionReducerWorkstationRequestEvent(2, t0.Add(2*time.Second), interfaces.WorkstationRequestPayload{
		DispatchID:   "dispatch-1",
		TransitionID: "t-review",
		Workstation:  interfaces.FactoryWorkstationRef{ID: "t-review", Name: "Review"},
		Inputs: []interfaces.WorkstationInput{{
			TokenID:  "tok-task-1",
			PlaceID:  "task:init",
			WorkItem: &input,
		}},
	})
	response := projectionReducerWorkstationResponseEvent(3, t0.Add(3*time.Second), interfaces.WorkstationResponsePayload{
		DispatchID:      "dispatch-1",
		TransitionID:    "t-review",
		Workstation:     interfaces.FactoryWorkstationRef{ID: "t-review", Name: "Review"},
		Result:          interfaces.WorkstationResult{Outcome: "ACCEPTED"},
		DurationMillis:  800,
		TraceData:       &interfaces.FactoryTraceData{TraceID: "trace-1", WorkIDs: []string{"work-1"}},
		ProviderSession: &interfaces.ProviderSessionMetadata{Provider: "codex", Kind: "session_id", ID: "sess-1"},
	})
	inference := projectionReducerGeneratedEvent(
		factoryapi.FactoryEventTypeInferenceResponse,
		"inference-response/dispatch-1/inference-request/1",
		3,
		t0.Add(2500*time.Millisecond),
		factoryapi.FactoryEventContext{DispatchId: stringPtrForProjectionTest("dispatch-1")},
		factoryapi.InferenceResponseEventPayload{
			InferenceRequestId: "dispatch-1/inference-request/1",
			Attempt:            1,
			Outcome:            factoryapi.InferenceOutcomeSucceeded,
			DurationMillis:     700,
			ProviderSession:    projectionReducerProviderSession(&interfaces.ProviderSessionMetadata{Provider: "codex", Kind: "session_id", ID: "sess-1"}),
		},
	)

	reducer := newFactoryWorldReducer(3)
	for _, event := range []factoryapi.FactoryEvent{
		projectionReducerInitialStructureEvent(t0),
		projectionReducerWorkInputEvent(1, t0.Add(time.Second), "tok-task-1", input),
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

func projectionReducerInitialStructureEvent(eventTime time.Time) factoryapi.FactoryEvent {
	payload := factoryapi.InitialStructureRequestEventPayload{
		Factory: factoryapi.Factory{
			WorkTypes: &[]factoryapi.WorkType{{
				Name: "task",
				States: []factoryapi.WorkState{
					{Name: "init", Type: factoryapi.WorkStateTypeINITIAL},
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
	return projectionReducerGeneratedEvent(factoryapi.FactoryEventTypeInitialStructureRequest, "initial", 0, eventTime, factoryapi.FactoryEventContext{}, payload)
}

func projectionReducerWorkInputEvent(tick int, eventTime time.Time, _ string, item interfaces.FactoryWorkItem) factoryapi.FactoryEvent {
	requestID := "request/" + item.ID
	context := factoryapi.FactoryEventContext{
		RequestId: stringPtrForProjectionTest(requestID),
		TraceIds:  &[]string{item.TraceID},
		WorkIds:   &[]string{item.ID},
	}
	payload := factoryapi.WorkRequestEventPayload{
		Type:  factoryapi.WorkRequestTypeFactoryRequestBatch,
		Works: &[]factoryapi.Work{projectionReducerGeneratedWork(item, requestID)},
	}
	return projectionReducerGeneratedEvent(factoryapi.FactoryEventTypeWorkRequest, "work-input/"+item.ID, tick, eventTime, context, payload)
}

func projectionReducerWorkstationRequestEvent(tick int, eventTime time.Time, payload interfaces.WorkstationRequestPayload) factoryapi.FactoryEvent {
	works := make([]factoryapi.Work, 0, len(payload.Inputs))
	inputRefs := make([]factoryapi.DispatchConsumedWorkRef, 0, len(payload.Inputs))
	inputWorkItems := make([]interfaces.FactoryWorkItem, 0, len(payload.Inputs))
	for _, input := range payload.Inputs {
		if input.WorkItem == nil {
			continue
		}
		inputWorkItems = append(inputWorkItems, *input.WorkItem)
		works = append(works, projectionReducerGeneratedWork(*input.WorkItem, ""))
		inputRefs = append(inputRefs, factoryapi.DispatchConsumedWorkRef{WorkId: input.WorkItem.ID})
	}
	context := factoryapi.FactoryEventContext{
		DispatchId:               stringPtrForProjectionTest(payload.DispatchID),
		CurrentChainingTraceId:   stringPtrForProjectionTest(interfaces.CurrentChainingTraceIDFromWorkItems(inputWorkItems)),
		PreviousChainingTraceIds: stringSlicePtrForProjectionTest(interfaces.PreviousChainingTraceIDsFromWorkItems(inputWorkItems)),
		TraceIds:                 stringSlicePtrForProjectionTest(projectionReducerTraceIDs(works)),
		WorkIds:                  stringSlicePtrForProjectionTest(projectionReducerWorkIDs(works)),
	}
	generatedPayload := factoryapi.DispatchRequestEventPayload{
		TransitionId:             payload.TransitionID,
		CurrentChainingTraceId:   context.CurrentChainingTraceId,
		PreviousChainingTraceIds: context.PreviousChainingTraceIds,
		Inputs:                   inputRefs,
	}
	return projectionReducerGeneratedEvent(factoryapi.FactoryEventTypeDispatchRequest, "request/"+payload.DispatchID, tick, eventTime, context, generatedPayload)
}

func projectionReducerWorkstationResponseEvent(tick int, eventTime time.Time, payload interfaces.WorkstationResponsePayload) factoryapi.FactoryEvent {
	outputWork := projectionReducerGeneratedOutputWork(payload)
	context := factoryapi.FactoryEventContext{
		DispatchId: stringPtrForProjectionTest(payload.DispatchID),
		TraceIds:   stringSlicePtrForProjectionTest(projectionReducerTraceIDs(outputWork)),
		WorkIds:    stringSlicePtrForProjectionTest(projectionReducerWorkIDs(outputWork)),
	}
	if payload.TraceData != nil {
		context.TraceIds = stringSlicePtrForProjectionTest([]string{payload.TraceData.TraceID})
		context.WorkIds = stringSlicePtrForProjectionTest(payload.TraceData.WorkIDs)
	}
	generatedPayload := factoryapi.DispatchResponseEventPayload{
		TransitionId:   payload.TransitionID,
		Outcome:        factoryapi.WorkOutcome(payload.Result.Outcome),
		DurationMillis: int64PtrForProjectionTest(payload.DurationMillis),
		OutputWork:     &outputWork,
	}
	return projectionReducerGeneratedEvent(factoryapi.FactoryEventTypeDispatchResponse, "response/"+payload.DispatchID, tick, eventTime, context, generatedPayload)
}

func projectionReducerGeneratedEvent(eventType factoryapi.FactoryEventType, id string, tick int, eventTime time.Time, context factoryapi.FactoryEventContext, payload any) factoryapi.FactoryEvent {
	context.Tick = tick
	context.EventTime = eventTime
	event := factoryapi.FactoryEvent{
		Context:       context,
		Id:            id,
		SchemaVersion: factoryapi.AgentFactoryEventV1,
		Type:          eventType,
	}
	switch typed := payload.(type) {
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
	case factoryapi.InferenceResponseEventPayload:
		if err := event.Payload.FromInferenceResponseEventPayload(typed); err != nil {
			panic(err)
		}
	case factoryapi.DispatchResponseEventPayload:
		if err := event.Payload.FromDispatchResponseEventPayload(typed); err != nil {
			panic(err)
		}
	default:
		panic("unsupported projection reducer test payload")
	}
	return event
}

func projectionReducerGeneratedWork(item interfaces.FactoryWorkItem, requestID string) factoryapi.Work {
	return factoryapi.Work{
		Name:                     item.DisplayName,
		RequestId:                stringPtrForProjectionTest(requestID),
		Tags:                     projectionReducerStringMap(item.Tags),
		CurrentChainingTraceId:   stringPtrForProjectionTest(item.CurrentChainingTraceID),
		PreviousChainingTraceIds: stringSlicePtrForProjectionTest(item.PreviousChainingTraceIDs),
		TraceId:                  stringPtrForProjectionTest(item.TraceID),
		WorkId:                   stringPtrForProjectionTest(item.ID),
		WorkTypeName:             stringPtrForProjectionTest(item.WorkTypeID),
	}
}

func projectionReducerGeneratedOutputWork(payload interfaces.WorkstationResponsePayload) []factoryapi.Work {
	works := make([]factoryapi.Work, 0, len(payload.OutputWork)+len(payload.Outputs))
	for _, item := range payload.OutputWork {
		works = append(works, projectionReducerGeneratedWork(item, ""))
	}
	for _, output := range payload.Outputs {
		if output.WorkItem != nil {
			works = append(works, projectionReducerGeneratedWork(*output.WorkItem, ""))
		}
	}
	if payload.TerminalWork != nil {
		works = append(works, projectionReducerGeneratedWork(payload.TerminalWork.WorkItem, ""))
	}
	return works
}

func projectionReducerProviderSession(session *interfaces.ProviderSessionMetadata) *factoryapi.ProviderSessionMetadata {
	if session == nil {
		return nil
	}
	return &factoryapi.ProviderSessionMetadata{
		Id:       stringPtrForProjectionTest(session.ID),
		Kind:     stringPtrForProjectionTest(session.Kind),
		Provider: stringPtrForProjectionTest(session.Provider),
	}
}

func projectionReducerStringMap(values map[string]string) *factoryapi.StringMap {
	if len(values) == 0 {
		return nil
	}
	converted := factoryapi.StringMap(values)
	return &converted
}

func projectionReducerTraceIDs(works []factoryapi.Work) []string {
	ids := make([]string, 0, len(works))
	for _, work := range works {
		if work.TraceId != nil && *work.TraceId != "" {
			ids = append(ids, *work.TraceId)
		}
	}
	return ids
}

func projectionReducerWorkIDs(works []factoryapi.Work) []string {
	ids := make([]string, 0, len(works))
	for _, work := range works {
		if work.WorkId != nil && *work.WorkId != "" {
			ids = append(ids, *work.WorkId)
		}
	}
	return ids
}

func stringPtrForProjectionTest(value string) *string {
	if value == "" {
		return nil
	}
	return &value
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

func generatedStringMapForProjectionTest(values map[string]string) *factoryapi.StringMap {
	if len(values) == 0 {
		return nil
	}
	converted := factoryapi.StringMap(values)
	return &converted
}
