package modelhost

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"time"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	modelassets "github.com/portpowered/infinite-you/pkg/services/models/internal/assets"

	localmodels "github.com/portpowered/infinite-you/pkg/services/models/internal/local"
	managedruntime "github.com/portpowered/infinite-you/pkg/services/models/internal/managedruntime"
)

// CatalogHost is the catalog-backed model host implementation for process-level wiring.
type CatalogHost struct {
	mu                sync.Mutex
	assetPuller       AssetPuller
	cacheInspector    CacheInspector
	sourceResolver    SourceResolver
	diagnostics       Diagnostics
	supervisor        supervisorSettings
	leases            map[string]*trackedLease
	byModel           map[string]map[string]struct{}
	runtimeSlots      map[string]*supervisedRuntime
	idleUnloadTimers  map[string]*time.Timer
	idleUnloadAfter   time.Duration
	maxLoadedRuntimes int
	seq               uint64
}

type trackedLease struct {
	lease      Lease
	modelKey   string
	runtimeID  string
	slotKey    string
	runtimeCfg *models.RuntimeConfig
	modelName  string
}

// NewHost constructs a process-wide model host after synchronously validating
// its required pull, cache, and process boundaries. Construction only allocates
// host state; it does not launch a subprocess or start application lifecycle.
func NewHost(
	assetPuller AssetPuller,
	cacheInspector CacheInspector,
	processLauncher ProcessLauncher,
	sourceResolver SourceResolver,
	readinessTimeout time.Duration,
	healthCheckInterval time.Duration,
	healthCheckPath string,
	healthChecker HealthChecker,
	clock Clock,
	serverStartBuilder ServerStartBuilder,
	diagnostics Diagnostics,
	idleUnloadAfter time.Duration,
	maxLoadedRuntimes int,
) (*CatalogHost, error) {
	if isNilDependency(assetPuller) {
		return nil, missingDependencyError("asset puller")
	}
	if isNilDependency(cacheInspector) {
		return nil, missingDependencyError("cache inspector")
	}
	if isNilDependency(processLauncher) {
		return nil, missingDependencyError("process launcher")
	}
	if isNilDependency(sourceResolver) {
		return nil, missingDependencyError("source resolver")
	}
	if readinessTimeout <= 0 {
		return nil, missingDependencyError("readiness timeout")
	}
	if healthCheckInterval <= 0 {
		return nil, missingDependencyError("health check interval")
	}
	if strings.TrimSpace(healthCheckPath) == "" {
		return nil, missingDependencyError("health check path")
	}
	if isNilDependency(healthChecker) {
		return nil, missingDependencyError("health checker")
	}
	if isNilDependency(clock) {
		return nil, missingDependencyError("clock")
	}
	if serverStartBuilder == nil {
		return nil, missingDependencyError("server start builder")
	}
	supervisor := supervisorSettings{
		ReadinessTimeout: readinessTimeout, HealthCheckInterval: healthCheckInterval,
		HealthCheckPath: healthCheckPath, ProcessLauncher: processLauncher,
		HealthChecker: healthChecker, Clock: clock, ServerStartBuilder: serverStartBuilder,
	}
	return newCatalogHost(
		assetPuller,
		cacheInspector,
		sourceResolver,
		supervisor,
		diagnostics,
		idleUnloadAfter,
		maxLoadedRuntimes,
	), nil
}

func newCatalogHost(
	assetPuller AssetPuller,
	cacheInspector CacheInspector,
	sourceResolver SourceResolver,
	supervisor supervisorSettings,
	diagnostics Diagnostics,
	idleUnloadAfter time.Duration,
	maxLoadedRuntimes int,
) *CatalogHost {
	idleUnloadAfter, maxLoadedRuntimes = normalizeLeasePolicy(idleUnloadAfter, maxLoadedRuntimes)
	supervisor.Diagnostics = diagnostics
	return &CatalogHost{
		assetPuller:       assetPuller,
		cacheInspector:    cacheInspector,
		sourceResolver:    sourceResolver,
		diagnostics:       diagnostics,
		supervisor:        supervisor,
		leases:            make(map[string]*trackedLease),
		byModel:           make(map[string]map[string]struct{}),
		runtimeSlots:      make(map[string]*supervisedRuntime),
		idleUnloadTimers:  make(map[string]*time.Timer),
		idleUnloadAfter:   idleUnloadAfter,
		maxLoadedRuntimes: maxLoadedRuntimes,
	}
}

func missingDependencyError(name string) error {
	return fmt.Errorf("%w: %s is required", ErrInvalidDependencies, name)
}

func isNilDependency(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

func (h *CatalogHost) ResolveIdentity(
	_ context.Context,
	runtimeCfg *models.RuntimeConfig,
	modelName string,
) (Identity, error) {
	entry, err := h.catalogEntry(runtimeCfg, modelName)
	if err != nil {
		return Identity{}, err
	}
	return h.identityFromCatalog(runtimeCfg, entry), nil
}

func (h *CatalogHost) InspectReadiness(
	ctx context.Context,
	runtimeCfg *models.RuntimeConfig,
	modelName string,
) (ReadinessSnapshot, error) {
	return h.inspectReadiness(ctx, runtimeCfg, modelName, true)
}

// InspectAssetReadiness classifies readiness from installed assets without supervised-runtime overlay.
func (h *CatalogHost) InspectAssetReadiness(
	ctx context.Context,
	runtimeCfg *models.RuntimeConfig,
	modelName string,
) (ReadinessSnapshot, error) {
	return h.inspectReadiness(ctx, runtimeCfg, modelName, false)
}

func (h *CatalogHost) inspectReadiness(
	ctx context.Context,
	runtimeCfg *models.RuntimeConfig,
	modelName string,
	overlaySupervised bool,
) (ReadinessSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return ReadinessSnapshot{}, cancelError(err)
	}
	entry, err := h.catalogEntry(runtimeCfg, modelName)
	if err != nil {
		return ReadinessSnapshot{}, err
	}
	identity := h.identityFromCatalog(runtimeCfg, entry)
	if string(entry.Summary.ProviderLocality) != models.RuntimeModelLocalityLocal {
		return ClassifyReadiness(identity, CacheInspection{}, false), nil
	}
	inspection, err := h.cacheInspector.InspectRuntimeCache(ctx, runtimeCfg, modelName)
	if err != nil {
		return ReadinessSnapshot{}, err
	}
	snapshot := ClassifyReadiness(identity, inspection, false)
	if overlaySupervised {
		snapshot = h.overlaySupervisedReadiness(snapshot, inspection, runtimeCfg, modelName)
	}
	return snapshot, nil
}

func (h *CatalogHost) Pull(
	ctx context.Context,
	runtimeCfg *models.RuntimeConfig,
	modelName string,
) (PullSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return PullSnapshot{}, cancelError(err)
	}
	entry, err := h.catalogEntry(runtimeCfg, modelName)
	if err != nil {
		return PullSnapshot{}, err
	}
	if string(entry.Summary.ProviderLocality) != models.RuntimeModelLocalityLocal {
		identity := h.identityFromCatalog(runtimeCfg, entry)
		snapshot := ClassifyReadiness(identity, CacheInspection{}, true)
		pullSnapshot := PullSnapshot{
			ReadinessSnapshot: snapshot,
			PullOutcome:       managedruntime.PullOutcomeUnsupportedRuntime,
		}
		return pullSnapshot, &ReadinessError{Snapshot: snapshot, Cause: ErrUnsupportedRuntime}
	}
	pullResult, err := h.assetPuller.PullModel(ctx, runtimeCfg, modelName)
	if err != nil {
		readiness := pullResult.Snapshot
		var pullErr *modelassets.PullError
		if errors.As(err, &pullErr) {
			readiness = managedRuntimePullSnapshot(runtimeCfg, entry, pullErr.Result)
		}
		if readiness.Identity.Name == "" {
			readiness = ClassifyReadiness(h.identityFromCatalog(runtimeCfg, entry), CacheInspection{}, false)
		}
		pullSnapshot := pullSnapshotFromAssetResult(pullResult, readiness)
		return pullSnapshot, err
	}
	readiness := pullResult.Snapshot
	if readiness.Identity.Name == "" {
		inspected, inspectErr := h.cacheInspector.InspectRuntimeCache(ctx, runtimeCfg, modelName)
		if inspectErr != nil {
			return PullSnapshot{}, inspectErr
		}
		readiness = ClassifyReadiness(h.identityFromCatalog(runtimeCfg, entry), inspected, false)
	}
	pullSnapshot := pullSnapshotFromAssetResult(pullResult, readiness)
	return pullSnapshot, nil
}

func pullSnapshotFromAssetResult(result AssetPullResult, readiness ReadinessSnapshot) PullSnapshot {
	outcome := result.PullOutcome
	if outcome == "" {
		outcome = managedruntime.PullOutcomeUnsupportedRuntime
	}
	return PullSnapshot{
		ReadinessSnapshot: readiness,
		PullOutcome:       outcome,
		LegacyOutcome:     result.LegacyOutcome,
		CachePath:         result.CachePath,
		Revision:          result.Revision,
		DownloadedFiles:   append([]PullDownloadedFile(nil), result.DownloadedFiles...),
	}
}

func (h *CatalogHost) AcquireLease(
	ctx context.Context,
	runtimeCfg *models.RuntimeConfig,
	modelName string,
	opts LeaseOptions,
) (Lease, error) {
	snapshot, inspection, modelKey, leaseCapacity, err := h.prepareAcquireLease(ctx, runtimeCfg, modelName)
	if err != nil {
		return Lease{}, err
	}
	if err := h.ensureSupervisedReadyForLease(ctx, runtimeCfg, modelName, inspection, &snapshot); err != nil {
		return Lease{}, err
	}
	if snapshot.ReadinessState != managedruntime.ReadinessStateReady {
		cause := readinessCause(snapshot)
		return Lease{}, &ReadinessError{Snapshot: snapshot, Cause: cause}
	}
	return h.issueLease(runtimeCfg, modelName, modelKey, leaseCapacity, snapshot, opts)
}

func (h *CatalogHost) prepareAcquireLease(
	ctx context.Context,
	runtimeCfg *models.RuntimeConfig,
	modelName string,
) (ReadinessSnapshot, CacheInspection, string, int, error) {
	if err := ctx.Err(); err != nil {
		return ReadinessSnapshot{}, CacheInspection{}, "", 0, cancelError(err)
	}
	entry, err := h.catalogEntry(runtimeCfg, modelName)
	if err != nil {
		return ReadinessSnapshot{}, CacheInspection{}, "", 0, err
	}
	identity := h.identityFromCatalog(runtimeCfg, entry)
	inspection, err := h.cacheInspector.InspectRuntimeCache(ctx, runtimeCfg, modelName)
	if err != nil {
		return ReadinessSnapshot{}, CacheInspection{}, "", 0, err
	}
	snapshot := h.overlaySupervisedReadiness(ClassifyReadiness(identity, inspection, false), inspection, runtimeCfg, modelName)
	modelKey := canonicalModelKey(modelName)
	leaseCapacity := h.leaseCapacityForModel(runtimeCfg, modelName)
	h.mu.Lock()
	exhausted := h.leaseCapacityExhausted(modelKey, leaseCapacity)
	h.mu.Unlock()
	if exhausted {
		h.diagnostics.logLeaseExhausted(identity)
		return ReadinessSnapshot{}, CacheInspection{}, "", 0, leaseCapacityError(modelName)
	}
	return snapshot, inspection, modelKey, leaseCapacity, nil
}

func (h *CatalogHost) ensureSupervisedReadyForLease(
	ctx context.Context,
	runtimeCfg *models.RuntimeConfig,
	modelName string,
	inspection CacheInspection,
	snapshot *ReadinessSnapshot,
) error {
	if !requiresSupervisedBackend(snapshot.Identity) || !inspection.Installed {
		return nil
	}
	worker, err := h.localWorkerForModel(runtimeCfg, modelName)
	if err != nil {
		return err
	}
	if !workerDeclaresSupervisedHealthEndpoint(worker) {
		return nil
	}
	if err := h.evictIdleRuntimesForCapacity(ctx, runtimeCfg, modelName); err != nil {
		return err
	}
	spec, err := h.supervisor.ServerStartBuilder(snapshot.Identity, inspection, worker)
	if err != nil {
		return err
	}
	slot := h.runtimeSlot(runtimeCfg, modelName)
	if err := slot.ensureReady(ctx, snapshot.Identity, spec); err != nil {
		return err
	}
	if !slot.isReady() {
		return slot.failureOutcome()
	}
	*snapshot = slot.readinessOverlay(snapshot.Identity, *snapshot)
	return nil
}

func (h *CatalogHost) issueLease(
	runtimeCfg *models.RuntimeConfig,
	modelName string,
	modelKey string,
	leaseCapacity int,
	snapshot ReadinessSnapshot,
	opts LeaseOptions,
) (Lease, error) {
	slotKey := h.runtimeSlotKey(runtimeCfg, modelName)
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.leaseCapacityExhausted(modelKey, leaseCapacity) {
		h.diagnostics.logLeaseExhausted(snapshot.Identity)
		return Lease{}, leaseCapacityError(modelName)
	}
	endpoint, err := h.supervisedLeaseEndpoint(slotKey, snapshot.Identity, runtimeCfg, modelName)
	if err != nil {
		return Lease{}, err
	}
	h.cancelIdleUnloadLocked(slotKey)
	h.seq++
	leaseID := fmt.Sprintf("model-lease-%d", h.seq)
	lease := Lease{
		ID:       leaseID,
		Identity: snapshot.Identity,
		Endpoint: endpoint,
		Holder:   strings.TrimSpace(opts.Holder),
	}
	h.leases[leaseID] = &trackedLease{
		lease:      lease,
		modelKey:   modelKey,
		runtimeID:  runtimeIdentityKey(runtimeCfg),
		slotKey:    slotKey,
		runtimeCfg: runtimeCfg,
		modelName:  modelName,
	}
	if h.byModel[modelKey] == nil {
		h.byModel[modelKey] = make(map[string]struct{})
	}
	h.byModel[modelKey][leaseID] = struct{}{}
	h.diagnostics.logLeaseAcquired(snapshot.Identity, leaseID)
	return lease, nil
}

func (h *CatalogHost) supervisedLeaseEndpoint(slotKey string, identity Identity, runtimeCfg *models.RuntimeConfig, modelName string) (string, error) {
	if !requiresSupervisedBackend(identity) {
		return "", nil
	}
	worker, err := h.localWorkerForModel(runtimeCfg, modelName)
	if err != nil {
		return "", err
	}
	if !workerDeclaresSupervisedHealthEndpoint(worker) {
		return "", nil
	}
	slot := h.runtimeSlots[slotKey]
	if slot == nil || !slot.isReady() {
		return "", ErrRuntimeNotReady
	}
	endpoint := slot.endpointValue()
	if endpoint == "" {
		return "", ErrRuntimeNotReady
	}
	return endpoint, nil
}

func (h *CatalogHost) ReleaseLease(_ context.Context, leaseID string) error {
	h.mu.Lock()
	tracked, ok := h.leases[leaseID]
	if !ok {
		h.mu.Unlock()
		return ErrLeaseNotFound
	}
	delete(h.leases, leaseID)
	modelKey := tracked.modelKey
	identity := tracked.lease.Identity
	if leases, ok := h.byModel[modelKey]; ok {
		delete(leases, leaseID)
		if len(leases) == 0 {
			delete(h.byModel, modelKey)
		}
	}
	slotKey := tracked.slotKey
	runtimeCfg := tracked.runtimeCfg
	modelName := tracked.modelName
	h.mu.Unlock()

	h.diagnostics.logLeaseReleased(identity, leaseID)

	if slotKey != "" && runtimeCfg != nil {
		h.mu.Lock()
		h.scheduleIdleUnloadIfIdle(slotKey, runtimeCfg, modelName)
		h.mu.Unlock()
	}
	return nil
}

func (h *CatalogHost) Unload(
	ctx context.Context,
	runtimeCfg *models.RuntimeConfig,
	modelName string,
) error {
	modelKey := canonicalModelKey(modelName)
	h.mu.Lock()
	if leases := h.byModel[modelKey]; len(leases) > 0 {
		h.mu.Unlock()
		return &ReadinessError{
			Snapshot: ReadinessSnapshot{
				Identity:       Identity{Name: strings.TrimSpace(modelName)},
				ReadinessState: managedruntime.ReadinessStateFailed,
				LifecycleState: managedruntime.LifecycleStateLoaded,
				FailureClass:   FailureClassCapacityExhausted,
			},
			Cause: ErrCapacityExhausted,
		}
	}
	slotKey := h.runtimeSlotKey(runtimeCfg, modelName)
	slot := h.runtimeSlots[slotKey]
	if slot != nil && slot.isLoading() {
		h.mu.Unlock()
		return &ReadinessError{
			Snapshot: ReadinessSnapshot{
				Identity:       Identity{Name: strings.TrimSpace(modelName)},
				ReadinessState: managedruntime.ReadinessStateLoading,
				LifecycleState: managedruntime.LifecycleStateLoading,
				FailureClass:   FailureClassCapacityExhausted,
			},
			Cause: ErrCapacityExhausted,
		}
	}
	h.cancelIdleUnloadLocked(slotKey)
	h.mu.Unlock()
	identity := Identity{Name: strings.TrimSpace(modelName)}
	if entry, entryErr := h.catalogEntry(runtimeCfg, modelName); entryErr == nil {
		identity = h.identityFromCatalog(runtimeCfg, entry)
	}
	h.diagnostics.logUnload(identity, "explicit")
	return h.unloadRuntime(ctx, runtimeCfg, modelName, slotKey)
}

func (h *CatalogHost) unloadRuntime(
	ctx context.Context,
	runtimeCfg *models.RuntimeConfig,
	modelName string,
	slotKey string,
) error {
	if slotKey == "" {
		slotKey = h.runtimeSlotKey(runtimeCfg, modelName)
	}
	h.mu.Lock()
	slot := h.runtimeSlots[slotKey]
	delete(h.runtimeSlots, slotKey)
	h.cancelIdleUnloadLocked(slotKey)
	h.mu.Unlock()
	if slot != nil {
		return slot.stop(ctx)
	}
	return nil
}

// Shutdown stops all supervised runtimes owned by the host.
func (h *CatalogHost) Shutdown(ctx context.Context) error {
	if h == nil {
		return nil
	}
	h.mu.Lock()
	for _, timer := range h.idleUnloadTimers {
		timer.Stop()
	}
	h.idleUnloadTimers = make(map[string]*time.Timer)
	slots := make([]*supervisedRuntime, 0, len(h.runtimeSlots))
	for _, slot := range h.runtimeSlots {
		slots = append(slots, slot)
	}
	h.runtimeSlots = make(map[string]*supervisedRuntime)
	h.mu.Unlock()

	var firstErr error
	for _, slot := range slots {
		if err := slot.stop(ctx); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (h *CatalogHost) overlaySupervisedReadiness(
	snapshot ReadinessSnapshot,
	inspection CacheInspection,
	runtimeCfg *models.RuntimeConfig,
	modelName string,
) ReadinessSnapshot {
	if !requiresSupervisedBackend(snapshot.Identity) || !inspection.Installed {
		return snapshot
	}
	slot := h.peekRuntimeSlot(runtimeCfg, modelName)
	if slot == nil {
		return snapshot
	}
	return slot.readinessOverlay(snapshot.Identity, snapshot)
}

func (h *CatalogHost) runtimeSlot(runtimeCfg *models.RuntimeConfig, modelName string) *supervisedRuntime {
	key := h.runtimeSlotKey(runtimeCfg, modelName)
	h.mu.Lock()
	defer h.mu.Unlock()
	slot, ok := h.runtimeSlots[key]
	if ok {
		return slot
	}
	slot = &supervisedRuntime{cfg: h.supervisor}
	h.runtimeSlots[key] = slot
	return slot
}

func (h *CatalogHost) peekRuntimeSlot(runtimeCfg *models.RuntimeConfig, modelName string) *supervisedRuntime {
	key := h.runtimeSlotKey(runtimeCfg, modelName)
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.runtimeSlots[key]
}

func (h *CatalogHost) runtimeSlotKey(runtimeCfg *models.RuntimeConfig, modelName string) string {
	return runtimeIdentityKey(runtimeCfg) + "|" + canonicalModelKey(modelName)
}

func (h *CatalogHost) localWorkerForModel(
	runtimeCfg *models.RuntimeConfig,
	modelName string,
) (*models.RuntimeWorker, error) {
	if runtimeCfg == nil {
		return nil, fmt.Errorf("runtime config is not available")
	}
	target := canonicalModelKey(modelName)
	for _, worker := range runtimeCfg.Workers {
		if canonicalModelKey(worker.Model) != target {
			continue
		}
		if strings.TrimSpace(worker.ModelLocality) != models.RuntimeModelLocalityLocal {
			continue
		}
		copied := worker
		return &copied, nil
	}
	return nil, fmt.Errorf("local model worker not found for %q", modelName)
}

func (h *CatalogHost) catalogEntry(runtimeCfg *models.RuntimeConfig, modelName string) (localmodels.CatalogEntry, error) {
	if runtimeCfg == nil {
		return localmodels.CatalogEntry{}, fmt.Errorf("runtime config is not available")
	}
	catalog := localmodels.BuildCatalog(runtimeCfg)
	key := localmodels.CanonicalModelName(modelName)
	if key == "" {
		return localmodels.CatalogEntry{}, fmt.Errorf("%w: empty model name", managedruntime.ErrNotFound)
	}
	entry, ok := catalog[key]
	if !ok {
		return localmodels.CatalogEntry{}, fmt.Errorf("%w: %s", managedruntime.ErrNotFound, modelName)
	}
	return entry, nil
}

func (h *CatalogHost) identityFromCatalog(
	runtimeCfg *models.RuntimeConfig,
	entry localmodels.CatalogEntry,
) Identity {
	identity := Identity{
		Name:                entry.Summary.Name,
		Locality:            managedruntime.Locality(entry.Summary.ProviderLocality),
		SupportedOperations: operationsFromCatalog(entry),
	}
	if resource := modelScopedResource(runtimeCfg, entry.Summary.Name); resource != nil {
		identity.Backend = strings.TrimSpace(resource.Backend)
		identity.LoadPolicy = strings.TrimSpace(resource.LoadPolicy)
		if h.sourceResolver != nil {
			resolution := h.sourceResolver.Resolve(
				entry.Summary.Name,
				resource.Backend,
				resource.LoadPolicy,
				resource.Provider,
			)
			identity.SourceKind = resolution.SourceKind
			identity.SourceID = resolution.SourceID
			identity.ResolverNotes = resolution.ResolverNotes
		}
	}
	return identity
}

func operationsFromCatalog(entry localmodels.CatalogEntry) []managedruntime.Operation {
	operations := make([]managedruntime.Operation, 0, len(entry.Summary.Operations))
	for _, operation := range entry.Summary.Operations {
		operations = append(operations, operation)
	}
	return operations
}

func modelScopedResource(runtimeCfg *models.RuntimeConfig, modelName string) *models.RuntimeResource {
	if runtimeCfg == nil {
		return nil
	}
	factoryCfg := runtimeCfg
	key := canonicalModelKey(modelName)
	for _, resource := range factoryCfg.Resources {
		if canonicalModelKey(resource.Model) != key {
			continue
		}
		if strings.TrimSpace(resource.Type) != models.RuntimeResourceTypeModel {
			continue
		}
		copied := resource
		return &copied
	}
	return nil
}

func managedRuntimePullSnapshot(
	runtimeCfg *models.RuntimeConfig,
	entry localmodels.CatalogEntry,
	result modelassets.PullResult,
) ReadinessSnapshot {
	identity := Identity{
		Name:     strings.TrimSpace(result.ModelName),
		Locality: managedruntime.Locality(entry.Summary.ProviderLocality),
	}
	if identity.Name == "" {
		identity.Name = entry.Summary.Name
	}
	if resource := modelScopedResource(runtimeCfg, identity.Name); resource != nil {
		identity.Backend = strings.TrimSpace(resource.Backend)
		identity.LoadPolicy = strings.TrimSpace(resource.LoadPolicy)
	}
	readiness := managedruntime.ReadinessState(strings.TrimSpace(result.ReadinessState))
	if readiness == "" {
		readiness = managedruntime.ReadinessStateFailed
	}
	lifecycle := managedruntime.LifecycleState(strings.TrimSpace(result.LifecycleState))
	if lifecycle == "" {
		lifecycle = managedruntime.LifecycleStateNotInstalled
	}
	return ReadinessSnapshot{
		Identity:       identity,
		ReadinessState: readiness,
		LifecycleState: lifecycle,
		FailureClass:   FailureClassForReadinessState(readiness),
		Diagnostics:    managedDiagnostics(identity, readiness, lifecycle),
	}
}

func readinessCause(snapshot ReadinessSnapshot) error {
	switch snapshot.FailureClass {
	case FailureClassMissingAssets:
		return ErrMissingAssets
	case FailureClassLoadingTimeout:
		return ErrLoadingTimeout
	case FailureClassProcessCrash:
		return ErrProcessCrash
	case FailureClassUnsupportedRuntime:
		return ErrUnsupportedRuntime
	case FailureClassCapacityExhausted:
		return ErrCapacityExhausted
	case FailureClassCancelled:
		return ErrCancelled
	default:
		return ErrRuntimeNotReady
	}
}

func cancelError(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w: %w", ErrCancelled, err)
}

func canonicalModelKey(modelName string) string {
	return strings.ToUpper(strings.TrimSpace(modelName))
}

func runtimeIdentityKey(runtimeCfg *models.RuntimeConfig) string {
	if runtimeCfg == nil {
		return ""
	}
	return runtimeCfg.FactoryDirectory
}
