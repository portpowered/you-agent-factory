package providersmcp

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

func booleanProperty(description string) map[string]any {
	return map[string]any{
		"type":        "boolean",
		"description": description,
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

func prerequisiteSchema() map[string]any {
	return objectSchema(map[string]any{
		"Kind":        stringProperty("Prerequisite kind such as configuration, credential, or dependency."),
		"Name":        stringProperty("Prerequisite name."),
		"Status":      stringProperty("Whether the prerequisite is satisfied or missing."),
		"Description": stringProperty("Bounded setup guidance for the prerequisite."),
	})
}

func providerDescriptorSchema() map[string]any {
	return objectSchema(map[string]any{
		"ID":            stringProperty("Providers-owned canonical provider identity."),
		"Aliases":       map[string]any{"type": "array", "items": stringProperty("Alternate provider identity string.")},
		"DisplayName":   stringProperty("Human-readable provider display name."),
		"Availability":  stringProperty("Catalog availability posture for the provider."),
		"Readiness":     stringProperty("Current provider readiness outcome."),
		"Prerequisites": map[string]any{"type": "array", "items": prerequisiteSchema()},
		"Capabilities":  map[string]any{"type": "array", "items": stringProperty("Provider-neutral capability identifier.")},
	})
}

func sessionRefSchema() map[string]any {
	return objectSchema(map[string]any{
		"Provider": stringProperty("Providers-owned provider identity for the session ref."),
		"Kind":     stringProperty("Session ref kind such as session_id."),
		"ID":       stringProperty("Detached provider session identifier."),
	}, "Provider", "Kind", "ID")
}

func executeProgressSchema() map[string]any {
	return objectSchema(map[string]any{
		"Phase":    stringProperty("Bounded in-flight progress phase."),
		"Detail":   stringProperty("Bounded progress detail."),
		"Metadata": map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string"}},
	})
}

func executeDiagnosticsSchema() map[string]any {
	return objectSchema(map[string]any{
		"DurationMillis": integerProperty("Sanitized attempt duration in milliseconds."),
		"Progress":       map[string]any{"type": "array", "items": executeProgressSchema()},
		"Metadata":       map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string"}},
	})
}

func integerProperty(description string) map[string]any {
	return map[string]any{
		"type":        "integer",
		"description": description,
	}
}

func listProvidersInputSchema() map[string]any {
	return objectSchema(map[string]any{})
}

func listProvidersResultSchema() map[string]any {
	return objectSchema(map[string]any{
		"Providers": map[string]any{
			"type":        "array",
			"description": "Detached provider catalog descriptors.",
			"items":       providerDescriptorSchema(),
		},
	})
}

func getProviderInputSchema() map[string]any {
	return objectSchema(map[string]any{
		"id": stringProperty("Providers-owned canonical provider identity."),
	}, "id")
}

func getProviderResultSchema() map[string]any {
	return objectSchema(map[string]any{
		"Provider": providerDescriptorSchema(),
	})
}

func executeInputSchema() map[string]any {
	return objectSchema(map[string]any{
		"provider":        stringProperty("Providers-owned canonical provider identity."),
		"attemptId":       stringProperty("Caller-owned attempt identifier for one normalized execution."),
		"workerType":      stringProperty("Optional worker type context for the attempt."),
		"workstationName": stringProperty("Optional workstation name context for the attempt."),
		"model":           stringProperty("Optional model identifier for the attempt."),
		"skipPermissions": booleanProperty("Whether provider permission prompts should be skipped."),
		"systemPrompt":    stringProperty("Optional system prompt for the attempt."),
		"userMessage":     stringProperty("Optional user message for the attempt."),
		"inputTokens": map[string]any{
			"type":        "array",
			"description": "Optional structured input tokens for the attempt.",
			"items":       map[string]any{},
		},
		"outputSchema":     stringProperty("Optional structured output schema for the attempt."),
		"resumeSession":    sessionRefInputSchema(),
		"workingDirectory": stringProperty("Optional working directory for the attempt."),
		"worktree":         stringProperty("Optional worktree path for the attempt."),
		"envVars": map[string]any{
			"type":                 "object",
			"additionalProperties": map[string]any{"type": "string"},
			"description":          "Optional environment variables for the attempt.",
		},
		"processEnvironment": map[string]any{
			"type":        "array",
			"description": "Optional process environment entries for the attempt.",
			"items":       stringProperty("Process environment entry."),
		},
	}, "provider", "attemptId")
}

func sessionRefInputSchema() map[string]any {
	return objectSchema(map[string]any{
		"provider": stringProperty("Providers-owned provider identity for the session ref."),
		"kind":     stringProperty("Session ref kind such as session_id."),
		"id":       stringProperty("Detached provider session identifier."),
	})
}

func executeResultSchema() map[string]any {
	return objectSchema(map[string]any{
		"Content":     stringProperty("Detached provider attempt content."),
		"SessionRef":  sessionRefSchema(),
		"Diagnostics": executeDiagnosticsSchema(),
	})
}
