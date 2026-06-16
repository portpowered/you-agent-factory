package modelhost

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/localmodels"
)

// CatalogHost is the catalog-backed model host implementation for process-level wiring.
type CatalogHost struct {
	mu            sync.Mutex
	assets        AssetGateway
	opts          Options
	supervisor    SupervisorConfig
	leases        map[string]*trackedLease
	byModel       map[string]map[string]struct{}
	runtimeSlots  map[string]*supervisedRuntime
	seq           uint64
}

type trackedLease struct {
	lease     Lease
	modelKey  string
	runtimeID string
}

// NewCatalogHost constructs a process-wide model host backed by managed asset integration.
func NewCatalogHost(assets AssetGateway, opts Options) *CatalogHost {
	if assets == nil {
		return nil
	}
	if opts.SourceResolver == nil {
		opts.SourceResolver = DefaultManagedRuntimeSourceResolverAdapter()
	}
	return &CatalogHost{
		assets:       assets,
		opts:         opts,
		supervisor:   normalizeSupervisorConfig(opts.Supervisor),
		leases:       make(map[string]*trackedLease),
		byModel:      make(map[string]map[string]struct{}),
		runtimeSlots: make(map[string]*supervisedRuntime),
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
	inspection, err := h.assets.InspectRuntimeCache(ctx, runtimeCfg, modelName)
	if err != nil {
		return ReadinessSnapshot{}, err
	}
	snapshot := ClassifyReadiness(identity, inspection, false)
	return h.overlaySupervisedReadiness(snapshot, inspection, runtimeCfg, modelName), nil
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
		return PullSnapshot{
			ReadinessSnapshot: snapshot,
			PullOutcome:       factoryapi.ManagedRuntimePullOutcomeUNSUPPORTEDRUNTIME,
		}, &ReadinessError{Snapshot: snapshot, Cause: ErrUnsupportedRuntime}
	}
	pullOutcome, readiness, err := h.assets.PullModel(ctx, runtimeCfg, modelName)
	if err != nil {
		var pullErr *apisurface.ManagedRuntimePullError
		if errors.As(err, &pullErr) {
			readiness = managedRuntimePullSnapshot(runtimeCfg, entry, pullErr.Result)
		}
		if readiness.Identity.Name == "" {
			readiness = ClassifyReadiness(h.identityFromCatalog(runtimeCfg, entry), CacheInspection{}, false)
		}
		return PullSnapshot{
			ReadinessSnapshot: readiness,
			PullOutcome:       pullOutcome,
		}, err
	}
	if readiness.Identity.Name == "" {
		inspected, inspectErr := h.assets.InspectRuntimeCache(ctx, runtimeCfg, modelName)
		if inspectErr != nil {
			return PullSnapshot{}, inspectErr
		}
		readiness = ClassifyReadiness(h.identityFromCatalog(runtimeCfg, entry), inspected, false)
	}
	return PullSnapshot{
		ReadinessSnapshot: readiness,
		PullOutcome:       pullOutcome,
	}, nil
}

func (h *CatalogHost) AcquireLease(
	ctx context.Context,
	runtimeCfg *factoryconfig.LoadedFactoryConfig,
	modelName string,
	opts LeaseOptions,
) (Lease, error) {
	if err := ctx.Err(); err != nil {
		return Lease{}, cancelError(err)
	}
	entry, err := h.catalogEntry(runtimeCfg, modelName)
	if err != nil {
		return Lease{}, err
	}
	identity := h.identityFromCatalog(runtimeCfg, entry)
	inspection, err := h.assets.InspectRuntimeCache(ctx, runtimeCfg, modelName)
	if err != nil {
		return Lease{}, err
	}
	snapshot := h.overlaySupervisedReadiness(ClassifyReadiness(identity, inspection, false), inspection, runtimeCfg, modelName)
	if requiresSupervisedBackend(snapshot.Identity) && inspection.Installed {
		worker, workerErr := h.localWorkerForModel(runtimeCfg, modelName)
		if workerErr != nil {
			return Lease{}, workerErr
		}
		spec, specErr := h.supervisor.ServerStartBuilder(snapshot.Identity, inspection, worker)
		if specErr != nil {
			return Lease{}, specErr
		}
		slot := h.runtimeSlot(runtimeCfg, modelName)
		if err := slot.ensureReady(ctx, spec); err != nil {
			return Lease{}, err
		}
		snapshot = slot.readinessOverlay(snapshot.Identity, snapshot)
	}
	if snapshot.ReadinessState != factoryapi.ManagedRuntimeReadinessStateREADY {
		cause := readinessCause(snapshot)
		return Lease{}, &ReadinessError{Snapshot: snapshot, Cause: cause}
	}
	endpoint := ""
	if requiresSupervisedBackend(snapshot.Identity) {
		endpoint = h.runtimeSlot(runtimeCfg, modelName).endpointValue()
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.seq++
	leaseID := fmt.Sprintf("model-lease-%d", h.seq)
	modelKey := canonicalModelKey(modelName)
	lease := Lease{
		ID:       leaseID,
		Identity: snapshot.Identity,
		Endpoint: endpoint,
		Holder:   strings.TrimSpace(opts.Holder),
	}
	h.leases[leaseID] = &trackedLease{
		lease:     lease,
		modelKey:  modelKey,
		runtimeID: runtimeIdentityKey(runtimeCfg),
	}
	if h.byModel[modelKey] == nil {
		h.byModel[modelKey] = make(map[string]struct{})
	}
	h.byModel[modelKey][leaseID] = struct{}{}
	return lease, nil
}

func (h *CatalogHost) ReleaseLease(_ context.Context, leaseID string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	tracked, ok := h.leases[leaseID]
	if !ok {
		return ErrLeaseNotFound
	}
	delete(h.leases, leaseID)
	if leases, ok := h.byModel[tracked.modelKey]; ok {
		delete(leases, leaseID)
		if len(leases) == 0 {
			delete(h.byModel, tracked.modelKey)
		}
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
	delete(h.runtimeSlots, slotKey)
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
	slot := h.runtimeSlot(runtimeCfg, modelName)
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
