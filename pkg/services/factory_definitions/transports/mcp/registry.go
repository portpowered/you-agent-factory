package factorydefinition

// DiscoverTools returns the canonical Factory Definitions MCP tool catalog in
// stable discovery order. Schemas mirror HTTP-DEF validation and current-factory
// contracts.
func DiscoverTools() []ToolDefinition {
	return []ToolDefinition{
		validateTool(),
		getCurrentTool(),
		saveCurrentTool(),
		installPackagedTool(),
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

func validateTool() ToolDefinition {
	return ToolDefinition{
		Name: ToolValidate,
		Description: "Validate one submitted Factory definition through the canonical " +
			"Definitions validation contract (POST /factory-validations) without starting sessions " +
			"or mutating catalog state.",
		InputSchema:  factoryDefinitionInputSchema(),
		OutputSchema: toolResponseSchema(factoryValidationResultSchema()),
		SuccessStableFields: []string{
			"result.targets",
			"result.targets[].code",
			"result.targets[].message",
			"result.targets[].severity",
			"result.targets[].subject",
		},
		ErrorStableFields: sharedErrorStableFields,
	}
}

func getCurrentTool() ToolDefinition {
	return ToolDefinition{
		Name: ToolGetCurrent,
		Description: "Read the current Factory for one Factory Session through the accepted " +
			"Definitions root (GET /factory-sessions/{session_id}/factory).",
		InputSchema:  getCurrentFactoryInputSchema(),
		OutputSchema: toolResponseSchema(factoryPayloadSchema()),
		SuccessStableFields: []string{
			"result.name",
			"result.version",
			"result.workTypes",
			"result.workers",
			"result.workstations",
		},
		ErrorStableFields: sharedErrorStableFields,
	}
}

func saveCurrentTool() ToolDefinition {
	return ToolDefinition{
		Name: ToolSaveCurrent,
		Description: "Save the current Factory for one Factory Session through the accepted " +
			"Definitions root (PUT /factory-sessions/{session_id}/factory).",
		InputSchema:  saveCurrentFactoryInputSchema(),
		OutputSchema: toolResponseSchema(factoryPayloadSchema()),
		SuccessStableFields: []string{
			"result.name",
			"result.version",
			"result.workTypes",
			"result.workers",
			"result.workstations",
		},
		ErrorStableFields: sharedErrorStableFields,
	}
}

func installPackagedTool() ToolDefinition {
	return ToolDefinition{
		Name: ToolInstallPackaged,
		Description: "Install one built-in packaged Factory through the Definitions-owned " +
			"distribute contract without using the CLI adapter path.",
		InputSchema:  installPackagedFactoryInputSchema(),
		OutputSchema: toolResponseSchema(installPackagedFactoryResultSchema()),
		SuccessStableFields: []string{
			"result.name",
			"result.factoryDir",
			"result.outcome",
			"result.format",
		},
		ErrorStableFields: sharedErrorStableFields,
	}
}
