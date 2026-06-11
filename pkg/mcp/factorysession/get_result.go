package factorysession

import (
	"context"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	apifactorysession "github.com/portpowered/infinite-you/pkg/apisurface/factorysession"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
)

// GetResultInput is the MCP request shape for you.factory_session.get_result.
type GetResultInput struct {
	SessionID        string                              `json:"sessionId"`
	Mode             *factoryapi.FactorySessionResultMode `json:"mode,omitempty"`
	IncludeArtifacts *bool                               `json:"includeArtifacts,omitempty"`
}

// GetResult retrieves one durable Factory Session result through the
// you.factory_session.get_result MCP tool.
func GetResult(service factorysessionexecution.Service, input GetResultInput) ToolResponse[factoryapi.FactorySessionResult] {
	if service == nil {
		envelope := unavailableServiceErrorEnvelope()
		return ToolResponse[factoryapi.FactorySessionResult]{Error: &envelope}
	}

	params := factoryapi.GetFactorySessionResultsParams{
		Mode:             input.Mode,
		IncludeArtifacts: input.IncludeArtifacts,
	}
	resultReq, err := apifactorysession.ResultRequestFromAPI(params)
	if err != nil {
		envelope := requestValidationErrorEnvelope(err)
		return ToolResponse[factoryapi.FactorySessionResult]{Error: &envelope}
	}

	sessionID := input.SessionID
	result, err := service.GetResult(context.Background(), sessionID, resultReq)
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
