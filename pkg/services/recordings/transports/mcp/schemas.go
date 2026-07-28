package recordingmcp

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

func toolResponseSchema(resultSchema map[string]any) map[string]any {
	return objectSchema(map[string]any{
		"result": resultSchema,
		"error":  toolErrorEnvelopeSchema(),
	})
}

func toolErrorEnvelopeSchema() map[string]any {
	return objectSchema(map[string]any{
		"code":        stringProperty("Stable machine-readable failure code."),
		"message":     stringProperty("Human-readable failure summary for MCP hosts."),
		"retryable":   map[string]any{"type": "boolean", "description": "Whether the caller should retry the same request later."},
		"recordingId": stringProperty("Recording identifier when the failure is scoped to one recording."),
		"details": map[string]any{
			"type":                 "object",
			"additionalProperties": true,
			"description":          "Optional structured diagnostics such as validation issues or availability hints.",
		},
	}, "code", "message", "retryable")
}

func canonicalEventScopeSchema() map[string]any {
	return objectSchema(map[string]any{
		"factorySessionId": stringProperty("Factory Session whose canonical history contains the event."),
	})
}

func canonicalEventCursorSchema() map[string]any {
	return objectSchema(map[string]any{
		"streamGenerationId": stringProperty("Stream generation that distinguishes overlapping numeric sequences."),
		"sequence":           integerProperty("Global canonical event sequence position."),
	}, "streamGenerationId", "sequence")
}

func canonicalEventSchema() map[string]any {
	return objectSchema(map[string]any{
		"id":            stringProperty("Canonical event identity."),
		"sequence":      integerProperty("Recordings-assigned global canonical sequence."),
		"factoryTick":   integerProperty("Factory tick associated with the event."),
		"scope":         canonicalEventScopeSchema(),
		"cursor":        canonicalEventCursorSchema(),
		"recordedAt":    stringProperty("RFC3339 timestamp when the event was recorded."),
		"kind":          stringProperty("Detached canonical event kind."),
		"payload":       stringProperty("Immutable JSON payload text for the canonical event."),
		"sourceContext": stringProperty("Detached producer correlation metadata."),
	}, "id", "kind", "payload")
}

func queryStatusInputSchema() map[string]any {
	return objectSchema(map[string]any{
		"recordingId": stringProperty("Stable recording identifier."),
	}, "recordingId")
}

func appendEventInputSchema() map[string]any {
	return objectSchema(map[string]any{
		"event": canonicalEventSchema(),
	}, "event")
}

func loadReplayInputSchema() map[string]any {
	return objectSchema(map[string]any{
		"recordingId": stringProperty("Stable recording identifier for neutral replay selection."),
	}, "recordingId")
}

func readPortableArtifactInputSchema() map[string]any {
	return objectSchema(map[string]any{
		"recordingId": stringProperty("Recording identifier for the export scope."),
		"reference":   stringProperty("Published portable artifact reference."),
	}, "recordingId", "reference")
}

func recordingStatusFactsSchema() map[string]any {
	return objectSchema(map[string]any{
		"RecordingID":    stringProperty("Stable recording identifier."),
		"Artifact":       stringProperty("Opaque portable artifact reference."),
		"Scope":          canonicalEventScopeSchema(),
		"State":          stringProperty("Observable recording lifecycle state."),
		"AcceptedEvents": integerProperty("Count of accepted canonical events."),
		"LastEvent":      canonicalEventCursorSchema(),
		"FlushedThrough": canonicalEventCursorSchema(),
		"FinalizedAt":    stringProperty("RFC3339 timestamp when the recording finalized."),
	})
}

func recordingStatusResultSchema() map[string]any {
	return objectSchema(map[string]any{
		"Status": recordingStatusFactsSchema(),
	})
}

func appendRecordedEventResultSchema() map[string]any {
	return objectSchema(map[string]any{
		"Event": canonicalEventSchema(),
	})
}

func replayRecordingFactsSchema() map[string]any {
	return objectSchema(map[string]any{
		"RecordingID": stringProperty("Stable recording identifier."),
		"Scope":     canonicalEventScopeSchema(),
		"Events": map[string]any{
			"type":        "array",
			"description": "Detached canonical facts selected for replay.",
			"items":       canonicalEventSchema(),
		},
	})
}

func loadReplayRecordingResultSchema() map[string]any {
	return objectSchema(map[string]any{
		"Recording": replayRecordingFactsSchema(),
	})
}

func portableArtifactSummarySchema() map[string]any {
	return objectSchema(map[string]any{
		"recordingId": stringProperty("Stable recording identifier."),
		"reference":   stringProperty("Opaque portable artifact reference."),
		"scope":       canonicalEventScopeSchema(),
		"state":       stringProperty("Observable recording lifecycle state."),
		"eventCount":  integerProperty("Count of canonical events in the artifact."),
		"available":   map[string]any{"type": "boolean", "description": "Whether the artifact is available for read."},
	})
}

func portableArtifactSchema() map[string]any {
	return objectSchema(map[string]any{
		"schemaVersion": stringProperty("Portable artifact schema version."),
		"summary":       portableArtifactSummarySchema(),
		"events": map[string]any{
			"type":        "array",
			"description": "Detached canonical events preserved in artifact order.",
			"items":       canonicalEventSchema(),
		},
		"integrity": objectSchema(map[string]any{
			"algorithm": stringProperty("Integrity algorithm identifier."),
			"digest":    stringProperty("Completed artifact digest."),
		}),
	})
}

func readPortableArtifactResultSchema() map[string]any {
	return objectSchema(map[string]any{
		"Artifact": portableArtifactSchema(),
	})
}
