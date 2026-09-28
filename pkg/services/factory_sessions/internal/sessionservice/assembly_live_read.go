package service

import (
	"context"
	"fmt"

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
	bound := runtimebinding.SessionStateFrom(session)
	if bound == nil || bound.Owner == nil {
		return factorysessions.ProjectionContext{}, fmt.Errorf("%w: session projection owner is unavailable", factorysessions.ErrRuntimeNotAvailable)
	}
	return bound.Owner.BuildSessionProjectionContext(ctx, session)
}

// Get reads live sessions from the process registry. Durable sessions retain
// their current owner until their execution state is folded into this root.
func (a *Assembly) Get(ctx context.Context, request factorysessions.SessionGetRequest) (factorysessions.SessionGetResult, error) {
	if request.Mode != factorysessions.SessionOperationModeLive {
		return a.Service.Get(ctx, request)
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
		return a.Service.List(ctx, request)
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
