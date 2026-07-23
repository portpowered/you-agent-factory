package factorysession

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
		"sessionId": stringProperty("Factory Session identifier when the failure is scoped to one session."),
		"details": map[string]any{
			"type":                 "object",
			"additionalProperties": true,
			"description":          "Optional structured diagnostics such as validation issues or availability hints.",
		},
	}, "code", "message", "retryable")
}

func factoryPreviewRequestSchema() map[string]any {
	return objectSchema(map[string]any{
		"sourceKind": enumStringProperty(
			"JavaScript orchestrator factory source request kind for Factory preview (POST /factories/preview).",
			"FACTORY_ID", "FACTORY_INLINE", "WORKFLOW_FILE", "WORKFLOW_NAME", "INLINE_WORKFLOW",
		),
		"sourceValue":        stringProperty("Requested workflow name, file ref, factory id, or inline label."),
		"inlineSource":       stringProperty("Inline orchestrator source text for INLINE_WORKFLOW or FACTORY_INLINE requests."),
		"artifactRoot":       stringProperty("Optional absolute artifact root requested with the factory source."),
		"allowFactoryLookup": booleanProperty("When true, explicit factory lookup is attempted after ordered workflow lookup."),
		"projectRoot":        stringProperty("Project root used for ordered JavaScript orchestrator source lookup."),
		"metadata": map[string]any{
			"type":                 "object",
			"additionalProperties": map[string]any{"type": "string"},
			"description":          "Optional JavaScript orchestrator metadata to validate with the source.",
		},
		"argsSchema": map[string]any{
			"type":                 "object",
			"additionalProperties": true,
			"description":          "Optional JSON Schema object describing Factory Session invocation arguments.",
		},
		"defaultPolicy": map[string]any{
			"type":                 "object",
			"additionalProperties": true,
			"description":          "Optional factory default policy layer merged into the effective policy preview.",
		},
		"requestedPolicy": map[string]any{
			"type":                 "object",
			"additionalProperties": true,
			"description":          "Optional request policy overrides merged into the effective policy preview.",
		},
	}, "sourceKind")
}

func factoryPreviewResultSchema() map[string]any {
	return objectSchema(map[string]any{
		"valid": booleanProperty("Whether the Factory preview found no blocking issues."),
		"sourceResolution": objectSchema(map[string]any{
			"sourceHash": stringProperty("Stable hash of the resolved workflow or factory source."),
			"sourceRef":  stringProperty("Resolved public source reference."),
		}),
		"policyPreview": objectSchema(map[string]any{
			"policyHash": stringProperty("Stable hash of the effective approved orchestrator policy preview."),
		}),
		"sourceValidationIssues": map[string]any{
			"type": "array",
			"items": objectSchema(map[string]any{
				"code":    stringProperty("Validation issue code."),
				"message": stringProperty("Validation issue message."),
				"path":    stringProperty("Optional source path for the issue."),
			}),
		},
	}, "valid")
}

func factorySessionExecutionRequestSchema() map[string]any {
	return objectSchema(map[string]any{
		"requestId": stringProperty("Caller-supplied idempotency key for durable Factory Session start."),
		"source": objectSchema(map[string]any{
			"kind": enumStringProperty(
				"Durable execution source kind.",
				"FACTORY_ID", "FACTORY_INLINE", "WORKFLOW_FILE", "WORKFLOW_NAME", "INLINE_WORKFLOW",
			),
			"factoryId":      stringProperty("Factory id when kind is FACTORY_ID."),
			"workflowFile":   stringProperty("Workflow file ref when kind is WORKFLOW_FILE."),
			"workflowName":   stringProperty("Workflow name when kind is WORKFLOW_NAME."),
			"inlineWorkflow": map[string]any{"type": "object", "description": "Inline workflow payload when kind is INLINE_WORKFLOW."},
		}, "kind"),
		"args": map[string]any{
			"type":                 "object",
			"additionalProperties": true,
			"description":          "Structured Factory Session invocation arguments validated by the resolved source.",
		},
		"orchestrator": map[string]any{
			"type":        "object",
			"description": "Optional orchestrator override when the resolved source does not fully determine orchestration.",
		},
		"requestedPolicy": map[string]any{
			"type":        "object",
			"description": "Caller-requested orchestrator policy before approval, if required.",
		},
		"wait": objectSchema(map[string]any{
			"timeoutMillis":   integerProperty("Sync wait timeout in milliseconds."),
			"cancelOnTimeout": booleanProperty("Whether to cancel the session when sync wait times out."),
		}),
	}, "requestId", "source")
}

func factorySessionExecutionResponseSchema() map[string]any {
	return objectSchema(map[string]any{
		"sessionId":           stringProperty("Stable durable Factory Session identifier."),
		"status":              durableLifecycleStatusSchema(),
		"orchestratorKind":    orchestratorKindSchema(),
		"resolvedSource":      resolvedSourceIdentitySchema(),
		"sourceHash":          stringProperty("Stable hash of the resolved workflow or factory source."),
		"effectivePolicyHash": stringProperty("Stable hash of the effective approved orchestrator policy."),
		"links": objectSchema(map[string]any{
			"session":    stringProperty("Relative URL for GET /factory-sessions/{session_id}."),
			"results":    stringProperty("Relative URL for GET /factory-sessions/{session_id}/results."),
			"dispatches": stringProperty("Relative URL for GET /factory-sessions/{session_id}/dispatches."),
			"artifacts":  stringProperty("Relative URL for GET /factory-sessions/{session_id}/artifacts."),
			"events":     stringProperty("Relative URL for GET /factory-sessions/{session_id}/events."),
		}),
	}, "sessionId", "status", "orchestratorKind", "resolvedSource")
}

func factorySessionSyncExecutionResponseSchema() map[string]any {
	props := factorySessionExecutionResponseSchema()["properties"].(map[string]any)
	props["syncOutcome"] = enumStringProperty(
		"Sync wait outcome for POST /factory-sessions/sync.",
		"COMPLETED", "TIMED_OUT", "FAILED",
	)
	props["timedOut"] = booleanProperty("Whether sync wait timed out before terminal completion.")
	props["result"] = factorySessionResultSchema()
	schema := factorySessionExecutionResponseSchema()
	schema["properties"] = props
	if required, ok := schema["required"].([]string); ok {
		schema["required"] = append(required, "syncOutcome")
	}
	return schema
}

func factorySessionDurableReadModelSchema() map[string]any {
	return objectSchema(map[string]any{
		"artifactRefs":           map[string]any{"type": "array", "items": map[string]any{"type": "object"}},
		"budgets":                map[string]any{"type": "object"},
		"dialect":                stringProperty("Resolved orchestrator dialect when available."),
		"effectivePolicy":        map[string]any{"type": "object"},
		"sessionId":              stringProperty("Stable durable Factory Session identifier."),
		"status":                 durableLifecycleStatusSchema(),
		"orchestratorKind":       orchestratorKindSchema(),
		"resolvedSource":         resolvedSourceIdentitySchema(),
		"sourceHash":             stringProperty("Stable hash of the resolved workflow or factory source."),
		"effectivePolicyHash":    stringProperty("Stable hash of the effective approved orchestrator policy."),
		"phase":                  stringProperty("Current JavaScript orchestrator phase when available."),
		"phaseSummaries":         map[string]any{"type": "array", "items": map[string]any{"type": "object"}},
		"latestCheckpoint":       map[string]any{"type": "object"},
		"lifecycle":              map[string]any{"type": "object"},
		"links":                  map[string]any{"type": "object"},
		"requestedPolicy":        map[string]any{"type": "object"},
		"failureDetail":          map[string]any{"type": "object"},
		"partialResultAvailable": booleanProperty("Whether a partial result remains available."),
		"staleLease":             booleanProperty("Whether the durable execution lease is stale."),
		"usage":                  map[string]any{"type": "object"},
		"progress": objectSchema(map[string]any{
			"totalDispatches":     map[string]any{"type": "integer"},
			"completedDispatches": map[string]any{"type": "integer"},
			"failedDispatches":    map[string]any{"type": "integer"},
			"inFlightDispatches":  map[string]any{"type": "integer"},
			"phaseCount":          map[string]any{"type": "integer"},
		}),
		"resultSummary": objectSchema(map[string]any{
			"resultStatus": enumStringProperty(
				"Latest durable result status projection.",
				"NOT_READY", "UNAVAILABLE", "PARTIAL", "FINAL", "FAILED_WITH_PARTIAL",
			),
			"summary":      stringProperty("Customer-visible result summary when available."),
			"artifactRefs": map[string]any{"type": "array", "items": map[string]any{"type": "object"}},
		}),
	}, "sessionId", "status", "orchestratorKind", "resolvedSource")
}

func listFactorySessionsResponseSchema() map[string]any {
	return objectSchema(map[string]any{
		"scope": enumStringProperty("Session list scope.", "live", "persisted", "all"),
		"sessions": map[string]any{
			"type": "array",
			"items": objectSchema(map[string]any{
				"id": stringProperty("Live workspace Factory Session identifier."),
			}),
		},
		"durableSessions": map[string]any{
			"type": "array",
			"items": objectSchema(map[string]any{
				"sessionId":        stringProperty("Stable durable Factory Session identifier."),
				"status":           durableLifecycleStatusSchema(),
				"orchestratorKind": orchestratorKindSchema(),
				"resolvedSource":   resolvedSourceIdentitySchema(),
			}, "sessionId", "status", "orchestratorKind", "resolvedSource"),
		},
	})
}

func factorySessionResultSchema() map[string]any {
	return objectSchema(map[string]any{
		"sessionId": stringProperty("Stable durable Factory Session identifier."),
		"resultStatus": enumStringProperty(
			"Durable Factory Session result status.",
			"NOT_READY", "UNAVAILABLE", "PARTIAL", "FINAL", "FAILED_WITH_PARTIAL",
		),
		"sessionStatus": durableLifecycleStatusSchema(),
		"mode":          enumStringProperty("Result retrieval mode.", "final", "partial"),
		"includeArtifacts": booleanProperty(
			"Whether artifact metadata was included in this response.",
		),
		"primaryResult": map[string]any{
			"type":        "array",
			"description": "Primary workflow output when resultStatus is PARTIAL or FINAL.",
		},
		"artifactIds": map[string]any{
			"type":        "array",
			"items":       map[string]any{"type": "string"},
			"description": "Artifact identifiers for materialized outputs.",
		},
		"availability": objectSchema(map[string]any{
			"reason":    stringProperty("Stable availability reason when resultStatus is NOT_READY or UNAVAILABLE."),
			"message":   stringProperty("Availability message for polling clients."),
			"retryable": booleanProperty("Whether polling or a later retry may return a ready result."),
		}),
		"failureDetail": objectSchema(map[string]any{
			"reason":  stringProperty("Stable failure reason when resultStatus is FAILED_WITH_PARTIAL."),
			"message": stringProperty("Failure message when resultStatus is FAILED_WITH_PARTIAL."),
		}),
	}, "sessionId", "resultStatus")
}

func listFactorySessionDispatchesResponseSchema() map[string]any {
	return objectSchema(map[string]any{
		"sessionId": stringProperty("Stable Factory Session identifier that owns the listed dispatches."),
		"dispatches": map[string]any{
			"type": "array",
			"items": objectSchema(map[string]any{
				"attempt":               integerProperty("One-based dispatch attempt when available."),
				"id":                    stringProperty("Stable dispatch identifier."),
				"dispatchKind":          stringProperty("Canonical dispatch kind shared across orchestrators."),
				"status":                stringProperty("Dispatch lifecycle status."),
				"phase":                 stringProperty("JavaScript phase when available."),
				"label":                 stringProperty("Customer-visible dispatch label when available."),
				"runnerId":              stringProperty("Selected runner identifier when available."),
				"model":                 stringProperty("Selected model identifier when available."),
				"provider":              stringProperty("Selected provider identifier when available."),
				"providerSessionRefs":   map[string]any{"type": "array", "items": map[string]any{"type": "object"}},
				"outputArtifactIds":     map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				"failureClassification": stringProperty("Safe machine-readable failure classification."),
				"failureDetail":         map[string]any{"type": "object"},
				"retryable":             booleanProperty("Whether the current failure is retryable."),
				"usage":                 map[string]any{"type": "object"},
				"warnings":              map[string]any{"type": "array", "items": map[string]any{"type": "object"}},
				"javascript":            map[string]any{"type": "object"},
			}, "id", "dispatchKind", "status"),
		},
	}, "sessionId", "dispatches")
}

func listFactorySessionArtifactsResponseSchema() map[string]any {
	return objectSchema(map[string]any{
		"sessionId": stringProperty("Stable Factory Session identifier that owns the listed artifacts."),
		"artifacts": map[string]any{
			"type": "array",
			"items": objectSchema(map[string]any{
				"id":              stringProperty("Stable FactoryArtifact identifier."),
				"kind":            stringProperty("FactoryArtifact kind."),
				"visibility":      stringProperty("Artifact visibility classification."),
				"sizeBytes":       integerProperty("Artifact size in bytes when known."),
				"contentHash":     stringProperty("Stable content hash when available."),
				"dispatchId":      stringProperty("Linked dispatch identifier when available."),
				"createdAt":       stringProperty("Artifact creation timestamp when available."),
				"label":           stringProperty("Customer-visible artifact label when available."),
				"auditMode":       stringProperty("Audit mode applied when the artifact was captured."),
				"redactionCounts": map[string]any{"type": "object"},
				"retrievalRef":    map[string]any{"type": "object"},
			}, "id", "kind"),
		},
	}, "sessionId", "artifacts")
}

func factorySessionLifecycleControlRequestSchema() map[string]any {
	return objectSchema(map[string]any{
		"sessionId": stringProperty("Stable durable Factory Session identifier."),
		"operation": enumStringProperty(
			"Lifecycle control operation for one Factory Session.",
			"APPROVE", "PAUSE", "RESUME", "CANCEL", "TERMINATE", "RETRY_DISPATCH",
		),
		"requestId": stringProperty("Optional idempotency key for one lifecycle control request."),
		"reason":    stringProperty("Optional operator-provided reason for audit and diagnostics."),
		"dispatchId": stringProperty(
			"Target dispatch identifier when operation is RETRY_DISPATCH.",
		),
		"approvalPreviewId": stringProperty("Approval preview identity when operation is APPROVE."),
		"approvedPolicy": map[string]any{
			"type":        "object",
			"description": "Approved orchestrator policy when operation is APPROVE.",
		},
	}, "sessionId", "operation")
}

func factorySessionLifecycleControlResponseSchema() map[string]any {
	return objectSchema(map[string]any{
		"sessionId": stringProperty("Stable durable Factory Session identifier."),
		"operation": enumStringProperty(
			"Lifecycle control operation evaluated for the Factory Session.",
			"APPROVE", "PAUSE", "RESUME", "CANCEL", "TERMINATE", "RETRY_DISPATCH",
		),
		"outcome": enumStringProperty(
			"Typed lifecycle-control outcome.",
			"ACCEPTED", "NO_OP", "INVALID_STATE", "TERMINAL_SESSION", "CONFLICT",
		),
		"status": durableLifecycleStatusSchema(),
		"detail": stringProperty("Optional human-readable detail explaining NO_OP or rejected outcomes."),
		"links": objectSchema(map[string]any{
			"session":    stringProperty("Relative URL for GET /factory-sessions/{session_id}."),
			"results":    stringProperty("Relative URL for GET /factory-sessions/{session_id}/results."),
			"dispatches": stringProperty("Relative URL for GET /factory-sessions/{session_id}/dispatches."),
			"artifacts":  stringProperty("Relative URL for GET /factory-sessions/{session_id}/artifacts."),
			"events":     stringProperty("Relative URL for GET /factory-sessions/{session_id}/events."),
		}),
	}, "sessionId", "operation", "outcome", "status")
}

func factorySessionEventReadResponseSchema() map[string]any {
	return objectSchema(map[string]any{
		"sessionId": stringProperty("Stable durable Factory Session identifier."),
		"events": map[string]any{
			"type": "array",
			"items": objectSchema(map[string]any{
				"id":            stringProperty("Canonical FactoryEvent identifier."),
				"type":          stringProperty("Canonical FactoryEvent type."),
				"schemaVersion": stringProperty("FactoryEvent envelope schema version."),
				"context": objectSchema(map[string]any{
					"eventTime":       stringProperty("RFC3339 event timestamp."),
					"sequence":        integerProperty("Append-only event-log sequence number."),
					"sessionId":       stringProperty("Owning Factory Session identifier."),
					"sessionSequence": integerProperty("Monotonic session-scoped event sequence when available."),
				}),
				"payload": map[string]any{
					"type":        "object",
					"description": "Typed FactoryEvent payload for session lifecycle, dispatches, and artifacts.",
				},
			}, "id", "type", "schemaVersion", "context"),
		},
	}, "sessionId", "events")
}

func factorySessionEventReadRequestSchema() map[string]any {
	return objectSchema(map[string]any{
		"sessionId": stringProperty("Stable durable Factory Session identifier."),
		"afterEventId": stringProperty(
			"Reconnect cursor: return events after this FactoryEvent identifier.",
		),
		"afterSequence": integerProperty(
			"Reconnect cursor: return events after this sessionSequence value.",
		),
	}, "sessionId")
}

func durableLifecycleStatusSchema() map[string]any {
	return enumStringProperty(
		"Durable Factory Session lifecycle status.",
		"QUEUED", "AWAITING_APPROVAL", "RUNNING", "PAUSED", "RESUMING",
		"SUCCEEDED", "FAILED", "CANCELING", "CANCELED", "TIMED_OUT", "INTERRUPTED",
	)
}

func orchestratorKindSchema() map[string]any {
	return enumStringProperty("Factory orchestrator kind.", "PETRI", "JAVASCRIPT")
}

func resolvedSourceIdentitySchema() map[string]any {
	return objectSchema(map[string]any{
		"kind":       stringProperty("Resolved durable execution source kind."),
		"sourceRef":  stringProperty("Resolved public source reference."),
		"sourceHash": stringProperty("Stable hash of the resolved source."),
	}, "kind")
}
