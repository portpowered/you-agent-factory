package service

import (
	"context"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
	scopedassets "github.com/portpowered/infinite-you/pkg/services/models/internal/services/assets"
	runtimehost "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_host"
	leaseswire "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_host/internal/services/leases/wire"
	runtimescopes "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_scopes"
)

// NewWired constructs Runtime Host with a host-backed SlotFactsProvider bound to
// the nested leases owner. Construction remains inert and does not launch
// subprocesses or start application lifecycle.
func NewWired(
	scopes runtimescopes.Service,
	assets scopedassets.Service,
	processLauncher modelseffects.HostProcessLauncher,
	hostHTTP modelseffects.HostHTTPDoer,
	hostClock modelseffects.HostClock,
	hostLogger modelseffects.HostDiagnosticLogger,
	hostMetrics modelseffects.HostMetricsRecorder,
	options ...runtimehost.Options,
) (runtimehost.Service, error) {
	adapter := &slotFactsAdapter{}
	leases, err := leaseswire.NewService(hostClock, adapter)
	if err != nil {
		return nil, err
	}
	host := New(
		scopes,
		assets,
		leases,
		processLauncher,
		hostHTTP,
		hostClock,
		hostLogger,
		hostMetrics,
		options...,
	).(*service)
	adapter.host = host
	return host, nil
}

type slotFactsAdapter struct {
	host *service
}

func (adapter *slotFactsAdapter) SlotFacts(
	ctx context.Context,
	scope models.RuntimeScopeRef,
	modelName string,
) (modelseffects.SlotFacts, error) {
	if adapter == nil || adapter.host == nil {
		return modelseffects.SlotFacts{}, models.ErrHostRuntimeNotReady
	}
	return adapter.host.slotFacts(ctx, scope, modelName)
}

func (s *service) slotFacts(
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

	return modelseffects.SlotFacts{
		Readiness:       snapshot.ReadinessState,
		Capacity:        capacity,
		ContendedHolder: contendedHolder,
	}, nil
}

const slotContendedLoadingHolder = "__runtime_host_loading__"
