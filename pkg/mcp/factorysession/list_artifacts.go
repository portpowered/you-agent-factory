package factorysession

import (
	"context"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	apifactorysession "github.com/portpowered/infinite-you/pkg/apisurface/factorysession"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
)

// ListArtifactsInput is the MCP request shape for you.factory_session.list_artifacts.
type ListArtifactsInput struct {
	SessionID string `json:"sessionId"`
}

// ListArtifacts returns deterministic FactoryArtifact summaries for one Factory
// Session through the you.factory_session.list_artifacts MCP tool.
func ListArtifacts(
	service factorysessionexecution.Service,
	input ListArtifactsInput,
) ToolResponse[factoryapi.ListFactorySessionArtifactsResponse] {
	if service == nil {
		envelope := unavailableServiceErrorEnvelope()
		return ToolResponse[factoryapi.ListFactorySessionArtifactsResponse]{Error: &envelope}
	}

	sessionID := input.SessionID
	result, err := service.ListArtifacts(context.Background(), sessionID)
	if err != nil {
		envelope := readErrorEnvelope(sessionID, err)
		return ToolResponse[factoryapi.ListFactorySessionArtifactsResponse]{Error: &envelope}
	}

	mapped := apifactorysession.ListArtifactsResponseToAPI(result)
	return ToolResponse[factoryapi.ListFactorySessionArtifactsResponse]{Result: &mapped}
}
