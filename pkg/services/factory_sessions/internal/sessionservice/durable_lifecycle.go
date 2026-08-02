package service

import (
	"context"
	"fmt"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/controlplane"
	durableexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/durable_execution"
)

// PauseDurableFactorySession applies durable pause control through the control plane.
func (s *Service) PauseDurableFactorySession(
	ctx context.Context,
	sessionID string,
	request factorysessions.ControlRequest,
) (factorysessions.LifecycleControlResult, error) {
	return s.applyDurableLifecycleControl(
		ctx,
		sessionID,
		request,
		controlplane.PauseDurableFactorySession,
	)
}

// ResumeDurableFactorySession applies durable resume control through the control plane.
func (s *Service) ResumeDurableFactorySession(
	ctx context.Context,
	sessionID string,
	request factorysessions.ControlRequest,
) (factorysessions.LifecycleControlResult, error) {
	return s.applyDurableLifecycleControl(
		ctx,
		sessionID,
		request,
		controlplane.ResumeDurableFactorySession,
	)
}

// CancelDurableFactorySession applies durable cancel control through the control plane.
func (s *Service) CancelDurableFactorySession(
	ctx context.Context,
	sessionID string,
	request factorysessions.ControlRequest,
) (factorysessions.LifecycleControlResult, error) {
	return s.applyDurableLifecycleControl(
		ctx,
		sessionID,
		request,
		controlplane.CancelDurableFactorySession,
	)
}

// TerminateDurableFactorySession applies durable terminate control through the control plane.
func (s *Service) TerminateDurableFactorySession(
	ctx context.Context,
	sessionID string,
	request factorysessions.ControlRequest,
) (factorysessions.LifecycleControlResult, error) {
	return s.applyDurableLifecycleControl(
		ctx,
		sessionID,
		request,
		controlplane.TerminateDurableFactorySession,
	)
}

// ApproveDurableFactorySession applies durable approve control through the control plane.
func (s *Service) ApproveDurableFactorySession(
	ctx context.Context,
	sessionID string,
	request factorysessions.ApproveRequest,
) (factorysessions.LifecycleControlResult, error) {
	if s == nil || s.host == nil {
		return factorysessions.LifecycleControlResult{}, fmt.Errorf("factory session gateway is required")
	}
	return controlplane.ApproveDurableFactorySession(ctx, s.durableLifecycleHost(), sessionID, request)
}

// RetryDurableFactorySessionDispatch applies durable retry-dispatch control through the control plane.
func (s *Service) RetryDurableFactorySessionDispatch(
	ctx context.Context,
	sessionID string,
	request factorysessions.RetryDispatchRequest,
) (factorysessions.LifecycleControlResult, error) {
	if s == nil || s.host == nil {
		return factorysessions.LifecycleControlResult{}, fmt.Errorf("factory session gateway is required")
	}
	return controlplane.RetryDurableFactorySessionDispatch(ctx, s.durableLifecycleHost(), sessionID, request)
}

// InterruptDurableFactorySessionDispatch applies durable interrupt-dispatch control through the control plane.
func (s *Service) InterruptDurableFactorySessionDispatch(
	ctx context.Context,
	sessionID string,
	request factorysessions.InterruptDispatchRequest,
) (factorysessions.LifecycleControlResult, error) {
	if s == nil || s.host == nil {
		return factorysessions.LifecycleControlResult{}, fmt.Errorf("factory session gateway is required")
	}
	return controlplane.InterruptDurableFactorySessionDispatch(ctx, s.durableLifecycleHost(), sessionID, request)
}

type durableLifecycleControlFunc func(
	context.Context,
	controlplane.DurableLifecycleHost,
	string,
	factorysessions.ControlRequest,
) (factorysessions.LifecycleControlResult, error)

func (s *Service) applyDurableLifecycleControl(
	ctx context.Context,
	sessionID string,
	request factorysessions.ControlRequest,
	apply durableLifecycleControlFunc,
) (factorysessions.LifecycleControlResult, error) {
	if s == nil || s.host == nil {
		return factorysessions.LifecycleControlResult{}, fmt.Errorf("factory session gateway is required")
	}
	return apply(ctx, s.durableLifecycleHost(), sessionID, request)
}

type durableLifecycleHost struct {
	execution durableexecution.Service
}

func (h durableLifecycleHost) DurableExecution() durableexecution.Service {
	return h.execution
}

func (s *Service) durableLifecycleHost() controlplane.DurableLifecycleHost {
	if s == nil {
		return durableLifecycleHost{}
	}
	return durableLifecycleHost{execution: s.durable}
}
