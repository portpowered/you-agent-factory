package service_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
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
	runtimescopeswire "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_scopes/wire"
	"go.uber.org/zap"
)

type modelServiceFixture struct {
	RuntimeConfig    func() *modelRuntimeConfig
	ModelHost        modelhost.Host
	ModelAssetPuller localmodels.AssetPuller
	Logger           *zap.Logger
	Clock            func() time.Time
	ModelPullMetrics modelseffects.PullMetricsRecorder
}

func mustConstructModelService(t *testing.T, fixture modelServiceFixture) *modelsservice.Service {
	t.Helper()
	if fixture.ModelAssetPuller == nil {
		fixture.ModelAssetPuller = externalConstructionAssetPuller{}
	}
	if fixture.ModelHost == nil {
		fixture.ModelHost = externalConstructionHost{puller: fixture.ModelAssetPuller}
	}
	if fixture.Logger == nil {
		fixture.Logger = zap.NewNop()
	}
	if fixture.Clock == nil {
		fixture.Clock = time.Now
	}
	svc, err := modelsservice.NewService(
		fixture.RuntimeConfig,
		fixture.ModelHost,
		fixture.ModelAssetPuller,
		fixture.Logger,
		fixture.Clock,
		fixture.ModelPullMetrics,
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return svc
}

type externalConstructionAssetPuller struct{}

func (externalConstructionAssetPuller) PullModel(context.Context, *modelRuntimeConfig, string) (models.PullResult, error) {
	return models.PullResult{}, nil
}
func (externalConstructionAssetPuller) EnsureModelAvailable(context.Context, *modelRuntimeConfig, *modelRuntimeWorker) error {
	return nil
}
func (externalConstructionAssetPuller) ResolveModelCache(context.Context, *modelRuntimeConfig, *modelRuntimeWorker) (localmodels.CacheLayout, error) {
	return localmodels.CacheLayout{}, nil
}
func (externalConstructionAssetPuller) InspectRuntimeCache(context.Context, *modelRuntimeConfig, string) (localmodels.RuntimeCacheInspection, error) {
	return localmodels.RuntimeCacheInspection{}, nil
}

type externalConstructionHost struct {
	puller localmodels.AssetPuller
}

func (externalConstructionHost) ResolveIdentity(context.Context, *modelRuntimeConfig, string) (modelhost.Identity, error) {
	return modelhost.Identity{}, nil
}
func (externalConstructionHost) InspectReadiness(context.Context, *modelRuntimeConfig, string) (modelhost.ReadinessSnapshot, error) {
	return modelhost.ReadinessSnapshot{}, nil
}
func (h externalConstructionHost) Pull(ctx context.Context, runtimeCfg *modelRuntimeConfig, modelName string) (modelhost.PullSnapshot, error) {
	result, err := localmodels.PullModelWithOptions(h.puller, ctx, runtimeCfg, modelName, localmodels.PullOptions{
		RuntimeCacheInspector: h.puller,
		SourceResolver:        localmodels.DefaultManagedRuntimeSourceResolver(),
	})
	files := make([]modelhost.PullDownloadedFile, 0, len(result.DownloadedFiles))
	for _, file := range result.DownloadedFiles {
		files = append(files, modelhost.PullDownloadedFile{Path: file.Path, Bytes: file.Bytes, SHA256: file.SHA256})
	}
	return modelhost.PullSnapshot{
		ReadinessSnapshot: modelhost.ReadinessSnapshot{
			Identity: modelhost.Identity{
				Name:          result.ModelName,
				Locality:      managedruntime.Locality(result.ProviderLocality),
				SourceKind:    result.SourceKind,
				SourceID:      result.SourceID,
				ResolverNotes: result.ResolverNotes,
			},
			ReadinessState: managedruntime.ReadinessState(result.ReadinessState),
			LifecycleState: managedruntime.LifecycleState(result.LifecycleState),
		},
		PullOutcome:     managedruntime.PullOutcome(result.ManagedPullOutcome),
		LegacyOutcome:   result.Outcome,
		CachePath:       result.CachePath,
		Revision:        result.Revision,
		DownloadedFiles: files,
	}, err
}
func (externalConstructionHost) AcquireLease(context.Context, *modelRuntimeConfig, string, modelhost.LeaseOptions) (modelhost.Lease, error) {
	return modelhost.Lease{}, nil
}
func (externalConstructionHost) ReleaseLease(context.Context, string) error { return nil }
func (externalConstructionHost) Unload(context.Context, *modelRuntimeConfig, string) error {
	return nil
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

func TestService_AcquireLease_RejectsEmptyModelName(t *testing.T) {
	t.Parallel()

	runtimeCfg := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	svc := mustConstructModelService(t, modelServiceFixture{
		RuntimeConfig: func() *modelRuntimeConfig { return runtimeCfg },
		ModelHost:     hostLeaseTestHost{},
	})

	_, err := svc.AcquireLease(context.Background(), models.AcquireLeaseRequest{})
	if !errors.Is(err, models.ErrNotFound) {
		t.Fatalf("AcquireLease empty model = %v, want ErrNotFound", err)
	}
}

func TestService_ReleaseLease_RejectsEmptyLeaseID(t *testing.T) {
	t.Parallel()

	runtimeCfg := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	svc := mustConstructModelService(t, modelServiceFixture{
		RuntimeConfig: func() *modelRuntimeConfig { return runtimeCfg },
		ModelHost:     hostLeaseTestHost{},
	})

	err := svc.ReleaseLease(context.Background(), models.ReleaseLeaseRequest{})
	if !errors.Is(err, models.ErrHostLeaseNotFound) {
		t.Fatalf("ReleaseLease empty lease id = %v, want ErrHostLeaseNotFound", err)
	}
}

func TestService_AcquireLease_ReturnsUnavailableWhenRuntimeMissing(t *testing.T) {
	t.Parallel()

	svc := mustConstructModelService(t, modelServiceFixture{
		RuntimeConfig: func() *modelRuntimeConfig { return nil },
		ModelHost:     hostLeaseTestHost{},
	})

	_, err := svc.AcquireLease(context.Background(), models.AcquireLeaseRequest{ModelName: "OMNIVOICE_Q4_K_M"})
	if err == nil || !strings.Contains(err.Error(), "runtime is not available") {
		t.Fatalf("AcquireLease missing runtime = %v, want runtime unavailable", err)
	}
}

func TestService_AcquireLeaseAndReleaseLease_HappyPathThroughStubHost(t *testing.T) {
	t.Parallel()

	runtimeCfg := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	host := hostLeaseTestHost{}
	svc := mustConstructModelService(t, modelServiceFixture{
		RuntimeConfig: func() *modelRuntimeConfig { return runtimeCfg },
		ModelHost:     host,
	})

	lease, err := svc.AcquireLease(context.Background(), models.AcquireLeaseRequest{
		ModelName: "OMNIVOICE_Q4_K_M",
		Holder:    "dispatch-1",
	})
	if err != nil {
		t.Fatalf("AcquireLease: %v", err)
	}
	if lease.ID != "lease-OMNIVOICE_Q4_K_M" || lease.Holder != "dispatch-1" || lease.Endpoint != "http://127.0.0.1:8080" {
		t.Fatalf("AcquireLease = %#v, want stub host lease", lease)
	}

	if err := svc.ReleaseLease(context.Background(), models.ReleaseLeaseRequest{LeaseID: lease.ID}); err != nil {
		t.Fatalf("ReleaseLease: %v", err)
	}
}

func TestService_GetModel_RejectsEmptyModelName(t *testing.T) {
	t.Parallel()

	runtimeCfg := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	svc := mustConstructModelService(t, modelServiceFixture{
		RuntimeConfig: func() *modelRuntimeConfig { return runtimeCfg },
	})

	_, err := svc.GetModel(context.Background(), " ")
	if !errors.Is(err, models.ErrNotFound) {
		t.Fatalf("GetModel empty name = %v, want ErrNotFound", err)
	}
}

func TestService_PullModel_RejectsEmptyModelName(t *testing.T) {
	t.Parallel()

	runtimeCfg := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	svc := mustConstructModelService(t, modelServiceFixture{
		RuntimeConfig: func() *modelRuntimeConfig { return runtimeCfg },
	})

	_, err := svc.PullModel(context.Background(), "")
	if !errors.Is(err, models.ErrNotFound) {
		t.Fatalf("PullModel empty name = %v, want ErrNotFound", err)
	}
}

func TestService_InspectRuntime_RejectsEmptyModelName(t *testing.T) {
	t.Parallel()

	runtimeCfg := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	svc := mustConstructModelService(t, modelServiceFixture{
		RuntimeConfig: func() *modelRuntimeConfig { return runtimeCfg },
	})

	_, err := svc.InspectRuntime(context.Background(), "")
	if !errors.Is(err, models.ErrNotFound) {
		t.Fatalf("InspectRuntime empty name = %v, want ErrNotFound", err)
	}
}

func TestService_InspectRuntime_ProjectsReadyHostAndPreservesFailure(t *testing.T) {
	t.Parallel()

	runtimeCfg := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	ready, err := mustConstructModelService(t, modelServiceFixture{
		RuntimeConfig: func() *modelRuntimeConfig { return runtimeCfg },
		ModelHost:     hostLeaseTestHost{},
	}).InspectRuntime(context.Background(), "OMNIVOICE_Q4_K_M")
	if err != nil {
		t.Fatalf("InspectRuntime ready: %v", err)
	}
	if ready.Identity != "OMNIVOICE_Q4_K_M" ||
		ready.ReadinessState != models.ReadinessStateReady ||
		ready.LifecycleState != models.LifecycleStateInstalled {
		t.Fatalf("InspectRuntime ready = %#v, want ready installed model", ready)
	}

	failing, err := mustConstructModelService(t, modelServiceFixture{
		RuntimeConfig: func() *modelRuntimeConfig { return runtimeCfg },
		ModelHost:     failingInspectHost{},
	}).InspectRuntime(context.Background(), "OMNIVOICE_Q4_K_M")
	if err == nil || !strings.Contains(err.Error(), "inspect failed") {
		t.Fatalf("InspectRuntime failure = (%#v, %v), want host inspection error", failing, err)
	}
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

// Root component proof: controlled asset/host effects, real scope registration,
// and public operations. No runtime service map is populated by the fixture.
func TestRootTwoScopesScopedExecutionInvokeAndPullWithoutRuntimeGraphs(t *testing.T) {
	t.Parallel()
	root, host, runtime, resources := newScopedLocalRoot(t, nil, nil)
	a, b := openScopedLocalRoot(t, root, "cache-a", "endpoint-a"), openScopedLocalRoot(t, root, "cache-b", "endpoint-b")
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	for _, scope := range []models.RuntimeScopeRef{a, b, a, b} {
		assertScopedLocalRootInvocation(t, root, ctx, scope, host, resources)
		assertScopedLocalRootPull(t, root, ctx, scope, host)
	}
	if _, err := root.CloseRuntimeScope(ctx, models.CloseRuntimeScopeRequest{Scope: a}); err != nil {
		t.Fatal(err)
	}
	if _, err := root.InvokeLocal(ctx, scopedLocalRootRequest(a)); !errors.Is(err, models.ErrRuntimeScopeClosed) {
		t.Fatalf("closed A: %v", err)
	}
	assertScopedLocalRootInvocation(t, root, ctx, b, host, resources)
	assertScopedLocalRootPull(t, root, ctx, b, host)
	if result, err := root.PullModelForScope(ctx, models.PullModelRequest{Scope: a, Name: "cache-a"}); !errors.Is(err, models.ErrRuntimeScopeClosed) || result.CachePath != "" {
		t.Fatalf("closed A pull=%#v,%v", result, err)
	}
	host.mu.Lock()
	defer host.mu.Unlock()
	if len(host.releases) != 5 || runtime.loads != 2 {
		t.Fatalf("released leases/loaded handles=%d/%d", len(host.releases), runtime.loads)
	}
	for lease, count := range host.releases {
		if count != 1 {
			t.Fatalf("lease %s released %d times", lease, count)
		}
	}
}

func TestRootScopedExecutionCloseWinningCacheResolution(t *testing.T) {
	t.Parallel()
	started, resume := make(chan struct{}), make(chan struct{})
	root, host, runtime, resources := newScopedLocalRoot(t, started, resume)
	a, b := openScopedLocalRoot(t, root, "gated-cache", "endpoint-a"), openScopedLocalRoot(t, root, "cache-b", "endpoint-b")
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		result, err := root.InvokeLocal(ctx, scopedLocalRootRequest(a))
		if result.Content != "" {
			err = fmt.Errorf("closed invocation published %q", result.Content)
		}
		done <- err
	}()
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("A did not enter asset resolution")
	}
	assertScopedLocalRootInvocation(t, root, ctx, b, host, resources)
	if _, err := root.CloseRuntimeScope(ctx, models.CloseRuntimeScopeRequest{Scope: a}); err != nil {
		t.Fatal(err)
	}
	close(resume)
	select {
	case err := <-done:
		if !errors.Is(err, models.ErrRuntimeScopeClosed) {
			t.Fatalf("late resolution: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("A did not return")
	}
	assertScopedLocalRootInvocation(t, root, ctx, b, host, resources)
	host.mu.Lock()
	defer host.mu.Unlock()
	if len(host.releases) != 3 || runtime.loads != 1 {
		t.Fatalf("release/load counts=%d/%d", len(host.releases), runtime.loads)
	}
	for lease, count := range host.releases {
		if count != 1 {
			t.Fatalf("lease %s released %d times", lease, count)
		}
	}
}

func scopedLocalRootRequest(scope models.RuntimeScopeRef) models.LocalInvocationRequest {
	return models.LocalInvocationRequest{Scope: scope, Holder: "dispatch",
		Worker:    models.LocalWorker{Name: "voice", Type: models.RuntimeWorkerTypeModel, Model: "voice", ModelLocality: models.RuntimeModelLocalityLocal, Resources: []models.LocalResource{{Name: "voice", Capacity: 1}}},
		Resources: []models.LocalResource{{Name: "voice", Type: models.RuntimeResourceTypeModel, Model: "voice", Backend: "TEST", LoadPolicy: "ON_DEMAND", Capacity: 1}}}
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

func assertScopedLocalRootInvocation(t *testing.T, root *modelsservice.Root, ctx context.Context, scope models.RuntimeScopeRef, host *scopedLocalRootHost, resources *localmodels.ResourceLimiter) {
	t.Helper()
	binding, err := host.scopes.Resolve(runtimescopes.Reference(scope.String()))
	if err != nil {
		t.Fatal(err)
	}
	request := scopedLocalRootRequest(scope)
	result, err := root.InvokeLocal(ctx, request)
	want := binding.CacheDirectory + "|" + binding.RuntimeConfig().FactoryDirectory
	if err != nil || !result.Handled || result.Content != want {
		t.Fatalf("scope %s invoke=%#v,%v want %q", scope, result, err, want)
	}
	host.mu.Lock()
	active := len(host.active)
	host.mu.Unlock()
	// A peer may hold a lease; this invocation's lease must have been released.
	if active > 1 {
		t.Fatalf("unexpected retained leases: %d", active)
	}
	config := &models.RuntimeConfig{Resources: []models.RuntimeResource{{Name: "voice", Type: models.RuntimeResourceTypeModel, Model: "voice", Backend: "TEST", LoadPolicy: "ON_DEMAND", Capacity: 1}}}
	worker := &models.RuntimeWorker{Name: "voice", Type: models.RuntimeWorkerTypeModel, Model: "voice", ModelLocality: models.RuntimeModelLocalityLocal, Resources: config.Resources}
	release, err := resources.Acquire(ctx, scope, config, worker)
	if err != nil || release == nil {
		t.Fatalf("capacity after invoke: %v", err)
	}
	release()
}

func newScopedLocalRoot(t *testing.T, started, resume chan struct{}) (*modelsservice.Root, *scopedLocalRootHost, *scopedLocalRootRuntime, *localmodels.ResourceLimiter) {
	t.Helper()
	scopes, err := runtimescopeswire.NewService(func() string { return "scoped-local-root" })
	if err != nil {
		t.Fatal(err)
	}
	host := &scopedLocalRootHost{scopes: scopes, active: map[string]models.RuntimeScopeRef{}, releases: map[string]int{}}
	assets := scopedLocalRootAssets{scopes: scopes, started: started, resume: resume}
	runtime := &scopedLocalRootRuntime{}
	resources, err := localmodels.NewResourceLimiter(modelseffects.LocalRuntimeHooks{}, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	execution, err := modelsservice.NewScopedLocalExecution(scopes, assets, host, runtime, resources, modelseffects.LocalRuntimeHooks{}, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	root, err := modelsservice.NewRoot(resources, execution,
		scopes, struct{ modelcatalog.Service }{}, assets, host, struct{ inference.Service }{}, zap.NewNop(), time.Now, nil, nil,
		func(context.Context, string) (string, error) { return "", models.ErrModelRevisionUnresolved }, nil, models.AssetHostPlatform{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := root.Close(context.Background()); err != nil {
			t.Error(err)
		}
	})
	return root, host, runtime, resources
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

type scopedLocalRootRuntime struct {
	loads   int
	failure error
}

func (*scopedLocalRootRuntime) Supports(models.RuntimeResource, *models.RuntimeWorker) bool {
	return true
}
func (r *scopedLocalRootRuntime) Load(_ context.Context, q localmodels.LoadRequest) (localmodels.Handle, error) {
	r.loads++
	if r.failure != nil {
		return nil, r.failure
	}
	return scopedLocalRootHandle(q.CachePath + "|" + q.ServingEndpoint), nil
}

type scopedLocalRootHandle string

func (h scopedLocalRootHandle) Invoke(context.Context, localmodels.InvocationRequest) (localmodels.InvocationResponse, error) {
	return localmodels.InvocationResponse{Content: string(h)}, nil
}

func TestRootScopedExecutionReleasesCapacityOnFailure(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name      string
		failure   error
		failLease bool
	}{
		{"lease", models.ErrHostCapacityExhausted, true}, {"load", errors.New("load failed"), false}, {"cancel-effect", context.Canceled, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			root, host, runtime, resources := newScopedLocalRoot(t, nil, nil)
			scope := openScopedLocalRoot(t, root, "selected-cache", "selected-endpoint")
			if test.failLease {
				host.failure = test.failure
			} else {
				runtime.failure = test.failure
			}
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			result, err := root.InvokeLocal(ctx, scopedLocalRootRequest(scope))
			if !errors.Is(err, test.failure) || !result.Handled || result.Content != "" {
				t.Fatalf("failure result=%#v,%v", result, err)
			}
			host.failure, runtime.failure = nil, nil
			// A retry through the same public Root proves reservation capacity recovered.
			assertScopedLocalRootInvocation(t, root, ctx, scope, host, resources)
			host.mu.Lock()
			defer host.mu.Unlock()
			expected := 2
			if test.failLease {
				expected = 1
			}
			if len(host.active) != 0 || len(host.releases) != expected {
				t.Fatalf("active/released=%d/%d", len(host.active), len(host.releases))
			}
			for lease, count := range host.releases {
				if count != 1 {
					t.Fatalf("lease %s released %d times", lease, count)
				}
			}
		})
	}
}
