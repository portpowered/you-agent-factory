package operatorsettingsmcp_test

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	mcpoperatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings/transports/mcp"
)

func TestDiscoverTools_ExposesExpectedOperatorSettingsTools(t *testing.T) {
	t.Parallel()

	tools := mcpoperatorsettings.DiscoverTools()
	if len(tools) != 3 {
		t.Fatalf("tool count = %d, want 3", len(tools))
	}

	wantNames := []string{
		mcpoperatorsettings.ToolLoadDocument,
		mcpoperatorsettings.ToolApplyDocumentUpdate,
		mcpoperatorsettings.ToolResolveEffective,
	}
	gotNames := mcpoperatorsettings.ToolNames()
	if !slices.Equal(gotNames, wantNames) {
		t.Fatalf("tool names = %#v, want %#v", gotNames, wantNames)
	}
	for _, name := range wantNames {
		if !mcpoperatorsettings.IsCanonicalToolHandlerRegistered(name) {
			t.Fatalf("canonical handler missing for %q", name)
		}
	}
}

func TestDiscoverTools_EachToolHasSchemasDescriptionsAndStableFields(t *testing.T) {
	t.Parallel()

	for _, tool := range mcpoperatorsettings.DiscoverTools() {
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

	loadProps := byName[mcpoperatorsettings.ToolLoadDocument].InputSchema["properties"].(map[string]any)
	for _, field := range []string{"path", "requireExisting"} {
		if _, ok := loadProps[field]; !ok {
			t.Fatalf("load document input missing %q", field)
		}
	}

	applyProps := byName[mcpoperatorsettings.ToolApplyDocumentUpdate].InputSchema["properties"].(map[string]any)
	for _, field := range []string{"path", "expectedBackendScope", "providerModel"} {
		if _, ok := applyProps[field]; !ok {
			t.Fatalf("apply document update input missing %q", field)
		}
	}

	resolveProps := byName[mcpoperatorsettings.ToolResolveEffective].InputSchema["properties"].(map[string]any)
	for _, field := range []string{"documentBaseline", "configPath"} {
		if _, ok := resolveProps[field]; !ok {
			t.Fatalf("resolve effective input missing %q", field)
		}
	}
}

func TestDiscoverTools_OutputSchemasDocumentSharedErrorEnvelope(t *testing.T) {
	t.Parallel()

	for _, tool := range mcpoperatorsettings.DiscoverTools() {
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

	first, err := json.Marshal(mcpoperatorsettings.DiscoverTools())
	if err != nil {
		t.Fatalf("marshal first discovery: %v", err)
	}
	second, err := json.Marshal(mcpoperatorsettings.DiscoverTools())
	if err != nil {
		t.Fatalf("marshal second discovery: %v", err)
	}
	if string(first) != string(second) {
		t.Fatalf("repeat discovery differs:\nfirst=%s\nsecond=%s", first, second)
	}
}

func TestToolByName_ReturnsCanonicalDefinitions(t *testing.T) {
	t.Parallel()

	for _, want := range mcpoperatorsettings.DiscoverTools() {
		got, ok := mcpoperatorsettings.ToolByName(want.Name)
		if !ok {
			t.Fatalf("ToolByName(%q) ok = false, want true", want.Name)
		}
		if got.Name != want.Name || got.Description != want.Description {
			t.Fatalf("ToolByName(%q) = %#v, want %#v", want.Name, got, want)
		}
	}

	if _, ok := mcpoperatorsettings.ToolByName("you.operator_settings.unknown"); ok {
		t.Fatal("ToolByName(unknown) ok = true, want false")
	}
}

func toolDefinitionsByName(t *testing.T) map[string]mcpoperatorsettings.ToolDefinition {
	t.Helper()

	byName := make(map[string]mcpoperatorsettings.ToolDefinition, len(mcpoperatorsettings.DiscoverTools()))
	for _, tool := range mcpoperatorsettings.DiscoverTools() {
		byName[tool.Name] = tool
	}
	return byName
}
