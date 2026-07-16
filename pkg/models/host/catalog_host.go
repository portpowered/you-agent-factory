package modelhost

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"time"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	localmodels "github.com/portpowered/infinite-you/pkg/models/local"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/transports/mapping"
)

// CatalogHost is the catalog-backed model host implementation for process-level wiring.
type CatalogHost struct {
	mu                sync.Mutex
	assetPuller       AssetPuller
	cacheInspector    CacheInspector
	opts              Options
	diagnostics       Diagnostics
	supervisor        SupervisorConfig
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
	runtimeCfg *factoryconfig.LoadedFactoryConfig
	modelName  string
}

// NewHost constructs a process-wide model host after synchronously validating
// its required pull, cache, and process boundaries. Construction only allocates
// host state; it does not launch a subprocess or start application lifecycle.
func NewHost(deps Dependencies) (*CatalogHost, error) {
	if isNilDependency(deps.AssetPuller) {
		return nil, missingDependencyError("asset puller")
	}
	if isNilDependency(deps.CacheInspector) {
		return nil, missingDependencyError("cache inspector")
	}
	if isNilDependency(deps.ProcessLauncher) {
		return nil, missingDependencyError("process launcher")
	}
	deps.Options.Supervisor.ProcessLauncher = deps.ProcessLauncher
	return newCatalogHost(deps.AssetPuller, deps.CacheInspector, deps.Options), nil
}

func newCatalogHost(assetPuller AssetPuller, cacheInspector CacheInspector, opts Options) *CatalogHost {
	if opts.SourceResolver == nil {
		opts.SourceResolver = DefaultManagedRuntimeSourceResolverAdapter()
	}
	idleUnloadAfter, maxLoadedRuntimes := normalizeLeasePolicyOptions(opts)
	supervisor := normalizeSupervisorConfig(opts.Supervisor)
	supervisor.Diagnostics = opts.Diagnostics
	return &CatalogHost{
		assetPuller:       assetPuller,
		cacheInspector:    cacheInspector,
		opts:              opts,
		diagnostics:       opts.Diagnostics,
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
	runtimeCfg *factoryconfig.LoadedFactoryConfig,
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
	runtimeCfg *factoryconfig.LoadedFactoryConfig,
	modelName string,
) (ReadinessSnapshot, error) {
	return h.inspectReadiness(ctx, runtimeCfg, modelName, true)
}

// InspectAssetReadiness classifies readiness from installed assets without supervised-runtime overlay.
func (h *CatalogHost) InspectAssetReadiness(
	ctx context.Context,
	runtimeCfg *factoryconfig.LoadedFactoryConfig,
	modelName string,
) (ReadinessSnapshot, error) {
	return h.inspectReadiness(ctx, runtimeCfg, modelName, false)
}

func (h *CatalogHost) inspectReadiness(
	ctx context.Context,
	runtimeCfg *factoryconfig.LoadedFactoryConfig,
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
	if entry.Summary.ProviderLocality != factoryapi.WorkerModelLocalityLocal {
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
	runtimeCfg *factoryconfig.LoadedFactoryConfig,
	modelName string,
) (PullSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return PullSnapshot{}, cancelError(err)
	}
	entry, err := h.catalogEntry(runtimeCfg, modelName)
	if err != nil {
		return PullSnapshot{}, err
	}
	if entry.Summary.ProviderLocality != factoryapi.WorkerModelLocalityLocal {
		identity := h.identityFromCatalog(runtimeCfg, entry)
		snapshot := ClassifyReadiness(identity, CacheInspection{}, true)
		pullSnapshot := PullSnapshot{
			ReadinessSnapshot: snapshot,
			PullOutcome:       factoryapi.ManagedRuntimePullOutcomeUNSUPPORTEDRUNTIME,
		}
		return pullSnapshot, &ReadinessError{Snapshot: snapshot, Cause: ErrUnsupportedRuntime}
	}
	pullResult, err := h.assetPuller.PullModel(ctx, runtimeCfg, modelName)
	if err != nil {
		readiness := pullResult.Snapshot
		var pullErr *apisurface.ManagedRuntimePullError
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
		outcome = factoryapi.ManagedRuntimePullOutcomeUNSUPPORTEDRUNTIME
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
	runtimeCfg *factoryconfig.LoadedFactoryConfig,
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
	if snapshot.ReadinessState != factoryapi.ManagedRuntimeReadinessStateREADY {
		cause := readinessCause(snapshot)
		return Lease{}, &ReadinessError{Snapshot: snapshot, Cause: cause}
	}
	return h.issueLease(runtimeCfg, modelName, modelKey, leaseCapacity, snapshot, opts)
}

func (h *CatalogHost) prepareAcquireLease(
	ctx context.Context,
	runtimeCfg *factoryconfig.LoadedFactoryConfig,
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
	runtimeCfg *factoryconfig.LoadedFactoryConfig,
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
	runtimeCfg *factoryconfig.LoadedFactoryConfig,
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

func (h *CatalogHost) supervisedLeaseEndpoint(slotKey string, identity Identity, runtimeCfg *factoryconfig.LoadedFactoryConfig, modelName string) (string, error) {
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
	runtimeCfg *factoryconfig.LoadedFactoryConfig,
	modelName string,
) error {
	modelKey := canonicalModelKey(modelName)
	h.mu.Lock()
	if leases := h.byModel[modelKey]; len(leases) > 0 {
		h.mu.Unlock()
		return &ReadinessError{
			Snapshot: ReadinessSnapshot{
				Identity:       Identity{Name: strings.TrimSpace(modelName)},
				ReadinessState: factoryapi.ManagedRuntimeReadinessStateFAILED,
				LifecycleState: factoryapi.ManagedRuntimeLifecycleStateLOADED,
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
				ReadinessState: factoryapi.ManagedRuntimeReadinessStateLOADING,
				LifecycleState: factoryapi.ManagedRuntimeLifecycleStateLOADING,
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
	runtimeCfg *factoryconfig.LoadedFactoryConfig,
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
	runtimeCfg *factoryconfig.LoadedFactoryConfig,
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

func (h *CatalogHost) runtimeSlot(runtimeCfg *factoryconfig.LoadedFactoryConfig, modelName string) *supervisedRuntime {
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

func (h *CatalogHost) peekRuntimeSlot(runtimeCfg *factoryconfig.LoadedFactoryConfig, modelName string) *supervisedRuntime {
	key := h.runtimeSlotKey(runtimeCfg, modelName)
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.runtimeSlots[key]
}

func (h *CatalogHost) runtimeSlotKey(runtimeCfg *factoryconfig.LoadedFactoryConfig, modelName string) string {
	return runtimeIdentityKey(runtimeCfg) + "|" + canonicalModelKey(modelName)
}

func (h *CatalogHost) localWorkerForModel(
	runtimeCfg *factoryconfig.LoadedFactoryConfig,
	modelName string,
) (*interfaces.WorkerConfig, error) {
	if runtimeCfg == nil || runtimeCfg.FactoryConfig() == nil {
		return nil, fmt.Errorf("runtime config is not available")
	}
	target := canonicalModelKey(modelName)
	for _, worker := range runtimeCfg.FactoryConfig().Workers {
		if canonicalModelKey(worker.Model) != target {
			continue
		}
		if strings.TrimSpace(worker.ModelLocality) != interfaces.ModelLocalityLocal {
			continue
		}
		copied := worker
		return &copied, nil
	}
	return nil, fmt.Errorf("local model worker not found for %q", modelName)
}

func (h *CatalogHost) catalogEntry(runtimeCfg *factoryconfig.LoadedFactoryConfig, modelName string) (localmodels.CatalogEntry, error) {
	if runtimeCfg == nil {
		return localmodels.CatalogEntry{}, fmt.Errorf("runtime config is not available")
	}
	catalog := localmodels.BuildCatalog(runtimeCfg)
	key := localmodels.CanonicalModelName(modelName)
	if key == "" {
		return localmodels.CatalogEntry{}, fmt.Errorf("%w: empty model name", apisurface.ErrModelNotFound)
	}
	entry, ok := catalog[key]
	if !ok {
		return localmodels.CatalogEntry{}, fmt.Errorf("%w: %s", apisurface.ErrModelNotFound, modelName)
	}
	return entry, nil
}

func (h *CatalogHost) identityFromCatalog(
	runtimeCfg *factoryconfig.LoadedFactoryConfig,
	entry localmodels.CatalogEntry,
) Identity {
	identity := Identity{
		Name:                entry.Summary.Name,
		Locality:            entry.Summary.ProviderLocality,
		SupportedOperations: append([]factoryapi.ModelOperation(nil), entry.Summary.Operations...),
	}
	if resource := modelScopedResource(runtimeCfg, entry.Summary.Name); resource != nil {
		identity.Backend = strings.TrimSpace(resource.Backend)
		identity.LoadPolicy = strings.TrimSpace(resource.LoadPolicy)
		if h.opts.SourceResolver != nil {
			resolution := h.opts.SourceResolver.Resolve(
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

func modelScopedResource(runtimeCfg *factoryconfig.LoadedFactoryConfig, modelName string) *interfaces.ResourceConfig {
	if runtimeCfg == nil || runtimeCfg.FactoryConfig() == nil {
		return nil
	}
	factoryCfg := runtimeCfg.FactoryConfig()
	key := canonicalModelKey(modelName)
	for _, resource := range factoryCfg.Resources {
		if canonicalModelKey(resource.Model) != key {
			continue
		}
		if strings.TrimSpace(resource.Type) != interfaces.ResourceTypeModel {
			continue
		}
		copied := resource
		return &copied
	}
	return nil
}

func managedRuntimePullSnapshot(
	runtimeCfg *factoryconfig.LoadedFactoryConfig,
	entry localmodels.CatalogEntry,
	result apisurface.ModelPullResult,
) ReadinessSnapshot {
	identity := Identity{
		Name:     strings.TrimSpace(result.ModelName),
		Locality: entry.Summary.ProviderLocality,
	}
	if identity.Name == "" {
		identity.Name = entry.Summary.Name
	}
	if resource := modelScopedResource(runtimeCfg, identity.Name); resource != nil {
		identity.Backend = strings.TrimSpace(resource.Backend)
		identity.LoadPolicy = strings.TrimSpace(resource.LoadPolicy)
	}
	readiness := factoryapi.ManagedRuntimeReadinessState(strings.TrimSpace(result.ReadinessState))
	if readiness == "" {
		readiness = factoryapi.ManagedRuntimeReadinessStateFAILED
	}
	lifecycle := factoryapi.ManagedRuntimeLifecycleState(strings.TrimSpace(result.LifecycleState))
	if lifecycle == "" {
		lifecycle = factoryapi.ManagedRuntimeLifecycleStateNOTINSTALLED
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

func runtimeIdentityKey(runtimeCfg *factoryconfig.LoadedFactoryConfig) string {
	if runtimeCfg == nil {
		return ""
	}
	return runtimeCfg.FactoryDir()
}
