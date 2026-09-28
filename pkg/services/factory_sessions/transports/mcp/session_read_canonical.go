package factorysession

import (
	"context"
	"errors"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apifactorysession "github.com/portpowered/infinite-you/pkg/transports/mapping/factorysession"
)

func listSessionsCanonical(ctx context.Context, sessions factorysessions.Service, prepare RequestPreparation, input ListSessionsInput) ToolResponse[factoryapi.ListFactorySessionsResponse] {
	if ctx == nil {
		envelope := executionErrorEnvelope(errMissingRequestContext)
		return ToolResponse[factoryapi.ListFactorySessionsResponse]{Error: &envelope}
	}
	if response, done := requestContextErrorResponse[factoryapi.ListFactorySessionsResponse](ctx); done {
		return response
	}
	if sessions == nil {
		envelope := unavailableServiceErrorEnvelope()
		return ToolResponse[factoryapi.ListFactorySessionsResponse]{Error: &envelope}
	}
	if prepare == nil {
		envelope := requestValidationErrorEnvelope(errors.New("Factory Session request preparation is required"))
		return ToolResponse[factoryapi.ListFactorySessionsResponse]{Error: &envelope}
	}
	request, err := apifactorysession.ListSessionsRequestFromAPI(factoryapi.ListFactorySessionsParams{Scope: input.Scope})
	if err == nil {
		request, err = prepare.PrepareListSessions(request)
	}
	if err != nil {
		envelope := executionErrorEnvelope(err)
		return ToolResponse[factoryapi.ListFactorySessionsResponse]{Error: &envelope}
	}
	mode := factorysessions.SessionOperationModeLive
	switch request.Scope {
	case factorysessions.SessionListScopeLive:
	case factorysessions.SessionListScopePersisted:
		mode = factorysessions.SessionOperationModeDurable
	case factorysessions.SessionListScopeAll:
		mode = factorysessions.SessionOperationModeAll
	default:
		envelope := requestValidationErrorEnvelope(errors.New("unsupported Factory Session list scope"))
		return ToolResponse[factoryapi.ListFactorySessionsResponse]{Error: &envelope}
	}
	result, err := sessions.List(ctx, factorysessions.SessionListRequest{Mode: mode, Filters: request.Filters})
	if err != nil {
		envelope := executionErrorEnvelope(err)
		return ToolResponse[factoryapi.ListFactorySessionsResponse]{Error: &envelope}
	}
	legacy := factorysessions.ListSessionsResult{Scope: request.Scope, DurableSessions: result.DurableSessions}
	for _, session := range result.Sessions {
		if session.Mode != factorysessions.SessionOperationModeLive {
			continue
		}
		legacy.LiveSessions = append(legacy.LiveSessions, factorysessions.LiveSessionSummary{
			ID: session.SessionID, FactoryDir: session.FactoryDir, FolderPath: session.FolderPath,
			Project: session.Project, IsDefault: session.IsDefault,
		})
	}
	mapped := apifactorysession.ListSessionsResponseToAPI(legacy)
	return ToolResponse[factoryapi.ListFactorySessionsResponse]{Result: &mapped}
}

func getSessionCanonical(ctx context.Context, sessions factorysessions.Service, input GetSessionInput) ToolResponse[factoryapi.FactorySessionDurableReadModel] {
	if ctx == nil {
		envelope := executionErrorEnvelope(errMissingRequestContext)
		return ToolResponse[factoryapi.FactorySessionDurableReadModel]{Error: &envelope}
	}
	if response, done := requestContextErrorResponse[factoryapi.FactorySessionDurableReadModel](ctx); done {
		return response
	}
	if sessions == nil {
		envelope := unavailableServiceErrorEnvelope()
		return ToolResponse[factoryapi.FactorySessionDurableReadModel]{Error: &envelope}
	}
	result, err := sessions.Get(ctx, factorysessions.SessionGetRequest{SessionID: input.SessionID, Mode: factorysessions.SessionOperationModeDurable})
	if err != nil {
		envelope := readErrorEnvelope(input.SessionID, err)
		return ToolResponse[factoryapi.FactorySessionDurableReadModel]{Error: &envelope}
	}
	if result.Durable == nil {
		envelope := executionErrorEnvelope(errors.New("canonical durable read returned no projection"))
		return ToolResponse[factoryapi.FactorySessionDurableReadModel]{Error: &envelope}
	}
	mapped := apifactorysession.SessionReadResponseToAPI(*result.Durable)
	return ToolResponse[factoryapi.FactorySessionDurableReadModel]{Result: &mapped}
}

func getResultCanonical(ctx context.Context, sessions factorysessions.Service, prepare RequestPreparation, input GetResultInput) ToolResponse[factoryapi.FactorySessionResult] {
	if ctx == nil {
		envelope := executionErrorEnvelope(errMissingRequestContext)
		return ToolResponse[factoryapi.FactorySessionResult]{Error: &envelope}
	}
	if response, done := requestContextErrorResponse[factoryapi.FactorySessionResult](ctx); done {
		return response
	}
	if sessions == nil {
		envelope := unavailableServiceErrorEnvelope()
		return ToolResponse[factoryapi.FactorySessionResult]{Error: &envelope}
	}
	if prepare == nil {
		envelope := requestValidationErrorEnvelope(errors.New("Factory Session request preparation is required"))
		return ToolResponse[factoryapi.FactorySessionResult]{Error: &envelope}
	}
	request, err := apifactorysession.ResultRequestFromAPI(factoryapi.GetFactorySessionResultsParams{Mode: input.Mode, IncludeArtifacts: input.IncludeArtifacts})
	if err == nil {
		request, err = prepare.PrepareResult(request)
	}
	if err != nil {
		envelope := requestValidationErrorEnvelope(err)
		return ToolResponse[factoryapi.FactorySessionResult]{Error: &envelope}
	}
	result, err := sessions.ReadResult(ctx, factorysessions.SessionResultReadRequest{SessionID: input.SessionID, Mode: factorysessions.SessionOperationModeDurable, Request: request})
	if err != nil {
		envelope := readErrorEnvelope(input.SessionID, err)
		return ToolResponse[factoryapi.FactorySessionResult]{Error: &envelope}
	}
	if result.Durable == nil {
		envelope := executionErrorEnvelope(errors.New("canonical durable result read returned no projection"))
		return ToolResponse[factoryapi.FactorySessionResult]{Error: &envelope}
	}
	durable := result.Durable
	if durable.Status == factorysessions.ResultStatusNotReady {
		envelope := resultNotReadyErrorEnvelope(input.SessionID, durable.Availability)
		return ToolResponse[factoryapi.FactorySessionResult]{Error: &envelope}
	}
	mapped := apifactorysession.ResultResponseToAPI(factorysessions.ResultReadResult{
		SessionID: durable.SessionID, ResultStatus: durable.Status, SessionStatus: durable.SessionStatus,
		Mode: durable.Mode, IncludeArtifacts: durable.IncludeArtifacts, PrimaryResult: durable.PrimaryResult,
		ArtifactIDs: durable.ArtifactIDs, ArtifactRefs: durable.ArtifactRefs, Failure: durable.Failure, Availability: durable.Availability,
	})
	return ToolResponse[factoryapi.FactorySessionResult]{Result: &mapped}
}
