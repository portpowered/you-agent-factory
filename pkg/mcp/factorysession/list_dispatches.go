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

// ListDispatches returns deterministic dispatch summaries for one Factory Session
// through the you.factory_session.list_dispatches MCP tool.
func ListDispatches(
	service factorysessionexecution.Service,
	input ListDispatchesInput,
) ToolResponse[factoryapi.ListFactorySessionDispatchesResponse] {
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
