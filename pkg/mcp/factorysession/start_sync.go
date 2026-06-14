package factorysession

import (
	"context"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	apifactorysession "github.com/portpowered/infinite-you/pkg/apisurface/factorysession"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
)

// StartSync runs the durable sync Factory Session contract for the
// you.factory_session.start_sync MCP tool through the shared execution service.
func StartSync(service factorysessionexecution.Service, input factoryapi.FactorySessionExecutionRequest) ToolResponse[factoryapi.FactorySessionSyncExecutionResponse] {
	if service == nil {
		envelope := unavailableServiceErrorEnvelope()
		return ToolResponse[factoryapi.FactorySessionSyncExecutionResponse]{Error: &envelope}
	}

	startReq, err := apifactorysession.StartRequestFromAPI(input)
	if err != nil {
		envelope := requestValidationErrorEnvelope(err)
		return ToolResponse[factoryapi.FactorySessionSyncExecutionResponse]{Error: &envelope}
	}

	result, err := service.StartSync(context.Background(), startReq)
	if err != nil {
		envelope := executionErrorEnvelope(err)
		return ToolResponse[factoryapi.FactorySessionSyncExecutionResponse]{Error: &envelope}
	}

	mapped := apifactorysession.SyncStartResponseToAPI(result)
	return ToolResponse[factoryapi.FactorySessionSyncExecutionResponse]{Result: &mapped}
}
