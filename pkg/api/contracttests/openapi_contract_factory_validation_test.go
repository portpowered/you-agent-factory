package apicontract_test

import "testing"

func TestOpenAPIContract_DefinesFactoryValidationEndpoint(t *testing.T) {
	doc := loadBundledOpenAPIDocument(t)
	paths, ok := doc["paths"].(map[string]any)
	if !ok {
		t.Fatalf("paths object is missing")
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
	assertRequestSchemaRef(t, postOperation, "#/components/schemas/FactoryPreviewRequest")
	assertResponseSchemaRef(t, postOperation, "200", "#/components/schemas/FactoryPreviewResult")
	assertResponseRef(t, postOperation, "400", "#/components/responses/BadRequest")
}

func TestOpenAPIContract_DefinesWorkflowPreviewCompatibilityEndpoint(t *testing.T) {
	doc := loadBundledOpenAPIDocument(t)
	paths, ok := doc["paths"].(map[string]any)
	if !ok {
		t.Fatalf("paths object is missing")
	}

	pathItem, ok := paths["/workflow-previews"].(map[string]any)
	if !ok {
		t.Fatal("paths./workflow-previews is missing")
	}
	postOperation, ok := pathItem["post"].(map[string]any)
	if !ok {
		t.Fatal("paths./workflow-previews.post is missing")
	}
	if deprecated, _ := postOperation["deprecated"].(bool); !deprecated {
		t.Fatal("paths./workflow-previews.post.deprecated should be true")
	}
	if got, _ := postOperation["operationId"].(string); got != "previewWorkflow" {
		t.Fatalf("paths./workflow-previews.post.operationId = %q, want previewWorkflow", got)
	}
	assertRequestSchemaRef(t, postOperation, "#/components/schemas/WorkflowPreviewRequest")
	assertResponseSchemaRef(t, postOperation, "200", "#/components/schemas/WorkflowPreviewResult")
	assertResponseRef(t, postOperation, "400", "#/components/responses/BadRequest")
}

func TestOpenAPIContract_FactoryPreviewResultSchemaMatchesSharedContract(t *testing.T) {
	schemas := loadBundledOpenAPIComponentSchemas(t)
	resultSchema := schemaObject(t, schemas, "FactoryPreviewResult")
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

func TestOpenAPIContract_WorkflowPreviewSchemasAliasFactoryPreview(t *testing.T) {
	schemas := loadBundledOpenAPIComponentSchemas(t)
	workflowRequest := schemaObject(t, schemas, "WorkflowPreviewRequest")
	if deprecated, _ := workflowRequest["deprecated"].(bool); !deprecated {
		t.Fatal("WorkflowPreviewRequest should be deprecated")
	}
	workflowResult := schemaObject(t, schemas, "WorkflowPreviewResult")
	if deprecated, _ := workflowResult["deprecated"].(bool); !deprecated {
		t.Fatal("WorkflowPreviewResult should be deprecated")
	}
}
