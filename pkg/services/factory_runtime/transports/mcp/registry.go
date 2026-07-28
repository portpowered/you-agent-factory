package mcp

// DiscoverTools returns the canonical Factory Runtime MCP tool catalog in stable
// discovery order. Schemas mirror the accepted Runtime root contract vocabulary.
func DiscoverTools() []ToolDefinition {
	return []ToolDefinition{
		controlPauseTool(),
		observeTool(),
		planDispatchTool(),
		acceptDispatchResultTool(),
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

func controlPauseTool() ToolDefinition {
	return ToolDefinition{
		Name: ToolControlPause,
		Description: "Pause one live Factory Runtime instance through the accepted Runtime root " +
			"pause control contract without importing Runtime internals.",
		InputSchema:  objectSchema(map[string]any{}),
		OutputSchema: toolResponseSchema(controlPauseResultSchema()),
		SuccessStableFields: []string{
			"result.Outcome",
		},
		ErrorStableFields: sharedErrorStableFields,
	}
}

func observeTool() ToolDefinition {
	return ToolDefinition{
		Name: ToolObserve,
		Description: "Return one live Factory Runtime observation through the accepted Runtime root " +
			"observe contract with orchestration-neutral status, progress, and resource views.",
		InputSchema: objectSchema(map[string]any{
			"scope": observationScopeSchema(),
		}),
		OutputSchema: toolResponseSchema(observeResultSchema()),
		SuccessStableFields: []string{
			"result.Observation",
			"result.Observation.Status",
			"result.Observation.Progress",
			"result.Observation.InFlightDispatches",
			"result.Observation.Results",
			"result.Observation.Resources",
			"result.Observation.Health",
		},
		ErrorStableFields: sharedErrorStableFields,
	}
}

func planDispatchTool() ToolDefinition {
	return ToolDefinition{
		Name: ToolPlanDispatch,
		Description: "Publish one dispatch intent through the accepted Runtime root plan-dispatch " +
			"contract without importing Runtime internals.",
		InputSchema: objectSchema(map[string]any{
			"dispatchId":      stringProperty("Stable dispatch identifier for the planned intent."),
			"correlationId":   stringProperty("Correlation identifier linking the planned intent to later results."),
			"workIds":         stringArrayProperty("Customer Work identifiers included in the dispatch intent."),
			"workstationName": stringProperty("Target workstation name for the dispatch intent."),
			"workerType":      stringProperty("Worker type selected for the dispatch intent."),
			"replayKey":       stringProperty("Replay key used for idempotent dispatch planning."),
		}, "dispatchId", "correlationId", "workIds", "workstationName", "workerType", "replayKey"),
		OutputSchema: toolResponseSchema(planDispatchResultSchema()),
		SuccessStableFields: []string{
			"result.Outcome",
			"result.DispatchID",
			"result.CorrelationID",
		},
		ErrorStableFields: sharedErrorStableFields,
	}
}

func acceptDispatchResultTool() ToolDefinition {
	return ToolDefinition{
		Name: ToolAcceptDispatchResult,
		Description: "Accept or retire one correlated worker result through the accepted Runtime root " +
			"accept-dispatch-result contract without importing Runtime internals.",
		InputSchema: objectSchema(map[string]any{
			"dispatchId":    stringProperty("Dispatch identifier for the correlated result."),
			"correlationId": stringProperty("Correlation identifier for the planned dispatch intent."),
			"workId":        stringProperty("Work identifier for the correlated worker result."),
			"resultOutcome": dispatchResultOutcomeSchema(),
		}, "dispatchId", "correlationId", "workId", "resultOutcome"),
		OutputSchema: toolResponseSchema(acceptDispatchResultResultSchema()),
		SuccessStableFields: []string{
			"result.Outcome",
			"result.DispatchID",
			"result.CorrelationID",
		},
		ErrorStableFields: sharedErrorStableFields,
	}
}
