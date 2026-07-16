package service

import (
	"context"
	"fmt"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	"github.com/portpowered/infinite-you/pkg/factory/sessions/controlplane"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/factorysession"
)

// ListFactorySessions returns live workspace session summaries through control-plane read policy.
func (s *Service) ListFactorySessions(ctx context.Context) (factoryapi.ListFactorySessionsResponse, error) {
	if s == nil || s.host == nil {
		return factoryapi.ListFactorySessionsResponse{}, fmt.Errorf("factory session gateway is required")
	}
	return controlplane.ListLiveFactorySessions(ctx, s.host)
}

// GetFactorySession returns one live session detail through control-plane read routing.
func (s *Service) GetFactorySession(ctx context.Context, sessionID string) (factoryapi.FactorySession, error) {
	if s == nil || s.host == nil {
		return factoryapi.FactorySession{}, fmt.Errorf("factory session gateway is required")
	}
	return controlplane.GetLiveFactorySession(ctx, s.host, sessionID)
}

// GetFactorySessionSyncPreflight validates reconnect cursors before live event recovery.
func (s *Service) GetFactorySessionSyncPreflight(
	ctx context.Context,
	sessionID string,
	reconnect *interfaces.FactoryEventReconnectCursor,
	logicalResolve *interfaces.FactorySessionLogicalResolveHint,
) (factoryapi.FactorySessionSyncPreflightResponse, error) {
	if s == nil || s.host == nil {
		return factoryapi.FactorySessionSyncPreflightResponse{}, fmt.Errorf("factory session gateway is required")
	}
	result, err := controlplane.GetLiveFactorySessionSyncPreflight(ctx, s.host, sessionID, reconnect, logicalResolve)
	if err != nil {
		return factoryapi.FactorySessionSyncPreflightResponse{}, err
	}
	return factorysession.SyncPreflightResultToAPI(result), nil
}

// GetFactorySessionResult returns the terminal JavaScript session result read shape.
func (s *Service) GetFactorySessionResult(ctx context.Context, sessionID string) (factoryapi.FactorySessionLiveResult, error) {
	if s == nil || s.host == nil {
		return factoryapi.FactorySessionLiveResult{}, fmt.Errorf("factory session gateway is required")
	}
	return controlplane.GetLiveFactorySessionResult(ctx, s.host, sessionID)
}

// GetFactorySessionPartialResult returns checkpoint-backed partial JavaScript results.
func (s *Service) GetFactorySessionPartialResult(
	ctx context.Context,
	sessionID string,
) (factoryapi.FactorySessionPartialResult, error) {
	if s == nil || s.host == nil {
		return factoryapi.FactorySessionPartialResult{}, fmt.Errorf("factory session gateway is required")
	}
	return controlplane.GetLiveFactorySessionPartialResult(ctx, s.host, sessionID)
}
