package factorysession

import (
	"context"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	apifactorysession "github.com/portpowered/infinite-you/pkg/apisurface/factorysession"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
)

// GetSessionInput is the MCP request shape for you.factory_session.get.
type GetSessionInput struct {
	SessionID string `json:"sessionId"`
}

// GetSession returns one durable Factory Session inspection read model through
// the you.factory_session.get MCP tool.
func GetSession(service factorysessionexecution.Service, input GetSessionInput) ToolResponse[factoryapi.FactorySessionDurableReadModel] {
	if service == nil {
		envelope := unavailableServiceErrorEnvelope()
		return ToolResponse[factoryapi.FactorySessionDurableReadModel]{Error: &envelope}
	}

	sessionID := input.SessionID
	result, err := service.GetSession(context.Background(), sessionID)
	if err != nil {
		envelope := readErrorEnvelope(sessionID, err)
		return ToolResponse[factoryapi.FactorySessionDurableReadModel]{Error: &envelope}
	}

	mapped := apifactorysession.SessionReadResponseToAPI(result)
	return ToolResponse[factoryapi.FactorySessionDurableReadModel]{Result: &mapped}
}
