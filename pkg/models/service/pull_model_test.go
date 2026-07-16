package service_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	workerconfig "github.com/portpowered/infinite-you/pkg/workers/config"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	modelhost "github.com/portpowered/infinite-you/pkg/models/host"
	localmodels "github.com/portpowered/infinite-you/pkg/models/local"
	modelsservice "github.com/portpowered/infinite-you/pkg/models/service"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
)

func TestService_PullModel_ReportsSuccessfulAlreadyPresentOutcome(t *testing.T) {
	t.Parallel()

	runtimeCfg := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	recorder := &capturingPullMetricsRecorder{}
	svc := mustConstructModelService(t, modelsservice.Dependencies{
		RuntimeConfig: func() *factoryconfig.LoadedFactoryConfig { return runtimeCfg },
		ModelAssetPuller: &stubPullAssetPuller{
			result: apisurface.ModelPullResult{
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

func TestService_PullModel_ReportsStillLoadingWhenAssetsRemainMissing(t *testing.T) {
	t.Parallel()

	runtimeCfg := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	svc := mustConstructModelService(t, modelsservice.Dependencies{
		RuntimeConfig: func() *factoryconfig.LoadedFactoryConfig { return runtimeCfg },
		ModelAssetPuller: &stubPullAssetPuller{
			result: apisurface.ModelPullResult{ModelName: "OMNIVOICE_Q4_K_M", Outcome: "PULLED"},
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

func TestService_PullModel_RecordsSourceFailureMetric(t *testing.T) {
	t.Parallel()

	runtimeCfg := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	recorder := &capturingPullMetricsRecorder{}
	svc := mustConstructModelService(t, modelsservice.Dependencies{
		RuntimeConfig:    func() *factoryconfig.LoadedFactoryConfig { return runtimeCfg },
		ModelAssetPuller: &stubPullAssetPuller{err: apisurface.ErrManagedRuntimeSourceFetchFailed},
		ModelPullMetrics: recorder,
	})

	_, err := svc.PullModel(context.Background(), "OMNIVOICE_Q4_K_M")
	pullErr, ok := apisurface.AsManagedRuntimePullError(err)
	if !ok {
		t.Fatalf("PullModel error = %v, want managed runtime pull failure", err)
	}
	if pullErr.Result.ManagedPullOutcome != "SOURCE_FETCH_FAILED" ||
		pullErr.Result.ReadinessState != "FAILED" {
		t.Fatalf("pull failure result = %#v, want SOURCE_FETCH_FAILED/FAILED", pullErr.Result)
	}
	if !errors.Is(err, apisurface.ErrManagedRuntimeSourceFetchFailed) {
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

func TestService_PullModel_ReturnsCanceledWhenContextCanceled(t *testing.T) {
	t.Parallel()

	runtimeCfg := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	svc := mustConstructModelService(t, modelsservice.Dependencies{
		RuntimeConfig:    func() *factoryconfig.LoadedFactoryConfig { return runtimeCfg },
		ModelAssetPuller: &cancelBlockingPullAssetPuller{},
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := svc.PullModel(ctx, "OMNIVOICE_Q4_K_M")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("PullModel error = %v, want context.Canceled", err)
	}
}

func TestService_PullModel_ProjectsModelHostResult(t *testing.T) {
	t.Parallel()

	runtimeCfg := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	host := &stubPullModelHost{snapshot: modelhost.PullSnapshot{
		ReadinessSnapshot: modelhost.ReadinessSnapshot{
			Identity: modelhost.Identity{
				Name:       "OMNIVOICE_Q4_K_M",
				Locality:   factoryapi.WorkerModelLocalityLocal,
				SourceKind: "MANAGED_RUNTIME",
				SourceID:   "managed:omnivoice",
			},
			ReadinessState: factoryapi.ManagedRuntimeReadinessStateREADY,
			LifecycleState: factoryapi.ManagedRuntimeLifecycleStateINSTALLED,
		},
		PullOutcome:   factoryapi.ManagedRuntimePullOutcomeINSTALLEDSUCCESSFULLY,
		LegacyOutcome: "PULLED",
		CachePath:     "/tmp/cache",
		Revision:      "rev1",
		DownloadedFiles: []modelhost.PullDownloadedFile{{
			Path: "model.gguf", Bytes: 42, SHA256: "abc",
		}},
	}}
	svc := mustConstructModelService(t, modelsservice.Dependencies{
		RuntimeConfig: func() *factoryconfig.LoadedFactoryConfig { return runtimeCfg },
		ModelHost:     host,
	})

	result, err := svc.PullModel(context.Background(), "OMNIVOICE_Q4_K_M")
	if err != nil {
		t.Fatalf("PullModel: %v", err)
	}
	if result.Outcome != "PULLED" || result.ManagedPullOutcome != "INSTALLED_SUCCESSFULLY" ||
		result.ReadinessState != "READY" || result.CachePath != "/tmp/cache" || len(result.DownloadedFiles) != 1 {
		t.Fatalf("pull result = %#v, want mapped host metadata", result)
	}
	if host.gotRuntimeCfg != runtimeCfg || host.gotModelName != "OMNIVOICE_Q4_K_M" {
		t.Fatalf("host call = (%p, %q), want supplied runtime and model", host.gotRuntimeCfg, host.gotModelName)
	}
}

func TestService_PullModel_ClassifiesModelHostTimeout(t *testing.T) {
	t.Parallel()

	runtimeCfg := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	recorder := &capturingPullMetricsRecorder{}
	svc := mustConstructModelService(t, modelsservice.Dependencies{
		RuntimeConfig:    func() *factoryconfig.LoadedFactoryConfig { return runtimeCfg },
		ModelHost:        &stubPullModelHost{err: context.DeadlineExceeded},
		ModelPullMetrics: recorder,
	})

	result, err := svc.PullModel(context.Background(), "OMNIVOICE_Q4_K_M")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("PullModel error = %v, want deadline exceeded cause", err)
	}
	if !apisurface.IsManagedRuntimePullError(err) || result.ManagedPullOutcome != "TIMED_OUT" || result.ReadinessState != "FAILED" {
		t.Fatalf("PullModel = (%#v, %v), want classified TIMED_OUT failure", result, err)
	}
	recorder.assertContainsMetric(t, "managed_runtime.pull.failure", map[string]string{
		"model_name": "OMNIVOICE_Q4_K_M", "pull_outcome": "TIMED_OUT",
	})
	if recorder.count("managed_runtime.pull.source_failure") != 0 {
		t.Fatalf("source failure metrics = %d, want 0 for timeout", recorder.count("managed_runtime.pull.source_failure"))
	}
}

func TestService_PullModel_PropagatesCancellationToModelHost(t *testing.T) {
	t.Parallel()

	runtimeCfg := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	host := &stubPullModelHost{waitForContext: true}
	svc := mustConstructModelService(t, modelsservice.Dependencies{
		RuntimeConfig: func() *factoryconfig.LoadedFactoryConfig { return runtimeCfg },
		ModelHost:     host,
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := svc.PullModel(ctx, "OMNIVOICE_Q4_K_M")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("PullModel error = %v, want context.Canceled", err)
	}
	if host.gotContext != ctx {
		t.Fatal("model host did not receive the original canceled context")
	}
}

func TestService_PullModel_MapsUnsupportedModelHostResult(t *testing.T) {
	t.Parallel()

	runtimeCfg := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	svc := mustConstructModelService(t, modelsservice.Dependencies{
		RuntimeConfig: func() *factoryconfig.LoadedFactoryConfig { return runtimeCfg },
		ModelHost:     &stubPullModelHost{err: modelhost.ErrUnsupportedRuntime},
	})

	_, err := svc.PullModel(context.Background(), "OMNIVOICE_Q4_K_M")
	if !errors.Is(err, apisurface.ErrModelPullUnsupported) {
		t.Fatalf("PullModel error = %v, want ErrModelPullUnsupported", err)
	}
}

func TestService_PullModel_LogsSuccessAndFailureOutcomes(t *testing.T) {
	t.Parallel()

	runtimeCfg := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	core, observed := observer.New(zap.InfoLevel)
	logger := zap.New(core)
	svc := mustConstructModelService(t, modelsservice.Dependencies{
		RuntimeConfig: func() *factoryconfig.LoadedFactoryConfig { return runtimeCfg },
		Logger:        logger,
		ModelAssetPuller: &stubPullAssetPuller{
			result: apisurface.ModelPullResult{
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

	svc = mustConstructModelService(t, modelsservice.Dependencies{
		RuntimeConfig:    func() *factoryconfig.LoadedFactoryConfig { return runtimeCfg },
		Logger:           logger,
		ModelAssetPuller: &stubPullAssetPuller{err: errors.New("pull failed")},
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
}

func TestService_PullModel_UsesInjectedClockForDuration(t *testing.T) {
	t.Parallel()

	runtimeCfg := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	core, observed := observer.New(zap.InfoLevel)
	times := []time.Time{
		time.Date(2026, time.July, 10, 12, 0, 0, 0, time.UTC),
		time.Date(2026, time.July, 10, 12, 0, 2, 500_000_000, time.UTC),
	}
	svc := mustConstructModelService(t, modelsservice.Dependencies{
		RuntimeConfig: func() *factoryconfig.LoadedFactoryConfig { return runtimeCfg },
		Clock: func() time.Time {
			now := times[0]
			times = times[1:]
			return now
		},
		Logger: zap.New(core),
		ModelAssetPuller: &stubPullAssetPuller{result: apisurface.ModelPullResult{
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
	metrics []modelsservice.PullMetric
}

func (r *capturingPullMetricsRecorder) RecordModelPullMetric(metric modelsservice.PullMetric) {
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

type stubPullModelHost struct {
	snapshot       modelhost.PullSnapshot
	err            error
	waitForContext bool
	gotContext     context.Context
	gotRuntimeCfg  *factoryconfig.LoadedFactoryConfig
	gotModelName   string
}

func (h *stubPullModelHost) Pull(ctx context.Context, runtimeCfg *factoryconfig.LoadedFactoryConfig, modelName string) (modelhost.PullSnapshot, error) {
	h.gotContext = ctx
	h.gotRuntimeCfg = runtimeCfg
	h.gotModelName = modelName
	if h.waitForContext {
		<-ctx.Done()
		return modelhost.PullSnapshot{}, ctx.Err()
	}
	return h.snapshot, h.err
}

func (*stubPullModelHost) ResolveIdentity(context.Context, *factoryconfig.LoadedFactoryConfig, string) (modelhost.Identity, error) {
	return modelhost.Identity{}, nil
}

func (*stubPullModelHost) InspectReadiness(context.Context, *factoryconfig.LoadedFactoryConfig, string) (modelhost.ReadinessSnapshot, error) {
	return modelhost.ReadinessSnapshot{}, nil
}

func (*stubPullModelHost) AcquireLease(context.Context, *factoryconfig.LoadedFactoryConfig, string, modelhost.LeaseOptions) (modelhost.Lease, error) {
	return modelhost.Lease{}, nil
}

func (*stubPullModelHost) ReleaseLease(context.Context, string) error { return nil }

func (*stubPullModelHost) Unload(context.Context, *factoryconfig.LoadedFactoryConfig, string) error {
	return nil
}

type stubPullAssetPuller struct {
	result     apisurface.ModelPullResult
	inspection localmodels.RuntimeCacheInspection
	cache      localmodels.CacheLayout
	err        error
}

func (p *stubPullAssetPuller) PullModel(context.Context, *factoryconfig.LoadedFactoryConfig, string) (apisurface.ModelPullResult, error) {
	return p.result, p.err
}

func (p *stubPullAssetPuller) EnsureModelAvailable(context.Context, *factoryconfig.LoadedFactoryConfig, *workerconfig.Config) error {
	return nil
}

func (p *stubPullAssetPuller) ResolveModelCache(context.Context, *factoryconfig.LoadedFactoryConfig, *workerconfig.Config) (localmodels.CacheLayout, error) {
	return p.cache, nil
}

func (p *stubPullAssetPuller) InspectRuntimeCache(context.Context, *factoryconfig.LoadedFactoryConfig, string) (localmodels.RuntimeCacheInspection, error) {
	return p.inspection, nil
}

type cancelBlockingPullAssetPuller struct{}

func (p *cancelBlockingPullAssetPuller) PullModel(ctx context.Context, _ *factoryconfig.LoadedFactoryConfig, _ string) (apisurface.ModelPullResult, error) {
	<-ctx.Done()
	return apisurface.ModelPullResult{}, ctx.Err()
}

func (p *cancelBlockingPullAssetPuller) EnsureModelAvailable(context.Context, *factoryconfig.LoadedFactoryConfig, *workerconfig.Config) error {
	return nil
}

func (p *cancelBlockingPullAssetPuller) ResolveModelCache(context.Context, *factoryconfig.LoadedFactoryConfig, *workerconfig.Config) (localmodels.CacheLayout, error) {
	return localmodels.CacheLayout{}, nil
}

func (p *cancelBlockingPullAssetPuller) InspectRuntimeCache(context.Context, *factoryconfig.LoadedFactoryConfig, string) (localmodels.RuntimeCacheInspection, error) {
	return localmodels.RuntimeCacheInspection{}, nil
}
