package service

import (
	"context"
	"fmt"

	"github.com/portpowered/infinite-you/pkg/factory/sessions/controlplane"
	factorysessionexecution "github.com/portpowered/infinite-you/pkg/factory/sessions/execution"
)

// PauseDurableFactorySession applies durable pause control through the control plane.
func (s *Service) PauseDurableFactorySession(
	ctx context.Context,
	sessionID string,
	request factorysessionexecution.ControlRequest,
) (factorysessionexecution.LifecycleControlResult, error) {
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
	request factorysessionexecution.ControlRequest,
) (factorysessionexecution.LifecycleControlResult, error) {
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
	request factorysessionexecution.ControlRequest,
) (factorysessionexecution.LifecycleControlResult, error) {
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
	request factorysessionexecution.ControlRequest,
) (factorysessionexecution.LifecycleControlResult, error) {
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
	request factorysessionexecution.ApproveRequest,
) (factorysessionexecution.LifecycleControlResult, error) {
	if s == nil || s.host == nil {
		return factorysessionexecution.LifecycleControlResult{}, fmt.Errorf("factory session gateway is required")
	}
	return controlplane.ApproveDurableFactorySession(ctx, s.host, sessionID, request)
}

// RetryDurableFactorySessionDispatch applies durable retry-dispatch control through the control plane.
func (s *Service) RetryDurableFactorySessionDispatch(
	ctx context.Context,
	sessionID string,
	request factorysessionexecution.RetryDispatchRequest,
) (factorysessionexecution.LifecycleControlResult, error) {
	if s == nil || s.host == nil {
		return factorysessionexecution.LifecycleControlResult{}, fmt.Errorf("factory session gateway is required")
	}
	return controlplane.RetryDurableFactorySessionDispatch(ctx, s.host, sessionID, request)
}

// InterruptDurableFactorySessionDispatch applies durable interrupt-dispatch control through the control plane.
func (s *Service) InterruptDurableFactorySessionDispatch(
	ctx context.Context,
	sessionID string,
	request factorysessionexecution.InterruptDispatchRequest,
) (factorysessionexecution.LifecycleControlResult, error) {
	if s == nil || s.host == nil {
		return factorysessionexecution.LifecycleControlResult{}, fmt.Errorf("factory session gateway is required")
	}
	return controlplane.InterruptDurableFactorySessionDispatch(ctx, s.host, sessionID, request)
}

type durableLifecycleControlFunc func(
	context.Context,
	controlplane.DurableLifecycleHost,
	string,
	factorysessionexecution.ControlRequest,
) (factorysessionexecution.LifecycleControlResult, error)

func (s *Service) applyDurableLifecycleControl(
	ctx context.Context,
	sessionID string,
	request factorysessionexecution.ControlRequest,
	apply durableLifecycleControlFunc,
) (factorysessionexecution.LifecycleControlResult, error) {
	if s == nil || s.host == nil {
		return factorysessionexecution.LifecycleControlResult{}, fmt.Errorf("factory session gateway is required")
	}
	return apply(ctx, s.host, sessionID, request)
}
