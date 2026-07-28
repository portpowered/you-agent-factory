package mcp_test

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	factoryrunmcp "github.com/portpowered/infinite-you/pkg/services/factory_runtime/transports/mcp"
)

func TestDiscoverTools_ExposesExpectedFactoryRuntimeTools(t *testing.T) {
	t.Parallel()

	tools := factoryrunmcp.DiscoverTools()
	if len(tools) != 4 {
		t.Fatalf("tool count = %d, want 4", len(tools))
	}

	wantNames := []string{
		factoryrunmcp.ToolControlPause,
		factoryrunmcp.ToolObserve,
		factoryrunmcp.ToolPlanDispatch,
		factoryrunmcp.ToolAcceptDispatchResult,
	}
	gotNames := factoryrunmcp.ToolNames()
	if !slices.Equal(gotNames, wantNames) {
		t.Fatalf("tool names = %#v, want %#v", gotNames, wantNames)
	}
}

func TestDiscoverTools_EachToolHasSchemasDescriptionsAndStableFields(t *testing.T) {
	t.Parallel()

	for _, tool := range factoryrunmcp.DiscoverTools() {
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

func TestDiscoverTools_HandlerRegistrationMatchesDiscovery(t *testing.T) {
	t.Parallel()

	for _, tool := range factoryrunmcp.DiscoverTools() {
		if !factoryrunmcp.IsCanonicalToolHandlerRegistered(tool.Name) {
			t.Fatalf("discovered tool %q has no registered handler", tool.Name)
		}
	}
}

func TestDiscoverTools_DeterministicCatalogSerialization(t *testing.T) {
	t.Parallel()

	first, err := json.Marshal(factoryrunmcp.DiscoverTools())
	if err != nil {
		t.Fatalf("marshal discovery catalog: %v", err)
	}
	second, err := json.Marshal(factoryrunmcp.DiscoverTools())
	if err != nil {
		t.Fatalf("marshal discovery catalog: %v", err)
	}
	if string(first) != string(second) {
		t.Fatalf("discovery catalog serialization differs between calls:\nfirst=%s\nsecond=%s", first, second)
	}
}

func TestDiscoverTools_UsesFactoryRuntimeVocabulary(t *testing.T) {
	t.Parallel()

	for _, tool := range factoryrunmcp.DiscoverTools() {
		if !strings.HasPrefix(tool.Name, "you.factory_runtime.") {
			t.Fatalf("tool %q name does not use Factory Runtime vocabulary prefix", tool.Name)
		}
	}
}

func TestToolByName_ReturnsCanonicalDefinitions(t *testing.T) {
	t.Parallel()

	for _, want := range factoryrunmcp.DiscoverTools() {
		got, ok := factoryrunmcp.ToolByName(want.Name)
		if !ok {
			t.Fatalf("ToolByName(%q) = false, want true", want.Name)
		}
		if got.Name != want.Name || got.Description != want.Description {
			t.Fatalf("ToolByName(%q) = %#v, want %#v", want.Name, got, want)
		}
	}
}
