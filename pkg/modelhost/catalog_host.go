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
	mu      sync.Mutex
	assets  AssetGateway
	opts    Options
	leases  map[string]*trackedLease
	byModel map[string]map[string]struct{}
	seq     uint64
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
		assets:  assets,
		opts:    opts,
		leases:  make(map[string]*trackedLease),
		byModel: make(map[string]map[string]struct{}),
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
	return ClassifyReadiness(identity, inspection, false), nil
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
	snapshot, err := h.InspectReadiness(ctx, runtimeCfg, modelName)
	if err != nil {
		return Lease{}, err
	}
	if snapshot.ReadinessState != factoryapi.ManagedRuntimeReadinessStateREADY {
		cause := readinessCause(snapshot)
		return Lease{}, &ReadinessError{Snapshot: snapshot, Cause: cause}
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.seq++
	leaseID := fmt.Sprintf("model-lease-%d", h.seq)
	modelKey := canonicalModelKey(modelName)
	lease := Lease{
		ID:       leaseID,
		Identity: snapshot.Identity,
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
	_ context.Context,
	runtimeCfg *factoryconfig.LoadedFactoryConfig,
	modelName string,
) error {
	modelKey := canonicalModelKey(modelName)
	h.mu.Lock()
	defer h.mu.Unlock()
	if leases := h.byModel[modelKey]; len(leases) > 0 {
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
	_ = runtimeCfg
	return nil
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
