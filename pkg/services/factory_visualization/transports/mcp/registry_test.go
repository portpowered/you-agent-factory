package factoryvisualization_test

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"

	mcpfactoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization/transports/mcp"
)

func TestDiscoverTools_ExposesExpectedVisualizationTools(t *testing.T) {
	t.Parallel()

	tools := mcpfactoryvisualization.DiscoverTools()
	if len(tools) != 8 {
		t.Fatalf("tool count = %d, want 8", len(tools))
	}

	wantNames := []string{
		mcpfactoryvisualization.ToolActivate,
		mcpfactoryvisualization.ToolJoin,
		mcpfactoryvisualization.ToolStopDrain,
		mcpfactoryvisualization.ToolObserve,
		mcpfactoryvisualization.ToolOpenPresentation,
		mcpfactoryvisualization.ToolPresentProgress,
		mcpfactoryvisualization.ToolFinalizePresentation,
		mcpfactoryvisualization.ToolClosePresentation,
	}
	gotNames := mcpfactoryvisualization.ToolNames()
	if !slices.Equal(gotNames, wantNames) {
		t.Fatalf("tool names = %#v, want %#v", gotNames, wantNames)
	}
}

func TestDiscoverTools_EachToolHasDescriptionAndInputSchema(t *testing.T) {
	t.Parallel()

	for _, tool := range mcpfactoryvisualization.DiscoverTools() {
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
	}
}

func TestDiscoverTools_CatalogIsStableUnderRepeatedDiscovery(t *testing.T) {
	t.Parallel()

	first, err := json.Marshal(mcpfactoryvisualization.DiscoverTools())
	if err != nil {
		t.Fatalf("marshal first discovery: %v", err)
	}
	second, err := json.Marshal(mcpfactoryvisualization.DiscoverTools())
	if err != nil {
		t.Fatalf("marshal second discovery: %v", err)
	}
	if !bytesEqual(first, second) {
		t.Fatalf("repeated discovery catalogs differ:\nfirst=%s\nsecond=%s", first, second)
	}
}

func TestDiscoverTools_RepresentativeInputSchemaFields(t *testing.T) {
	t.Parallel()

	byName := toolDefinitionsByName(t)

	activateProps := byName[mcpfactoryvisualization.ToolActivate].InputSchema["properties"].(map[string]any)
	if _, ok := activateProps["mode"]; !ok {
		t.Fatal("activate input schema missing mode")
	}

	observeProps := byName[mcpfactoryvisualization.ToolObserve].InputSchema["properties"].(map[string]any)
	if _, ok := observeProps["mode"]; !ok {
		t.Fatal("observe input schema missing mode")
	}
	reconnect, ok := observeProps["reconnect"].(map[string]any)
	if !ok {
		t.Fatal("observe input schema missing reconnect object")
	}
	reconnectProps, ok := reconnect["properties"].(map[string]any)
	if !ok {
		t.Fatal("observe reconnect schema missing properties")
	}
	for _, field := range []string{"afterEventId", "afterSequence"} {
		if _, ok := reconnectProps[field]; !ok {
			t.Fatalf("observe reconnect schema missing %q", field)
		}
	}

	presentProps := byName[mcpfactoryvisualization.ToolPresentProgress].InputSchema["properties"].(map[string]any)
	for _, field := range []string{"sessionId", "records"} {
		if _, ok := presentProps[field]; !ok {
			t.Fatalf("present_progress input schema missing %q", field)
		}
	}
}

func TestToolByName_ReturnsCatalogEntryForKnownTool(t *testing.T) {
	t.Parallel()

	tool, ok := mcpfactoryvisualization.ToolByName(mcpfactoryvisualization.ToolObserve)
	if !ok {
		t.Fatal("ToolByName(observe) ok = false, want true")
	}
	if tool.Name != mcpfactoryvisualization.ToolObserve {
		t.Fatalf("ToolByName(observe).Name = %q, want %q", tool.Name, mcpfactoryvisualization.ToolObserve)
	}
}

func TestToolByName_UnknownToolReturnsFalse(t *testing.T) {
	t.Parallel()

	_, ok := mcpfactoryvisualization.ToolByName("you.factory_visualization.unknown")
	if ok {
		t.Fatal("ToolByName(unknown) ok = true, want false")
	}
}

func TestIsCanonicalToolHandlerRegistered_OnlyActivateIsRegistered(t *testing.T) {
	t.Parallel()

	if !mcpfactoryvisualization.IsCanonicalToolHandlerRegistered(mcpfactoryvisualization.ToolActivate) {
		t.Fatal("activate handler should be registered")
	}
	for _, name := range []string{
		mcpfactoryvisualization.ToolJoin,
		mcpfactoryvisualization.ToolStopDrain,
		mcpfactoryvisualization.ToolObserve,
		mcpfactoryvisualization.ToolOpenPresentation,
		mcpfactoryvisualization.ToolPresentProgress,
		mcpfactoryvisualization.ToolFinalizePresentation,
		mcpfactoryvisualization.ToolClosePresentation,
	} {
		if mcpfactoryvisualization.IsCanonicalToolHandlerRegistered(name) {
			t.Fatalf("handler for %q should not be registered yet", name)
		}
	}
}

func toolDefinitionsByName(t *testing.T) map[string]mcpfactoryvisualization.ToolDefinition {
	t.Helper()

	byName := make(map[string]mcpfactoryvisualization.ToolDefinition, len(mcpfactoryvisualization.DiscoverTools()))
	for _, tool := range mcpfactoryvisualization.DiscoverTools() {
		byName[tool.Name] = tool
	}
	return byName
}

func bytesEqual(left, right []byte) bool {
	return reflect.DeepEqual(left, right)
}
