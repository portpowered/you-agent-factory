package apicontract_test

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	generatedclient "github.com/portpowered/infinite-you/pkg/transports/http/client"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestOpenAPIAuthoring_ResponseEventStreamDeclaresEphemeralSSEContract(t *testing.T) {
	doc := loadAuthoredOpenAPIDoc(t)
	paths, ok := doc["paths"].(map[string]any)
	if !ok {
		t.Fatal("paths object is missing")
	}
	operation := pathOperation(t, paths, responseEventStreamPath, "get")

	if got := operation["operationId"]; got != "getFactoryResponseEventsBySessionId" {
		t.Fatalf("response-event operationId = %v", got)
	}
	description, _ := operation["description"].(string)
	for _, phrase := range []string{
		"ephemeral FactoryResponseEvent observation records",
		"outside canonical FactoryEvent replay",
		"retained matching records",
		"continues with live matching records",
		"decimal FactoryResponseEvent.sequence",
		"first emitted record is STREAM_GAP",
		"never falls back to the current or default session",
	} {
		if !strings.Contains(description, phrase) {
			t.Fatalf("response-event description missing %q", phrase)
		}
	}

	parameters, ok := operation["parameters"].([]any)
	if !ok {
		t.Fatal("response-event operation parameters are missing")
	}
	for _, ref := range []string{
		"#/components/parameters/SessionID",
		"#/components/parameters/ResponseEventAfterSequence",
		"#/components/parameters/ResponseEventDispatchID",
		"#/components/parameters/ResponseEventKind",
	} {
		assertParameterRef(t, parameters, ref)
	}

	assertEventStreamSchemaRef(t, operation, "#/components/schemas/FactoryResponseEvent")
	for status, ref := range map[string]string{
		"400": "#/components/responses/ResponseEventBadRequest",
		"404": "#/components/responses/ResponseEventSessionNotFound",
		"410": "#/components/responses/ResponseEventStreamExpired",
		"500": "#/components/responses/InternalError",
	} {
		assertResponseRef(t, operation, status, ref)
	}
}

func TestOpenAPIAuthoring_ResponseEventStreamParametersAreConstrained(t *testing.T) {
	afterSequence := loadAuthoredComponentFragment(t, "../../../../api/components/parameters/ResponseEventAfterSequence.yaml")
	assertQueryParameter(t, afterSequence, "after_sequence")
	afterSequenceSchema := objectField(t, afterSequence, "schema")
	if got := afterSequenceSchema["format"]; got != "int64" {
		t.Fatalf("after_sequence format = %v, want int64", got)
	}
	if got := afterSequenceSchema["minimum"]; got != 0 {
		t.Fatalf("after_sequence minimum = %v, want 0", got)
	}
	if !strings.Contains(afterSequence["description"].(string), "Last acknowledged FactoryResponseEvent.sequence") {
		t.Fatal("after_sequence must identify the last acknowledged response sequence")
	}

	dispatchID := loadAuthoredComponentFragment(t, "../../../../api/components/parameters/ResponseEventDispatchID.yaml")
	assertQueryParameter(t, dispatchID, "dispatch_id")

	kind := loadAuthoredComponentFragment(t, "../../../../api/components/parameters/ResponseEventKind.yaml")
	assertQueryParameter(t, kind, "kind")
	if kind["style"] != "form" || kind["explode"] != true {
		t.Fatalf("kind repetition encoding = style:%v explode:%v, want form/true", kind["style"], kind["explode"])
	}
	kindItems := objectField(t, objectField(t, kind, "schema"), "items")
	if got := kindItems["$ref"]; got != "../schemas/response-events/FactoryResponseEventKind.yaml" {
		t.Fatalf("kind items ref = %v", got)
	}
}

func TestOpenAPIAuthoring_ResponseEventStreamErrorsAreTyped(t *testing.T) {
	badRequest := loadAuthoredComponentFragment(t, "../../../../api/components/responses/ResponseEventBadRequest.yaml")
	assertResponseFragmentSchema(t, badRequest, "../schemas/api/ErrorResponse.yaml")
	assertResponseFragmentExampleCodes(t, badRequest, "INVALID_RESPONSE_EVENT_CURSOR", "INVALID_RESPONSE_EVENT_FILTER")

	notFound := loadAuthoredComponentFragment(t, "../../../../api/components/responses/ResponseEventSessionNotFound.yaml")
	assertResponseFragmentSchema(t, notFound, "../schemas/api/ErrorResponse.yaml")
	assertResponseFragmentExampleCodes(t, notFound, "RESPONSE_EVENT_SESSION_NOT_FOUND")
	if !strings.Contains(notFound["description"].(string), "never falls back") {
		t.Fatal("response-event 404 must prohibit current/default-session fallback")
	}

	expired := loadAuthoredComponentFragment(t, "../../../../api/components/responses/ResponseEventStreamExpired.yaml")
	assertResponseFragmentSchema(t, expired, "../schemas/api/ErrorResponse.yaml")
	assertResponseFragmentExampleCodes(t, expired, "RESPONSE_EVENT_STREAM_EXPIRED")
}

func TestGeneratedGoClientBuildsFilteredResponseEventRequest(t *testing.T) {
	const reconnectCursor int64 = 4_294_967_296
	var afterSequence int64 = generatedclient.ResponseEventAfterSequence(reconnectCursor)
	dispatchID := generatedclient.ResponseEventDispatchID("dispatch/one")
	kinds := generatedclient.ResponseEventKind{
		generatedclient.FactoryResponseEventKindMessage,
		generatedclient.FactoryResponseEventKindTool,
	}
	request, err := generatedclient.NewGetFactoryResponseEventsBySessionIdRequest(
		"http://localhost:7437",
		generatedclient.SessionID("session one"),
		&generatedclient.GetFactoryResponseEventsBySessionIdParams{
			AfterSequence: &afterSequence,
			DispatchId:    &dispatchID,
			Kind:          &kinds,
		},
	)
	if err != nil {
		t.Fatalf("build generated response-event request: %v", err)
	}
	if got, want := request.URL.EscapedPath(), "/factory-sessions/session%20one/response-events"; got != want {
		t.Fatalf("generated response-event path = %q, want %q", got, want)
	}
	query := request.URL.Query()
	if query.Get("after_sequence") != "4294967296" || query.Get("dispatch_id") != "dispatch/one" {
		t.Fatalf("generated response-event query = %q", request.URL.RawQuery)
	}
	if got := query["kind"]; len(got) != 2 || got[0] != "MESSAGE" || got[1] != "TOOL" {
		t.Fatalf("generated repeated kind values = %#v, want [MESSAGE TOOL]", got)
	}
}

func TestGeneratedGoClientExposesResponseEventSuccessAndTypedErrors(t *testing.T) {
	response := generatedclient.GetFactoryResponseEventsBySessionIdClientResponse{
		Body:    []byte("data: {}\n\n"),
		JSON400: &generatedclient.ResponseEventBadRequest{},
		JSON404: &generatedclient.ResponseEventSessionNotFound{},
		JSON410: &generatedclient.ResponseEventStreamExpired{},
		JSON500: &generatedclient.InternalError{},
	}
	if len(response.Body) == 0 || response.JSON400 == nil || response.JSON404 == nil || response.JSON410 == nil || response.JSON500 == nil {
		t.Fatal("generated Go client must expose SSE success and every typed response-event error")
	}
}

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
	factoryapi.FactoryEventTypeAgentRunResponse,
	factoryapi.FactoryEventTypeDispatchResponse,
	factoryapi.FactoryEventTypeWorkStateChange,
	factoryapi.FactoryEventTypeFactoryStateResponse,
	factoryapi.FactoryEventTypeRunResponse,
	factoryapi.FactoryEventTypeSessionStarted,
	factoryapi.FactoryEventTypeSessionPaused,
	factoryapi.FactoryEventTypeSessionResumed,
	factoryapi.FactoryEventTypeSessionResultUpdated,
	factoryapi.FactoryEventTypeSessionCompleted,
	factoryapi.FactoryEventTypeSessionLifecycleControl,
	factoryapi.FactoryEventTypeOrchestratorPhaseChanged,
	factoryapi.FactoryEventTypeOrchestratorCheckpointWritten,
	factoryapi.FactoryEventTypeDispatchQueued,
	factoryapi.FactoryEventTypeDispatchInterrupted,
	factoryapi.FactoryEventTypeDispatchReconciled,
	factoryapi.FactoryEventTypeJavaScriptCheckpointRef,
	factoryapi.FactoryEventTypeJavaScriptPhaseChange,
	factoryapi.FactoryEventTypeArtifactCreated,
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
	factoryapi.FactoryEventTypeAgentRunResponse: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsAgentRunResponseEventPayload()
		return err
	},
	factoryapi.FactoryEventTypeDispatchResponse: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsDispatchResponseEventPayload()
		return err
	},
	factoryapi.FactoryEventTypeWorkStateChange: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsWorkStateChangeEventPayload()
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
	factoryapi.FactoryEventTypeSessionStarted: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsSessionStartedEventPayload()
		return err
	},
	factoryapi.FactoryEventTypeSessionPaused: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsSessionPausedEventPayload()
		return err
	},
	factoryapi.FactoryEventTypeSessionResumed: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsSessionResumedEventPayload()
		return err
	},
	factoryapi.FactoryEventTypeSessionResultUpdated: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsSessionResultUpdatedEventPayload()
		return err
	},
	factoryapi.FactoryEventTypeSessionCompleted: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsSessionCompletedEventPayload()
		return err
	},
	factoryapi.FactoryEventTypeSessionLifecycleControl: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsSessionLifecycleControlEventPayload()
		return err
	},
	factoryapi.FactoryEventTypeOrchestratorPhaseChanged: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsOrchestratorPhaseChangedEventPayload()
		return err
	},
	factoryapi.FactoryEventTypeOrchestratorCheckpointWritten: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsOrchestratorCheckpointWrittenEventPayload()
		return err
	},
	factoryapi.FactoryEventTypeDispatchQueued: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsDispatchQueuedEventPayload()
		return err
	},
	factoryapi.FactoryEventTypeDispatchInterrupted: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsDispatchInterruptedEventPayload()
		return err
	},
	factoryapi.FactoryEventTypeDispatchReconciled: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsDispatchReconciledEventPayload()
		return err
	},
	factoryapi.FactoryEventTypeJavaScriptCheckpointRef: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsJavaScriptCheckpointRefEventPayload()
		return err
	},
	factoryapi.FactoryEventTypeJavaScriptPhaseChange: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsJavaScriptPhaseChangeEventPayload()
		return err
	},
	factoryapi.FactoryEventTypeArtifactCreated: func(payload factoryapi.FactoryEvent_Payload) error {
		_, err := payload.AsArtifactCreatedEventPayload()
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
	reflect.TypeOf(factoryapi.AgentRunResponseEventPayload{}): func(payload *factoryapi.FactoryEvent_Payload, value any) error {
		return payload.FromAgentRunResponseEventPayload(value.(factoryapi.AgentRunResponseEventPayload))
	},
	reflect.TypeOf(factoryapi.DispatchResponseEventPayload{}): func(payload *factoryapi.FactoryEvent_Payload, value any) error {
		return payload.FromDispatchResponseEventPayload(value.(factoryapi.DispatchResponseEventPayload))
	},
	reflect.TypeOf(factoryapi.WorkStateChangeEventPayload{}): func(payload *factoryapi.FactoryEvent_Payload, value any) error {
		return payload.FromWorkStateChangeEventPayload(value.(factoryapi.WorkStateChangeEventPayload))
	},
	reflect.TypeOf(factoryapi.FactoryStateResponseEventPayload{}): func(payload *factoryapi.FactoryEvent_Payload, value any) error {
		return payload.FromFactoryStateResponseEventPayload(value.(factoryapi.FactoryStateResponseEventPayload))
	},
	reflect.TypeOf(factoryapi.RunResponseEventPayload{}): func(payload *factoryapi.FactoryEvent_Payload, value any) error {
		return payload.FromRunResponseEventPayload(value.(factoryapi.RunResponseEventPayload))
	},
	reflect.TypeOf(factoryapi.SessionStartedEventPayload{}): func(payload *factoryapi.FactoryEvent_Payload, value any) error {
		return payload.FromSessionStartedEventPayload(value.(factoryapi.SessionStartedEventPayload))
	},
	reflect.TypeOf(factoryapi.SessionPausedEventPayload{}): func(payload *factoryapi.FactoryEvent_Payload, value any) error {
		return payload.FromSessionPausedEventPayload(value.(factoryapi.SessionPausedEventPayload))
	},
	reflect.TypeOf(factoryapi.SessionResumedEventPayload{}): func(payload *factoryapi.FactoryEvent_Payload, value any) error {
		return payload.FromSessionResumedEventPayload(value.(factoryapi.SessionResumedEventPayload))
	},
	reflect.TypeOf(factoryapi.SessionResultUpdatedEventPayload{}): func(payload *factoryapi.FactoryEvent_Payload, value any) error {
		return payload.FromSessionResultUpdatedEventPayload(value.(factoryapi.SessionResultUpdatedEventPayload))
	},
	reflect.TypeOf(factoryapi.SessionCompletedEventPayload{}): func(payload *factoryapi.FactoryEvent_Payload, value any) error {
		return payload.FromSessionCompletedEventPayload(value.(factoryapi.SessionCompletedEventPayload))
	},
	reflect.TypeOf(factoryapi.SessionLifecycleControlEventPayload{}): func(payload *factoryapi.FactoryEvent_Payload, value any) error {
		return payload.FromSessionLifecycleControlEventPayload(value.(factoryapi.SessionLifecycleControlEventPayload))
	},
	reflect.TypeOf(factoryapi.OrchestratorPhaseChangedEventPayload{}): func(payload *factoryapi.FactoryEvent_Payload, value any) error {
		return payload.FromOrchestratorPhaseChangedEventPayload(value.(factoryapi.OrchestratorPhaseChangedEventPayload))
	},
	reflect.TypeOf(factoryapi.OrchestratorCheckpointWrittenEventPayload{}): func(payload *factoryapi.FactoryEvent_Payload, value any) error {
		return payload.FromOrchestratorCheckpointWrittenEventPayload(value.(factoryapi.OrchestratorCheckpointWrittenEventPayload))
	},
	reflect.TypeOf(factoryapi.DispatchQueuedEventPayload{}): func(payload *factoryapi.FactoryEvent_Payload, value any) error {
		return payload.FromDispatchQueuedEventPayload(value.(factoryapi.DispatchQueuedEventPayload))
	},
	reflect.TypeOf(factoryapi.DispatchInterruptedEventPayload{}): func(payload *factoryapi.FactoryEvent_Payload, value any) error {
		return payload.FromDispatchInterruptedEventPayload(value.(factoryapi.DispatchInterruptedEventPayload))
	},
	reflect.TypeOf(factoryapi.DispatchReconciledEventPayload{}): func(payload *factoryapi.FactoryEvent_Payload, value any) error {
		return payload.FromDispatchReconciledEventPayload(value.(factoryapi.DispatchReconciledEventPayload))
	},
	reflect.TypeOf(factoryapi.JavaScriptCheckpointRefEventPayload{}): func(payload *factoryapi.FactoryEvent_Payload, value any) error {
		return payload.FromJavaScriptCheckpointRefEventPayload(value.(factoryapi.JavaScriptCheckpointRefEventPayload))
	},
	reflect.TypeOf(factoryapi.JavaScriptPhaseChangeEventPayload{}): func(payload *factoryapi.FactoryEvent_Payload, value any) error {
		return payload.FromJavaScriptPhaseChangeEventPayload(value.(factoryapi.JavaScriptPhaseChangeEventPayload))
	},
	reflect.TypeOf(factoryapi.ArtifactCreatedEventPayload{}): func(payload *factoryapi.FactoryEvent_Payload, value any) error {
		return payload.FromArtifactCreatedEventPayload(value.(factoryapi.ArtifactCreatedEventPayload))
	},
}

func generatedNamedFactoryFixture() factoryapi.Factory {
	unknownNamedPolicy := factoryapi.FactoryInvocationUnknownNamedArgumentPolicyReject
	parameterTypeHint := factoryapi.FactoryInvocationParameterTypeHintString
	parameterValueMode := factoryapi.FactoryInvocationParameterValueModeExact
	bindingKind := factoryapi.FactoryInvocationParameterBindingKindPositional
	outputContractMode := factoryapi.FactoryInvocationOutputContractModeInline
	return factoryapi.Factory{
		Name: "customer-support-triage",
		Examples: &[]factoryapi.FactoryInvocationExample{
			generatedFactoryInvocationExample("basic", map[string]interface{}{"input": "brief.md"}),
		},
		InvocationSignature: &factoryapi.FactoryInvocationSignature{
			UnknownNamedArgumentPolicy: &unknownNamedPolicy,
			Parameters: &[]factoryapi.FactoryInvocationParameter{{
				Name:         "input",
				ExternalName: stringPtr("input"),
				TypeHint:     &parameterTypeHint,
				ValueMode:    &parameterValueMode,
				Bindings: &[]factoryapi.FactoryInvocationParameterBinding{{
					Kind:     bindingKind,
					Position: intPtr(1),
				}},
			}},
			OutputContract: &factoryapi.FactoryInvocationOutputContract{
				Mode: &outputContractMode,
			},
		},
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
			ExecutorProvider: workerProviderPtr(factoryapi.WorkerProvider("SCRIPT_WRAP")),
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

func generatedFactoryInvocationExample(name string, args map[string]interface{}) factoryapi.FactoryInvocationExample {
	generatedArgs := make(factoryapi.FactoryInvocationArguments, len(args))
	for key, value := range args {
		var union factoryapi.FactoryInvocationArguments_AdditionalProperties
		switch typed := value.(type) {
		case string:
			_ = union.FromFactoryInvocationArguments0(typed)
		case []string:
			_ = union.FromFactoryInvocationArguments1(typed)
		}
		generatedArgs[key] = union
	}
	return factoryapi.FactoryInvocationExample{
		Name: name,
		Description: factoryapi.NameValue{
			Type:  factoryapi.LOCALIZABLEASSET,
			Value: name,
		},
		Args: generatedArgs,
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

func assertTextOmitsInternalResponseStreamTerms(t *testing.T, text string) {
	t.Helper()

	forbiddenTerms := []string{
		"SessionResponseStream",
		"SessionResponseStreamEvent",
		"SessionResponseStreamEventKind",
		"ExternalEventType",
		"CompactionSummary",
		"CompactionReason",
		"STREAM_COMPACTION_SIGNAL",
		"PROGRESS_FRAGMENT",
		"RESPONSE_FRAGMENT",
		"response.completed",
		"response.output_text.delta",
		"response.failed",
		"session.created",
	}
	for _, forbidden := range forbiddenTerms {
		if strings.Contains(text, forbidden) {
			t.Fatalf("unexpected internal response-stream term %q in public artifact text", forbidden)
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

func durationMillisPtr(value int64) *int64 {
	return &value
}

func factoryEventSessionResultStatusPtr(value factoryapi.FactoryEventSessionResultStatus) *factoryapi.FactoryEventSessionResultStatus {
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

type generatedSessionLifecycleFixture struct {
	sessionID           string
	orchestratorKind    factoryapi.FactoryOrchestratorKind
	orchestratorDialect string
	phaseID             string
	phaseName           string
	dispatchID          string
	source              string
	sessionSequence     int
}

func (f *generatedSessionLifecycleFixture) nextSessionSequence() int {
	current := f.sessionSequence
	f.sessionSequence++
	return current
}

func generatedFactorySessionLifecycleEvents(t *testing.T, eventTime time.Time) []factoryapi.FactoryEvent {
	t.Helper()
	fixture := &generatedSessionLifecycleFixture{
		sessionID:           "session-alpha",
		orchestratorKind:    factoryapi.JAVASCRIPT,
		orchestratorDialect: "workflow-v1",
		phaseID:             "phase-plan",
		phaseName:           "plan",
		dispatchID:          "dispatch-child-1",
		source:              "api",
	}
	events := make([]factoryapi.FactoryEvent, 0, 8)
	events = append(events, generatedSessionLifecycleBracketEvents(t, fixture, eventTime)...)
	events = append(events, generatedSessionLifecyclePauseResumeEvents(t, fixture, eventTime)...)
	events = append(events, generatedSessionLifecycleOrchestratorEvents(t, fixture, eventTime)...)
	events = append(events, generatedSessionLifecycleDispatchEvents(t, fixture, eventTime)...)
	return events
}

func generatedSessionLifecycleBracketEvents(
	t *testing.T,
	fixture *generatedSessionLifecycleFixture,
	eventTime time.Time,
) []factoryapi.FactoryEvent {
	t.Helper()
	return []factoryapi.FactoryEvent{
		{
			SchemaVersion: factoryapi.AgentFactoryEventV1,
			Id:            "event-session-started",
			Type:          factoryapi.FactoryEventTypeSessionStarted,
			Context: factoryapi.FactoryEventContext{
				Sequence:            10,
				Tick:                5,
				EventTime:           eventTime,
				SessionId:           &fixture.sessionID,
				SessionSequence:     intPtr(fixture.nextSessionSequence()),
				OrchestratorKind:    &fixture.orchestratorKind,
				OrchestratorDialect: &fixture.orchestratorDialect,
				Source:              &fixture.source,
			},
			Payload: factoryEventPayload(t, factoryapi.SessionStartedEventPayload{
				FactoryId:  stringPtr("factory-alpha"),
				SourceRef:  stringPtr("workflow/main.js"),
				SourceHash: stringPtr("sha256:source"),
				PolicyHash: stringPtr("sha256:policy"),
				ArgsDigest: stringPtr("sha256:args"),
				StartedAt:  eventTime,
			}),
		},
		{
			SchemaVersion: factoryapi.AgentFactoryEventV1,
			Id:            "event-session-result-updated",
			Type:          factoryapi.FactoryEventTypeSessionResultUpdated,
			Context: factoryapi.FactoryEventContext{
				Sequence:         13,
				Tick:             6,
				EventTime:        eventTime,
				SessionId:        &fixture.sessionID,
				SessionSequence:  intPtr(fixture.nextSessionSequence()),
				OrchestratorKind: &fixture.orchestratorKind,
				PhaseId:          &fixture.phaseID,
				PhaseName:        &fixture.phaseName,
				Source:           &fixture.source,
			},
			Payload: factoryEventPayload(t, factoryapi.SessionResultUpdatedEventPayload{
				ResultStatus: factoryapi.FactoryEventSessionResultStatusPartial,
				ArtifactIds:  &[]string{"artifact-partial-1"},
			}),
		},
		{
			SchemaVersion: factoryapi.AgentFactoryEventV1,
			Id:            "event-session-lifecycle-control",
			Type:          factoryapi.FactoryEventTypeSessionLifecycleControl,
			Context: factoryapi.FactoryEventContext{
				Sequence:         14,
				Tick:             7,
				EventTime:        eventTime,
				SessionId:        &fixture.sessionID,
				SessionSequence:  intPtr(fixture.nextSessionSequence()),
				OrchestratorKind: &fixture.orchestratorKind,
				Source:           &fixture.source,
			},
			Payload: factoryEventPayload(t, factoryapi.SessionLifecycleControlEventPayload{
				Operation:      factoryapi.FactorySessionLifecycleControlKindPause,
				Outcome:        factoryapi.FactorySessionLifecycleControlOutcomeAccepted,
				PreviousStatus: factoryapi.FactorySessionDurableLifecycleStatusRunning,
				NewStatus:      factoryapi.FactorySessionDurableLifecycleStatusPaused,
				OccurredAt:     eventTime,
				Reason:         stringPtr("operator requested pause"),
			}),
		},
		{
			SchemaVersion: factoryapi.AgentFactoryEventV1,
			Id:            "event-session-completed",
			Type:          factoryapi.FactoryEventTypeSessionCompleted,
			Context: factoryapi.FactoryEventContext{
				Sequence:         13,
				Tick:             8,
				EventTime:        eventTime,
				SessionId:        &fixture.sessionID,
				SessionSequence:  intPtr(fixture.nextSessionSequence()),
				OrchestratorKind: &fixture.orchestratorKind,
				Source:           &fixture.source,
			},
			Payload: factoryEventPayload(t, factoryapi.SessionCompletedEventPayload{
				FinalStatus:    factoryapi.FactorySessionDurableLifecycleStatusSucceeded,
				CompletedAt:    eventTime,
				DurationMillis: durationMillisPtr(2000),
				ResultStatus:   factoryEventSessionResultStatusPtr(factoryapi.FactoryEventSessionResultStatusFinal),
				ArtifactIds:    &[]string{"artifact-result-1"},
				DispatchCounts: &factoryapi.FactorySessionJavaScriptChildDispatchCounts{
					Queued:    0,
					Running:   0,
					Completed: 2,
				},
			}),
		},
	}
}

func generatedSessionLifecyclePauseResumeEvents(
	t *testing.T,
	fixture *generatedSessionLifecycleFixture,
	eventTime time.Time,
) []factoryapi.FactoryEvent {
	t.Helper()
	return []factoryapi.FactoryEvent{
		{
			SchemaVersion: factoryapi.AgentFactoryEventV1,
			Id:            "event-session-paused",
			Type:          factoryapi.FactoryEventTypeSessionPaused,
			Context: factoryapi.FactoryEventContext{
				Sequence:         11,
				Tick:             6,
				EventTime:        eventTime,
				SessionId:        &fixture.sessionID,
				SessionSequence:  intPtr(fixture.nextSessionSequence()),
				OrchestratorKind: &fixture.orchestratorKind,
				Source:           &fixture.source,
			},
			Payload: factoryEventPayload(t, factoryapi.SessionPausedEventPayload{
				Status:   factoryapi.FactorySessionDurableLifecycleStatusPaused,
				PausedAt: eventTime,
			}),
		},
		{
			SchemaVersion: factoryapi.AgentFactoryEventV1,
			Id:            "event-session-resumed",
			Type:          factoryapi.FactoryEventTypeSessionResumed,
			Context: factoryapi.FactoryEventContext{
				Sequence:         12,
				Tick:             6,
				EventTime:        eventTime,
				SessionId:        &fixture.sessionID,
				SessionSequence:  intPtr(fixture.nextSessionSequence()),
				OrchestratorKind: &fixture.orchestratorKind,
				Source:           &fixture.source,
			},
			Payload: factoryEventPayload(t, factoryapi.SessionResumedEventPayload{
				Status:    factoryapi.FactorySessionDurableLifecycleStatusRunning,
				ResumedAt: eventTime,
			}),
		},
	}
}

func generatedSessionLifecycleOrchestratorEvents(
	t *testing.T,
	fixture *generatedSessionLifecycleFixture,
	eventTime time.Time,
) []factoryapi.FactoryEvent {
	t.Helper()
	return []factoryapi.FactoryEvent{
		{
			SchemaVersion: factoryapi.AgentFactoryEventV1,
			Id:            "event-orchestrator-phase-changed",
			Type:          factoryapi.FactoryEventTypeOrchestratorPhaseChanged,
			Context: factoryapi.FactoryEventContext{
				Sequence:         13,
				Tick:             7,
				EventTime:        eventTime,
				SessionId:        &fixture.sessionID,
				SessionSequence:  intPtr(fixture.nextSessionSequence()),
				OrchestratorKind: &fixture.orchestratorKind,
				PhaseId:          stringPtr("phase-execute"),
				PhaseName:        stringPtr("execute"),
				Source:           &fixture.source,
			},
			Payload: factoryEventPayload(t, factoryapi.OrchestratorPhaseChangedEventPayload{
				PreviousPhaseId:   stringPtr("phase-plan"),
				PreviousPhaseName: stringPtr("plan"),
				PhaseStatus:       factoryapi.ACTIVE,
				StartedAt:         &eventTime,
				ProgressSummary:   stringPtr("Entered execute phase"),
			}),
		},
		{
			SchemaVersion: factoryapi.AgentFactoryEventV1,
			Id:            "event-orchestrator-checkpoint-written",
			Type:          factoryapi.FactoryEventTypeOrchestratorCheckpointWritten,
			Context: factoryapi.FactoryEventContext{
				Sequence:         14,
				Tick:             8,
				EventTime:        eventTime,
				SessionId:        &fixture.sessionID,
				SessionSequence:  intPtr(fixture.nextSessionSequence()),
				OrchestratorKind: &fixture.orchestratorKind,
				PhaseId:          stringPtr("phase-execute"),
				PhaseName:        stringPtr("execute"),
				CheckpointId:     stringPtr("ckpt-2"),
				Source:           &fixture.source,
			},
			Payload: factoryEventPayload(t, factoryapi.OrchestratorCheckpointWrittenEventPayload{
				Label:                 "after-plan",
				Timestamp:             &eventTime,
				SourceHash:            stringPtr("sha256:source"),
				RuntimeSnapshotDigest: stringPtr("sha256:snapshot"),
				ArtifactRef: &factoryapi.FactoryArtifactRef{
					Id:         "artifact-ckpt-2",
					Kind:       factoryapi.FactoryArtifactKindCHECKPOINT,
					Visibility: factoryapi.FactoryArtifactVisibilityINTERNALCHECKPOINT,
				},
				ResumabilityStatus: factoryapi.RESUMABLE,
			}),
		},
	}
}

func generatedSessionLifecycleDispatchEvents(
	t *testing.T,
	fixture *generatedSessionLifecycleFixture,
	eventTime time.Time,
) []factoryapi.FactoryEvent {
	t.Helper()
	return []factoryapi.FactoryEvent{
		{
			SchemaVersion: factoryapi.AgentFactoryEventV1,
			Id:            "event-dispatch-queued",
			Type:          factoryapi.FactoryEventTypeDispatchQueued,
			Context: factoryapi.FactoryEventContext{
				Sequence:         15,
				Tick:             8,
				EventTime:        eventTime,
				SessionId:        &fixture.sessionID,
				SessionSequence:  intPtr(fixture.nextSessionSequence()),
				OrchestratorKind: &fixture.orchestratorKind,
				PhaseId:          stringPtr("phase-execute"),
				PhaseName:        stringPtr("execute"),
				DispatchId:       &fixture.dispatchID,
				Source:           &fixture.source,
			},
			Payload: factoryEventPayload(t, factoryapi.DispatchQueuedEventPayload{
				DispatchKind:  factoryapi.FactoryDispatchKindJAVASCRIPTAGENT,
				Label:         stringPtr("summarize findings"),
				QueuePosition: intPtr(0),
				PromptDigest:  stringPtr("sha256:prompt"),
				SchemaDigest:  stringPtr("sha256:schema"),
				InputWorkIds:  &[]string{"work-1"},
			}),
		},
		{
			SchemaVersion: factoryapi.AgentFactoryEventV1,
			Id:            "event-dispatch-interrupted",
			Type:          factoryapi.FactoryEventTypeDispatchInterrupted,
			Context: factoryapi.FactoryEventContext{
				Sequence:         16,
				Tick:             8,
				EventTime:        eventTime,
				SessionId:        &fixture.sessionID,
				SessionSequence:  intPtr(fixture.nextSessionSequence()),
				OrchestratorKind: &fixture.orchestratorKind,
				DispatchId:       &fixture.dispatchID,
				Source:           &fixture.source,
			},
			Payload: factoryEventPayload(t, factoryapi.DispatchInterruptedEventPayload{
				Reason:         "provider disconnected",
				ObservedStatus: factoryapi.FactoryDispatchStatusFAILED,
				InterruptedAt:  eventTime,
				RetryPlanned:   true,
			}),
		},
		{
			SchemaVersion: factoryapi.AgentFactoryEventV1,
			Id:            "event-dispatch-reconciled",
			Type:          factoryapi.FactoryEventTypeDispatchReconciled,
			Context: factoryapi.FactoryEventContext{
				Sequence:         17,
				Tick:             9,
				EventTime:        eventTime,
				SessionId:        &fixture.sessionID,
				SessionSequence:  intPtr(fixture.nextSessionSequence()),
				OrchestratorKind: &fixture.orchestratorKind,
				DispatchId:       &fixture.dispatchID,
				Source:           stringPtr("replay"),
			},
			Payload: factoryEventPayload(t, factoryapi.DispatchReconciledEventPayload{
				ReconciledStatus:     factoryapi.FactoryDispatchStatusCOMPLETED,
				ReconciliationSource: factoryapi.PROVIDERSESSION,
				Replayed:             true,
				ArtifactIds:          &[]string{"artifact-result-1"},
			}),
		},
	}
}

func generatedFactoryAgentRunEvents(t *testing.T) []factoryapi.FactoryEvent {
	t.Helper()
	eventTime := time.Date(2026, 4, 18, 12, 30, 0, 0, time.UTC)
	traceIDs := []string{"trace-1"}
	workIDs := []string{"work-1"}
	agentDispatchID := "dispatch-agent-1"
	toolPolicy := "DISABLED"
	executionBehavior := factoryapi.AgentRun
	toolCallCount := int32(0)
	return []factoryapi.FactoryEvent{
		{
			SchemaVersion: factoryapi.AgentFactoryEventV1,
			Id:            "event-agent-run-response",
			Type:          factoryapi.FactoryEventTypeAgentRunResponse,
			Context: factoryapi.FactoryEventContext{
				Sequence:   11,
				Tick:       2,
				EventTime:  eventTime,
				TraceIds:   &traceIDs,
				WorkIds:    &workIDs,
				DispatchId: &agentDispatchID,
			},
			Payload: factoryEventPayload(t, factoryapi.AgentRunResponseEventPayload{
				AgentRunId:     "agent-run-1",
				Outcome:        factoryapi.WorkOutcomeAccepted,
				DurationMillis: 420,
				Diagnostics: &factoryapi.SafeWorkDiagnostics{
					AgentRun: &factoryapi.SafeAgentRunDiagnostic{
						ExecutionBehavior: &executionBehavior,
						ToolPolicy:        &toolPolicy,
						ToolCallCount:     &toolCallCount,
						ToolDiagnostics:   &[]factoryapi.AgentRunToolDiagnosticEntry{},
						Transcript:        &[]factoryapi.AgentRunTranscriptEntry{},
					},
				},
			}),
		},
	}
}

func assertOpenAPI3RefPropertyDescription(t *testing.T, schema *openapi3.Schema, schemaName string, propertyName string) *openapi3.Schema {
	t.Helper()
	property, ok := schema.Properties[propertyName]
	if !ok || property == nil || property.Value == nil {
		t.Fatalf("%s.properties.%s is missing", schemaName, propertyName)
	}
	assertOpenAPI3Description(t, schemaName+".properties."+propertyName, property.Value.Description)
	return resolveOpenAPI3ReferencedPropertySchema(t, property.Value, schemaName+".properties."+propertyName)
}

func resolveOpenAPI3ReferencedPropertySchema(t *testing.T, schema *openapi3.Schema, path string) *openapi3.Schema {
	t.Helper()
	if schema == nil {
		t.Fatalf("%s schema is missing", path)
	}
	if len(schema.AllOf) == 1 {
		if schema.AllOf[0].Value != nil {
			return schema.AllOf[0].Value
		}
		if schema.AllOf[0].Ref != "" {
			t.Fatalf("%s allOf[0] ref %q is not resolved", path, schema.AllOf[0].Ref)
		}
	}
	return schema
}
