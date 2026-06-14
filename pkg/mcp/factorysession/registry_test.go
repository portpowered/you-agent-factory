package factorysession_test

import (
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	mcpfactorysession "github.com/portpowered/infinite-you/pkg/mcp/factorysession"
)

func TestDiscoverTools_ExposesExpectedFactorySessionTools(t *testing.T) {
	tools := mcpfactorysession.DiscoverTools()
	if len(tools) != 8 {
		t.Fatalf("tool count = %d, want 8", len(tools))
	}

	wantNames := []string{
		mcpfactorysession.ToolListSessions,
		mcpfactorysession.ToolValidateSource,
		mcpfactorysession.ToolStartSync,
		mcpfactorysession.ToolStartAsync,
		mcpfactorysession.ToolGetSession,
		mcpfactorysession.ToolGetResult,
		mcpfactorysession.ToolListDispatches,
		mcpfactorysession.ToolListArtifacts,
	}
	gotNames := mcpfactorysession.ToolNames()
	if !slices.Equal(gotNames, wantNames) {
		t.Fatalf("tool names = %#v, want %#v", gotNames, wantNames)
	}
}

func TestDiscoverTools_EachToolHasSchemasDescriptionsAndStableFields(t *testing.T) {
	for _, tool := range mcpfactorysession.DiscoverTools() {
		if strings.TrimSpace(tool.Name) == "" {
			t.Fatal("tool name is required")
		}
		if strings.TrimSpace(tool.Description) == "" {
			t.Fatalf("tool %q description is required", tool.Name)
		}
		if len(tool.InputSchema) == 0 {
			t.Fatalf("tool %q input schema is required", tool.Name)
		}
		if schemaType, _ := tool.InputSchema["type"].(string); schemaType != "object" {
			t.Fatalf("tool %q input schema type = %q, want object", tool.Name, schemaType)
		}
		if len(tool.OutputSchema) == 0 {
			t.Fatalf("tool %q output schema is required", tool.Name)
		}
		if len(tool.SuccessStableFields) == 0 {
			t.Fatalf("tool %q success stable fields are required", tool.Name)
		}
		if len(tool.ErrorStableFields) == 0 {
			t.Fatalf("tool %q error stable fields are required", tool.Name)
		}
		for _, field := range append(tool.SuccessStableFields, tool.ErrorStableFields...) {
			if strings.TrimSpace(field) == "" {
				t.Fatalf("tool %q has empty stable field entry", tool.Name)
			}
		}
	}
}

func TestDiscoverTools_UsesFactorySessionVocabularyNotWorkflowPreviewPrimarySurface(t *testing.T) {
	forbidden := []string{
		"/workflow-previews",
		"workflow-previews",
		"WorkflowPreview",
	}
	for _, tool := range mcpfactorysession.DiscoverTools() {
		encoded, err := json.Marshal(tool)
		if err != nil {
			t.Fatalf("marshal tool %q: %v", tool.Name, err)
		}
		payload := string(encoded)
		for _, term := range forbidden {
			if strings.Contains(payload, term) {
				t.Fatalf("tool %q exposes deprecated primary surface term %q", tool.Name, term)
			}
		}
		if !strings.Contains(tool.Description, "Factory Session") &&
			tool.Name != mcpfactorysession.ToolValidateSource {
			t.Fatalf("tool %q description should mention Factory Session vocabulary", tool.Name)
		}
	}
}

func TestDiscoverTools_OutputSchemasDocumentSharedErrorEnvelope(t *testing.T) {
	for _, tool := range mcpfactorysession.DiscoverTools() {
		properties, ok := tool.OutputSchema["properties"].(map[string]any)
		if !ok {
			t.Fatalf("tool %q output schema properties missing", tool.Name)
		}
		errorSchema, ok := properties["error"].(map[string]any)
		if !ok {
			t.Fatalf("tool %q output schema missing error envelope", tool.Name)
		}
		errorProps, ok := errorSchema["properties"].(map[string]any)
		if !ok {
			t.Fatalf("tool %q error envelope properties missing", tool.Name)
		}
		for _, field := range []string{"code", "message", "retryable"} {
			if _, ok := errorProps[field]; !ok {
				t.Fatalf("tool %q error envelope missing %q", tool.Name, field)
			}
		}
	}
}

func TestDiscoverTools_RepresentativeSchemaFields(t *testing.T) {
	byName := map[string]mcpfactorysession.ToolDefinition{}
	for _, tool := range mcpfactorysession.DiscoverTools() {
		byName[tool.Name] = tool
	}

	listProps := byName[mcpfactorysession.ToolListSessions].InputSchema["properties"].(map[string]any)
	if _, ok := listProps["scope"]; !ok {
		t.Fatal("list sessions input missing scope")
	}

	validateProps := byName[mcpfactorysession.ToolValidateSource].InputSchema["properties"].(map[string]any)
	if _, ok := validateProps["sourceKind"]; !ok {
		t.Fatal("validate source input missing sourceKind")
	}

	startProps := byName[mcpfactorysession.ToolStartAsync].InputSchema["properties"].(map[string]any)
	if _, ok := startProps["requestId"]; !ok {
		t.Fatal("start async input missing requestId")
	}
	if _, ok := startProps["source"]; !ok {
		t.Fatal("start async input missing source")
	}

	getResultProps := byName[mcpfactorysession.ToolGetResult].InputSchema["properties"].(map[string]any)
	for _, field := range []string{"sessionId", "mode", "includeArtifacts"} {
		if _, ok := getResultProps[field]; !ok {
			t.Fatalf("get result input missing %q", field)
		}
	}

	syncResultProps := byName[mcpfactorysession.ToolStartSync].OutputSchema["properties"].(map[string]any)["result"].(map[string]any)["properties"].(map[string]any)
	if _, ok := syncResultProps["syncOutcome"]; !ok {
		t.Fatal("start sync output result missing syncOutcome")
	}

	dispatchResultProps := byName[mcpfactorysession.ToolListDispatches].OutputSchema["properties"].(map[string]any)["result"].(map[string]any)["properties"].(map[string]any)
	dispatches, ok := dispatchResultProps["dispatches"].(map[string]any)
	if !ok {
		t.Fatal("list dispatches output missing dispatches array schema")
	}
	itemProps := dispatches["items"].(map[string]any)["properties"].(map[string]any)
	if _, ok := itemProps["id"]; !ok {
		t.Fatal("dispatch summary schema missing id")
	}
	if _, ok := itemProps["dispatchId"]; ok {
		t.Fatal("dispatch summary schema should use id, not dispatchId")
	}
}

func TestDiscoverTools_SchemasAlignWithGeneratedAPITypes(t *testing.T) {
	byName := map[string]mcpfactorysession.ToolDefinition{}
	for _, tool := range mcpfactorysession.DiscoverTools() {
		byName[tool.Name] = tool
	}

	syncResultProps := byName[mcpfactorysession.ToolStartSync].OutputSchema["properties"].(map[string]any)["result"].(map[string]any)["properties"].(map[string]any)
	syncOutcome := syncResultProps["syncOutcome"].(map[string]any)
	assertEnumMatchesGeneratedType(
		t,
		"syncOutcome",
		syncOutcome["enum"],
		reflect.TypeOf(factoryapi.FactorySessionSyncExecutionOutcome("")),
	)

	dispatchItemProps := dispatchSummaryItemProperties(t, byName[mcpfactorysession.ToolListDispatches])
	assertSchemaUsesPublicJSONFields(t, dispatchItemProps, "dispatch summary", "id", "status", "dispatchKind")
	assertSchemaOmitsDeprecatedJSONFields(t, dispatchItemProps, "dispatch summary", "dispatchId", "artifactId")

	artifactItemProps := artifactSummaryItemProperties(t, byName[mcpfactorysession.ToolListArtifacts])
	assertSchemaUsesPublicJSONFields(t, artifactItemProps, "artifact summary", "id", "kind", "visibility", "dispatchId")
	assertSchemaOmitsDeprecatedJSONFields(t, artifactItemProps, "artifact summary", "artifactId")
}

func dispatchSummaryItemProperties(t *testing.T, tool mcpfactorysession.ToolDefinition) map[string]any {
	t.Helper()
	dispatchResultProps := tool.OutputSchema["properties"].(map[string]any)["result"].(map[string]any)["properties"].(map[string]any)
	dispatches, ok := dispatchResultProps["dispatches"].(map[string]any)
	if !ok {
		t.Fatal("list dispatches output missing dispatches array schema")
	}
	return dispatches["items"].(map[string]any)["properties"].(map[string]any)
}

func artifactSummaryItemProperties(t *testing.T, tool mcpfactorysession.ToolDefinition) map[string]any {
	t.Helper()
	artifactResultProps := tool.OutputSchema["properties"].(map[string]any)["result"].(map[string]any)["properties"].(map[string]any)
	artifacts, ok := artifactResultProps["artifacts"].(map[string]any)
	if !ok {
		t.Fatal("list artifacts output missing artifacts array schema")
	}
	return artifacts["items"].(map[string]any)["properties"].(map[string]any)
}

func assertEnumMatchesGeneratedType(t *testing.T, field string, actual any, enumType reflect.Type) {
	t.Helper()
	gotStrings, err := enumValuesAsStrings(actual)
	if err != nil {
		t.Fatalf("%s enum: %v", field, err)
	}
	var want []string
	switch enumType.Name() {
	case "FactorySessionSyncExecutionOutcome":
		want = []string{
			string(factoryapi.FactorySessionSyncExecutionOutcomeCompleted),
			string(factoryapi.FactorySessionSyncExecutionOutcomeStillRunning),
			string(factoryapi.FactorySessionSyncExecutionOutcomeTimedOut),
		}
	default:
		t.Fatalf("unsupported generated enum type %q", enumType.Name())
	}
	if !slices.Equal(gotStrings, want) {
		t.Fatalf("%s enum = %#v, want %#v", field, gotStrings, want)
	}
}

func enumValuesAsStrings(actual any) ([]string, error) {
	switch values := actual.(type) {
	case []string:
		return values, nil
	case []any:
		got := make([]string, 0, len(values))
		for _, value := range values {
			got = append(got, value.(string))
		}
		return got, nil
	default:
		return nil, fmt.Errorf("enum = %T, want []string or []any", actual)
	}
}

func assertSchemaUsesPublicJSONFields(t *testing.T, properties map[string]any, label string, fields ...string) {
	t.Helper()
	for _, field := range fields {
		if _, ok := properties[field]; !ok {
			t.Fatalf("%s schema missing public field %q", label, field)
		}
	}
}

func assertSchemaOmitsDeprecatedJSONFields(t *testing.T, properties map[string]any, label string, fields ...string) {
	t.Helper()
	for _, field := range fields {
		if _, ok := properties[field]; ok {
			t.Fatalf("%s schema should not expose deprecated field %q", label, field)
		}
	}
}

func TestMockClientDiscovery_RoundTripsJSON(t *testing.T) {
	tools := mcpfactorysession.DiscoverTools()
	encoded, err := json.Marshal(tools)
	if err != nil {
		t.Fatalf("marshal discovery catalog: %v", err)
	}

	var decoded []mcpfactorysession.ToolDefinition
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal discovery catalog: %v", err)
	}
	if len(decoded) != len(tools) {
		t.Fatalf("decoded tool count = %d, want %d", len(decoded), len(tools))
	}
	if decoded[0].Name != mcpfactorysession.ToolListSessions {
		t.Fatalf("first tool = %q, want %q", decoded[0].Name, mcpfactorysession.ToolListSessions)
	}
}
