package service

import (
	"context"
	"fmt"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
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
