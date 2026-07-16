package controlplane

import (
	"context"
	"fmt"
	"strings"

	factorysessions "github.com/portpowered/infinite-you/pkg/factory/sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/factorysession"
)

// LiveReadHost exposes live session registry and projection seams owned by the composition root.
type LiveReadHost interface {
	ListLiveSessionIDs() []string
	GetLiveSession(sessionID string) *factorysessions.LiveSession
	RequireSession(sessionID string) (*factorysessions.LiveSession, error)
	BuildSessionProjectionContext(context.Context, *factorysessions.LiveSession) (factorysessions.ProjectionContext, error)
}

// IsDurableExecutionSessionID reports whether session reads route to durable execution.
func IsDurableExecutionSessionID(sessionID string) bool {
	return strings.HasPrefix(strings.TrimSpace(sessionID), "dur-sess-")
}

// ListLiveFactorySessions returns live workspace session summaries with runtime projection when available.
func ListLiveFactorySessions(ctx context.Context, host LiveReadHost) (factoryapi.ListFactorySessionsResponse, error) {
	if host == nil {
		return factoryapi.ListFactorySessionsResponse{}, nil
	}
	sessionIDs := host.ListLiveSessionIDs()
	summaries := make([]factoryapi.FactorySessionSummary, 0, len(sessionIDs))
	for _, sessionID := range sessionIDs {
		session := host.GetLiveSession(sessionID)
		if session == nil {
			continue
		}
		projectionCtx, err := host.BuildSessionProjectionContext(ctx, session)
		if err != nil {
			summaries = append(summaries, factorysession.SessionSummaryToAPI(session))
			continue
		}
		summaries = append(summaries, factorysessions.SummaryWithRuntime(projectionCtx))
	}
	factorysession.SortSessionSummaries(summaries)
	return factoryapi.ListFactorySessionsResponse{Sessions: summaries}, nil
}

// GetLiveFactorySession returns one live session detail after control-plane read routing.
func GetLiveFactorySession(
	ctx context.Context,
	host LiveReadHost,
	sessionID string,
) (factoryapi.FactorySession, error) {
	if IsDurableExecutionSessionID(sessionID) {
		return factoryapi.FactorySession{}, fmt.Errorf("%w: %s", apisurface.ErrFactorySessionNotFound, sessionID)
	}
	if host == nil {
		return factoryapi.FactorySession{}, fmt.Errorf("factory session gateway is required")
	}
	session, err := host.RequireSession(sessionID)
	if err != nil {
		return factoryapi.FactorySession{}, err
	}
	projectionCtx, err := host.BuildSessionProjectionContext(ctx, session)
	if err != nil {
		return factoryapi.FactorySession{}, err
	}
	return factorysessions.SessionResponse(projectionCtx), nil
}
