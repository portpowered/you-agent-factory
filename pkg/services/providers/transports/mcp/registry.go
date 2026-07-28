package providersmcp

// DiscoverTools returns the canonical Providers MCP tool catalog in stable
// discovery order. Schemas mirror accepted Providers root request contracts.
func DiscoverTools() []ToolDefinition {
	return []ToolDefinition{
		listProvidersTool(),
		getProviderTool(),
		executeTool(),
	}
}

// ToolNames returns stable canonical tool names in discovery order.
func ToolNames() []string {
	tools := DiscoverTools()
	names := make([]string, len(tools))
	for i, tool := range tools {
		names[i] = tool.Name
	}
	return names
}

// ToolByName returns one canonical tool definition by stable name.
func ToolByName(name string) (ToolDefinition, bool) {
	for _, tool := range DiscoverTools() {
		if tool.Name == name {
			return tool, true
		}
	}
	return ToolDefinition{}, false
}

// IsCanonicalToolHandlerRegistered reports whether the live CallTool path
// registers a handler for one canonical Providers tool name.
func IsCanonicalToolHandlerRegistered(name string) bool {
	_, ok := canonicalToolHandlers[name]
	return ok
}

func listProvidersTool() ToolDefinition {
	return ToolDefinition{
		Name: ToolListProviders,
		Description: "List detached provider catalog descriptors through the accepted " +
			"Providers root without constructing Providers internals.",
		InputSchema:  listProvidersInputSchema(),
		OutputSchema: toolResponseSchema(listProvidersResultSchema()),
		SuccessStableFields: []string{
			"result.Providers",
			"result.Providers[].ID",
			"result.Providers[].DisplayName",
			"result.Providers[].Availability",
			"result.Providers[].Readiness",
			"result.Providers[].Capabilities",
		},
		ErrorStableFields: sharedErrorStableFields,
	}
}

func getProviderTool() ToolDefinition {
	return ToolDefinition{
		Name: ToolGetProvider,
		Description: "Look up one detached provider catalog descriptor by Providers-owned " +
			"identity through the accepted Providers root without constructing Providers internals.",
		InputSchema:  getProviderInputSchema(),
		OutputSchema: toolResponseSchema(getProviderResultSchema()),
		SuccessStableFields: []string{
			"result.Provider",
			"result.Provider.ID",
			"result.Provider.DisplayName",
			"result.Provider.Availability",
			"result.Provider.Readiness",
			"result.Provider.Capabilities",
		},
		ErrorStableFields: sharedErrorStableFields,
	}
}

func executeTool() ToolDefinition {
	return ToolDefinition{
		Name: ToolExecute,
		Description: "Perform exactly one normalized provider execution attempt through the " +
			"accepted Providers root and return detached content and optional session facts.",
		InputSchema:  executeInputSchema(),
		OutputSchema: toolResponseSchema(executeResultSchema()),
		SuccessStableFields: []string{
			"result.Content",
			"result.SessionRef",
			"result.Diagnostics",
		},
		ErrorStableFields: sharedErrorStableFields,
	}
}
