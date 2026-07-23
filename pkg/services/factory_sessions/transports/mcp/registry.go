package factorysession

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

const (
	// ToolInventoryFormatVersion is the reviewed MCP tool identity baseline version.
	ToolInventoryFormatVersion = "1"
	// ToolInventoryProtocolVersion pins the MCP protocol revision for inventory docs.
	ToolInventoryProtocolVersion = "2024-11-05"
	// ToolInventoryBaselineRelativePath is the reviewed canonical MCP tool inventory fixture.
	ToolInventoryBaselineRelativePath = "contracts/testdata/baseline/mcp-tools.json"
)

// ToolInventory is a pure, read-only projection of canonical MCP tool identities.
type ToolInventory struct {
	FormatVersion   string               `json:"formatVersion"`
	ProtocolVersion string               `json:"protocolVersion"`
	Tools           []ToolInventoryEntry `json:"tools"`
}

// ToolInventoryEntry records one canonical tool identity without result contracts.
type ToolInventoryEntry struct {
	IDCandidate       string         `json:"idCandidate"`
	Name              string         `json:"name"`
	Description       string         `json:"description"`
	InputSchema       map[string]any `json:"inputSchema"`
	HandlerRegistered bool           `json:"handlerRegistered"`
}

// ProjectToolInventory builds a sorted inventory document over DiscoverTools.
// Compatibility aliases are excluded. Input schemas are deep-copied and
// recursively key-canonicalized so serialization order stays stable.
func ProjectToolInventory() (ToolInventory, error) {
	return ProjectToolInventoryFromDiscovered(DiscoverTools())
}

// ProjectToolInventoryFromDiscovered builds one inventory document from an
// explicit canonical discovery slice. Production callers should use
// ProjectToolInventory.
func ProjectToolInventoryFromDiscovered(discovered []ToolDefinition) (ToolInventory, error) {
	entries := make([]ToolInventoryEntry, 0, len(discovered))
	for _, tool := range discovered {
		inputSchema, err := CanonicalizeInputSchema(tool.InputSchema)
		if err != nil {
			return ToolInventory{}, err
		}
		entries = append(entries, ToolInventoryEntry{
			IDCandidate:       deriveToolIDCandidate(tool.Name),
			Name:              tool.Name,
			Description:       tool.Description,
			InputSchema:       inputSchema,
			HandlerRegistered: IsCanonicalToolHandlerRegistered(tool.Name),
		})
	}
	slices.SortFunc(entries, func(left, right ToolInventoryEntry) int {
		return strings.Compare(left.Name, right.Name)
	})
	return ToolInventory{
		FormatVersion:   ToolInventoryFormatVersion,
		ProtocolVersion: ToolInventoryProtocolVersion,
		Tools:           entries,
	}, nil
}

// MarshalToolInventoryJSON encodes one inventory document with stable map key order.
func MarshalToolInventoryJSON(inventory ToolInventory) ([]byte, error) {
	return json.Marshal(inventory)
}

// VerifyProjectedToolInventory projects the canonical inventory and fails when
// any discovered tool lacks a registered handler or a compatibility alias
// appears as a canonical inventory entry.
func VerifyProjectedToolInventory() error {
	inventory, err := ProjectToolInventory()
	if err != nil {
		return err
	}
	return VerifyToolInventory(inventory)
}

// VerifyToolInventory fails when any inventoried canonical tool lacks handler
// registration evidence.
func VerifyToolInventory(inventory ToolInventory) error {
	for _, tool := range inventory.Tools {
		if !tool.HandlerRegistered || !IsCanonicalToolHandlerRegistered(tool.Name) {
			return fmt.Errorf("discovered canonical tool %q has no registered handler", tool.Name)
		}
	}
	return nil
}

func deriveToolIDCandidate(name string) string {
	candidate := strings.TrimPrefix(name, "you.")
	return strings.ReplaceAll(candidate, "_", "-")
}

// CanonicalizeInputSchema normalizes one JSON Schema object map for stable comparison.
func CanonicalizeInputSchema(schema map[string]any) (map[string]any, error) {
	if len(schema) == 0 {
		return nil, nil
	}
	canonical, err := canonicalizeSchemaValue(schema)
	if err != nil {
		return nil, err
	}
	out, ok := canonical.(map[string]any)
	if !ok {
		return nil, errInvalidInputSchemaType
	}
	return out, nil
}

var errInvalidInputSchemaType = &schemaCanonicalizationError{message: "input schema must be an object"}

type schemaCanonicalizationError struct {
	message string
}

func (e *schemaCanonicalizationError) Error() string {
	return e.message
}

func canonicalizeSchemaValue(value any) (any, error) {
	switch typed := value.(type) {
	case nil:
		return nil, nil
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		slices.Sort(keys)
		out := make(map[string]any, len(typed))
		for _, key := range keys {
			canonical, err := canonicalizeSchemaValue(typed[key])
			if err != nil {
				return nil, err
			}
			out[key] = canonical
		}
		return out, nil
	case []any:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			canonical, err := canonicalizeSchemaValue(item)
			if err != nil {
				return nil, err
			}
			out = append(out, canonical)
		}
		return out, nil
	default:
		return typed, nil
	}
}

// DiscoverTools returns the canonical JavaScript-orchestrated Factory Session
// MCP tool catalog in stable discovery order. Schemas mirror durable REST and
// Factory preview contracts.
func DiscoverTools() []ToolDefinition {
	return []ToolDefinition{
		listSessionsTool(),
		validateSourceTool(),
		startSyncTool(),
		startAsyncTool(),
		getSessionTool(),
		getResultTool(),
		listDispatchesTool(),
		listArtifactsTool(),
		controlTool(),
		readEventsTool(),
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

func listSessionsTool() ToolDefinition {
	return ToolDefinition{
		Name: ToolListSessions,
		Description: "List Factory Sessions for one scope (live workspace, persisted durable execution, or all). " +
			"Uses GET /factory-sessions durable listing vocabulary.",
		InputSchema: objectSchema(map[string]any{
			"scope": enumStringProperty(
				"Session list scope. Defaults to live when omitted.",
				"live", "persisted", "all",
			),
		}),
		OutputSchema: toolResponseSchema(listFactorySessionsResponseSchema()),
		SuccessStableFields: []string{
			"result.scope",
			"result.sessions",
			"result.durableSessions",
			"result.durableSessions[].sessionId",
			"result.durableSessions[].status",
			"result.durableSessions[].orchestratorKind",
		},
		ErrorStableFields: sharedErrorStableFields,
	}
}

func validateSourceTool() ToolDefinition {
	return ToolDefinition{
		Name: ToolValidateSource,
		Description: "Validate JavaScript orchestrator factory source through the canonical Factory preview contract " +
			"(POST /factories/preview) before starting a Factory Session.",
		InputSchema:  factoryPreviewRequestSchema(),
		OutputSchema: toolResponseSchema(factoryPreviewResultSchema()),
		SuccessStableFields: []string{
			"result.valid",
			"result.sourceResolution.sourceHash",
			"result.sourceResolution.sourceRef",
			"result.policyPreview.policyHash",
			"result.sourceValidationIssues",
		},
		ErrorStableFields: sharedErrorStableFields,
	}
}

func startSyncTool() ToolDefinition {
	return ToolDefinition{
		Name: ToolStartSync,
		Description: "Start one sync Factory Session and wait for terminal completion or timeout. " +
			"Maps to POST /factory-sessions/sync.",
		InputSchema:  factorySessionExecutionRequestSchema(),
		OutputSchema: toolResponseSchema(factorySessionSyncExecutionResponseSchema()),
		SuccessStableFields: []string{
			"result.sessionId",
			"result.status",
			"result.syncOutcome",
			"result.sourceHash",
			"result.effectivePolicyHash",
			"result.result.resultStatus",
			"result.links.results",
		},
		ErrorStableFields: sharedErrorStableFields,
	}
}

func startAsyncTool() ToolDefinition {
	return ToolDefinition{
		Name: ToolStartAsync,
		Description: "Start one async Factory Session and return an accepted or running summary for polling. " +
			"Maps to POST /factory-sessions/async.",
		InputSchema:  factorySessionExecutionRequestSchema(),
		OutputSchema: toolResponseSchema(factorySessionExecutionResponseSchema()),
		SuccessStableFields: []string{
			"result.sessionId",
			"result.status",
			"result.orchestratorKind",
			"result.sourceHash",
			"result.effectivePolicyHash",
			"result.links.session",
		},
		ErrorStableFields: sharedErrorStableFields,
	}
}

func getSessionTool() ToolDefinition {
	return ToolDefinition{
		Name: ToolGetSession,
		Description: "Get one durable Factory Session inspection read model with lifecycle status, source identity, " +
			"phase, progress, and result summary. Maps to GET /factory-sessions/{session_id}.",
		InputSchema: objectSchema(map[string]any{
			"sessionId": stringProperty("Stable durable Factory Session identifier."),
		}, "sessionId"),
		OutputSchema: toolResponseSchema(factorySessionDurableReadModelSchema()),
		SuccessStableFields: []string{
			"result.sessionId",
			"result.status",
			"result.orchestratorKind",
			"result.resolvedSource",
			"result.phase",
			"result.phaseSummaries",
			"result.progress",
			"result.latestCheckpoint",
			"result.effectivePolicy",
			"result.budgets",
			"result.usage",
			"result.artifactRefs",
			"result.resultSummary.resultStatus",
		},
		ErrorStableFields: sharedErrorStableFields,
	}
}

func getResultTool() ToolDefinition {
	return ToolDefinition{
		Name: ToolGetResult,
		Description: "Retrieve one durable Factory Session result in final or partial mode with optional artifact metadata. " +
			"Maps to GET /factory-sessions/{session_id}/results.",
		InputSchema: objectSchema(map[string]any{
			"sessionId": stringProperty("Stable durable Factory Session identifier."),
			"mode": enumStringProperty(
				"Result retrieval mode. Defaults to final when omitted.",
				"final", "partial",
			),
			"includeArtifacts": booleanProperty(
				"When true, include FactoryArtifact metadata refs in the response.",
			),
		}, "sessionId"),
		OutputSchema: toolResponseSchema(factorySessionResultSchema()),
		SuccessStableFields: []string{
			"result.sessionId",
			"result.resultStatus",
			"result.sessionStatus",
			"result.primaryResult",
			"result.artifactIds",
			"result.availability.reason",
			"result.availability.retryable",
		},
		ErrorStableFields: sharedErrorStableFields,
	}
}

func listDispatchesTool() ToolDefinition {
	return ToolDefinition{
		Name: ToolListDispatches,
		Description: "List dispatch summaries for one Factory Session, including dispatch id, status, kind, phase, " +
			"and provider-session correlation metadata when available. Maps to GET /factory-sessions/{session_id}/dispatches.",
		InputSchema: objectSchema(map[string]any{
			"sessionId": stringProperty("Stable durable Factory Session identifier."),
			"phase":     stringProperty("Exact canonical phase identifier."),
			"status":    stringProperty("Canonical Dispatch lifecycle status."),
		}, "sessionId"),
		OutputSchema: toolResponseSchema(listFactorySessionDispatchesResponseSchema()),
		SuccessStableFields: []string{
			"result.sessionId",
			"result.dispatches",
			"result.dispatches[].id",
			"result.dispatches[].status",
			"result.dispatches[].dispatchKind",
			"result.dispatches[].phase",
			"result.dispatches[].runnerId",
			"result.dispatches[].model",
			"result.dispatches[].providerSessionRefs",
			"result.dispatches[].attempt",
			"result.dispatches[].outputArtifactIds",
			"result.dispatches[].failureDetail",
		},
		ErrorStableFields: sharedErrorStableFields,
	}
}

func listArtifactsTool() ToolDefinition {
	return ToolDefinition{
		Name: ToolListArtifacts,
		Description: "List FactoryArtifact summaries for one Factory Session, including artifact id, kind, visibility, " +
			"size or hash metadata, and dispatch linkage when available. Maps to GET /factory-sessions/{session_id}/artifacts.",
		InputSchema: objectSchema(map[string]any{
			"sessionId": stringProperty("Stable durable Factory Session identifier."),
		}, "sessionId"),
		OutputSchema: toolResponseSchema(listFactorySessionArtifactsResponseSchema()),
		SuccessStableFields: []string{
			"result.sessionId",
			"result.artifacts",
			"result.artifacts[].id",
			"result.artifacts[].kind",
			"result.artifacts[].visibility",
			"result.artifacts[].dispatchId",
		},
		ErrorStableFields: sharedErrorStableFields,
	}
}

func controlTool() ToolDefinition {
	return ToolDefinition{
		Name: ToolControl,
		Description: "Apply one durable Factory Session lifecycle control such as approve, pause, resume, cancel, " +
			"terminate, or retry-dispatch. Maps to POST /factory-sessions/{session_id}/{control}.",
		InputSchema:  factorySessionLifecycleControlRequestSchema(),
		OutputSchema: toolResponseSchema(factorySessionLifecycleControlResponseSchema()),
		SuccessStableFields: []string{
			"result.sessionId",
			"result.operation",
			"result.outcome",
			"result.status",
			"result.links.session",
		},
		ErrorStableFields: sharedErrorStableFields,
	}
}

func readEventsTool() ToolDefinition {
	return ToolDefinition{
		Name: ToolReadEvents,
		Description: "Read ordered Factory Session event facts for reconnect and inspection without exposing " +
			"internal Petri-net terminology as the primary public vocabulary. Maps to GET /factory-sessions/{session_id}/events.",
		InputSchema:  factorySessionEventReadRequestSchema(),
		OutputSchema: toolResponseSchema(factorySessionEventReadResponseSchema()),
		SuccessStableFields: []string{
			"result.sessionId",
			"result.events",
			"result.events[].id",
			"result.events[].type",
			"result.events[].context.eventTime",
			"result.events[].context.sequence",
			"result.events[].context.sessionId",
		},
		ErrorStableFields: sharedErrorStableFields,
	}
}
