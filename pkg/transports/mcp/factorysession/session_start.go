package factorysession

import (
	"context"

	factorysessionexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apifactorysession "github.com/portpowered/infinite-you/pkg/transports/mapping/factorysession"
)

// StartAsync runs the durable async Factory Session contract for the
// you.factory_session.start_async MCP tool through the shared execution service.
func StartAsync(ctx context.Context, service factorysessionexecution.ExecutionService, prepare factorysessionexecution.RequestPreparation, input factoryapi.FactorySessionExecutionRequest) ToolResponse[factoryapi.FactorySessionExecutionResponse] {
	if ctx == nil {
		envelope := executionErrorEnvelope(errMissingRequestContext)
		return ToolResponse[factoryapi.FactorySessionExecutionResponse]{Error: &envelope}
	}
	if service == nil {
		envelope := unavailableServiceErrorEnvelope()
		return ToolResponse[factoryapi.FactorySessionExecutionResponse]{Error: &envelope}
	}

	startReq, err := apifactorysession.StartRequestFromAPI(input)
	if err == nil {
		startReq, err = prepare.PrepareStart(startReq)
	}
	if err != nil {
		envelope := requestValidationErrorEnvelope(err)
		return ToolResponse[factoryapi.FactorySessionExecutionResponse]{Error: &envelope}
	}

	result, err := service.StartAsync(ctx, startReq)
	if err != nil {
		envelope := executionErrorEnvelope(err)
		return ToolResponse[factoryapi.FactorySessionExecutionResponse]{Error: &envelope}
	}

	mapped := apifactorysession.AsyncStartResponseToAPI(result)
	return ToolResponse[factoryapi.FactorySessionExecutionResponse]{Result: &mapped}
}

// StartSync runs the durable sync Factory Session contract for the
// you.factory_session.start_sync MCP tool through the shared execution service.
func StartSync(ctx context.Context, service factorysessionexecution.ExecutionService, prepare factorysessionexecution.RequestPreparation, input factoryapi.FactorySessionExecutionRequest) ToolResponse[factoryapi.FactorySessionSyncExecutionResponse] {
	if ctx == nil {
		envelope := executionErrorEnvelope(errMissingRequestContext)
		return ToolResponse[factoryapi.FactorySessionSyncExecutionResponse]{Error: &envelope}
	}
	if service == nil {
		envelope := unavailableServiceErrorEnvelope()
		return ToolResponse[factoryapi.FactorySessionSyncExecutionResponse]{Error: &envelope}
	}

	startReq, err := apifactorysession.StartRequestFromAPI(input)
	if err == nil {
		startReq, err = prepare.PrepareStart(startReq)
	}
	if err != nil {
		envelope := requestValidationErrorEnvelope(err)
		return ToolResponse[factoryapi.FactorySessionSyncExecutionResponse]{Error: &envelope}
	}

	result, err := service.StartSync(ctx, startReq)
	if err != nil {
		envelope := executionErrorEnvelope(err)
		return ToolResponse[factoryapi.FactorySessionSyncExecutionResponse]{Error: &envelope}
	}

	mapped := apifactorysession.SyncStartResponseToAPI(result)
	return ToolResponse[factoryapi.FactorySessionSyncExecutionResponse]{Result: &mapped}
}
