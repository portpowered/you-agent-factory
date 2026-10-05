package workersessionmcp

// ToolDefinition describes one adapter-owned input contract.
type ToolDefinition struct {
	Name        string
	InputSchema map[string]any
}

// DiscoverTools projects the three supported Worker Session input contracts.
func DiscoverTools() []ToolDefinition {
	return []ToolDefinition{
		{Name: ToolList, InputSchema: listSchema()},
		{Name: ToolRead, InputSchema: readSchema()},
		{Name: ToolControl, InputSchema: controlSchema()},
	}
}

func listSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"history": map[string]any{
				"type":    "string",
				"enum":    []any{"active", "all", "archived"},
				"default": "all",
			},
			"scope": map[string]any{
				"type": "string",
				"enum": []any{"direct", "factory", "all"},
			},
			"state": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "string",
					"enum": []any{"RESERVED", "STARTING", "RUNNING", "PAUSED", "COMPLETED", "FAILED", "CANCELED", "TERMINATED"},
				},
			},
			"limit": map[string]any{
				"type":    "integer",
				"minimum": 1,
			},
			"nextToken": map[string]any{
				"type":      "string",
				"minLength": 1,
			},
		},
	}
}

func readSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []any{"workerSessionId"},
		"properties": map[string]any{
			"workerSessionId": map[string]any{
				"type":      "string",
				"minLength": 1,
			},
			"view": map[string]any{
				"type":    "string",
				"enum":    []any{"summary", "transcript", "events", "logs"},
				"default": "summary",
			},
			"limit": map[string]any{
				"type":    "integer",
				"minimum": 1,
				"maximum": 1000,
				"default": 100,
			},
			"nextToken": map[string]any{
				"type":      "string",
				"minLength": 1,
			},
		},
	}
}

func controlSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []any{"workerSessionId", "operation"},
		"properties": map[string]any{
			"workerSessionId": map[string]any{
				"type":      "string",
				"minLength": 1,
			},
			"operation": map[string]any{
				"type": "string",
				"enum": []any{"CANCEL", "TERMINATE", "INTERRUPT"},
			},
			"requestId": map[string]any{
				"type":      "string",
				"minLength": 1,
			},
			"successorWorkerSessionId": map[string]any{
				"type":      "string",
				"minLength": 1,
			},
			"replacementMessage": map[string]any{
				"type":      "string",
				"minLength": 1,
			},
		},
	}
}
