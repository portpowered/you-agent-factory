package workflow

import mcpfactorysession "github.com/portpowered/infinite-you/pkg/mcp/factorysession"

// DiscoverTools returns the workflow preview MCP tool catalog served by the
// canonical you mcp serve entrypoint.
func DiscoverTools() []mcpfactorysession.ToolDefinition {
	return mcpfactorysession.PreviewToolDefinitions()
}

// ToolNames returns stable preview tool names in discovery order.
func ToolNames() []string {
	tools := DiscoverTools()
	names := make([]string, len(tools))
	for i, tool := range tools {
		names[i] = tool.Name
	}
	return names
}
