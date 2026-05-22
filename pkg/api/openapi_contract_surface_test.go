package api

import (
	"context"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

func TestOpenAPIContract_ContainsCoveredJSONOperations(t *testing.T) {
	doc := loadBundledOpenAPIDocument(t)
	paths, ok := doc["paths"].(map[string]any)
	if !ok {
		t.Fatalf("paths object is missing")
	}
	schemas := componentSchemas(t, doc)

	assertPublishedOperations(t, paths)
	assertRemovedPaths(t, paths)
	assertPublishedSurfaceSchemas(t, schemas)
	assertSubmitWorkSurfaceSchemas(t, schemas)
	assertWorkRequestSurfaceSchemas(t, schemas)
	assertWorkContentSurfaceSchemas(t, schemas)
	assertWorkstationSurfaceSchemas(t, schemas)
	assertErrorSurfaceSchemas(t, schemas)
}

func TestOpenAPIContract_WorkstationCronIsScheduleOnly(t *testing.T) {
	doc := loadBundledOpenAPIDocument(t)
	cronSchema := schemaObject(t, componentSchemas(t, doc), "WorkstationCron")
	assertRequiredFields(t, cronSchema, "schedule")
	properties := schemaProperties(t, cronSchema, "WorkstationCron")
	assertSchemaPropertiesPresent(t, properties, "WorkstationCron", "schedule", "triggerAtStart", "jitter", "expiryWindow")
	assertPropertiesAbsent(t, properties, "WorkstationCron", "trigger_at_start", "expiry_window", "interval")
}

func TestOpenAPIContract_DefaultLocalServerURL(t *testing.T) {
	doc := loadValidatedOpenAPIContract(t)
	if len(doc.Servers) != 1 {
		t.Fatalf("openapi contract servers = %d, want 1", len(doc.Servers))
	}
	if got := doc.Servers[0].URL; got != "http://localhost:7437" {
		t.Fatalf("openapi contract default local server url = %q, want %q", got, "http://localhost:7437")
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
	assertOpenAPI3PropertyDescription(t, factory, "Factory", "id")
	workType := assertOpenAPI3ArrayPropertyDescription(t, factory, "Factory", "workTypes")
	resource := assertOpenAPI3ArrayPropertyDescription(t, factory, "Factory", "resources")
	worker := assertOpenAPI3ArrayPropertyDescription(t, factory, "Factory", "workers")
	workstation := assertOpenAPI3ArrayPropertyDescription(t, factory, "Factory", "workstations")

	assertFactorySchemaDescriptions(t, workType, resource, worker, workstation)
}

func TestOpenAPIContract_FactorySchemaPublishesCanonicalNameField(t *testing.T) {
	doc := loadValidatedOpenAPIContract(t)
	factorySchema := requireOpenAPI3ComponentSchema(t, doc, "Factory")
	assertOpenAPI3Description(t, "Factory", factorySchema.Description)
	assertRequiredStringValues(t, factorySchema.Required, "name")
	assertOpenAPI3PropertyRef(t, factorySchema, "Factory", "name", "#/components/schemas/FactoryName")
	if factorySchema.Example == nil {
		t.Fatal("Factory.example is missing")
	}
	if err := factorySchema.VisitJSON(factorySchema.Example); err != nil {
		t.Fatalf("Factory.example should validate: %v", err)
	}
}

func TestOpenAPIContract_FactoryOperationsPublishMachineReadableErrors(t *testing.T) {
	doc := loadBundledOpenAPIDocument(t)
	paths, ok := doc["paths"].(map[string]any)
	if !ok {
		t.Fatalf("paths object is missing")
	}
	components, ok := doc["components"].(map[string]any)
	if !ok {
		t.Fatalf("components object is missing")
	}
	responses, ok := components["responses"].(map[string]any)
	if !ok {
		t.Fatalf("components.responses object is missing")
	}

	assertFactoryOperationResponses(t, paths)
	assertFactoryResponseExamples(t, responses)
}

func TestOpenAPIContract_DefinesWorkstationRequestProjectionSlice(t *testing.T) {
	schemas := loadBundledOpenAPIComponentSchemas(t)
	assertWorkstationRequestProjectionSchemasPresent(t, schemas)
	assertWorkstationRequestProjectionSliceSchema(t, schemas)
	assertWorkstationRequestViewSchema(t, schemas)
	assertWorkstationRequestPayloadSchemas(t, schemas)
	assertWorkstationRequestScriptBoundarySchemas(t, schemas)
}

func TestOpenAPIContract_ListWorkReturnsStructuredWorkResults(t *testing.T) {
	schemas := loadBundledOpenAPIComponentSchemas(t)
	doc := loadBundledOpenAPIDocument(t)

	listWorkResponse := schemaObject(t, schemas, "ListWorkResponse")
	listWorkProperties := schemaProperties(t, listWorkResponse, "ListWorkResponse")
	assertArrayItemRef(t, listWorkProperties, "results", "#/components/schemas/Work")
	if _, ok := listWorkProperties["paginationContext"].(map[string]any); !ok {
		t.Fatal("ListWorkResponse.properties.paginationContext is missing")
	}

	work := schemaObject(t, schemas, "Work")
	workProperties := schemaProperties(t, work, "Work")
	state, ok := workProperties["state"].(map[string]any)
	if !ok {
		t.Fatal("Work.properties.state is missing")
	}
	if got := state["$ref"]; got != "#/components/schemas/WorkState" {
		t.Fatalf("Work.properties.state.$ref = %v, want #/components/schemas/WorkState", got)
	}

	listWork := doc["paths"].(map[string]any)["/work"].(map[string]any)["get"].(map[string]any)
	parameters, ok := listWork["parameters"].([]any)
	if !ok {
		t.Fatal("paths./work.get.parameters is missing")
	}
	assertParameterRef(t, parameters, "#/components/parameters/StateName")
	assertParameterRef(t, parameters, "#/components/parameters/StateType")
	assertParameterRef(t, parameters, "#/components/parameters/SortBy")
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

func assertPublishedOperations(t *testing.T, paths map[string]any) {
	t.Helper()
	requiredOperations := map[string][]string{
		"/work":                       {"get", "post"},
		"/work-requests/{request_id}": {"put"},
		"/work/{id}":                  {"get"},
		"/events":                     {"get"},
		"/status":                     {"get"},
		"/provider-sessions/detail":   {"get"},
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
}

func assertRemovedPaths(t *testing.T, paths map[string]any) {
	t.Helper()
	for _, path := range []string{
		"/dashboard",
		"/dashboard/stream",
		"/state",
		"/traces/{id}",
		"/traces/{trace_id}",
		"/work/{id}/trace",
		"/workflows",
		"/workflows/{workflow_id}",
	} {
		if _, ok := paths[path]; ok {
			t.Fatalf("paths.%s must not be published for removed factory endpoints", path)
		}
	}
}

func assertPublishedSurfaceSchemas(t *testing.T, schemas map[string]any) {
	t.Helper()
	for _, schema := range []string{
		"SubmitWorkRequest", "SubmitWorkResponse", "UpsertWorkRequestResponse", "WorkRequest", "Work", "WorkContent",
		"WorkContentPart", "WorkContentPartType", "WorkTextContentPart", "WorkImageContentPart", "Relation", "ListWorkResponse",
		"TokenResponse", "ErrorFamily", "ErrorResponse", "FactoryName", "StatusCategories", "StatusResponse", "Factory", "Workstation", "WorkstationKind",
	} {
		if _, ok := schemas[schema]; !ok {
			t.Fatalf("components.schemas.%s is missing", schema)
		}
	}
	for _, schema := range []string{
		"DashboardResponse", "DashboardRuntime", "DashboardTopology", "ListWorkflowsResponse", "StateResponse", "TraceResponse", "WorkflowResponse",
	} {
		if _, ok := schemas[schema]; ok {
			t.Fatalf("components.schemas.%s must not be published for removed factory endpoints", schema)
		}
	}
}

func assertSubmitWorkSurfaceSchemas(t *testing.T, schemas map[string]any) {
	t.Helper()
	submitWorkRequestSchema, ok := schemas["SubmitWorkRequest"].(map[string]any)
	if !ok {
		t.Fatalf("components.schemas.SubmitWorkRequest must be an object schema")
	}
	submitWorkRequestRequired, ok := submitWorkRequestSchema["required"].([]any)
	if !ok {
		t.Fatalf("components.schemas.SubmitWorkRequest.required is missing")
	}
	if !containsString(submitWorkRequestRequired, "name") || !containsString(submitWorkRequestRequired, "workTypeName") {
		t.Fatalf("components.schemas.SubmitWorkRequest.required must include name and workTypeName")
	}
	submitWorkRequestProperties := schemaProperties(t, submitWorkRequestSchema, "SubmitWorkRequest")
	assertSchemaPropertiesPresent(t, submitWorkRequestProperties, "SubmitWorkRequest", "name", "workTypeName", "currentChainingTraceId")
	assertPropertyRef(t, submitWorkRequestProperties, "content", "#/components/schemas/WorkContent")
	assertArrayItemRef(t, submitWorkRequestProperties, "relations", "#/components/schemas/SubmitRelation")
	assertPropertiesAbsent(t, submitWorkRequestProperties, "SubmitWorkRequest", "work_type_id")

	submitRelationSchema := schemaObject(t, schemas, "SubmitRelation")
	assertRequiredFields(t, submitRelationSchema, "type", "targetWorkId")
	submitRelationProperties := schemaProperties(t, submitRelationSchema, "SubmitRelation")
	assertSchemaPropertiesPresent(t, submitRelationProperties, "SubmitRelation", "type", "targetWorkId")
	assertPropertiesAbsent(t, submitRelationProperties, "SubmitRelation", "sourceWorkName", "targetWorkName")
}

func assertWorkRequestSurfaceSchemas(t *testing.T, schemas map[string]any) {
	t.Helper()
	workRequestSchema := schemaObject(t, schemas, "WorkRequest")
	assertRequiredFields(t, workRequestSchema, "requestId", "type")
	workRequestProperties := schemaProperties(t, workRequestSchema, "WorkRequest")
	assertSchemaPropertiesPresent(t, workRequestProperties, "WorkRequest", "requestId", "currentChainingTraceId", "type")
	workRequestType := schemaObject(t, schemas, "WorkRequestType")
	assertEnumValues(t, workRequestType, "WorkRequestType", []string{"FACTORY_REQUEST_BATCH"})
	workRequestTypeVarNames, ok := workRequestType["x-enum-varnames"].([]any)
	if !ok {
		t.Fatalf("components.schemas.WorkRequestType.x-enum-varnames is missing")
	}
	if containsString(workRequestTypeVarNames, "WorkRequestTypeDefault") {
		t.Fatalf("components.schemas.WorkRequestType must not advertise legacy DEFAULT request type")
	}

	workSchema := schemaObject(t, schemas, "Work")
	workProperties := schemaProperties(t, workSchema, "Work")
	assertSchemaPropertiesPresent(t, workProperties, "Work", "name", "workId", "requestId", "workTypeName", "state", "currentChainingTraceId", "previousChainingTraceIds", "traceId", "content", "payload", "tags", "relations")
	assertPropertyRef(t, workProperties, "content", "#/components/schemas/WorkContent")
	assertArrayItemRef(t, workProperties, "relations", "#/components/schemas/Relation")
	assertPropertiesAbsent(t, workProperties, "Work", "work_type_id", "target_state")
}

func assertWorkContentSurfaceSchemas(t *testing.T, schemas map[string]any) {
	t.Helper()
	workContentSchema := schemaObject(t, schemas, "WorkContent")
	if got, ok := workContentSchema["type"].(string); !ok || got != "array" {
		t.Fatalf("components.schemas.WorkContent.type = %v, want array", workContentSchema["type"])
	}
	workContentItems, ok := workContentSchema["items"].(map[string]any)
	if !ok || workContentItems["$ref"] != "#/components/schemas/WorkContentPart" {
		t.Fatalf("components.schemas.WorkContent.items must reference WorkContentPart")
	}

	workContentPartSchema := schemaObject(t, schemas, "WorkContentPart")
	assertSchemaOneOfRefs(t, workContentPartSchema, "WorkContentPart", []string{
		"#/components/schemas/WorkTextContentPart",
		"#/components/schemas/WorkImageContentPart",
		"#/components/schemas/WorkAudioContentPart",
		"#/components/schemas/WorkJsonContentPart",
		"#/components/schemas/WorkBinaryContentPart",
	})
	assertEnumValues(t, schemaObject(t, schemas, "WorkContentPartType"), "WorkContentPartType", []string{"text", "image", "TEXT", "IMAGE", "AUDIO", "JSON", "BINARY"})

	assertWorkContentPartSchemaVariant(t, schemas, "WorkTextContentPart", "type", "text")
	assertWorkContentPartSchemaVariant(t, schemas, "WorkImageContentPart", "type", "file")
	assertWorkContentPartSchemaVariant(t, schemas, "WorkAudioContentPart", "type", "file")
	assertWorkContentPartSchemaVariant(t, schemas, "WorkJsonContentPart", "type", "json")
	assertWorkContentPartSchemaVariant(t, schemas, "WorkBinaryContentPart", "type", "file")
	assertSchemaPropertiesPresent(t, schemaProperties(t, schemaObject(t, schemas, "WorkContentCommonFields"), "WorkContentCommonFields"), "WorkContentCommonFields", "label", "role", "contentType", "artifactId", "metadata")
}

func assertWorkContentPartSchemaVariant(t *testing.T, schemas map[string]any, schemaName string, requiredFields ...string) {
	t.Helper()

	schema := schemaObject(t, schemas, schemaName)
	allOf, ok := schema["allOf"].([]any)
	if !ok || len(allOf) != 2 {
		t.Fatalf("%s.allOf has %d entries, want 2", schemaName, len(allOf))
	}
	commonRef, ok := allOf[0].(map[string]any)
	if !ok || commonRef["$ref"] != "#/components/schemas/WorkContentCommonFields" {
		t.Fatalf("%s first allOf entry must reference WorkContentCommonFields", schemaName)
	}
	inlineSchema, ok := allOf[1].(map[string]any)
	if !ok {
		t.Fatalf("%s inline allOf schema is missing", schemaName)
	}
	assertRequiredFields(t, inlineSchema, requiredFields...)
	assertSchemaPropertiesPresent(t, schemaProperties(t, inlineSchema, schemaName), schemaName, requiredFields...)
}

func assertWorkstationSurfaceSchemas(t *testing.T, schemas map[string]any) {
	t.Helper()
	workstationSchema := schemaObject(t, schemas, "Workstation")
	workstationProperties := schemaProperties(t, workstationSchema, "Workstation")
	assertPropertyRef(t, workstationProperties, "behavior", "#/components/schemas/WorkstationKind")
	assertPropertyRef(t, workstationProperties, "type", "#/components/schemas/WorkstationType")
	assertPropertiesAbsent(t, workstationProperties, "Workstation", "timeout", "runtime_type")
	assertEnumValues(t, schemaObject(t, schemas, "WorkstationKind"), "WorkstationKind", []string{"STANDARD", "REPEATER", "CRON"})
	assertEnumValues(t, schemaObject(t, schemas, "WorkstationType"), "WorkstationType", []string{"MODEL_WORKSTATION", "LOGICAL_MOVE"})

	factorySchema := schemaObject(t, schemas, "Factory")
	factoryProperties := schemaProperties(t, factorySchema, "Factory")
	assertPropertiesAbsent(t, factoryProperties, "Factory", "exhaustion_rules", "exhaustionRules")
	if _, ok := schemas["ExhaustionRule"]; ok {
		t.Fatalf("components.schemas.ExhaustionRule must not be advertised")
	}
	if description := strings.ToLower(factorySchema["description"].(string)); !strings.Contains(description, "guarded logical_move workstations") {
		t.Fatalf("components.schemas.Factory.description must direct guarded loop breakers to guarded LOGICAL_MOVE workstations")
	}
	guardsProperty, ok := workstationProperties["guards"].(map[string]any)
	if !ok {
		t.Fatalf("components.schemas.Workstation.properties.guards is missing")
	}
	if description := strings.ToLower(guardsProperty["description"].(string)); !strings.Contains(description, "visit_count") {
		t.Fatalf("components.schemas.Workstation.properties.guards must describe visit_count loop-breaker guards")
	}
}

func assertErrorSurfaceSchemas(t *testing.T, schemas map[string]any) {
	t.Helper()
	errorSchema := schemaObject(t, schemas, "ErrorResponse")
	assertRequiredFields(t, errorSchema, "message", "family", "code")
	properties := schemaProperties(t, errorSchema, "ErrorResponse")
	assertPropertyRef(t, properties, "family", "#/components/schemas/ErrorFamily")
	codeProperty, ok := properties["code"].(map[string]any)
	if !ok {
		t.Fatalf("components.schemas.ErrorResponse.properties.code is missing")
	}
	codeEnum, ok := codeProperty["enum"].([]any)
	if !ok {
		t.Fatalf("components.schemas.ErrorResponse.properties.code.enum is missing")
	}
	for _, code := range []string{"BAD_REQUEST", "INVALID_FACTORY_NAME", "FACTORY_ALREADY_EXISTS", "INVALID_FACTORY", "FACTORY_NOT_IDLE", "NOT_FOUND", "INTERNAL_ERROR"} {
		if !containsString(codeEnum, code) {
			t.Fatalf("components.schemas.ErrorResponse.properties.code.enum is missing %q", code)
		}
	}
	assertEnumValues(t, schemaObject(t, schemas, "ErrorFamily"), "ErrorFamily", []string{
		"BAD_REQUEST",
		"CONFLICT",
		"NOT_FOUND",
		"INTERNAL_SERVER_ERROR",
	})
}

func assertFactorySchemaDescriptions(t *testing.T, workType, resource, worker, workstation *openapi3.Schema) {
	t.Helper()
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
	for _, propertyName := range []string{"name", "behavior", "type", "worker", "limits", "resources", "stopWords", "inputs", "outputs", "guards"} {
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
	assertOpenAPI3ArrayPropertyDescription(t, workstationIO, "WorkstationIO", "guards")

	assertOpenAPI3Description(t, "Guard", workstationGuard.Description)
	for _, propertyName := range []string{"type", "workstation", "maxVisits", "matchConfig", "parentInput", "matchInput", "spawnedBy"} {
		assertOpenAPI3PropertyDescription(t, workstationGuard, "Guard", propertyName)
	}
}

func assertFactoryOperationResponses(t *testing.T, paths map[string]any) {
	t.Helper()
	createFactory := pathOperation(t, paths, "/factory", "post")
	assertResponseSchemaRef(t, createFactory, "201", "#/components/schemas/Factory")
	assertResponseRef(t, createFactory, "400", "#/components/responses/CreateFactoryBadRequest")
	assertResponseRef(t, createFactory, "409", "#/components/responses/CreateFactoryConflict")

	currentFactory := pathOperation(t, paths, "/factory/~current", "get")
	assertResponseSchemaRef(t, currentFactory, "200", "#/components/schemas/Factory")
	assertResponseRef(t, currentFactory, "404", "#/components/responses/CurrentFactoryNotFound")

	getEditableFactory := pathOperation(t, paths, "/factory/~current/editable-definition", "get")
	assertResponseSchemaRef(t, getEditableFactory, "200", "#/components/schemas/EditableFactoryDefinition")
	assertResponseRef(t, getEditableFactory, "404", "#/components/responses/CurrentFactoryNotFound")

	saveEditableFactory := pathOperation(t, paths, "/factory/~current/editable-definition", "put")
	assertResponseSchemaRef(t, saveEditableFactory, "200", "#/components/schemas/EditableFactoryDefinition")
	assertResponseRef(t, saveEditableFactory, "400", "#/components/responses/SaveEditableFactoryDefinitionBadRequest")
	assertResponseRef(t, saveEditableFactory, "409", "#/components/responses/SaveEditableFactoryDefinitionConflict")
	assertResponseRef(t, saveEditableFactory, "404", "#/components/responses/CurrentFactoryNotFound")
}

func assertFactoryResponseExamples(t *testing.T, responses map[string]any) {
	t.Helper()
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
	assertResponseExampleCodeFamilies(t, responses, "SaveEditableFactoryDefinitionBadRequest", map[string]string{
		"INVALID_FACTORY": "BAD_REQUEST",
	})
	assertResponseExampleCodeFamilies(t, responses, "SaveEditableFactoryDefinitionConflict", map[string]string{
		"FACTORY_NOT_IDLE": "CONFLICT",
	})
}
