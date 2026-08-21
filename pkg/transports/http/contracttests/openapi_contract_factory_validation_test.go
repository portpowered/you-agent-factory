package apicontract_test

import (
	"testing"
)

func TestOpenAPIContract_WorkerSessionObservationPublishesOptionalResolvedFacts(t *testing.T) {
	doc := loadBundledOpenAPIDocument(t)
	schemas := componentSchemas(t, doc)
	observation := schemaObject(t, schemas, "WorkerSessionObservation")
	properties := schemaProperties(t, observation, "WorkerSessionObservation")

	assertSchemaPropertiesPresent(t, properties, "WorkerSessionObservation", "model", "reasoningEffort")
	required, ok := observation["required"].([]any)
	if !ok {
		t.Fatalf("WorkerSessionObservation.required is missing")
	}
	for _, field := range []string{"model", "reasoningEffort"} {
		if containsString(required, field) {
			t.Fatalf("WorkerSessionObservation.%s must remain optional", field)
		}
		property := properties[field].(map[string]any)
		if property["type"] != "string" {
			t.Fatalf("WorkerSessionObservation.%s type = %#v, want string", field, property["type"])
		}
		if description, ok := property["description"].(string); !ok || description == "" {
			t.Fatalf("WorkerSessionObservation.%s description is missing", field)
		}
	}

	listResponse := schemaObject(t, schemas, "ListWorkerSessionsResponse")
	listProperties := schemaProperties(t, listResponse, "ListWorkerSessionsResponse")
	sessions := listProperties["sessions"].(map[string]any)
	items := sessions["items"].(map[string]any)
	if items["$ref"] != "#/components/schemas/WorkerSessionObservation" {
		t.Fatalf("ListWorkerSessionsResponse.sessions.items = %#v, want WorkerSessionObservation", items["$ref"])
	}
}

func assertHumanApprovalSurfaceSchemas(t *testing.T, schemas map[string]any, paths map[string]any) {
	t.Helper()
	listOperation := pathOperation(t, paths, "/factory-sessions/{session_id}/approvals", "get")
	if got := listOperation["operationId"]; got != "listHumanApprovalsBySessionId" {
		t.Fatalf("human approval list operationId = %v, want listHumanApprovalsBySessionId", got)
	}
	listParameters, ok := listOperation["parameters"].([]any)
	if !ok {
		t.Fatalf("human approval list parameters are missing")
	}
	assertParameterRef(t, listParameters, "#/components/parameters/SessionID")
	assertParameterRef(t, listParameters, "#/components/parameters/HumanApprovalStatus")
	assertResponseSchemaRef(t, listOperation, "200", "#/components/schemas/ListHumanApprovalsResponse")
	assertResponseRef(t, listOperation, "400", "#/components/responses/BadRequest")
	assertResponseRef(t, listOperation, "404", "#/components/responses/NotFound")
	assertResponseRef(t, listOperation, "500", "#/components/responses/InternalError")

	showOperation := pathOperation(t, paths, "/factory-sessions/{session_id}/approvals/{approval_id}", "get")
	if got := showOperation["operationId"]; got != "getHumanApprovalBySessionId" {
		t.Fatalf("human approval show operationId = %v, want getHumanApprovalBySessionId", got)
	}
	showParameters, ok := showOperation["parameters"].([]any)
	if !ok {
		t.Fatalf("human approval show parameters are missing")
	}
	assertParameterRef(t, showParameters, "#/components/parameters/SessionID")
	assertParameterRef(t, showParameters, "#/components/parameters/HumanApprovalID")
	assertResponseSchemaRef(t, showOperation, "200", "#/components/schemas/HumanApproval")
	assertResponseRef(t, showOperation, "404", "#/components/responses/NotFound")
	assertResponseRef(t, showOperation, "500", "#/components/responses/InternalError")

	approval := schemaObject(t, schemas, "HumanApproval")
	assertRequiredFields(t, approval, "approvalId", "sessionId", "dispatchId", "workstationId", "workstationName", "decisions", "status", "workIds")
	approvalProperties := schemaProperties(t, approval, "HumanApproval")
	assertStringArrayProperty(t, approvalProperties, "workIds")
	decisions, ok := approvalProperties["decisions"].(map[string]any)
	if !ok {
		t.Fatalf("HumanApproval.properties.decisions is missing")
	}
	decisionItems, ok := decisions["items"].(map[string]any)
	if !ok {
		t.Fatalf("HumanApproval.properties.decisions.items is missing")
	}
	assertEnumValues(t, decisionItems, "HumanApproval.decisions", []string{"APPROVE", "REJECT"})
	status, ok := approvalProperties["status"].(map[string]any)
	if !ok {
		t.Fatalf("HumanApproval.properties.status is missing")
	}
	assertEnumValues(t, status, "HumanApproval.status", []string{"PENDING"})

	eventPayload := schemaObject(t, schemas, "HumanApprovalRequestedEventPayload")
	assertRequiredFields(t, eventPayload, "approvalId", "workstationId", "decisions", "status")
	eventPayloadProperties := schemaProperties(t, eventPayload, "HumanApprovalRequestedEventPayload")
	eventDecisions, ok := eventPayloadProperties["decisions"].(map[string]any)
	if !ok {
		t.Fatalf("HumanApprovalRequestedEventPayload.properties.decisions is missing")
	}
	eventDecisionItems, ok := eventDecisions["items"].(map[string]any)
	if !ok {
		t.Fatalf("HumanApprovalRequestedEventPayload.properties.decisions.items is missing")
	}
	assertEnumValues(t, eventDecisionItems, "HumanApprovalRequestedEventPayload.decisions", []string{"APPROVE", "REJECT"})
	eventStatus, ok := eventPayloadProperties["status"].(map[string]any)
	if !ok {
		t.Fatalf("HumanApprovalRequestedEventPayload.properties.status is missing")
	}
	assertEnumValues(t, eventStatus, "HumanApprovalRequestedEventPayload.status", []string{"PENDING"})

	runtimeProperties := schemaProperties(t, schemaObject(t, schemas, "FactorySessionRuntime"), "FactorySessionRuntime")
	assertArrayItemRef(t, runtimeProperties, "pendingHumanApprovals", "#/components/schemas/HumanApproval")
	workProperties := schemaProperties(t, schemaObject(t, schemas, "Work"), "Work")
	assertPropertyRef(t, workProperties, "humanApproval", "#/components/schemas/HumanApproval")
}

func TestOpenAPIContract_DefinesFactoryValidationEndpoint(t *testing.T) {
	doc := loadBundledOpenAPIDocument(t)
	paths, ok := doc["paths"].(map[string]any)
	if !ok {
		t.Fatalf("paths object is missing")
	}
	if _, exists := paths["/workflow-previews"]; exists {
		t.Fatal("removed /workflow-previews alias must not be published")
	}

	pathItem, ok := paths["/factory-validations"].(map[string]any)
	if !ok {
		t.Fatal("paths./factory-validations is missing")
	}
	postOperation, ok := pathItem["post"].(map[string]any)
	if !ok {
		t.Fatal("paths./factory-validations.post is missing")
	}
	if got, _ := postOperation["operationId"].(string); got != "validateFactory" {
		t.Fatalf("paths./factory-validations.post.operationId = %q, want validateFactory", got)
	}
	assertRequestSchemaRef(t, postOperation, "#/components/schemas/Factory")
	assertResponseSchemaRef(t, postOperation, "200", "#/components/schemas/FactoryValidationResult")
	assertResponseRef(t, postOperation, "400", "#/components/responses/BadRequest")
	for _, removed := range []string{"WorkflowPreviewRequest", "WorkflowPreviewResult"} {
		if _, exists := loadBundledOpenAPIComponentSchemas(t)[removed]; exists {
			t.Fatalf("removed schema %s must not be published", removed)
		}
	}
}

func TestOpenAPIContract_FactoryValidationResultSchemaMatchesCanonicalTargetShape(t *testing.T) {
	schemas := loadBundledOpenAPIComponentSchemas(t)
	doc := loadValidatedOpenAPIContract(t)

	resultSchema := schemaObject(t, schemas, "FactoryValidationResult")
	assertRequiredFields(t, resultSchema, "targets")
	resultProperties := schemaProperties(t, resultSchema, "FactoryValidationResult")
	assertArrayItemRef(t, resultProperties, "targets", "#/components/schemas/FactoryValidationTarget")

	targetSchema := schemaObject(t, schemas, "FactoryValidationTarget")
	assertRequiredFields(t, targetSchema, "code", "severity", "message", "subject")
	targetProperties := schemaProperties(t, targetSchema, "FactoryValidationTarget")
	assertPropertyRef(t, targetProperties, "severity", "#/components/schemas/FactoryValidationSeverity")
	assertPropertyRef(t, targetProperties, "subject", "#/components/schemas/FactoryValidationSubject")

	subjectSchema := schemaObject(t, schemas, "FactoryValidationSubject")
	assertRequiredFields(t, subjectSchema, "type", "id", "location")
	subjectProperties := schemaProperties(t, subjectSchema, "FactoryValidationSubject")
	assertPropertyRef(t, subjectProperties, "type", "#/components/schemas/FactoryValidationSubjectType")
	assertPropertyRef(t, subjectProperties, "location", "#/components/schemas/FactoryValidationSubjectLocation")

	assertEnumValues(t, schemaObject(t, schemas, "FactoryValidationSeverity"), "FactoryValidationSeverity", []string{"error", "warning", "hint"})
	assertEnumValues(t, schemaObject(t, schemas, "FactoryValidationSubjectType"), "FactoryValidationSubjectType", []string{
		"FACTORY", "WORKSTATION", "WORK_TYPE", "WORK_STATE", "WORKER", "RESOURCE", "ROUTE",
	})
	assertEnumValues(t, schemaObject(t, schemas, "FactoryValidationSubjectLocation"), "FactoryValidationSubjectLocation", []string{
		"ON_REJECTION", "ON_FAILURE", "OUTPUTS", "INPUTS", "STATES", "TERMINAL", "REFERENCE", "DEFINITION",
	})

	resultOpenAPI := requireOpenAPI3ComponentSchema(t, doc, "FactoryValidationResult")
	if resultOpenAPI.Example == nil {
		t.Fatal("FactoryValidationResult.example is missing")
	}
	if err := resultOpenAPI.VisitJSON(resultOpenAPI.Example); err != nil {
		t.Fatalf("FactoryValidationResult.example should validate: %v", err)
	}
	exampleTargets, ok := resultOpenAPI.Example.(map[string]any)["targets"].([]any)
	if !ok || len(exampleTargets) < 2 {
		t.Fatalf("FactoryValidationResult.example.targets = %#v, want multi-target invalid factory example", resultOpenAPI.Example)
	}
}

func TestOpenAPIContract_DefinesFactoryPreviewEndpoint(t *testing.T) {
	doc := loadBundledOpenAPIDocument(t)
	paths, ok := doc["paths"].(map[string]any)
	if !ok {
		t.Fatalf("paths object is missing")
	}

	pathItem, ok := paths["/factories/preview"].(map[string]any)
	if !ok {
		t.Fatal("paths./factories/preview is missing")
	}
	postOperation, ok := pathItem["post"].(map[string]any)
	if !ok {
		t.Fatal("paths./factories/preview.post is missing")
	}
	if got, _ := postOperation["operationId"].(string); got != "previewFactory" {
		t.Fatalf("paths./factories/preview.post.operationId = %q, want previewFactory", got)
	}
	if deprecated, ok := postOperation["deprecated"].(bool); ok && deprecated {
		t.Fatal("paths./factories/preview.post must not be deprecated")
	}
	tags, ok := postOperation["tags"].([]any)
	if !ok || len(tags) == 0 {
		t.Fatal("paths./factories/preview.post.tags is missing")
	}
	if got, _ := tags[0].(string); got != "Factory" {
		t.Fatalf("paths./factories/preview.post.tags[0] = %q, want Factory", got)
	}
	assertRequestSchemaRef(t, postOperation, "#/components/schemas/FactoryPreviewRequest")
	assertResponseSchemaRef(t, postOperation, "200", "#/components/schemas/FactoryPreviewResult")
	assertResponseRef(t, postOperation, "400", "#/components/responses/BadRequest")
}

func TestOpenAPIContract_FactoryPreviewRequestSchemaMatchesSharedContract(t *testing.T) {
	schemas := loadBundledOpenAPIComponentSchemas(t)
	requestSchema := schemaObject(t, schemas, "FactoryPreviewRequest")
	if deprecated, ok := requestSchema["deprecated"].(bool); ok && deprecated {
		t.Fatal("FactoryPreviewRequest must not be deprecated")
	}
	assertRequiredFields(t, requestSchema, "sourceKind")
	requestProperties := schemaProperties(t, requestSchema, "FactoryPreviewRequest")
	sourceKind, ok := requestProperties["sourceKind"].(map[string]any)
	if !ok {
		t.Fatal("FactoryPreviewRequest.properties.sourceKind is missing")
	}
	assertEnumValues(t, sourceKind, "FactoryPreviewRequest.properties.sourceKind", []string{
		"FACTORY_ID",
		"FACTORY_INLINE",
		"WORKFLOW_FILE",
		"WORKFLOW_NAME",
		"INLINE_WORKFLOW",
	})
}

func TestOpenAPIContract_FactoryPreviewResultSchemaMatchesSharedContract(t *testing.T) {
	schemas := loadBundledOpenAPIComponentSchemas(t)
	resultSchema := schemaObject(t, schemas, "FactoryPreviewResult")
	if deprecated, ok := resultSchema["deprecated"].(bool); ok && deprecated {
		t.Fatal("FactoryPreviewResult must not be deprecated")
	}
	assertRequiredFields(t, resultSchema,
		"valid",
		"sourceResolution",
		"sourceValidationIssues",
		"policyPreview",
		"resultConstraints",
	)
	resultProperties := schemaProperties(t, resultSchema, "FactoryPreviewResult")
	assertPropertyRef(t, resultProperties, "sourceResolution", "#/components/schemas/WorkflowSourceResolution")
	assertPropertyRef(t, resultProperties, "policyPreview", "#/components/schemas/WorkflowPolicyPreview")
	assertPropertyRef(t, resultProperties, "resultConstraints", "#/components/schemas/WorkflowResultConstraints")
}
