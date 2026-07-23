package factorysession

import (
	"context"

	factorysessionexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apifactorysession "github.com/portpowered/infinite-you/pkg/transports/mapping/factorysession"
)

// ListSessionsInput is the MCP request shape for you.factory_session.list.
type ListSessionsInput struct {
	Scope *factoryapi.FactorySessionListScope `json:"scope,omitempty"`
}

// ListSessions returns scoped Factory Session summaries through the
// you.factory_session.list MCP tool.
func ListSessions(ctx context.Context, service factorysessionexecution.ExecutionService, prepare RequestPreparation, input ListSessionsInput) ToolResponse[factoryapi.ListFactorySessionsResponse] {
	if ctx == nil {
		envelope := executionErrorEnvelope(errMissingRequestContext)
		return ToolResponse[factoryapi.ListFactorySessionsResponse]{Error: &envelope}
	}
	if service == nil {
		envelope := unavailableServiceErrorEnvelope()
		return ToolResponse[factoryapi.ListFactorySessionsResponse]{Error: &envelope}
	}

	listReq, err := apifactorysession.ListSessionsRequestFromAPI(factoryapi.ListFactorySessionsParams{
		Scope: input.Scope,
	})
	if err == nil {
		listReq, err = prepare.PrepareListSessions(listReq)
	}
	if err != nil {
		envelope := executionErrorEnvelope(err)
		return ToolResponse[factoryapi.ListFactorySessionsResponse]{Error: &envelope}
	}

	result, err := service.ListSessions(ctx, listReq)
	if err != nil {
		envelope := executionErrorEnvelope(err)
		return ToolResponse[factoryapi.ListFactorySessionsResponse]{Error: &envelope}
	}

	mapped := apifactorysession.ListSessionsResponseToAPI(result)
	return ToolResponse[factoryapi.ListFactorySessionsResponse]{Result: &mapped}
}

// GetSessionInput is the MCP request shape for you.factory_session.get.
type GetSessionInput struct {
	SessionID string `json:"sessionId"`
}

// GetSession returns one durable Factory Session inspection read model through
// the you.factory_session.get MCP tool.
func GetSession(ctx context.Context, service factorysessionexecution.ExecutionService, input GetSessionInput) ToolResponse[factoryapi.FactorySessionDurableReadModel] {
	if ctx == nil {
		envelope := executionErrorEnvelope(errMissingRequestContext)
		return ToolResponse[factoryapi.FactorySessionDurableReadModel]{Error: &envelope}
	}
	if service == nil {
		envelope := unavailableServiceErrorEnvelope()
		return ToolResponse[factoryapi.FactorySessionDurableReadModel]{Error: &envelope}
	}

	sessionID := input.SessionID
	result, err := service.GetSession(ctx, sessionID)
	if err != nil {
		envelope := readErrorEnvelope(sessionID, err)
		return ToolResponse[factoryapi.FactorySessionDurableReadModel]{Error: &envelope}
	}

	mapped := apifactorysession.SessionReadResponseToAPI(result)
	return ToolResponse[factoryapi.FactorySessionDurableReadModel]{Result: &mapped}
}

// GetResultInput is the MCP request shape for you.factory_session.get_result.
type GetResultInput struct {
	SessionID        string                               `json:"sessionId"`
	Mode             *factoryapi.FactorySessionResultMode `json:"mode,omitempty"`
	IncludeArtifacts *bool                                `json:"includeArtifacts,omitempty"`
}

// GetResult retrieves one durable Factory Session result through the
// you.factory_session.get_result MCP tool.
func GetResult(ctx context.Context, service factorysessionexecution.ExecutionService, prepare RequestPreparation, input GetResultInput) ToolResponse[factoryapi.FactorySessionResult] {
	if ctx == nil {
		envelope := executionErrorEnvelope(errMissingRequestContext)
		return ToolResponse[factoryapi.FactorySessionResult]{Error: &envelope}
	}
	if service == nil {
		envelope := unavailableServiceErrorEnvelope()
		return ToolResponse[factoryapi.FactorySessionResult]{Error: &envelope}
	}

	params := factoryapi.GetFactorySessionResultsParams{
		Mode:             input.Mode,
		IncludeArtifacts: input.IncludeArtifacts,
	}
	resultReq, err := apifactorysession.ResultRequestFromAPI(params)
	if err == nil {
		resultReq, err = prepare.PrepareResult(resultReq)
	}
	if err != nil {
		envelope := requestValidationErrorEnvelope(err)
		return ToolResponse[factoryapi.FactorySessionResult]{Error: &envelope}
	}

	sessionID := input.SessionID
	result, err := service.GetResult(ctx, sessionID, resultReq)
	if err != nil {
		envelope := readErrorEnvelope(sessionID, err)
		return ToolResponse[factoryapi.FactorySessionResult]{Error: &envelope}
	}
	if result.ResultStatus == factorysessionexecution.ResultStatusNotReady {
		envelope := resultNotReadyErrorEnvelope(sessionID, result.Availability)
		return ToolResponse[factoryapi.FactorySessionResult]{Error: &envelope}
	}

	mapped := apifactorysession.ResultResponseToAPI(result)
	return ToolResponse[factoryapi.FactorySessionResult]{Result: &mapped}
}
