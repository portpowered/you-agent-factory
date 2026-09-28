package http

import (
	"context"
	"fmt"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	factorysessionmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factorysession"
)

func (a *Adapter) sessionResult(ctx context.Context, sessionID string, params factoryapi.GetFactorySessionResultsParams) (factoryapi.FactorySessionResult, error) {
	request, err := factorysessionmapping.ResultRequestFromAPI(params)
	if err != nil {
		return factoryapi.FactorySessionResult{}, err
	}
	read, err := a.sessions.ReadResult(ctx, factorysessions.SessionResultReadRequest{
		SessionID: sessionID, Mode: factorysessions.SessionOperationModeDurable, Request: request,
	})
	if err != nil {
		return factoryapi.FactorySessionResult{}, err
	}
	if read.Durable == nil {
		return factoryapi.FactorySessionResult{}, fmt.Errorf("durable factory session result is unavailable")
	}
	result := read.Durable
	return factorysessionmapping.ResultResponseToAPI(factorysessions.ResultReadResult{
		SessionID: result.SessionID, ResultStatus: result.Status, SessionStatus: result.SessionStatus,
		Mode: result.Mode, IncludeArtifacts: result.IncludeArtifacts, PrimaryResult: result.PrimaryResult,
		ArtifactIDs: result.ArtifactIDs, ArtifactRefs: result.ArtifactRefs,
		Failure: result.Failure, Availability: result.Availability,
	}), nil
}

func (a *Adapter) sessionDispatches(ctx context.Context, sessionID string, params factoryapi.ListFactorySessionDispatchesParams) (factoryapi.ListFactorySessionDispatchesResponse, error) {
	filters := factorysessions.DispatchFilters{}
	if params.Phase != nil {
		filters.Phase = string(*params.Phase)
	}
	if params.Status != nil {
		filters.Status = factorysessions.DispatchStatus(*params.Status)
	}
	result, err := a.sessions.QueryDispatches(ctx, factorysessions.DispatchQueryRequest{SessionID: sessionID, Filters: filters})
	if err != nil {
		return factoryapi.ListFactorySessionDispatchesResponse{}, err
	}
	return factorysessionmapping.ListDispatchesResponseToAPI(result), nil
}

func (a *Adapter) sessionDispatch(ctx context.Context, sessionID, dispatchID string) (factoryapi.FactoryDispatch, error) {
	result, err := a.inspection.InspectDispatch(ctx, factorysessions.SessionDispatchInspectRequest{SessionID: sessionID, DispatchID: dispatchID})
	if err != nil {
		return factoryapi.FactoryDispatch{}, err
	}
	return factorysessionmapping.DispatchDetailResponseToAPI(result), nil
}

func (a *Adapter) sessionArtifacts(ctx context.Context, sessionID string) (factoryapi.ListFactorySessionArtifactsResponse, error) {
	result, err := a.inspection.QueryArtifacts(ctx, factorysessions.SessionArtifactQueryRequest{SessionID: sessionID})
	if err != nil {
		return factoryapi.ListFactorySessionArtifactsResponse{}, err
	}
	return factorysessionmapping.ListArtifactsResponseToAPI(result), nil
}

func (a *Adapter) sessionArtifact(ctx context.Context, sessionID, artifactID string) (factoryapi.FactorySessionArtifactDetail, error) {
	result, err := a.inspection.InspectArtifact(ctx, factorysessions.SessionArtifactInspectRequest{SessionID: sessionID, ArtifactID: artifactID})
	if err != nil {
		return factoryapi.FactorySessionArtifactDetail{}, err
	}
	return factorysessionmapping.ArtifactDetailResponseToAPI(result), nil
}

func (a *Adapter) sessionEventRequest(sessionID string, params factoryapi.GetEventsBySessionIdParams) (factorysessions.SessionEventQueryRequest, error) {
	reconnect, err := factorysessionmapping.EventReconnectRequestFromAPI(params)
	return factorysessions.SessionEventQueryRequest{SessionID: sessionID, Reconnect: reconnect}, err
}

func (a *Adapter) sessionEvents(ctx context.Context, sessionID string, params factoryapi.GetEventsBySessionIdParams) (*factorydefinitions.FactoryEventStream, error) {
	request, err := a.sessionEventRequest(sessionID, params)
	if err != nil {
		return nil, err
	}
	return a.inspection.QueryEventStream(ctx, request)
}

func (a *Adapter) sessionProbeEvents(ctx context.Context, sessionID string, params factoryapi.GetEventsBySessionIdParams) error {
	request, err := a.sessionEventRequest(sessionID, params)
	if err != nil {
		return err
	}
	return a.inspection.ProbeEvents(ctx, request)
}
