package factorysession

import (
	"context"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	apifactorysession "github.com/portpowered/infinite-you/pkg/apisurface/factorysession"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
)

// ReadEventsInput is the MCP request shape for you.factory_session.read_events.
type ReadEventsInput struct {
	SessionID     string `json:"sessionId"`
	AfterEventID  string `json:"afterEventId,omitempty"`
	AfterSequence *int   `json:"afterSequence,omitempty"`
}

// ReadEventsResult is the MCP response shape for you.factory_session.read_events.
type ReadEventsResult struct {
	SessionID string                   `json:"sessionId"`
	Events    []factoryapi.FactoryEvent `json:"events,omitempty"`
}

// ReadEvents returns ordered Factory Session event facts for reconnect and
// inspection through the you.factory_session.read_events MCP tool.
func ReadEvents(service factorysessionexecution.Service, input ReadEventsInput) ToolResponse[ReadEventsResult] {
	if service == nil {
		envelope := unavailableServiceErrorEnvelope()
		return ToolResponse[ReadEventsResult]{Error: &envelope}
	}

	params := factoryapi.GetEventsBySessionIdParams{}
	if trimmed := input.AfterEventID; trimmed != "" {
		afterEventID := factoryapi.AfterEventId(trimmed)
		params.AfterEventId = &afterEventID
	}
	if input.AfterSequence != nil {
		sequence := factoryapi.AfterSequence(*input.AfterSequence)
		params.AfterSequence = &sequence
	}
	reconnect, err := apifactorysession.EventReconnectRequestFromAPI(params)
	if err != nil {
		envelope := requestValidationErrorEnvelope(err)
		return ToolResponse[ReadEventsResult]{Error: &envelope}
	}

	sessionID := input.SessionID
	result, err := service.ReadEvents(context.Background(), sessionID, reconnect)
	if err != nil {
		envelope := eventReadErrorEnvelope(sessionID, err)
		return ToolResponse[ReadEventsResult]{Error: &envelope}
	}

	mapped := ReadEventsResult{
		SessionID: result.SessionID,
		Events:    apifactorysession.EventReadResponseToAPI(result),
	}
	return ToolResponse[ReadEventsResult]{Result: &mapped}
}
