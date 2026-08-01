// Package modelmcp exposes MCP tool operations for Models service root
// contracts backed by an injected Models Service root.
package modelmcp

import "encoding/json"

// Tool names use Models vocabulary and align with accepted root operations.
const (
	ToolListCatalog     = "you.model.list_catalog"
	ToolPrepareAssets   = "you.model.prepare_assets"
	ToolAcquireLease    = "you.model.acquire_lease"
	ToolInvokeWithLease = "you.model.invoke_with_lease"
)

// Stable error envelope fields shared by every Models MCP tool.
var sharedErrorStableFields = []string{
	"error.code",
	"error.message",
	"error.retryable",
	"error.details",
}

// ToolErrorEnvelope is the stable MCP failure shape for Models tools.
type ToolErrorEnvelope struct {
	Code      string         `json:"code"`
	Message   string         `json:"message"`
	Retryable bool           `json:"retryable"`
	Details   map[string]any `json:"details,omitempty"`
}

// ToolResponse wraps one tool outcome with either a typed result or a stable error.
type ToolResponse[T any] struct {
	Result *T                 `json:"result,omitempty"`
	Error  *ToolErrorEnvelope `json:"error,omitempty"`
}

// ToolDefinition is one discoverable MCP tool with typed schemas and documented
// stable response fields for success and error envelopes.
type ToolDefinition struct {
	Name                string         `json:"name"`
	Description         string         `json:"description"`
	InputSchema         map[string]any `json:"inputSchema"`
	OutputSchema        map[string]any `json:"outputSchema"`
	SuccessStableFields []string       `json:"successStableFields"`
	ErrorStableFields   []string       `json:"errorStableFields"`
}

// MarshalJSON encodes one tool definition for MCP hosts and mock clients.
func (t ToolDefinition) MarshalJSON() ([]byte, error) {
	type alias ToolDefinition
	return json.Marshal(alias(t))
}

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
		"code":      stringProperty("Stable machine-readable failure code."),
		"message":   stringProperty("Human-readable failure summary for MCP hosts."),
		"retryable": map[string]any{"type": "boolean", "description": "Whether the caller should retry the same request later."},
		"details": map[string]any{
			"type":                 "object",
			"additionalProperties": true,
			"description":          "Optional structured diagnostics such as validation issues or scope hints.",
		},
	}, "code", "message", "retryable")
}

func runtimeScopeRefProperty() map[string]any {
	return stringProperty("Opaque Models runtime-scope reference from OpenRuntimeScope.")
}

func listCatalogInputSchema() map[string]any {
	return objectSchema(map[string]any{
		"runtimeScopeRef": runtimeScopeRefProperty(),
	}, "runtimeScopeRef")
}

func prepareAssetsInputSchema() map[string]any {
	return objectSchema(map[string]any{
		"runtimeScopeRef": runtimeScopeRefProperty(),
		"name":            stringProperty("Scoped catalog model name whose assets should be prepared."),
	}, "runtimeScopeRef", "name")
}

func acquireLeaseInputSchema() map[string]any {
	return objectSchema(map[string]any{
		"runtimeScopeRef": runtimeScopeRefProperty(),
		"name":            stringProperty("Scoped catalog model name whose capacity should be leased."),
		"holder":          stringProperty("Non-empty lease holder identity for capacity accounting."),
	}, "runtimeScopeRef", "name", "holder")
}

func inferenceInputSchema() map[string]any {
	return objectSchema(map[string]any{
		"contentType": stringProperty("Detached inference input content type."),
		"content":     stringProperty("Detached inference input payload."),
	}, "contentType", "content")
}

func invokeWithLeaseInputSchema() map[string]any {
	return objectSchema(map[string]any{
		"runtimeScopeRef": runtimeScopeRefProperty(),
		"leaseRef":        stringProperty("Opaque Models lease capability reference from acquire_lease."),
		"holder":          stringProperty("Lease holder identity that must match the issued lease."),
		"modelName":       stringProperty("Scoped catalog model name to invoke."),
		"operation":       stringProperty("Scoped model operation identifier."),
		"responseMode":    stringProperty("Optional response-mode selector when the operation supports alternate representations."),
		"input":           inferenceInputSchema(),
	}, "runtimeScopeRef", "leaseRef", "holder", "modelName", "operation", "input")
}

func catalogSummarySchema() map[string]any {
	return objectSchema(map[string]any{
		"Name":             stringProperty("Catalog model name."),
		"ProviderLocality": stringProperty("Detached provider locality classification."),
		"Status":           stringProperty("Catalog readiness status."),
		"LoadState":        stringProperty("Detached runtime load state."),
	})
}

func listCatalogResultSchema() map[string]any {
	return objectSchema(map[string]any{
		"Models": map[string]any{
			"type":        "array",
			"description": "Detached catalog summaries for the scoped runtime.",
			"items":       catalogSummarySchema(),
		},
	})
}

func assetSnapshotSchema() map[string]any {
	return objectSchema(map[string]any{
		"ModelName": stringProperty("Scoped model name for the prepared assets."),
		"Readiness": stringProperty("Detached asset readiness state."),
	})
}

func prepareAssetsResultSchema() map[string]any {
	return objectSchema(map[string]any{
		"Asset":   assetSnapshotSchema(),
		"Outcome": stringProperty("Whether assets were already available or newly prepared."),
	})
}

func modelLeaseSchema() map[string]any {
	return objectSchema(map[string]any{
		"Lease": map[string]any{
			"type":        "object",
			"description": "Opaque issued lease capability reference.",
		},
		"Scope":         runtimeScopeRefProperty(),
		"ModelName":     stringProperty("Scoped catalog model name bound to the lease."),
		"Holder":        stringProperty("Lease holder identity."),
		"ExpiresAt":     stringProperty("RFC3339 lease expiry timestamp."),
		"Status":        stringProperty("Observable lease lifecycle status."),
		"HostReadiness": stringProperty("Detached host readiness state at lease issue."),
	})
}

func acquireLeaseResultSchema() map[string]any {
	return objectSchema(map[string]any{
		"Lease": modelLeaseSchema(),
	})
}

func inferenceContentSchema() map[string]any {
	return objectSchema(map[string]any{
		"ContentType": stringProperty("Detached inference output content type."),
		"Content":     stringProperty("Detached inference output payload."),
	})
}

func inferenceArtifactSchema() map[string]any {
	return objectSchema(map[string]any{
		"Artifact":  stringProperty("Opaque inference artifact reference."),
		"Name":      stringProperty("Artifact display name."),
		"MediaType": stringProperty("Artifact media type."),
		"SizeBytes": integerProperty("Artifact size in bytes."),
	})
}

func invokeWithLeaseResultSchema() map[string]any {
	return objectSchema(map[string]any{
		"Invocation":       stringProperty("Opaque invocation identity."),
		"Scope":            runtimeScopeRefProperty(),
		"Lease":            stringProperty("Opaque lease capability reference used for the invocation."),
		"ModelName":        stringProperty("Scoped catalog model name that was invoked."),
		"Operation":        stringProperty("Scoped model operation identifier."),
		"Status":           stringProperty("Observable invocation lifecycle status."),
		"Content":          map[string]any{"type": "array", "items": inferenceContentSchema()},
		"Artifacts":        map[string]any{"type": "array", "items": inferenceArtifactSchema()},
		"LeaseDisposition": stringProperty("Whether capacity was retained, released, or expired after invocation."),
	})
}

// SuccessTextEncodingSerializedJSON documents serialized JSON tool payloads in text content.
const SuccessTextEncodingSerializedJSON = "serialized-json"

// EncodeSuccessCallToolResult builds the live MCP tools/call success envelope for one
// serialized tool-response payload. Callers use this shape to document current server
// transport without mutating handlers.
func EncodeSuccessCallToolResult(toolResponse json.RawMessage) map[string]any {
	return map[string]any{
		"content": []map[string]any{
			{
				"type": "text",
				"text": string(toolResponse),
			},
		},
	}
}

// MarshalSuccessCallToolResultJSON encodes one success CallToolResult with stable key order.
func MarshalSuccessCallToolResultJSON(toolResponse json.RawMessage) ([]byte, error) {
	return json.Marshal(EncodeSuccessCallToolResult(toolResponse))
}
