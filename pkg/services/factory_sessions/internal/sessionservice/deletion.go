package service

import (
	"context"
	"fmt"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	liveruntime "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/live_runtime"
)

var _ factorysessions.LiveDeletionService = (*Service)(nil)
var _ factorysessions.LiveDeletionService = (*Assembly)(nil)

// DeleteFactorySession applies the Factory Sessions-owned safe deletion policy
// without routing the request through the destructive close lifecycle path.
func (s *Service) DeleteFactorySession(ctx context.Context, sessionID string) error {
	if s == nil || s.host == nil {
		return fmt.Errorf("factory session gateway is required")
	}
	deleter, ok := s.liveRuntime.(liveruntime.DeletionService)
	if !ok {
		return fmt.Errorf("factory session deletion service is unavailable")
	}
	return deleter.Delete(ctx, sessionID)
}

// DeleteFactorySession routes the public deletion policy to the selected
// detached live-session owner. The assembly never substitutes the destructive
// close lifecycle operation for this capability.
func (a *Assembly) DeleteFactorySession(ctx context.Context, sessionID string) error {
	owner, err := a.detachedLiveControlOwner(sessionID)
	if err != nil {
		return err
	}
	deletion, ok := owner.(factorysessions.LiveDeletionService)
	if !ok {
		return fmt.Errorf("%w: live deletion capability unavailable", factorysessions.ErrDetachedServiceUnavailable)
	}
	return deletion.DeleteFactorySession(ctx, sessionID)
}
