package service

import (
	"context"
	"errors"
	"fmt"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/controlplane"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimebinding"
)

var _ controlplane.LiveReadHost = (*Assembly)(nil)

func (a *Assembly) ListLiveSessionIDs() []string {
	if a == nil || a.registry == nil {
		return nil
	}
	return a.registry.IDs()
}

func (a *Assembly) GetLiveSession(sessionID string) *livesession.LiveSession {
	return a.Resolve(sessionID)
}

func (a *Assembly) RequireSession(sessionID string) (*livesession.LiveSession, error) {
	if a == nil || a.state == nil {
		return nil, factorysessions.ErrRuntimeNotAvailable
	}
	return a.state.RequireSession(sessionID)
}

func (a *Assembly) BuildSessionProjectionContext(
	ctx context.Context,
	session *livesession.LiveSession,
) (factorysessions.ProjectionContext, error) {
	if a == nil || a.projectionReader == nil {
		return factorysessions.ProjectionContext{}, fmt.Errorf("%w: session projection owner is unavailable", factorysessions.ErrRuntimeNotAvailable)
	}
	return a.projectionReader.BuildSessionProjectionContext(ctx, session)
}

// GetFactorySessionSyncPreflight uses the gateway attached to the selected
// live session. The process-owned durable gateway has no live runtime host.
func (a *Assembly) GetFactorySessionSyncPreflight(
	ctx context.Context,
	sessionID string,
	reconnect *factorydefinitions.FactoryEventReconnectCursor,
	logicalResolve *factorydefinitions.FactorySessionLogicalResolveHint,
) (factorysessions.SyncPreflightResult, error) {
	if a == nil || a.state == nil {
		return factorysessions.SyncPreflightResult{}, factorysessions.ErrRuntimeNotAvailable
	}
	session := a.Resolve(sessionID)
	if session == nil {
		// An active gateway can resolve a stale ~default selector or logical
		// session key even when the requested ID is no longer in the registry.
		session = a.state.Current()
	}
	if session == nil {
		return factorysessions.SyncPreflightResult{
			RequestedSessionID: strings.TrimSpace(sessionID),
			Reason:             factorysessions.SyncPreflightReasonSessionNotFound,
		}, nil
	}
	bound := runtimebinding.SessionStateFrom(session)
	if bound == nil || bound.Owner == nil {
		return factorysessions.SyncPreflightResult{}, factorysessions.ErrRuntimeNotAvailable
	}
	owner, ok := bound.Owner.(interface {
		Gateway() roles.SessionGateway
	})
	if !ok {
		return factorysessions.SyncPreflightResult{}, factorysessions.ErrRuntimeNotAvailable
	}
	gateway := owner.Gateway()
	if gateway == nil {
		return factorysessions.SyncPreflightResult{}, factorysessions.ErrRuntimeNotAvailable
	}
	return gateway.GetFactorySessionSyncPreflight(ctx, sessionID, reconnect, logicalResolve)
}

// ApplyLiveChange routes a live mutation to the gateway that owns the selected
// session. The process durable gateway has no live runtime host.
func (a *Assembly) ApplyLiveChange(ctx context.Context, sessionID string, request factorysessions.LiveChangeRequest) (factorysessions.LiveChangeResult, error) {
	if a != nil && a.Resolve(sessionID) == nil {
		owner, ok := a.SessionGateway.(*Service)
		if !ok {
			return factorysessions.LiveChangeResult{}, factorysessions.ErrRuntimeNotAvailable
		}
		if _, err := owner.GetSession(ctx, sessionID); err != nil {
			if errors.Is(err, factorysessions.ErrDurableSessionNotFound) {
				return factorysessions.LiveChangeResult{}, fmt.Errorf("%w: %s", factorysessions.ErrSessionNotFound, sessionID)
			}
			return factorysessions.LiveChangeResult{}, err
		}
		current := a.Resolve(factorysessions.DefaultSessionID)
		if current == nil {
			ids := a.ListLiveSessionIDs()
			if len(ids) == 1 {
				current = a.Resolve(ids[0])
			}
		}
		if current == nil {
			return factorysessions.LiveChangeResult{}, factorysessions.ErrRuntimeNotAvailable
		}
		instance := runtimebinding.BundleFromSession(current)
		if instance == nil {
			return factorysessions.LiveChangeResult{}, factorysessions.ErrRuntimeNotAvailable
		}
		return owner.ApplyDurableLiveChange(ctx, sessionID, request, instance.RuntimeService(), current.FactoryDir)
	}
	gateway, err := a.liveChangeGateway(sessionID)
	if err != nil {
		return factorysessions.LiveChangeResult{}, err
	}
	return gateway.ApplyLiveChange(ctx, sessionID, request)
}

func (a *Assembly) RecoverLiveChange(ctx context.Context, sessionID, requestID string) (factorysessions.LiveChangeResult, error) {
	if a != nil && a.Resolve(sessionID) == nil {
		owner, ok := a.SessionGateway.(*Service)
		if !ok {
			return factorysessions.LiveChangeResult{}, factorysessions.ErrRuntimeNotAvailable
		}
		return owner.RecoverDurableLiveChange(ctx, sessionID, requestID)
	}
	gateway, err := a.liveChangeGateway(sessionID)
	if err != nil {
		return factorysessions.LiveChangeResult{}, err
	}
	return gateway.RecoverLiveChange(ctx, sessionID, requestID)
}

func (a *Assembly) liveChangeGateway(sessionID string) (roles.SessionGateway, error) {
	if a == nil || a.state == nil {
		return nil, factorysessions.ErrRuntimeNotAvailable
	}
	session := a.Resolve(sessionID)
	if session == nil {
		return nil, fmt.Errorf("%w: %s", factorysessions.ErrSessionNotFound, sessionID)
	}
	bound := runtimebinding.SessionStateFrom(session)
	if bound == nil || bound.Owner == nil {
		return nil, factorysessions.ErrRuntimeNotAvailable
	}
	owner, ok := bound.Owner.(interface {
		Gateway() roles.SessionGateway
	})
	if !ok || owner.Gateway() == nil {
		return nil, factorysessions.ErrRuntimeNotAvailable
	}
	return owner.Gateway(), nil
}

// Get reads live sessions from the process registry. Durable sessions retain
// their current owner until their execution state is folded into this root.
func (a *Assembly) Get(ctx context.Context, request factorysessions.SessionGetRequest) (factorysessions.SessionGetResult, error) {
	if request.Mode != factorysessions.SessionOperationModeLive {
		return a.SessionGateway.Get(ctx, request)
	}
	if err := validateCanonicalSessionID(request.SessionID); err != nil {
		return factorysessions.SessionGetResult{}, err
	}
	projection, err := a.GetFactorySession(ctx, request.SessionID)
	if err != nil {
		return factorysessions.SessionGetResult{}, err
	}
	return factorysessions.SessionGetResult{Session: canonicalLiveSessionView(projection)}, nil
}

func (a *Assembly) List(ctx context.Context, request factorysessions.SessionListRequest) (factorysessions.SessionListResult, error) {
	if request.Mode != factorysessions.SessionOperationModeLive {
		return a.SessionGateway.List(ctx, request)
	}
	if _, err := canonicalSessionListFilters(request.Mode, request.Filters); err != nil {
		return factorysessions.SessionListResult{}, err
	}
	projections, err := a.ListFactorySessions(ctx)
	if err != nil {
		return factorysessions.SessionListResult{}, err
	}
	return factorysessions.SessionListResult{Mode: request.Mode, Sessions: canonicalLiveSessionViews(projections)}, nil
}
