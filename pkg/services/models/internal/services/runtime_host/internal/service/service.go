package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

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
		scopes:          scopes,
		assets:          assets,
		processLauncher: processLauncher,
		hostHTTP:        hostHTTP,
		hostClock:       hostClock,
		hostLogger:      hostLogger,
		hostMetrics:     hostMetrics,
		supervisor:      supervisor,
		runtimeSlots:    make(map[string]*supervisedRuntime),
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

	slotKey := runtimeSlotKey(request.Scope, request.Name)
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
	context.Context,
	models.StopModelHostRequest,
) (models.StopModelHostResult, error) {
	return models.StopModelHostResult{}, models.ErrUnsupportedOperation
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
