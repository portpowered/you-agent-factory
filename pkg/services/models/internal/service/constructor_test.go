package service_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
	modelhost "github.com/portpowered/infinite-you/pkg/services/models/internal/legacyhost"
	localmodels "github.com/portpowered/infinite-you/pkg/services/models/internal/local"
	managedruntime "github.com/portpowered/infinite-you/pkg/services/models/internal/managedruntime"
	modelsservice "github.com/portpowered/infinite-you/pkg/services/models/internal/service"
	scopedassets "github.com/portpowered/infinite-you/pkg/services/models/internal/services/assets"
	modelcatalog "github.com/portpowered/infinite-you/pkg/services/models/internal/services/catalog"
	inference "github.com/portpowered/infinite-you/pkg/services/models/internal/services/inference"
	runtimehost "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_host"
	runtimescopes "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_scopes"

	"go.uber.org/zap"
)

type modelServiceFixture struct {
	RuntimeConfig    func() *modelRuntimeConfig
	ModelAssetPuller localmodels.AssetPuller
	Logger           *zap.Logger
	Clock            func() time.Time
	ModelPullMetrics modelseffects.PullMetricsRecorder
}

type modelRuntimeConfig = models.RuntimeConfig
type modelRuntimeWorker = models.RuntimeWorker
type modelRuntimeResource = models.RuntimeResource

type testFactoryConfig struct {
	Name             string
	Workers          []modelRuntimeWorker
	Resources        []modelRuntimeResource
	ResourceManifest *testResourceManifest
}

type testResourceManifest struct{ RequiredTools []testRequiredTool }
type testRequiredTool struct{ Name, Command string }

func projectTestModelsRuntimeConfig(factoryDir string, cfg *testFactoryConfig) *modelRuntimeConfig {
	if cfg == nil {
		return nil
	}
	return &modelRuntimeConfig{
		FactoryDirectory: factoryDir,
		Workers:          append([]modelRuntimeWorker(nil), cfg.Workers...),
		Resources:        append([]modelRuntimeResource(nil), cfg.Resources...),
	}
}

// Root forwards scoped lease requests and preserves the selected host's outcomes.
// Validation belongs to Runtime Host; this fixture observes that boundary.
func TestRootScopedLeaseDelegationPreservesRequestsAndOutcomes(t *testing.T) {
	t.Parallel()
	scope, err := (models.RuntimeScopeRef{}).Parse("lease-scope")
	if err != nil {
		t.Fatal(err)
	}
	lease, err := (models.ModelLeaseRef{}).Parse("selected-lease")
	if err != nil {
		t.Fatal(err)
	}
	acquire := models.AcquireModelLeaseRequest{Scope: scope, Name: "voice", Holder: "dispatch-1"}
	release := models.ReleaseModelLeaseRequest{Scope: scope, Lease: lease}
	failure := errors.New("selected host failure")
	for _, test := range []struct {
		name    string
		acquire models.AcquireModelLeaseRequest
		release models.ReleaseModelLeaseRequest
		failure error
	}{
		{"success", acquire, release, nil},
		{"empty model", models.AcquireModelLeaseRequest{Scope: scope, Holder: acquire.Holder}, release, nil},
		{"empty lease", acquire, models.ReleaseModelLeaseRequest{Scope: scope}, nil},
		{"host unavailable", acquire, release, models.ErrHostRuntimeNotReady},
		{"host fault", acquire, release, failure},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			host := &rootLeaseBoundary{failure: test.failure, lease: models.ModelLease{Lease: lease, Scope: scope, ModelName: acquire.Name, Holder: acquire.Holder, Status: models.ModelLeaseStatusActive, HostReadiness: models.ReadinessStateReady}}
			root := newLeaseBoundaryRoot(t, host)
			type contextKey struct{}
			ctx := context.WithValue(t.Context(), contextKey{}, "selected")
			assertRootLeaseAcquisition(t, root, host, ctx, test.acquire)
			assertRootLeaseRelease(t, root, host, ctx, test.release)
		})
	}
}

func assertRootLeaseAcquisition(t *testing.T, root *modelsservice.Root, host *rootLeaseBoundary, ctx context.Context, request models.AcquireModelLeaseRequest) {
	t.Helper()
	acquired, gotErr := root.AcquireModelLease(ctx, request)
	wantErr := host.failure
	if request.Name == "" {
		wantErr = models.ErrNotFound
	}
	if !errors.Is(gotErr, wantErr) || host.acquired != request || host.acquireContext != ctx {
		t.Fatalf("acquire = %#v, %v; request = %#v", acquired, gotErr, host.acquired)
	}
	if wantErr == nil && acquired.Lease != host.lease {
		t.Fatalf("acquired lease = %#v", acquired.Lease)
	}
}
func assertRootLeaseRelease(t *testing.T, root *modelsservice.Root, host *rootLeaseBoundary, ctx context.Context, request models.ReleaseModelLeaseRequest) {
	t.Helper()
	released, gotErr := root.ReleaseModelLease(ctx, request)
	wantErr := host.failure
	if request.Lease.IsZero() {
		wantErr = models.ErrHostLeaseNotFound
	}
	if !errors.Is(gotErr, wantErr) || host.released != request || host.releaseContext != ctx {
		t.Fatalf("release = %#v, %v; request = %#v", released, gotErr, host.released)
	}
	if wantErr == nil && (released.Lease != host.lease || released.Outcome != models.ModelLeaseReleased) {
		t.Fatalf("released lease = %#v", released)
	}
}

type rootLeaseBoundary struct {
	runtimehost.Service
	failure                        error
	lease                          models.ModelLease
	acquired                       models.AcquireModelLeaseRequest
	released                       models.ReleaseModelLeaseRequest
	acquireContext, releaseContext context.Context
}

func (h *rootLeaseBoundary) AcquireModelLease(ctx context.Context, r models.AcquireModelLeaseRequest) (models.AcquireModelLeaseResult, error) {
	h.acquired, h.acquireContext = r, ctx
	if err := r.Validate(); err != nil {
		return models.AcquireModelLeaseResult{}, err
	}
	return models.AcquireModelLeaseResult{Lease: h.lease}, h.failure
}
func (h *rootLeaseBoundary) ReleaseModelLease(ctx context.Context, r models.ReleaseModelLeaseRequest) (models.ReleaseModelLeaseResult, error) {
	h.released, h.releaseContext = r, ctx
	if err := r.Validate(); err != nil {
		return models.ReleaseModelLeaseResult{}, err
	}
	return models.ReleaseModelLeaseResult{Lease: h.lease, Outcome: models.ModelLeaseReleased}, h.failure
}
func newLeaseBoundaryRoot(t *testing.T, host runtimehost.Service) *modelsservice.Root {
	t.Helper()
	resources, err := localmodels.NewResourceLimiter(modelseffects.LocalRuntimeHooks{}, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	root, err := modelsservice.NewRoot(resources,
		func(context.Context, models.PullModelRequest) (models.PullResult, error) {
			return models.PullResult{}, nil
		},
		func(models.RuntimeScopeRef) {}, func() {},
		struct{ runtimescopes.Service }{}, struct{ modelcatalog.Service }{}, struct{ scopedassets.Service }{}, host, struct{ inference.Service }{},
		zap.NewNop(), time.Now, nil, nil,
		func(context.Context, string) (string, error) { return "", models.ErrModelRevisionUnresolved }, nil, models.AssetHostPlatform{})
	if err != nil {
		t.Fatal(err)
	}
	return root
}

type hostLeaseTestHost struct{}

func (hostLeaseTestHost) ResolveIdentity(_ context.Context, _ *modelRuntimeConfig, modelName string) (modelhost.Identity, error) {
	return modelhost.Identity{Name: modelName, Locality: managedruntime.LocalityLocal}, nil
}

func (hostLeaseTestHost) InspectReadiness(_ context.Context, _ *modelRuntimeConfig, modelName string) (modelhost.ReadinessSnapshot, error) {
	return modelhost.ReadinessSnapshot{
		Identity:       modelhost.Identity{Name: modelName, Locality: managedruntime.LocalityLocal},
		ReadinessState: managedruntime.ReadinessStateReady,
		LifecycleState: managedruntime.LifecycleStateInstalled,
	}, nil
}

func (hostLeaseTestHost) Pull(context.Context, *modelRuntimeConfig, string) (modelhost.PullSnapshot, error) {
	return modelhost.PullSnapshot{}, errors.New("pull unavailable in test host")
}

func (hostLeaseTestHost) AcquireLease(_ context.Context, _ *modelRuntimeConfig, modelName string, opts modelhost.LeaseOptions) (modelhost.Lease, error) {
	return modelhost.Lease{
		ID:       "lease-" + modelName,
		Holder:   opts.Holder,
		Endpoint: "http://127.0.0.1:8080",
		Identity: modelhost.Identity{Name: modelName, Locality: managedruntime.LocalityLocal},
	}, nil
}

func (hostLeaseTestHost) ReleaseLease(context.Context, string) error {
	return nil
}

func (hostLeaseTestHost) Unload(context.Context, *modelRuntimeConfig, string) error {
	return errors.New("unload unavailable in test host")
}

func openScopedLocalRoot(t *testing.T, root *modelsservice.Root, cache, endpoint string) models.RuntimeScopeRef {
	t.Helper()
	opened, err := root.OpenRuntimeScope(t.Context(), models.OpenRuntimeScopeRequest{Config: models.RuntimeScopeConfig{
		CacheDirectory: cache,
		Runtime: models.RuntimeConfig{
			BaseDirectory: cache, FactoryDirectory: endpoint,
			Resources: []models.RuntimeResource{{Name: "voice", Type: models.RuntimeResourceTypeModel,
				Model: "voice", Backend: "LLAMACPP", LoadPolicy: "ON_DEMAND", Capacity: 1},
				{Name: cache, Type: models.RuntimeResourceTypeModel, Model: cache, Backend: "LLAMACPP", LoadPolicy: "ON_DEMAND"}},
			Workers: []models.RuntimeWorker{{Name: "voice", Model: "voice", ModelLocality: models.RuntimeModelLocalityLocal,
				Args: []string{"--health-endpoint", endpoint}, Resources: []models.RuntimeResource{{Name: "voice", Capacity: 1}}},
				{Name: cache, Type: models.RuntimeWorkerTypeModel, Model: cache, ModelLocality: models.RuntimeModelLocalityLocal, Resources: []models.RuntimeResource{{Name: cache}}}},
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	return opened.Scope
}

func assertScopedLocalRootPull(t *testing.T, root *modelsservice.Root, ctx context.Context, scope models.RuntimeScopeRef, host *scopedLocalRootHost) {
	t.Helper()
	binding, err := host.scopes.Resolve(runtimescopes.Reference(scope.String()))
	if err != nil {
		t.Fatal(err)
	}
	result, err := root.PullModelForScope(ctx, models.PullModelRequest{Scope: scope, Name: binding.CacheDirectory})
	if err != nil || result.ModelName != binding.CacheDirectory || result.CachePath != binding.CacheDirectory || result.ReadinessState != "READY" {
		t.Fatalf("scope %s pull=%#v,%v", scope, result, err)
	}
}

type scopedLocalRootAssets struct {
	scopedassets.Service
	scopes          runtimescopes.Service
	started, resume chan struct{}
}

func (a scopedLocalRootAssets) PrepareModelAssets(_ context.Context, request models.PrepareModelAssetsRequest) (models.PrepareModelAssetsResult, error) {
	if _, err := a.scopes.Resolve(runtimescopes.Reference(request.Scope.String())); err != nil {
		return models.PrepareModelAssetsResult{}, err
	}
	return models.PrepareModelAssetsResult{Outcome: models.AssetPreparationPrepared, Asset: models.AssetSnapshot{
		ModelName: request.Name, Readiness: models.AssetReadinessAvailable,
	}}, nil
}

func (a scopedLocalRootAssets) InspectRuntimeCache(_ context.Context, request models.InspectModelAssetsRequest) (scopedassets.RuntimeCacheInspection, error) {
	binding, err := a.scopes.Resolve(runtimescopes.Reference(request.Scope.String()))
	if err != nil {
		return scopedassets.RuntimeCacheInspection{}, err
	}
	return scopedassets.RuntimeCacheInspection{Supported: true, Installed: true, CachePath: binding.CacheDirectory, Revision: "immutable"}, nil
}

func (a scopedLocalRootAssets) ResolveRuntimeCache(ctx context.Context, request models.InspectModelAssetsRequest) (scopedassets.RuntimeCacheLayout, error) {
	binding, err := a.scopes.Resolve(runtimescopes.Reference(request.Scope.String()))
	if err != nil {
		return scopedassets.RuntimeCacheLayout{}, err
	}
	if binding.CacheDirectory == "gated-cache" {
		close(a.started)
		select {
		case <-a.resume:
		case <-ctx.Done():
			return scopedassets.RuntimeCacheLayout{}, ctx.Err()
		}
	}
	return scopedassets.RuntimeCacheLayout{ModelName: request.Name, CachePath: binding.CacheDirectory, Revision: "immutable"}, nil
}

type scopedLocalRootHost struct {
	runtimehost.Service
	scopes   runtimescopes.Service
	failure  error
	mu       sync.Mutex
	sequence int
	active   map[string]models.RuntimeScopeRef
	releases map[string]int
}

func (h *scopedLocalRootHost) EnsureModelHost(_ context.Context, r models.EnsureModelHostRequest) (models.EnsureModelHostResult, error) {
	binding, err := h.scopes.Resolve(runtimescopes.Reference(r.Scope.String()))
	if err != nil {
		return models.EnsureModelHostResult{}, err
	}
	return models.EnsureModelHostResult{Host: models.ModelHostSnapshot{Scope: r.Scope, ModelName: r.Name, ReadinessState: models.ReadinessStateReady, Diagnostics: map[string]string{"endpoint": binding.RuntimeConfig().FactoryDirectory}}}, nil
}
func (h *scopedLocalRootHost) AcquireModelLease(_ context.Context, r models.AcquireModelLeaseRequest) (models.AcquireModelLeaseResult, error) {
	if h.failure != nil {
		return models.AcquireModelLeaseResult{}, h.failure
	}
	if err := r.Validate(); err != nil {
		return models.AcquireModelLeaseResult{}, err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.sequence++
	ref, err := (models.ModelLeaseRef{}).Parse(fmt.Sprintf("local-root-lease:%d", h.sequence))
	if err != nil {
		return models.AcquireModelLeaseResult{}, err
	}
	h.active[ref.String()] = r.Scope
	return models.AcquireModelLeaseResult{Lease: models.ModelLease{Lease: ref, Scope: r.Scope, Holder: r.Holder, ModelName: r.Name, Status: models.ModelLeaseStatusActive}}, nil
}
func (h *scopedLocalRootHost) ReleaseModelLease(ctx context.Context, r models.ReleaseModelLeaseRequest) (models.ReleaseModelLeaseResult, error) {
	if err := ctx.Err(); err != nil {
		return models.ReleaseModelLeaseResult{}, err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.active[r.Lease.String()] != r.Scope {
		return models.ReleaseModelLeaseResult{}, errors.New("lease scope mismatch")
	}
	delete(h.active, r.Lease.String())
	h.releases[r.Lease.String()]++
	return models.ReleaseModelLeaseResult{Outcome: models.ModelLeaseReleased}, nil
}
func (*scopedLocalRootHost) Shutdown(context.Context) error { return nil }

type scopedLocalRootHandle string

func assertIndividualExecutionCleanup(t *testing.T, root *modelsservice.Root, ctx context.Context, scope models.RuntimeScopeRef, closed *models.RuntimeScopeRef, stopped *bool) {
	t.Helper()
	closeResult, err := root.CloseRuntimeScope(ctx, models.CloseRuntimeScopeRequest{Scope: scope})
	if err != nil || !closeResult.Closed || *closed != scope || *stopped {
		t.Fatalf("scope close=%#v, %v, scope=%s, stopped=%t", closeResult, err, closed, *stopped)
	}
	if err := root.Close(ctx); err != nil {
		t.Fatal(err)
	}
	if !*stopped {
		t.Fatal("process close did not retire execution")
	}
}
