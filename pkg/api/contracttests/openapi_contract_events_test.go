package apicontract_test

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/portpowered/infinite-you/pkg/api/generated"
)

func TestOpenAPIContract_DefinesUnifiedFactoryEventLog(t *testing.T) {
	doc := loadBundledOpenAPIDocument(t)
	schemas := componentSchemas(t, doc)

	assertUnifiedEventSchemasPresent(t, schemas)
	assertUnifiedEventLegacySchemasAbsent(t, schemas)
	assertUnifiedEventEnvelope(t, schemas)
	assertUnifiedEventContext(t, schemas)
	assertUnifiedRunRequestEvent(t, schemas)
	assertUnifiedFactorySchema(t, schemas)
	assertUnifiedWorkRequestEvent(t, schemas)
	assertUnifiedDispatchEvents(t, schemas)
	assertUnifiedModelEvents(t, schemas)
	assertUnifiedInferenceEvents(t, schemas)
	assertUnifiedScriptEvents(t, schemas)
	assertUnifiedStateEvent(t, schemas)
}

func TestOpenAPIContract_CanonicalFactoryEventVocabularyFixtureValidatesAndRetiresLegacyNames(t *testing.T) {
	doc := loadValidatedOpenAPIContract(t)
	fixture := loadCanonicalFactoryEventVocabularyFixture(t)
	seenTypes := make([]string, 0, len(fixture))
	for i, event := range fixture {
		seenTypes = append(seenTypes, assertCanonicalFactoryEventFixtureEntry(t, doc, i, event))
	}
	assertStringSetsMatch(t, seenTypes, canonicalFactoryEventTypeValues)
}

func TestOpenAPIContract_GeneratedModelsOmitLegacyConfig(t *testing.T) {
	data, err := os.ReadFile("../generated/server.gen.go")
	if err != nil {
		t.Fatalf("read generated server models: %v", err)
	}
	legacyGeneratedConfigType := "type " + strings.Join([]string{"Effective", "Config"}, "") + " struct"
	if strings.Contains(string(data), legacyGeneratedConfigType) {
		t.Fatal("generated OpenAPI models must not contain legacy config structs")
	}
}

func TestOpenAPIContract_RunRequestPayloadValidatesFactoryConfig(t *testing.T) {
	doc := loadValidatedOpenAPIContract(t)
	schema := doc.Components.Schemas["RunRequestEventPayload"].Value

	validPayload := validRunRequestPayloadFixture()
	if err := schema.VisitJSON(validPayload); err != nil {
		t.Fatalf("factory run-request payload should validate: %v", err)
	}

	legacyConfigOnlyPayload := map[string]any{
		"recordedAt": "2026-04-10T12:00:00Z",
		strings.Join([]string{"effective", "Config"}, ""): map[string]any{
			"factory": map[string]any{},
		},
	}
	if err := schema.VisitJSON(legacyConfigOnlyPayload); err == nil {
		t.Fatal("legacy-config-only run-request payload should not validate")
	}
}

func assertUnifiedEventSchemasPresent(t *testing.T, schemas map[string]any) {
	t.Helper()
	for _, schema := range []string{
		"FactoryEvent", "FactoryEventContext", "FactoryEventType", "DispatchConsumedWorkRef", "DispatchRequestEventMetadata",
		"RunRequestEventPayload", "InitialStructureRequestEventPayload", "FactoryChangeEventPayload", "WorkRequestEventPayload",
		"RelationshipChangeRequestEventPayload", "DispatchRequestEventPayload", "ModelRequestEventPayload", "ModelResponseEventPayload", "InferenceRequestEventPayload", "InferenceResponseEventPayload",
		"ScriptRequestEventPayload", "ScriptResponseEventPayload", "InferenceOutcome", "ScriptExecutionOutcome", "ScriptFailureType",
		"DispatchResponseEventPayload", "FactoryStateResponseEventPayload", "RunResponseEventPayload",
	} {
		if _, ok := schemas[schema]; !ok {
			t.Fatalf("components.schemas.%s is missing", schema)
		}
	}
}

func assertUnifiedEventLegacySchemasAbsent(t *testing.T, schemas map[string]any) {
	t.Helper()
	legacyGeneratedConfigSchema := strings.Join([]string{"Effective", "Config"}, "")
	for _, legacySchema := range []string{
		"FactoryWorkItem", "FactoryRelation", "RecordedWorkRequest", "RecordedSubmission", "RecordedDispatch", "RecordedCompletion", legacyGeneratedConfigSchema,
	} {
		if _, ok := schemas[legacySchema]; ok {
			t.Fatalf("components.schemas.%s must not be introduced beside generated FactoryEvent", legacySchema)
		}
	}
}

func assertUnifiedEventEnvelope(t *testing.T, schemas map[string]any) {
	t.Helper()
	factoryEvent := schemaObject(t, schemas, "FactoryEvent")
	assertRequiredFields(t, factoryEvent, "schemaVersion", "id", "type", "context", "payload")
	factoryEventProperties := schemaProperties(t, factoryEvent, "FactoryEvent")
	assertPropertyRef(t, factoryEventProperties, "type", "#/components/schemas/FactoryEventType")
	assertPropertyRef(t, factoryEventProperties, "context", "#/components/schemas/FactoryEventContext")
	assertPayloadUnionRefs(t, factoryEventProperties, bundledFactoryEventPayloadRefs)

	eventType := schemaObject(t, schemas, "FactoryEventType")
	assertEnumValues(t, eventType, "FactoryEventType", canonicalFactoryEventTypeValues)
	assertEnumOmitValues(t, eventType, "FactoryEventType", retiredFactoryEventTypeValues)
}

func assertUnifiedEventContext(t *testing.T, schemas map[string]any) {
	t.Helper()
	context := schemaObject(t, schemas, "FactoryEventContext")
	assertRequiredFields(t, context, "sequence", "tick", "eventTime")
	contextProperties := schemaProperties(t, context, "FactoryEventContext")
	assertSchemaPropertiesPresent(t, contextProperties, "FactoryEventContext", "eventTime", "requestId", "traceIds", "workIds", "dispatchId", "currentChainingTraceId")
	assertStringArrayProperty(t, contextProperties, "previousChainingTraceIds")
	assertPropertiesAbsent(t, contextProperties, "FactoryEventContext", "event_time", "request_id", "trace_ids", "work_ids", "dispatch_id")
}

func assertUnifiedRunRequestEvent(t *testing.T, schemas map[string]any) {
	t.Helper()
	initialStructure := schemaObject(t, schemas, "InitialStructureRequestEventPayload")
	initialStructureProperties := schemaProperties(t, initialStructure, "InitialStructureRequestEventPayload")
	assertPropertyRef(t, initialStructureProperties, "factory", "#/components/schemas/Factory")
	assertPropertiesAbsent(t, initialStructureProperties, "InitialStructureRequestEventPayload", "workflowId")
	if _, ok := reflect.TypeOf(generated.InitialStructureRequestEventPayload{}).FieldByName("WorkflowId"); ok {
		t.Fatal("generated.InitialStructureRequestEventPayload must not expose WorkflowId")
	}

	runRequest := schemaObject(t, schemas, "RunRequestEventPayload")
	assertRequiredFields(t, runRequest, "recordedAt", "factory")
	runRequestProperties := schemaProperties(t, runRequest, "RunRequestEventPayload")
	assertPropertyRef(t, runRequestProperties, "factory", "#/components/schemas/Factory")
	if _, ok := runRequestProperties[strings.Join([]string{"effective", "Config"}, "")]; ok {
		t.Fatalf("RunRequestEventPayload.properties must not expose legacy config")
	}
}

func assertUnifiedFactorySchema(t *testing.T, schemas map[string]any) {
	t.Helper()
	factory := schemaObject(t, schemas, "Factory")
	factoryProperties := schemaProperties(t, factory, "Factory")
	assertSchemaPropertiesPresent(t, factoryProperties, "Factory", "factoryDirectory", "sourceDirectory", "metadata", "inputTypes", "workTypes")
	assertPropertiesAbsent(t, factoryProperties, "Factory", "factory_dir", "source_directory", "workflow_id", "input_types", "work_types", "exhaustion_rules", "exhaustionRules")
	assertArrayItemRef(t, factoryProperties, "workers", "#/components/schemas/Worker")
	assertArrayItemRef(t, factoryProperties, "workstations", "#/components/schemas/Workstation")
}

func assertUnifiedWorkRequestEvent(t *testing.T, schemas map[string]any) {
	t.Helper()
	workRequest := schemaObject(t, schemas, "WorkRequestEventPayload")
	workRequestProperties := schemaProperties(t, workRequest, "WorkRequestEventPayload")
	assertPropertyRef(t, workRequestProperties, "type", "#/components/schemas/WorkRequestType")
	assertArrayItemRef(t, workRequestProperties, "works", "#/components/schemas/Work")
	assertArrayItemRef(t, workRequestProperties, "relations", "#/components/schemas/Relation")
	assertSchemaPropertiesPresent(t, workRequestProperties, "WorkRequestEventPayload", "source", "parentLineage")
	assertPropertiesAbsent(t, workRequestProperties, "WorkRequestEventPayload", "request", "requestId", "traceIds", "workIds", "dispatchId")

	relationshipChange := schemaObject(t, schemas, "RelationshipChangeRequestEventPayload")
	assertPropertyRef(t, schemaProperties(t, relationshipChange, "RelationshipChangeRequestEventPayload"), "relation", "#/components/schemas/Relation")
}

func assertUnifiedDispatchEvents(t *testing.T, schemas map[string]any) {
	t.Helper()
	dispatchRequest := schemaObject(t, schemas, "DispatchRequestEventPayload")
	dispatchRequestProperties := schemaProperties(t, dispatchRequest, "DispatchRequestEventPayload")
	assertDeprecatedEventFields(t, dispatchRequestProperties, "DispatchRequestEventPayload", "currentChainingTraceId", "previousChainingTraceIds")
	assertArrayItemRef(t, dispatchRequestProperties, "inputs", "#/components/schemas/DispatchConsumedWorkRef")
	assertArrayItemRef(t, dispatchRequestProperties, "resources", "#/components/schemas/Resource")
	assertPropertyRef(t, dispatchRequestProperties, "metadata", "#/components/schemas/DispatchRequestEventMetadata")
	assertPropertiesAbsent(t, dispatchRequestProperties, "DispatchRequestEventPayload", "dispatchId", "workstation", "worker")
	assertNoDispatchConfigCopies(t, dispatchRequestProperties, "DispatchRequestEventPayload")

	dispatchInput := schemaObject(t, schemas, "DispatchConsumedWorkRef")
	assertRequiredFields(t, dispatchInput, "workId")
	assertPropertiesAbsent(t, schemaProperties(t, dispatchInput, "DispatchConsumedWorkRef"), "DispatchConsumedWorkRef", "traceId", "workTypeName", "name", "requestId", "state", "tags")
	dispatchMetadata := schemaObject(t, schemas, "DispatchRequestEventMetadata")
	assertPropertiesAbsent(t, schemaProperties(t, dispatchMetadata, "DispatchRequestEventMetadata"), "DispatchRequestEventMetadata", "requestId", "dispatchId", "traceIds", "workIds")

	dispatchResponse := schemaObject(t, schemas, "DispatchResponseEventPayload")
	dispatchResponseProperties := schemaProperties(t, dispatchResponse, "DispatchResponseEventPayload")
	assertDeprecatedEventFields(t, dispatchResponseProperties, "DispatchResponseEventPayload", "currentChainingTraceId", "previousChainingTraceIds")
	assertArrayItemRef(t, dispatchResponseProperties, "outputWork", "#/components/schemas/Work")
	assertArrayItemRef(t, dispatchResponseProperties, "outputResources", "#/components/schemas/Resource")
	assertPropertiesAbsent(t, dispatchResponseProperties, "DispatchResponseEventPayload", "dispatchId", "workstation", "worker", "inputs", "providerSession", "diagnostics")
	assertNoDispatchConfigCopies(t, dispatchResponseProperties, "DispatchResponseEventPayload")

	workMetricsProperties := schemaProperties(t, schemaObject(t, schemas, "WorkMetrics"), "WorkMetrics")
	assertInt64Property(t, workMetricsProperties, "durationMillis")
	assertPropertiesAbsent(t, workMetricsProperties, "WorkMetrics", "durationNanos")
}

func assertUnifiedInferenceEvents(t *testing.T, schemas map[string]any) {
	t.Helper()
	inferenceRequest := schemaObject(t, schemas, "InferenceRequestEventPayload")
	assertRequiredFields(t, inferenceRequest, "inferenceRequestId", "attempt", "workingDirectory", "worktree", "prompt")
	inferenceRequestProperties := schemaProperties(t, inferenceRequest, "InferenceRequestEventPayload")
	assertSchemaPropertiesPresent(t, inferenceRequestProperties, "InferenceRequestEventPayload", "inferenceRequestId", "attempt", "workingDirectory", "worktree", "prompt")
	assertPropertiesAbsent(t, inferenceRequestProperties, "InferenceRequestEventPayload", "dispatchId", "transitionId")

	inferenceResponse := schemaObject(t, schemas, "InferenceResponseEventPayload")
	assertRequiredFields(t, inferenceResponse, "inferenceRequestId", "attempt", "outcome", "durationMillis")
	inferenceResponseProperties := schemaProperties(t, inferenceResponse, "InferenceResponseEventPayload")
	assertPropertyRef(t, inferenceResponseProperties, "outcome", "#/components/schemas/InferenceOutcome")
	assertSchemaPropertiesPresent(t, inferenceResponseProperties, "InferenceResponseEventPayload", "inferenceRequestId", "attempt", "response", "durationMillis", "providerSession", "diagnostics", "exitCode", "errorClass")
	assertPropertiesAbsent(t, inferenceResponseProperties, "InferenceResponseEventPayload", "dispatchId", "transitionId")
	assertEnumValues(t, schemaObject(t, schemas, "InferenceOutcome"), "InferenceOutcome", []string{"SUCCEEDED", "FAILED"})
}

func assertUnifiedModelEvents(t *testing.T, schemas map[string]any) {
	t.Helper()
	modelRequest := schemaObject(t, schemas, "ModelRequestEventPayload")
	assertRequiredFields(t, modelRequest, "modelRequestId", "attempt", "operation", "worker", "model", "providerLocality")
	modelRequestProperties := schemaProperties(t, modelRequest, "ModelRequestEventPayload")
	assertSchemaPropertiesPresent(t, modelRequestProperties, "ModelRequestEventPayload", "modelRequestId", "attempt", "operation", "worker", "model", "providerLocality", "resources", "bindings", "workingDirectory", "worktree")
	assertArrayItemRef(t, modelRequestProperties, "resources", "#/components/schemas/ModelResourceSummary")
	assertArrayItemRef(t, modelRequestProperties, "bindings", "#/components/schemas/ResolvedModelOperationBinding")
	assertPropertiesAbsent(t, modelRequestProperties, "ModelRequestEventPayload", "dispatchId", "transitionId")

	modelResponse := schemaObject(t, schemas, "ModelResponseEventPayload")
	assertRequiredFields(t, modelResponse, "modelRequestId", "attempt", "operation", "worker", "model", "providerLocality", "outcome", "durationMillis")
	modelResponseProperties := schemaProperties(t, modelResponse, "ModelResponseEventPayload")
	assertSchemaPropertiesPresent(t, modelResponseProperties, "ModelResponseEventPayload", "modelRequestId", "attempt", "operation", "worker", "model", "providerLocality", "durationMillis", "resources", "bindings", "resourceWaitMillis", "resourceAcquired", "loadRequested", "loadReused", "loadDurationMillis", "outputPreview", "outputContent", "diagnostics", "errorClass")
	assertArrayItemRef(t, modelResponseProperties, "resources", "#/components/schemas/ModelResourceSummary")
	assertArrayItemRef(t, modelResponseProperties, "bindings", "#/components/schemas/ResolvedModelOperationBinding")
	assertPropertyRef(t, modelResponseProperties, "outcome", "#/components/schemas/InferenceOutcome")
	assertPropertyRef(t, modelResponseProperties, "outputContent", "#/components/schemas/WorkContent")
	assertPropertyRef(t, modelResponseProperties, "diagnostics", "#/components/schemas/SafeWorkDiagnostics")
	assertPropertiesAbsent(t, modelResponseProperties, "ModelResponseEventPayload", "dispatchId", "transitionId")
}

func assertUnifiedScriptEvents(t *testing.T, schemas map[string]any) {
	t.Helper()
	scriptRequest := schemaObject(t, schemas, "ScriptRequestEventPayload")
	assertRequiredFields(t, scriptRequest, "scriptRequestId", "dispatchId", "transitionId", "attempt", "command", "args")
	scriptRequestProperties := schemaProperties(t, scriptRequest, "ScriptRequestEventPayload")
	assertSchemaPropertiesPresent(t, scriptRequestProperties, "ScriptRequestEventPayload", "scriptRequestId", "dispatchId", "transitionId", "attempt", "command", "args")
	assertPropertiesAbsent(t, scriptRequestProperties, "ScriptRequestEventPayload", "stdin", "env")
	assertEventDescriptionMentionsHiddenFields(t, scriptRequest["description"], "ScriptRequestEventPayload")

	scriptResponse := schemaObject(t, schemas, "ScriptResponseEventPayload")
	assertRequiredFields(t, scriptResponse, "scriptRequestId", "dispatchId", "transitionId", "attempt", "outcome", "stdout", "stderr", "durationMillis")
	scriptResponseProperties := schemaProperties(t, scriptResponse, "ScriptResponseEventPayload")
	assertPropertyRef(t, scriptResponseProperties, "outcome", "#/components/schemas/ScriptExecutionOutcome")
	assertPropertyRef(t, scriptResponseProperties, "failureType", "#/components/schemas/ScriptFailureType")
	assertSchemaPropertiesPresent(t, scriptResponseProperties, "ScriptResponseEventPayload", "scriptRequestId", "dispatchId", "transitionId", "attempt", "stdout", "stderr", "durationMillis", "exitCode")
	assertPropertiesAbsent(t, scriptResponseProperties, "ScriptResponseEventPayload", "stdin", "env")
	assertEventDescriptionMentionsHiddenFields(t, scriptResponse["description"], "ScriptResponseEventPayload")
	assertEnumValues(t, schemaObject(t, schemas, "ScriptExecutionOutcome"), "ScriptExecutionOutcome", []string{"SUCCEEDED", "FAILED_EXIT_CODE", "TIMED_OUT", "PROCESS_ERROR"})
	assertEnumValues(t, schemaObject(t, schemas, "ScriptFailureType"), "ScriptFailureType", []string{"TIMEOUT", "PROCESS_ERROR"})
}

func assertUnifiedStateEvent(t *testing.T, schemas map[string]any) {
	t.Helper()
	stateResponse := schemaObject(t, schemas, "FactoryStateResponseEventPayload")
	assertPropertyRef(t, schemaProperties(t, stateResponse, "FactoryStateResponseEventPayload"), "state", "#/components/schemas/FactoryState")
}

func assertDeprecatedEventFields(t *testing.T, properties map[string]any, schemaName string, fields ...string) {
	t.Helper()
	for _, field := range fields {
		property, ok := properties[field].(map[string]any)
		if !ok {
			t.Fatalf("%s.properties.%s is missing", schemaName, field)
		}
		if got, ok := property["deprecated"].(bool); !ok || !got {
			t.Fatalf("%s.properties.%s must be marked deprecated", schemaName, field)
		}
	}
}

func assertEventDescriptionMentionsHiddenFields(t *testing.T, description any, schemaName string) {
	t.Helper()
	text, _ := description.(string)
	normalized := strings.ToLower(text)
	if !strings.Contains(normalized, "stdin") || !strings.Contains(normalized, "environment") {
		t.Fatalf("%s.description must document excluded stdin and environment data, got %q", schemaName, text)
	}
}

func validRunRequestPayloadFixture() map[string]any {
	return map[string]any{
		"recordedAt": "2026-04-10T12:00:00Z",
		"factory": map[string]any{
			"name":             "runtime-factory",
			"factoryDirectory": "/tmp/runtime-factory",
			"sourceDirectory":  "/tmp/customer-factory",
			"id":               "runtime-factory",
			"metadata":         map[string]any{"factory_hash": "sha256:test"},
			"inputTypes": []any{
				map[string]any{"name": "default", "type": "DEFAULT"},
			},
			"workTypes": []any{
				map[string]any{
					"name": "story",
					"states": []any{
						map[string]any{"name": "init", "type": "INITIAL"},
						map[string]any{"name": "complete", "type": "TERMINAL"},
						map[string]any{"name": "failed", "type": "FAILED"},
					},
				},
			},
			"workers": []any{map[string]any{
				"name":             "executor",
				"type":             "MODEL_WORKER",
				"modelProvider":    "CLAUDE",
				"executorProvider": "SCRIPT_WRAP",
				"stopToken":        "<COMPLETE>",
				"skipPermissions":  true,
				"command":          "echo",
			}},
			"workstations": []any{map[string]any{
				"name":         "execute",
				"worker":       "executor",
				"behavior":     "STANDARD",
				"type":         "MODEL_WORKSTATION",
				"promptFile":   "prompt.md",
				"body":         "Finish {{ .WorkID }}",
				"outputSchema": "{\"type\":\"object\"}",
				"inputs":       []any{map[string]any{"workType": "story", "state": "init"}},
				"outputs":      []any{map[string]any{"workType": "story", "state": "complete"}},
				"onRejection":  []map[string]any{{"workType": "story", "state": "init"}},
				"onFailure":    []map[string]any{{"workType": "story", "state": "failed"}},
				"resources":    []any{map[string]any{"name": "agent-slot", "capacity": 1}},
				"limits": map[string]any{
					"maxRetries":       2,
					"maxExecutionTime": "2m",
				},
				"cron": map[string]any{
					"schedule":       "*/5 * * * *",
					"triggerAtStart": true,
					"expiryWindow":   "30s",
				},
				"guards":           []any{map[string]any{"type": "VISIT_COUNT", "workstation": "execute", "maxVisits": 3}},
				"stopWords":        []any{"DONE", "RETRY"},
				"workingDirectory": "/tmp/worktree",
				"env":              map[string]any{"TEAM": "factory"},
			}},
		},
	}
}

func assertCanonicalFactoryEventFixtureEntry(
	t *testing.T,
	doc *openapi3.T,
	index int,
	event map[string]any,
) string {
	t.Helper()
	eventType := requireCanonicalFactoryEventFixtureType(t, index, event, doc.Components.Schemas["FactoryEventType"].Value)
	contextMap, payloadMap := requireCanonicalFactoryEventFixtureBoundaryObjects(
		t,
		doc,
		index,
		eventType,
		event,
		doc.Components.Schemas["FactoryEventContext"].Value,
	)
	assertCanonicalFactoryEventFixtureOwnership(t, eventType, contextMap, payloadMap)
	assertCanonicalFactoryEventFixtureEnvelope(t, index, event)
	return eventType
}

func requireCanonicalFactoryEventFixtureType(
	t *testing.T,
	index int,
	event map[string]any,
	eventTypeSchema *openapi3.Schema,
) string {
	t.Helper()
	eventType, ok := event["type"].(string)
	if !ok {
		t.Fatalf("canonical event vocabulary fixture event %d type = %T, want string", index, event["type"])
	}
	if err := eventTypeSchema.VisitJSON(eventType); err != nil {
		t.Fatalf("canonical event vocabulary fixture event %d type should validate: %v", index, err)
	}
	return eventType
}

func requireCanonicalFactoryEventFixtureBoundaryObjects(
	t *testing.T,
	doc *openapi3.T,
	index int,
	eventType string,
	event map[string]any,
	eventContextSchema *openapi3.Schema,
) (map[string]any, map[string]any) {
	t.Helper()
	contextValue, ok := event["context"]
	if !ok {
		t.Fatalf("canonical event vocabulary fixture event %d context is missing", index)
	}
	if err := eventContextSchema.VisitJSON(contextValue); err != nil {
		t.Fatalf("canonical event vocabulary fixture event %d context should validate: %v", index, err)
	}
	payloadValue, ok := event["payload"]
	if !ok {
		t.Fatalf("canonical event vocabulary fixture event %d payload is missing", index)
	}
	payloadSchemaName, ok := canonicalFactoryEventPayloadSchemaNamesByType[eventType]
	if !ok {
		t.Fatalf("canonical event vocabulary fixture event %d type %q has no payload schema mapping", index, eventType)
	}
	if err := doc.Components.Schemas[payloadSchemaName].Value.VisitJSON(payloadValue); err != nil {
		t.Fatalf("canonical event vocabulary fixture event %d payload should validate against %s: %v", index, payloadSchemaName, err)
	}
	contextMap, ok := contextValue.(map[string]any)
	if !ok {
		t.Fatalf("canonical event vocabulary fixture event %d context = %T, want object", index, contextValue)
	}
	payloadMap, ok := payloadValue.(map[string]any)
	if !ok {
		t.Fatalf("canonical event vocabulary fixture event %d payload = %T, want object", index, payloadValue)
	}
	return contextMap, payloadMap
}

func assertCanonicalFactoryEventFixtureOwnership(t *testing.T, eventType string, contextMap, payloadMap map[string]any) {
	t.Helper()
	switch eventType {
	case "DISPATCH_REQUEST":
		assertJSONKeysAbsent(t, payloadMap, "canonical dispatch request payload", "dispatchId", "workstation", "worker")
		assertJSONKeysPresent(t, contextMap, "canonical dispatch request context", "dispatchId", "requestId", "currentChainingTraceId", "previousChainingTraceIds")
		if metadata, ok := payloadMap["metadata"].(map[string]any); ok {
			assertJSONKeysAbsent(t, metadata, "canonical dispatch request metadata", "requestId")
		}
	case "INFERENCE_REQUEST":
		assertJSONKeysAbsent(t, payloadMap, "canonical inference request payload", "dispatchId", "transitionId")
		assertJSONKeysPresent(t, contextMap, "canonical inference request context", "dispatchId")
	case "INFERENCE_RESPONSE":
		assertJSONKeysAbsent(t, payloadMap, "canonical inference response payload", "dispatchId", "transitionId")
		assertJSONKeysPresent(t, contextMap, "canonical inference response context", "dispatchId")
	case "DISPATCH_RESPONSE":
		assertJSONKeysAbsent(t, payloadMap, "canonical dispatch response payload", "dispatchId", "workstation", "worker")
		assertJSONKeysPresent(t, contextMap, "canonical dispatch response context", "dispatchId", "currentChainingTraceId", "previousChainingTraceIds")
	}
}

func assertCanonicalFactoryEventFixtureEnvelope(t *testing.T, index int, event map[string]any) {
	t.Helper()
	if got, ok := event["schemaVersion"].(string); !ok || got != "agent-factory.event.v1" {
		t.Fatalf("canonical event vocabulary fixture event %d schemaVersion = %#v, want %q", index, event["schemaVersion"], "agent-factory.event.v1")
	}
	if _, ok := event["id"].(string); !ok {
		t.Fatalf("canonical event vocabulary fixture event %d id = %T, want string", index, event["id"])
	}
}
