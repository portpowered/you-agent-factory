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
