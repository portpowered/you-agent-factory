package workflow_test

import (
	"slices"
	"testing"

	mcpfactorysession "github.com/portpowered/infinite-you/pkg/mcp/factorysession"
	mcpworkflow "github.com/portpowered/infinite-you/pkg/mcp/workflow"
)

func TestDiscoverTools_ExposesPreviewCatalog(t *testing.T) {
	tools := mcpworkflow.DiscoverTools()
	if len(tools) != 2 {
		t.Fatalf("tool count = %d, want 2", len(tools))
	}

	wantNames := []string{
		mcpfactorysession.ToolValidateSource,
		mcpfactorysession.ToolStartPreview,
	}
	gotNames := mcpworkflow.ToolNames()
	if !slices.Equal(gotNames, wantNames) {
		t.Fatalf("tool names = %#v, want %#v", gotNames, wantNames)
	}
}

func TestDiscoverTools_UsesFactorySessionVocabulary(t *testing.T) {
	for _, tool := range mcpworkflow.DiscoverTools() {
		if tool.Name != mcpfactorysession.ToolValidateSource && tool.Name != mcpfactorysession.ToolStartPreview {
			t.Fatalf("unexpected tool name %q", tool.Name)
		}
		if len(tool.InputSchema) == 0 || len(tool.OutputSchema) == 0 {
			t.Fatalf("tool %q missing schemas", tool.Name)
		}
	}
}
