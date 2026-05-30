package apicontract_test

import "testing"

func TestOpenAPIContract_ErrorResponseTargetsUseCanonicalValidationTargetShape(t *testing.T) {
	schemas := loadBundledOpenAPIComponentSchemas(t)

	errorResponse := schemaObject(t, schemas, "ErrorResponse")
	properties := schemaProperties(t, errorResponse, "ErrorResponse")
	assertArrayItemRef(t, properties, "targets", "#/components/schemas/FactoryValidationTarget")

	targetSchema := schemaObject(t, schemas, "FactoryValidationTarget")
	assertRequiredFields(t, targetSchema, "code", "severity", "message", "subject")
}

func TestOpenAPIContract_FactoryWriteFailureExamplesIncludeCanonicalTargets(t *testing.T) {
	doc := loadBundledOpenAPIDocument(t)
	components, ok := doc["components"].(map[string]any)
	if !ok {
		t.Fatal("components object is missing")
	}
	responses, ok := components["responses"].(map[string]any)
	if !ok {
		t.Fatal("components.responses object is missing")
	}

	assertResponseExampleHasCanonicalWorkstationFailureTarget(t, responses, "SaveCurrentFactoryBadRequest", "invalidFactory")
	assertResponseExampleHasCanonicalWorkstationFailureTarget(t, responses, "CreateFactoryBadRequest", "invalidFactory")
}

func assertResponseExampleHasCanonicalWorkstationFailureTarget(
	t *testing.T,
	responses map[string]any,
	responseName string,
	exampleName string,
) {
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
	exampleWrapper, ok := examples[exampleName].(map[string]any)
	if !ok {
		t.Fatalf("components.responses.%s example %s is missing", responseName, exampleName)
	}
	example, ok := exampleWrapper["value"].(map[string]any)
	if !ok {
		t.Fatalf("components.responses.%s example %s value must be an object", responseName, exampleName)
	}
	targets, ok := example["targets"].([]any)
	if !ok || len(targets) == 0 {
		t.Fatalf("%s.%s targets = %#v, want canonical validation targets", responseName, exampleName, example["targets"])
	}
	first, ok := targets[0].(map[string]any)
	if !ok {
		t.Fatalf("%s.%s first target = %#v, want object", responseName, exampleName, targets[0])
	}
	subject, ok := first["subject"].(map[string]any)
	if !ok {
		t.Fatalf("%s.%s target subject = %#v, want object", responseName, exampleName, first["subject"])
	}
	if got, _ := first["code"].(string); got != "factory.workstation.missingFailureRoute" {
		t.Fatalf("%s.%s target code = %q, want factory.workstation.missingFailureRoute", responseName, exampleName, got)
	}
	if got, _ := subject["type"].(string); got != "WORKSTATION" {
		t.Fatalf("%s.%s subject type = %q, want WORKSTATION", responseName, exampleName, got)
	}
	if got, _ := subject["id"].(string); got != "bob" {
		t.Fatalf("%s.%s subject id = %q, want bob", responseName, exampleName, got)
	}
	if got, _ := subject["location"].(string); got != "ON_FAILURE" {
		t.Fatalf("%s.%s subject location = %q, want ON_FAILURE", responseName, exampleName, got)
	}
}
