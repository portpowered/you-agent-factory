package service

import (
	"context"
	"sync"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
	scopedassets "github.com/portpowered/infinite-you/pkg/services/models/internal/services/assets"
	runtimescopes "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_scopes"
)

// SlotState owns keyed host handles, holder counts and idle timers.
type SlotState struct {
	mu               sync.Mutex
	runtimeSlots     map[string]*supervisedRuntime
	capacityHolders  map[string]int
	idleUnloadTimers map[string]*idleUnload
}

func NewSlotState() *SlotState {
	return &SlotState{
		runtimeSlots:     make(map[string]*supervisedRuntime),
		capacityHolders:  make(map[string]int),
		idleUnloadTimers: make(map[string]*idleUnload),
	}
}

type slotFactsProvider struct {
	*SlotState
	scopes runtimescopes.Service
	assets scopedassets.Service
}

func NewSlotFacts(scopes runtimescopes.Service, assets scopedassets.Service, state *SlotState) modelseffects.SlotFactsProvider {
	return &slotFactsProvider{SlotState: state, scopes: scopes, assets: assets}
}

func (s *slotFactsProvider) SlotFacts(
	ctx context.Context,
	scope models.RuntimeScopeRef,
	modelName string,
) (modelseffects.SlotFacts, error) {
	if s == nil || s.scopes == nil || s.assets == nil {
		return modelseffects.SlotFacts{}, models.ErrUnavailable
	}
	if err := hostContextError(ctx); err != nil {
		return modelseffects.SlotFacts{}, err
	}
	binding, err := s.scopes.Resolve(runtimescopes.Reference(scope.String()))
	if err != nil {
		return modelseffects.SlotFacts{}, scopeError(err)
	}
	inspection, err := s.assets.InspectRuntimeCache(ctx, models.InspectModelAssetsRequest{
		Scope: scope,
		Name:  modelName,
	})
	if err != nil {
		return modelseffects.SlotFacts{}, err
	}
	snapshot := hostSnapshotFromAssets(scope, modelName, inspection)
	snapshot = s.overlaySupervisedReadiness(
		binding,
		scope,
		modelName,
		inspection,
		snapshot,
	)

	capacity := models.DefaultInvocationCapacity
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

	return modelseffects.SlotFacts{
		Readiness:       snapshot.ReadinessState,
		Capacity:        capacity,
		ContendedHolder: contendedHolder,
	}, nil
}

const slotContendedLoadingHolder = "__runtime_host_loading__"
