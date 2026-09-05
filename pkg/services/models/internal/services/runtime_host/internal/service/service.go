package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
	managedruntime "github.com/portpowered/infinite-you/pkg/services/models/internal/managedruntime"
	scopedassets "github.com/portpowered/infinite-you/pkg/services/models/internal/services/assets"
	runtimehost "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_host"
	hostleases "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_host/internal/services/leases"
	leaseswire "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_host/internal/services/leases/wire"
	runtimescopes "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_scopes"
)

type service struct {
	scopes            runtimescopes.Service
	assets            scopedassets.Service
	leases            hostleases.Service
	processLauncher   modelseffects.HostProcessLauncher
	hostHTTP          modelseffects.HostHTTPDoer
	hostClock         modelseffects.HostClock
	hostLogger        modelseffects.HostDiagnosticLogger
	hostMetrics       modelseffects.HostMetricsRecorder
	supervisor        supervisorSettings
	mu                sync.Mutex
	runtimeSlots      map[string]*supervisedRuntime
	capacityHolders   map[string]int
	idleUnloadTimers  map[string]*idleUnload
	idleUnloadAfter   time.Duration
	maxLoadedRuntimes int
}

type idleUnload struct {
	timer  modelseffects.HostTimer
	cancel chan struct{}
}

var _ runtimehost.Service = (*service)(nil)
var _ modelseffects.SlotCapacityCoordinator = (*service)(nil)

// New constructs an inert Runtime Host that validates and retains injected
// supervision effects without launching subprocesses or starting lifecycle.
func New(
	scopes runtimescopes.Service,
	assets scopedassets.Service,
	leases hostleases.Service,
	processLauncher modelseffects.HostProcessLauncher,
	hostHTTP modelseffects.HostHTTPDoer,
	hostClock modelseffects.HostClock,
	hostLogger modelseffects.HostDiagnosticLogger,
	hostMetrics modelseffects.HostMetricsRecorder,
	options ...runtimehost.Options,
) runtimehost.Service {
	hostOptions := runtimehost.Options{}
	if len(options) > 0 {
		hostOptions = options[0]
	}
	diagnostics := hostDiagnostics{logger: hostLogger, metrics: hostMetrics}
	supervisor := supervisorSettings{
		ReadinessTimeout:     DefaultReadinessTimeout,
		HealthCheckInterval:  DefaultHealthCheckInterval,
		HealthCheckPath:      DefaultHealthCheckPath,
		ProcessLauncher:      processLauncher,
		HealthChecker:        HTTPHealthChecker{Client: hostHTTP, Path: DefaultHealthCheckPath},
		ProtocolNegotiator:   hostOptions.ProtocolNegotiator,
		CompatibilityChecker: hostOptions.CompatibilityChecker,
		Platform:             hostOptions.Platform,
		Clock:                hostClock,
		ServerStartBuilder:   defaultServerStartBuilder,
		Diagnostics:          diagnostics,
	}
	s := &service{
		scopes:            scopes,
		assets:            assets,
		leases:            leases,
		processLauncher:   processLauncher,
		hostHTTP:          hostHTTP,
		hostClock:         hostClock,
		hostLogger:        hostLogger,
		hostMetrics:       hostMetrics,
		supervisor:        supervisor,
		runtimeSlots:      make(map[string]*supervisedRuntime),
		capacityHolders:   make(map[string]int),
		idleUnloadTimers:  make(map[string]*idleUnload),
		idleUnloadAfter:   0,
		maxLoadedRuntimes: 0,
	}
	s.idleUnloadAfter, s.maxLoadedRuntimes = normalizeHostPolicy(
		hostOptions.IdleUnloadAfter,
		hostOptions.MaxLoadedRuntimes,
	)
	leaseswire.BindCoordinator(leases, s)
	return s
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
	snapshot = sanitizeManagedHostSnapshot(
		snapshot,
		supervisedIdentityForModel(binding.RuntimeConfig(), binding.OperatorModels, request.Name),
	)
	snapshot = s.overlaySupervisedReadiness(
		binding,
		request.Scope,
		request.Name,
		inspection,
		snapshot,
	)
	return models.InspectModelHostResult{Host: snapshot}, nil
}

// pkgmaintcheck:ignore-cyclomatic-complexity pre-existing baseline debt recorded 2026-08-08; refactor this code below the maintainability threshold and remove this exemption
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
	cacheProjection := projectAssetCacheState(inspection)
	if cacheProjection.ReadinessState != models.ReadinessStateReady {
		return models.EnsureModelHostResult{}, fmt.Errorf(
			"%w: model assets are not installed",
			models.ErrHostMissingAssets,
		)
	}

	runtimeCfg := binding.RuntimeConfig()
	identity := supervisedIdentityForModel(runtimeCfg, binding.OperatorModels, request.Name)
	baseSnapshot := hostSnapshotFromAssets(request.Scope, request.Name, inspection)
	baseSnapshot = sanitizeManagedHostSnapshot(baseSnapshot, identity)

	if !requiresRuntimeHostBackend(identity.Backend) {
		return models.EnsureModelHostResult{
			Host:    baseSnapshot,
			Outcome: models.HostEnsureAlreadyReady,
		}, nil
	}

	worker, err := localWorkerForModel(runtimeCfg, request.Name)
	if err != nil {
		return models.EnsureModelHostResult{}, err
	}
	if identity.Revision == "" {
		identity.Revision = strings.TrimSpace(cacheInspection.Revision)
	}

	var spec modelseffects.HostProcessStartSpec
	if requiresPinnedGRPCBackend(identity.Backend) {
		if err := s.validatePinnedBackend(ctx, identity); err != nil {
			return models.EnsureModelHostResult{}, err
		}
		spec, err = defaultGRPCServerStartBuilder(identity, cacheInspection, worker)
	} else {
		if !workerDeclaresSupervisedHealthEndpoint(worker) {
			return models.EnsureModelHostResult{
				Host:    baseSnapshot,
				Outcome: models.HostEnsureAlreadyReady,
			}, nil
		}
		spec, err = s.supervisor.ServerStartBuilder(identity, cacheInspection, worker)
	}
	if err != nil {
		return models.EnsureModelHostResult{}, err
	}

	if err := s.evictIdleRuntimesForCapacity(ctx, request.Scope, request.Name, identity); err != nil {
		return models.EnsureModelHostResult{}, err
	}

	slotKey := runtimeSlotKey(request.Scope, request.Name)
	s.cancelIdleUnload(slotKey)
	slot := s.runtimeSlot(slotKey, request.Scope, request.Name)
	wasReady := slot.isReady()
	if err := slot.ensureReady(ctx, identity, spec); err != nil {
		return models.EnsureModelHostResult{}, err
	}
	if !slot.isReady() {
		return models.EnsureModelHostResult{}, slot.failureOutcome()
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
	identity := supervisedIdentityForModel(binding.RuntimeConfig(), binding.OperatorModels, request.Name)
	baseSnapshot = sanitizeManagedHostSnapshot(baseSnapshot, identity)

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
		if timer == nil {
			continue
		}
		if timer.timer != nil {
			timer.timer.Stop()
		}
		if timer.cancel != nil {
			close(timer.cancel)
		}
	}
	s.idleUnloadTimers = make(map[string]*idleUnload)
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

// CloseRuntimeScope releases only the supervised runtimes and lease capacity
// owned by one scope. Process-wide Models shutdown remains responsible for
// the final safety net when the root itself is closed.
func (s *service) CloseRuntimeScope(
	ctx context.Context,
	scope models.RuntimeScopeRef,
) error {
	if s == nil {
		return nil
	}
	if scope.IsZero() {
		return models.ErrRuntimeScopeInvalid
	}
	if ctx == nil {
		ctx = context.Background()
	}
	slots, modelNames := s.detachRuntimeScope(scope)
	for _, modelName := range modelNames {
		s.revokeHostLeases(scope, modelName)
	}
	return stopSupervisedRuntimes(context.WithoutCancel(ctx), slots)
}

func (s *service) detachRuntimeScope(scope models.RuntimeScopeRef) ([]*supervisedRuntime, []string) {
	prefix := scope.String() + "|"

	s.mu.Lock()
	defer s.mu.Unlock()
	slots := make([]*supervisedRuntime, 0)
	modelNames := make([]string, 0)
	for key, slot := range s.runtimeSlots {
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		delete(s.runtimeSlots, key)
		s.cancelIdleUnloadLocked(key)
		slots = append(slots, slot)
		if modelName := strings.TrimPrefix(key, prefix); modelName != "" {
			modelNames = append(modelNames, modelName)
		}
	}
	for key := range s.idleUnloadTimers {
		if strings.HasPrefix(key, prefix) {
			s.cancelIdleUnloadLocked(key)
		}
	}
	for key := range s.capacityHolders {
		if strings.HasPrefix(key, prefix) {
			delete(s.capacityHolders, key)
		}
	}
	return slots, modelNames
}

func stopSupervisedRuntimes(ctx context.Context, slots []*supervisedRuntime) error {
	var firstErr error
	for _, slot := range slots {
		if slot == nil {
			continue
		}
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

// OnLeaseCapacityAcquired records one active holder for idle-unload policy.
func (s *service) OnLeaseCapacityAcquired(
	scope models.RuntimeScopeRef,
	modelName string,
) {
	slotKey := runtimeSlotKey(scope, modelName)
	s.mu.Lock()
	s.acquireSlotCapacityLocked(slotKey)
	s.mu.Unlock()
}

// OnLeaseCapacityReleased frees one holder and may schedule idle unload.
func (s *service) OnLeaseCapacityReleased(
	scope models.RuntimeScopeRef,
	modelName string,
) {
	binding, err := s.scopes.Resolve(runtimescopes.Reference(scope.String()))
	if err != nil {
		return
	}
	s.releaseSlotCapacityWithOverlays(scope, modelName, binding.RuntimeConfig(), binding.OperatorModels)
}

func (s *service) releaseSlotCapacity(
	scope models.RuntimeScopeRef,
	modelName string,
	runtimeCfg *models.RuntimeConfig,
) {
	s.releaseSlotCapacityWithOverlays(scope, modelName, runtimeCfg, nil)
}

func (s *service) releaseSlotCapacityWithOverlays(
	scope models.RuntimeScopeRef,
	modelName string,
	runtimeCfg *models.RuntimeConfig,
	overlays map[string]models.ModelOverlay,
) {
	slotKey := runtimeSlotKey(scope, modelName)
	identity := supervisedIdentityForModel(runtimeCfg, overlays, modelName)

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
	identity := supervisedIdentityForModel(binding.RuntimeConfig(), binding.OperatorModels, modelName)
	if !requiresRuntimeHostBackend(identity.Backend) ||
		projectAssetCacheState(inspection).ReadinessState != models.ReadinessStateReady {
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

// InvocationEndpoint returns the private endpoint for a ready supervised
// runtime. It is an internal parent-to-adapter seam; public host snapshots
// intentionally continue to omit transport addresses.
func (s *service) InvocationEndpoint(
	ctx context.Context,
	scope models.RuntimeScopeRef,
	modelName string,
) (string, error) {
	if err := hostContextError(ctx); err != nil {
		return "", err
	}
	if s == nil {
		return "", models.ErrUnavailable
	}
	slot := s.peekRuntimeSlot(runtimeSlotKey(scope, modelName))
	if endpoint := slot.invocationEndpoint(); endpoint != "" {
		return endpoint, nil
	}
	return "", models.ErrHostRuntimeNotReady
}

func (s *service) runtimeSlot(
	slotKey string,
	scope models.RuntimeScopeRef,
	modelName string,
) *supervisedRuntime {
	s.mu.Lock()
	defer s.mu.Unlock()
	slot, ok := s.runtimeSlots[slotKey]
	if ok {
		return slot
	}
	settings := s.supervisor
	processFailureObserver := settings.onProcessFailure
	settings.onProcessFailure = func() {
		s.revokeHostLeases(scope, modelName)
		if processFailureObserver != nil {
			processFailureObserver()
		}
	}
	slot = &supervisedRuntime{cfg: settings}
	s.runtimeSlots[slotKey] = slot
	return slot
}

func (s *service) revokeHostLeases(scope models.RuntimeScopeRef, modelName string) {
	revoker, ok := s.leases.(interface {
		RevokeModelLeases(models.RuntimeScopeRef, string)
	})
	if !ok || revoker == nil {
		return
	}
	revoker.RevokeModelLeases(scope, modelName)
}

func (s *service) validatePinnedBackend(
	ctx context.Context,
	identity supervisedIdentity,
) error {
	platform := s.supervisor.Platform
	if strings.TrimSpace(platform.OperatingSystem) == "" ||
		strings.TrimSpace(platform.Architecture) == "" {
		return hostReadinessFailure(
			identity,
			models.HostFailureClassUnsupportedPlatform,
			models.ErrHostUnsupportedPlatform,
		)
	}
	if s.supervisor.CompatibilityChecker == nil {
		return hostReadinessFailure(
			identity,
			models.HostFailureClassUnsupportedPlatform,
			models.ErrHostUnsupportedPlatform,
		)
	}
	if err := s.supervisor.CompatibilityChecker.Check(ctx, modelseffects.HostCompatibilityRequest{
		Backend:   identity.Backend,
		ModelName: identity.Name,
		Revision:  identity.Revision,
		Platform:  platform,
	}); err != nil {
		return hostReadinessFailure(
			identity,
			models.HostFailureClassUnsupportedPlatform,
			errors.Join(models.ErrHostUnsupportedPlatform, err),
		)
	}
	if s.supervisor.ProtocolNegotiator == nil {
		return hostReadinessFailure(
			identity,
			models.HostFailureClassProtocol,
			models.ErrHostProtocolIncompatible,
		)
	}
	return nil
}

func hostReadinessFailure(
	identity supervisedIdentity,
	class models.HostFailureClass,
	cause error,
) error {
	return &models.HostReadinessError{
		Snapshot: models.HostReadinessSnapshot{
			Identity: models.HostIdentity{
				Name:       identity.Name,
				Backend:    identity.Backend,
				LoadPolicy: identity.LoadPolicy,
			},
			ReadinessState: models.ReadinessStateFailed,
			LifecycleState: models.LifecycleStateInstalled,
			FailureClass:   class,
		},
		Cause: cause,
	}
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
	projection := projectAssetCacheState(inspection)
	snapshot.ReadinessState = projection.ReadinessState
	snapshot.LifecycleState = projection.LifecycleState
	if inspection.CachePath != "" {
		snapshot.Diagnostics["cachePath"] = inspection.CachePath
	}
	if inspection.Revision != "" {
		snapshot.Diagnostics["revision"] = inspection.Revision
	}
	if inspection.InstalledFileCount > 0 {
		snapshot.Diagnostics["installedFileCount"] = fmt.Sprintf("%d", inspection.InstalledFileCount)
	}
	if len(inspection.MissingAssets) > 0 {
		snapshot.Diagnostics["missingAssets"] = strings.Join(inspection.MissingAssets, ",")
	}
	if inspection.ManifestPresent {
		snapshot.Diagnostics["manifestValid"] = fmt.Sprintf("%t", inspection.ManifestValid)
	}
	if inspection.ActivePull {
		snapshot.Diagnostics["activePull"] = "true"
	}
	if reason := strings.TrimSpace(projection.FailureReason); reason != "" {
		snapshot.Diagnostics["failureReason"] = reason
	}
	return snapshot
}

func projectAssetCacheState(
	inspection scopedassets.RuntimeCacheInspection,
) models.ManagedRuntimeStateProjection {
	return managedruntime.ProjectManagedRuntimeState(
		models.ManagedRuntimeCacheFacts{
			Locality:           models.LocalityLocal,
			Supported:          inspection.Supported,
			Installed:          inspection.Installed,
			ManifestPresent:    inspection.ManifestPresent,
			ManifestValid:      inspection.ManifestValid,
			ExpectedArtifacts:  append([]models.AssetRequirement(nil), inspection.ExpectedArtifacts...),
			ObservedArtifacts:  append([]models.AssetArtifact(nil), inspection.ObservedArtifacts...),
			InstalledFileCount: inspection.InstalledFileCount,
			PartialArtifacts:   inspection.PartialArtifacts,
			ActivePull:         inspection.ActivePull,
			IntegrityVerified:  inspection.IntegrityVerified,
			FailureReason:      inspection.FailureReason,
		},
		models.ManagedRuntimeHostFacts{},
	)
}

func sanitizeManagedHostSnapshot(
	snapshot models.ModelHostSnapshot,
	identity supervisedIdentity,
) models.ModelHostSnapshot {
	if requiresPinnedGRPCBackend(identity.Backend) {
		delete(snapshot.Diagnostics, "cachePath")
		delete(snapshot.Diagnostics, "endpoint")
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
