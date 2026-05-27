package apicontract_test

import (
	"encoding/json"
	"reflect"
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

var generatedFactoryEventPayloadDecoders = map[factoryapi.FactoryEventType]func(factoryapi.FactoryEvent_Payload) error{
	factoryapi.FactoryEventTypeRunRequest: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsRunRequestEventPayload()
		return err
	},
	factoryapi.FactoryEventTypeInitialStructureRequest: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsInitialStructureRequestEventPayload()
		return err
	},
	factoryapi.FactoryEventTypeFactoryChange: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsFactoryChangeEventPayload()
		return err
	},
	factoryapi.FactoryEventTypeWorkRequest: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsWorkRequestEventPayload()
		return err
	},
	factoryapi.FactoryEventTypeRelationshipChangeRequest: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsRelationshipChangeRequestEventPayload()
		return err
	},
	factoryapi.FactoryEventTypeDispatchRequest: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsDispatchRequestEventPayload()
		return err
	},
	factoryapi.FactoryEventTypeModelRequest: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsModelRequestEventPayload()
		return err
	},
	factoryapi.FactoryEventTypeModelResponse: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsModelResponseEventPayload()
		return err
	},
	factoryapi.FactoryEventTypeInferenceRequest: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsInferenceRequestEventPayload()
		return err
	},
	factoryapi.FactoryEventTypeInferenceResponse: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsInferenceResponseEventPayload()
		return err
	},
	factoryapi.FactoryEventTypeScriptRequest: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsScriptRequestEventPayload()
		return err
	},
	factoryapi.FactoryEventTypeScriptResponse: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsScriptResponseEventPayload()
		return err
	},
	factoryapi.FactoryEventTypeDispatchResponse: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsDispatchResponseEventPayload()
		return err
	},
	factoryapi.FactoryEventTypeFactoryStateResponse: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsFactoryStateResponseEventPayload()
		return err
	},
	factoryapi.FactoryEventTypeRunResponse: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsRunResponseEventPayload()
		return err
	},
}

var generatedFactoryEventPayloadEncoders = map[reflect.Type]func(*factoryapi.FactoryEvent_Payload, any) error{
	reflect.TypeOf(factoryapi.RunRequestEventPayload{}): func(payload *factoryapi.FactoryEvent_Payload, value any) error {
		return payload.FromRunRequestEventPayload(value.(factoryapi.RunRequestEventPayload))
	},
	reflect.TypeOf(factoryapi.InitialStructureRequestEventPayload{}): func(payload *factoryapi.FactoryEvent_Payload, value any) error {
		return payload.FromInitialStructureRequestEventPayload(value.(factoryapi.InitialStructureRequestEventPayload))
	},
	reflect.TypeOf(factoryapi.FactoryChangeEventPayload{}): func(payload *factoryapi.FactoryEvent_Payload, value any) error {
		return payload.FromFactoryChangeEventPayload(value.(factoryapi.FactoryChangeEventPayload))
	},
	reflect.TypeOf(factoryapi.WorkRequestEventPayload{}): func(payload *factoryapi.FactoryEvent_Payload, value any) error {
		return payload.FromWorkRequestEventPayload(value.(factoryapi.WorkRequestEventPayload))
	},
	reflect.TypeOf(factoryapi.RelationshipChangeRequestEventPayload{}): func(payload *factoryapi.FactoryEvent_Payload, value any) error {
		return payload.FromRelationshipChangeRequestEventPayload(value.(factoryapi.RelationshipChangeRequestEventPayload))
	},
	reflect.TypeOf(factoryapi.DispatchRequestEventPayload{}): func(payload *factoryapi.FactoryEvent_Payload, value any) error {
		return payload.FromDispatchRequestEventPayload(value.(factoryapi.DispatchRequestEventPayload))
	},
	reflect.TypeOf(factoryapi.ModelRequestEventPayload{}): func(payload *factoryapi.FactoryEvent_Payload, value any) error {
		return payload.FromModelRequestEventPayload(value.(factoryapi.ModelRequestEventPayload))
	},
	reflect.TypeOf(factoryapi.ModelResponseEventPayload{}): func(payload *factoryapi.FactoryEvent_Payload, value any) error {
		return payload.FromModelResponseEventPayload(value.(factoryapi.ModelResponseEventPayload))
	},
	reflect.TypeOf(factoryapi.InferenceRequestEventPayload{}): func(payload *factoryapi.FactoryEvent_Payload, value any) error {
		return payload.FromInferenceRequestEventPayload(value.(factoryapi.InferenceRequestEventPayload))
	},
	reflect.TypeOf(factoryapi.InferenceResponseEventPayload{}): func(payload *factoryapi.FactoryEvent_Payload, value any) error {
		return payload.FromInferenceResponseEventPayload(value.(factoryapi.InferenceResponseEventPayload))
	},
	reflect.TypeOf(factoryapi.ScriptRequestEventPayload{}): func(payload *factoryapi.FactoryEvent_Payload, value any) error {
		return payload.FromScriptRequestEventPayload(value.(factoryapi.ScriptRequestEventPayload))
	},
	reflect.TypeOf(factoryapi.ScriptResponseEventPayload{}): func(payload *factoryapi.FactoryEvent_Payload, value any) error {
		return payload.FromScriptResponseEventPayload(value.(factoryapi.ScriptResponseEventPayload))
	},
	reflect.TypeOf(factoryapi.DispatchResponseEventPayload{}): func(payload *factoryapi.FactoryEvent_Payload, value any) error {
		return payload.FromDispatchResponseEventPayload(value.(factoryapi.DispatchResponseEventPayload))
	},
	reflect.TypeOf(factoryapi.FactoryStateResponseEventPayload{}): func(payload *factoryapi.FactoryEvent_Payload, value any) error {
		return payload.FromFactoryStateResponseEventPayload(value.(factoryapi.FactoryStateResponseEventPayload))
	},
	reflect.TypeOf(factoryapi.RunResponseEventPayload{}): func(payload *factoryapi.FactoryEvent_Payload, value any) error {
		return payload.FromRunResponseEventPayload(value.(factoryapi.RunResponseEventPayload))
	},
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
			Outputs: &[]factoryapi.WorkstationIO{{
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

	decode, ok := generatedFactoryEventPayloadDecoders[event.Type]
	if !ok {
		t.Fatalf("unexpected canonical event type %q", event.Type)
	}
	if err := decode(event.Payload); err != nil {
		t.Fatalf("decode %s %s payload: %v", event.Id, strings.ToLower(strings.ReplaceAll(string(event.Type), "_", "-")), err)
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
	encode, ok := generatedFactoryEventPayloadEncoders[reflect.TypeOf(payload)]
	if !ok {
		t.Fatalf("unsupported event payload type %T", payload)
	}
	if err := encode(&eventPayload, payload); err != nil {
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
