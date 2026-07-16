package projections

import (
	"encoding/json"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
)

func TestFactoryRelationsFromRequest_PreservesRequestNameAndContextResolution(t *testing.T) {
	reducer := newFactoryWorldReducer(1)
	reducer.stateValue.WorkRequestsByID["request-1"] = interfaces.WorkRequestPayload{
		RequestID: "request-1",
		WorkItems: []work.FactoryWorkItem{
			{ID: "work-parent", DisplayName: "parent"},
			{ID: "work-child", DisplayName: "child"},
			{ID: "work-prerequisite", DisplayName: "prerequisite"},
		},
	}

	relations := []work.WorkRequestEventRelation{
		{
			Type:           work.WorkRelationType("PARENT_CHILD"),
			SourceWorkName: "child",
			TargetWorkName: "parent",
		},
		{
			Type:           work.WorkRelationType("DEPENDS_ON"),
			SourceWorkName: "",
			TargetWorkName: "prerequisite",
			RequiredState:  "complete",
		},
		{
			Type:           work.WorkRelationType("DEPENDS_ON"),
			SourceWorkName: "child",
			TargetWorkName: "missing",
		},
	}

	got := reducer.factoryRelationsFromRequest(relations, interfaces.FactoryEventContext{
		RequestID: stringPtrForProjectionTest("request-1"),
		TraceIDs:  &[]string{"trace-1"},
		WorkIDs:   &[]string{"work-child", "work-prerequisite"},
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

func TestFactoryRelationsFromRequest_PreservesNilInput(t *testing.T) {
	reducer := newFactoryWorldReducer(1)

	if got := reducer.factoryRelationsFromRequest(nil, interfaces.FactoryEventContext{}); got != nil {
		t.Fatalf("nil relations = %#v, want nil", got)
	}
}

func TestFactoryWorldReducer_RemoveTokenCleansWorkIndexes(t *testing.T) {
	reducer := newFactoryWorldReducer(0)
	firstItem := work.FactoryWorkItem{ID: "work-1", WorkTypeID: "task", TraceID: "trace-1", PlaceID: "task:init"}
	secondItem := work.FactoryWorkItem{ID: "work-2", WorkTypeID: "task", TraceID: "trace-2", PlaceID: "task:init"}

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

func TestFactoryWorldReducer_AppliesCanonicalWorkerExecutionEvents(t *testing.T) {
	t0 := time.Date(2026, 7, 16, 1, 30, 0, 0, time.UTC)
	dispatchID := "dispatch-1"
	context := interfaces.FactoryEventContext{DispatchID: &dispatchID, Tick: 3, EventTime: t0}
	response := "finished"
	exitCode := 0
	failureType := workerexecution.ScriptFailureTypeTimeout
	providerSession := &workerexecution.ProviderSessionMetadata{Provider: "codex", Kind: "session_id", ID: "session-1"}
	diagnostics := json.RawMessage(`{"provider":{"provider":"codex","model":"gpt-5"}}`)
	events := []interfaces.FactoryEvent{
		canonicalWorldProjectionEvent(t, interfaces.FactoryEventTypeInferenceRequest, context, workerexecution.InferenceRequestEventPayload{
			Attempt: 2, InferenceRequestID: "inference-1", Prompt: "review", WorkingDirectory: "/workspace", Worktree: "branch-a",
		}),
		canonicalWorldProjectionEvent(t, interfaces.FactoryEventTypeInferenceResponse, context, workerexecution.InferenceResponseEventPayload{
			Attempt: 2, InferenceRequestID: "inference-1", Outcome: workerexecution.InferenceOutcomeSucceeded,
			Response: &response, DurationMillis: 1250, ExitCode: &exitCode, ProviderSession: providerSession, Diagnostics: diagnostics,
		}),
		canonicalWorldProjectionEvent(t, interfaces.FactoryEventTypeScriptRequest, context, workerexecution.ScriptRequestEventPayload{
			Attempt: 1, DispatchID: dispatchID, TransitionID: "review", ScriptRequestID: "script-1", Command: "go", Args: []string{"test", "./..."},
		}),
		canonicalWorldProjectionEvent(t, interfaces.FactoryEventTypeScriptResponse, context, workerexecution.ScriptResponseEventPayload{
			Attempt: 1, DispatchID: dispatchID, TransitionID: "review", ScriptRequestID: "script-1",
			Outcome: workerexecution.ScriptExecutionOutcomeTimedOut, Stdout: "partial", Stderr: "deadline", DurationMillis: 5000,
			ExitCode: &exitCode, FailureType: &failureType,
		}),
		canonicalWorldProjectionEvent(t, interfaces.FactoryEventTypeAgentRunResponse, context, workerexecution.AgentRunResponseEventPayload{
			AgentRunID: "agent-run-1", Outcome: "SUCCEEDED", DurationMillis: 1400, Diagnostics: diagnostics,
		}),
	}

	reducer := newFactoryWorldReducer(3)
	for _, event := range events {
		if err := reducer.applyWorkerExecutionEvent(event); err != nil {
			t.Fatalf("apply canonical %s event: %v", event.Type, err)
		}
	}
	assertCanonicalInferenceProjection(t, reducer, dispatchID, response)
	assertCanonicalScriptProjection(t, reducer, dispatchID)
	assertCanonicalAgentRunProjection(t, reducer, dispatchID)
}

func assertCanonicalInferenceProjection(t *testing.T, reducer *factoryWorldReducer, dispatchID string, response string) {
	t.Helper()
	attempt := reducer.stateValue.InferenceAttemptsByDispatchID[dispatchID]["inference-1"]
	if attempt.Attempt != 2 || attempt.Prompt != "review" || attempt.Response != response || attempt.DurationMillis != 1250 {
		t.Fatalf("inference attempt = %#v, want canonical request and response facts", attempt)
	}
	if attempt.ProviderSession == nil || attempt.ProviderSession.ID != "session-1" || attempt.Diagnostics == nil || attempt.Diagnostics.Provider == nil || attempt.Diagnostics.Provider.Model != "gpt-5" {
		t.Fatalf("inference diagnostics = %#v, provider session = %#v", attempt.Diagnostics, attempt.ProviderSession)
	}
}

func assertCanonicalScriptProjection(t *testing.T, reducer *factoryWorldReducer, dispatchID string) {
	t.Helper()
	request := reducer.stateValue.ScriptRequestsByDispatchID[dispatchID]["script-1"]
	if request.Command != "go" || len(request.Args) != 2 || request.Args[1] != "./..." {
		t.Fatalf("script request = %#v, want canonical command facts", request)
	}
	scriptResponse := reducer.stateValue.ScriptResponsesByDispatchID[dispatchID]["script-1"]
	if scriptResponse.Outcome != "TIMED_OUT" || scriptResponse.FailureType != "TIMEOUT" || scriptResponse.Stderr != "deadline" {
		t.Fatalf("script response = %#v, want canonical timeout facts", scriptResponse)
	}
}

func assertCanonicalAgentRunProjection(t *testing.T, reducer *factoryWorldReducer, dispatchID string) {
	t.Helper()
	agentResponse := reducer.stateValue.AgentRunResponsesByDispatchID[dispatchID]["agent-run-1"]
	if agentResponse.Outcome != "SUCCEEDED" || agentResponse.Diagnostics == nil || agentResponse.Diagnostics.Provider == nil || agentResponse.Diagnostics.Provider.Provider != "codex" {
		t.Fatalf("agent response = %#v, want canonical result and diagnostics", agentResponse)
	}
}

func TestFactoryWorldReducer_RejectsMalformedCanonicalWorkerDiagnostics(t *testing.T) {
	dispatchID := "dispatch-1"
	event := canonicalWorldProjectionEvent(t, interfaces.FactoryEventTypeInferenceResponse, interfaces.FactoryEventContext{
		DispatchID: &dispatchID,
	}, workerexecution.InferenceResponseEventPayload{
		InferenceRequestID: "inference-1",
		Diagnostics:        json.RawMessage(`"not-an-object"`),
	})
	reducer := newFactoryWorldReducer(0)

	if err := reducer.applyWorkerExecutionEvent(event); err == nil {
		t.Fatal("apply malformed canonical inference diagnostics error = nil, want decode failure")
	}
	if len(reducer.stateValue.InferenceAttemptsByDispatchID[dispatchID]) != 0 {
		t.Fatalf("inference attempts = %#v, want no projection mutation after decode failure", reducer.stateValue.InferenceAttemptsByDispatchID[dispatchID])
	}
}

func TestFactoryWorldReducer_DetachesCompletedConsumedInputsFromDispatchSource(t *testing.T) {
	t0 := time.Date(2026, 5, 19, 9, 0, 0, 0, time.UTC)
	input := work.FactoryWorkItem{
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
		ProviderSession: &workerexecution.ProviderSessionMetadata{Provider: "codex", Kind: "session_id", ID: "sess-1"},
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
			ProviderSession:    projectionReducerProviderSession(&workerexecution.ProviderSessionMetadata{Provider: "codex", Kind: "session_id", ID: "sess-1"}),
		},
	)

	reducer := newFactoryWorldReducer(3)
	for _, event := range []factoryapi.FactoryEvent{
		projectionReducerInitialStructureEvent(t0),
		projectionReducerWorkInputEvent(1, t0.Add(time.Second), "tok-task-1", input),
		request,
	} {
		if err := reducer.apply(mustCanonicalProjectionEvent(t, event)); err != nil {
			t.Fatalf("apply pre-completion event %q: %v", event.Type, err)
		}
	}

	dispatchSource := reducer.stateValue.ActiveDispatches["dispatch-1"]
	if len(dispatchSource.Inputs) != 1 || dispatchSource.Inputs[0].WorkItem == nil {
		t.Fatalf("dispatch source inputs = %#v, want one traced work item", dispatchSource.Inputs)
	}

	if err := reducer.apply(mustCanonicalProjectionEvent(t, inference)); err != nil {
		t.Fatalf("apply inference event: %v", err)
	}
	if err := reducer.apply(mustCanonicalProjectionEvent(t, response)); err != nil {
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

func mustCanonicalProjectionEvent(t *testing.T, event factoryapi.FactoryEvent) interfaces.FactoryEvent {
	t.Helper()
	canonicalEvent, err := interfaces.NewFactoryEvent(event)
	if err != nil {
		t.Fatalf("convert projection event %q: %v", event.Type, err)
	}
	return canonicalEvent
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

func projectionReducerWorkInputEvent(tick int, eventTime time.Time, _ string, item work.FactoryWorkItem) factoryapi.FactoryEvent {
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
	inputWorkItems := make([]work.FactoryWorkItem, 0, len(payload.Inputs))
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
		CurrentChainingTraceId:   stringPtrForProjectionTest(work.CurrentChainingTraceIDFromWorkItems(inputWorkItems)),
		PreviousChainingTraceIds: stringSlicePtrForProjectionTest(work.PreviousChainingTraceIDsFromWorkItems(inputWorkItems)),
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

func projectionReducerGeneratedWork(item work.FactoryWorkItem, requestID string) factoryapi.Work {
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

func projectionReducerProviderSession(session *workerexecution.ProviderSessionMetadata) *factoryapi.ProviderSessionMetadata {
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

func TestWorkItemRefsForProjectionOwners_FilterCustomerWorkAndPreserveLineage(t *testing.T) {
	itemsByID := map[string]work.FactoryWorkItem{
		"work-2": {ID: "work-2", WorkTypeID: "task", DisplayName: "Second", CurrentChainingTraceID: "chain-2", PreviousChainingTraceIDs: []string{"chain-0", "chain-1"}, TraceID: "trace-2"},
		"work-1": {ID: "work-1", WorkTypeID: "task", DisplayName: "First", CurrentChainingTraceID: "chain-1", PreviousChainingTraceIDs: []string{"chain-0"}, TraceID: "trace-1"},
		"time-1": {ID: "time-1", WorkTypeID: interfaces.SystemTimeWorkTypeID, DisplayName: "tick"},
	}

	refsByID := workItemRefsForIDs(work.WorkPayloadLineageProjection{}, []string{"work-2", "time-1", "work-1", "work-2"}, itemsByID)
	if len(refsByID) != 2 || refsByID[0].WorkID != "work-1" || refsByID[1].WorkID != "work-2" {
		t.Fatalf("workItemRefsForIDs = %#v, want sorted customer refs", refsByID)
	}
	if refsByID[0].CurrentChainingTraceID != "chain-1" || len(refsByID[1].PreviousChainingTraceIDs) != 2 {
		t.Fatalf("workItemRefsForIDs lineage = %#v, want explicit chaining fields", refsByID)
	}
	if refsByID[0].ChainingTraceDepth != 0 || refsByID[1].ChainingTraceDepth != 0 {
		t.Fatalf("workItemRefsForIDs unexpected implicit depth = %#v, want zero when source depth absent", refsByID)
	}

	refsForItems := workItemRefsForItems(work.WorkPayloadLineageProjection{}, []work.FactoryWorkItem{
		itemsByID["work-2"],
		itemsByID["time-1"],
		itemsByID["work-2"],
		itemsByID["work-1"],
	})
	if len(refsForItems) != 2 || refsForItems[0].WorkID != "work-2" || refsForItems[1].WorkID != "work-1" {
		t.Fatalf("workItemRefsForItems = %#v, want first-occurrence customer refs", refsForItems)
	}

	refsForInputs := workItemRefsForInputs(work.WorkPayloadLineageProjection{}, []interfaces.WorkstationInput{
		{WorkItem: &work.FactoryWorkItem{ID: "work-1", WorkTypeID: "task", DisplayName: "First", CurrentChainingTraceID: "chain-1", PreviousChainingTraceIDs: []string{"chain-0"}}},
		{WorkItem: &work.FactoryWorkItem{ID: "time-1", WorkTypeID: interfaces.SystemTimeWorkTypeID, DisplayName: "tick"}},
		{WorkItem: &work.FactoryWorkItem{ID: "work-1", WorkTypeID: "task", DisplayName: "First", CurrentChainingTraceID: "chain-1", PreviousChainingTraceIDs: []string{"chain-0"}}},
		{WorkItem: &work.FactoryWorkItem{ID: "work-2", WorkTypeID: "task", DisplayName: "Second", CurrentChainingTraceID: "chain-2", PreviousChainingTraceIDs: []string{"chain-0", "chain-1"}}},
	})
	if len(refsForInputs) != 2 || refsForInputs[0].WorkID != "work-1" || refsForInputs[1].WorkID != "work-2" {
		t.Fatalf("workItemRefsForInputs = %#v, want first-occurrence customer refs", refsForInputs)
	}
}

func TestCanonicalSessionLifecycleEventsReconstructBracketAndResult(t *testing.T) {
	t.Parallel()
	startedAt := time.Date(2026, time.July, 15, 20, 0, 0, 0, time.FixedZone("fixture", -7*60*60))
	completedAt := startedAt.Add(3 * time.Second)
	durationMillis := int64(3000)
	resultStatus := interfaces.FactorySessionResultStatusFinal
	sessionID, orchestratorKind := "session-1", "javascript"
	reducer := newFactoryWorldReducer(3)

	events := []interfaces.FactoryEvent{
		canonicalWorldProjectionEvent(t, interfaces.FactoryEventTypeSessionStarted, interfaces.FactoryEventContext{
			SessionID: &sessionID, OrchestratorKind: &orchestratorKind, EventTime: startedAt,
		}, interfaces.FactorySessionStartedEventPayload{FactoryID: stringPtrForProjectionTest("factory-1"), StartedAt: startedAt}),
		canonicalWorldProjectionEvent(t, interfaces.FactoryEventTypeSessionResultUpdated, interfaces.FactoryEventContext{
			SessionID: &sessionID, EventTime: startedAt.Add(time.Second),
		}, interfaces.FactorySessionResultUpdatedEventPayload{
			ArtifactIDs:  []string{"artifact-1"},
			ResultStatus: resultStatus,
			ResultSummary: []work.WorkContentPart{{
				Type: work.WorkContentPartTypeText, Text: "done",
			}},
		}),
		canonicalWorldProjectionEvent(t, interfaces.FactoryEventTypeSessionCompleted, interfaces.FactoryEventContext{
			SessionID: &sessionID, EventTime: completedAt,
		}, interfaces.FactorySessionCompletedEventPayload{
			ArtifactIDs:    []string{"artifact-1"},
			CompletedAt:    completedAt,
			DurationMillis: &durationMillis,
			FinalStatus:    interfaces.FactorySessionLifecycleStatusSucceeded,
			ResultStatus:   &resultStatus,
		}),
	}
	for _, event := range events {
		handled, err := reducer.applySessionLifecycleEvent(event)
		if err != nil || !handled {
			t.Fatalf("applySessionLifecycleEvent(%s) = handled %t, err %v", event.Type, handled, err)
		}
	}

	bracket := reducer.stateValue.SessionBracket
	if bracket == nil || bracket.SessionID != sessionID || !bracket.Terminal {
		t.Fatalf("session bracket = %#v, want terminal %q bracket", bracket, sessionID)
	}
	if bracket.StartedAt != startedAt.UTC() || bracket.CompletedAt != completedAt.UTC() {
		t.Fatalf("session bracket times = %s..%s, want UTC %s..%s", bracket.StartedAt, bracket.CompletedAt, startedAt.UTC(), completedAt.UTC())
	}
	if bracket.ResultStatus != string(resultStatus) || len(bracket.ResultSummary) != 1 || bracket.ResultSummary[0].Text != "done" {
		t.Fatalf("session result = status %q summary %#v", bracket.ResultStatus, bracket.ResultSummary)
	}
	if reducer.stateValue.JavaScriptRuntime == nil || reducer.stateValue.JavaScriptRuntime.PrimaryResult[0].Text != "done" {
		t.Fatalf("javascript runtime result = %#v", reducer.stateValue.JavaScriptRuntime)
	}
}

func TestCanonicalWorkEventsReconstructRequestAndRelationships(t *testing.T) {
	t.Parallel()
	eventTime := time.Date(2026, time.July, 16, 3, 0, 0, 0, time.UTC)
	requestID, traceID := "request-1", "trace-1"
	reducer := newFactoryWorldReducer(2)
	requestEvent := canonicalWorldProjectionEvent(t, interfaces.FactoryEventTypeWorkRequest, interfaces.FactoryEventContext{
		EventTime: eventTime, RequestID: &requestID, Tick: 1, TraceIDs: &[]string{traceID},
	}, work.WorkRequestEventPayload{
		Source: "operator", Type: work.WorkRequestTypeFactoryRequestBatch,
		ParentLineage: []string{"parent-request"},
		Works: []work.WorkRequestEventWork{
			{Name: "parent", WorkID: "work-parent", WorkTypeID: "task", Tags: map[string]string{"role": "parent"}},
			{Name: "child", WorkID: "work-child", WorkTypeID: "task", Content: []work.WorkContentPart{{Type: work.WorkContentPartType("TEXT"), Text: "draft"}}},
		},
		Relations: []work.WorkRequestEventRelation{{
			Type: work.WorkRelationType("PARENT_CHILD"), SourceWorkName: "child", TargetWorkName: "parent",
		}},
	})
	if err := reducer.applyWorkRequestEvent(requestEvent); err != nil {
		t.Fatalf("applyWorkRequestEvent: %v", err)
	}

	changeEvent := canonicalWorldProjectionEvent(t, interfaces.FactoryEventTypeRelationshipChangeRequest, interfaces.FactoryEventContext{
		EventTime: eventTime.Add(time.Second), RequestID: &requestID, Tick: 2, TraceIDs: &[]string{traceID}, WorkIDs: &[]string{"work-child", "work-prerequisite"},
	}, work.RelationshipChangeRequestEventPayload{Relation: work.WorkRequestEventRelation{
		Type: work.WorkRelationType("DEPENDS_ON"), TargetWorkID: "work-prerequisite", TargetWorkName: "prerequisite", RequiredState: "complete",
	}})
	if err := reducer.applyRelationshipChangeEvent(changeEvent); err != nil {
		t.Fatalf("applyRelationshipChangeEvent: %v", err)
	}

	request := reducer.stateValue.WorkRequestsByID[requestID]
	if request.Source != "operator" || request.TraceID != traceID || len(request.WorkItems) != 2 {
		t.Fatalf("request projection = %#v", request)
	}
	child := reducer.stateValue.WorkItemsByID["work-child"]
	if child.TraceID != traceID || len(child.Content) != 1 || child.Content[0].Type != work.WorkContentPartTypeText || child.Content[0].Text != "draft" {
		t.Fatalf("child projection = %#v", child)
	}
	relations := reducer.stateValue.RelationsByWorkID["work-child"]
	if len(relations) != 2 || relations[0].TargetWorkID != "work-parent" || relations[1].TargetWorkID != "work-prerequisite" || relations[1].RequiredState != "complete" {
		t.Fatalf("relationship projection = %#v", relations)
	}
}

func TestCanonicalDispatchRequestReconstructsActiveDispatchAndConsumedLineage(t *testing.T) {
	t.Parallel()
	eventTime := time.Date(2026, time.July, 16, 4, 0, 0, 0, time.UTC)
	requestID, traceID, dispatchID := "request-1", "trace-1", "dispatch-1"
	currentTraceID := "chain-current"
	reducer := newFactoryWorldReducer(2)
	reducer.applyInitialStructure(interfaces.InitialStructurePayload{
		Resources: []interfaces.FactoryResource{{ID: "gpu", Name: "gpu", Capacity: 1}},
		Workers: []interfaces.FactoryWorker{{
			ID: "worker-1", Provider: "provider-1", Model: "model-1",
		}},
		Workstations: []interfaces.FactoryWorkstation{{
			ID: "review", Name: "Review", WorkerID: "worker-1", InputPlaceIDs: []string{"task:ready"},
		}},
		Places: []interfaces.FactoryPlace{
			{ID: "gpu:available", TypeID: "gpu", State: "available", Category: "PROCESSING"},
			{ID: "task:ready", TypeID: "task", State: "ready", Category: "INITIAL"},
		},
	})
	requestEvent := canonicalWorldProjectionEvent(t, interfaces.FactoryEventTypeWorkRequest, interfaces.FactoryEventContext{
		EventTime: eventTime, RequestID: &requestID, Tick: 1, TraceIDs: &[]string{traceID},
	}, work.WorkRequestEventPayload{
		Type: work.WorkRequestTypeFactoryRequestBatch,
		Works: []work.WorkRequestEventWork{{
			WorkID: "work-1", WorkTypeID: "task", Name: "Draft", Content: []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "payload"}},
		}},
	})
	if err := reducer.applyWorkRequestEvent(requestEvent); err != nil {
		t.Fatalf("applyWorkRequestEvent: %v", err)
	}

	dispatchEvent := canonicalWorldProjectionEvent(t, interfaces.FactoryEventTypeDispatchRequest, interfaces.FactoryEventContext{
		CurrentChainingTraceID:   &currentTraceID,
		DispatchID:               &dispatchID,
		EventTime:                eventTime.Add(time.Second),
		PreviousChainingTraceIDs: &[]string{"chain-parent"},
		Tick:                     2,
		TraceIDs:                 &[]string{traceID},
		WorkIDs:                  &[]string{"work-1"},
	}, interfaces.DispatchRequestEventPayload{
		CurrentChainingTraceID:   stringPtrForProjectionTest("payload-current"),
		Inputs:                   []interfaces.DispatchConsumedWorkRef{{WorkID: "work-1"}},
		PreviousChainingTraceIDs: &[]string{"payload-parent"},
		Resources:                &[]interfaces.DispatchResourceRef{{Name: "gpu", Capacity: 1}},
		TransitionID:             "review",
	})
	if err := reducer.applyDispatchRequestEvent(dispatchEvent); err != nil {
		t.Fatalf("applyDispatchRequestEvent: %v", err)
	}
	assertCanonicalDispatchMetadata(t, reducer, dispatchID, currentTraceID)
	assertCanonicalDispatchSideEffects(t, reducer, dispatchID)
}

func assertCanonicalDispatchMetadata(t *testing.T, reducer *factoryWorldReducer, dispatchID, currentTraceID string) {
	t.Helper()
	dispatch := reducer.stateValue.ActiveDispatches[dispatchID]
	if dispatch.TransitionID != "review" || dispatch.Workstation.Name != "Review" || dispatch.Provider != "provider-1" || dispatch.Model != "model-1" {
		t.Fatalf("active dispatch metadata = %#v", dispatch)
	}
	if dispatch.CurrentChainingTraceID != currentTraceID || len(dispatch.PreviousChainingTraceIDs) != 1 || dispatch.PreviousChainingTraceIDs[0] != "chain-parent" {
		t.Fatalf("active dispatch chaining trace = current %q previous %#v", dispatch.CurrentChainingTraceID, dispatch.PreviousChainingTraceIDs)
	}
	if len(dispatch.Inputs) != 1 || dispatch.Inputs[0].WorkItem == nil || dispatch.Inputs[0].WorkItem.Content[0].Text != "payload" {
		t.Fatalf("active dispatch inputs = %#v", dispatch.Inputs)
	}
}

func assertCanonicalDispatchSideEffects(t *testing.T, reducer *factoryWorldReducer, dispatchID string) {
	t.Helper()
	dispatch := reducer.stateValue.ActiveDispatches[dispatchID]
	if len(dispatch.Resources) != 1 || dispatch.Resources[0].ResourceID != "gpu" || dispatch.Resources[0].TokenID == "" {
		t.Fatalf("active dispatch resources = %#v", dispatch.Resources)
	}
	if _, available := reducer.tokenPlaces[resourceTokenID("gpu", 0)]; available {
		t.Fatal("consumed gpu token remains available")
	}
	resolution := reducer.stateValue.PayloadLineage.ResolveConsumedInputSnapshot(dispatchID, "work-1")
	if resolution.Status != work.WorkPayloadResolutionResolved || resolution.Snapshot == nil || resolution.Snapshot.WorkItem.Content[0].Text != "payload" {
		t.Fatalf("consumed payload lineage = %#v", resolution)
	}
}

func TestCanonicalOrchestratorProgressEventsReconstructPhaseAndCheckpoint(t *testing.T) {
	t.Parallel()
	eventTime := time.Date(2026, time.July, 16, 1, 0, 0, 0, time.UTC)
	phaseID, phaseName := "phase-2", "verify"
	checkpointID, artifactHash := "checkpoint-1", "sha256:fixture"
	artifactSize := int64(42)
	reducer := newFactoryWorldReducer(2)

	events := []interfaces.FactoryEvent{
		canonicalWorldProjectionEvent(t, interfaces.FactoryEventTypeOrchestratorPhaseChanged, interfaces.FactoryEventContext{
			EventTime: eventTime, PhaseID: &phaseID, PhaseName: &phaseName,
		}, interfaces.OrchestratorPhaseChangedEventPayload{
			PhaseStatus:       interfaces.OrchestratorPhaseStatusActive,
			PreviousPhaseName: stringPtrForProjectionTest("build"),
		}),
		canonicalWorldProjectionEvent(t, interfaces.FactoryEventTypeOrchestratorCheckpointWritten, interfaces.FactoryEventContext{
			CheckpointID: &checkpointID, EventTime: eventTime.Add(time.Second),
		}, interfaces.OrchestratorCheckpointWrittenEventPayload{
			ArtifactRef: &interfaces.FactoryArtifactRef{
				ContentHash: &artifactHash, ID: "artifact-1", Kind: "CHECKPOINT", SizeBytes: &artifactSize, Visibility: "INTERNAL_CHECKPOINT",
			},
			Label:              "after verify",
			ResumabilityStatus: interfaces.CheckpointResumabilityStatusResumable,
			Timestamp:          timePtrForProjectionTest(eventTime.Add(time.Second)),
			Warnings:           []interfaces.FactoryDispatchWarning{{Code: "CHECKPOINT_WARNING", Message: "fixture warning"}},
		}),
	}
	for _, event := range events {
		handled, err := reducer.applyOrchestratorProgressEvent(event)
		if err != nil || !handled {
			t.Fatalf("applyOrchestratorProgressEvent(%s) = handled %t, err %v", event.Type, handled, err)
		}
	}

	assertCanonicalOrchestratorProgress(t, reducer.stateValue.JavaScriptRuntime, phaseName, checkpointID, artifactHash, artifactSize)
}

func TestCanonicalDispatchLifecycleEventsReconstructQueueInterruptAndReplay(t *testing.T) {
	t.Parallel()
	eventTime := time.Date(2026, time.July, 16, 2, 0, 0, 0, time.UTC)
	dispatchID, phaseName := "dispatch-1", "execute"
	reducer := newFactoryWorldReducer(3)
	artifactIDs := []string{"artifact-1"}
	events := []interfaces.FactoryEvent{
		canonicalWorldProjectionEvent(t, interfaces.FactoryEventTypeDispatchQueued, interfaces.FactoryEventContext{
			DispatchID: &dispatchID, EventTime: eventTime, PhaseName: &phaseName,
		}, interfaces.DispatchQueuedEventPayload{
			DispatchKind: interfaces.FactoryDispatchKindJavaScriptAgent,
			InputWorkIDs: &[]string{"work-1"}, PromptDigest: stringPtrForProjectionTest("sha256:prompt"),
		}),
		canonicalWorldProjectionEvent(t, interfaces.FactoryEventTypeDispatchInterrupted, interfaces.FactoryEventContext{
			DispatchID: &dispatchID, EventTime: eventTime.Add(time.Second),
		}, interfaces.DispatchInterruptedEventPayload{
			InterruptedAt: eventTime.Add(time.Second), ObservedStatus: interfaces.FactoryDispatchStatusFailed, Reason: "provider disconnected", RetryPlanned: true,
		}),
		canonicalWorldProjectionEvent(t, interfaces.FactoryEventTypeDispatchReconciled, interfaces.FactoryEventContext{
			DispatchID: &dispatchID, EventTime: eventTime.Add(2 * time.Second),
		}, interfaces.DispatchReconciledEventPayload{
			ArtifactIDs: &artifactIDs, ReconciledStatus: interfaces.FactoryDispatchStatusCompleted,
			ReconciliationSource: interfaces.DispatchReconciliationSourceStreamReplay, Replayed: true,
		}),
	}
	for _, event := range events {
		handled, err := reducer.applyDispatchLifecycleEvent(event)
		if err != nil || !handled {
			t.Fatalf("applyDispatchLifecycleEvent(%s) = handled %t, err %v", event.Type, handled, err)
		}
	}
	dispatch := reducer.stateValue.JavaScriptRuntime.Dispatches[0]
	if dispatch.ID != dispatchID || dispatch.Status != string(interfaces.FactoryDispatchStatusCompleted) || dispatch.Phase != phaseName {
		t.Fatalf("dispatch = %#v, want completed %q dispatch in %q", dispatch, dispatchID, phaseName)
	}
	if dispatch.JavaScript == nil || dispatch.JavaScript.TaskKind != "AGENT" || dispatch.PromptDigest != "sha256:prompt" {
		t.Fatalf("dispatch metadata = %#v, want domain-owned JavaScript queue facts", dispatch)
	}
	if len(dispatch.RelatedWorkIDs) != 1 || dispatch.RelatedWorkIDs[0] != "work-1" || len(dispatch.ArtifactIDs) != 1 || dispatch.ArtifactIDs[0] != "artifact-1" {
		t.Fatalf("dispatch lineage/artifacts = %#v / %#v", dispatch.RelatedWorkIDs, dispatch.ArtifactIDs)
	}
}

func assertCanonicalOrchestratorProgress(
	t *testing.T,
	runtime *interfaces.FactorySessionJavaScriptRuntimeState,
	phaseName string,
	checkpointID string,
	artifactHash string,
	artifactSize int64,
) {
	t.Helper()
	if runtime == nil || runtime.Phase != phaseName || runtime.ScriptStatus != "RUNNING" {
		t.Fatalf("javascript runtime = %#v", runtime)
	}
	if len(runtime.Phases) != 2 || runtime.Phases[0] != "build" || runtime.Phases[1] != phaseName {
		t.Fatalf("phase history = %#v", runtime.Phases)
	}
	checkpoint := runtime.Checkpoints[0]
	if checkpoint.ID != checkpointID || checkpoint.ArtifactRef == nil || checkpoint.ArtifactRef.ContentHash != artifactHash {
		t.Fatalf("checkpoint = %#v", checkpoint)
	}
	if checkpoint.ArtifactRef.SizeBytes != artifactSize || len(checkpoint.Warnings) != 1 || checkpoint.Warnings[0].Code != "CHECKPOINT_WARNING" {
		t.Fatalf("checkpoint metadata = %#v", checkpoint)
	}
}

func canonicalWorldProjectionEvent(
	t *testing.T,
	eventType interfaces.FactoryEventType,
	context interfaces.FactoryEventContext,
	payload any,
) interfaces.FactoryEvent {
	t.Helper()
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal %s payload: %v", eventType, err)
	}
	return interfaces.FactoryEvent{
		Context: context, Id: "test/" + string(eventType), Payload: encoded,
		SchemaVersion: interfaces.FactoryEventSchemaVersionV1, Type: eventType,
	}
}

func timePtrForProjectionTest(value time.Time) *time.Time { return &value }
