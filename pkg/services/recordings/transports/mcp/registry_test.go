package recordingmcp_test

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	mcprecording "github.com/portpowered/infinite-you/pkg/services/recordings/transports/mcp"
)

func TestDiscoverTools_ExposesExpectedRecordingsTools(t *testing.T) {
	t.Parallel()

	tools := mcprecording.DiscoverTools()
	if len(tools) != 4 {
		t.Fatalf("tool count = %d, want 4", len(tools))
	}

	wantNames := []string{
		mcprecording.ToolQueryStatus,
		mcprecording.ToolAppendEvent,
		mcprecording.ToolLoadReplay,
		mcprecording.ToolReadPortableArtifact,
	}
	gotNames := mcprecording.ToolNames()
	if !slices.Equal(gotNames, wantNames) {
		t.Fatalf("tool names = %#v, want %#v", gotNames, wantNames)
	}
}

func TestDiscoverTools_EachToolHasSchemasDescriptionsAndStableFields(t *testing.T) {
	t.Parallel()

	for _, tool := range mcprecording.DiscoverTools() {
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

	for _, tool := range mcprecording.DiscoverTools() {
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

	first, err := json.Marshal(mcprecording.DiscoverTools())
	if err != nil {
		t.Fatalf("marshal first discovery: %v", err)
	}
	second, err := json.Marshal(mcprecording.DiscoverTools())
	if err != nil {
		t.Fatalf("marshal second discovery: %v", err)
	}
	if string(first) != string(second) {
		t.Fatalf("repeat discovery differs:\nfirst=%s\nsecond=%s", first, second)
	}
}

func TestToolByName_ReturnsCatalogEntryForKnownTools(t *testing.T) {
	t.Parallel()

	for _, want := range mcprecording.DiscoverTools() {
		got, ok := mcprecording.ToolByName(want.Name)
		if !ok {
			t.Fatalf("ToolByName(%q) = false, want true", want.Name)
		}
		if got.Name != want.Name || got.Description != want.Description {
			t.Fatalf("ToolByName(%q) = %#v, want %#v", want.Name, got, want)
		}
	}
}

func TestIsCanonicalToolHandlerRegistered_ReportsQueryStatusOnly(t *testing.T) {
	t.Parallel()

	if !mcprecording.IsCanonicalToolHandlerRegistered(mcprecording.ToolQueryStatus) {
		t.Fatal("query_status handler should be registered")
	}
	for _, name := range []string{
		mcprecording.ToolAppendEvent,
		mcprecording.ToolLoadReplay,
		mcprecording.ToolReadPortableArtifact,
	} {
		if mcprecording.IsCanonicalToolHandlerRegistered(name) {
			t.Fatalf("handler for %q should not be registered yet", name)
		}
	}
}
