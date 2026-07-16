package service

import (
	"context"
	"fmt"
	"strings"

	factorysessionexecution "github.com/portpowered/infinite-you/pkg/factory/sessions/execution"
)

// PauseLiveFactorySession applies live pause control through the dataplane.
func (s *Service) PauseLiveFactorySession(
	ctx context.Context,
	sessionID string,
	request factorysessionexecution.ControlRequest,
) (factorysessionexecution.LifecycleControlResult, error) {
	return s.applyLiveLifecycleControl(ctx, sessionID, factorysessionexecution.LifecycleControlPause, request)
}

// ResumeLiveFactorySession applies live resume control through the dataplane.
func (s *Service) ResumeLiveFactorySession(
	ctx context.Context,
	sessionID string,
	request factorysessionexecution.ControlRequest,
) (factorysessionexecution.LifecycleControlResult, error) {
	return s.applyLiveLifecycleControl(ctx, sessionID, factorysessionexecution.LifecycleControlResume, request)
}

// CloseFactorySession stops one live session through the dataplane.
func (s *Service) CloseFactorySession(ctx context.Context, sessionID string) error {
	if s == nil || s.host == nil {
		return fmt.Errorf("factory session gateway is required")
	}
	if strings.TrimSpace(sessionID) == "" {
		return fmt.Errorf("factory session id is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.liveLifecycle.CloseSession(sessionID)
}

func (s *Service) applyLiveLifecycleControl(
	ctx context.Context,
	sessionID string,
	operation factorysessionexecution.LifecycleControlKind,
	request factorysessionexecution.ControlRequest,
) (factorysessionexecution.LifecycleControlResult, error) {
	if s == nil || s.host == nil {
		return factorysessionexecution.LifecycleControlResult{}, fmt.Errorf("factory session gateway is required")
	}
	return s.liveLifecycle.ApplyControl(ctx, sessionID, operation, request)
}
