package docs

import (
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestMarkdown_SessionsResponseEventStreamAlignsWithOpenAPI(t *testing.T) {
	doc, err := Markdown("sessions")
	if err != nil {
		t.Fatalf("Markdown(sessions) error = %v", err)
	}
	description := loadAuthoredOpenAPIOperationDescription(
		t,
		"/factory-sessions/{session_id}/response-events",
		"get",
	)
	for _, alignment := range []struct {
		openAPIPhrase string
		docPhrase     string
	}{
		{"ephemeral FactoryResponseEvent observation records", "ephemeral invocation observation"},
		{"outside canonical FactoryEvent replay", "outside canonical `FactoryEvent` replay"},
		{"retained matching records", "retained matching records"},
		{"continues with live matching records", "live matching records"},
		{"after_sequence", "after_sequence"},
		{"first emitted record is STREAM_GAP", "first emitted record is `STREAM_GAP`"},
		{"never falls back to the current or default session", "never falls back"},
	} {
		if !strings.Contains(description, alignment.openAPIPhrase) {
			t.Fatalf("authored OpenAPI response-event description missing %q", alignment.openAPIPhrase)
		}
		if !strings.Contains(doc, alignment.docPhrase) {
			t.Fatalf("packaged sessions guide missing OpenAPI-aligned phrase %q (from OpenAPI %q)", alignment.docPhrase, alignment.openAPIPhrase)
		}
	}

	notFound := loadAuthoredOpenAPIComponentFragment(t, "../../../../api/components/responses/ResponseEventSessionNotFound.yaml")
	expired := loadAuthoredOpenAPIComponentFragment(t, "../../../../api/components/responses/ResponseEventStreamExpired.yaml")
	badRequest := loadAuthoredOpenAPIComponentFragment(t, "../../../../api/components/responses/ResponseEventBadRequest.yaml")
	for _, item := range []struct {
		response map[string]any
		codes    []string
	}{
		{response: notFound, codes: []string{"RESPONSE_EVENT_SESSION_NOT_FOUND"}},
		{response: expired, codes: []string{"RESPONSE_EVENT_STREAM_EXPIRED"}},
		{response: badRequest, codes: []string{"INVALID_RESPONSE_EVENT_CURSOR"}},
	} {
		gotCodes := openAPIResponseExampleCodes(t, item.response)
		for _, wantCode := range item.codes {
			if !containsString(gotCodes, wantCode) {
				t.Fatalf("authored OpenAPI response examples missing code %q", wantCode)
			}
			if !strings.Contains(doc, wantCode) {
				t.Fatalf("packaged sessions guide missing authored OpenAPI error code %q", wantCode)
			}
		}
	}
	if !strings.Contains(doc, "related bad-request codes") {
		t.Fatal("packaged sessions guide must acknowledge additional authored OpenAPI bad-request codes")
	}
}

func TestMarkdown_WorkersProviderFidelityAlignsWithPublicContract(t *testing.T) {
	doc, err := Markdown("workers")
	if err != nil {
		t.Fatalf("Markdown(workers) error = %v", err)
	}
	section := extractMarkdownSection(doc, "## Response-stream provider fidelity")
	if section == "" {
		t.Fatal("packaged workers guide missing response-stream provider fidelity section")
	}

	fidelity := loadAuthoredOpenAPIComponentFragment(
		t,
		"../../../../api/components/schemas/response-events/FactoryResponseEventProvenanceFidelity.yaml",
	)
	finalOnlyDescription := openAPIEnumDescription(t, fidelity, "FINAL_ONLY")
	if !strings.Contains(strings.ToLower(section), "final-only") {
		t.Fatalf("workers provider-fidelity section missing customer-facing FINAL_ONLY label; OpenAPI says %q", finalOnlyDescription)
	}
	sectionLower := strings.ToLower(section)
	if !strings.Contains(sectionLower, "final") ||
		(!strings.Contains(sectionLower, "outcome") && !strings.Contains(sectionLower, "terminal semantic")) {
		t.Fatalf("workers provider-fidelity section must reflect OpenAPI FINAL_ONLY description %q", finalOnlyDescription)
	}

	streamGap := loadAuthoredOpenAPIComponentFragment(
		t,
		"../../../../api/components/schemas/response-events/payloads/FactoryResponseEventStreamGapPayload.yaml",
	)
	retentionGap := openAPIOneOfVariant(t, streamGap, 0)
	for _, field := range []string{"fromSequence", "toSequence", "firstAvailableSequence"} {
		properties := openAPISchemaProperties(t, retentionGap)
		if properties[field] == nil {
			t.Fatalf("authored OpenAPI STREAM_GAP payload missing field %q", field)
		}
		if !strings.Contains(section, field) {
			t.Fatalf("workers provider-fidelity section missing OpenAPI STREAM_GAP field %q", field)
		}
	}

	responseEventDescription := loadAuthoredOpenAPIOperationDescription(
		t,
		"/factory-sessions/{session_id}/response-events",
		"get",
	)
	if !strings.Contains(responseEventDescription, "ephemeral FactoryResponseEvent observation records") {
		t.Fatal("authored OpenAPI response-event description missing ephemeral observation contract")
	}
	if !strings.Contains(section, "GET /factory-sessions/{session_id}/response-events") {
		t.Fatal("workers provider-fidelity section must route consumers to the authored response-event SSE route")
	}
}

func loadAuthoredOpenAPIOperationDescription(t *testing.T, path, method string) string {
	t.Helper()
	doc := loadAuthoredOpenAPIDoc(t)
	paths, ok := doc["paths"].(map[string]any)
	if !ok {
		t.Fatal("authored OpenAPI paths object is missing")
	}
	pathItem, ok := paths[path].(map[string]any)
	if !ok {
		t.Fatalf("authored OpenAPI path %q is missing", path)
	}
	operation, ok := pathItem[method].(map[string]any)
	if !ok {
		t.Fatalf("authored OpenAPI %s %s operation is missing", method, path)
	}
	description, ok := operation["description"].(string)
	if !ok || strings.TrimSpace(description) == "" {
		t.Fatalf("authored OpenAPI %s %s description is missing", method, path)
	}
	return description
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

func loadAuthoredOpenAPIComponentFragment(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read authored OpenAPI component %s: %v", path, err)
	}
	var fragment map[string]any
	if err := yaml.Unmarshal(data, &fragment); err != nil {
		t.Fatalf("parse authored OpenAPI component %s: %v", path, err)
	}
	return fragment
}

func openAPIResponseExampleCodes(t *testing.T, response map[string]any) []string {
	t.Helper()
	content, ok := response["content"].(map[string]any)
	if !ok {
		t.Fatal("authored OpenAPI response content is missing")
	}
	applicationJSON, ok := content["application/json"].(map[string]any)
	if !ok {
		t.Fatal("authored OpenAPI response application/json content is missing")
	}
	examples, ok := applicationJSON["examples"].(map[string]any)
	if !ok {
		t.Fatal("authored OpenAPI response examples are missing")
	}
	codes := make([]string, 0, len(examples))
	for _, rawExample := range examples {
		example, ok := rawExample.(map[string]any)
		if !ok {
			t.Fatal("authored OpenAPI response example must be an object")
		}
		value, ok := example["value"].(map[string]any)
		if !ok {
			t.Fatal("authored OpenAPI response example value must be an object")
		}
		code, ok := value["code"].(string)
		if !ok {
			t.Fatal("authored OpenAPI response example code must be a string")
		}
		codes = append(codes, code)
	}
	return codes
}

func openAPIEnumDescription(t *testing.T, fragment map[string]any, enumValue string) string {
	t.Helper()
	enums, ok := fragment["enum"].([]any)
	if !ok {
		t.Fatal("authored OpenAPI enum values are missing")
	}
	descriptions, ok := fragment["x-enum-descriptions"].([]any)
	if !ok {
		t.Fatal("authored OpenAPI enum descriptions are missing")
	}
	for index, rawValue := range enums {
		value, ok := rawValue.(string)
		if !ok || value != enumValue {
			continue
		}
		if index >= len(descriptions) {
			t.Fatalf("authored OpenAPI enum description missing for %q", enumValue)
		}
		description, ok := descriptions[index].(string)
		if !ok || strings.TrimSpace(description) == "" {
			t.Fatalf("authored OpenAPI enum description for %q is missing", enumValue)
		}
		return description
	}
	t.Fatalf("authored OpenAPI enum value %q is missing", enumValue)
	return ""
}

func openAPISchemaProperties(t *testing.T, fragment map[string]any) map[string]any {
	t.Helper()
	properties, ok := fragment["properties"].(map[string]any)
	if !ok {
		t.Fatal("authored OpenAPI schema properties are missing")
	}
	return properties
}

func openAPIOneOfVariant(t *testing.T, fragment map[string]any, index int) map[string]any {
	t.Helper()
	oneOf, ok := fragment["oneOf"].([]any)
	if !ok || index >= len(oneOf) {
		t.Fatalf("authored OpenAPI oneOf variant %d is missing", index)
	}
	variant, ok := oneOf[index].(map[string]any)
	if !ok {
		t.Fatalf("authored OpenAPI oneOf variant %d must be an object", index)
	}
	return variant
}

func extractMarkdownSection(doc, heading string) string {
	start := strings.Index(doc, heading)
	if start < 0 {
		return ""
	}
	rest := doc[start+len(heading):]
	next := strings.Index(rest, "\n## ")
	if next < 0 {
		return strings.TrimSpace(rest)
	}
	return strings.TrimSpace(rest[:next])
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
