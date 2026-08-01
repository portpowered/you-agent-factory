package modelhost

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	localmodels "github.com/portpowered/infinite-you/pkg/services/models/internal/local"
	managedruntime "github.com/portpowered/infinite-you/pkg/services/models/internal/managedruntime"
	runtimehost "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_host"
)

// ScopedCompatHost implements scoped runtime pull, readiness, and lease behavior
// by delegating supervision to the packaged Runtime Host subservice instead of
// constructing a per-scope CatalogHost supervisor.
type ScopedCompatHost struct {
	scope          models.RuntimeScopeRef
	runtimeHost    runtimehost.Service
	assetGateway   AssetGateway
	sourceResolver SourceResolver
	diagnostics    Diagnostics

	mu      sync.Mutex
	leases  map[string]*scopedTrackedLease
	byModel map[string]map[string]struct{}
	seq     uint64
}

type scopedTrackedLease struct {
	lease      Lease
	modelKey   string
	runtimeCfg *models.RuntimeConfig
	modelName  string
}

// NewScopedCompatHost constructs a scoped runtime host that routes supervise,
// health, reuse, and unload through the packaged Runtime Host subservice.
func NewScopedCompatHost(
	scope models.RuntimeScopeRef,
	runtimeHost runtimehost.Service,
	assetGateway AssetGateway,
	sourceResolver SourceResolver,
	diagnostics Diagnostics,
) (*ScopedCompatHost, error) {
	if scope.IsZero() {
		return nil, missingDependencyError("runtime scope")
	}
	if isNilDependency(runtimeHost) {
		return nil, missingDependencyError("packaged runtime host")
	}
	if isNilDependency(assetGateway) {
		return nil, missingDependencyError("asset gateway")
	}
	if sourceResolver == nil {
		return nil, missingDependencyError("source resolver")
	}
	return &ScopedCompatHost{
		scope:          scope,
		runtimeHost:    runtimeHost,
		assetGateway:   assetGateway,
		sourceResolver: sourceResolver,
		diagnostics:    diagnostics,
		leases:         make(map[string]*scopedTrackedLease),
		byModel:        make(map[string]map[string]struct{}),
	}, nil
}

func (h *ScopedCompatHost) ResolveIdentity(
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

func (h *ScopedCompatHost) InspectReadiness(
	ctx context.Context,
	runtimeCfg *models.RuntimeConfig,
	modelName string,
) (ReadinessSnapshot, error) {
	return h.inspectReadiness(ctx, runtimeCfg, modelName, true)
}

func (h *ScopedCompatHost) Pull(
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
	pullResult, err := h.assetGateway.PullModel(ctx, runtimeCfg, modelName)
	if err != nil {
		readiness := pullResult.Snapshot
		var pullErr *models.PullError
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
		inspected, inspectErr := h.assetGateway.InspectRuntimeCache(ctx, runtimeCfg, modelName)
		if inspectErr != nil {
			return PullSnapshot{}, inspectErr
		}
		readiness = ClassifyReadiness(h.identityFromCatalog(runtimeCfg, entry), inspected, false)
	}
	pullSnapshot := pullSnapshotFromAssetResult(pullResult, readiness)
	return pullSnapshot, nil
}

func (h *ScopedCompatHost) AcquireLease(
	ctx context.Context,
	runtimeCfg *models.RuntimeConfig,
	modelName string,
	opts LeaseOptions,
) (Lease, error) {
	snapshot, inspection, modelKey, leaseCapacity, err := h.prepareAcquireLease(ctx, runtimeCfg, modelName)
	if err != nil {
		return Lease{}, err
	}
	if err := h.ensureSupervisedReadyForLease(ctx, modelName, inspection, &snapshot); err != nil {
		return Lease{}, err
	}
	if snapshot.ReadinessState != managedruntime.ReadinessStateReady {
		cause := readinessCause(snapshot)
		return Lease{}, &ReadinessError{Snapshot: snapshot, Cause: cause}
	}
	return h.issueLease(runtimeCfg, modelName, modelKey, leaseCapacity, snapshot, opts)
}

func (h *ScopedCompatHost) ReleaseLease(_ context.Context, leaseID string) error {
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
	h.mu.Unlock()

	h.diagnostics.logLeaseReleased(identity, leaseID)
	return nil
}

func (h *ScopedCompatHost) Unload(
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
	h.mu.Unlock()

	_, err := h.runtimeHost.StopModelHost(ctx, models.StopModelHostRequest{
		Scope: h.scope,
		Name:  modelName,
	})
	return err
}

func (h *ScopedCompatHost) inspectReadiness(
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
	inspection, err := h.assetGateway.InspectRuntimeCache(ctx, runtimeCfg, modelName)
	if err != nil {
		return ReadinessSnapshot{}, err
	}
	snapshot := ClassifyReadiness(identity, inspection, false)
	if overlaySupervised {
		inspected, inspectErr := h.runtimeHost.InspectModelHost(ctx, models.InspectModelHostRequest{
			Scope: h.scope,
			Name:  modelName,
		})
		if inspectErr != nil {
			return ReadinessSnapshot{}, inspectErr
		}
		snapshot = readinessFromModelHostSnapshot(inspected.Host, identity)
	}
	return snapshot, nil
}

func (h *ScopedCompatHost) prepareAcquireLease(
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
	inspection, err := h.assetGateway.InspectRuntimeCache(ctx, runtimeCfg, modelName)
	if err != nil {
		return ReadinessSnapshot{}, CacheInspection{}, "", 0, err
	}
	snapshot, err := h.inspectReadiness(ctx, runtimeCfg, modelName, true)
	if err != nil {
		return ReadinessSnapshot{}, CacheInspection{}, "", 0, err
	}
	if snapshot.Identity.Name == "" {
		snapshot.Identity = identity
	}
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

func (h *ScopedCompatHost) ensureSupervisedReadyForLease(
	ctx context.Context,
	modelName string,
	inspection CacheInspection,
	snapshot *ReadinessSnapshot,
) error {
	if !requiresSupervisedBackend(snapshot.Identity) || !inspection.Installed {
		return nil
	}
	ensureResult, err := h.runtimeHost.EnsureModelHost(ctx, models.EnsureModelHostRequest{
		Scope: h.scope,
		Name:  modelName,
	})
	if err != nil {
		return err
	}
	updated := readinessFromModelHostSnapshot(ensureResult.Host, snapshot.Identity)
	*snapshot = updated
	return nil
}

func (h *ScopedCompatHost) issueLease(
	runtimeCfg *models.RuntimeConfig,
	modelName string,
	modelKey string,
	leaseCapacity int,
	snapshot ReadinessSnapshot,
	opts LeaseOptions,
) (Lease, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.leaseCapacityExhausted(modelKey, leaseCapacity) {
		h.diagnostics.logLeaseExhausted(snapshot.Identity)
		return Lease{}, leaseCapacityError(modelName)
	}
	endpoint, err := h.supervisedLeaseEndpoint(runtimeCfg, modelName, snapshot.Identity, snapshot.Diagnostics)
	if err != nil {
		return Lease{}, err
	}
	h.seq++
	leaseID := fmt.Sprintf("model-lease-%d", h.seq)
	lease := Lease{
		ID:       leaseID,
		Identity: snapshot.Identity,
		Endpoint: endpoint,
		Holder:   strings.TrimSpace(opts.Holder),
	}
	h.leases[leaseID] = &scopedTrackedLease{
		lease:      lease,
		modelKey:   modelKey,
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

func (h *ScopedCompatHost) leaseCapacityForModel(runtimeCfg *models.RuntimeConfig, modelName string) int {
	resource := modelScopedResource(runtimeCfg, modelName)
	if resource == nil || resource.Capacity <= 0 {
		return 0
	}
	return resource.Capacity
}

func (h *ScopedCompatHost) leaseCapacityExhausted(modelKey string, capacity int) bool {
	if capacity <= 0 {
		return false
	}
	return len(h.byModel[modelKey]) >= capacity
}

func (h *ScopedCompatHost) catalogEntry(runtimeCfg *models.RuntimeConfig, modelName string) (localmodels.CatalogEntry, error) {
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

func (h *ScopedCompatHost) identityFromCatalog(
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

func (h *ScopedCompatHost) supervisedLeaseEndpoint(
	runtimeCfg *models.RuntimeConfig,
	modelName string,
	identity Identity,
	diagnostics map[string]string,
) (string, error) {
	if !requiresSupervisedBackend(identity) {
		return "", nil
	}
	worker, err := localWorkerForModel(runtimeCfg, modelName)
	if err != nil {
		return "", err
	}
	if !workerDeclaresSupervisedHealthEndpoint(worker) {
		return "", nil
	}
	endpoint := strings.TrimSpace(diagnostics["endpoint"])
	if endpoint == "" {
		return "", ErrRuntimeNotReady
	}
	return endpoint, nil
}

func localWorkerForModel(
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

func readinessFromModelHostSnapshot(host models.ModelHostSnapshot, identity Identity) ReadinessSnapshot {
	readiness := managedruntime.ReadinessState(string(host.ReadinessState))
	lifecycle := managedruntime.LifecycleState(string(host.LifecycleState))
	snapshot := ReadinessSnapshot{
		Identity:       identity,
		ReadinessState: readiness,
		LifecycleState: lifecycle,
		FailureClass:   FailureClassForReadinessState(readiness),
		Diagnostics:    managedDiagnostics(identity, readiness, lifecycle),
	}
	if len(host.Diagnostics) > 0 {
		snapshot.Diagnostics = mergeDiagnostics(identity, readiness, lifecycle, host.Diagnostics)
	}
	return snapshot
}
