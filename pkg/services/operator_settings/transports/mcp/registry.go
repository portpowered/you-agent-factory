package operatorsettingsmcp

// DiscoverTools returns the canonical Operator Settings MCP tool catalog in
// stable discovery order. Schemas mirror accepted Operator Settings root request
// contracts.
func DiscoverTools() []ToolDefinition {
	return []ToolDefinition{
		loadDocumentTool(),
		applyDocumentUpdateTool(),
		resolveEffectiveTool(),
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
// registers a handler for one canonical Operator Settings tool name.
func IsCanonicalToolHandlerRegistered(name string) bool {
	_, ok := canonicalToolHandlers[name]
	return ok
}

func loadDocumentTool() ToolDefinition {
	return ToolDefinition{
		Name: ToolLoadDocument,
		Description: "Load the operator settings document at one path through the accepted " +
			"Operator Settings root without constructing Operator Settings internals.",
		InputSchema:  loadDocumentInputSchema(),
		OutputSchema: toolResponseSchema(loadDocumentResultSchema()),
		SuccessStableFields: []string{
			"result.Document",
			"result.Document.BackendScopeID",
			"result.Document.Defaults",
			"result.Path",
			"result.Found",
		},
		ErrorStableFields: sharedErrorStableFields,
	}
}

func applyDocumentUpdateTool() ToolDefinition {
	return ToolDefinition{
		Name: ToolApplyDocumentUpdate,
		Description: "Apply one semantic operator document update through the accepted " +
			"Operator Settings root without constructing Operator Settings internals.",
		InputSchema:  applyDocumentUpdateInputSchema(),
		OutputSchema: toolResponseSchema(applyDocumentUpdateResultSchema()),
		SuccessStableFields: []string{
			"result.Document",
			"result.Document.Defaults",
			"result.Path",
			"result.Persisted",
		},
		ErrorStableFields: sharedErrorStableFields,
	}
}

func resolveEffectiveTool() ToolDefinition {
	return ToolDefinition{
		Name: ToolResolveEffective,
		Description: "Resolve effective operator defaults from detached baseline and override " +
			"facts through the accepted Operator Settings root without mutating the document.",
		InputSchema:  resolveEffectiveInputSchema(),
		OutputSchema: toolResponseSchema(resolveEffectiveResultSchema()),
		SuccessStableFields: []string{
			"result.Selection",
			"result.Selection.WorkerModelProvider",
			"result.Selection.WorkerModel",
			"result.Selection.WorkerModelProviderSource",
			"result.Selection.WorkerModelSource",
			"result.Selection.ConfigPath",
		},
		ErrorStableFields: sharedErrorStableFields,
	}
}
