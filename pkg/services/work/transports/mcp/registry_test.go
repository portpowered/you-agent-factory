package workmcp_test

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	workmcp "github.com/portpowered/infinite-you/pkg/services/work/transports/mcp"
)

func TestDiscoverTools_ExposesExpectedWorkTools(t *testing.T) {
	t.Parallel()

	tools := workmcp.DiscoverTools()
	if len(tools) != 4 {
		t.Fatalf("tool count = %d, want 4", len(tools))
	}

	wantNames := []string{
		workmcp.ToolSubmit,
		workmcp.ToolList,
		workmcp.ToolGet,
		workmcp.ToolMove,
	}
	gotNames := workmcp.ToolNames()
	if !slices.Equal(gotNames, wantNames) {
		t.Fatalf("tool names = %#v, want %#v", gotNames, wantNames)
	}
}

func TestDiscoverTools_EachToolHasSchemasDescriptionsAndStableFields(t *testing.T) {
	t.Parallel()

	for _, tool := range workmcp.DiscoverTools() {
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
		additional, ok := tool.InputSchema["additionalProperties"].(bool)
		if !ok || additional {
			t.Fatalf("tool %q input schema additionalProperties = %v, want false", tool.Name, tool.InputSchema["additionalProperties"])
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
	}
}

func TestDiscoverTools_OutputSchemasDocumentSharedErrorEnvelope(t *testing.T) {
	t.Parallel()

	for _, tool := range workmcp.DiscoverTools() {
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

func TestDiscoverTools_RepeatDiscoveryIsStable(t *testing.T) {
	t.Parallel()

	first, err := json.Marshal(workmcp.DiscoverTools())
	if err != nil {
		t.Fatalf("marshal first discovery: %v", err)
	}
	second, err := json.Marshal(workmcp.DiscoverTools())
	if err != nil {
		t.Fatalf("marshal second discovery: %v", err)
	}
	if string(first) != string(second) {
		t.Fatalf("repeat discovery differs:\nfirst=%s\nsecond=%s", first, second)
	}
}

func TestDiscoverTools_RepresentativeInputSchemaFields(t *testing.T) {
	t.Parallel()

	byName := toolDefinitionsByName(t)

	submitProps := byName[workmcp.ToolSubmit].InputSchema["properties"].(map[string]any)
	for _, field := range []string{"sessionId", "workRequest"} {
		if _, ok := submitProps[field]; !ok {
			t.Fatalf("submit input schema missing %q", field)
		}
	}

	listProps := byName[workmcp.ToolList].InputSchema["properties"].(map[string]any)
	for _, field := range []string{"sessionId", "workTypeName", "maxResults", "nextToken"} {
		if _, ok := listProps[field]; !ok {
			t.Fatalf("list input schema missing %q", field)
		}
	}

	getProps := byName[workmcp.ToolGet].InputSchema["properties"].(map[string]any)
	for _, field := range []string{"sessionId", "workId"} {
		if _, ok := getProps[field]; !ok {
			t.Fatalf("get input schema missing %q", field)
		}
	}

	moveProps := byName[workmcp.ToolMove].InputSchema["properties"].(map[string]any)
	for _, field := range []string{"sessionId", "workId", "stateName", "requestId"} {
		if _, ok := moveProps[field]; !ok {
			t.Fatalf("move input schema missing %q", field)
		}
	}
}

func TestToolByName_ReturnsCatalogEntryForKnownTools(t *testing.T) {
	t.Parallel()

	for _, want := range workmcp.DiscoverTools() {
		got, ok := workmcp.ToolByName(want.Name)
		if !ok {
			t.Fatalf("ToolByName(%q) = false, want true", want.Name)
		}
		if got.Name != want.Name || got.Description != want.Description {
			t.Fatalf("ToolByName(%q) = %#v, want %#v", want.Name, got, want)
		}
	}
}

func TestToolByName_UnknownToolReturnsFalse(t *testing.T) {
	t.Parallel()

	_, ok := workmcp.ToolByName("you.work.unknown")
	if ok {
		t.Fatal("ToolByName(unknown) ok = true, want false")
	}
}

func TestIsCanonicalToolHandlerRegistered_ReportsListGetAndSubmitHandlers(t *testing.T) {
	t.Parallel()

	if !workmcp.IsCanonicalToolHandlerRegistered(workmcp.ToolGet) {
		t.Fatal("get handler should be registered")
	}
	if !workmcp.IsCanonicalToolHandlerRegistered(workmcp.ToolList) {
		t.Fatal("list handler should be registered")
	}
	if !workmcp.IsCanonicalToolHandlerRegistered(workmcp.ToolSubmit) {
		t.Fatal("submit handler should be registered")
	}
	if workmcp.IsCanonicalToolHandlerRegistered(workmcp.ToolMove) {
		t.Fatal("move handler should not be registered yet")
	}
}

func toolDefinitionsByName(t *testing.T) map[string]workmcp.ToolDefinition {
	t.Helper()

	byName := make(map[string]workmcp.ToolDefinition, len(workmcp.DiscoverTools()))
	for _, tool := range workmcp.DiscoverTools() {
		byName[tool.Name] = tool
	}
	return byName
}
