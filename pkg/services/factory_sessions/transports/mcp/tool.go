// Package factorysession exposes MCP tool discovery for durable Factory Session
// operations backed by the shared factorysessionexecution service contract.
package factorysession

import (
	"context"
	"encoding/json"
	"fmt"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/transports/mapping"
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

// ValidateSource runs the canonical Factory preview contract for the
// you.factory_session.validate_source MCP tool without provider execution.
func ValidateSource(
	ctx context.Context,
	workflows factoryruntime.WorkflowPreviewOperation,
	input factoryapi.FactoryPreviewRequest,
) ToolResponse[factoryapi.FactoryPreviewResult] {
	previewInput, err := apisurface.FactoryPreviewInputFromAPI(input)
	if err != nil {
		envelope := requestValidationErrorEnvelope(err)
		return ToolResponse[factoryapi.FactoryPreviewResult]{Error: &envelope}
	}

	if workflows == nil {
		envelope := requestValidationErrorEnvelope(fmt.Errorf("workflow preview is unavailable"))
		return ToolResponse[factoryapi.FactoryPreviewResult]{Error: &envelope}
	}
	workflowPreview, err := workflows.PreviewWorkflow(ctx, previewInput)
	if err != nil {
		envelope := requestValidationErrorEnvelope(err)
		return ToolResponse[factoryapi.FactoryPreviewResult]{Error: &envelope}
	}
	preview := apisurface.FactoryPreviewResultFromPreview(workflowPreview)
	if !preview.Valid {
		envelope := validationErrorEnvelopeFromPreview(preview)
		return ToolResponse[factoryapi.FactoryPreviewResult]{Error: &envelope}
	}
	return ToolResponse[factoryapi.FactoryPreviewResult]{Result: &preview}
}
