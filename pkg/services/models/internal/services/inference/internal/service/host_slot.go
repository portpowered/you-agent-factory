package service

import (
	"context"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	inference "github.com/portpowered/infinite-you/pkg/services/models/internal/services/inference"
)

func (s *service) acquireHostSlot(
	ctx context.Context,
	request models.InvokeModelRequest,
) (inference.HostHandleSlot, error) {
	if s == nil || s.runtimeHost == nil {
		return inference.HostHandleSlot{}, models.ErrUnavailable
	}

	result, err := s.runtimeHost.EnsureModelHost(ctx, models.EnsureModelHostRequest{
		Scope: request.Scope,
		Name:  request.ModelName,
	})
	if err != nil {
		return inference.HostHandleSlot{}, err
	}
	if result.Host.ReadinessState != models.ReadinessStateReady {
		return inference.HostHandleSlot{}, models.ErrHostRuntimeNotReady
	}

	return inference.HostHandleSlot{
		Reused:   result.Outcome == models.HostEnsureAlreadyReady,
		Endpoint: s.invocationEndpoint(ctx, request.Scope, request.ModelName),
	}, nil
}

func (s *service) invocationEndpoint(
	ctx context.Context,
	scope models.RuntimeScopeRef,
	modelName string,
) string {
	provider, ok := s.runtimeHost.(interface {
		InvocationEndpoint(context.Context, models.RuntimeScopeRef, string) (string, error)
	})
	if !ok {
		return ""
	}
	endpoint, err := provider.InvocationEndpoint(ctx, scope, modelName)
	if err != nil {
		return ""
	}
	return endpoint
}
