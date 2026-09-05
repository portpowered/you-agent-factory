package omni_artifact_test

import (
	"testing"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func inspectJourney(t *testing.T, events []factoryapi.FactoryEvent) successfulJourney {
	t.Helper()
	if len(events) < 7 {
		t.Fatalf("canonical event count = %d, want at least seven events", len(events))
	}
	journey := successfulJourney{
		workRequest:      requiredFactoryEvent(t, events, factoryapi.FactoryEventTypeWorkRequest),
		dispatchRequest:  requiredFactoryEvent(t, events, factoryapi.FactoryEventTypeDispatchRequest),
		modelRequest:     requiredFactoryEvent(t, events, factoryapi.FactoryEventTypeModelRequest),
		modelResponse:    requiredFactoryEvent(t, events, factoryapi.FactoryEventTypeModelResponse),
		dispatchResponse: requiredFactoryEvent(t, events, factoryapi.FactoryEventTypeDispatchResponse),
	}
	if sessionCompleted := optionalFactoryEvent(events, factoryapi.FactoryEventTypeSessionCompleted); sessionCompleted != nil {
		journey.sessionCompleted = *sessionCompleted
	}
	assertJourneyOrder(t, events, journey.sessionCompleted)
	workPayload, err := journey.workRequest.Payload.AsWorkRequestEventPayload()
	if err != nil {
		t.Fatalf("decode WORK_REQUEST: %v", err)
	}
	if workPayload.Works == nil || len(*workPayload.Works) != 1 {
		t.Fatalf("WORK_REQUEST works = %#v, want one Work", workPayload.Works)
	}
	journey.inputWork = (*workPayload.Works)[0]
	dispatchPayload, err := journey.dispatchRequest.Payload.AsDispatchRequestEventPayload()
	if err != nil {
		t.Fatalf("decode DISPATCH_REQUEST: %v", err)
	}
	if len(dispatchPayload.Inputs) != 1 {
		t.Fatalf("DISPATCH_REQUEST inputs = %#v, want one Work reference", dispatchPayload.Inputs)
	}
	if journey.inputWork.WorkId == nil || dispatchPayload.Inputs[0].WorkId != *journey.inputWork.WorkId {
		t.Fatalf("dispatch input = %#v, want Work ID %q", dispatchPayload.Inputs[0], requiredString(journey.inputWork.WorkId))
	}
	if association := optionalFactoryEvent(events, factoryapi.FactoryEventTypeDispatchWorkerSessionAssociation); association != nil {
		payload, decodeErr := association.Payload.AsDispatchWorkerSessionAssociationEventPayload()
		if decodeErr != nil || payload.WorkerSessionId == "" {
			t.Fatalf("worker-session association = %#v, decode error=%v", association, decodeErr)
		}
	}
	modelRequest, err := journey.modelRequest.Payload.AsModelRequestEventPayload()
	if err != nil {
		t.Fatalf("decode MODEL_REQUEST: %v", err)
	}
	if modelRequest.Operation != models.OperationOMNI || modelRequest.ModelRequestId == "" || modelRequest.Worker == "" {
		t.Fatalf("MODEL_REQUEST = %#v, want OMNI request identity", modelRequest)
	}
	modelResponse, err := journey.modelResponse.Payload.AsModelResponseEventPayload()
	if err != nil {
		t.Fatalf("decode MODEL_RESPONSE: %v", err)
	}
	if modelResponse.ModelRequestId != modelRequest.ModelRequestId {
		t.Fatalf("MODEL_RESPONSE model request identity = %q, want %q", modelResponse.ModelRequestId, modelRequest.ModelRequestId)
	}
	dispatchResponse, err := journey.dispatchResponse.Payload.AsDispatchResponseEventPayload()
	if err != nil {
		t.Fatalf("decode DISPATCH_RESPONSE: %v", err)
	}
	if dispatchResponse.Outcome != factoryapi.WorkOutcomeAccepted || dispatchResponse.OutputWork == nil || len(*dispatchResponse.OutputWork) != 1 {
		t.Fatalf("DISPATCH_RESPONSE = %#v, want accepted one-work output", dispatchResponse)
	}
	journey.outputWork = (*dispatchResponse.OutputWork)[0]
	if journey.outputWork.State == nil || journey.outputWork.State.Type != factoryapi.WorkStateTypeTERMINAL {
		t.Fatalf("output Work state = %#v, want terminal", journey.outputWork.State)
	}
	if journey.outputWork.Content == nil || len(*journey.outputWork.Content) != 1 {
		t.Fatalf("output Work content = %#v, want one materialized part", journey.outputWork.Content)
	}
	journey.outputPart = workTextPart(t, journey.outputWork.Content, "output Work")
	if journey.outputWork.WorkId == nil || requiredString(journey.outputWork.WorkId) != requiredString(journey.inputWork.WorkId) {
		t.Fatalf("output Work ID = %q, want input %q", requiredString(journey.outputWork.WorkId), requiredString(journey.inputWork.WorkId))
	}
	if journey.outputWork.CurrentChainingTraceId == nil || requiredString(journey.outputWork.CurrentChainingTraceId) != firstTraceID(t, journey.workRequest) {
		t.Fatalf("output Work current trace = %q, want %q", requiredString(journey.outputWork.CurrentChainingTraceId), firstTraceID(t, journey.workRequest))
	}
	if journey.outputWork.TraceId == nil || requiredString(journey.outputWork.TraceId) != firstTraceID(t, journey.workRequest) {
		t.Fatalf("output Work trace = %q, want %q", requiredString(journey.outputWork.TraceId), firstTraceID(t, journey.workRequest))
	}
	if !jsonEqual(t, journey.outputWork.PreviousChainingTraceIds, journey.dispatchResponse.Context.PreviousChainingTraceIds) {
		t.Fatalf("output Work prior chaining trace IDs = %#v, want dispatch context %#v", journey.outputWork.PreviousChainingTraceIds, journey.dispatchResponse.Context.PreviousChainingTraceIds)
	}
	if !jsonEqual(t, journey.outputWork.CurrentChainingTraceId, journey.dispatchResponse.Context.CurrentChainingTraceId) {
		t.Fatalf("output Work current chaining trace = %#v, want dispatch context %#v", journey.outputWork.CurrentChainingTraceId, journey.dispatchResponse.Context.CurrentChainingTraceId)
	}
	assertEventLineage(t, events, "canonical journey")
	return journey
}

func assertJourneyOrder(t *testing.T, events []factoryapi.FactoryEvent, sessionCompleted factoryapi.FactoryEvent) {
	t.Helper()
	order := []factoryapi.FactoryEventType{
		factoryapi.FactoryEventTypeWorkRequest,
		factoryapi.FactoryEventTypeDispatchRequest,
		factoryapi.FactoryEventTypeModelRequest,
		factoryapi.FactoryEventTypeModelResponse,
		factoryapi.FactoryEventTypeDispatchResponse,
	}
	if sessionCompleted.Type == factoryapi.FactoryEventTypeSessionCompleted {
		order = append(order, factoryapi.FactoryEventTypeSessionCompleted)
	}
	previous := -1
	for _, eventType := range order {
		current := eventIndex(events, eventType)
		if current <= previous {
			t.Fatalf("canonical event order has %s at index %d after index %d", eventType, current, previous)
		}
		previous = current
	}
}
