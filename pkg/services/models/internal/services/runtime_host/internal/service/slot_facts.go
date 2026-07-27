package service

import (
	"context"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	hostleases "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_host/internal/services/leases"
	runtimescopes "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_scopes"
)

type slotFactsAdapter struct {
	host *service
}

func (adapter *slotFactsAdapter) SlotFacts(
	ctx context.Context,
	scope models.RuntimeScopeRef,
	modelName string,
) (hostleases.SlotFacts, error) {
	if adapter == nil || adapter.host == nil {
		return hostleases.SlotFacts{}, models.ErrHostRuntimeNotReady
	}
	return adapter.host.slotFacts(ctx, scope, modelName)
}

func (s *service) slotFacts(
	ctx context.Context,
	scope models.RuntimeScopeRef,
	modelName string,
) (hostleases.SlotFacts, error) {
	if s == nil || s.scopes == nil || s.assets == nil {
		return hostleases.SlotFacts{}, models.ErrUnavailable
	}
	if err := hostContextError(ctx); err != nil {
		return hostleases.SlotFacts{}, err
	}
	binding, err := s.scopes.Resolve(runtimescopes.Reference(scope.String()))
	if err != nil {
		return hostleases.SlotFacts{}, scopeError(err)
	}
	inspection, err := s.assets.InspectRuntimeCache(ctx, models.InspectModelAssetsRequest{
		Scope: scope,
		Name:  modelName,
	})
	if err != nil {
		return hostleases.SlotFacts{}, err
	}
	snapshot := hostSnapshotFromAssets(scope, modelName, inspection)
	snapshot = s.overlaySupervisedReadiness(
		binding,
		scope,
		modelName,
		inspection,
		snapshot,
	)

	capacity := 0
	if resource := modelScopedResource(binding.RuntimeConfig(), modelName); resource != nil &&
		resource.Capacity > 0 {
		capacity = resource.Capacity
	}

	contendedHolder := ""
	slotKey := runtimeSlotKey(scope, modelName)
	s.mu.Lock()
	if slot := s.runtimeSlots[slotKey]; slot != nil && slot.isLoading() {
		contendedHolder = slotContendedLoadingHolder
	}
	s.mu.Unlock()

	return hostleases.SlotFacts{
		Readiness:       snapshot.ReadinessState,
		Capacity:        capacity,
		ContendedHolder: contendedHolder,
	}, nil
}

const slotContendedLoadingHolder = "__runtime_host_loading__"
