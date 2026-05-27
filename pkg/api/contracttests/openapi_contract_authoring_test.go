package apicontract_test

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/api/generated"
	"gopkg.in/yaml.v3"
)

func TestOpenAPIContract_FactoryEventEnvelopeRefsSharedSchemas(t *testing.T) {
	doc := loadBundledOpenAPIDocument(t)
	factoryEvent := schemaObject(t, componentSchemas(t, doc), "FactoryEvent")
	factoryEventProperties := schemaProperties(t, factoryEvent, "FactoryEvent")
	assertPropertyRef(t, factoryEventProperties, "type", "#/components/schemas/FactoryEventType")
	assertPropertyRef(t, factoryEventProperties, "context", "#/components/schemas/FactoryEventContext")
	assertPayloadUnionRefs(t, factoryEventProperties, bundledFactoryEventPayloadRefs)
}

func TestOpenAPIContract_BundledFactoryEventSchemasRemainComplete(t *testing.T) {
	doc := loadBundledOpenAPIDocument(t)
	schemas := componentSchemas(t, doc)
	assertSchemaNamesPresent(t, schemas, bundledFactoryEventContractSchemaNames)

	factoryEvent := schemaObject(t, schemas, "FactoryEvent")
	assertRequiredFields(t, factoryEvent, "schemaVersion", "id", "type", "context", "payload")
	factoryEventProperties := schemaProperties(t, factoryEvent, "FactoryEvent")
	assertPropertyRef(t, factoryEventProperties, "type", "#/components/schemas/FactoryEventType")
	assertPropertyRef(t, factoryEventProperties, "context", "#/components/schemas/FactoryEventContext")
	assertPayloadUnionRefs(t, factoryEventProperties, bundledFactoryEventPayloadRefs)
	assertEnumValues(t, schemaObject(t, schemas, "FactoryEventType"), "FactoryEventType", canonicalFactoryEventTypeValues)

	contextProperties := schemaProperties(t, schemaObject(t, schemas, "FactoryEventContext"), "FactoryEventContext")
	assertSchemaPropertiesPresent(t, contextProperties, "FactoryEventContext", "eventTime", "requestId", "traceIds", "workIds", "dispatchId", "currentChainingTraceId")
	assertStringArrayProperty(t, contextProperties, "previousChainingTraceIds")

	assertBundledEventPayloadRefs(t, schemas)
	assertBundledEventStreamRoute(t, doc)
}

func TestOpenAPIAuthoring_EventSchemasUseDedicatedFragments(t *testing.T) {
	doc := loadAuthoredOpenAPIDoc(t)
	schemas := componentSchemas(t, doc)
	expectedRefs := map[string]string{
		"FactoryEvent":                          "./components/schemas/events/FactoryEvent.yaml",
		"FactoryEventType":                      "./components/schemas/events/FactoryEventType.yaml",
		"FactoryEventContext":                   "./components/schemas/events/FactoryEventContext.yaml",
		"DispatchConsumedWorkRef":               "./components/schemas/events/DispatchConsumedWorkRef.yaml",
		"DispatchRequestEventMetadata":          "./components/schemas/events/DispatchRequestEventMetadata.yaml",
		"RunRequestEventPayload":                "./components/schemas/events/payloads/RunRequestEventPayload.yaml",
		"InitialStructureRequestEventPayload":   "./components/schemas/events/payloads/InitialStructureRequestEventPayload.yaml",
		"FactoryChangeEventPayload":             "./components/schemas/events/payloads/FactoryChangeEventPayload.yaml",
		"WorkRequestEventPayload":               "./components/schemas/events/payloads/WorkRequestEventPayload.yaml",
		"RelationshipChangeRequestEventPayload": "./components/schemas/events/payloads/RelationshipChangeRequestEventPayload.yaml",
		"DispatchRequestEventPayload":           "./components/schemas/events/payloads/DispatchRequestEventPayload.yaml",
		"ModelRequestEventPayload":              "./components/schemas/events/payloads/ModelRequestEventPayload.yaml",
		"ModelResponseEventPayload":             "./components/schemas/events/payloads/ModelResponseEventPayload.yaml",
		"InferenceRequestEventPayload":          "./components/schemas/events/payloads/InferenceRequestEventPayload.yaml",
		"InferenceResponseEventPayload":         "./components/schemas/events/payloads/InferenceResponseEventPayload.yaml",
		"ScriptRequestEventPayload":             "./components/schemas/events/payloads/ScriptRequestEventPayload.yaml",
		"ScriptResponseEventPayload":            "./components/schemas/events/payloads/ScriptResponseEventPayload.yaml",
		"DispatchResponseEventPayload":          "./components/schemas/events/payloads/DispatchResponseEventPayload.yaml",
		"FactoryStateResponseEventPayload":      "./components/schemas/events/payloads/FactoryStateResponseEventPayload.yaml",
		"RunResponseEventPayload":               "./components/schemas/events/payloads/RunResponseEventPayload.yaml",
		"InferenceOutcome":                      "./components/schemas/events/InferenceOutcome.yaml",
		"ScriptExecutionOutcome":                "./components/schemas/events/ScriptExecutionOutcome.yaml",
		"ScriptFailureType":                     "./components/schemas/events/ScriptFailureType.yaml",
		"FactoryState":                          "./components/schemas/events/FactoryState.yaml",
		"WorkOutcome":                           "./components/schemas/events/WorkOutcome.yaml",
		"WorkFailureFamily":                     "./components/schemas/events/WorkFailureFamily.yaml",
		"WorkFailureType":                       "./components/schemas/events/WorkFailureType.yaml",
		"ProviderFailureMetadata":               "./components/schemas/events/ProviderFailureMetadata.yaml",
		"ProviderSessionMetadata":               "./components/schemas/events/ProviderSessionMetadata.yaml",
		"WorkMetrics":                           "./components/schemas/events/WorkMetrics.yaml",
		"WorkDiagnostics":                       "./components/schemas/events/WorkDiagnostics.yaml",
		"RenderedPromptDiagnostic":              "./components/schemas/events/RenderedPromptDiagnostic.yaml",
		"ProviderDiagnostic":                    "./components/schemas/events/ProviderDiagnostic.yaml",
		"Diagnostics":                           "./components/schemas/events/Diagnostics.yaml",
		"SafeWorkDiagnostics":                   "./components/schemas/events/SafeWorkDiagnostics.yaml",
		"WallClock":                             "./components/schemas/events/WallClock.yaml",
	}
	for schemaName, wantRef := range expectedRefs {
		assertSchemaRef(t, schemas, schemaName, wantRef)
	}
	if _, ok := schemas["payloads"]; ok {
		t.Fatal("components.schemas.payloads must not be reintroduced as a monolithic event payload source")
	}

	paths, ok := doc["paths"].(map[string]any)
	if !ok {
		t.Fatal("paths object is missing")
	}
	assertEventStreamSchemaRef(t, pathOperation(t, paths, "/events", "get"), "#/components/schemas/FactoryEvent")
}

func TestOpenAPIAuthoring_FactoryWorldSchemasUseDedicatedFragments(t *testing.T) {
	doc := loadAuthoredOpenAPIDoc(t)
	schemas := componentSchemas(t, doc)
	expectedRefs := map[string]string{
		"FactoryWorldWorkstationRequestProjectionSlice": "./components/schemas/factory-world/FactoryWorldWorkstationRequestProjectionSlice.yaml",
		"FactoryWorldRenderedPromptDiagnostic":          "./components/schemas/factory-world/FactoryWorldRenderedPromptDiagnostic.yaml",
		"FactoryWorldProviderDiagnostic":                "./components/schemas/factory-world/FactoryWorldProviderDiagnostic.yaml",
		"FactoryWorldWorkDiagnostics":                   "./components/schemas/factory-world/FactoryWorldWorkDiagnostics.yaml",
		"FactoryWorldWorkItemRef":                       "./components/schemas/factory-world/FactoryWorldWorkItemRef.yaml",
		"FactoryWorldTokenView":                         "./components/schemas/factory-world/FactoryWorldTokenView.yaml",
		"FactoryWorldMutationView":                      "./components/schemas/factory-world/FactoryWorldMutationView.yaml",
		"FactoryWorldScriptRequestView":                 "./components/schemas/factory-world/FactoryWorldScriptRequestView.yaml",
		"FactoryWorldScriptResponseView":                "./components/schemas/factory-world/FactoryWorldScriptResponseView.yaml",
		"FactoryWorldWorkstationRequestCountView":       "./components/schemas/factory-world/FactoryWorldWorkstationRequestCountView.yaml",
		"FactoryWorldWorkstationRequestRequestView":     "./components/schemas/factory-world/FactoryWorldWorkstationRequestRequestView.yaml",
		"FactoryWorldWorkstationRequestResponseView":    "./components/schemas/factory-world/FactoryWorldWorkstationRequestResponseView.yaml",
		"FactoryWorldWorkstationRequestView":            "./components/schemas/factory-world/FactoryWorldWorkstationRequestView.yaml",
	}
	for schemaName, wantRef := range expectedRefs {
		assertSchemaRef(t, schemas, schemaName, wantRef)
	}
}

func TestOpenAPIAuthoring_APISchemasUseDedicatedFragments(t *testing.T) {
	doc := loadAuthoredOpenAPIDoc(t)
	schemas := componentSchemas(t, doc)
	expectedRefs := map[string]string{
		"SubmitWorkRequest":                   "./components/schemas/api/SubmitWorkRequest.yaml",
		"SubmitRelation":                      "./components/schemas/api/SubmitRelation.yaml",
		"SubmitWorkResponse":                  "./components/schemas/api/SubmitWorkResponse.yaml",
		"UpsertWorkRequestResponse":           "./components/schemas/api/UpsertWorkRequestResponse.yaml",
		"ListWorkResponse":                    "./components/schemas/api/ListWorkResponse.yaml",
		"PaginationContext":                   "./components/schemas/api/PaginationContext.yaml",
		"TokenResponse":                       "./components/schemas/api/TokenResponse.yaml",
		"TokenHistory":                        "./components/schemas/api/TokenHistory.yaml",
		"StatusCategories":                    "./components/schemas/api/StatusCategories.yaml",
		"StatusResponse":                      "./components/schemas/api/StatusResponse.yaml",
		"ListModelsResponse":                  "./components/schemas/api/ListModelsResponse.yaml",
		"ModelSummary":                        "./components/schemas/api/ModelSummary.yaml",
		"ModelDetail":                         "./components/schemas/api/ModelDetail.yaml",
		"ModelInvocationRequest":              "./components/schemas/api/ModelInvocationRequest.yaml",
		"ModelInvocationOptions":              "./components/schemas/api/ModelInvocationOptions.yaml",
		"ModelInvocationResponseMode":         "./components/schemas/api/ModelInvocationResponseMode.yaml",
		"ModelInvocationResponse":             "./components/schemas/api/ModelInvocationResponse.yaml",
		"ResolvedModelOperationBinding":       "./components/schemas/api/ResolvedModelOperationBinding.yaml",
		"ResolvedModelOperationBindingSource": "./components/schemas/api/ResolvedModelOperationBindingSource.yaml",
		"ErrorFamily":                         "./components/schemas/api/ErrorFamily.yaml",
		"ErrorResponse":                       "./components/schemas/api/ErrorResponse.yaml",
		"WorkRequest":                         "./components/schemas/api/WorkRequest.yaml",
		"WorkRequestType":                     "./components/schemas/api/WorkRequestType.yaml",
	}
	for schemaName, wantRef := range expectedRefs {
		assertSchemaRef(t, schemas, schemaName, wantRef)
	}
}

func TestOpenAPIAuthoring_DataModelSchemasUseDedicatedFragments(t *testing.T) {
	doc := loadAuthoredOpenAPIDoc(t)
	schemas := componentSchemas(t, doc)
	expectedRefs := map[string]string{
		"FactoryName":         "./components/schemas/data-models/FactoryName.yaml",
		"Factory":             "./components/schemas/data-models/Factory.yaml",
		"FactoryGuard":        "./components/schemas/data-models/FactoryGuard.yaml",
		"ResourceManifest":    "./components/schemas/data-models/ResourceManifest.yaml",
		"RequiredTool":        "./components/schemas/data-models/RequiredTool.yaml",
		"BundledFile":         "./components/schemas/data-models/BundledFile.yaml",
		"BundledFileContent":  "./components/schemas/data-models/BundledFileContent.yaml",
		"InputType":           "./components/schemas/data-models/InputType.yaml",
		"InputKind":           "./components/schemas/data-models/InputKind.yaml",
		"WorkType":            "./components/schemas/data-models/WorkType.yaml",
		"WorkState":           "./components/schemas/data-models/WorkState.yaml",
		"WorkStateType":       "./components/schemas/data-models/WorkStateType.yaml",
		"Resource":            "./components/schemas/data-models/Resource.yaml",
		"ResourceType":        "./components/schemas/data-models/ResourceType.yaml",
		"Worker":              "./components/schemas/data-models/Worker.yaml",
		"WorkerType":          "./components/schemas/data-models/WorkerType.yaml",
		"WorkerModelProvider": "./components/schemas/data-models/WorkerModelProvider.yaml",
		"WorkerProvider":      "./components/schemas/data-models/WorkerProvider.yaml",
		"Workstation":         "./components/schemas/data-models/Workstation.yaml",
		"WorkstationLimits":   "./components/schemas/data-models/WorkstationLimits.yaml",
		"WorkstationKind":     "./components/schemas/data-models/WorkstationKind.yaml",
		"WorkstationType":     "./components/schemas/data-models/WorkstationType.yaml",
		"WorkstationCron":     "./components/schemas/data-models/WorkstationCron.yaml",
		"GuardType":           "./components/schemas/data-models/GuardType.yaml",
		"Guard":               "./components/schemas/data-models/Guard.yaml",
		"GuardMatchConfig":    "./components/schemas/data-models/GuardMatchConfig.yaml",
		"WorkstationIO":       "./components/schemas/data-models/WorkstationIO.yaml",
		"Transition":          "./components/schemas/data-models/Transition.yaml",
		"Work":                "./components/schemas/data-models/Work.yaml",
		"Relation":            "./components/schemas/data-models/Relation.yaml",
		"RelationType":        "./components/schemas/data-models/RelationType.yaml",
	}
	for schemaName, wantRef := range expectedRefs {
		assertSchemaRef(t, schemas, schemaName, wantRef)
	}
}

func TestOpenAPIContract_FactoryExampleUsesGuardedLoopBreaker(t *testing.T) {
	doc := loadValidatedOpenAPIContract(t)
	factorySchema := doc.Components.Schemas["Factory"].Value
	example, ok := factorySchema.Example.(map[string]any)
	if !ok {
		t.Fatalf("Factory.example must be an object, got %T", factorySchema.Example)
	}
	if _, ok := example["exhaustion_rules"]; ok {
		t.Fatalf("Factory.example must not advertise exhaustion_rules")
	}
	if err := factorySchema.VisitJSON(example); err != nil {
		t.Fatalf("Factory.example should validate: %v", err)
	}

	workstations, ok := example["workstations"].([]any)
	if !ok {
		t.Fatalf("Factory.example.workstations must be an array")
	}
	foundGuardedLoopBreaker := false
	for _, item := range workstations {
		workstation, ok := item.(map[string]any)
		if !ok || workstation["type"] != "LOGICAL_MOVE" {
			continue
		}
		guards, ok := workstation["guards"].([]any)
		if !ok || len(guards) == 0 {
			continue
		}
		guard, ok := guards[0].(map[string]any)
		if ok && guard["type"] == "VISIT_COUNT" {
			foundGuardedLoopBreaker = true
			break
		}
	}
	if !foundGuardedLoopBreaker {
		t.Fatal("Factory.example must include a guarded LOGICAL_MOVE workstation using a VISIT_COUNT guard")
	}
}

func TestOpenAPIContract_GeneratedFactoryModelRetiresExhaustionRules(t *testing.T) {
	factoryType := reflect.TypeOf(generated.Factory{})
	assertGeneratedFactoryTypeRetiresExhaustionRules(t, factoryType)
	payload := generatedFactoryLoopBreakerPayload(t)
	if _, ok := payload["exhaustion_rules"]; ok {
		t.Fatal("generated.Factory payload must not include exhaustion_rules")
	}
	if _, ok := payload["exhaustionRules"]; ok {
		t.Fatal("generated.Factory payload must not include exhaustionRules")
	}
	assertGeneratedFactoryLoopBreakerPayload(t, payload)
}

func TestOpenAPIContract_GeneratedProviderFailureModelUsesPublishedFailureEnums(t *testing.T) {
	assertGeneratedFieldType(t, reflect.TypeOf(generated.ProviderFailureMetadata{}), "Family", reflect.TypeOf((*generated.WorkFailureFamily)(nil)))
	assertGeneratedFieldType(t, reflect.TypeOf(generated.ProviderFailureMetadata{}), "Type", reflect.TypeOf((*generated.WorkFailureType)(nil)))
}

func TestOpenAPIContract_WorkerSchemaAndGeneratedModelRetireLegacyFields(t *testing.T) {
	doc := loadBundledOpenAPIDocument(t)
	workerSchema := schemaObject(t, componentSchemas(t, doc), "Worker")
	workerProperties := schemaProperties(t, workerSchema, "Worker")
	assertSchemaPropertiesPresent(t, workerProperties, "Worker", "name", "provider", "modelProvider", "modelLocality", "executorProvider", "operations")
	assertPropertiesAbsent(t, workerProperties, "Worker", "sessionId", "concurrency")

	modelProviderProperty, ok := workerProperties["modelProvider"].(map[string]any)
	if !ok {
		t.Fatal("Worker.properties.modelProvider must be an object")
	}
	modelProviderDescription, _ := modelProviderProperty["description"].(string)
	if !strings.Contains(modelProviderDescription, "CLAUDE") || !strings.Contains(modelProviderDescription, "CODEX") {
		t.Fatalf("Worker.properties.modelProvider.description must document built-in values, got %q", modelProviderDescription)
	}
	executorProviderProperty, ok := workerProperties["executorProvider"].(map[string]any)
	if !ok {
		t.Fatal("Worker.properties.executorProvider must be an object")
	}
	executorProviderDescription, _ := executorProviderProperty["description"].(string)
	if !strings.Contains(executorProviderDescription, "SCRIPT_WRAP") {
		t.Fatalf("Worker.properties.executorProvider.description must document the public built-in value, got %q", executorProviderDescription)
	}

	workerType := reflect.TypeOf(generated.Worker{})
	for _, field := range []string{"Provider", "ExecutorProvider", "ModelProvider", "ModelLocality", "Operations"} {
		if _, ok := workerType.FieldByName(field); !ok {
			t.Fatalf("generated.Worker must expose %s", field)
		}
	}
	for _, retired := range []string{"SessionId", "Concurrency"} {
		if _, ok := workerType.FieldByName(retired); ok {
			t.Fatalf("generated.Worker must not expose %s", retired)
		}
	}

	executorProvider := generated.WorkerProviderScriptWrap
	modelProvider := generated.WorkerModelProviderClaude
	modelLocality := generated.WorkerModelLocalityLocal
	payloadBytes, err := json.Marshal(generated.Worker{
		Name:             "executor",
		ExecutorProvider: &executorProvider,
		ModelLocality:    &modelLocality,
		ModelProvider:    &modelProvider,
	})
	if err != nil {
		t.Fatalf("marshal generated.Worker: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		t.Fatalf("unmarshal generated.Worker payload: %v", err)
	}
	if payload["executorProvider"] != string(executorProvider) || payload["modelProvider"] != string(modelProvider) || payload["modelLocality"] != string(modelLocality) {
		t.Fatalf("generated.Worker payload = %#v, want canonical provider fields", payload)
	}
	assertJSONKeysAbsent(t, payload, "generated.Worker payload", "provider", "sessionId", "concurrency")
}

func loadAuthoredOpenAPIDoc(t *testing.T) map[string]any {
	t.Helper()
	data, err := os.ReadFile("../../../api/openapi-main.yaml")
	if err != nil {
		t.Fatalf("read authored openapi contract: %v", err)
	}
	var doc map[string]any
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parse authored openapi contract: %v", err)
	}
	return doc
}

func assertBundledEventPayloadRefs(t *testing.T, schemas map[string]any) {
	t.Helper()
	inferenceResponseProperties := schemaProperties(t, schemaObject(t, schemas, "InferenceResponseEventPayload"), "InferenceResponseEventPayload")
	assertPropertyRef(t, inferenceResponseProperties, "outcome", "#/components/schemas/InferenceOutcome")
	assertPropertyRef(t, inferenceResponseProperties, "providerSession", "#/components/schemas/ProviderSessionMetadata")
	assertPropertyRef(t, inferenceResponseProperties, "diagnostics", "#/components/schemas/SafeWorkDiagnostics")

	scriptResponseProperties := schemaProperties(t, schemaObject(t, schemas, "ScriptResponseEventPayload"), "ScriptResponseEventPayload")
	assertPropertyRef(t, scriptResponseProperties, "outcome", "#/components/schemas/ScriptExecutionOutcome")
	assertPropertyRef(t, scriptResponseProperties, "failureType", "#/components/schemas/ScriptFailureType")

	dispatchResponseProperties := schemaProperties(t, schemaObject(t, schemas, "DispatchResponseEventPayload"), "DispatchResponseEventPayload")
	assertPropertyRef(t, dispatchResponseProperties, "outcome", "#/components/schemas/WorkOutcome")
	assertPropertyRef(t, dispatchResponseProperties, "providerFailure", "#/components/schemas/ProviderFailureMetadata")
	assertPropertyRef(t, dispatchResponseProperties, "metrics", "#/components/schemas/WorkMetrics")
	workMetricsProperties := schemaProperties(t, schemaObject(t, schemas, "WorkMetrics"), "WorkMetrics")
	assertInt64Property(t, workMetricsProperties, "durationMillis")
	assertPropertiesAbsent(t, workMetricsProperties, "WorkMetrics", "durationNanos")
	assertPublishedWorkFailureSchemas(t, schemas, schemaProperties(t, schemaObject(t, schemas, "ProviderFailureMetadata"), "ProviderFailureMetadata"))

	providerFailureProperties := schemaProperties(t, schemaObject(t, schemas, "ProviderFailureMetadata"), "ProviderFailureMetadata")
	assertPublishedWorkFailureSchemas(t, schemas, providerFailureProperties)

	stateResponseProperties := schemaProperties(t, schemaObject(t, schemas, "FactoryStateResponseEventPayload"), "FactoryStateResponseEventPayload")
	assertPropertyRef(t, stateResponseProperties, "previousState", "#/components/schemas/FactoryState")
	assertPropertyRef(t, stateResponseProperties, "state", "#/components/schemas/FactoryState")

	runResponseProperties := schemaProperties(t, schemaObject(t, schemas, "RunResponseEventPayload"), "RunResponseEventPayload")
	assertPropertyRef(t, runResponseProperties, "state", "#/components/schemas/FactoryState")
	assertPropertyRef(t, runResponseProperties, "wallClock", "#/components/schemas/WallClock")
	assertPropertyRef(t, runResponseProperties, "diagnostics", "#/components/schemas/Diagnostics")
}

func assertPublishedWorkFailureSchemas(t *testing.T, schemas map[string]any, providerFailureProperties map[string]any) {
	t.Helper()
	assertPropertyRef(t, providerFailureProperties, "family", "#/components/schemas/WorkFailureFamily")
	assertPropertyRef(t, providerFailureProperties, "type", "#/components/schemas/WorkFailureType")
	assertEnumValues(t, schemaObject(t, schemas, "WorkFailureFamily"), "WorkFailureFamily", []string{"terminal", "retryable", "throttle"})
	assertEnumValues(t, schemaObject(t, schemas, "WorkFailureType"), "WorkFailureType", []string{
		"auth_failure",
		"permanent_bad_request",
		"throttled",
		"internal_server_error",
		"timeout",
		"unknown",
		"misconfigured",
	})
}

func assertBundledEventStreamRoute(t *testing.T, doc map[string]any) {
	t.Helper()
	paths, ok := doc["paths"].(map[string]any)
	if !ok {
		t.Fatal("paths object is missing")
	}
	assertEventStreamSchemaRef(t, pathOperation(t, paths, "/events", "get"), "#/components/schemas/FactoryEvent")
}

func assertGeneratedFactoryTypeRetiresExhaustionRules(t *testing.T, factoryType reflect.Type) {
	t.Helper()
	if _, ok := factoryType.FieldByName("ExhaustionRules"); ok {
		t.Fatal("generated.Factory must not expose an ExhaustionRules field")
	}
	for i := 0; i < factoryType.NumField(); i++ {
		field := factoryType.Field(i)
		jsonTag := strings.Split(field.Tag.Get("json"), ",")[0]
		if jsonTag == "exhaustion_rules" || jsonTag == "exhaustionRules" {
			t.Fatalf("generated.Factory must not expose json field %q", jsonTag)
		}
	}
}

func generatedFactoryLoopBreakerPayload(t *testing.T) map[string]any {
	t.Helper()
	logicalMoveType := generated.WorkstationTypeLogicalMove
	guardedWorkstation := "review-story"
	maxVisits := 3
	factory := generated.Factory{
		Workstations: &[]generated.Workstation{{
			Name:    "review-story-loop-breaker",
			Worker:  "logical-move",
			Type:    &logicalMoveType,
			Inputs:  []generated.WorkstationIO{{WorkType: "story", State: "in_review"}},
			Outputs: &[]generated.WorkstationIO{{WorkType: "story", State: "failed"}},
			Guards: &[]generated.Guard{{
				Type:        generated.GuardTypeVisitCount,
				Workstation: &guardedWorkstation,
				MaxVisits:   &maxVisits,
			}},
		}},
	}

	payloadBytes, err := json.Marshal(factory)
	if err != nil {
		t.Fatalf("marshal generated.Factory: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		t.Fatalf("unmarshal generated.Factory payload: %v", err)
	}
	return payload
}

func assertGeneratedFactoryLoopBreakerPayload(t *testing.T, payload map[string]any) {
	t.Helper()
	workstations, ok := payload["workstations"].([]any)
	if !ok || len(workstations) != 1 {
		t.Fatalf("generated.Factory payload must contain one workstation, got %#v", payload["workstations"])
	}
	workstation, ok := workstations[0].(map[string]any)
	if !ok {
		t.Fatalf("generated.Factory workstation payload must be an object, got %T", workstations[0])
	}
	if workstation["type"] != "LOGICAL_MOVE" {
		t.Fatalf("generated.Factory workstation type = %#v, want LOGICAL_MOVE", workstation["type"])
	}
	guards, ok := workstation["guards"].([]any)
	if !ok || len(guards) != 1 {
		t.Fatalf("generated.Factory workstation guards = %#v, want one VISIT_COUNT guard", workstation["guards"])
	}
	guard, ok := guards[0].(map[string]any)
	if !ok {
		t.Fatalf("generated.Factory workstation guard must be an object, got %T", guards[0])
	}
	if guard["type"] != string(generated.GuardTypeVisitCount) {
		t.Fatalf("generated.Factory workstation guard type = %#v, want %q", guard["type"], generated.GuardTypeVisitCount)
	}
	if guard["workstation"] != "review-story" {
		t.Fatalf("generated.Factory workstation guard workstation = %#v, want %q", guard["workstation"], "review-story")
	}
	if got, ok := guard["maxVisits"].(float64); !ok || int(got) != 3 {
		t.Fatalf("generated.Factory workstation guard maxVisits = %#v, want %d", guard["maxVisits"], 3)
	}
}
