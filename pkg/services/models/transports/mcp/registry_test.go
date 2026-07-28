package modelmcp_test

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	modelmcp "github.com/portpowered/infinite-you/pkg/services/models/transports/mcp"
)

func TestDiscoverTools_ExposesExpectedModelsTools(t *testing.T) {
	t.Parallel()

	tools := modelmcp.DiscoverTools()
	if len(tools) != 4 {
		t.Fatalf("tool count = %d, want 4", len(tools))
	}

	wantNames := []string{
		modelmcp.ToolListCatalog,
		modelmcp.ToolPrepareAssets,
		modelmcp.ToolAcquireLease,
		modelmcp.ToolInvokeWithLease,
	}
	gotNames := modelmcp.ToolNames()
	if !slices.Equal(gotNames, wantNames) {
		t.Fatalf("tool names = %#v, want %#v", gotNames, wantNames)
	}
}

func TestDiscoverTools_EachToolHasSchemasDescriptionsAndStableFields(t *testing.T) {
	t.Parallel()

	for _, tool := range modelmcp.DiscoverTools() {
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

func TestDiscoverTools_RepresentativeSchemaFields(t *testing.T) {
	t.Parallel()

	byName := toolDefinitionsByName(t)

	listProps := byName[modelmcp.ToolListCatalog].InputSchema["properties"].(map[string]any)
	if _, ok := listProps["runtimeScopeRef"]; !ok {
		t.Fatal("list catalog input missing runtimeScopeRef")
	}

	prepareProps := byName[modelmcp.ToolPrepareAssets].InputSchema["properties"].(map[string]any)
	for _, field := range []string{"runtimeScopeRef", "name"} {
		if _, ok := prepareProps[field]; !ok {
			t.Fatalf("prepare assets input missing %q", field)
		}
	}

	acquireProps := byName[modelmcp.ToolAcquireLease].InputSchema["properties"].(map[string]any)
	for _, field := range []string{"runtimeScopeRef", "name", "holder"} {
		if _, ok := acquireProps[field]; !ok {
			t.Fatalf("acquire lease input missing %q", field)
		}
	}

	invokeProps := byName[modelmcp.ToolInvokeWithLease].InputSchema["properties"].(map[string]any)
	for _, field := range []string{"runtimeScopeRef", "leaseRef", "holder", "modelName", "operation", "input"} {
		if _, ok := invokeProps[field]; !ok {
			t.Fatalf("invoke with lease input missing %q", field)
		}
	}
}

func TestDiscoverTools_OutputSchemasDocumentSharedErrorEnvelope(t *testing.T) {
	t.Parallel()

	for _, tool := range modelmcp.DiscoverTools() {
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

	first, err := json.Marshal(modelmcp.DiscoverTools())
	if err != nil {
		t.Fatalf("marshal first discovery: %v", err)
	}
	second, err := json.Marshal(modelmcp.DiscoverTools())
	if err != nil {
		t.Fatalf("marshal second discovery: %v", err)
	}
	if string(first) != string(second) {
		t.Fatalf("repeat discovery differs:\nfirst=%s\nsecond=%s", first, second)
	}
}

func TestToolByName_ReturnsCatalogEntryForKnownTools(t *testing.T) {
	t.Parallel()

	for _, want := range modelmcp.DiscoverTools() {
		got, ok := modelmcp.ToolByName(want.Name)
		if !ok {
			t.Fatalf("ToolByName(%q) = false, want true", want.Name)
		}
		if got.Name != want.Name || got.Description != want.Description {
			t.Fatalf("ToolByName(%q) = %#v, want %#v", want.Name, got, want)
		}
	}

	if _, ok := modelmcp.ToolByName("you.model.unknown_tool"); ok {
		t.Fatal("ToolByName(unknown) = true, want false")
	}
}

func TestIsCanonicalToolHandlerRegistered_ReportsListCatalogTool(t *testing.T) {
	t.Parallel()

	if !modelmcp.IsCanonicalToolHandlerRegistered(modelmcp.ToolListCatalog) {
		t.Fatalf("handler for %q should be registered", modelmcp.ToolListCatalog)
	}
}

func toolDefinitionsByName(t *testing.T) map[string]modelmcp.ToolDefinition {
	t.Helper()

	byName := make(map[string]modelmcp.ToolDefinition, len(modelmcp.DiscoverTools()))
	for _, tool := range modelmcp.DiscoverTools() {
		byName[tool.Name] = tool
	}
	return byName
}
