// Package factorysession exposes MCP tool discovery for durable Factory Session
// operations backed by the shared factorysessionexecution service contract.
package factorysession

import (
	"encoding/json"

	"github.com/portpowered/infinite-you/pkg/apisurface"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// Tool names use Factory Session vocabulary and align with durable REST routes.
const (
	ToolListSessions   = "you.factory_session.list"
	ToolValidateSource = "you.factory_session.validate_source"
	ToolStartSync      = "you.factory_session.start_sync"
	ToolStartAsync     = "you.factory_session.start_async"
	ToolGetSession     = "you.factory_session.get"
	ToolGetResult      = "you.factory_session.get_result"
	ToolListDispatches = "you.factory_session.list_dispatches"
	ToolListArtifacts  = "you.factory_session.list_artifacts"
	ToolControl        = "you.factory_session.control"
	ToolReadEvents     = "you.factory_session.read_events"
)

// Workflow-named compatibility aliases resolve to canonical Factory Session tools.
const (
	ToolWorkflowValidate  = "you.workflow.validate"
	ToolWorkflowRun       = "you.workflow.run"
	ToolWorkflowStatus    = "you.workflow.status"
	ToolWorkflowResult    = "you.workflow.result"
	ToolWorkflowArtifacts = "you.workflow.artifacts"
)

// Stable error envelope fields shared by every dynamic workflow MCP tool.
var sharedErrorStableFields = []string{
	"error.code",
	"error.message",
	"error.retryable",
	"error.sessionId",
	"error.details",
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

// CompatibilityAlias documents one workflow-named MCP tool alias that resolves
// to a canonical Factory Session tool implementation.
type CompatibilityAlias struct {
	Name          string `json:"name"`
	CanonicalName string `json:"canonicalName"`
	Description   string `json:"description"`
	// CompatibilityOnly is always true for workflow-named aliases.
	CompatibilityOnly bool `json:"compatibilityOnly"`
}

// ToolErrorEnvelope is the stable MCP failure shape for Factory Session tools.
type ToolErrorEnvelope struct {
	Code      string         `json:"code"`
	Message   string         `json:"message"`
	Retryable bool           `json:"retryable"`
	SessionID string         `json:"sessionId,omitempty"`
	Details   map[string]any `json:"details,omitempty"`
}

// ToolResponse wraps one tool outcome with either a typed result or a stable error.
type ToolResponse[T any] struct {
	Result *T                 `json:"result,omitempty"`
	Error  *ToolErrorEnvelope `json:"error,omitempty"`
}

// MarshalJSON encodes one tool definition for MCP hosts and mock clients.
func (t ToolDefinition) MarshalJSON() ([]byte, error) {
	type alias ToolDefinition
	return json.Marshal(alias(t))
}

// DiscoverCompatibilityAliases returns workflow-named MCP tool aliases that
// resolve to canonical Factory Session tool implementations.
func DiscoverCompatibilityAliases() []CompatibilityAlias {
	return []CompatibilityAlias{
		{
			Name:          ToolWorkflowValidate,
			CanonicalName: ToolValidateSource,
			Description: "Compatibility-only alias for you.factory_session.validate_source. " +
				"Uses the same Factory preview validation contract and response shape.",
			CompatibilityOnly: true,
		},
		{
			Name:          ToolWorkflowRun,
			CanonicalName: ToolStartSync,
			Description: "Compatibility-only alias for you.factory_session.start_sync. " +
				"Uses the same sync Factory Session start contract and response shape.",
			CompatibilityOnly: true,
		},
		{
			Name:          ToolWorkflowStatus,
			CanonicalName: ToolGetSession,
			Description: "Compatibility-only alias for you.factory_session.get. " +
				"Uses the same durable Factory Session status read model.",
			CompatibilityOnly: true,
		},
		{
			Name:          ToolWorkflowResult,
			CanonicalName: ToolGetResult,
			Description: "Compatibility-only alias for you.factory_session.get_result. " +
				"Uses the same durable Factory Session result read contract.",
			CompatibilityOnly: true,
		},
		{
			Name:          ToolWorkflowArtifacts,
			CanonicalName: ToolListArtifacts,
			Description: "Compatibility-only alias for you.factory_session.list_artifacts. " +
				"Uses the same FactoryArtifact listing response shape.",
			CompatibilityOnly: true,
		},
	}
}

// ResolveToolName maps one workflow compatibility alias to its canonical Factory
// Session tool name. Unknown names pass through unchanged.
func ResolveToolName(name string) string {
	for _, alias := range DiscoverCompatibilityAliases() {
		if alias.Name == name {
			return alias.CanonicalName
		}
	}
	return name
}

// ValidateSource runs the canonical Factory preview contract for the
// you.factory_session.validate_source MCP tool without provider execution.
func ValidateSource(input factoryapi.FactoryPreviewRequest) ToolResponse[factoryapi.FactoryPreviewResult] {
	previewInput, err := apisurface.FactoryPreviewRequestFromAPI(input)
	if err != nil {
		envelope := requestValidationErrorEnvelope(err)
		return ToolResponse[factoryapi.FactoryPreviewResult]{Error: &envelope}
	}

	preview := apisurface.FactoryPreviewResultFromPreview(apisurface.BuildFactoryPreview(previewInput))
	if !preview.Valid {
		envelope := validationErrorEnvelopeFromPreview(preview)
		return ToolResponse[factoryapi.FactoryPreviewResult]{Error: &envelope}
	}
	return ToolResponse[factoryapi.FactoryPreviewResult]{Result: &preview}
}
