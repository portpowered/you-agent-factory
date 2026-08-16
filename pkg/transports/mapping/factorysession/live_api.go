package factorysession

import (
	"context"
	"errors"
	"fmt"
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
)

// LiveAPI maps generated live-session contracts onto the canonical Factory
// Session application gateway and registry.
type LiveAPI struct {
	control factorysessions.LiveControlService
	gateway LiveGateway
}

var _ apisurface.LiveSessionAPI = (*LiveAPI)(nil)

// LiveGateway is the non-control live-session boundary retained by the
// transport mapper for response streams, reconnect, and result inspection.
// These methods are the P5B transport compatibility seam; canonical live
// result ownership remains on Factory Sessions. Live control itself is
// injected separately through the owner-published
// factorysessions.LiveControlService capability retained for P5A.
type LiveGateway interface {
	factorysessions.LiveResultService
	SubscribeFactoryResponseEvents(context.Context, factorysessions.ResponseEventSubscriptionRequest) (*factorysessions.ResponseEventCursor, error)
	GetFactorySessionSyncPreflight(context.Context, string, *interfaces.FactoryEventReconnectCursor, *interfaces.FactorySessionLogicalResolveHint) (factorysessions.SyncPreflightResult, error)
}

// NewLiveAPI constructs the transport-facing live Factory Session service.
func NewLiveAPI(control factorysessions.LiveControlService, gateway LiveGateway) *LiveAPI {
	return &LiveAPI{control: control, gateway: gateway}
}

func (a *LiveAPI) requireControl() (factorysessions.LiveControlService, error) {
	if a == nil || a.control == nil {
		return nil, fmt.Errorf("Factory Session live-control service is required")
	}
	return a.control, nil
}

func (a *LiveAPI) requireGateway() (LiveGateway, error) {
	if a == nil || a.gateway == nil {
		return nil, fmt.Errorf("Factory Session service is required")
	}
	return a.gateway, nil
}

func (a *LiveAPI) ListFactorySessions(ctx context.Context) (factoryapi.ListFactorySessionsResponse, error) {
	control, err := a.requireControl()
	if err != nil {
		return factoryapi.ListFactorySessionsResponse{}, err
	}
	reads, err := control.ListFactorySessions(ctx)
	if err != nil {
		return factoryapi.ListFactorySessionsResponse{}, err
	}
	return ReadProjectionsToAPI(reads), nil
}

func (a *LiveAPI) GetFactorySession(ctx context.Context, sessionID string) (factoryapi.FactorySession, error) {
	control, err := a.requireControl()
	if err != nil {
		return factoryapi.FactorySession{}, err
	}
	projection, err := control.GetFactorySession(ctx, sessionID)
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
	control, err := a.requireControl()
	if err != nil {
		return factoryapi.OpenFactorySessionResponse{}, err
	}
	result, err := control.OpenFactorySession(ctx, OpenRequestFromAPI(request))
	if err != nil {
		return factoryapi.OpenFactorySessionResponse{}, err
	}
	if result == nil || strings.TrimSpace(result.SessionID) == "" {
		return OpenResultToAPI(result), nil
	}
	return OpenResultToAPI(result), nil
}

func (a *LiveAPI) CloseFactorySession(ctx context.Context, sessionID string) error {
	control, err := a.requireControl()
	if err != nil {
		return err
	}
	return control.CloseFactorySession(ctx, sessionID)
}

func (a *LiveAPI) PauseLiveFactorySession(ctx context.Context, sessionID string, request factorysessions.LiveControlRequest) (factoryapi.FactorySessionLifecycleControlResponse, error) {
	control, err := a.requireControl()
	if err != nil {
		return factoryapi.FactorySessionLifecycleControlResponse{}, err
	}
	result, err := control.PauseLiveFactorySession(ctx, sessionID, request)
	if err != nil {
		return factoryapi.FactorySessionLifecycleControlResponse{}, err
	}
	return LifecycleControlResponseToAPI(result), nil
}

func (a *LiveAPI) ResumeLiveFactorySession(ctx context.Context, sessionID string, request factorysessions.LiveControlRequest) (factoryapi.FactorySessionLifecycleControlResponse, error) {
	control, err := a.requireControl()
	if err != nil {
		return factoryapi.FactorySessionLifecycleControlResponse{}, err
	}
	result, err := control.ResumeLiveFactorySession(ctx, sessionID, request)
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
