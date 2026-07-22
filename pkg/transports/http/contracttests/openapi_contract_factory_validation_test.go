package apicontract_test

import (
	"testing"
)

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
