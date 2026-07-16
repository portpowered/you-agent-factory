package service

import (
	"context"
	"fmt"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factorysessions "github.com/portpowered/infinite-you/pkg/factory/sessions"
	"github.com/portpowered/infinite-you/pkg/factory/sessions/controlplane"
	workflowresult "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/result"
)

// ListFactorySessions returns live workspace session summaries through control-plane read policy.
func (s *Service) ListFactorySessions(ctx context.Context) ([]factorysessions.ReadProjection, error) {
	if s == nil || s.host == nil {
		return nil, fmt.Errorf("factory session gateway is required")
	}
	return controlplane.ListLiveFactorySessions(ctx, s.host)
}

// GetFactorySession returns one live session detail through control-plane read routing.
func (s *Service) GetFactorySession(ctx context.Context, sessionID string) (factorysessions.ProjectionContext, error) {
	if s == nil || s.host == nil {
		return factorysessions.ProjectionContext{}, fmt.Errorf("factory session gateway is required")
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
	return controlplane.GetLiveFactorySessionSyncPreflight(ctx, s.host, sessionID, reconnect, logicalResolve)
}

// GetFactorySessionResult returns the terminal JavaScript session result read shape.
func (s *Service) GetFactorySessionResult(ctx context.Context, sessionID string) (workflowresult.LiveSessionResult, error) {
	if s == nil || s.host == nil {
		return workflowresult.LiveSessionResult{}, fmt.Errorf("factory session gateway is required")
	}
	return controlplane.GetLiveFactorySessionResult(ctx, s.host, sessionID)
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
