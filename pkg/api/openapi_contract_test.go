package api

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/portpowered/agent-factory/pkg/api/generated"
	"gopkg.in/yaml.v3"
)

var canonicalFactoryEventTypeValues = []string{
	"RUN_REQUEST",
	"INITIAL_STRUCTURE_REQUEST",
	"WORK_REQUEST",
	"RELATIONSHIP_CHANGE_REQUEST",
	"DISPATCH_REQUEST",
	"INFERENCE_REQUEST",
	"INFERENCE_RESPONSE",
	"SCRIPT_REQUEST",
	"SCRIPT_RESPONSE",
	"DISPATCH_RESPONSE",
	"FACTORY_STATE_RESPONSE",
	"RUN_RESPONSE",
}

var retiredFactoryEventTypeValues = []string{
	"RUN_STARTED",
	"INITIAL_STRUCTURE",
	"RELATIONSHIP_CHANGE",
	"DISPATCH_CREATED",
	"DISPATCH_COMPLETED",
	"FACTORY_STATE_CHANGE",
	"RUN_FINISHED",
}

var bundledFactoryEventContractSchemaNames = []string{
	"FactoryEvent",
	"FactoryEventContext",
	"FactoryEventType",
	"DispatchConsumedWorkRef",
	"DispatchRequestEventMetadata",
	"FactoryState",
	"InferenceOutcome",
	"Diagnostics",
	"ProviderDiagnostic",
	"ProviderFailureMetadata",
	"ProviderSessionMetadata",
	"RenderedPromptDiagnostic",
	"SafeWorkDiagnostics",
	"WallClock",
	"WorkDiagnostics",
	"WorkMetrics",
	"WorkOutcome",
	"RunRequestEventPayload",
	"InitialStructureRequestEventPayload",
	"WorkRequestEventPayload",
	"RelationshipChangeRequestEventPayload",
	"DispatchRequestEventPayload",
	"InferenceRequestEventPayload",
	"InferenceResponseEventPayload",
	"ScriptRequestEventPayload",
	"ScriptResponseEventPayload",
	"ScriptExecutionOutcome",
	"ScriptFailureType",
	"DispatchResponseEventPayload",
	"FactoryStateResponseEventPayload",
	"RunResponseEventPayload",
}

var bundledFactoryEventPayloadRefs = []string{
	"#/components/schemas/RunRequestEventPayload",
	"#/components/schemas/InitialStructureRequestEventPayload",
	"#/components/schemas/WorkRequestEventPayload",
	"#/components/schemas/RelationshipChangeRequestEventPayload",
	"#/components/schemas/DispatchRequestEventPayload",
	"#/components/schemas/InferenceRequestEventPayload",
	"#/components/schemas/InferenceResponseEventPayload",
	"#/components/schemas/ScriptRequestEventPayload",
	"#/components/schemas/ScriptResponseEventPayload",
	"#/components/schemas/DispatchResponseEventPayload",
	"#/components/schemas/FactoryStateResponseEventPayload",
	"#/components/schemas/RunResponseEventPayload",
}

var bundledFactoryEventTypeValues = canonicalFactoryEventTypeValues

var canonicalFactoryEventPayloadSchemaNamesByType = map[string]string{
	"RUN_REQUEST":                 "RunRequestEventPayload",
	"INITIAL_STRUCTURE_REQUEST":   "InitialStructureRequestEventPayload",
	"WORK_REQUEST":                "WorkRequestEventPayload",
	"RELATIONSHIP_CHANGE_REQUEST": "RelationshipChangeRequestEventPayload",
	"DISPATCH_REQUEST":            "DispatchRequestEventPayload",
	"INFERENCE_REQUEST":           "InferenceRequestEventPayload",
	"INFERENCE_RESPONSE":          "InferenceResponseEventPayload",
	"SCRIPT_REQUEST":              "ScriptRequestEventPayload",
	"SCRIPT_RESPONSE":             "ScriptResponseEventPayload",
	"DISPATCH_RESPONSE":           "DispatchResponseEventPayload",
	"FACTORY_STATE_RESPONSE":      "FactoryStateResponseEventPayload",
	"RUN_RESPONSE":                "RunResponseEventPayload",
}

// portos:func-length-exception owner=agent-factory reason=openapi-contract-table review=2026-07-18 removal=split-operation-and-schema-assertions-before-next-openapi-surface-change
func TestOpenAPIContract_ContainsCoveredJSONOperations(t *testing.T) {
	data, err := os.ReadFile("../../api/openapi.yaml")
	if err != nil {
		t.Fatalf("read openapi contract: %v", err)
	}

	var doc map[string]any
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parse openapi contract: %v", err)
	}

	if got, ok := doc["openapi"].(string); !ok || got == "" {
		t.Fatalf("openapi version is missing")
	}
	if _, ok := doc["info"].(map[string]any); !ok {
		t.Fatalf("info object is missing")
	}
	paths, ok := doc["paths"].(map[string]any)
	if !ok {
		t.Fatalf("paths object is missing")
	}

	requiredOperations := map[string][]string{
		"/work":                       {"get", "post"},
		"/work-requests/{request_id}": {"put"},
		"/work/{id}":                  {"get"},
		"/events":                     {"get"},
		"/status":                     {"get"},
		"/factory":                    {"post"},
		"/factory/~current":           {"get"},
	}
	for path, methods := range requiredOperations {
		pathItem, ok := paths[path].(map[string]any)
		if !ok {
			t.Fatalf("paths.%s is missing", path)
		}
		for _, method := range methods {
			operation, ok := pathItem[method].(map[string]any)
			if !ok {
				t.Fatalf("paths.%s.%s operation is missing", path, method)
			}
			if _, ok := operation["responses"].(map[string]any); !ok {
				t.Fatalf("paths.%s.%s.responses is missing", path, method)
			}
			if _, ok := operation["description"].(string); !ok {
				t.Fatalf("paths.%s.%s.description is missing", path, method)
			}
		}
	}

	removedPaths := []string{
		"/dashboard",
		"/dashboard/stream",
		"/state",
		"/traces/{id}",
		"/traces/{trace_id}",
		"/work/{id}/trace",
		"/workflows",
		"/workflows/{workflow_id}",
	}
	for _, path := range removedPaths {
		if _, ok := paths[path]; ok {
			t.Fatalf("paths.%s must not be published for removed factory endpoints", path)
		}
	}

	components, ok := doc["components"].(map[string]any)
	if !ok {
		t.Fatalf("components object is missing")
	}
	schemas, ok := components["schemas"].(map[string]any)
	if !ok {
		t.Fatalf("components.schemas object is missing")
	}
	requiredSchemas := []string{
		"SubmitWorkRequest",
		"SubmitWorkResponse",
		"UpsertWorkRequestResponse",
		"WorkRequest",
		"Work",
		"Relation",
		"ListWorkResponse",
		"TokenResponse",
		"ErrorFamily",
		"ErrorResponse",
		"FactoryName",
		"NamedFactory",
		"StatusCategories",
		"StatusResponse",
	}
	for _, schema := range requiredSchemas {
		if _, ok := schemas[schema]; !ok {
			t.Fatalf("components.schemas.%s is missing", schema)
		}
	}
	for _, schema := range []string{"Factory", "Workstation", "WorkstationKind"} {
		if _, ok := schemas[schema]; !ok {
			t.Fatalf("components.schemas.%s is missing", schema)
		}
	}

	removedSchemas := []string{
		"DashboardResponse",
		"DashboardRuntime",
		"DashboardTopology",
		"ListWorkflowsResponse",
		"StateResponse",
		"TraceResponse",
		"WorkflowResponse",
	}
	for _, schema := range removedSchemas {
		if _, ok := schemas[schema]; ok {
			t.Fatalf("components.schemas.%s must not be published for removed factory endpoints", schema)
		}
	}

	submitWorkRequestSchema, ok := schemas["SubmitWorkRequest"].(map[string]any)
	if !ok {
		t.Fatalf("components.schemas.SubmitWorkRequest must be an object schema")
	}
	submitWorkRequestRequired, ok := submitWorkRequestSchema["required"].([]any)
	if !ok {
		t.Fatalf("components.schemas.SubmitWorkRequest.required is missing")
	}
	if !containsString(submitWorkRequestRequired, "workTypeName") {
		t.Fatalf("components.schemas.SubmitWorkRequest.required is missing workTypeName")
	}
	submitWorkRequestProperties, ok := submitWorkRequestSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("components.schemas.SubmitWorkRequest.properties is missing")
	}
	if _, ok := submitWorkRequestProperties["workTypeName"].(map[string]any); !ok {
		t.Fatalf("components.schemas.SubmitWorkRequest.properties.workTypeName is missing")
	}
	if _, ok := submitWorkRequestProperties["currentChainingTraceId"].(map[string]any); !ok {
		t.Fatalf("components.schemas.SubmitWorkRequest.properties.currentChainingTraceId is missing")
	}
	assertArrayItemRef(t, submitWorkRequestProperties, "relations", "#/components/schemas/SubmitRelation")
	if _, ok := submitWorkRequestProperties["work_type_id"]; ok {
		t.Fatalf("components.schemas.SubmitWorkRequest.properties.work_type_id must not be advertised for submitted work")
	}

	submitRelationSchema := schemaObject(t, schemas, "SubmitRelation")
	assertRequiredFields(t, submitRelationSchema, "type", "targetWorkId")
	submitRelationProperties := schemaProperties(t, submitRelationSchema, "SubmitRelation")
	assertSchemaPropertiesPresent(t, submitRelationProperties, "SubmitRelation", "type", "targetWorkId")
	assertPropertiesAbsent(t, submitRelationProperties, "SubmitRelation", "sourceWorkName", "targetWorkName")

	workRequestSchema, ok := schemas["WorkRequest"].(map[string]any)
	if !ok {
		t.Fatalf("components.schemas.WorkRequest must be an object schema")
	}
	workRequestRequired, ok := workRequestSchema["required"].([]any)
	if !ok {
		t.Fatalf("components.schemas.WorkRequest.required is missing")
	}
	for _, field := range []string{"requestId", "type"} {
		if !containsString(workRequestRequired, field) {
			t.Fatalf("components.schemas.WorkRequest.required is missing %q", field)
		}
	}
	workRequestProperties, ok := workRequestSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("components.schemas.WorkRequest.properties is missing")
	}
	if _, ok := workRequestProperties["requestId"].(map[string]any); !ok {
		t.Fatalf("components.schemas.WorkRequest.properties.requestId is missing")
	}
	if _, ok := workRequestProperties["currentChainingTraceId"].(map[string]any); !ok {
		t.Fatalf("components.schemas.WorkRequest.properties.currentChainingTraceId is missing")
	}
	if _, ok := workRequestProperties["type"].(map[string]any); !ok {
		t.Fatalf("components.schemas.WorkRequest.properties.type is missing")
	}
	workRequestType := schemaObject(t, schemas, "WorkRequestType")
	assertEnumValues(t, workRequestType, "WorkRequestType", []string{"FACTORY_REQUEST_BATCH"})
	workRequestTypeVarNames, ok := workRequestType["x-enum-varnames"].([]any)
	if !ok {
		t.Fatalf("components.schemas.WorkRequestType.x-enum-varnames is missing")
	}
	if containsString(workRequestTypeVarNames, "WorkRequestTypeDefault") {
		t.Fatalf("components.schemas.WorkRequestType must not advertise legacy DEFAULT request type")
	}

	workSchema, ok := schemas["Work"].(map[string]any)
	if !ok {
		t.Fatalf("components.schemas.Work must be an object schema")
	}
	workProperties, ok := workSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("components.schemas.Work.properties is missing")
	}
	for _, field := range []string{"name", "workId", "requestId", "workTypeName", "state", "currentChainingTraceId", "previousChainingTraceIds", "traceId", "payload", "tags"} {
		if _, ok := workProperties[field].(map[string]any); !ok {
			t.Fatalf("components.schemas.Work.properties.%s is missing", field)
		}
	}
	if _, ok := workProperties["work_type_id"]; ok {
		t.Fatalf("components.schemas.Work.properties.work_type_id must not be advertised for submitted work items")
	}
	if _, ok := workProperties["target_state"]; ok {
		t.Fatalf("components.schemas.Work.properties.target_state must not be advertised for submitted work items")
	}

	workstationSchema, ok := schemas["Workstation"].(map[string]any)
	if !ok {
		t.Fatalf("components.schemas.Workstation must be an object schema")
	}
	workstationProperties, ok := workstationSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("components.schemas.Workstation.properties is missing")
	}
	assertPropertyRef(t, workstationProperties, "kind", "#/components/schemas/WorkstationKind")
	assertPropertyRef(t, workstationProperties, "type", "#/components/schemas/WorkstationType")
	if _, ok := workstationProperties["timeout"]; ok {
		t.Fatalf("components.schemas.Workstation.properties.timeout must not be advertised")
	}
	if _, ok := workstationProperties["runtime_type"]; ok {
		t.Fatalf("components.schemas.Workstation.properties.runtime_type must not be advertised")
	}
	workstationKind := schemaObject(t, schemas, "WorkstationKind")
	assertEnumValues(t, workstationKind, "WorkstationKind", []string{"STANDARD", "REPEATER", "CRON"})
	workstationType := schemaObject(t, schemas, "WorkstationType")
	assertEnumValues(t, workstationType, "WorkstationType", []string{"MODEL_WORKSTATION", "LOGICAL_MOVE"})
	factorySchema := schemaObject(t, schemas, "Factory")
	factoryProperties := schemaProperties(t, factorySchema, "Factory")
	if _, ok := factoryProperties["exhaustion_rules"]; ok {
		t.Fatalf("components.schemas.Factory.properties.exhaustion_rules must not be advertised")
	}
	if _, ok := factoryProperties["exhaustionRules"]; ok {
		t.Fatalf("components.schemas.Factory.properties.exhaustionRules must not be advertised")
	}
	if _, ok := schemas["ExhaustionRule"]; ok {
		t.Fatalf("components.schemas.ExhaustionRule must not be advertised")
	}
	if description := strings.ToLower(factorySchema["description"].(string)); !strings.Contains(description, "guarded logical_move workstations") {
		t.Fatalf("components.schemas.Factory.description must direct guarded loop breakers to guarded LOGICAL_MOVE workstations")
	}
	if guardsProperty, ok := workstationProperties["guards"].(map[string]any); !ok {
		t.Fatalf("components.schemas.Workstation.properties.guards is missing")
	} else if description := strings.ToLower(guardsProperty["description"].(string)); !strings.Contains(description, "visit_count") {
		t.Fatalf("components.schemas.Workstation.properties.guards must describe visit_count loop-breaker guards")
	}

	errorSchema, ok := schemas["ErrorResponse"].(map[string]any)
	if !ok {
		t.Fatalf("components.schemas.ErrorResponse must be an object schema")
	}
	requiredFields, ok := errorSchema["required"].([]any)
	if !ok {
		t.Fatalf("components.schemas.ErrorResponse.required is missing")
	}
	for _, field := range []string{"message", "family", "code"} {
		if !containsString(requiredFields, field) {
			t.Fatalf("components.schemas.ErrorResponse.required is missing %q", field)
		}
	}
	properties, ok := errorSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("components.schemas.ErrorResponse.properties is missing")
	}
	assertPropertyRef(t, properties, "family", "#/components/schemas/ErrorFamily")
	codeProperty, ok := properties["code"].(map[string]any)
	if !ok {
		t.Fatalf("components.schemas.ErrorResponse.properties.code is missing")
	}
	codeEnum, ok := codeProperty["enum"].([]any)
	if !ok {
		t.Fatalf("components.schemas.ErrorResponse.properties.code.enum is missing")
	}
	for _, code := range []string{
		"BAD_REQUEST",
		"INVALID_FACTORY_NAME",
		"FACTORY_ALREADY_EXISTS",
		"INVALID_FACTORY",
		"FACTORY_NOT_IDLE",
		"NOT_FOUND",
		"INTERNAL_ERROR",
	} {
		if !containsString(codeEnum, code) {
			t.Fatalf("components.schemas.ErrorResponse.properties.code.enum is missing %q", code)
		}
	}

	errorFamilySchema := schemaObject(t, schemas, "ErrorFamily")
	assertEnumValues(t, errorFamilySchema, "ErrorFamily", []string{
		"BAD_REQUEST",
		"CONFLICT",
		"NOT_FOUND",
		"INTERNAL_SERVER_ERROR",
	})
}

func TestOpenAPIContract_WorkstationCronIsScheduleOnly(t *testing.T) {
	data, err := os.ReadFile("../../api/openapi.yaml")
	if err != nil {
		t.Fatalf("read openapi contract: %v", err)
	}

	var doc map[string]any
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parse openapi contract: %v", err)
	}

	cronSchema := schemaObject(t, componentSchemas(t, doc), "WorkstationCron")
	assertRequiredFields(t, cronSchema, "schedule")
	properties := schemaProperties(t, cronSchema, "WorkstationCron")
	for _, field := range []string{"schedule", "triggerAtStart", "jitter", "expiryWindow"} {
		if _, ok := properties[field].(map[string]any); !ok {
			t.Fatalf("WorkstationCron.properties.%s is missing", field)
		}
	}
	for _, retiredField := range []string{"trigger_at_start", "expiry_window"} {
		if _, ok := properties[retiredField]; ok {
			t.Fatalf("WorkstationCron.properties.%s must not be advertised", retiredField)
		}
	}
	if _, ok := properties["interval"]; ok {
		t.Fatalf("WorkstationCron.properties.interval must not be advertised")
	}
}

func TestOpenAPIContract_FactorySchemaGraphIncludesCustomerFacingDescriptions(t *testing.T) {
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromFile("../../api/openapi.yaml")
	if err != nil {
		t.Fatalf("load openapi contract: %v", err)
	}
	if err := doc.Validate(context.Background()); err != nil {
		t.Fatalf("validate openapi contract: %v", err)
	}

	factory := requireOpenAPI3ComponentSchema(t, doc, "Factory")
	assertOpenAPI3Description(t, "Factory", factory.Description)
	assertOpenAPI3PropertyDescription(t, factory, "Factory", "project")
	workType := assertOpenAPI3ArrayPropertyDescription(t, factory, "Factory", "workTypes")
	resource := assertOpenAPI3ArrayPropertyDescription(t, factory, "Factory", "resources")
	worker := assertOpenAPI3ArrayPropertyDescription(t, factory, "Factory", "workers")
	workstation := assertOpenAPI3ArrayPropertyDescription(t, factory, "Factory", "workstations")

	assertOpenAPI3Description(t, "WorkType", workType.Description)
	assertOpenAPI3PropertyDescription(t, workType, "WorkType", "name")
	workState := assertOpenAPI3ArrayPropertyDescription(t, workType, "WorkType", "states")

	assertOpenAPI3Description(t, "WorkState", workState.Description)
	assertOpenAPI3PropertyDescription(t, workState, "WorkState", "name")
	assertOpenAPI3PropertyDescription(t, workState, "WorkState", "type")

	assertOpenAPI3Description(t, "Resource", resource.Description)
	assertOpenAPI3PropertyDescription(t, resource, "Resource", "name")
	assertOpenAPI3PropertyDescription(t, resource, "Resource", "capacity")

	assertOpenAPI3Description(t, "Worker", worker.Description)
	for _, propertyName := range []string{"name", "type", "model", "modelProvider", "executorProvider", "command", "resources", "timeout"} {
		assertOpenAPI3PropertyDescription(t, worker, "Worker", propertyName)
	}

	assertOpenAPI3Description(t, "Workstation", workstation.Description)
	for _, propertyName := range []string{"name", "kind", "type", "worker", "limits", "resources", "stopWords", "inputs", "outputs", "guards"} {
		assertOpenAPI3PropertyDescription(t, workstation, "Workstation", propertyName)
	}
	workstationLimits := assertOpenAPI3RefPropertyDescription(t, workstation, "Workstation", "limits")
	workstationCron := assertOpenAPI3RefPropertyDescription(t, workstation, "Workstation", "cron")
	workstationIO := assertOpenAPI3ArrayPropertyDescription(t, workstation, "Workstation", "inputs")
	workstationGuard := assertOpenAPI3ArrayPropertyDescription(t, workstation, "Workstation", "guards")

	assertOpenAPI3Description(t, "WorkstationLimits", workstationLimits.Description)
	assertOpenAPI3PropertyDescription(t, workstationLimits, "WorkstationLimits", "maxRetries")
	assertOpenAPI3PropertyDescription(t, workstationLimits, "WorkstationLimits", "maxExecutionTime")

	assertOpenAPI3Description(t, "WorkstationCron", workstationCron.Description)
	for _, propertyName := range []string{"schedule", "triggerAtStart", "jitter", "expiryWindow"} {
		assertOpenAPI3PropertyDescription(t, workstationCron, "WorkstationCron", propertyName)
	}

	assertOpenAPI3Description(t, "WorkstationIO", workstationIO.Description)
	for _, propertyName := range []string{"workType", "state", "guards"} {
		assertOpenAPI3PropertyDescription(t, workstationIO, "WorkstationIO", propertyName)
	}
	inputGuard := assertOpenAPI3ArrayPropertyDescription(t, workstationIO, "WorkstationIO", "guards")

	assertOpenAPI3Description(t, "WorkstationGuard", workstationGuard.Description)
	for _, propertyName := range []string{"type", "workstation", "maxVisits"} {
		assertOpenAPI3PropertyDescription(t, workstationGuard, "WorkstationGuard", propertyName)
	}

	assertOpenAPI3Description(t, "InputGuard", inputGuard.Description)
	for _, propertyName := range []string{"type", "parentInput", "spawnedBy"} {
		assertOpenAPI3PropertyDescription(t, inputGuard, "InputGuard", propertyName)
	}
}

func TestOpenAPIContract_NamedFactorySchemaReusesCanonicalFactoryShape(t *testing.T) {
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromFile("../../api/openapi.yaml")
	if err != nil {
		t.Fatalf("load openapi contract: %v", err)
	}
	if err := doc.Validate(context.Background()); err != nil {
		t.Fatalf("validate openapi contract: %v", err)
	}

	namedFactory := requireOpenAPI3ComponentSchema(t, doc, "NamedFactory")
	assertOpenAPI3Description(t, "NamedFactory", namedFactory.Description)
	assertRequiredStringValues(t, namedFactory.Required, "name", "factory")
	assertOpenAPI3PropertyRef(t, namedFactory, "NamedFactory", "name", "#/components/schemas/FactoryName")
	assertOpenAPI3PropertyRef(t, namedFactory, "NamedFactory", "factory", "#/components/schemas/Factory")
	if namedFactory.Example == nil {
		t.Fatal("NamedFactory.example is missing")
	}
	if err := namedFactory.VisitJSON(namedFactory.Example); err != nil {
		t.Fatalf("NamedFactory.example should validate: %v", err)
	}
}

func TestOpenAPIContract_NamedFactoryOperationsPublishMachineReadableErrors(t *testing.T) {
	data, err := os.ReadFile("../../api/openapi.yaml")
	if err != nil {
		t.Fatalf("read openapi contract: %v", err)
	}

	var doc map[string]any
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parse openapi contract: %v", err)
	}

	paths, ok := doc["paths"].(map[string]any)
	if !ok {
		t.Fatalf("paths object is missing")
	}

	createFactory := pathOperation(t, paths, "/factory", "post")
	assertResponseSchemaRef(t, createFactory, "201", "#/components/schemas/NamedFactory")
	assertResponseRef(t, createFactory, "400", "#/components/responses/CreateFactoryBadRequest")
	assertResponseRef(t, createFactory, "409", "#/components/responses/CreateFactoryConflict")

	currentFactory := pathOperation(t, paths, "/factory/~current", "get")
	assertResponseSchemaRef(t, currentFactory, "200", "#/components/schemas/NamedFactory")
	assertResponseRef(t, currentFactory, "404", "#/components/responses/CurrentFactoryNotFound")

	components, ok := doc["components"].(map[string]any)
	if !ok {
		t.Fatalf("components object is missing")
	}
	responses, ok := components["responses"].(map[string]any)
	if !ok {
		t.Fatalf("components.responses object is missing")
	}

	assertResponseExampleCodeFamilies(t, responses, "CreateFactoryBadRequest", map[string]string{
		"INVALID_FACTORY_NAME": "BAD_REQUEST",
		"INVALID_FACTORY":      "BAD_REQUEST",
	})
	assertResponseExampleCodeFamilies(t, responses, "CreateFactoryConflict", map[string]string{
		"FACTORY_ALREADY_EXISTS": "CONFLICT",
		"FACTORY_NOT_IDLE":       "CONFLICT",
	})
	assertResponseExampleCodeFamilies(t, responses, "CurrentFactoryNotFound", map[string]string{
		"NOT_FOUND": "NOT_FOUND",
	})
}

func TestOpenAPIContract_DefinesWorkstationRequestProjectionSlice(t *testing.T) {
	schemas := loadBundledOpenAPIComponentSchemas(t)

	assertWorkstationRequestProjectionSchemasPresent(t, schemas)
	assertWorkstationRequestProjectionSliceSchema(t, schemas)
	assertWorkstationRequestViewSchema(t, schemas)
	assertWorkstationRequestPayloadSchemas(t, schemas)
	assertWorkstationRequestScriptBoundarySchemas(t, schemas)
}

func TestOpenAPIContract_PublicRuntimeAndFactoryWorldSchemasUseCamelCase(t *testing.T) {
	schemas := loadBundledOpenAPIComponentSchemas(t)

	offenses := collectSnakeCaseComponentFields(t, schemas, []string{
		"Relation",
		"StatusResponse",
		"SubmitWorkRequest",
		"SubmitWorkResponse",
		"TokenHistory",
		"TokenResponse",
		"UpsertWorkRequestResponse",
		"Work",
		"WorkRequest",
		"FactoryWorldWorkstationRequestProjectionSlice",
		"FactoryWorldRenderedPromptDiagnostic",
		"FactoryWorldProviderDiagnostic",
		"FactoryWorldWorkDiagnostics",
		"FactoryWorldWorkItemRef",
		"FactoryWorldTokenView",
		"FactoryWorldMutationView",
		"FactoryWorldScriptRequestView",
		"FactoryWorldScriptResponseView",
		"FactoryWorldWorkstationRequestCountView",
		"FactoryWorldWorkstationRequestRequestView",
		"FactoryWorldWorkstationRequestResponseView",
		"FactoryWorldWorkstationRequestView",
	})
	if len(offenses) == 0 {
		return
	}

	t.Fatalf("public runtime and factory-world schemas must use camelCase:\n- %s", strings.Join(offenses, "\n- "))
}

func loadBundledOpenAPIComponentSchemas(t *testing.T) map[string]any {
	t.Helper()

	data, err := os.ReadFile("../../api/openapi.yaml")
	if err != nil {
		t.Fatalf("read openapi contract: %v", err)
	}

	var doc map[string]any
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parse openapi contract: %v", err)
	}

	schemas := componentSchemas(t, doc)
	assertProjectionSchemasPresent(t, schemas)
	assertWorkstationRequestProjectionSliceSchema(t, schemas)
	assertWorkstationRequestViewSchema(t, schemas)
	assertWorkstationRequestRequestSchema(t, schemas)
	assertWorkstationRequestWorkRefSchemas(t, schemas)
	assertWorkstationRequestResponseSchema(t, schemas)
	return schemas
}

func collectSnakeCaseComponentFields(t *testing.T, schemas map[string]any, rootSchemas []string) []string {
	t.Helper()

	visited := make(map[string]bool)
	offenses := make(map[string]struct{})
	for _, schemaName := range rootSchemas {
		collectSnakeCaseFieldsFromComponent(t, schemas, schemaName, visited, offenses)
	}

	out := make([]string, 0, len(offenses))
	for offense := range offenses {
		out = append(out, offense)
	}
	sort.Strings(out)
	return out
}

func collectSnakeCaseFieldsFromComponent(
	t *testing.T,
	schemas map[string]any,
	schemaName string,
	visited map[string]bool,
	offenses map[string]struct{},
) {
	t.Helper()

	if visited[schemaName] {
		return
	}
	visited[schemaName] = true
	collectSnakeCaseFieldsFromSchema(t, schemas, schemaName, schemaObject(t, schemas, schemaName), visited, offenses)
}

func collectSnakeCaseFieldsFromSchema(
	t *testing.T,
	schemas map[string]any,
	path string,
	schema map[string]any,
	visited map[string]bool,
	offenses map[string]struct{},
) {
	t.Helper()

	if properties, ok := schema["properties"].(map[string]any); ok {
		for propertyName, propertyAny := range properties {
			if strings.Contains(propertyName, "_") {
				offenses[path+"."+propertyName] = struct{}{}
			}
			propertySchema, ok := propertyAny.(map[string]any)
			if !ok {
				continue
			}
			collectSnakeCaseFieldsFromSubSchema(t, schemas, propertySchema, visited, offenses)
		}
	}

	if additionalProperties, ok := schema["additionalProperties"].(map[string]any); ok {
		collectSnakeCaseFieldsFromSubSchema(t, schemas, additionalProperties, visited, offenses)
	}
}

func collectSnakeCaseFieldsFromSubSchema(
	t *testing.T,
	schemas map[string]any,
	schema map[string]any,
	visited map[string]bool,
	offenses map[string]struct{},
) {
	t.Helper()

	if refName, ok := openAPISchemaNameFromRef(schema["$ref"]); ok {
		collectSnakeCaseFieldsFromComponent(t, schemas, refName, visited, offenses)
	}
	if items, ok := schema["items"].(map[string]any); ok {
		collectSnakeCaseFieldsFromSubSchema(t, schemas, items, visited, offenses)
	}
	for _, compositionKey := range []string{"allOf", "anyOf", "oneOf"} {
		items, ok := schema[compositionKey].([]any)
		if !ok {
			continue
		}
		for _, item := range items {
			itemSchema, ok := item.(map[string]any)
			if !ok {
				continue
			}
			collectSnakeCaseFieldsFromSubSchema(t, schemas, itemSchema, visited, offenses)
		}
	}
	if _, ok := schema["properties"].(map[string]any); ok {
		collectSnakeCaseFieldsFromSchema(t, schemas, "<inline>", schema, visited, offenses)
	}
	if additionalProperties, ok := schema["additionalProperties"].(map[string]any); ok {
		collectSnakeCaseFieldsFromSubSchema(t, schemas, additionalProperties, visited, offenses)
	}
}

func assertWorkstationRequestProjectionSchemasPresent(t *testing.T, schemas map[string]any) {
	t.Helper()

	for _, schema := range []string{
		"FactoryWorldWorkstationRequestProjectionSlice",
		"FactoryWorldScriptRequestView",
		"FactoryWorldScriptResponseView",
		"FactoryWorldWorkstationRequestView",
		"FactoryWorldWorkstationRequestCountView",
		"FactoryWorldWorkstationRequestRequestView",
		"FactoryWorldWorkstationRequestResponseView",
		"FactoryWorldWorkItemRef",
		"FactoryWorldTokenView",
		"FactoryWorldMutationView",
		"FactoryWorldWorkDiagnostics",
		"FactoryWorldProviderDiagnostic",
		"FactoryWorldRenderedPromptDiagnostic",
	} {
		if _, ok := schemas[schema]; !ok {
			t.Fatalf("components.schemas.%s is missing", schema)
		}
	}
}

func assertWorkstationRequestPayloadSchemas(t *testing.T, schemas map[string]any) {
	t.Helper()

	requestPayload := schemaObject(t, schemas, "FactoryWorldWorkstationRequestRequestView")
	requestPayloadProperties := schemaProperties(t, requestPayload, "FactoryWorldWorkstationRequestRequestView")
	assertSchemaPropertiesPresent(t, requestPayloadProperties, "FactoryWorldWorkstationRequestRequestView", "startedAt", "requestTime", "prompt", "workingDirectory", "worktree", "provider", "model")
	assertArrayItemRef(t, requestPayloadProperties, "inputWorkItems", "#/components/schemas/FactoryWorldWorkItemRef")
	assertArrayItemRef(t, requestPayloadProperties, "consumedTokens", "#/components/schemas/FactoryWorldTokenView")
	assertPropertyRef(t, requestPayloadProperties, "requestMetadata", "#/components/schemas/StringMap")
	assertPropertyRef(t, requestPayloadProperties, "scriptRequest", "#/components/schemas/FactoryWorldScriptRequestView")

	responsePayload := schemaObject(t, schemas, "FactoryWorldWorkstationRequestResponseView")
	responsePayloadProperties := schemaProperties(t, responsePayload, "FactoryWorldWorkstationRequestResponseView")
	assertPropertyRef(t, responsePayloadProperties, "providerSession", "#/components/schemas/ProviderSessionMetadata")
	assertPropertyRef(t, responsePayloadProperties, "diagnostics", "#/components/schemas/FactoryWorldWorkDiagnostics")
	assertPropertyRef(t, responsePayloadProperties, "responseMetadata", "#/components/schemas/StringMap")
	assertPropertyRef(t, responsePayloadProperties, "scriptResponse", "#/components/schemas/FactoryWorldScriptResponseView")
	assertArrayItemRef(t, responsePayloadProperties, "outputWorkItems", "#/components/schemas/FactoryWorldWorkItemRef")
	assertArrayItemRef(t, responsePayloadProperties, "outputMutations", "#/components/schemas/FactoryWorldMutationView")
	assertSchemaPropertiesPresent(t, responsePayloadProperties, "FactoryWorldWorkstationRequestResponseView", "outcome", "feedback", "failureReason", "failureMessage", "responseText", "errorClass", "endTime", "durationMillis")
}

func assertWorkstationRequestScriptBoundarySchemas(t *testing.T, schemas map[string]any) {
	t.Helper()

	scriptRequestPayload := schemaObject(t, schemas, "FactoryWorldScriptRequestView")
	scriptRequestPayloadProperties := schemaProperties(t, scriptRequestPayload, "FactoryWorldScriptRequestView")
	for _, field := range []string{"scriptRequestId", "attempt", "command", "args"} {
		if _, ok := scriptRequestPayloadProperties[field].(map[string]any); !ok {
			t.Fatalf("FactoryWorldScriptRequestView.properties.%s is missing", field)
		}
	}

	scriptResponsePayload := schemaObject(t, schemas, "FactoryWorldScriptResponseView")
	scriptResponsePayloadProperties := schemaProperties(t, scriptResponsePayload, "FactoryWorldScriptResponseView")
	for _, field := range []string{"scriptRequestId", "attempt", "outcome", "stdout", "stderr", "durationMillis", "exitCode", "failureType"} {
		if _, ok := scriptResponsePayloadProperties[field].(map[string]any); !ok {
			t.Fatalf("FactoryWorldScriptResponseView.properties.%s is missing", field)
		}
	}
}

func TestOpenAPIContract_FactoryEventEnvelopeRefsSharedSchemas(t *testing.T) {
	data, err := os.ReadFile("../../api/openapi.yaml")
	if err != nil {
		t.Fatalf("read openapi contract: %v", err)
	}

	var doc map[string]any
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parse openapi contract: %v", err)
	}

	factoryEvent := schemaObject(t, componentSchemas(t, doc), "FactoryEvent")
	factoryEventProperties := schemaProperties(t, factoryEvent, "FactoryEvent")
	assertPropertyRef(t, factoryEventProperties, "type", "#/components/schemas/FactoryEventType")
	assertPropertyRef(t, factoryEventProperties, "context", "#/components/schemas/FactoryEventContext")
	assertPayloadUnionRefs(t, factoryEventProperties, bundledFactoryEventPayloadRefs)
}

func TestOpenAPIContract_BundledFactoryEventSchemasRemainComplete(t *testing.T) {
	data, err := os.ReadFile("../../api/openapi.yaml")
	if err != nil {
		t.Fatalf("read openapi contract: %v", err)
	}

	var doc map[string]any
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parse openapi contract: %v", err)
	}

	schemas := componentSchemas(t, doc)
	assertSchemaNamesPresent(t, schemas, bundledFactoryEventContractSchemaNames)

	factoryEvent := schemaObject(t, schemas, "FactoryEvent")
	assertRequiredFields(t, factoryEvent, "schemaVersion", "id", "type", "context", "payload")
	factoryEventProperties := schemaProperties(t, factoryEvent, "FactoryEvent")
	assertPropertyRef(t, factoryEventProperties, "type", "#/components/schemas/FactoryEventType")
	assertPropertyRef(t, factoryEventProperties, "context", "#/components/schemas/FactoryEventContext")
	assertPayloadUnionRefs(t, factoryEventProperties, bundledFactoryEventPayloadRefs)
	assertEnumValues(t, schemaObject(t, schemas, "FactoryEventType"), "FactoryEventType", bundledFactoryEventTypeValues)

	contextProperties := schemaProperties(t, schemaObject(t, schemas, "FactoryEventContext"), "FactoryEventContext")
	for _, field := range []string{"eventTime", "requestId", "traceIds", "workIds", "dispatchId"} {
		if _, ok := contextProperties[field]; !ok {
			t.Fatalf("FactoryEventContext.properties.%s is missing", field)
		}
	}

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

	stateResponseProperties := schemaProperties(t, schemaObject(t, schemas, "FactoryStateResponseEventPayload"), "FactoryStateResponseEventPayload")
	assertPropertyRef(t, stateResponseProperties, "previousState", "#/components/schemas/FactoryState")
	assertPropertyRef(t, stateResponseProperties, "state", "#/components/schemas/FactoryState")

	runResponseProperties := schemaProperties(t, schemaObject(t, schemas, "RunResponseEventPayload"), "RunResponseEventPayload")
	assertPropertyRef(t, runResponseProperties, "state", "#/components/schemas/FactoryState")
	assertPropertyRef(t, runResponseProperties, "wallClock", "#/components/schemas/WallClock")
	assertPropertyRef(t, runResponseProperties, "diagnostics", "#/components/schemas/Diagnostics")

	paths, ok := doc["paths"].(map[string]any)
	if !ok {
		t.Fatal("paths object is missing")
	}
	assertEventStreamSchemaRef(
		t,
		pathOperation(t, paths, "/events", "get"),
		"#/components/schemas/FactoryEvent",
	)
}

func TestOpenAPIAuthoring_EventSchemasUseDedicatedFragments(t *testing.T) {
	data, err := os.ReadFile("../../api/openapi-main.yaml")
	if err != nil {
		t.Fatalf("read authored openapi contract: %v", err)
	}

	var doc map[string]any
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parse authored openapi contract: %v", err)
	}

	schemas := componentSchemas(t, doc)
	expectedRefs := map[string]string{
		"FactoryEvent":                          "./components/schemas/events/FactoryEvent.yaml",
		"FactoryEventType":                      "./components/schemas/events/FactoryEventType.yaml",
		"FactoryEventContext":                   "./components/schemas/events/FactoryEventContext.yaml",
		"DispatchConsumedWorkRef":               "./components/schemas/events/DispatchConsumedWorkRef.yaml",
		"DispatchRequestEventMetadata":          "./components/schemas/events/DispatchRequestEventMetadata.yaml",
		"RunRequestEventPayload":                "./components/schemas/events/payloads/RunRequestEventPayload.yaml",
		"InitialStructureRequestEventPayload":   "./components/schemas/events/payloads/InitialStructureRequestEventPayload.yaml",
		"WorkRequestEventPayload":               "./components/schemas/events/payloads/WorkRequestEventPayload.yaml",
		"RelationshipChangeRequestEventPayload": "./components/schemas/events/payloads/RelationshipChangeRequestEventPayload.yaml",
		"DispatchRequestEventPayload":           "./components/schemas/events/payloads/DispatchRequestEventPayload.yaml",
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
	assertEventStreamSchemaRef(
		t,
		pathOperation(t, paths, "/events", "get"),
		"#/components/schemas/FactoryEvent",
	)
}

func TestOpenAPIAuthoring_FactoryWorldSchemasUseDedicatedFragments(t *testing.T) {
	data, err := os.ReadFile("../../api/openapi-main.yaml")
	if err != nil {
		t.Fatalf("read authored openapi contract: %v", err)
	}

	var doc map[string]any
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parse authored openapi contract: %v", err)
	}

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
	data, err := os.ReadFile("../../api/openapi-main.yaml")
	if err != nil {
		t.Fatalf("read authored openapi contract: %v", err)
	}

	var doc map[string]any
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parse authored openapi contract: %v", err)
	}

	schemas := componentSchemas(t, doc)
	expectedRefs := map[string]string{
		"SubmitWorkRequest":         "./components/schemas/api/SubmitWorkRequest.yaml",
		"SubmitRelation":            "./components/schemas/api/SubmitRelation.yaml",
		"SubmitWorkResponse":        "./components/schemas/api/SubmitWorkResponse.yaml",
		"UpsertWorkRequestResponse": "./components/schemas/api/UpsertWorkRequestResponse.yaml",
		"ListWorkResponse":          "./components/schemas/api/ListWorkResponse.yaml",
		"PaginationContext":         "./components/schemas/api/PaginationContext.yaml",
		"TokenResponse":             "./components/schemas/api/TokenResponse.yaml",
		"TokenHistory":              "./components/schemas/api/TokenHistory.yaml",
		"StatusCategories":          "./components/schemas/api/StatusCategories.yaml",
		"StatusResponse":            "./components/schemas/api/StatusResponse.yaml",
		"ErrorFamily":               "./components/schemas/api/ErrorFamily.yaml",
		"ErrorResponse":             "./components/schemas/api/ErrorResponse.yaml",
		"WorkRequest":               "./components/schemas/api/WorkRequest.yaml",
		"WorkRequestType":           "./components/schemas/api/WorkRequestType.yaml",
	}
	for schemaName, wantRef := range expectedRefs {
		assertSchemaRef(t, schemas, schemaName, wantRef)
	}
}

func TestOpenAPIAuthoring_DataModelSchemasUseDedicatedFragments(t *testing.T) {
	data, err := os.ReadFile("../../api/openapi-main.yaml")
	if err != nil {
		t.Fatalf("read authored openapi contract: %v", err)
	}

	var doc map[string]any
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parse authored openapi contract: %v", err)
	}

	schemas := componentSchemas(t, doc)
	expectedRefs := map[string]string{
		"FactoryName":                 "./components/schemas/data-models/FactoryName.yaml",
		"NamedFactory":                "./components/schemas/data-models/NamedFactory.yaml",
		"Factory":                     "./components/schemas/data-models/Factory.yaml",
		"ResourceManifest":            "./components/schemas/data-models/ResourceManifest.yaml",
		"RequiredTool":                "./components/schemas/data-models/RequiredTool.yaml",
		"BundledFile":                 "./components/schemas/data-models/BundledFile.yaml",
		"BundledFileContent":          "./components/schemas/data-models/BundledFileContent.yaml",
		"InputType":                   "./components/schemas/data-models/InputType.yaml",
		"InputKind":                   "./components/schemas/data-models/InputKind.yaml",
		"WorkType":                    "./components/schemas/data-models/WorkType.yaml",
		"WorkState":                   "./components/schemas/data-models/WorkState.yaml",
		"WorkStateType":               "./components/schemas/data-models/WorkStateType.yaml",
		"Resource":                    "./components/schemas/data-models/Resource.yaml",
		"Worker":                      "./components/schemas/data-models/Worker.yaml",
		"WorkerType":                  "./components/schemas/data-models/WorkerType.yaml",
		"WorkerModelProvider":         "./components/schemas/data-models/WorkerModelProvider.yaml",
		"WorkerProvider":              "./components/schemas/data-models/WorkerProvider.yaml",
		"Workstation":                 "./components/schemas/data-models/Workstation.yaml",
		"WorkstationLimits":           "./components/schemas/data-models/WorkstationLimits.yaml",
		"WorkstationKind":             "./components/schemas/data-models/WorkstationKind.yaml",
		"WorkstationType":             "./components/schemas/data-models/WorkstationType.yaml",
		"WorkstationCron":             "./components/schemas/data-models/WorkstationCron.yaml",
		"WorkstationGuardType":        "./components/schemas/data-models/WorkstationGuardType.yaml",
		"WorkstationGuard":            "./components/schemas/data-models/WorkstationGuard.yaml",
		"WorkstationGuardMatchConfig": "./components/schemas/data-models/WorkstationGuardMatchConfig.yaml",
		"WorkstationIO":               "./components/schemas/data-models/WorkstationIO.yaml",
		"InputGuard":                  "./components/schemas/data-models/InputGuard.yaml",
		"InputGuardType":              "./components/schemas/data-models/InputGuardType.yaml",
		"Transition":                  "./components/schemas/data-models/Transition.yaml",
		"Work":                        "./components/schemas/data-models/Work.yaml",
		"Relation":                    "./components/schemas/data-models/Relation.yaml",
		"RelationType":                "./components/schemas/data-models/RelationType.yaml",
	}
	for schemaName, wantRef := range expectedRefs {
		assertSchemaRef(t, schemas, schemaName, wantRef)
	}
}

// portos:func-length-exception owner=agent-factory reason=unified-event-schema-contract review=2026-07-18 removal=split-event-context-payload-and-dispatch-contract-assertions-before-next-event-schema-change
func TestOpenAPIContract_DefinesUnifiedFactoryEventLog(t *testing.T) {
	data, err := os.ReadFile("../../api/openapi.yaml")
	if err != nil {
		t.Fatalf("read openapi contract: %v", err)
	}

	var doc map[string]any
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parse openapi contract: %v", err)
	}

	schemas := componentSchemas(t, doc)
	for _, schema := range []string{
		"FactoryEvent",
		"FactoryEventContext",
		"FactoryEventType",
		"DispatchConsumedWorkRef",
		"DispatchRequestEventMetadata",
		"RunRequestEventPayload",
		"InitialStructureRequestEventPayload",
		"WorkRequestEventPayload",
		"RelationshipChangeRequestEventPayload",
		"DispatchRequestEventPayload",
		"InferenceRequestEventPayload",
		"InferenceResponseEventPayload",
		"ScriptRequestEventPayload",
		"ScriptResponseEventPayload",
		"InferenceOutcome",
		"ScriptExecutionOutcome",
		"ScriptFailureType",
		"DispatchResponseEventPayload",
		"FactoryStateResponseEventPayload",
		"RunResponseEventPayload",
	} {
		if _, ok := schemas[schema]; !ok {
			t.Fatalf("components.schemas.%s is missing", schema)
		}
	}
	legacyGeneratedConfigSchema := strings.Join([]string{"Effective", "Config"}, "")
	for _, legacySchema := range []string{
		"FactoryWorkItem",
		"FactoryRelation",
		"RecordedWorkRequest",
		"RecordedSubmission",
		"RecordedDispatch",
		"RecordedCompletion",
		legacyGeneratedConfigSchema,
	} {
		if _, ok := schemas[legacySchema]; ok {
			t.Fatalf("components.schemas.%s must not be introduced beside generated FactoryEvent", legacySchema)
		}
	}

	factoryEvent := schemaObject(t, schemas, "FactoryEvent")
	assertRequiredFields(t, factoryEvent, "schemaVersion", "id", "type", "context", "payload")
	factoryEventProperties := schemaProperties(t, factoryEvent, "FactoryEvent")
	assertPropertyRef(t, factoryEventProperties, "type", "#/components/schemas/FactoryEventType")
	assertPropertyRef(t, factoryEventProperties, "context", "#/components/schemas/FactoryEventContext")
	assertPayloadUnionRefs(t, factoryEventProperties, []string{
		"#/components/schemas/RunRequestEventPayload",
		"#/components/schemas/InitialStructureRequestEventPayload",
		"#/components/schemas/WorkRequestEventPayload",
		"#/components/schemas/RelationshipChangeRequestEventPayload",
		"#/components/schemas/DispatchRequestEventPayload",
		"#/components/schemas/InferenceRequestEventPayload",
		"#/components/schemas/InferenceResponseEventPayload",
		"#/components/schemas/ScriptRequestEventPayload",
		"#/components/schemas/ScriptResponseEventPayload",
		"#/components/schemas/DispatchResponseEventPayload",
		"#/components/schemas/FactoryStateResponseEventPayload",
		"#/components/schemas/RunResponseEventPayload",
	})

	eventType := schemaObject(t, schemas, "FactoryEventType")
	assertEnumValues(t, eventType, "FactoryEventType", canonicalFactoryEventTypeValues)
	assertEnumOmitValues(t, eventType, "FactoryEventType", retiredFactoryEventTypeValues)

	context := schemaObject(t, schemas, "FactoryEventContext")
	assertRequiredFields(t, context, "sequence", "tick", "eventTime")
	contextProperties := schemaProperties(t, context, "FactoryEventContext")
	for _, field := range []string{"eventTime", "requestId", "traceIds", "workIds", "dispatchId"} {
		if _, ok := contextProperties[field]; !ok {
			t.Fatalf("FactoryEventContext.properties.%s is missing", field)
		}
	}
	for _, snakeCaseField := range []string{"event_time", "request_id", "trace_ids", "work_ids", "dispatch_id"} {
		if _, ok := contextProperties[snakeCaseField]; ok {
			t.Fatalf("FactoryEventContext.properties.%s must use camelCase", snakeCaseField)
		}
	}

	initialStructure := schemaObject(t, schemas, "InitialStructureRequestEventPayload")
	assertPropertyRef(t, schemaProperties(t, initialStructure, "InitialStructureRequestEventPayload"), "factory", "#/components/schemas/Factory")

	runRequest := schemaObject(t, schemas, "RunRequestEventPayload")
	assertRequiredFields(t, runRequest, "recordedAt", "factory")
	runRequestProperties := schemaProperties(t, runRequest, "RunRequestEventPayload")
	assertPropertyRef(t, runRequestProperties, "factory", "#/components/schemas/Factory")
	if _, ok := runRequestProperties[strings.Join([]string{"effective", "Config"}, "")]; ok {
		t.Fatalf("RunRequestEventPayload.properties must not expose legacy config")
	}

	factory := schemaObject(t, schemas, "Factory")
	factoryProperties := schemaProperties(t, factory, "Factory")
	for _, field := range []string{"factoryDir", "sourceDirectory", "workflowId", "metadata", "inputTypes", "workTypes"} {
		if _, ok := factoryProperties[field]; !ok {
			t.Fatalf("Factory.properties.%s is missing", field)
		}
	}
	for _, retiredField := range []string{"factory_dir", "source_directory", "workflow_id", "input_types", "work_types", "exhaustion_rules"} {
		if _, ok := factoryProperties[retiredField]; ok {
			t.Fatalf("Factory.properties.%s must not be advertised", retiredField)
		}
	}
	if _, ok := factoryProperties["exhaustionRules"]; ok {
		t.Fatal("Factory.properties.exhaustionRules must not be advertised")
	}
	assertArrayItemRef(t, factoryProperties, "workers", "#/components/schemas/Worker")
	assertArrayItemRef(t, factoryProperties, "workstations", "#/components/schemas/Workstation")

	workRequest := schemaObject(t, schemas, "WorkRequestEventPayload")
	workRequestProperties := schemaProperties(t, workRequest, "WorkRequestEventPayload")
	assertPropertyRef(t, workRequestProperties, "type", "#/components/schemas/WorkRequestType")
	assertArrayItemRef(t, workRequestProperties, "works", "#/components/schemas/Work")
	assertArrayItemRef(t, workRequestProperties, "relations", "#/components/schemas/Relation")
	for _, field := range []string{"source", "parentLineage"} {
		if _, ok := workRequestProperties[field]; !ok {
			t.Fatalf("WorkRequestEventPayload.properties.%s is missing", field)
		}
	}
	for _, misplacedIdentityField := range []string{"request", "requestId", "traceIds", "workIds", "dispatchId"} {
		if _, ok := workRequestProperties[misplacedIdentityField]; ok {
			t.Fatalf("WorkRequestEventPayload.properties.%s must be carried through FactoryEvent.context", misplacedIdentityField)
		}
	}

	relationshipChange := schemaObject(t, schemas, "RelationshipChangeRequestEventPayload")
	assertPropertyRef(t, schemaProperties(t, relationshipChange, "RelationshipChangeRequestEventPayload"), "relation", "#/components/schemas/Relation")

	dispatchRequest := schemaObject(t, schemas, "DispatchRequestEventPayload")
	dispatchRequestProperties := schemaProperties(t, dispatchRequest, "DispatchRequestEventPayload")
	for _, field := range []string{"currentChainingTraceId", "previousChainingTraceIds"} {
		if _, ok := dispatchRequestProperties[field]; !ok {
			t.Fatalf("DispatchRequestEventPayload.properties.%s is missing", field)
		}
	}
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

	inferenceRequest := schemaObject(t, schemas, "InferenceRequestEventPayload")
	assertRequiredFields(t, inferenceRequest, "inferenceRequestId", "attempt", "workingDirectory", "worktree", "prompt")
	inferenceRequestProperties := schemaProperties(t, inferenceRequest, "InferenceRequestEventPayload")
	for _, field := range []string{"inferenceRequestId", "attempt", "workingDirectory", "worktree", "prompt"} {
		if _, ok := inferenceRequestProperties[field]; !ok {
			t.Fatalf("InferenceRequestEventPayload.properties.%s is missing", field)
		}
	}
	assertPropertiesAbsent(t, inferenceRequestProperties, "InferenceRequestEventPayload", "dispatchId", "transitionId")

	inferenceResponse := schemaObject(t, schemas, "InferenceResponseEventPayload")
	assertRequiredFields(t, inferenceResponse, "inferenceRequestId", "attempt", "outcome", "durationMillis")
	inferenceResponseProperties := schemaProperties(t, inferenceResponse, "InferenceResponseEventPayload")
	assertPropertyRef(t, inferenceResponseProperties, "outcome", "#/components/schemas/InferenceOutcome")
	for _, field := range []string{"inferenceRequestId", "attempt", "response", "durationMillis", "providerSession", "diagnostics", "exitCode", "errorClass"} {
		if _, ok := inferenceResponseProperties[field]; !ok {
			t.Fatalf("InferenceResponseEventPayload.properties.%s is missing", field)
		}
	}
	assertPropertiesAbsent(t, inferenceResponseProperties, "InferenceResponseEventPayload", "dispatchId", "transitionId")
	assertEnumValues(t, schemaObject(t, schemas, "InferenceOutcome"), "InferenceOutcome", []string{"SUCCEEDED", "FAILED"})

	scriptRequest := schemaObject(t, schemas, "ScriptRequestEventPayload")
	assertRequiredFields(t, scriptRequest, "scriptRequestId", "dispatchId", "transitionId", "attempt", "command", "args")
	scriptRequestProperties := schemaProperties(t, scriptRequest, "ScriptRequestEventPayload")
	for _, field := range []string{"scriptRequestId", "dispatchId", "transitionId", "attempt", "command", "args"} {
		if _, ok := scriptRequestProperties[field]; !ok {
			t.Fatalf("ScriptRequestEventPayload.properties.%s is missing", field)
		}
	}
	for _, hiddenField := range []string{"stdin", "env"} {
		if _, ok := scriptRequestProperties[hiddenField]; ok {
			t.Fatalf("ScriptRequestEventPayload.properties.%s must not be advertised", hiddenField)
		}
	}
	scriptRequestDescription, _ := scriptRequest["description"].(string)
	if normalized := strings.ToLower(scriptRequestDescription); !strings.Contains(normalized, "stdin") || !strings.Contains(normalized, "environment") {
		t.Fatalf("ScriptRequestEventPayload.description must document excluded stdin and environment data, got %q", scriptRequestDescription)
	}

	scriptResponse := schemaObject(t, schemas, "ScriptResponseEventPayload")
	assertRequiredFields(t, scriptResponse, "scriptRequestId", "dispatchId", "transitionId", "attempt", "outcome", "stdout", "stderr", "durationMillis")
	scriptResponseProperties := schemaProperties(t, scriptResponse, "ScriptResponseEventPayload")
	assertPropertyRef(t, scriptResponseProperties, "outcome", "#/components/schemas/ScriptExecutionOutcome")
	assertPropertyRef(t, scriptResponseProperties, "failureType", "#/components/schemas/ScriptFailureType")
	for _, field := range []string{"scriptRequestId", "dispatchId", "transitionId", "attempt", "stdout", "stderr", "durationMillis", "exitCode"} {
		if _, ok := scriptResponseProperties[field]; !ok {
			t.Fatalf("ScriptResponseEventPayload.properties.%s is missing", field)
		}
	}
	for _, hiddenField := range []string{"stdin", "env"} {
		if _, ok := scriptResponseProperties[hiddenField]; ok {
			t.Fatalf("ScriptResponseEventPayload.properties.%s must not be advertised", hiddenField)
		}
	}
	scriptResponseDescription, _ := scriptResponse["description"].(string)
	if normalized := strings.ToLower(scriptResponseDescription); !strings.Contains(normalized, "stdin") || !strings.Contains(normalized, "environment") {
		t.Fatalf("ScriptResponseEventPayload.description must document excluded stdin and environment data, got %q", scriptResponseDescription)
	}
	assertEnumValues(t, schemaObject(t, schemas, "ScriptExecutionOutcome"), "ScriptExecutionOutcome", []string{"SUCCEEDED", "FAILED_EXIT_CODE", "TIMED_OUT", "PROCESS_ERROR"})
	assertEnumValues(t, schemaObject(t, schemas, "ScriptFailureType"), "ScriptFailureType", []string{"TIMEOUT", "PROCESS_ERROR"})

	dispatchResponse := schemaObject(t, schemas, "DispatchResponseEventPayload")
	dispatchResponseProperties := schemaProperties(t, dispatchResponse, "DispatchResponseEventPayload")
	for _, field := range []string{"currentChainingTraceId", "previousChainingTraceIds"} {
		if _, ok := dispatchResponseProperties[field]; !ok {
			t.Fatalf("DispatchResponseEventPayload.properties.%s is missing", field)
		}
	}
	assertArrayItemRef(t, dispatchResponseProperties, "outputWork", "#/components/schemas/Work")
	assertArrayItemRef(t, dispatchResponseProperties, "outputResources", "#/components/schemas/Resource")
	assertPropertiesAbsent(t, dispatchResponseProperties, "DispatchResponseEventPayload", "dispatchId", "workstation", "worker", "inputs", "providerSession", "diagnostics")
	assertNoDispatchConfigCopies(t, dispatchResponseProperties, "DispatchResponseEventPayload")

	stateResponse := schemaObject(t, schemas, "FactoryStateResponseEventPayload")
	assertPropertyRef(t, schemaProperties(t, stateResponse, "FactoryStateResponseEventPayload"), "state", "#/components/schemas/FactoryState")
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

func loadValidatedOpenAPIContract(t *testing.T) *openapi3.T {
	t.Helper()

	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromFile("../../api/openapi.yaml")
	if err != nil {
		t.Fatalf("load openapi contract: %v", err)
	}
	if err := doc.Validate(context.Background()); err != nil {
		t.Fatalf("validate openapi contract: %v", err)
	}
	return doc
}

func loadCanonicalFactoryEventVocabularyFixture(t *testing.T) []map[string]any {
	t.Helper()

	fixtureBytes, err := os.ReadFile("testdata/canonical-event-vocabulary-stream.json")
	if err != nil {
		t.Fatalf("read canonical event vocabulary fixture: %v", err)
	}
	assertJSONStringLiteralMissing(t, string(fixtureBytes), retiredFactoryEventTypeValues...)

	var fixture []map[string]any
	if err := json.Unmarshal(fixtureBytes, &fixture); err != nil {
		t.Fatalf("parse canonical event vocabulary fixture: %v", err)
	}
	if len(fixture) != len(canonicalFactoryEventTypeValues) {
		t.Fatalf("canonical event vocabulary fixture length = %d, want %d", len(fixture), len(canonicalFactoryEventTypeValues))
	}
	return fixture
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

func assertCanonicalFactoryEventFixtureOwnership(
	t *testing.T,
	eventType string,
	contextMap map[string]any,
	payloadMap map[string]any,
) {
	t.Helper()

	switch eventType {
	case "DISPATCH_REQUEST":
		assertJSONKeysAbsent(t, payloadMap, "canonical dispatch request payload", "dispatchId", "workstation", "worker")
		assertJSONKeysPresent(t, contextMap, "canonical dispatch request context", "dispatchId", "requestId")
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
		assertJSONKeysPresent(t, contextMap, "canonical dispatch response context", "dispatchId")
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

func TestOpenAPIContract_GeneratedModelsOmitLegacyConfig(t *testing.T) {
	data, err := os.ReadFile("generated/server.gen.go")
	if err != nil {
		t.Fatalf("read generated server models: %v", err)
	}
	legacyGeneratedConfigType := "type " + strings.Join([]string{"Effective", "Config"}, "") + " struct"
	if strings.Contains(string(data), legacyGeneratedConfigType) {
		t.Fatal("generated OpenAPI models must not contain legacy config structs")
	}
}

// portos:func-length-exception owner=agent-factory reason=schema-validation-fixture review=2026-07-20 removal=extract-run-request-payload-builder-before-next-run-request-contract-change
func TestOpenAPIContract_RunRequestPayloadValidatesFactoryConfig(t *testing.T) {
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromFile("../../api/openapi.yaml")
	if err != nil {
		t.Fatalf("load openapi contract: %v", err)
	}
	if err := doc.Validate(context.Background()); err != nil {
		t.Fatalf("validate openapi contract: %v", err)
	}
	schema := doc.Components.Schemas["RunRequestEventPayload"].Value
	validPayload := map[string]any{
		"recordedAt": "2026-04-10T12:00:00Z",
		"factory": map[string]any{
			"factoryDir":      "/tmp/runtime-factory",
			"sourceDirectory": "/tmp/customer-factory",
			"workflowId":      "workflow-123",
			"metadata":        map[string]any{"factory_hash": "sha256:test"},
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
				"modelProvider":    "claude",
				"executorProvider": "script_wrap",
				"stopToken":        "<COMPLETE>",
				"skipPermissions":  true,
				"command":          "echo",
			}},
			"workstations": []any{map[string]any{
				"name":           "execute",
				"worker":         "executor",
				"kind":           "STANDARD",
				"type":           "MODEL_WORKSTATION",
				"promptFile":     "prompt.md",
				"promptTemplate": "Finish {{ .WorkID }}",
				"outputSchema":   "{\"type\":\"object\"}",
				"inputs": []any{
					map[string]any{"workType": "story", "state": "init"},
				},
				"outputs": []any{
					map[string]any{"workType": "story", "state": "complete"},
				},
				"onRejection": map[string]any{"workType": "story", "state": "init"},
				"onFailure":   map[string]any{"workType": "story", "state": "failed"},
				"resources": []any{
					map[string]any{"name": "agent-slot", "capacity": 1},
				},
				"limits": map[string]any{
					"maxRetries":       2,
					"maxExecutionTime": "2m",
				},
				"cron": map[string]any{
					"schedule":       "*/5 * * * *",
					"triggerAtStart": true,
					"expiryWindow":   "30s",
				},
				"guards": []any{
					map[string]any{"type": "VISIT_COUNT", "workstation": "execute", "maxVisits": 3},
				},
				"stopWords":        []any{"DONE", "RETRY"},
				"workingDirectory": "/tmp/worktree",
				"env":              map[string]any{"TEAM": "factory"},
			}},
		},
	}
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

func TestOpenAPIContract_FactoryExampleUsesGuardedLoopBreaker(t *testing.T) {
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromFile("../../api/openapi.yaml")
	if err != nil {
		t.Fatalf("load openapi contract: %v", err)
	}
	if err := doc.Validate(context.Background()); err != nil {
		t.Fatalf("validate openapi contract: %v", err)
	}

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
		if !ok {
			continue
		}
		if workstation["type"] != "LOGICAL_MOVE" {
			continue
		}
		guards, ok := workstation["guards"].([]any)
		if !ok || len(guards) == 0 {
			continue
		}
		guard, ok := guards[0].(map[string]any)
		if !ok {
			continue
		}
		if guard["type"] == "VISIT_COUNT" {
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

func TestOpenAPIContract_WorkerSchemaAndGeneratedModelRetireLegacyFields(t *testing.T) {
	data, err := os.ReadFile("../../api/openapi.yaml")
	if err != nil {
		t.Fatalf("read openapi contract: %v", err)
	}

	var doc map[string]any
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parse openapi contract: %v", err)
	}

	workerSchema := schemaObject(t, componentSchemas(t, doc), "Worker")
	workerProperties := schemaProperties(t, workerSchema, "Worker")
	for _, field := range []string{"name", "modelProvider", "executorProvider"} {
		if _, ok := workerProperties[field].(map[string]any); !ok {
			t.Fatalf("Worker.properties.%s is missing", field)
		}
	}
	for _, retired := range []string{"provider", "sessionId", "concurrency"} {
		if _, ok := workerProperties[retired]; ok {
			t.Fatalf("Worker.properties.%s must not be advertised", retired)
		}
	}
	modelProviderProperty, ok := workerProperties["modelProvider"].(map[string]any)
	if !ok {
		t.Fatal("Worker.properties.modelProvider must be an object")
	}
	modelProviderDescription, _ := modelProviderProperty["description"].(string)
	if !strings.Contains(modelProviderDescription, "claude") || !strings.Contains(modelProviderDescription, "codex") {
		t.Fatalf("Worker.properties.modelProvider.description must document built-in values, got %q", modelProviderDescription)
	}
	executorProviderProperty, ok := workerProperties["executorProvider"].(map[string]any)
	if !ok {
		t.Fatal("Worker.properties.executorProvider must be an object")
	}
	executorProviderDescription, _ := executorProviderProperty["description"].(string)
	if !strings.Contains(executorProviderDescription, "script_wrap") {
		t.Fatalf("Worker.properties.executorProvider.description must document the public built-in value, got %q", executorProviderDescription)
	}

	workerType := reflect.TypeOf(generated.Worker{})
	for _, field := range []string{"ExecutorProvider", "ModelProvider"} {
		if _, ok := workerType.FieldByName(field); !ok {
			t.Fatalf("generated.Worker must expose %s", field)
		}
	}
	for _, retired := range []string{"Provider", "SessionId", "Concurrency"} {
		if _, ok := workerType.FieldByName(retired); ok {
			t.Fatalf("generated.Worker must not expose %s", retired)
		}
	}

	executorProvider := generated.WorkerProviderScriptWrap
	modelProvider := generated.WorkerModelProviderClaude
	payloadBytes, err := json.Marshal(generated.Worker{
		Name:             "executor",
		ExecutorProvider: &executorProvider,
		ModelProvider:    &modelProvider,
	})
	if err != nil {
		t.Fatalf("marshal generated.Worker: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		t.Fatalf("unmarshal generated.Worker payload: %v", err)
	}
	if payload["executorProvider"] != string(executorProvider) || payload["modelProvider"] != string(modelProvider) {
		t.Fatalf("generated.Worker payload = %#v, want canonical provider fields", payload)
	}
	for _, retired := range []string{"provider", "sessionId", "concurrency"} {
		if _, ok := payload[retired]; ok {
			t.Fatalf("generated.Worker payload must not include retired %q field: %#v", retired, payload)
		}
	}
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
			Outputs: []generated.WorkstationIO{{WorkType: "story", State: "failed"}},
			Guards: &[]generated.WorkstationGuard{{
				Type:        generated.WorkstationGuardTypeVisitCount,
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
	if guard["type"] != string(generated.WorkstationGuardTypeVisitCount) {
		t.Fatalf("generated.Factory workstation guard type = %#v, want %q", guard["type"], generated.WorkstationGuardTypeVisitCount)
	}
	if guard["workstation"] != "review-story" {
		t.Fatalf("generated.Factory workstation guard workstation = %#v, want %q", guard["workstation"], "review-story")
	}
	if got, ok := guard["maxVisits"].(float64); !ok || int(got) != 3 {
		t.Fatalf("generated.Factory workstation guard maxVisits = %#v, want %d", guard["maxVisits"], 3)
	}
}

func assertSchemaNamesPresent(t *testing.T, schemas map[string]any, names []string) {
	t.Helper()

	for _, name := range names {
		if _, ok := schemas[name]; !ok {
			t.Fatalf("components.schemas.%s is missing", name)
		}
	}
}

func containsString(values []any, want string) bool {
	for _, value := range values {
		if got, ok := value.(string); ok && got == want {
			return true
		}
	}
	return false
}

func requireOpenAPI3ComponentSchema(t *testing.T, doc *openapi3.T, schemaName string) *openapi3.Schema {
	t.Helper()

	schemaRef, ok := doc.Components.Schemas[schemaName]
	if !ok || schemaRef == nil || schemaRef.Value == nil {
		t.Fatalf("components.schemas.%s is missing", schemaName)
	}
	return schemaRef.Value
}

func assertOpenAPI3Description(t *testing.T, path string, description string) {
	t.Helper()

	if strings.TrimSpace(description) == "" {
		t.Fatalf("%s description is empty", path)
	}
}

func assertOpenAPI3PropertyDescription(t *testing.T, schema *openapi3.Schema, schemaName string, propertyName string) *openapi3.Schema {
	t.Helper()

	property, ok := schema.Properties[propertyName]
	if !ok || property == nil || property.Value == nil {
		t.Fatalf("%s.properties.%s is missing", schemaName, propertyName)
	}
	assertOpenAPI3Description(t, schemaName+".properties."+propertyName, property.Value.Description)
	return property.Value
}

func assertOpenAPI3ArrayPropertyDescription(t *testing.T, schema *openapi3.Schema, schemaName string, propertyName string) *openapi3.Schema {
	t.Helper()

	property := assertOpenAPI3PropertyDescription(t, schema, schemaName, propertyName)
	if property.Items == nil || property.Items.Value == nil {
		t.Fatalf("%s.properties.%s.items is missing", schemaName, propertyName)
	}
	return property.Items.Value
}

func assertOpenAPI3RefPropertyDescription(t *testing.T, schema *openapi3.Schema, schemaName string, propertyName string) *openapi3.Schema {
	t.Helper()

	return assertOpenAPI3PropertyDescription(t, schema, schemaName, propertyName)
}

func assertOpenAPI3PropertyRef(t *testing.T, schema *openapi3.Schema, schemaName string, propertyName string, wantRef string) {
	t.Helper()

	property, ok := schema.Properties[propertyName]
	if !ok || property == nil {
		t.Fatalf("%s.properties.%s is missing", schemaName, propertyName)
	}
	if property.Ref != wantRef {
		t.Fatalf("%s.properties.%s.$ref = %q, want %q", schemaName, propertyName, property.Ref, wantRef)
	}
}

func assertRequiredStringValues(t *testing.T, got []string, want ...string) {
	t.Helper()

	for _, wantValue := range want {
		found := false
		for _, gotValue := range got {
			if gotValue == wantValue {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("required fields are missing %q", wantValue)
		}
	}
}

func componentSchemas(t *testing.T, doc map[string]any) map[string]any {
	t.Helper()

	components, ok := doc["components"].(map[string]any)
	if !ok {
		t.Fatalf("components object is missing")
	}
	schemas, ok := components["schemas"].(map[string]any)
	if !ok {
		t.Fatalf("components.schemas object is missing")
	}
	return schemas
}

func schemaObject(t *testing.T, schemas map[string]any, schemaName string) map[string]any {
	t.Helper()

	schema, ok := schemas[schemaName].(map[string]any)
	if !ok {
		t.Fatalf("components.schemas.%s must be an object schema", schemaName)
	}
	return schema
}

func schemaProperties(t *testing.T, schema map[string]any, schemaName string) map[string]any {
	t.Helper()

	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("%s.properties is missing", schemaName)
	}
	return properties
}

func assertSchemaPropertiesPresent(t *testing.T, properties map[string]any, schemaName string, fields ...string) {
	t.Helper()

	for _, field := range fields {
		if _, ok := properties[field].(map[string]any); !ok {
			t.Fatalf("%s.properties.%s is missing", schemaName, field)
		}
	}
}

func assertRequiredFields(t *testing.T, schema map[string]any, fields ...string) {
	t.Helper()

	requiredFields, ok := schema["required"].([]any)
	if !ok {
		t.Fatalf("schema.required is missing")
	}
	for _, field := range fields {
		if !containsString(requiredFields, field) {
			t.Fatalf("schema.required is missing %q", field)
		}
	}
}

func assertEnumValues(t *testing.T, schema map[string]any, schemaName string, values []string) {
	t.Helper()

	enumValues, ok := schema["enum"].([]any)
	if !ok {
		t.Fatalf("%s.enum is missing", schemaName)
	}
	if len(enumValues) != len(values) {
		t.Fatalf("%s.enum has %d values, want %d", schemaName, len(enumValues), len(values))
	}
	for _, value := range values {
		if !containsString(enumValues, value) {
			t.Fatalf("%s.enum is missing %q", schemaName, value)
		}
	}
}

func assertEnumOmitValues(t *testing.T, schema map[string]any, schemaName string, values []string) {
	t.Helper()

	enumValues, ok := schema["enum"].([]any)
	if !ok {
		t.Fatalf("%s.enum is missing", schemaName)
	}
	for _, value := range values {
		if containsString(enumValues, value) {
			t.Fatalf("%s.enum must not contain %q", schemaName, value)
		}
	}
}

func assertPayloadUnionRefs(t *testing.T, properties map[string]any, wantRefs []string) {
	t.Helper()

	payload, ok := properties["payload"].(map[string]any)
	if !ok {
		t.Fatalf("FactoryEvent.properties.payload is missing")
	}
	oneOf, ok := payload["oneOf"].([]any)
	if !ok {
		t.Fatalf("FactoryEvent.properties.payload.oneOf is missing")
	}
	if len(oneOf) != len(wantRefs) {
		t.Fatalf("FactoryEvent payload union has %d refs, want %d", len(oneOf), len(wantRefs))
	}
	for _, wantRef := range wantRefs {
		found := false
		for _, item := range oneOf {
			refObject, ok := item.(map[string]any)
			if ok && refObject["$ref"] == wantRef {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("FactoryEvent payload union is missing %s", wantRef)
		}
	}
}

func assertPropertyRef(t *testing.T, properties map[string]any, propertyName string, wantRef string) {
	t.Helper()

	property, ok := properties[propertyName].(map[string]any)
	if !ok {
		t.Fatalf("properties.%s is missing", propertyName)
	}
	if got, ok := property["$ref"].(string); !ok || got != wantRef {
		t.Fatalf("properties.%s.$ref = %v, want %s", propertyName, property["$ref"], wantRef)
	}
}

func assertSchemaRef(t *testing.T, schemas map[string]any, schemaName string, wantRef string) {
	t.Helper()

	schema, ok := schemas[schemaName].(map[string]any)
	if !ok {
		t.Fatalf("components.schemas.%s must be an object schema", schemaName)
	}
	if got, ok := schema["$ref"].(string); !ok || got != wantRef {
		t.Fatalf("components.schemas.%s.$ref = %v, want %s", schemaName, schema["$ref"], wantRef)
	}
}

func assertArrayItemRef(t *testing.T, properties map[string]any, propertyName string, wantRef string) {
	t.Helper()

	property, ok := properties[propertyName].(map[string]any)
	if !ok {
		t.Fatalf("properties.%s is missing", propertyName)
	}
	items, ok := property["items"].(map[string]any)
	if !ok {
		t.Fatalf("properties.%s.items is missing", propertyName)
	}
	if got, ok := items["$ref"].(string); !ok || got != wantRef {
		t.Fatalf("properties.%s.items.$ref = %v, want %s", propertyName, items["$ref"], wantRef)
	}
}

func assertStringArrayProperty(t *testing.T, properties map[string]any, propertyName string) {
	t.Helper()

	property, ok := properties[propertyName].(map[string]any)
	if !ok {
		t.Fatalf("properties.%s is missing", propertyName)
	}
	items, ok := property["items"].(map[string]any)
	if !ok {
		t.Fatalf("properties.%s.items is missing", propertyName)
	}
	if got, ok := items["type"].(string); !ok || got != "string" {
		t.Fatalf("properties.%s.items.type = %v, want string", propertyName, items["type"])
	}
}

func assertProjectionSchemasPresent(t *testing.T, schemas map[string]any) {
	t.Helper()

	for _, schema := range []string{
		"FactoryWorldWorkstationRequestProjectionSlice",
		"FactoryWorldWorkstationRequestView",
		"FactoryWorldWorkstationRequestCountView",
		"FactoryWorldWorkstationRequestRequestView",
		"FactoryWorldWorkstationRequestResponseView",
		"FactoryWorldWorkItemRef",
		"FactoryWorldTokenView",
		"FactoryWorldMutationView",
		"FactoryWorldWorkDiagnostics",
		"FactoryWorldProviderDiagnostic",
		"FactoryWorldRenderedPromptDiagnostic",
	} {
		if _, ok := schemas[schema]; !ok {
			t.Fatalf("components.schemas.%s is missing", schema)
		}
	}
}

func assertWorkstationRequestProjectionSliceSchema(t *testing.T, schemas map[string]any) {
	t.Helper()

	projectionSlice := schemaObject(t, schemas, "FactoryWorldWorkstationRequestProjectionSlice")
	sliceProperties := schemaProperties(t, projectionSlice, "FactoryWorldWorkstationRequestProjectionSlice")
	workstationRequestsByDispatchID, ok := sliceProperties["workstationRequestsByDispatchId"].(map[string]any)
	if !ok {
		t.Fatalf("FactoryWorldWorkstationRequestProjectionSlice.properties.workstationRequestsByDispatchId is missing")
	}
	additionalProperties, ok := workstationRequestsByDispatchID["additionalProperties"].(map[string]any)
	if !ok {
		t.Fatalf("FactoryWorldWorkstationRequestProjectionSlice.properties.workstationRequestsByDispatchId.additionalProperties is missing")
	}
	if got, ok := additionalProperties["$ref"].(string); !ok || got != "#/components/schemas/FactoryWorldWorkstationRequestView" {
		t.Fatalf("FactoryWorldWorkstationRequestProjectionSlice workstation request map ref = %v, want %s", additionalProperties["$ref"], "#/components/schemas/FactoryWorldWorkstationRequestView")
	}
}

func assertWorkstationRequestViewSchema(t *testing.T, schemas map[string]any) {
	t.Helper()

	requestView := schemaObject(t, schemas, "FactoryWorldWorkstationRequestView")
	assertRequiredFields(t, requestView, "dispatchId", "transitionId", "counts", "request")
	requestViewProperties := schemaProperties(t, requestView, "FactoryWorldWorkstationRequestView")
	assertPropertyRef(t, requestViewProperties, "counts", "#/components/schemas/FactoryWorldWorkstationRequestCountView")
	assertPropertyRef(t, requestViewProperties, "request", "#/components/schemas/FactoryWorldWorkstationRequestRequestView")
	assertPropertyRef(t, requestViewProperties, "response", "#/components/schemas/FactoryWorldWorkstationRequestResponseView")

	countView := schemaObject(t, schemas, "FactoryWorldWorkstationRequestCountView")
	assertRequiredFields(t, countView, "dispatchedCount", "respondedCount", "erroredCount")
}

func assertWorkstationRequestRequestSchema(t *testing.T, schemas map[string]any) {
	t.Helper()

	requestPayload := schemaObject(t, schemas, "FactoryWorldWorkstationRequestRequestView")
	requestPayloadProperties := schemaProperties(t, requestPayload, "FactoryWorldWorkstationRequestRequestView")
	for _, field := range []string{
		"startedAt",
		"requestTime",
		"prompt",
		"workingDirectory",
		"worktree",
		"provider",
		"model",
		"currentChainingTraceId",
	} {
		if _, ok := requestPayloadProperties[field].(map[string]any); !ok {
			t.Fatalf("FactoryWorldWorkstationRequestRequestView.properties.%s is missing", field)
		}
	}
	assertStringArrayProperty(t, requestPayloadProperties, "previousChainingTraceIds")
	assertArrayItemRef(t, requestPayloadProperties, "inputWorkItems", "#/components/schemas/FactoryWorldWorkItemRef")
	assertArrayItemRef(t, requestPayloadProperties, "consumedTokens", "#/components/schemas/FactoryWorldTokenView")
	assertPropertyRef(t, requestPayloadProperties, "requestMetadata", "#/components/schemas/StringMap")
}

func assertWorkstationRequestWorkRefSchemas(t *testing.T, schemas map[string]any) {
	t.Helper()

	workItemRef := schemaObject(t, schemas, "FactoryWorldWorkItemRef")
	workItemRefProperties := schemaProperties(t, workItemRef, "FactoryWorldWorkItemRef")
	for _, field := range []string{"workId", "workTypeId", "displayName", "traceId", "currentChainingTraceId"} {
		if _, ok := workItemRefProperties[field].(map[string]any); !ok {
			t.Fatalf("FactoryWorldWorkItemRef.properties.%s is missing", field)
		}
	}
	assertStringArrayProperty(t, workItemRefProperties, "previousChainingTraceIds")

	tokenView := schemaObject(t, schemas, "FactoryWorldTokenView")
	tokenViewProperties := schemaProperties(t, tokenView, "FactoryWorldTokenView")
	for _, field := range []string{"tokenId", "placeId", "workId", "workTypeId", "traceId", "currentChainingTraceId"} {
		if _, ok := tokenViewProperties[field].(map[string]any); !ok {
			t.Fatalf("FactoryWorldTokenView.properties.%s is missing", field)
		}
	}
	assertStringArrayProperty(t, tokenViewProperties, "previousChainingTraceIds")
}

func assertWorkstationRequestResponseSchema(t *testing.T, schemas map[string]any) {
	t.Helper()

	responsePayload := schemaObject(t, schemas, "FactoryWorldWorkstationRequestResponseView")
	responsePayloadProperties := schemaProperties(t, responsePayload, "FactoryWorldWorkstationRequestResponseView")
	assertPropertyRef(t, responsePayloadProperties, "providerSession", "#/components/schemas/ProviderSessionMetadata")
	assertPropertyRef(t, responsePayloadProperties, "diagnostics", "#/components/schemas/FactoryWorldWorkDiagnostics")
	assertPropertyRef(t, responsePayloadProperties, "responseMetadata", "#/components/schemas/StringMap")
	assertArrayItemRef(t, responsePayloadProperties, "outputWorkItems", "#/components/schemas/FactoryWorldWorkItemRef")
	assertArrayItemRef(t, responsePayloadProperties, "outputMutations", "#/components/schemas/FactoryWorldMutationView")
	for _, field := range []string{
		"outcome",
		"feedback",
		"failureReason",
		"failureMessage",
		"responseText",
		"errorClass",
		"endTime",
		"durationMillis",
	} {
		if _, ok := responsePayloadProperties[field].(map[string]any); !ok {
			t.Fatalf("FactoryWorldWorkstationRequestResponseView.properties.%s is missing", field)
		}
	}
}

func assertJSONStringLiteralMissing(t *testing.T, haystack string, needles ...string) {
	t.Helper()

	for _, needle := range needles {
		if strings.Contains(haystack, `"`+needle+`"`) {
			t.Fatalf("unexpected retired string %q found in fixture", needle)
		}
	}
}

func assertStringSetsMatch(t *testing.T, got []string, want []string) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("string set length = %d, want %d", len(got), len(want))
	}

	gotCounts := make(map[string]int, len(got))
	for _, value := range got {
		gotCounts[value]++
	}
	for _, value := range want {
		if gotCounts[value] == 0 {
			t.Fatalf("string set is missing %q", value)
		}
		gotCounts[value]--
	}
	for value, remaining := range gotCounts {
		if remaining != 0 {
			t.Fatalf("string set contains unexpected count for %q: %d", value, remaining)
		}
	}
}

func assertNoDispatchConfigCopies(t *testing.T, properties map[string]any, schemaName string) {
	t.Helper()

	for _, field := range []string{
		"model",
		"provider",
		"promptFile",
		"promptTemplate",
		"outputSchema",
		"worktree",
		"workingDirectory",
		"workerType",
		"workstationName",
		"workstationType",
	} {
		if _, ok := properties[field]; ok {
			t.Fatalf("%s.properties.%s duplicates Worker or Workstation configuration", schemaName, field)
		}
	}
}

func pathOperation(t *testing.T, paths map[string]any, path string, method string) map[string]any {
	t.Helper()

	pathItem, ok := paths[path].(map[string]any)
	if !ok {
		t.Fatalf("paths.%s is missing", path)
	}
	operation, ok := pathItem[method].(map[string]any)
	if !ok {
		t.Fatalf("paths.%s.%s is missing", path, method)
	}
	return operation
}

func assertEventStreamSchemaRef(t *testing.T, operation map[string]any, wantRef string) {
	t.Helper()

	responses, ok := operation["responses"].(map[string]any)
	if !ok {
		t.Fatalf("operation.responses is missing")
	}
	response, ok := responses["200"].(map[string]any)
	if !ok {
		t.Fatal("operation.responses.200 is missing")
	}
	content, ok := response["content"].(map[string]any)
	if !ok {
		t.Fatal("operation.responses.200.content is missing")
	}
	eventStream, ok := content["text/event-stream"].(map[string]any)
	if !ok {
		t.Fatal("operation.responses.200.content.text/event-stream is missing")
	}
	xEventSchema, ok := eventStream["x-event-schema"].(string)
	if !ok {
		t.Fatal("operation.responses.200.content.text/event-stream.x-event-schema is missing")
	}
	if xEventSchema != wantRef {
		t.Fatalf("operation.responses.200.content.text/event-stream.x-event-schema = %q, want %s", xEventSchema, wantRef)
	}
}

func assertResponseSchemaRef(t *testing.T, operation map[string]any, status string, wantRef string) {
	t.Helper()

	responses, ok := operation["responses"].(map[string]any)
	if !ok {
		t.Fatalf("operation.responses is missing")
	}
	response, ok := responses[status].(map[string]any)
	if !ok {
		t.Fatalf("operation.responses.%s is missing", status)
	}
	content, ok := response["content"].(map[string]any)
	if !ok {
		t.Fatalf("operation.responses.%s.content is missing", status)
	}
	applicationJSON, ok := content["application/json"].(map[string]any)
	if !ok {
		t.Fatalf("operation.responses.%s.content.application/json is missing", status)
	}
	schema, ok := applicationJSON["schema"].(map[string]any)
	if !ok {
		t.Fatalf("operation.responses.%s.content.application/json.schema is missing", status)
	}
	if got, ok := schema["$ref"].(string); !ok || got != wantRef {
		t.Fatalf("operation.responses.%s.content.application/json.schema.$ref = %v, want %s", status, schema["$ref"], wantRef)
	}
}

func assertResponseRef(t *testing.T, operation map[string]any, status string, wantRef string) {
	t.Helper()

	responses, ok := operation["responses"].(map[string]any)
	if !ok {
		t.Fatalf("operation.responses is missing")
	}
	response, ok := responses[status].(map[string]any)
	if !ok {
		t.Fatalf("operation.responses.%s is missing", status)
	}
	if got, ok := response["$ref"].(string); !ok || got != wantRef {
		t.Fatalf("operation.responses.%s.$ref = %v, want %s", status, response["$ref"], wantRef)
	}
}

func assertResponseExampleCodeFamilies(t *testing.T, responses map[string]any, responseName string, wantCodeFamilies map[string]string) {
	t.Helper()

	response, ok := responses[responseName].(map[string]any)
	if !ok {
		t.Fatalf("components.responses.%s is missing", responseName)
	}
	content, ok := response["content"].(map[string]any)
	if !ok {
		t.Fatalf("components.responses.%s.content is missing", responseName)
	}
	applicationJSON, ok := content["application/json"].(map[string]any)
	if !ok {
		t.Fatalf("components.responses.%s.content.application/json is missing", responseName)
	}
	examples, ok := applicationJSON["examples"].(map[string]any)
	if !ok {
		t.Fatalf("components.responses.%s.content.application/json.examples is missing", responseName)
	}

	seenCodeFamilies := make(map[string]string, len(examples))
	for exampleName, value := range examples {
		example, ok := value.(map[string]any)
		if !ok {
			t.Fatalf("components.responses.%s example %s must be an object", responseName, exampleName)
		}
		payload, ok := example["value"].(map[string]any)
		if !ok {
			t.Fatalf("components.responses.%s example %s value must be an object", responseName, exampleName)
		}
		code, ok := payload["code"].(string)
		if !ok {
			t.Fatalf("components.responses.%s example %s code must be a string", responseName, exampleName)
		}
		family, ok := payload["family"].(string)
		if !ok {
			t.Fatalf("components.responses.%s example %s family must be a string", responseName, exampleName)
		}
		seenCodeFamilies[code] = family
	}

	if len(seenCodeFamilies) != len(wantCodeFamilies) {
		t.Fatalf("components.responses.%s example count = %d, want %d", responseName, len(seenCodeFamilies), len(wantCodeFamilies))
	}
	for code, wantFamily := range wantCodeFamilies {
		if gotFamily, ok := seenCodeFamilies[code]; !ok {
			t.Fatalf("components.responses.%s is missing example for code %q", responseName, code)
		} else if gotFamily != wantFamily {
			t.Fatalf("components.responses.%s example for code %q family = %q, want %q", responseName, code, gotFamily, wantFamily)
		}
	}
}

func assertPropertiesAbsent(t *testing.T, properties map[string]any, schemaName string, fields ...string) {
	t.Helper()

	for _, field := range fields {
		if _, ok := properties[field]; ok {
			t.Fatalf("%s.properties.%s must not be advertised", schemaName, field)
		}
	}
}

func assertJSONKeysAbsent(t *testing.T, object map[string]any, name string, keys ...string) {
	t.Helper()

	for _, key := range keys {
		if _, ok := object[key]; ok {
			t.Fatalf("%s.%s must not be present", name, key)
		}
	}
}

func assertJSONKeysPresent(t *testing.T, object map[string]any, name string, keys ...string) {
	t.Helper()

	for _, key := range keys {
		if _, ok := object[key]; !ok {
			t.Fatalf("%s.%s is missing", name, key)
		}
	}
}
