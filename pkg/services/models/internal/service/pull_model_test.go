package service_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	apisurface "github.com/portpowered/infinite-you/pkg/services/models"
	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
	localmodels "github.com/portpowered/infinite-you/pkg/services/models/internal/local"
	managedruntime "github.com/portpowered/infinite-you/pkg/services/models/internal/managedruntime"
	modelsservice "github.com/portpowered/infinite-you/pkg/services/models/internal/service"
	scopedassets "github.com/portpowered/infinite-you/pkg/services/models/internal/services/assets"
	modelcatalog "github.com/portpowered/infinite-you/pkg/services/models/internal/services/catalog"
	inference "github.com/portpowered/infinite-you/pkg/services/models/internal/services/inference"
	runtimescopeswire "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_scopes/wire"
)

func TestRootScopedPull_ReportsSuccessfulAlreadyPresentOutcome(t *testing.T) {
	t.Parallel()

	runtimeCfg := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	recorder := &capturingPullMetricsRecorder{}
	svc := mustConstructScopedPull(t, modelServiceFixture{
		RuntimeConfig: func() *modelRuntimeConfig { return runtimeCfg },
		ModelAssetPuller: &stubPullAssetPuller{
			result: apisurface.PullResult{
				ModelName:          "OMNIVOICE_Q4_K_M",
				Outcome:            "ALREADY_PRESENT",
				ManagedPullOutcome: "ALREADY_READY",
				ReadinessState:     "READY",
				LifecycleState:     "READY",
				SourceKind:         "MANAGED_RUNTIME",
				CachePath:          "/tmp/cache",
				Revision:           "rev1",
			},
			inspection: localmodels.RuntimeCacheInspection{
				Supported: true,
				Installed: true,
				CachePath: "/tmp/cache",
				Revision:  "rev1",
			},
		},
		ModelPullMetrics: recorder,
	})

	result, err := svc.PullModel(context.Background(), "OMNIVOICE_Q4_K_M")
	if err != nil {
		t.Fatalf("PullModel: %v", err)
	}
	if result.ReadinessState != "READY" || result.ManagedPullOutcome != "ALREADY_READY" {
		t.Fatalf("pull result = %#v, want READY already-ready outcome", result)
	}
	recorder.assertContainsMetric(t, "managed_runtime.pull.attempts", map[string]string{"model_name": "OMNIVOICE_Q4_K_M"})
	recorder.assertContainsMetric(t, "managed_runtime.pull.success", map[string]string{
		"model_name":      "OMNIVOICE_Q4_K_M",
		"pull_outcome":    "ALREADY_READY",
		"readiness_state": "READY",
	})
	if recorder.count("managed_runtime.pull.attempts") != 1 || recorder.count("managed_runtime.pull.success") != 1 {
		t.Fatalf("pull metric counts = %#v, want one attempt and one success", recorder.metrics)
	}
}

func TestRootScopedPull_ReportsStillLoadingWhenAssetsRemainMissing(t *testing.T) {
	t.Parallel()

	runtimeCfg := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	svc := mustConstructScopedPull(t, modelServiceFixture{
		RuntimeConfig: func() *modelRuntimeConfig { return runtimeCfg },
		ModelAssetPuller: &stubPullAssetPuller{
			result: apisurface.PullResult{ModelName: "OMNIVOICE_Q4_K_M", Outcome: "PULLED"},
			inspection: localmodels.RuntimeCacheInspection{
				Supported:     true,
				Installed:     false,
				MissingAssets: []string{"model.gguf"},
			},
		},
	})

	result, err := svc.PullModel(context.Background(), "OMNIVOICE_Q4_K_M")
	if err != nil {
		t.Fatalf("PullModel: %v", err)
	}
	if result.ManagedPullOutcome != "STILL_LOADING" || result.ReadinessState != "LOADING" || result.LifecycleState != "INSTALLING" {
		t.Fatalf("pull result = %#v, want STILL_LOADING/LOADING/INSTALLING", result)
	}
}

func TestRootScopedPull_RecordsSourceFailureMetric(t *testing.T) {
	t.Parallel()

	runtimeCfg := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	recorder := &capturingPullMetricsRecorder{}
	svc := mustConstructScopedPull(t, modelServiceFixture{
		RuntimeConfig:    func() *modelRuntimeConfig { return runtimeCfg },
		ModelAssetPuller: &stubPullAssetPuller{err: apisurface.ErrSourceFetchFailed},
		ModelPullMetrics: recorder,
	})

	_, err := svc.PullModel(context.Background(), "OMNIVOICE_Q4_K_M")
	var pullErr *apisurface.PullError
	if !errors.As(err, &pullErr) {
		t.Fatalf("PullModel error = %v, want managed runtime pull failure", err)
	}
	if pullErr.Result.ManagedPullOutcome != "SOURCE_FETCH_FAILED" ||
		pullErr.Result.ReadinessState != "FAILED" {
		t.Fatalf("pull failure result = %#v, want SOURCE_FETCH_FAILED/FAILED", pullErr.Result)
	}
	if !errors.Is(err, apisurface.ErrSourceFetchFailed) {
		t.Fatalf("PullModel error = %v, want source fetch failure cause", err)
	}
	recorder.assertContainsMetric(t, "managed_runtime.pull.failure", map[string]string{
		"model_name":   "OMNIVOICE_Q4_K_M",
		"pull_outcome": "SOURCE_FETCH_FAILED",
	})
	recorder.assertContainsMetric(t, "managed_runtime.pull.source_failure", map[string]string{
		"model_name": "OMNIVOICE_Q4_K_M",
	})
	if recorder.count("managed_runtime.pull.attempts") != 1 || recorder.count("managed_runtime.pull.failure") != 1 ||
		recorder.count("managed_runtime.pull.source_failure") != 1 {
		t.Fatalf("pull metric counts = %#v, want one attempt, failure, and source failure", recorder.metrics)
	}
}

func TestRootScopedPull_ReturnsCanceledWhenContextCanceled(t *testing.T) {
	t.Parallel()

	runtimeCfg := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	svc := mustConstructScopedPull(t, modelServiceFixture{
		RuntimeConfig:    func() *modelRuntimeConfig { return runtimeCfg },
		ModelAssetPuller: &cancelBlockingPullAssetPuller{},
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := svc.PullModel(ctx, "OMNIVOICE_Q4_K_M")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("PullModel error = %v, want context.Canceled", err)
	}
}

func TestRootScopedPull_ClassifiesAssetTimeout(t *testing.T) {
	t.Parallel()

	runtimeCfg := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	recorder := &capturingPullMetricsRecorder{}
	svc := mustConstructScopedPull(t, modelServiceFixture{
		RuntimeConfig:    func() *modelRuntimeConfig { return runtimeCfg },
		ModelAssetPuller: &stubPullAssetPuller{err: context.DeadlineExceeded},
		ModelPullMetrics: recorder,
	})

	result, err := svc.PullModel(context.Background(), "OMNIVOICE_Q4_K_M")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("PullModel error = %v, want deadline exceeded cause", err)
	}
	var pullErr *apisurface.PullError
	if !errors.As(err, &pullErr) || result.ManagedPullOutcome != "TIMED_OUT" || result.ReadinessState != "FAILED" {
		t.Fatalf("PullModel = (%#v, %v), want classified TIMED_OUT failure", result, err)
	}
	recorder.assertContainsMetric(t, "managed_runtime.pull.failure", map[string]string{
		"model_name": "OMNIVOICE_Q4_K_M", "pull_outcome": "TIMED_OUT",
	})
	if recorder.count("managed_runtime.pull.source_failure") != 0 {
		t.Fatalf("source failure metrics = %d, want 0 for timeout", recorder.count("managed_runtime.pull.source_failure"))
	}
}

func TestRootScopedPull_LogsSuccessAndFailureOutcomes(t *testing.T) {
	t.Parallel()

	runtimeCfg := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	core, observed := observer.New(zap.InfoLevel)
	logger := zap.New(core)
	svc := mustConstructScopedPull(t, modelServiceFixture{
		RuntimeConfig: func() *modelRuntimeConfig { return runtimeCfg },
		Logger:        logger,
		ModelAssetPuller: &stubPullAssetPuller{
			result: apisurface.PullResult{
				ModelName:          "OMNIVOICE_Q4_K_M",
				ManagedPullOutcome: "ALREADY_READY",
				ReadinessState:     "READY",
				LifecycleState:     "READY",
				SourceKind:         "MANAGED_RUNTIME",
			},
		},
	})

	if _, err := svc.PullModel(context.Background(), "OMNIVOICE_Q4_K_M"); err != nil {
		t.Fatalf("PullModel success path: %v", err)
	}

	svc = mustConstructScopedPull(t, modelServiceFixture{
		RuntimeConfig: func() *modelRuntimeConfig { return runtimeCfg },
		Logger:        logger,
		ModelAssetPuller: &stubPullAssetPuller{
			result: apisurface.PullResult{
				ModelName: "OMNIVOICE_Q4_K_M", SourceKind: "UPSTREAM_REPOSITORY",
				SourceID: "owner/repo", Revision: "rev-1",
				PullDiagnostics: apisurface.PullDiagnostics{
					ResolvedRepository: "owner/repo", Revision: "rev-1",
					File: "weights.gguf", Operation: "verify downloaded asset",
				},
			},
			err: errors.New("opaque body=secret C:\\private\\weights"),
		},
	})
	if _, err := svc.PullModel(context.Background(), "OMNIVOICE_Q4_K_M"); err == nil {
		t.Fatal("PullModel failure path: nil error, want pull failure")
	}
	if observed.FilterMessage("managed runtime pull completed").Len() != 1 {
		t.Fatalf("success logs = %d, want 1", observed.FilterMessage("managed runtime pull completed").Len())
	}
	if observed.FilterMessage("managed runtime pull failed").Len() != 1 {
		t.Fatalf("failure logs = %d, want 1", observed.FilterMessage("managed runtime pull failed").Len())
	}
	if observed.FilterMessage("managed runtime pull started").Len() != 2 {
		t.Fatalf("start logs = %d, want one per pull", observed.FilterMessage("managed runtime pull started").Len())
	}
	failureEntries := observed.FilterMessage("managed runtime pull failed").All()
	if got := failureEntries[0].ContextMap()["failure_reason"]; got != "pull_failed" {
		t.Fatalf("failure reason = %#v, want pull_failed", got)
	}
	if got := failureEntries[0].ContextMap()["lifecycle_state"]; got != string(managedruntime.LifecycleStateNotInstalled) {
		t.Fatalf("failure lifecycle = %#v, want NOT_INSTALLED", got)
	}
	fields := failureEntries[0].ContextMap()
	for key, want := range map[string]string{
		"model_name": "OMNIVOICE_Q4_K_M", "operation": "verify downloaded asset",
		"terminal_classification": "ASSET_PREPARATION_FAILED", "resolved_source": "owner/repo",
		"resolved_repository": "owner/repo", "revision": "rev-1", "file": "weights.gguf",
	} {
		if got := fields[key]; got != want {
			t.Fatalf("failure field %s = %#v, want %q", key, got, want)
		}
	}
	if _, leaked := fields["error"]; leaked {
		t.Fatalf("failure log retained raw error field: %#v", fields)
	}
}

func TestRootScopedPull_UsesInjectedClockForDuration(t *testing.T) {
	t.Parallel()

	runtimeCfg := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	core, observed := observer.New(zap.InfoLevel)
	times := []time.Time{
		time.Date(2026, time.July, 10, 12, 0, 0, 0, time.UTC),
		time.Date(2026, time.July, 10, 12, 0, 2, 500_000_000, time.UTC),
	}
	svc := mustConstructScopedPull(t, modelServiceFixture{
		RuntimeConfig: func() *modelRuntimeConfig { return runtimeCfg },
		Clock: func() time.Time {
			now := times[0]
			times = times[1:]
			return now
		},
		Logger: zap.New(core),
		ModelAssetPuller: &stubPullAssetPuller{result: apisurface.PullResult{
			ModelName:          "OMNIVOICE_Q4_K_M",
			ManagedPullOutcome: "ALREADY_READY",
			ReadinessState:     "READY",
			LifecycleState:     "READY",
			SourceKind:         "MANAGED_RUNTIME",
		}},
	})

	if _, err := svc.PullModel(context.Background(), "OMNIVOICE_Q4_K_M"); err != nil {
		t.Fatalf("PullModel: %v", err)
	}
	entries := observed.FilterMessage("managed runtime pull completed").All()
	if len(entries) != 1 {
		t.Fatalf("success logs = %d, want 1", len(entries))
	}
	if got := entries[0].ContextMap()["duration"]; got != 2500*time.Millisecond {
		t.Fatalf("logged duration = %#v, want 2.5s in nanoseconds", got)
	}
}

type capturingPullMetricsRecorder struct {
	mu      sync.Mutex
	metrics []modelseffects.PullMetric
}

func (r *capturingPullMetricsRecorder) RecordModelPullMetric(metric modelseffects.PullMetric) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.metrics = append(r.metrics, metric)
}

func (r *capturingPullMetricsRecorder) assertContainsMetric(t *testing.T, name string, labels map[string]string) {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, metric := range r.metrics {
		if metric.Name != name {
			continue
		}
		match := true
		for key, value := range labels {
			if metric.Labels[key] != value {
				match = false
				break
			}
		}
		if match {
			return
		}
	}
	t.Fatalf("metrics %#v do not contain %q with labels %#v", r.metrics, name, labels)
}

func (r *capturingPullMetricsRecorder) count(name string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	count := 0
	for _, metric := range r.metrics {
		if metric.Name == name {
			count++
		}
	}
	return count
}

type stubPullAssetPuller struct {
	result     apisurface.PullResult
	inspection localmodels.RuntimeCacheInspection
	cache      localmodels.CacheLayout
	err        error
	gotContext context.Context
	gotConfig  *modelRuntimeConfig
	gotName    string
}

func (p *stubPullAssetPuller) PullModel(ctx context.Context, config *modelRuntimeConfig, name string) (apisurface.PullResult, error) {
	p.gotContext, p.gotConfig, p.gotName = ctx, config, name
	return p.result, p.err
}

func (p *stubPullAssetPuller) EnsureModelAvailable(context.Context, *modelRuntimeConfig, *modelRuntimeWorker) error {
	return nil
}

func (p *stubPullAssetPuller) ResolveModelCache(context.Context, *modelRuntimeConfig, *modelRuntimeWorker) (localmodels.CacheLayout, error) {
	return p.cache, nil
}

func (p *stubPullAssetPuller) InspectRuntimeCache(context.Context, *modelRuntimeConfig, string) (localmodels.RuntimeCacheInspection, error) {
	return p.inspection, nil
}

type cancelBlockingPullAssetPuller struct{}

func (p *cancelBlockingPullAssetPuller) PullModel(ctx context.Context, _ *modelRuntimeConfig, _ string) (apisurface.PullResult, error) {
	<-ctx.Done()
	return apisurface.PullResult{}, ctx.Err()
}

func (p *cancelBlockingPullAssetPuller) EnsureModelAvailable(context.Context, *modelRuntimeConfig, *modelRuntimeWorker) error {
	return nil
}

func (p *cancelBlockingPullAssetPuller) ResolveModelCache(context.Context, *modelRuntimeConfig, *modelRuntimeWorker) (localmodels.CacheLayout, error) {
	return localmodels.CacheLayout{}, nil
}

func (p *cancelBlockingPullAssetPuller) InspectRuntimeCache(context.Context, *modelRuntimeConfig, string) (localmodels.RuntimeCacheInspection, error) {
	return localmodels.RuntimeCacheInspection{}, nil
}

// scopedPullFixture observes Root's public scoped operation with controlled
// asset effects. Host snapshot conversion belonged only to the retired adapter.
type scopedPullFixture struct {
	root  *modelsservice.Root
	scope apisurface.RuntimeScopeRef
}

func (f scopedPullFixture) PullModel(ctx context.Context, name string) (apisurface.PullResult, error) {
	return f.root.PullModelForScope(ctx, apisurface.PullModelRequest{Scope: f.scope, Name: name})
}

func mustConstructScopedPull(t *testing.T, fixture modelServiceFixture) scopedPullFixture {
	t.Helper()
	scopes, err := runtimescopeswire.NewService(func() string { return "pull-observation" })
	if err != nil {
		t.Fatal(err)
	}
	scope, err := scopes.Open(apisurface.RuntimeBinding{RuntimeConfig: fixture.RuntimeConfig})
	if err != nil {
		t.Fatal(err)
	}
	if fixture.Logger == nil {
		fixture.Logger = zap.NewNop()
	}
	if fixture.Clock == nil {
		fixture.Clock = time.Now
	}
	pull := func(ctx context.Context, request apisurface.PullModelRequest) (apisurface.PullResult, error) {
		return localmodels.PullModelWithOptions(fixture.ModelAssetPuller, ctx, fixture.RuntimeConfig(), request.Name, localmodels.PullOptions{
			RuntimeCacheInspector: fixture.ModelAssetPuller, SourceResolver: localmodels.DefaultManagedRuntimeSourceResolver(),
		})
	}
	resources, err := localmodels.NewResourceLimiter(modelseffects.LocalRuntimeHooks{}, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	root, err := modelsservice.NewRoot(resources, pull,
		func(context.Context, apisurface.LocalInvocationRequest) (apisurface.LocalInvocationResult, error) {
			return apisurface.LocalInvocationResult{}, nil
		},
		func(apisurface.RuntimeScopeRef) {}, func() {}, scopes, struct{ modelcatalog.Service }{}, struct{ scopedassets.Service }{},
		&scopedLocalRootHost{}, struct{ inference.Service }{}, fixture.Logger, fixture.Clock, fixture.ModelPullMetrics, nil,
		func(context.Context, string) (string, error) { return "", apisurface.ErrModelRevisionUnresolved }, nil, apisurface.AssetHostPlatform{})
	if err != nil {
		t.Fatal(err)
	}
	parsedScope, err := (apisurface.RuntimeScopeRef{}).Parse(string(scope))
	if err != nil {
		t.Fatal(err)
	}
	return scopedPullFixture{root: root, scope: parsedScope}
}

func TestRootScopedPullRejectsEmptyName(t *testing.T) {
	t.Parallel()
	_, err := (&modelsservice.Root{}).PullModelForScope(t.Context(), apisurface.PullModelRequest{Name: " "})
	if !errors.Is(err, apisurface.ErrNotFound) {
		t.Fatalf("empty name error = %v", err)
	}
}

func TestRootScopedPullPreservesAssetMetadataAndOperationInputs(t *testing.T) {
	t.Parallel()
	config := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	assets := &stubPullAssetPuller{
		result: apisurface.PullResult{ModelName: "OMNIVOICE_Q4_K_M", Outcome: "PULLED", CachePath: "/tmp/cache", Revision: "rev1",
			DownloadedFiles: []apisurface.DownloadedFile{{Path: "model.gguf", Bytes: 42, SHA256: "abc"}}},
		inspection: localmodels.RuntimeCacheInspection{Supported: true, Installed: true, CachePath: "/tmp/cache", Revision: "rev1"},
	}
	fixture := mustConstructScopedPull(t, modelServiceFixture{RuntimeConfig: func() *modelRuntimeConfig { return config }, ModelAssetPuller: assets})
	result, err := fixture.PullModel(t.Context(), "OMNIVOICE_Q4_K_M")
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != "PULLED" || result.CachePath != "/tmp/cache" || result.Revision != "rev1" || len(result.DownloadedFiles) != 1 || result.DownloadedFiles[0].Bytes != 42 || result.DownloadedFiles[0].SHA256 != "abc" {
		t.Fatalf("asset metadata = %#v", result)
	}
	if assets.gotContext != t.Context() || assets.gotConfig != config || assets.gotName != "OMNIVOICE_Q4_K_M" {
		t.Fatal("asset pull lost selected context, configuration or model name")
	}
}

func TestRootScopedPullPreservesUnsupportedClassification(t *testing.T) {
	t.Parallel()
	config := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	fixture := mustConstructScopedPull(t, modelServiceFixture{RuntimeConfig: func() *modelRuntimeConfig { return config },
		ModelAssetPuller: &stubPullAssetPuller{err: apisurface.ErrPullUnsupported}})
	_, err := fixture.PullModel(t.Context(), "OMNIVOICE_Q4_K_M")
	if !errors.Is(err, apisurface.ErrPullUnsupported) {
		t.Fatalf("unsupported pull error = %v", err)
	}
}
