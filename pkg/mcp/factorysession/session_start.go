package factorysession

import (
	"context"

	apifactorysession "github.com/portpowered/infinite-you/pkg/apisurface/factorysession"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// StartAsync runs the durable async Factory Session contract for the
// you.factory_session.start_async MCP tool through the shared execution service.
func StartAsync(service factorysessionexecution.Service, input factoryapi.FactorySessionExecutionRequest) ToolResponse[factoryapi.FactorySessionExecutionResponse] {
	if service == nil {
		envelope := unavailableServiceErrorEnvelope()
		return ToolResponse[factoryapi.FactorySessionExecutionResponse]{Error: &envelope}
	}

	startReq, err := apifactorysession.StartRequestFromAPI(input)
	if err != nil {
		envelope := requestValidationErrorEnvelope(err)
		return ToolResponse[factoryapi.FactorySessionExecutionResponse]{Error: &envelope}
	}

	result, err := service.StartAsync(context.Background(), startReq)
	if err != nil {
		envelope := executionErrorEnvelope(err)
		return ToolResponse[factoryapi.FactorySessionExecutionResponse]{Error: &envelope}
	}

	mapped := apifactorysession.AsyncStartResponseToAPI(result)
	return ToolResponse[factoryapi.FactorySessionExecutionResponse]{Result: &mapped}
}

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
