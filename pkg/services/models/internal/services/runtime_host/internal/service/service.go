package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	scopedassets "github.com/portpowered/infinite-you/pkg/services/models/internal/services/assets"
	runtimescopes "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_scopes"
	runtimehost "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_host"
)

type service struct {
	scopes          runtimescopes.Service
	assets          scopedassets.Service
	processLauncher models.HostProcessLauncher
	hostHTTP        models.HostHTTPDoer
	hostClock       models.HostClock
	hostLogger      models.HostDiagnosticLogger
	hostMetrics     models.HostMetricsRecorder
	supervisor      supervisorSettings
	mu              sync.Mutex
	runtimeSlots    map[string]*supervisedRuntime
	capacityHolders map[string]int
	idleUnloadTimers map[string]*time.Timer
	idleUnloadAfter  time.Duration
	maxLoadedRuntimes int
}

var _ runtimehost.Service = (*service)(nil)

// New constructs an inert Runtime Host that validates and retains injected
// supervision effects without launching subprocesses or starting lifecycle.
func New(
	scopes runtimescopes.Service,
	assets scopedassets.Service,
	processLauncher models.HostProcessLauncher,
	hostHTTP models.HostHTTPDoer,
	hostClock models.HostClock,
	hostLogger models.HostDiagnosticLogger,
	hostMetrics models.HostMetricsRecorder,
) runtimehost.Service {
	diagnostics := hostDiagnostics{logger: hostLogger, metrics: hostMetrics}
	supervisor := supervisorSettings{
		ReadinessTimeout:    DefaultReadinessTimeout,
		HealthCheckInterval: DefaultHealthCheckInterval,
		HealthCheckPath:     DefaultHealthCheckPath,
		ProcessLauncher:     processLauncher,
		HealthChecker:       HTTPHealthChecker{Client: hostHTTP, Path: DefaultHealthCheckPath},
		Clock:               hostClock,
		ServerStartBuilder:  defaultServerStartBuilder,
		Diagnostics:         diagnostics,
	}
	return &service{
		scopes:            scopes,
		assets:            assets,
		processLauncher:   processLauncher,
		hostHTTP:          hostHTTP,
		hostClock:         hostClock,
		hostLogger:        hostLogger,
		hostMetrics:       hostMetrics,
		supervisor:        supervisor,
		runtimeSlots:      make(map[string]*supervisedRuntime),
		capacityHolders:   make(map[string]int),
		idleUnloadTimers:  make(map[string]*time.Timer),
		idleUnloadAfter:   0,
		maxLoadedRuntimes: 0,
	}
}

func (s *service) InspectModelHost(
	ctx context.Context,
	request models.InspectModelHostRequest,
) (models.InspectModelHostResult, error) {
	if err := request.Validate(); err != nil {
		return models.InspectModelHostResult{}, err
	}
	if err := hostContextError(ctx); err != nil {
		return models.InspectModelHostResult{}, err
	}
	if s == nil || s.scopes == nil || s.assets == nil {
		return models.InspectModelHostResult{}, models.ErrUnavailable
	}
	binding, err := s.scopes.Resolve(runtimescopes.Reference(request.Scope.String()))
	if err != nil {
		return models.InspectModelHostResult{}, scopeError(err)
	}
	inspection, err := s.assets.InspectRuntimeCache(ctx, models.InspectModelAssetsRequest{
		Scope: request.Scope,
		Name:  request.Name,
	})
	if err != nil {
		return models.InspectModelHostResult{}, err
	}
	snapshot := hostSnapshotFromAssets(request.Scope, request.Name, inspection)
	snapshot = s.overlaySupervisedReadiness(
		binding,
		request.Scope,
		request.Name,
		inspection,
		snapshot,
	)
	return models.InspectModelHostResult{Host: snapshot}, nil
}

func (s *service) EnsureModelHost(
	ctx context.Context,
	request models.EnsureModelHostRequest,
) (models.EnsureModelHostResult, error) {
	if err := request.Validate(); err != nil {
		return models.EnsureModelHostResult{}, err
	}
	if err := hostContextError(ctx); err != nil {
		return models.EnsureModelHostResult{}, err
	}
	if s == nil || s.scopes == nil || s.assets == nil {
		return models.EnsureModelHostResult{}, models.ErrUnavailable
	}
	binding, err := s.scopes.Resolve(runtimescopes.Reference(request.Scope.String()))
	if err != nil {
		return models.EnsureModelHostResult{}, scopeError(err)
	}
	inspection, err := s.assets.InspectRuntimeCache(ctx, models.InspectModelAssetsRequest{
		Scope: request.Scope,
		Name:  request.Name,
	})
	if err != nil {
		return models.EnsureModelHostResult{}, err
	}
	cacheInspection := cacheInspectionFromAssets(inspection)
	if !cacheInspection.Installed {
		return models.EnsureModelHostResult{}, fmt.Errorf(
			"%w: model assets are not installed",
			models.ErrHostMissingAssets,
		)
	}

	runtimeCfg := binding.RuntimeConfig()
	identity := supervisedIdentityForModel(runtimeCfg, request.Name)
	baseSnapshot := hostSnapshotFromAssets(request.Scope, request.Name, inspection)

	if !requiresSupervisedBackend(identity.Backend) {
		return models.EnsureModelHostResult{
			Host:    baseSnapshot,
			Outcome: models.HostEnsureAlreadyReady,
		}, nil
	}

	worker, err := localWorkerForModel(runtimeCfg, request.Name)
	if err != nil {
		return models.EnsureModelHostResult{}, err
	}
	if !workerDeclaresSupervisedHealthEndpoint(worker) {
		return models.EnsureModelHostResult{
			Host:    baseSnapshot,
			Outcome: models.HostEnsureAlreadyReady,
		}, nil
	}

	spec, err := s.supervisor.ServerStartBuilder(identity, cacheInspection, worker)
	if err != nil {
		return models.EnsureModelHostResult{}, err
	}

	if err := s.evictIdleRuntimesForCapacity(ctx, request.Scope, request.Name, identity); err != nil {
		return models.EnsureModelHostResult{}, err
	}

	slotKey := runtimeSlotKey(request.Scope, request.Name)
	s.cancelIdleUnload(slotKey)
	slot := s.runtimeSlot(slotKey)
	wasReady := slot.isReady()
	if err := slot.ensureReady(ctx, identity, spec); err != nil {
		return models.EnsureModelHostResult{}, err
	}
	if !slot.isReady() {
		return models.EnsureModelHostResult{}, slot.failureOutcomeLocked()
	}

	snapshot := slot.hostSnapshotOverlay(request.Scope, request.Name, baseSnapshot)
	outcome := models.HostEnsureBecameReady
	if wasReady {
		outcome = models.HostEnsureAlreadyReady
	}
	return models.EnsureModelHostResult{Host: snapshot, Outcome: outcome}, nil
}

func (s *service) StopModelHost(
	ctx context.Context,
	request models.StopModelHostRequest,
) (models.StopModelHostResult, error) {
	if err := request.Validate(); err != nil {
		return models.StopModelHostResult{}, err
	}
	if err := hostContextError(ctx); err != nil {
		return models.StopModelHostResult{}, err
	}
	if s == nil || s.scopes == nil || s.assets == nil {
		return models.StopModelHostResult{}, models.ErrUnavailable
	}
	binding, err := s.scopes.Resolve(runtimescopes.Reference(request.Scope.String()))
	if err != nil {
		return models.StopModelHostResult{}, scopeError(err)
	}
	inspection, err := s.assets.InspectRuntimeCache(ctx, models.InspectModelAssetsRequest{
		Scope: request.Scope,
		Name:  request.Name,
	})
	if err != nil {
		return models.StopModelHostResult{}, err
	}
	baseSnapshot := hostSnapshotFromAssets(request.Scope, request.Name, inspection)
	identity := supervisedIdentityForModel(binding.RuntimeConfig(), request.Name)

	slotKey := runtimeSlotKey(request.Scope, request.Name)
	s.mu.Lock()
	if s.slotHasActiveHoldersLocked(slotKey) {
		s.mu.Unlock()
		return models.StopModelHostResult{}, capacityExhaustedError(request.Name)
	}
	slot := s.runtimeSlots[slotKey]
	if slot != nil && slot.isLoading() {
		s.mu.Unlock()
		return models.StopModelHostResult{}, loadingCapacityExhaustedError(request.Name)
	}
	wasLoaded := slot != nil && slot.isReady()
	s.cancelIdleUnloadLocked(slotKey)
	s.mu.Unlock()

	if wasLoaded {
		s.supervisor.Diagnostics.logUnload(identity, "explicit")
		if err := s.unloadRuntime(ctx, identity, slotKey); err != nil {
			return models.StopModelHostResult{}, err
		}
		snapshot := baseSnapshot
		snapshot.ReadinessState = models.ReadinessStateReady
		snapshot.LifecycleState = models.LifecycleStateInstalled
		return models.StopModelHostResult{
			Host:    snapshot,
			Outcome: models.HostStopStopped,
		}, nil
	}

	return models.StopModelHostResult{
		Host:    baseSnapshot,
		Outcome: models.HostStopAlreadyStopped,
	}, nil
}

// Shutdown stops all supervised runtimes and cancels outstanding idle-unload timers.
func (s *service) Shutdown(ctx context.Context) error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	for _, timer := range s.idleUnloadTimers {
		timer.Stop()
	}
	s.idleUnloadTimers = make(map[string]*time.Timer)
	slots := make([]*supervisedRuntime, 0, len(s.runtimeSlots))
	for _, slot := range s.runtimeSlots {
		slots = append(slots, slot)
	}
	s.runtimeSlots = make(map[string]*supervisedRuntime)
	s.mu.Unlock()

	var firstErr error
	for _, slot := range slots {
		if err := slot.stop(ctx); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (s *service) unloadRuntime(
	ctx context.Context,
	identity supervisedIdentity,
	slotKey string,
) error {
	s.mu.Lock()
	slot := s.runtimeSlots[slotKey]
	delete(s.runtimeSlots, slotKey)
	s.cancelIdleUnloadLocked(slotKey)
	s.mu.Unlock()
	if slot != nil {
		return slot.stop(ctx)
	}
	return nil
}

func (s *service) cancelIdleUnload(slotKey string) {
	s.mu.Lock()
	s.cancelIdleUnloadLocked(slotKey)
	s.mu.Unlock()
}

func (s *service) releaseSlotCapacity(
	scope models.RuntimeScopeRef,
	modelName string,
	runtimeCfg *models.RuntimeConfig,
) {
	slotKey := runtimeSlotKey(scope, modelName)
	identity := supervisedIdentityForModel(runtimeCfg, modelName)

	s.mu.Lock()
	s.releaseSlotCapacityLocked(slotKey)
	s.scheduleIdleUnloadIfIdle(slotKey, identity)
	s.mu.Unlock()
}

func (s *service) overlaySupervisedReadiness(
	binding models.RuntimeBinding,
	scope models.RuntimeScopeRef,
	modelName string,
	inspection scopedassets.RuntimeCacheInspection,
	snapshot models.ModelHostSnapshot,
) models.ModelHostSnapshot {
	identity := supervisedIdentityForModel(binding.RuntimeConfig(), modelName)
	if !requiresSupervisedBackend(identity.Backend) || !inspection.Installed {
		return snapshot
	}
	slot := s.peekRuntimeSlot(runtimeSlotKey(scope, modelName))
	if slot == nil {
		return snapshot
	}
	return slot.hostSnapshotOverlay(scope, modelName, snapshot)
}

func (s *service) peekRuntimeSlot(slotKey string) *supervisedRuntime {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.runtimeSlots[slotKey]
}

func (s *service) runtimeSlot(slotKey string) *supervisedRuntime {
	s.mu.Lock()
	defer s.mu.Unlock()
	slot, ok := s.runtimeSlots[slotKey]
	if ok {
		return slot
	}
	slot = &supervisedRuntime{cfg: s.supervisor}
	s.runtimeSlots[slotKey] = slot
	return slot
}

func hostSnapshotFromAssets(
	scope models.RuntimeScopeRef,
	modelName string,
	inspection scopedassets.RuntimeCacheInspection,
) models.ModelHostSnapshot {
	snapshot := models.ModelHostSnapshot{
		Scope:       scope,
		ModelName:   modelName,
		Diagnostics: map[string]string{},
	}
	if !inspection.Supported {
		snapshot.ReadinessState = models.ReadinessStateUnsupported
		snapshot.LifecycleState = models.LifecycleStateNotApplicable
		return snapshot
	}
	if inspection.Installed {
		snapshot.ReadinessState = models.ReadinessStateReady
		snapshot.LifecycleState = models.LifecycleStateInstalled
		if inspection.CachePath != "" {
			snapshot.Diagnostics["cachePath"] = inspection.CachePath
		}
		if inspection.Revision != "" {
			snapshot.Diagnostics["revision"] = inspection.Revision
		}
		if inspection.InstalledFileCount > 0 {
			snapshot.Diagnostics["installedFileCount"] = fmt.Sprintf("%d", inspection.InstalledFileCount)
		}
		return snapshot
	}
	snapshot.ReadinessState = models.ReadinessStateMissing
	snapshot.LifecycleState = models.LifecycleStateNotInstalled
	if len(inspection.MissingAssets) > 0 {
		snapshot.Diagnostics["missingAssets"] = strings.Join(inspection.MissingAssets, ",")
	}
	return snapshot
}

func scopeError(err error) error {
	switch {
	case errors.Is(err, runtimescopes.ErrScopeForeign):
		return fmt.Errorf("%w: %v", models.ErrRuntimeScopeForeign, err)
	case errors.Is(err, runtimescopes.ErrScopeClosed):
		return fmt.Errorf("%w: %v", models.ErrRuntimeScopeClosed, err)
	case errors.Is(err, runtimescopes.ErrScopeUnknown):
		return fmt.Errorf("%w: %v", models.ErrRuntimeScopeStale, err)
	default:
		return models.ErrUnavailable
	}
}

func hostContextError(ctx context.Context) error {
	if ctx == nil || ctx.Err() == nil {
		return nil
	}
	return fmt.Errorf(
		"%w: %w",
		models.ErrHostCancelled,
		errors.Join(ctx.Err(), context.Cause(ctx)),
	)
}
