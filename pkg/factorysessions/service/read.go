package service

import (
	"context"
	"fmt"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/factorysessions/controlplane"
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
