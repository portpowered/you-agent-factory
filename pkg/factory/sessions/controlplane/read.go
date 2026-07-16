package controlplane

import (
	"context"
	"fmt"
	"sort"
	"strings"

	factorysessions "github.com/portpowered/infinite-you/pkg/factory/sessions"
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
func ListLiveFactorySessions(ctx context.Context, host LiveReadHost) ([]factorysessions.ReadProjection, error) {
	if host == nil {
		return nil, nil
	}
	sessionIDs := host.ListLiveSessionIDs()
	reads := make([]factorysessions.ReadProjection, 0, len(sessionIDs))
	for _, sessionID := range sessionIDs {
		session := host.GetLiveSession(sessionID)
		if session == nil {
			continue
		}
		projectionCtx, err := host.BuildSessionProjectionContext(ctx, session)
		if err != nil {
			reads = append(reads, factorysessions.ReadProjection{
				Context: factorysessions.ProjectionContext{Session: session},
			})
			continue
		}
		reads = append(reads, factorysessions.ReadProjection{
			Context:          projectionCtx,
			RuntimeAvailable: true,
		})
	}
	sort.SliceStable(reads, func(i, j int) bool {
		left := reads[i].Context.Session
		right := reads[j].Context.Session
		if left.IsDefault != right.IsDefault {
			return left.IsDefault
		}
		return factorysessions.CanonicalFactorySessionID(left) < factorysessions.CanonicalFactorySessionID(right)
	})
	return reads, nil
}

// GetLiveFactorySession returns one live session detail after control-plane read routing.
func GetLiveFactorySession(
	ctx context.Context,
	host LiveReadHost,
	sessionID string,
) (factorysessions.ProjectionContext, error) {
	if IsDurableExecutionSessionID(sessionID) {
		return factorysessions.ProjectionContext{}, fmt.Errorf("%w: %s", factorysessions.ErrNotFound, sessionID)
	}
	if host == nil {
		return factorysessions.ProjectionContext{}, fmt.Errorf("factory session gateway is required")
	}
	session, err := host.RequireSession(sessionID)
	if err != nil {
		return factorysessions.ProjectionContext{}, err
	}
	projectionCtx, err := host.BuildSessionProjectionContext(ctx, session)
	if err != nil {
		return factorysessions.ProjectionContext{}, err
	}
	return projectionCtx, nil
}
