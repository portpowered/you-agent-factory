package workmcp

// DiscoverTools returns the canonical Work MCP tool catalog in stable discovery
// order. Schemas mirror accepted Work root request and result contracts.
func DiscoverTools() []ToolDefinition {
	return []ToolDefinition{
		submitTool(),
		listTool(),
		getTool(),
		moveTool(),
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
// registers a handler for one canonical Work tool name.
func IsCanonicalToolHandlerRegistered(name string) bool {
	_, ok := canonicalToolHandlers[name]
	return ok
}

func submitTool() ToolDefinition {
	return ToolDefinition{
		Name: ToolSubmit,
		Description: "Submit one already-decoded Work Request through the accepted " +
			"Work root admission slice without constructing Work internals.",
		InputSchema:  submitInputSchema(),
		OutputSchema: toolResponseSchema(workRequestSubmitResultSchema()),
		SuccessStableFields: []string{
			"result.RequestID",
			"result.TraceID",
			"result.WorkID",
			"result.Accepted",
			"result.Works",
		},
		ErrorStableFields: sharedErrorStableFields,
	}
}

func listTool() ToolDefinition {
	return ToolDefinition{
		Name: ToolList,
		Description: "List Work items for one Factory Session through the accepted " +
			"Work root state-access slice without HTTP or CLI transport paths.",
		InputSchema:  listInputSchema(),
		OutputSchema: toolResponseSchema(listResultSchema()),
		SuccessStableFields: []string{
			"result.Results",
			"result.Results[].WorkID",
			"result.Results[].WorkTypeName",
			"result.MaxResults",
			"result.NextToken",
		},
		ErrorStableFields: sharedErrorStableFields,
	}
}

func getTool() ToolDefinition {
	return ToolDefinition{
		Name: ToolGet,
		Description: "Read one detached Work ReadModel through the accepted Work root " +
			"state-access slice without constructing Work internals.",
		InputSchema:  getInputSchema(),
		OutputSchema: toolResponseSchema(readModelSchema()),
		SuccessStableFields: []string{
			"result.WorkID",
			"result.WorkTypeName",
			"result.State",
			"result.Name",
		},
		ErrorStableFields: sharedErrorStableFields,
	}
}

func moveTool() ToolDefinition {
	return ToolDefinition{
		Name: ToolMove,
		Description: "Move one Work item to another authored state through the accepted " +
			"Work root state-access slice without constructing Work internals.",
		InputSchema:  moveInputSchema(),
		OutputSchema: toolResponseSchema(operatorMoveResultSchema()),
		SuccessStableFields: []string{
			"result.WorkID",
			"result.FromState",
			"result.ToState",
		},
		ErrorStableFields: sharedErrorStableFields,
	}
}
