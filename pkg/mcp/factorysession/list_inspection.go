package factorysession

import (
	"context"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	apifactorysession "github.com/portpowered/infinite-you/pkg/apisurface/factorysession"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
)

// ListDispatchesInput is the MCP request shape for you.factory_session.list_dispatches.
type ListDispatchesInput struct {
	SessionID string `json:"sessionId"`
}

// ListDispatches returns durable Factory Session dispatch summaries through the
// you.factory_session.list_dispatches MCP tool.
func ListDispatches(service factorysessionexecution.Service, input ListDispatchesInput) ToolResponse[factoryapi.ListFactorySessionDispatchesResponse] {
	if service == nil {
		envelope := unavailableServiceErrorEnvelope()
		return ToolResponse[factoryapi.ListFactorySessionDispatchesResponse]{Error: &envelope}
	}

	sessionID := input.SessionID
	result, err := service.ListDispatches(context.Background(), sessionID)
	if err != nil {
		envelope := readErrorEnvelope(sessionID, err)
		return ToolResponse[factoryapi.ListFactorySessionDispatchesResponse]{Error: &envelope}
	}

	mapped := apifactorysession.ListDispatchesResponseToAPI(result)
	return ToolResponse[factoryapi.ListFactorySessionDispatchesResponse]{Result: &mapped}
}

// ListArtifactsInput is the MCP request shape for you.factory_session.list_artifacts.
type ListArtifactsInput struct {
	SessionID string `json:"sessionId"`
}

// ListArtifacts returns durable Factory Session artifact summaries through the
// you.factory_session.list_artifacts MCP tool.
func ListArtifacts(service factorysessionexecution.Service, input ListArtifactsInput) ToolResponse[factoryapi.ListFactorySessionArtifactsResponse] {
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
