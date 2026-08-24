package service

import (
	"context"
	"fmt"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	liveruntime "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/live_runtime"
)

// PauseLiveFactorySession applies live pause control through the dataplane.
func (s *Service) PauseLiveFactorySession(
	ctx context.Context,
	sessionID string,
	request factorysessions.ControlRequest,
) (factorysessions.LifecycleControlResult, error) {
	return s.applyLiveLifecycleControl(ctx, sessionID, factorysessions.LifecycleControlPause, request)
}

// ResumeLiveFactorySession applies live resume control through the dataplane.
func (s *Service) ResumeLiveFactorySession(
	ctx context.Context,
	sessionID string,
	request factorysessions.ControlRequest,
) (factorysessions.LifecycleControlResult, error) {
	return s.applyLiveLifecycleControl(ctx, sessionID, factorysessions.LifecycleControlResume, request)
}

// CancelLiveFactorySession requests graceful cancellation for one live
// session while retaining the stopped session in the registry for inspection
// and a subsequent safe delete.
func (s *Service) CancelLiveFactorySession(
	ctx context.Context,
	sessionID string,
	request factorysessions.ControlRequest,
) (factorysessions.LifecycleControlResult, error) {
	return s.applyLiveLifecycleControl(ctx, sessionID, factorysessions.LifecycleControlCancel, request)
}

// TerminateLiveFactorySession requests forced termination for one live session
// while retaining the stopped session in the registry for inspection and a
// subsequent safe delete.
func (s *Service) TerminateLiveFactorySession(
	ctx context.Context,
	sessionID string,
	request factorysessions.ControlRequest,
) (factorysessions.LifecycleControlResult, error) {
	return s.applyLiveLifecycleControl(ctx, sessionID, factorysessions.LifecycleControlTerminate, request)
}

// CloseFactorySession stops one live session through the dataplane.
func (s *Service) CloseFactorySession(ctx context.Context, sessionID string) error {
	if s == nil || s.host == nil {
		return fmt.Errorf("factory session gateway is required")
	}
	return s.liveRuntime.Close(ctx, sessionID)
}

func (s *Service) applyLiveLifecycleControl(
	ctx context.Context,
	sessionID string,
	operation factorysessions.LifecycleControlKind,
	request factorysessions.ControlRequest,
) (factorysessions.LifecycleControlResult, error) {
	if s == nil || s.host == nil {
		return factorysessions.LifecycleControlResult{}, fmt.Errorf("factory session gateway is required")
	}
	return s.liveRuntime.ApplyControl(ctx, sessionID, operation, request)
}

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
