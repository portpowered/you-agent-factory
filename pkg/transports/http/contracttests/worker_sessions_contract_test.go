package apicontract_test

import "testing"

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
