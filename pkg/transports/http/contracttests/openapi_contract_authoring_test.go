package apicontract_test

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	generated "github.com/portpowered/infinite-you/pkg/transports/http/generated"
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
	assertSchemaPropertiesPresent(
		t,
		contextProperties,
		"FactoryEventContext",
		"eventTime",
		"sessionId",
		"sessionSequence",
		"orchestratorKind",
		"orchestratorDialect",
		"phaseId",
		"phaseName",
		"checkpointId",
		"requestId",
		"traceIds",
		"workIds",
		"dispatchId",
		"currentChainingTraceId",
		"source",
	)
	assertStringArrayProperty(t, contextProperties, "previousChainingTraceIds")

	assertBundledEventPayloadRefs(t, schemas)
	assertBundledEventStreamRoute(t, doc)
}

func TestOpenAPIContract_ResponseEventStreamIsBundledWithTypedOutcomes(t *testing.T) {
	doc := loadBundledOpenAPIDocument(t)
	paths := objectField(t, doc, "paths")
	operation := pathOperation(t, paths, responseEventStreamPath, "get")
	assertEventStreamSchemaRef(t, operation, "#/components/schemas/FactoryResponseEvent")
	for status, ref := range map[string]string{
		"400": "#/components/responses/ResponseEventBadRequest",
		"404": "#/components/responses/ResponseEventSessionNotFound",
		"410": "#/components/responses/ResponseEventStreamExpired",
		"500": "#/components/responses/InternalError",
	} {
		assertResponseRef(t, operation, status, ref)
	}
	for _, ref := range []string{
		"#/components/parameters/SessionID",
		"#/components/parameters/ResponseEventAfterSequence",
		"#/components/parameters/ResponseEventDispatchID",
		"#/components/parameters/ResponseEventKind",
	} {
		assertParameterRef(t, operation["parameters"].([]any), ref)
	}

	bundledParameters := objectField(t, objectField(t, doc, "components"), "parameters")
	afterSequence := objectField(t, bundledParameters, "ResponseEventAfterSequence")
	afterSequenceSchema := objectField(t, afterSequence, "schema")
	if got := afterSequenceSchema["format"]; got != "int64" {
		t.Fatalf("bundled after_sequence format = %v, want int64", got)
	}
	if got := afterSequenceSchema["minimum"]; got != 0 {
		t.Fatalf("bundled after_sequence minimum = %v, want 0", got)
	}
	kind := objectField(t, bundledParameters, "ResponseEventKind")
	if kind["style"] != "form" || kind["explode"] != true {
		t.Fatalf("bundled kind repetition encoding = style:%v explode:%v, want form/true", kind["style"], kind["explode"])
	}
	if got := objectField(t, objectField(t, kind, "schema"), "items")["$ref"]; got != "#/components/schemas/FactoryResponseEventKind" {
		t.Fatalf("bundled kind items ref = %v", got)
	}
}

func TestOpenAPIAuthoring_EventSchemasUseDedicatedFragments(t *testing.T) {
	doc := loadAuthoredOpenAPIDoc(t)
	schemas := componentSchemas(t, doc)
	expectedRefs := map[string]string{
		"FactoryEvent":                              "./components/schemas/events/FactoryEvent.yaml",
		"FactoryEventType":                          "./components/schemas/events/FactoryEventType.yaml",
		"FactoryEventContext":                       "./components/schemas/events/FactoryEventContext.yaml",
		"DispatchConsumedWorkRef":                   "./components/schemas/events/DispatchConsumedWorkRef.yaml",
		"DispatchRequestEventMetadata":              "./components/schemas/events/DispatchRequestEventMetadata.yaml",
		"RunRequestEventPayload":                    "./components/schemas/events/payloads/RunRequestEventPayload.yaml",
		"InitialStructureRequestEventPayload":       "./components/schemas/events/payloads/InitialStructureRequestEventPayload.yaml",
		"FactoryChangeEventPayload":                 "./components/schemas/events/payloads/FactoryChangeEventPayload.yaml",
		"WorkRequestEventPayload":                   "./components/schemas/events/payloads/WorkRequestEventPayload.yaml",
		"RelationshipChangeRequestEventPayload":     "./components/schemas/events/payloads/RelationshipChangeRequestEventPayload.yaml",
		"DispatchRequestEventPayload":               "./components/schemas/events/payloads/DispatchRequestEventPayload.yaml",
		"ModelRequestEventPayload":                  "./components/schemas/events/payloads/ModelRequestEventPayload.yaml",
		"ModelResponseEventPayload":                 "./components/schemas/events/payloads/ModelResponseEventPayload.yaml",
		"InferenceRequestEventPayload":              "./components/schemas/events/payloads/InferenceRequestEventPayload.yaml",
		"InferenceResponseEventPayload":             "./components/schemas/events/payloads/InferenceResponseEventPayload.yaml",
		"ScriptRequestEventPayload":                 "./components/schemas/events/payloads/ScriptRequestEventPayload.yaml",
		"ScriptResponseEventPayload":                "./components/schemas/events/payloads/ScriptResponseEventPayload.yaml",
		"AgentRunResponseEventPayload":              "./components/schemas/events/payloads/AgentRunResponseEventPayload.yaml",
		"SafeAgentRunDiagnostic":                    "./components/schemas/events/SafeAgentRunDiagnostic.yaml",
		"AgentRunToolDiagnosticEntry":               "./components/schemas/events/AgentRunToolDiagnosticEntry.yaml",
		"AgentRunTranscriptEntry":                   "./components/schemas/events/AgentRunTranscriptEntry.yaml",
		"DispatchResponseEventPayload":              "./components/schemas/events/payloads/DispatchResponseEventPayload.yaml",
		"WorkStateChangeEventPayload":               "./components/schemas/events/payloads/WorkStateChangeEventPayload.yaml",
		"WorkStateChangeSource":                     "./components/schemas/events/WorkStateChangeSource.yaml",
		"FactoryStateResponseEventPayload":          "./components/schemas/events/payloads/FactoryStateResponseEventPayload.yaml",
		"RunResponseEventPayload":                   "./components/schemas/events/payloads/RunResponseEventPayload.yaml",
		"FactoryEventSessionResultStatus":           "./components/schemas/events/FactoryEventSessionResultStatus.yaml",
		"OrchestratorPhaseStatus":                   "./components/schemas/events/OrchestratorPhaseStatus.yaml",
		"CheckpointResumabilityStatus":              "./components/schemas/events/CheckpointResumabilityStatus.yaml",
		"DispatchReconciliationSource":              "./components/schemas/events/DispatchReconciliationSource.yaml",
		"SessionStartedEventPayload":                "./components/schemas/events/payloads/SessionStartedEventPayload.yaml",
		"SessionPausedEventPayload":                 "./components/schemas/events/payloads/SessionPausedEventPayload.yaml",
		"SessionResumedEventPayload":                "./components/schemas/events/payloads/SessionResumedEventPayload.yaml",
		"SessionResultUpdatedEventPayload":          "./components/schemas/events/payloads/SessionResultUpdatedEventPayload.yaml",
		"SessionCompletedEventPayload":              "./components/schemas/events/payloads/SessionCompletedEventPayload.yaml",
		"OrchestratorPhaseChangedEventPayload":      "./components/schemas/events/payloads/OrchestratorPhaseChangedEventPayload.yaml",
		"OrchestratorCheckpointWrittenEventPayload": "./components/schemas/events/payloads/OrchestratorCheckpointWrittenEventPayload.yaml",
		"DispatchQueuedEventPayload":                "./components/schemas/events/payloads/DispatchQueuedEventPayload.yaml",
		"DispatchInterruptedEventPayload":           "./components/schemas/events/payloads/DispatchInterruptedEventPayload.yaml",
		"DispatchReconciledEventPayload":            "./components/schemas/events/payloads/DispatchReconciledEventPayload.yaml",
		"JavaScriptCheckpointRefEventPayload":       "./components/schemas/events/payloads/JavaScriptCheckpointRefEventPayload.yaml",
		"InferenceOutcome":                          "./components/schemas/events/InferenceOutcome.yaml",
		"ScriptExecutionOutcome":                    "./components/schemas/events/ScriptExecutionOutcome.yaml",
		"ScriptFailureType":                         "./components/schemas/events/ScriptFailureType.yaml",
		"FactoryState":                              "./components/schemas/events/FactoryState.yaml",
		"WorkOutcome":                               "./components/schemas/events/WorkOutcome.yaml",
		"WorkFailureFamily":                         "./components/schemas/events/WorkFailureFamily.yaml",
		"WorkFailureType":                           "./components/schemas/events/WorkFailureType.yaml",
		"ProviderFailureMetadata":                   "./components/schemas/events/ProviderFailureMetadata.yaml",
		"ProviderSessionMetadata":                   "./components/schemas/events/ProviderSessionMetadata.yaml",
		"WorkMetrics":                               "./components/schemas/events/WorkMetrics.yaml",
		"WorkDiagnostics":                           "./components/schemas/events/WorkDiagnostics.yaml",
		"RenderedPromptDiagnostic":                  "./components/schemas/events/RenderedPromptDiagnostic.yaml",
		"ProviderDiagnostic":                        "./components/schemas/events/ProviderDiagnostic.yaml",
		"InvocationDiagnostic":                      "./components/schemas/events/InvocationDiagnostic.yaml",
		"InvocationParameterDiagnostic":             "./components/schemas/events/InvocationParameterDiagnostic.yaml",
		"Diagnostics":                               "./components/schemas/events/Diagnostics.yaml",
		"SafeWorkDiagnostics":                       "./components/schemas/events/SafeWorkDiagnostics.yaml",
		"WallClock":                                 "./components/schemas/events/WallClock.yaml",
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
	assertEventStreamSchemaRef(t, pathOperation(t, paths, "/factory-sessions/{session_id}/events", "get"), "#/components/schemas/FactoryEvent")
}

func TestOpenAPIAuthoring_FactoryWorldSchemasUseDedicatedFragments(t *testing.T) {
	doc := loadAuthoredOpenAPIDoc(t)
	schemas := componentSchemas(t, doc)
	expectedRefs := map[string]string{
		"FactoryWorldWorkstationRequestProjectionSlice": "./components/schemas/factory-world/FactoryWorldWorkstationRequestProjectionSlice.yaml",
		"FactoryWorldWorkMoveOperationProjectionSlice":  "./components/schemas/factory-world/FactoryWorldWorkMoveOperationProjectionSlice.yaml",
		"FactoryWorldWorkMoveOperationView":             "./components/schemas/factory-world/FactoryWorldWorkMoveOperationView.yaml",
		"FactoryWorldRenderedPromptDiagnostic":          "./components/schemas/factory-world/FactoryWorldRenderedPromptDiagnostic.yaml",
		"FactoryWorldProviderDiagnostic":                "./components/schemas/factory-world/FactoryWorldProviderDiagnostic.yaml",
		"FactoryWorldInvocationDiagnostic":              "./components/schemas/factory-world/FactoryWorldInvocationDiagnostic.yaml",
		"FactoryWorldInvocationParameterDiagnostic":     "./components/schemas/factory-world/FactoryWorldInvocationParameterDiagnostic.yaml",
		"FactoryWorldWorkDiagnostics":                   "./components/schemas/factory-world/FactoryWorldWorkDiagnostics.yaml",
		"FactoryWorldWorkItemRef":                       "./components/schemas/factory-world/FactoryWorldWorkItemRef.yaml",
		"FactoryWorldTokenView":                         "./components/schemas/factory-world/FactoryWorldTokenView.yaml",
		"FactoryWorldMutationView":                      "./components/schemas/factory-world/FactoryWorldMutationView.yaml",
		"FactoryWorldScriptRequestView":                 "./components/schemas/factory-world/FactoryWorldScriptRequestView.yaml",
		"FactoryWorldScriptResponseView":                "./components/schemas/factory-world/FactoryWorldScriptResponseView.yaml",
		"FactoryWorldAgentRunInspectionView":            "./components/schemas/factory-world/FactoryWorldAgentRunInspectionView.yaml",
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
		"InvocationInputSourceKind":           "./components/schemas/api/InvocationInputSourceKind.yaml",
		"InvocationRequest":                   "./components/schemas/api/InvocationRequest.yaml",
		"InvocationResponse":                  "./components/schemas/api/InvocationResponse.yaml",
		"InvocationTerminalStatus":            "./components/schemas/api/InvocationTerminalStatus.yaml",
		"UpsertWorkRequestSubmittedWork":      "./components/schemas/api/UpsertWorkRequestSubmittedWork.yaml",
		"UpsertWorkRequestResponse":           "./components/schemas/api/UpsertWorkRequestResponse.yaml",
		"MoveWorkRequest":                     "./components/schemas/api/MoveWorkRequest.yaml",
		"ListWorkResponse":                    "./components/schemas/api/ListWorkResponse.yaml",
		"PaginationContext":                   "./components/schemas/api/PaginationContext.yaml",
		"TokenResponse":                       "./components/schemas/api/TokenResponse.yaml",
		"TokenHistory":                        "./components/schemas/api/TokenHistory.yaml",
		"StatusCategories":                    "./components/schemas/api/StatusCategories.yaml",
		"StatusResponse":                      "./components/schemas/api/StatusResponse.yaml",
		"ListModelsResponse":                  "./components/schemas/api/ListModelsResponse.yaml",
		"ManagedRuntime":                      "./components/schemas/api/ManagedRuntime.yaml",
		"ManagedRuntimeLifecycleState":        "./components/schemas/api/ManagedRuntimeLifecycleState.yaml",
		"ManagedRuntimePullOutcome":           "./components/schemas/api/ManagedRuntimePullOutcome.yaml",
		"ManagedRuntimePullResult":            "./components/schemas/api/ManagedRuntimePullResult.yaml",
		"ManagedRuntimeReadinessState":        "./components/schemas/api/ManagedRuntimeReadinessState.yaml",
		"ManagedRuntimeSourceDiagnostics":     "./components/schemas/api/ManagedRuntimeSourceDiagnostics.yaml",
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
		"FactoryName":            "./components/schemas/data-models/FactoryName.yaml",
		"Factory":                "./components/schemas/data-models/Factory.yaml",
		"InvocationReturn":       "./components/schemas/data-models/InvocationReturn.yaml",
		"InvocationReturnPolicy": "./components/schemas/data-models/InvocationReturnPolicy.yaml",
		"FactoryGuard":           "./components/schemas/data-models/FactoryGuard.yaml",
		"ResourceManifest":       "./components/schemas/data-models/ResourceManifest.yaml",
		"RequiredTool":           "./components/schemas/data-models/RequiredTool.yaml",
		"BundledFile":            "./components/schemas/data-models/BundledFile.yaml",
		"BundledFileContent":     "./components/schemas/data-models/BundledFileContent.yaml",
		"InputType":              "./components/schemas/data-models/InputType.yaml",
		"InputKind":              "./components/schemas/data-models/InputKind.yaml",
		"WorkType":               "./components/schemas/data-models/WorkType.yaml",
		"WorkState":              "./components/schemas/data-models/WorkState.yaml",
		"WorkStateType":          "./components/schemas/data-models/WorkStateType.yaml",
		"Resource":               "./components/schemas/data-models/Resource.yaml",
		"ResourceType":           "./components/schemas/data-models/ResourceType.yaml",
		"Worker":                 "./components/schemas/data-models/Worker.yaml",
		"WorkerType":             "./components/schemas/data-models/WorkerType.yaml",
		"WorkerModelProvider":    "./components/schemas/data-models/WorkerModelProvider.yaml",
		"ProviderIdentity":       "./components/schemas/data-models/ProviderIdentity.yaml",
		"WorkerProvider":         "./components/schemas/data-models/WorkerProvider.yaml",
		"Workstation":            "./components/schemas/data-models/Workstation.yaml",
		"WorkstationLimits":      "./components/schemas/data-models/WorkstationLimits.yaml",
		"WorkstationKind":        "./components/schemas/data-models/WorkstationKind.yaml",
		"WorkstationType":        "./components/schemas/data-models/WorkstationType.yaml",
		"WorkstationCron":        "./components/schemas/data-models/WorkstationCron.yaml",
		"GuardType":              "./components/schemas/data-models/GuardType.yaml",
		"Guard":                  "./components/schemas/data-models/Guard.yaml",
		"GuardMatchConfig":       "./components/schemas/data-models/GuardMatchConfig.yaml",
		"WorkstationIO":          "./components/schemas/data-models/WorkstationIO.yaml",
		"Transition":             "./components/schemas/data-models/Transition.yaml",
		"Work":                   "./components/schemas/data-models/Work.yaml",
		"Relation":               "./components/schemas/data-models/Relation.yaml",
		"RelationType":           "./components/schemas/data-models/RelationType.yaml",
	}
	for schemaName, wantRef := range expectedRefs {
		assertSchemaRef(t, schemas, schemaName, wantRef)
	}
}

func TestOpenAPIContract_FactoryExampleUsesGuardedLoopBreaker(t *testing.T) {
	doc := loadValidatedOpenAPIContract(t)
	factorySchema := doc.Components.Schemas["Factory"].Value
	example := loadFactoryGuardedLoopBreakerExampleFixture(t)
	if _, ok := example["exhaustion_rules"]; ok {
		t.Fatalf("factory guarded-loop-breaker example must not advertise exhaustion_rules")
	}
	if err := factorySchema.VisitJSON(example); err != nil {
		t.Fatalf("factory guarded-loop-breaker example should validate: %v", err)
	}

	workstations, ok := example["workstations"].([]any)
	if !ok {
		t.Fatalf("factory guarded-loop-breaker example.workstations must be an array")
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
		t.Fatal("factory guarded-loop-breaker example must include a guarded LOGICAL_MOVE workstation using a VISIT_COUNT guard")
	}
}

func loadFactoryGuardedLoopBreakerExampleFixture(t *testing.T) map[string]any {
	t.Helper()
	payload, err := os.ReadFile("testdata/factory-guarded-loop-breaker-example.json")
	if err != nil {
		t.Fatalf("read factory guarded-loop-breaker example fixture: %v", err)
	}
	var example map[string]any
	if err := json.Unmarshal(payload, &example); err != nil {
		t.Fatalf("decode factory guarded-loop-breaker example fixture: %v", err)
	}
	return example
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
	data, err := os.ReadFile("../../../../api/openapi-main.yaml")
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
	assertPropertyRef(t, dispatchResponseProperties, "failureDetail", "#/components/schemas/FailureDetail")
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
		"missing_executable",
		"command_line_too_long",
	})
}

func assertBundledEventStreamRoute(t *testing.T, doc map[string]any) {
	t.Helper()
	paths, ok := doc["paths"].(map[string]any)
	if !ok {
		t.Fatal("paths object is missing")
	}
	assertEventStreamSchemaRef(t, pathOperation(t, paths, "/factory-sessions/{session_id}/events", "get"), "#/components/schemas/FactoryEvent")
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
			Guards: &[]generated.WorkstationGuard{{
				Type:        generated.WorkstationGuardTypeVISITCOUNT,
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
func TestOpenAPIContract_FactoryExposesOrchestratorSchema(t *testing.T) {
	schemas := loadBundledOpenAPIComponentSchemas(t)
	assertSchemaPropertyRef(t, schemas, "Factory", "orchestrator", "#/components/schemas/FactoryOrchestrator")
	assertSchemaPropertyRef(t, schemas, "FactoryOrchestrator", "kind", "#/components/schemas/FactoryOrchestratorKind")
	assertSchemaPropertyRef(t, schemas, "FactoryOrchestrator", "petri", "#/components/schemas/FactoryOrchestratorPetriConfig")
	assertSchemaPropertyRef(t, schemas, "FactoryOrchestrator", "javascript", "#/components/schemas/FactoryOrchestratorJavaScriptConfig")
	assertEnumValues(t, schemaObject(t, schemas, "FactoryOrchestratorKind"), "FactoryOrchestratorKind", []string{"PETRI", "JAVASCRIPT"})
}

func TestGeneratedFactoryContracts_OrchestratorTypesAgreeWithOpenAPI(t *testing.T) {
	assertGeneratedFieldType(t, reflect.TypeOf(factoryapi.Factory{}), "Orchestrator", reflect.TypeOf((*factoryapi.FactoryOrchestrator)(nil)))
	assertGeneratedFieldType(t, reflect.TypeOf(factoryapi.FactoryOrchestrator{}), "Kind", reflect.TypeOf(factoryapi.FactoryOrchestratorKind("")))
	assertGeneratedFieldType(t, reflect.TypeOf(factoryapi.FactoryOrchestrator{}), "Petri", reflect.TypeOf((*factoryapi.FactoryOrchestratorPetriConfig)(nil)))
	assertGeneratedFieldType(t, reflect.TypeOf(factoryapi.FactoryOrchestrator{}), "Javascript", reflect.TypeOf((*factoryapi.FactoryOrchestratorJavaScriptConfig)(nil)))
}

func TestGeneratedFactoryContracts_JavaScriptOrchestratorRoundTrip(t *testing.T) {
	argsSchema := map[string]any{"type": "object"}
	defaultPolicy := map[string]any{"maxAgents": 2}
	factory := factoryapi.Factory{
		Name: "dynamic-workflow",
		Orchestrator: &factoryapi.FactoryOrchestrator{
			Kind: factoryapi.JAVASCRIPT,
			Javascript: &factoryapi.FactoryOrchestratorJavaScriptConfig{
				SourceRef:     stringPtr("factory/workflows/review.js"),
				Entrypoint:    stringPtr("main"),
				ArgsSchema:    &argsSchema,
				DefaultPolicy: &defaultPolicy,
			},
		},
	}

	encoded, err := json.Marshal(factory)
	if err != nil {
		t.Fatalf("marshal generated JavaScript factory: %v", err)
	}
	var decoded factoryapi.Factory
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal generated JavaScript factory: %v", err)
	}
	if decoded.Orchestrator == nil || decoded.Orchestrator.Kind != factoryapi.JAVASCRIPT {
		t.Fatalf("decoded orchestrator = %#v, want JAVASCRIPT", decoded.Orchestrator)
	}
	if decoded.Orchestrator.Javascript == nil || decoded.Orchestrator.Javascript.SourceRef == nil {
		t.Fatalf("decoded javascript config = %#v", decoded.Orchestrator.Javascript)
	}
}

func TestOpenAPIContract_FactorySessionExposesRuntimeProjectionSchema(t *testing.T) {
	schemas := loadBundledOpenAPIComponentSchemas(t)
	assertSchemaPropertyRef(t, schemas, "FactorySession", "runtime", "#/components/schemas/FactorySessionRuntime")
	assertSchemaPropertyRef(t, schemas, "FactorySessionSummary", "runtime", "#/components/schemas/FactorySessionRuntime")
	assertSchemaPropertyRef(t, schemas, "FactorySessionRuntime", "orchestratorKind", "#/components/schemas/FactoryOrchestratorKind")
	assertSchemaPropertyRef(t, schemas, "FactorySessionRuntime", "status", "#/components/schemas/FactorySessionStatus")
	assertSchemaPropertyRef(t, schemas, "FactorySessionRuntime", "streamIdentity", "#/components/schemas/FactorySessionStreamIdentity")
	assertSchemaPropertyRef(t, schemas, "FactorySessionRuntime", "petri", "#/components/schemas/FactorySessionPetriProjection")
	assertSchemaPropertyRef(t, schemas, "FactorySessionRuntime", "javascript", "#/components/schemas/FactorySessionJavaScriptProjection")
	assertEnumValues(t, schemaObject(t, schemas, "FactorySessionStatus"), "FactorySessionStatus", []string{"ACTIVE", "IDLE", "FINISHED"})
}

func TestOpenAPIContract_SessionEventStreamHandshakeExposesIdentityHeaders(t *testing.T) {
	doc := loadBundledOpenAPIDocument(t)
	paths, ok := doc["paths"].(map[string]any)
	if !ok {
		t.Fatal("bundled OpenAPI paths are missing")
	}
	operation := pathOperation(t, paths, "/factory-sessions/{session_id}/events", "get")
	assertResponseHeaderString(
		t,
		operation,
		"200",
		"X-Factory-Session-Backend-Scope-Id",
	)
	assertResponseHeaderString(
		t,
		operation,
		"200",
		"X-Factory-Session-Logical-Session-Key-Id",
	)
	assertResponseHeaderString(
		t,
		operation,
		"200",
		"X-Factory-Session-Factory-Session-Id",
	)
	assertResponseHeaderString(
		t,
		operation,
		"200",
		"X-Factory-Session-Stream-Generation-Id",
	)
}

func TestGeneratedFactorySessionContracts_RuntimeTypesAgreeWithOpenAPI(t *testing.T) {
	assertGeneratedFieldType(t, reflect.TypeOf(factoryapi.FactorySession{}), "Runtime", reflect.TypeOf(factoryapi.FactorySessionRuntime{}))
	assertGeneratedFieldType(t, reflect.TypeOf(factoryapi.FactorySessionSummary{}), "Runtime", reflect.TypeOf((*factoryapi.FactorySessionRuntime)(nil)))
	assertGeneratedFieldType(t, reflect.TypeOf(factoryapi.FactorySessionRuntime{}), "OrchestratorKind", reflect.TypeOf(factoryapi.FactoryOrchestratorKind("")))
	assertGeneratedFieldType(t, reflect.TypeOf(factoryapi.FactorySessionRuntime{}), "StreamIdentity", reflect.TypeOf((*factoryapi.FactorySessionStreamIdentity)(nil)))
	assertGeneratedFieldType(t, reflect.TypeOf(factoryapi.FactorySessionRuntime{}), "Petri", reflect.TypeOf((*factoryapi.FactorySessionPetriProjection)(nil)))
	assertGeneratedFieldType(t, reflect.TypeOf(factoryapi.FactorySessionRuntime{}), "Javascript", reflect.TypeOf((*factoryapi.FactorySessionJavaScriptProjection)(nil)))
}

func TestGeneratedFactorySessionContracts_JavaScriptRuntimeRoundTrip(t *testing.T) {
	phase := "review"
	argsDigest := "sha256:args"
	checkpoints := []factoryapi.FactorySessionJavaScriptCheckpointRef{{Id: "ckpt-1"}}
	session := factoryapi.FactorySession{
		Id: "session-js",
		Target: factoryapi.FactorySessionTargetRef{
			Kind: factoryapi.FactorySessionTargetRefKindDefault,
		},
		FactoryDir: "/factories/js",
		FolderPath: "/workspace",
		Project:    "dynamic-workflow",
		Runtime: factoryapi.FactorySessionRuntime{
			OrchestratorKind: factoryapi.JAVASCRIPT,
			Status:           factoryapi.FactorySessionStatusIDLE,
			Progress: factoryapi.FactorySessionProgress{
				FactoryState:  "UNKNOWN",
				Categories:    factoryapi.StatusCategories{},
				InFlightCount: 0,
				TotalTokens:   0,
			},
			Usage: factoryapi.FactorySessionUsage{Resources: []factoryapi.ResourceUsage{}},
			Lifecycle: factoryapi.FactorySessionLifecycle{
				StartedAt: time.Date(2026, 6, 8, 14, 0, 0, 0, time.UTC),
				UpdatedAt: time.Date(2026, 6, 8, 14, 0, 0, 0, time.UTC),
			},
			Javascript: &factoryapi.FactorySessionJavaScriptProjection{
				Phase:        &phase,
				Phases:       []string{"plan", "review"},
				ArgsDigest:   &argsDigest,
				Checkpoints:  &checkpoints,
				ScriptStatus: factoryapi.FactorySessionJavaScriptScriptStatusRUNNING,
				ChildDispatchCounts: factoryapi.FactorySessionJavaScriptChildDispatchCounts{
					Queued: 1, Running: 0, Completed: 2,
				},
			},
		},
	}
	encoded, err := json.Marshal(session)
	if err != nil {
		t.Fatalf("marshal generated JavaScript session: %v", err)
	}
	var decoded factoryapi.FactorySession
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal generated JavaScript session: %v", err)
	}
	if decoded.Runtime.Javascript == nil || decoded.Runtime.OrchestratorKind != factoryapi.JAVASCRIPT {
		t.Fatalf("decoded session runtime = %#v", decoded.Runtime)
	}
}

func TestOpenAPIContract_CheckpointSchemasExposeArtifactMetadataOnly(t *testing.T) {
	schemas := loadBundledOpenAPIComponentSchemas(t)
	assertSchemaPropertyRef(t, schemas, "FactorySessionJavaScriptCheckpointRef", "artifactRef", "#/components/schemas/FactoryArtifactRef")
	assertSchemaPropertyRef(t, schemas, "FactorySessionLiveResult", "resultArtifactRef", "#/components/schemas/FactoryArtifactRef")
	assertSchemaPropertyRef(t, schemas, "FactorySessionPartialResult", "partialResultArtifactRef", "#/components/schemas/FactoryArtifactRef")
	assertSchemaPropertyRef(t, schemas, "JavaScriptCheckpointRefEventPayload", "artifactRef", "#/components/schemas/FactoryArtifactRef")
	assertEnumValues(t, schemaObject(t, schemas, "FactoryArtifactVisibility"), "FactoryArtifactVisibility", []string{"PUBLIC", "INTERNAL_CHECKPOINT"})
	assertEnumValues(t, schemaObject(t, schemas, "FactoryArtifactKind"), "FactoryArtifactKind", []string{
		"FINAL_RESULT",
		"CHILD_RESULT",
		"FINDING",
		"PATCH",
		"LOG",
		"DATASET",
		"CHECKPOINT",
		"WORKTREE_SUMMARY",
	})
}

func TestGeneratedCheckpointContracts_ArtifactTypesAgreeWithOpenAPI(t *testing.T) {
	assertGeneratedFieldType(t, reflect.TypeOf(factoryapi.FactorySessionJavaScriptCheckpointRef{}), "ArtifactRef", reflect.TypeOf((*factoryapi.FactoryArtifactRef)(nil)))
	assertGeneratedFieldType(t, reflect.TypeOf(factoryapi.FactorySessionLiveResult{}), "ResultArtifactRef", reflect.TypeOf((*factoryapi.FactoryArtifactRef)(nil)))
	assertGeneratedFieldType(t, reflect.TypeOf(factoryapi.FactorySessionPartialResult{}), "PartialResultArtifactRef", reflect.TypeOf((*factoryapi.FactoryArtifactRef)(nil)))
	assertGeneratedFieldType(t, reflect.TypeOf(factoryapi.JavaScriptCheckpointRefEventPayload{}), "ArtifactRef", reflect.TypeOf(factoryapi.FactoryArtifactRef{}))
}

func TestOpenAPIContract_FactoryDispatchAndArtifactSchemasExposeSharedProjectionFields(t *testing.T) {
	schemas := loadBundledOpenAPIComponentSchemas(t)
	runtimeSchema := schemaObject(t, schemas, "FactorySessionRuntime")
	runtimeProperties, _ := runtimeSchema["properties"].(map[string]any)
	assertSchemaPropertyRef(t, schemas, "FactorySessionRuntime", "streamIdentity", "#/components/schemas/FactorySessionStreamIdentity")
	streamIdentityProperties := schemaProperties(t, schemaObject(t, schemas, "FactorySessionStreamIdentity"), "FactorySessionStreamIdentity")
	assertRequiredFields(t, schemaObject(t, schemas, "FactorySessionStreamIdentity"), "backendScopeID", "logicalSessionKeyID", "factorySessionID", "streamGenerationID")
	streamGenerationID, ok := streamIdentityProperties["streamGenerationID"].(map[string]any)
	if !ok {
		t.Fatal("FactorySessionStreamIdentity.streamGenerationID schema is missing")
	}
	if got, ok := streamGenerationID["type"].(string); !ok || got != "string" {
		t.Fatalf("FactorySessionStreamIdentity.streamGenerationID.type = %v, want string", streamGenerationID["type"])
	}
	if _, ok := runtimeProperties["dispatches"]; ok {
		t.Fatal("FactorySessionRuntime.dispatches is present, want dispatch-free session reads")
	}
	assertArrayItemRef(t, runtimeProperties, "artifacts", "#/components/schemas/FactoryArtifact")
	assertSchemaPropertyRef(t, schemas, "FactoryDispatch", "orchestratorKind", "#/components/schemas/FactoryOrchestratorKind")
	assertSchemaPropertyRef(t, schemas, "FactoryDispatch", "dispatchKind", "#/components/schemas/FactoryDispatchKind")
	assertSchemaPropertyRef(t, schemas, "FactoryDispatch", "status", "#/components/schemas/FactoryDispatchStatus")
	assertSchemaPropertyRef(t, schemas, "FactoryDispatch", "petri", "#/components/schemas/FactoryDispatchPetriProjection")
	assertSchemaPropertyRef(t, schemas, "FactoryDispatch", "javascript", "#/components/schemas/FactoryDispatchJavaScriptProjection")
	assertSchemaPropertyRef(t, schemas, "FactoryArtifact", "kind", "#/components/schemas/FactoryArtifactKind")
	assertSchemaPropertyRef(t, schemas, "FactoryArtifact", "visibility", "#/components/schemas/FactoryArtifactVisibility")
	assertSchemaPropertyRef(t, schemas, "FactoryArtifact", "auditMode", "#/components/schemas/FactoryArtifactAuditMode")
	assertEnumValues(t, schemaObject(t, schemas, "FactoryDispatchKind"), "FactoryDispatchKind", []string{
		"PETRI_TRANSITION",
		"JAVASCRIPT_AGENT",
		"JAVASCRIPT_VERIFY",
		"JAVASCRIPT_SYNTHESIZE",
		"JAVASCRIPT_TOOL",
		"JAVASCRIPT_SCRIPT",
		"JAVASCRIPT_SYSTEM",
	})
	assertEnumValues(t, schemaObject(t, schemas, "FactoryDispatchStatus"), "FactoryDispatchStatus", []string{
		"QUEUED", "RUNNING", "COMPLETED", "FAILED", "INTERRUPTED",
	})
	assertEnumValues(t, schemaObject(t, schemas, "FactoryArtifactKind"), "FactoryArtifactKind", []string{
		"FINAL_RESULT",
		"CHILD_RESULT",
		"FINDING",
		"PATCH",
		"LOG",
		"DATASET",
		"CHECKPOINT",
		"WORKTREE_SUMMARY",
	})
}

func TestGeneratedDispatchArtifactContracts_RuntimeTypesAgreeWithOpenAPI(t *testing.T) {
	if _, ok := reflect.TypeOf(factoryapi.FactorySessionRuntime{}).FieldByName("Dispatches"); ok {
		t.Fatal("generated FactorySessionRuntime.Dispatches is present, want dispatch-free session reads")
	}
	assertGeneratedFieldType(t, reflect.TypeOf(factoryapi.FactorySessionRuntime{}), "Artifacts", reflect.TypeOf((*[]factoryapi.FactoryArtifact)(nil)))
	assertGeneratedFieldType(t, reflect.TypeOf(factoryapi.FactoryDispatch{}), "Petri", reflect.TypeOf((*factoryapi.FactoryDispatchPetriProjection)(nil)))
	assertGeneratedFieldType(t, reflect.TypeOf(factoryapi.FactoryDispatch{}), "Javascript", reflect.TypeOf((*factoryapi.FactoryDispatchJavaScriptProjection)(nil)))
	assertGeneratedFieldType(t, reflect.TypeOf(factoryapi.FactoryArtifact{}), "AuditMode", reflect.TypeOf((*factoryapi.FactoryArtifactAuditMode)(nil)))
}

func TestOpenAPIAuthoring_FactorySessionRuntimeOmitsDispatches(t *testing.T) {
	data, err := os.ReadFile("../../../../api/components/schemas/api/FactorySessionRuntime.yaml")
	if err != nil {
		t.Fatalf("read authored FactorySessionRuntime schema: %v", err)
	}
	var schema map[string]any
	if err := yaml.Unmarshal(data, &schema); err != nil {
		t.Fatalf("parse authored FactorySessionRuntime schema: %v", err)
	}

	properties := schemaProperties(t, schema, "FactorySessionRuntime")
	if _, ok := properties["dispatches"]; ok {
		t.Fatal("authored FactorySessionRuntime.dispatches is present, want dispatch-free session reads")
	}
}

func TestGeneratedFactorySessionRuntime_OmitsDispatchesField(t *testing.T) {
	runtimeType := reflect.TypeOf(factoryapi.FactorySessionRuntime{})
	if _, ok := runtimeType.FieldByName("Dispatches"); ok {
		t.Fatal("generated FactorySessionRuntime.Dispatches is present, want dispatch-free session reads")
	}
}

func TestGeneratedDispatchArtifactContracts_SessionReadListDetailTypesAgreeWithOpenAPI(t *testing.T) {
	assertGeneratedFieldType(t, reflect.TypeOf(factoryapi.ListFactorySessionDispatchesResponse{}), "Dispatches", reflect.TypeOf([]factoryapi.FactorySessionDispatchSummary{}))
	assertGeneratedFieldType(t, reflect.TypeOf(factoryapi.ListFactorySessionArtifactsResponse{}), "Artifacts", reflect.TypeOf([]factoryapi.FactorySessionArtifactSummary{}))
	assertGeneratedFieldType(t, reflect.TypeOf(factoryapi.FactorySessionDispatchSummary{}), "Status", reflect.TypeOf(factoryapi.FactoryDispatchStatus("")))
	assertGeneratedFieldType(t, reflect.TypeOf(factoryapi.FactorySessionDispatchSummary{}), "DispatchKind", reflect.TypeOf(factoryapi.FactoryDispatchKind("")))
	assertGeneratedFieldType(t, reflect.TypeOf(factoryapi.FactorySessionDispatchSummary{}), "Javascript", reflect.TypeOf((*factoryapi.FactoryDispatchJavaScriptProjection)(nil)))
	assertGeneratedFieldType(t, reflect.TypeOf(factoryapi.FactorySessionArtifactSummary{}), "Kind", reflect.TypeOf(factoryapi.FactoryArtifactKind("")))
	assertGeneratedFieldType(t, reflect.TypeOf(factoryapi.FactorySessionArtifactSummary{}), "Visibility", reflect.TypeOf(factoryapi.FactoryArtifactVisibility("")))
	assertGeneratedFieldType(t, reflect.TypeOf(factoryapi.FactorySessionArtifactDetail{}), "ContentRef", reflect.TypeOf((*factoryapi.FactorySessionArtifactRetrievalRef)(nil)))
	assertGeneratedFieldType(t, reflect.TypeOf(factoryapi.FactorySessionArtifactDetail{}), "Content", reflect.TypeOf((*factoryapi.WorkContent)(nil)))
}

func TestGeneratedDispatchArtifactContracts_PetriAndJavaScriptRoundTrip(t *testing.T) {
	label := "process"
	phase := "review"
	dispatches := []factoryapi.FactoryDispatch{{
		Id:               "dispatch-petri-1",
		SessionId:        "~default",
		OrchestratorKind: factoryapi.PETRI,
		DispatchKind:     factoryapi.FactoryDispatchKindPETRITRANSITION,
		Status:           factoryapi.FactoryDispatchStatusRUNNING,
		Label:            &label,
		Petri: &factoryapi.FactoryDispatchPetriProjection{
			TransitionId: "tr-process",
		},
	}, {
		Id:               "dispatch-agent-1",
		SessionId:        "session-js",
		OrchestratorKind: factoryapi.JAVASCRIPT,
		DispatchKind:     factoryapi.FactoryDispatchKindJAVASCRIPTAGENT,
		Status:           factoryapi.FactoryDispatchStatusCOMPLETED,
		Phase:            &phase,
		Javascript: &factoryapi.FactoryDispatchJavaScriptProjection{
			TaskKind: factoryapi.FactoryDispatchJavaScriptTaskKindAGENT,
		},
	}}
	auditMode := factoryapi.FactoryArtifactAuditModeREDACTED
	artifacts := []factoryapi.FactoryArtifact{{
		Id:         "artifact-child-1",
		Kind:       factoryapi.FactoryArtifactKindCHILDRESULT,
		Visibility: factoryapi.FactoryArtifactVisibilityPUBLIC,
		AuditMode:  &auditMode,
	}}
	runtime := factoryapi.FactorySessionRuntime{
		OrchestratorKind: factoryapi.JAVASCRIPT,
		Status:           factoryapi.FactorySessionStatusIDLE,
		Progress: factoryapi.FactorySessionProgress{
			FactoryState:  "UNKNOWN",
			Categories:    factoryapi.StatusCategories{},
			InFlightCount: 0,
			TotalTokens:   0,
		},
		Usage:     factoryapi.FactorySessionUsage{Resources: []factoryapi.ResourceUsage{}},
		Lifecycle: factoryapi.FactorySessionLifecycle{StartedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()},
		Artifacts: &artifacts,
	}
	encoded, err := json.Marshal(runtime)
	if err != nil {
		t.Fatalf("marshal generated runtime: %v", err)
	}
	var decoded factoryapi.FactorySessionRuntime
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal generated runtime: %v", err)
	}
	if decoded.Artifacts == nil || len(*decoded.Artifacts) != 1 {
		t.Fatalf("decoded artifacts = %#v, want one entry", decoded.Artifacts)
	}
	dispatchPayload, err := json.Marshal(dispatches)
	if err != nil {
		t.Fatalf("marshal generated dispatches: %v", err)
	}
	var decodedDispatches []factoryapi.FactoryDispatch
	if err := json.Unmarshal(dispatchPayload, &decodedDispatches); err != nil {
		t.Fatalf("unmarshal generated dispatches: %v", err)
	}
	if len(decodedDispatches) != 2 {
		t.Fatalf("decoded dispatches = %#v, want two entries", decodedDispatches)
	}
}
