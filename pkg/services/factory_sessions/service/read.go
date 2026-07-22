package service

import (
	"context"
	"fmt"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	workflowresult "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/controlplane"
)

// ResolveFactorySession returns the canonical live session entity for
// boundary adapters that need transient response-event or summary state.
func (s *Service) ResolveFactorySession(sessionID string) *factorysessions.LiveSession {
	if s == nil || s.host == nil {
		return nil
	}
	return s.host.GetLiveSession(sessionID)
}

// SubscribeFactoryResponseEvents resolves exactly one live Factory Session and
// delegates cursor, filtering, and retained-then-live policy to its owner.
func (s *Service) SubscribeFactoryResponseEvents(
	ctx context.Context,
	request factorysessions.ResponseEventSubscriptionRequest,
) (factorysessions.ResponseEventCursor, error) {
	if s == nil || s.host == nil {
		return nil, fmt.Errorf("factory session gateway is required")
	}
	return factorysessions.SubscribeFactoryResponseEvents(
		ctx,
		s.host.GetLiveSession(request.SessionID),
		request,
	)
}

// ListFactorySessions returns live workspace session summaries through control-plane read policy.
func (s *Service) ListFactorySessions(ctx context.Context) ([]factorysessions.ReadProjection, error) {
	if s == nil || s.host == nil {
		return nil, fmt.Errorf("factory session gateway is required")
	}
	return controlplane.ListLiveFactorySessions(ctx, s.host)
}

// GetFactorySession returns one live session detail through control-plane read routing.
func (s *Service) GetFactorySession(ctx context.Context, sessionID string) (factorysessions.SessionProjection, error) {
	if s == nil || s.host == nil {
		return factorysessions.SessionProjection{}, fmt.Errorf("factory session gateway is required")
	}
	return controlplane.GetLiveFactorySession(ctx, s.host, sessionID)
}

// GetFactorySessionSyncPreflight validates reconnect cursors before live event recovery.
func (s *Service) GetFactorySessionSyncPreflight(
	ctx context.Context,
	sessionID string,
	reconnect *interfaces.FactoryEventReconnectCursor,
	logicalResolve *interfaces.FactorySessionLogicalResolveHint,
) (factorysessions.SyncPreflightResult, error) {
	if s == nil || s.host == nil {
		return factorysessions.SyncPreflightResult{}, fmt.Errorf("factory session gateway is required")
	}
	return controlplane.GetLiveFactorySessionSyncPreflight(
		ctx, s.host, sessionID, reconnect, logicalResolve, s.reconnects,
	)
}

// GetFactorySessionResult returns the terminal JavaScript session result read shape.
func (s *Service) GetFactorySessionResult(ctx context.Context, sessionID string) (workflowresult.LiveSessionResult, error) {
	if s == nil || s.host == nil {
		return workflowresult.LiveSessionResult{}, fmt.Errorf("factory session gateway is required")
	}
	return controlplane.GetLiveFactorySessionResult(ctx, s.host, s.results, sessionID)
}

// GetFactorySessionPartialResult returns checkpoint-backed partial JavaScript results.
func (s *Service) GetFactorySessionPartialResult(
	ctx context.Context,
	sessionID string,
) (workflowresult.PartialSessionResult, error) {
	if s == nil || s.host == nil {
		return workflowresult.PartialSessionResult{}, fmt.Errorf("factory session gateway is required")
	}
	return controlplane.GetLiveFactorySessionPartialResult(ctx, s.host, sessionID)
}
