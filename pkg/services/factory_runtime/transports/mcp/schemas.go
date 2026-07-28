package mcp

func objectSchema(properties map[string]any, required ...string) map[string]any {
	schema := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties":           properties,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

func stringProperty(description string) map[string]any {
	return map[string]any{
		"type":        "string",
		"description": description,
	}
}

func integerProperty(description string) map[string]any {
	return map[string]any{
		"type":        "integer",
		"description": description,
	}
}

func enumStringProperty(description string, values ...string) map[string]any {
	return map[string]any{
		"type":        "string",
		"description": description,
		"enum":        values,
	}
}

func toolResponseSchema(resultSchema map[string]any) map[string]any {
	return objectSchema(map[string]any{
		"result": resultSchema,
		"error":  toolErrorEnvelopeSchema(),
	})
}

func toolErrorEnvelopeSchema() map[string]any {
	return objectSchema(map[string]any{
		"code":      stringProperty("Stable machine-readable failure code."),
		"message":   stringProperty("Human-readable failure summary for MCP hosts."),
		"retryable": map[string]any{"type": "boolean", "description": "Whether the caller should retry the same request later."},
		"details": map[string]any{
			"type":                 "object",
			"additionalProperties": true,
			"description":          "Optional structured diagnostics such as validation issues or availability hints.",
		},
	}, "code", "message", "retryable")
}

func controlPauseResultSchema() map[string]any {
	return objectSchema(map[string]any{
		"Outcome": enumStringProperty(
			"Plain pause control outcome published at the Factory Runtime root.",
			"ACCEPTED", "NO_OP",
		),
	}, "Outcome")
}

func observationScopeSchema() map[string]any {
	return enumStringProperty(
		"Live observation scope. Empty means FULL at the Runtime root.",
		"FULL", "STATUS", "PROGRESS", "DISPATCHES", "RESULTS", "RESOURCES", "HEALTH",
	)
}

func observationSchema() map[string]any {
	return objectSchema(map[string]any{
		"Status": enumStringProperty(
			"Plain live operational status vocabulary.",
			"ACTIVE", "IDLE", "FINISHED",
		),
		"Progress": objectSchema(map[string]any{
			"InFlightDispatchCount": integerProperty("Count of in-flight dispatches."),
			"TickCount":             integerProperty("Logical engine tick count."),
			"TotalWorkCount":        integerProperty("Total customer Work count."),
		}),
		"InFlightDispatches": map[string]any{
			"type": "array",
			"items": objectSchema(map[string]any{
				"DispatchID":      stringProperty("Dispatch identifier."),
				"WorkIDs":         map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				"WorkstationName": stringProperty("Workstation name."),
				"Status":          stringProperty("Dispatch status."),
			}),
		},
		"Results": map[string]any{
			"type": "array",
			"items": objectSchema(map[string]any{
				"DispatchID": stringProperty("Dispatch identifier."),
				"WorkID":     stringProperty("Work identifier."),
				"Outcome":    stringProperty("Result outcome."),
			}),
		},
		"Resources": map[string]any{
			"type": "array",
			"items": objectSchema(map[string]any{
				"ResourceID":     stringProperty("Resource identifier."),
				"ResourceName":   stringProperty("Resource name."),
				"ResourceType":   stringProperty("Resource type."),
				"InUseCount":     integerProperty("Resources in use."),
				"AvailableCount": integerProperty("Resources available."),
			}),
		},
		"Health": objectSchema(map[string]any{
			"FactoryState":           stringProperty("Hosted factory state."),
			"LifecycleControlStatus": stringProperty("Lifecycle control status."),
			"StreamGenerationID":     stringProperty("Stream generation identifier."),
		}),
	})
}

func observeResultSchema() map[string]any {
	return objectSchema(map[string]any{
		"Observation": observationSchema(),
	}, "Observation")
}
