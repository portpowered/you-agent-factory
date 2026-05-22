package api_test

import (
	"encoding/json"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
)

var canonicalFactoryEventTypes = []factoryapi.FactoryEventType{
	factoryapi.FactoryEventTypeRunRequest,
	factoryapi.FactoryEventTypeInitialStructureRequest,
	factoryapi.FactoryEventTypeFactoryChange,
	factoryapi.FactoryEventTypeWorkRequest,
	factoryapi.FactoryEventTypeRelationshipChangeRequest,
	factoryapi.FactoryEventTypeDispatchRequest,
	factoryapi.FactoryEventTypeModelRequest,
	factoryapi.FactoryEventTypeModelResponse,
	factoryapi.FactoryEventTypeInferenceRequest,
	factoryapi.FactoryEventTypeInferenceResponse,
	factoryapi.FactoryEventTypeScriptRequest,
	factoryapi.FactoryEventTypeScriptResponse,
	factoryapi.FactoryEventTypeDispatchResponse,
	factoryapi.FactoryEventTypeFactoryStateResponse,
	factoryapi.FactoryEventTypeRunResponse,
}

var retiredFactoryEventTypeStrings = []string{
	"RUN_STARTED",
	"INITIAL_STRUCTURE",
	"RELATIONSHIP_CHANGE",
	"DISPATCH_CREATED",
	"DISPATCH_COMPLETED",
	"FACTORY_STATE_CHANGE",
	"RUN_FINISHED",
}

func generatedNamedFactoryFixture() factoryapi.Factory {
	return factoryapi.Factory{
		Name: "customer-support-triage",
		WorkTypes: &[]factoryapi.WorkType{{
			Name: "task",
			States: []factoryapi.WorkState{
				{Name: "init", Type: factoryapi.WorkStateTypeINITIAL},
				{Name: "done", Type: factoryapi.WorkStateTypeTERMINAL},
			},
		}},
		Workers: &[]factoryapi.Worker{{
			Name:             "planner",
			Type:             workerTypePtr(factoryapi.WorkerTypeModelWorker),
			ModelProvider:    workerModelProviderPtr(factoryapi.WorkerModelProviderClaude),
			ExecutorProvider: workerProviderPtr(factoryapi.WorkerProviderScriptWrap),
			Model:            stringPtr("claude-sonnet-4-20250514"),
		}},
		Workstations: &[]factoryapi.Workstation{{
			Name:   "plan-task",
			Worker: "planner",
			Inputs: []factoryapi.WorkstationIO{{WorkType: "task", State: "init"}},
			Outputs: []factoryapi.WorkstationIO{{
				WorkType: "task",
				State:    "done",
			}},
			OnContinue: &[]factoryapi.WorkstationIO{
				{WorkType: "task", State: "init"},
				{WorkType: "task", State: "queued"},
			},
			OnRejection: &[]factoryapi.WorkstationIO{
				{WorkType: "task", State: "review"},
			},
			OnFailure: &[]factoryapi.WorkstationIO{
				{WorkType: "task", State: "failed"},
			},
		}},
	}
}

func requireGeneratedFactoryEventPayloadRoundTrip(t *testing.T, event factoryapi.FactoryEvent) {
	t.Helper()

	switch event.Type {
	case factoryapi.FactoryEventTypeRunRequest:
		if _, err := event.Payload.AsRunRequestEventPayload(); err != nil {
			t.Fatalf("decode %s run-request payload: %v", event.Id, err)
		}
	case factoryapi.FactoryEventTypeInitialStructureRequest:
		if _, err := event.Payload.AsInitialStructureRequestEventPayload(); err != nil {
			t.Fatalf("decode %s initial-structure payload: %v", event.Id, err)
		}
	case factoryapi.FactoryEventTypeFactoryChange:
		if _, err := event.Payload.AsFactoryChangeEventPayload(); err != nil {
			t.Fatalf("decode %s factory-change payload: %v", event.Id, err)
		}
	case factoryapi.FactoryEventTypeWorkRequest:
		if _, err := event.Payload.AsWorkRequestEventPayload(); err != nil {
			t.Fatalf("decode %s work-request payload: %v", event.Id, err)
		}
	case factoryapi.FactoryEventTypeRelationshipChangeRequest:
		if _, err := event.Payload.AsRelationshipChangeRequestEventPayload(); err != nil {
			t.Fatalf("decode %s relationship-change payload: %v", event.Id, err)
		}
	case factoryapi.FactoryEventTypeDispatchRequest:
		if _, err := event.Payload.AsDispatchRequestEventPayload(); err != nil {
			t.Fatalf("decode %s dispatch-request payload: %v", event.Id, err)
		}
	case factoryapi.FactoryEventTypeModelRequest:
		if _, err := event.Payload.AsModelRequestEventPayload(); err != nil {
			t.Fatalf("decode %s model-request payload: %v", event.Id, err)
		}
	case factoryapi.FactoryEventTypeModelResponse:
		if _, err := event.Payload.AsModelResponseEventPayload(); err != nil {
			t.Fatalf("decode %s model-response payload: %v", event.Id, err)
		}
	case factoryapi.FactoryEventTypeInferenceRequest:
		if _, err := event.Payload.AsInferenceRequestEventPayload(); err != nil {
			t.Fatalf("decode %s inference-request payload: %v", event.Id, err)
		}
	case factoryapi.FactoryEventTypeInferenceResponse:
		if _, err := event.Payload.AsInferenceResponseEventPayload(); err != nil {
			t.Fatalf("decode %s inference-response payload: %v", event.Id, err)
		}
	case factoryapi.FactoryEventTypeScriptRequest:
		if _, err := event.Payload.AsScriptRequestEventPayload(); err != nil {
			t.Fatalf("decode %s script-request payload: %v", event.Id, err)
		}
	case factoryapi.FactoryEventTypeScriptResponse:
		if _, err := event.Payload.AsScriptResponseEventPayload(); err != nil {
			t.Fatalf("decode %s script-response payload: %v", event.Id, err)
		}
	case factoryapi.FactoryEventTypeDispatchResponse:
		if _, err := event.Payload.AsDispatchResponseEventPayload(); err != nil {
			t.Fatalf("decode %s dispatch-response payload: %v", event.Id, err)
		}
	case factoryapi.FactoryEventTypeFactoryStateResponse:
		if _, err := event.Payload.AsFactoryStateResponseEventPayload(); err != nil {
			t.Fatalf("decode %s factory-state payload: %v", event.Id, err)
		}
	case factoryapi.FactoryEventTypeRunResponse:
		if _, err := event.Payload.AsRunResponseEventPayload(); err != nil {
			t.Fatalf("decode %s run-response payload: %v", event.Id, err)
		}
	default:
		t.Fatalf("unexpected canonical event type %q", event.Type)
	}
}

func assertTextOmitsRetiredEventNames(t *testing.T, text string) {
	t.Helper()

	for _, retired := range retiredFactoryEventTypeStrings {
		if strings.Contains(text, `"`+retired+`"`) {
			t.Fatalf("unexpected retired public event name %q in artifact text", retired)
		}
	}
}

func stringPtr(value string) *string {
	return &value
}

func workerModelProviderPtr(value factoryapi.WorkerModelProvider) *factoryapi.WorkerModelProvider {
	return &value
}

func workerProviderPtr(value factoryapi.WorkerProvider) *factoryapi.WorkerProvider {
	return &value
}

func workerTypePtr(value factoryapi.WorkerType) *factoryapi.WorkerType {
	return &value
}

func intPtr(value int) *int {
	return &value
}

func boolPtr(value bool) *bool {
	return &value
}

func factoryEventPayload(t *testing.T, payload any) factoryapi.FactoryEvent_Payload {
	t.Helper()

	var eventPayload factoryapi.FactoryEvent_Payload
	var err error
	switch typed := payload.(type) {
	case factoryapi.RunRequestEventPayload:
		err = eventPayload.FromRunRequestEventPayload(typed)
	case factoryapi.InitialStructureRequestEventPayload:
		err = eventPayload.FromInitialStructureRequestEventPayload(typed)
	case factoryapi.FactoryChangeEventPayload:
		err = eventPayload.FromFactoryChangeEventPayload(typed)
	case factoryapi.WorkRequestEventPayload:
		err = eventPayload.FromWorkRequestEventPayload(typed)
	case factoryapi.RelationshipChangeRequestEventPayload:
		err = eventPayload.FromRelationshipChangeRequestEventPayload(typed)
	case factoryapi.DispatchRequestEventPayload:
		err = eventPayload.FromDispatchRequestEventPayload(typed)
	case factoryapi.ModelRequestEventPayload:
		err = eventPayload.FromModelRequestEventPayload(typed)
	case factoryapi.ModelResponseEventPayload:
		err = eventPayload.FromModelResponseEventPayload(typed)
	case factoryapi.InferenceRequestEventPayload:
		err = eventPayload.FromInferenceRequestEventPayload(typed)
	case factoryapi.InferenceResponseEventPayload:
		err = eventPayload.FromInferenceResponseEventPayload(typed)
	case factoryapi.ScriptRequestEventPayload:
		err = eventPayload.FromScriptRequestEventPayload(typed)
	case factoryapi.ScriptResponseEventPayload:
		err = eventPayload.FromScriptResponseEventPayload(typed)
	case factoryapi.DispatchResponseEventPayload:
		err = eventPayload.FromDispatchResponseEventPayload(typed)
	case factoryapi.FactoryStateResponseEventPayload:
		err = eventPayload.FromFactoryStateResponseEventPayload(typed)
	case factoryapi.RunResponseEventPayload:
		err = eventPayload.FromRunResponseEventPayload(typed)
	default:
		t.Fatalf("unsupported event payload type %T", payload)
	}
	if err != nil {
		t.Fatalf("encode generated FactoryEvent payload: %v", err)
	}
	return eventPayload
}

func decodeRoundTripJSON[T any](t *testing.T, encoded []byte, target *T, label string) {
	t.Helper()

	if err := json.Unmarshal(encoded, target); err != nil {
		t.Fatalf("unmarshal %s: %v", label, err)
	}
}
