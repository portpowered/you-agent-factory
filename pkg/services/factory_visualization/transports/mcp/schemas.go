package factoryvisualization

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
			"description":          "Optional structured diagnostics such as validation issues or lifecycle hints.",
		},
	}, "code", "message", "retryable")
}

func activateInputSchema() map[string]any {
	return objectSchema(map[string]any{
		"mode": enumStringProperty(
			"Activation mode that leaves the inert Visualization root through retained-then-live projection.",
			"RETAINED_THEN_LIVE",
		),
	}, "mode")
}

func joinInputSchema() map[string]any {
	return objectSchema(map[string]any{})
}

func stopDrainInputSchema() map[string]any {
	return objectSchema(map[string]any{})
}

func observeInputSchema() map[string]any {
	return objectSchema(map[string]any{
		"mode": enumStringProperty(
			"Observe mode for one detached retained-then-live Factory view projection.",
			"RETAINED_THEN_LIVE",
		),
		"reconnect": objectSchema(map[string]any{
			"afterEventId":  stringProperty("Resume observation after this retained event identifier."),
			"afterSequence": integerProperty("Resume observation after this retained event sequence."),
		}),
	}, "mode")
}

func openPresentationInputSchema() map[string]any {
	return objectSchema(map[string]any{
		"mode": enumStringProperty(
			"Presentation delivery mode selecting Visualization-owned drain/backpressure policy.",
			"BEST_EFFORT", "LOSSLESS",
		),
	}, "mode")
}

func presentProgressInputSchema() map[string]any {
	return objectSchema(map[string]any{
		"sessionId": stringProperty("Opened Visualization presentation session identifier."),
		"records": map[string]any{
			"type":        "array",
			"description": "Ordered progress records to enqueue onto the presentation session.",
			"items": objectSchema(map[string]any{
				"payload": stringProperty("Base64-encoded progress payload bytes."),
			}, "payload"),
		},
	}, "sessionId", "records")
}

func finalizePresentationInputSchema() map[string]any {
	return objectSchema(map[string]any{
		"sessionId": stringProperty("Opened Visualization presentation session identifier."),
		"terminal": objectSchema(map[string]any{
			"payload": stringProperty("Base64-encoded terminal payload committed after drain."),
		}, "payload"),
	}, "sessionId")
}

func closePresentationInputSchema() map[string]any {
	return objectSchema(map[string]any{
		"sessionId": stringProperty("Opened Visualization presentation session identifier."),
	}, "sessionId")
}

func activateResultSchema() map[string]any {
	return objectSchema(map[string]any{
		"State": enumStringProperty("Published Visualization lifecycle state.", "INERT", "STARTED", "STOPPED"),
	}, "State")
}

func joinResultSchema() map[string]any {
	return activateResultSchema()
}

func stopDrainResultSchema() map[string]any {
	return activateResultSchema()
}

func observeResultSchema() map[string]any {
	return objectSchema(map[string]any{
		"View": objectSchema(map[string]any{
			"TickCount":          integerProperty("Projected tick count in the detached view."),
			"RetainedEventCount": integerProperty("Retained event count included in the projection."),
			"ObservedAt":         stringProperty("RFC3339 timestamp when the view was observed."),
		}),
	})
}

func openPresentationResultSchema() map[string]any {
	return objectSchema(map[string]any{
		"SessionID": stringProperty("Opened presentation session identifier."),
		"Mode": enumStringProperty(
			"Presentation delivery mode for the opened session.",
			"BEST_EFFORT", "LOSSLESS",
		),
	})
}

func presentProgressResultSchema() map[string]any {
	return objectSchema(map[string]any{
		"AcceptedCount": integerProperty("Number of progress records accepted by the presentation session."),
	})
}

func finalizePresentationResultSchema() map[string]any {
	return objectSchema(map[string]any{
		"Finalized":    map[string]any{"type": "boolean", "description": "Whether finalize committed a terminal write."},
		"ProgressSeen": map[string]any{"type": "boolean", "description": "Whether any progress was accepted before finalize."},
	})
}

func closePresentationResultSchema() map[string]any {
	return objectSchema(map[string]any{
		"DroppedCount": integerProperty("Number of progress records dropped during close-and-drain."),
	})
}
