package service

import (
	"context"
	"fmt"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimebinding"
)

// SubscribeFactoryEventsForSession reads the event stream from the runtime
// owner attached to the canonical live session.
func (a *Assembly) SubscribeFactoryEventsForSession(ctx context.Context, sessionID string, reconnect *factorydefinitions.FactoryEventReconnectCursor) (*factorydefinitions.FactoryEventStream, error) {
	if a == nil {
		return nil, factorysessions.ErrRuntimeNotAvailable
	}
	id := strings.TrimSpace(sessionID)
	session := a.Resolve(id)
	if session == nil {
		return nil, fmt.Errorf("%w: %s", factorysessions.ErrSessionNotFound, id)
	}
	bound := runtimebinding.SessionStateFrom(session)
	if bound == nil || bound.Owner == nil {
		return nil, fmt.Errorf("%w: session event owner is unavailable", factorysessions.ErrRuntimeNotAvailable)
	}
	owner, ok := bound.Owner.(interface {
		SubscribeFactoryEventsForSession(context.Context, string, *factorydefinitions.FactoryEventReconnectCursor) (*factorydefinitions.FactoryEventStream, error)
	})
	if !ok {
		return nil, fmt.Errorf("%w: session event subscription is unavailable", factorysessions.ErrRuntimeNotAvailable)
	}
	return owner.SubscribeFactoryEventsForSession(ctx, id, reconnect)
}

func (a *Assembly) ProbeFactoryEventsForSession(ctx context.Context, sessionID string, reconnect *factorydefinitions.FactoryEventReconnectCursor) error {
	probeCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	_, err := a.SubscribeFactoryEventsForSession(probeCtx, sessionID, reconnect)
	return err
}
