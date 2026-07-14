package service

import (
	"context"
	"fmt"

	"github.com/portpowered/infinite-you/pkg/factory/sessions/controlplane"
	factorysessionexecution "github.com/portpowered/infinite-you/pkg/factory/sessions/execution"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/factorysession"
)

// PauseDurableFactorySession applies durable pause control through the control plane.
func (s *Service) PauseDurableFactorySession(
	ctx context.Context,
	sessionID string,
	request factoryapi.FactorySessionLifecycleControlRequest,
) (factoryapi.FactorySessionLifecycleControlResponse, error) {
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
	request factoryapi.FactorySessionLifecycleControlRequest,
) (factoryapi.FactorySessionLifecycleControlResponse, error) {
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
	request factoryapi.FactorySessionLifecycleControlRequest,
) (factoryapi.FactorySessionLifecycleControlResponse, error) {
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
	request factoryapi.FactorySessionLifecycleControlRequest,
) (factoryapi.FactorySessionLifecycleControlResponse, error) {
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
	request factoryapi.FactorySessionApproveRequest,
) (factoryapi.FactorySessionLifecycleControlResponse, error) {
	if s == nil || s.host == nil {
		return factoryapi.FactorySessionLifecycleControlResponse{}, fmt.Errorf("factory session gateway is required")
	}
	approve, err := factorysession.ApproveRequestFromAPI(request)
	if err != nil {
		return factoryapi.FactorySessionLifecycleControlResponse{}, err
	}
	result, err := controlplane.ApproveDurableFactorySession(ctx, s.host, sessionID, approve)
	if err != nil {
		return factoryapi.FactorySessionLifecycleControlResponse{}, err
	}
	return factorysession.LifecycleControlResponseToAPI(result), nil
}

// RetryDurableFactorySessionDispatch applies durable retry-dispatch control through the control plane.
func (s *Service) RetryDurableFactorySessionDispatch(
	ctx context.Context,
	sessionID string,
	request factoryapi.FactorySessionRetryDispatchRequest,
) (factoryapi.FactorySessionLifecycleControlResponse, error) {
	if s == nil || s.host == nil {
		return factoryapi.FactorySessionLifecycleControlResponse{}, fmt.Errorf("factory session gateway is required")
	}
	retry, err := factorysession.RetryDispatchRequestFromAPI(request)
	if err != nil {
		return factoryapi.FactorySessionLifecycleControlResponse{}, err
	}
	result, err := controlplane.RetryDurableFactorySessionDispatch(ctx, s.host, sessionID, retry)
	if err != nil {
		return factoryapi.FactorySessionLifecycleControlResponse{}, err
	}
	return factorysession.LifecycleControlResponseToAPI(result), nil
}

// InterruptDurableFactorySessionDispatch applies durable interrupt-dispatch control through the control plane.
func (s *Service) InterruptDurableFactorySessionDispatch(
	ctx context.Context,
	sessionID string,
	request factoryapi.FactorySessionInterruptDispatchRequest,
) (factoryapi.FactorySessionLifecycleControlResponse, error) {
	if s == nil || s.host == nil {
		return factoryapi.FactorySessionLifecycleControlResponse{}, fmt.Errorf("factory session gateway is required")
	}
	interrupt, err := factorysession.InterruptDispatchRequestFromAPI(request)
	if err != nil {
		return factoryapi.FactorySessionLifecycleControlResponse{}, err
	}
	result, err := controlplane.InterruptDurableFactorySessionDispatch(ctx, s.host, sessionID, interrupt)
	if err != nil {
		return factoryapi.FactorySessionLifecycleControlResponse{}, err
	}
	return factorysession.LifecycleControlResponseToAPI(result), nil
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
	request factoryapi.FactorySessionLifecycleControlRequest,
	apply durableLifecycleControlFunc,
) (factoryapi.FactorySessionLifecycleControlResponse, error) {
	if s == nil || s.host == nil {
		return factoryapi.FactorySessionLifecycleControlResponse{}, fmt.Errorf("factory session gateway is required")
	}
	control, err := factorysession.ControlRequestFromAPI(request)
	if err != nil {
		return factoryapi.FactorySessionLifecycleControlResponse{}, err
	}
	result, err := apply(ctx, s.host, sessionID, control)
	if err != nil {
		return factoryapi.FactorySessionLifecycleControlResponse{}, err
	}
	return factorysession.LifecycleControlResponseToAPI(result), nil
}
