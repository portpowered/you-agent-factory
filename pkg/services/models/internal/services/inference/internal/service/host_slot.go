package service

import (
	"context"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	inference "github.com/portpowered/infinite-you/pkg/services/models/internal/services/inference"
)

type hostSlotKey struct {
	scope     models.RuntimeScopeRef
	modelName string
}

type hostSlotEntry struct {
	warm bool
}

func (s *service) acquireHostSlot(
	ctx context.Context,
	request models.InvokeModelRequest,
) (inference.HostHandleSlot, error) {
	if s == nil || s.runtimeHost == nil {
		return inference.HostHandleSlot{}, models.ErrUnavailable
	}
	key := hostSlotKey{scope: request.Scope, modelName: request.ModelName}

	s.hostMu.Lock()
	entry := s.hostSlots[key]
	if entry != nil && entry.warm {
		s.hostMu.Unlock()
		return inference.HostHandleSlot{Reused: true}, nil
	}
	s.hostMu.Unlock()

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

	s.hostMu.Lock()
	if s.hostSlots[key] == nil {
		s.hostSlots[key] = &hostSlotEntry{}
	}
	reused := s.hostSlots[key].warm
	s.hostSlots[key].warm = true
	s.hostMu.Unlock()

	return inference.HostHandleSlot{Reused: reused}, nil
}
