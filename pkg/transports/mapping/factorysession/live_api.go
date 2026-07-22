package factorysession

import (
	"context"
	"errors"
	"fmt"
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	workflowresult "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessionexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
)

// LiveAPI maps generated live-session contracts onto the canonical Factory
// Session application gateway and registry.
type LiveAPI struct {
	gateway LiveGateway
}

var _ apisurface.LiveSessionAPI = (*LiveAPI)(nil)

// LiveGateway is the narrow domain boundary required by the live-session
// transport mapper.
type LiveGateway interface {
	OpenFactorySession(context.Context, factorysessions.OpenRequest) (*factorysessions.OpenResult, error)
	ListFactorySessions(context.Context) ([]factorysessions.ReadProjection, error)
	GetFactorySession(context.Context, string) (factorysessions.SessionProjection, error)
	ResolveFactorySession(string) *factorysessions.LiveSession
	SubscribeFactoryResponseEvents(context.Context, factorysessions.ResponseEventSubscriptionRequest) (factorysessions.ResponseEventCursor, error)
	GetFactorySessionSyncPreflight(context.Context, string, *interfaces.FactoryEventReconnectCursor, *interfaces.FactorySessionLogicalResolveHint) (factorysessions.SyncPreflightResult, error)
	GetFactorySessionResult(context.Context, string) (workflowresult.LiveSessionResult, error)
	GetFactorySessionPartialResult(context.Context, string) (workflowresult.PartialSessionResult, error)
	PauseLiveFactorySession(context.Context, string, factorysessionexecution.ControlRequest) (factorysessionexecution.LifecycleControlResult, error)
	ResumeLiveFactorySession(context.Context, string, factorysessionexecution.ControlRequest) (factorysessionexecution.LifecycleControlResult, error)
	CloseFactorySession(context.Context, string) error
}

// NewLiveAPI constructs the transport-facing live Factory Session service.
func NewLiveAPI(gateway LiveGateway) *LiveAPI {
	return &LiveAPI{gateway: gateway}
}

func (a *LiveAPI) requireGateway() (LiveGateway, error) {
	if a == nil || a.gateway == nil {
		return nil, fmt.Errorf("Factory Session service is required")
	}
	return a.gateway, nil
}

func (a *LiveAPI) ListFactorySessions(ctx context.Context) (factoryapi.ListFactorySessionsResponse, error) {
	gateway, err := a.requireGateway()
	if err != nil {
		return factoryapi.ListFactorySessionsResponse{}, err
	}
	reads, err := gateway.ListFactorySessions(ctx)
	if err != nil {
		return factoryapi.ListFactorySessionsResponse{}, err
	}
	return ReadProjectionsToAPI(reads), nil
}

func (a *LiveAPI) GetFactorySession(ctx context.Context, sessionID string) (factoryapi.FactorySession, error) {
	gateway, err := a.requireGateway()
	if err != nil {
		return factoryapi.FactorySession{}, err
	}
	projection, err := gateway.GetFactorySession(ctx, sessionID)
	if err != nil {
		return factoryapi.FactorySession{}, err
	}
	return SessionResponseToAPI(projection), nil
}

func (a *LiveAPI) GetFactorySessionSyncPreflight(ctx context.Context, sessionID string, options interfaces.FactorySessionSyncPreflightOptions) (factoryapi.FactorySessionSyncPreflightResponse, error) {
	gateway, err := a.requireGateway()
	if err != nil {
		return factoryapi.FactorySessionSyncPreflightResponse{}, err
	}
	result, err := gateway.GetFactorySessionSyncPreflight(ctx, sessionID, options.Reconnect, logicalResolveHint(options))
	if err != nil {
		return factoryapi.FactorySessionSyncPreflightResponse{}, err
	}
	return SyncPreflightResultToAPI(result), nil
}

func logicalResolveHint(options interfaces.FactorySessionSyncPreflightOptions) *interfaces.FactorySessionLogicalResolveHint {
	backendScopeID, logicalSessionKeyID := "", ""
	if options.BackendScopeID != nil {
		backendScopeID = strings.TrimSpace(*options.BackendScopeID)
	}
	if options.LogicalSessionKeyID != nil {
		logicalSessionKeyID = strings.TrimSpace(*options.LogicalSessionKeyID)
	}
	if backendScopeID == "" && logicalSessionKeyID == "" {
		return nil
	}
	return &interfaces.FactorySessionLogicalResolveHint{BackendScopeID: backendScopeID, LogicalSessionKeyID: logicalSessionKeyID}
}

func (a *LiveAPI) GetFactorySessionResult(ctx context.Context, sessionID string) (factoryapi.FactorySessionLiveResult, error) {
	gateway, err := a.requireGateway()
	if err != nil {
		return factoryapi.FactorySessionLiveResult{}, err
	}
	result, err := gateway.GetFactorySessionResult(ctx, sessionID)
	if err != nil {
		return factoryapi.FactorySessionLiveResult{}, err
	}
	return apisurface.WorkflowSessionLiveResultToAPI(result), nil
}

func (a *LiveAPI) GetFactorySessionPartialResult(ctx context.Context, sessionID string) (factoryapi.FactorySessionPartialResult, error) {
	gateway, err := a.requireGateway()
	if err != nil {
		return factoryapi.FactorySessionPartialResult{}, err
	}
	result, err := gateway.GetFactorySessionPartialResult(ctx, sessionID)
	if err != nil {
		return factoryapi.FactorySessionPartialResult{}, err
	}
	return apisurface.WorkflowSessionPartialResultToAPI(result), nil
}

func (a *LiveAPI) OpenFactorySession(ctx context.Context, request factoryapi.OpenFactorySessionRequest) (factoryapi.OpenFactorySessionResponse, error) {
	gateway, err := a.requireGateway()
	if err != nil {
		return factoryapi.OpenFactorySessionResponse{}, err
	}
	result, err := gateway.OpenFactorySession(ctx, OpenRequestFromAPI(request))
	if err != nil {
		return factoryapi.OpenFactorySessionResponse{}, err
	}
	if result == nil || strings.TrimSpace(result.SessionID) == "" {
		return OpenResultToAPI(result, nil), nil
	}
	return OpenResultToAPI(result, gateway.ResolveFactorySession(result.SessionID)), nil
}

func (a *LiveAPI) CloseFactorySession(ctx context.Context, sessionID string) error {
	gateway, err := a.requireGateway()
	if err != nil {
		return err
	}
	return gateway.CloseFactorySession(ctx, sessionID)
}

func (a *LiveAPI) PauseLiveFactorySession(ctx context.Context, sessionID string, control factorysessionexecution.ControlRequest) (factoryapi.FactorySessionLifecycleControlResponse, error) {
	gateway, err := a.requireGateway()
	if err != nil {
		return factoryapi.FactorySessionLifecycleControlResponse{}, err
	}
	result, err := gateway.PauseLiveFactorySession(ctx, sessionID, control)
	if err != nil {
		return factoryapi.FactorySessionLifecycleControlResponse{}, err
	}
	return LifecycleControlResponseToAPI(result), nil
}

func (a *LiveAPI) ResumeLiveFactorySession(ctx context.Context, sessionID string, control factorysessionexecution.ControlRequest) (factoryapi.FactorySessionLifecycleControlResponse, error) {
	gateway, err := a.requireGateway()
	if err != nil {
		return factoryapi.FactorySessionLifecycleControlResponse{}, err
	}
	result, err := gateway.ResumeLiveFactorySession(ctx, sessionID, control)
	if err != nil {
		return factoryapi.FactorySessionLifecycleControlResponse{}, err
	}
	return LifecycleControlResponseToAPI(result), nil
}

func (a *LiveAPI) SubscribeFactoryResponseEventsForSession(
	ctx context.Context,
	request factorysessions.ResponseEventSubscriptionRequest,
) (apisurface.FactoryResponseEventSubscription, error) {
	gateway, err := a.requireGateway()
	if err != nil {
		return nil, err
	}
	cursor, err := gateway.SubscribeFactoryResponseEvents(ctx, request)
	if err != nil {
		if errors.Is(err, factorysessions.ErrResponseEventStoreExpired) {
			return nil, fmt.Errorf("%w: %s", apisurface.ErrFactoryResponseEventStreamExpired, request.SessionID)
		}
		return nil, err
	}
	return NewResponseEventSubscription(cursor), nil
}
