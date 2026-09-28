package factorysession

import (
	"context"
	"errors"
	"strings"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apifactorysession "github.com/portpowered/infinite-you/pkg/transports/mapping/factorysession"
)

func canonicalStartRequest(prepare RequestPreparation, workingRoot string, input factoryapi.FactorySessionExecutionRequest, synchronous bool) (factorysessions.SessionStartRequest, error) {
	legacy, err := apifactorysession.StartRequestFromAPI(input)
	if err != nil {
		return factorysessions.SessionStartRequest{}, err
	}
	if prepare == nil {
		return factorysessions.SessionStartRequest{}, errors.New("Factory Session request preparation is required")
	}
	legacy, err = prepare.PrepareStart(legacy)
	if err != nil {
		return factorysessions.SessionStartRequest{}, err
	}
	request := factorysessions.SessionStartRequest{
		Mode:           factorysessions.SessionOperationModeDurable,
		Correlation:    factorysessions.SessionOperationCorrelation{RequestID: legacy.RequestID},
		FolderPath:     strings.TrimSpace(workingRoot),
		Source:         legacy.Source,
		Args:           legacy.Args,
		Policy:         legacy.RequestedPolicy,
		Orchestrator:   legacy.Orchestrator,
		RuntimeOptions: legacy.Runtime,
		Synchronous:    synchronous,
	}
	if legacy.Wait != nil {
		request.Wait.CancelOnTimeout = legacy.Wait.CancelOnTimeout
		if legacy.Wait.TimeoutMillis != nil {
			request.Wait.TimeoutMillis = *legacy.Wait.TimeoutMillis
		}
	}
	return request, nil
}

func startAsyncCanonical(ctx context.Context, sessions factorysessions.Service, prepare RequestPreparation, workingRoot string, input factoryapi.FactorySessionExecutionRequest) ToolResponse[factoryapi.FactorySessionExecutionResponse] {
	if ctx == nil {
		envelope := executionErrorEnvelope(errMissingRequestContext)
		return ToolResponse[factoryapi.FactorySessionExecutionResponse]{Error: &envelope}
	}
	if response, done := requestContextErrorResponse[factoryapi.FactorySessionExecutionResponse](ctx); done {
		return response
	}
	if sessions == nil {
		envelope := unavailableServiceErrorEnvelope()
		return ToolResponse[factoryapi.FactorySessionExecutionResponse]{Error: &envelope}
	}
	request, err := canonicalStartRequest(prepare, workingRoot, input, false)
	if err != nil {
		envelope := requestValidationErrorEnvelope(err)
		return ToolResponse[factoryapi.FactorySessionExecutionResponse]{Error: &envelope}
	}
	result, err := sessions.Start(ctx, request)
	if err != nil {
		envelope := executionErrorEnvelope(err)
		return ToolResponse[factoryapi.FactorySessionExecutionResponse]{Error: &envelope}
	}
	if result.Async == nil {
		envelope := executionErrorEnvelope(errors.New("canonical durable start returned no async result"))
		return ToolResponse[factoryapi.FactorySessionExecutionResponse]{Error: &envelope}
	}
	mapped := apifactorysession.AsyncStartResponseToAPI(*result.Async)
	return ToolResponse[factoryapi.FactorySessionExecutionResponse]{Result: &mapped}
}

func startSyncCanonical(ctx context.Context, sessions factorysessions.Service, prepare RequestPreparation, workingRoot string, input factoryapi.FactorySessionExecutionRequest) ToolResponse[factoryapi.FactorySessionSyncExecutionResponse] {
	if ctx == nil {
		envelope := executionErrorEnvelope(errMissingRequestContext)
		return ToolResponse[factoryapi.FactorySessionSyncExecutionResponse]{Error: &envelope}
	}
	if response, done := requestContextErrorResponse[factoryapi.FactorySessionSyncExecutionResponse](ctx); done {
		return response
	}
	if sessions == nil {
		envelope := unavailableServiceErrorEnvelope()
		return ToolResponse[factoryapi.FactorySessionSyncExecutionResponse]{Error: &envelope}
	}
	request, err := canonicalStartRequest(prepare, workingRoot, input, true)
	if err != nil {
		envelope := requestValidationErrorEnvelope(err)
		return ToolResponse[factoryapi.FactorySessionSyncExecutionResponse]{Error: &envelope}
	}
	result, err := sessions.Start(ctx, request)
	if err != nil {
		envelope := executionErrorEnvelope(err)
		return ToolResponse[factoryapi.FactorySessionSyncExecutionResponse]{Error: &envelope}
	}
	if result.Sync == nil {
		envelope := executionErrorEnvelope(errors.New("canonical durable start returned no sync result"))
		return ToolResponse[factoryapi.FactorySessionSyncExecutionResponse]{Error: &envelope}
	}
	mapped := apifactorysession.SyncStartResponseToAPI(*result.Sync)
	return ToolResponse[factoryapi.FactorySessionSyncExecutionResponse]{Result: &mapped}
}
