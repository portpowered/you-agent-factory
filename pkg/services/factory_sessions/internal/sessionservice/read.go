package service

import (
	"context"
	"errors"
	"fmt"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	workflowresult "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/controlplane"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
	responsestreamservice "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/response_stream"
)

// ResolveFactorySession returns the canonical live session entity for
// boundary adapters that need transient response-event or summary state.
func (s *Service) ResolveFactorySession(sessionID string) *livesession.LiveSession {
	if s == nil || s.host == nil {
		return nil
	}
	return s.liveRuntime.Resolve(sessionID)
}

// SubscribeFactoryResponseEvents resolves exactly one live Factory Session and
// delegates cursor, filtering, and retained-then-live policy to its owner.
func (s *Service) SubscribeFactoryResponseEvents(
	ctx context.Context,
	request factorysessions.ResponseEventSubscriptionRequest,
) (*factorysessions.ResponseEventCursor, error) {
	if s == nil || s.host == nil {
		return nil, fmt.Errorf("factory session gateway is required")
	}
	session := s.liveRuntime.Resolve(request.SessionID)
	if session == nil {
		return nil, factorysessions.ErrSessionNotFound
	}
	if session.ResponseEvents == nil {
		return nil, factorysessions.ErrRuntimeNotAvailable
	}
	if s.responseEvents == nil {
		return nil, factorysessions.ErrRuntimeNotAvailable
	}
	cursor, err := s.responseEvents.Subscribe(ctx, session.ResponseEvents, responsestreamservice.SubscriptionRequest{
		AfterSequence: request.AfterSequence,
		DispatchID:    request.DispatchID,
		Kinds:         request.Kinds,
	})
	switch {
	case errors.Is(err, responsestreamservice.ErrInvalidCursor):
		return nil, factorysessions.ErrInvalidResponseEventCursor
	case errors.Is(err, responsestreamservice.ErrInvalidFilter):
		return nil, factorysessions.ErrInvalidResponseEventFilter
	default:
		return cursor, err
	}
}

// SubscribeDurableFactoryResponseEvents opens one durable-session response-event
// cursor through the bound durable execution implementation.
func (s *Service) SubscribeDurableFactoryResponseEvents(
	ctx context.Context,
	request factorysessions.ResponseEventSubscriptionRequest,
) (*factorysessions.ResponseEventCursor, error) {
	if s == nil || s.durable == nil {
		return nil, factorysessions.ErrRuntimeNotAvailable
	}
	subscriber, ok := s.durable.(interface {
		SubscribeResponseEvents(context.Context, string, factorysessions.ResponseEventSubscriptionRequest) (*factorysessions.ResponseEventCursor, error)
	})
	if !ok {
		return nil, factorysessions.ErrRuntimeNotAvailable
	}
	return subscriber.SubscribeResponseEvents(ctx, request.SessionID, request)
}

// ListFactorySessions returns live workspace session summaries through control-plane read policy.
func (s *Service) ListFactorySessions(ctx context.Context) ([]factorysessions.ReadProjection, error) {
	if s == nil || s.host == nil {
		return nil, fmt.Errorf("factory session gateway is required")
	}
	return s.liveRuntime.List(ctx)
}

// GetFactorySession returns one live session detail through control-plane read routing.
func (s *Service) GetFactorySession(ctx context.Context, sessionID string) (factorysessions.SessionProjection, error) {
	if s == nil || s.host == nil {
		return factorysessions.SessionProjection{}, fmt.Errorf("factory session gateway is required")
	}
	return s.liveRuntime.Get(ctx, sessionID)
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
