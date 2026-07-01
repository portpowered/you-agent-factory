package service_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/localmodels"
	modelsservice "github.com/portpowered/infinite-you/pkg/models/service"
	"github.com/portpowered/infinite-you/pkg/apisurface"
)

func TestService_PullModel_ReportsSuccessfulAlreadyPresentOutcome(t *testing.T) {
	t.Parallel()

	runtimeCfg := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	recorder := &capturingPullMetricsRecorder{}
	svc := modelsservice.New(modelsservice.Dependencies{
		RuntimeConfig: func() *factoryconfig.LoadedFactoryConfig { return runtimeCfg },
		ModelAssetPuller: func() localmodels.AssetPuller {
			return &stubPullAssetPuller{
				result: apisurface.ModelPullResult{
					ModelName:           "OMNIVOICE_Q4_K_M",
					Outcome:             "ALREADY_PRESENT",
					ManagedPullOutcome:  "ALREADY_READY",
					ReadinessState:      "READY",
					LifecycleState:      "READY",
					SourceKind:          "MANAGED_RUNTIME",
					CachePath:           "/tmp/cache",
					Revision:            "rev1",
				},
				inspection: localmodels.RuntimeCacheInspection{
					Supported: true,
					Installed: true,
					CachePath: "/tmp/cache",
					Revision:  "rev1",
				},
			}
		},
		ModelPullMetrics: func() modelsservice.PullMetricsRecorder { return recorder },
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
}

func TestService_PullModel_RecordsSourceFailureMetric(t *testing.T) {
	t.Parallel()

	runtimeCfg := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	recorder := &capturingPullMetricsRecorder{}
	svc := modelsservice.New(modelsservice.Dependencies{
		RuntimeConfig: func() *factoryconfig.LoadedFactoryConfig { return runtimeCfg },
		ModelAssetPuller: func() localmodels.AssetPuller {
			return &stubPullAssetPuller{err: apisurface.ErrManagedRuntimeSourceFetchFailed}
		},
		ModelPullMetrics: func() modelsservice.PullMetricsRecorder { return recorder },
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
}

func TestService_PullModel_ReturnsCanceledWhenContextCanceled(t *testing.T) {
	t.Parallel()

	runtimeCfg := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	svc := modelsservice.New(modelsservice.Dependencies{
		RuntimeConfig: func() *factoryconfig.LoadedFactoryConfig { return runtimeCfg },
		ModelAssetPuller: func() localmodels.AssetPuller {
			return &cancelBlockingPullAssetPuller{}
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := svc.PullModel(ctx, "OMNIVOICE_Q4_K_M")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("PullModel error = %v, want context.Canceled", err)
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

type stubPullAssetPuller struct {
	result     apisurface.ModelPullResult
	inspection localmodels.RuntimeCacheInspection
	cache      localmodels.CacheLayout
	err        error
}

func (p *stubPullAssetPuller) PullModel(context.Context, *factoryconfig.LoadedFactoryConfig, string) (apisurface.ModelPullResult, error) {
	return p.result, p.err
}

func (p *stubPullAssetPuller) EnsureModelAvailable(context.Context, *factoryconfig.LoadedFactoryConfig, *interfaces.WorkerConfig) error {
	return nil
}

func (p *stubPullAssetPuller) ResolveModelCache(context.Context, *factoryconfig.LoadedFactoryConfig, *interfaces.WorkerConfig) (localmodels.CacheLayout, error) {
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

func (p *cancelBlockingPullAssetPuller) EnsureModelAvailable(context.Context, *factoryconfig.LoadedFactoryConfig, *interfaces.WorkerConfig) error {
	return nil
}

func (p *cancelBlockingPullAssetPuller) ResolveModelCache(context.Context, *factoryconfig.LoadedFactoryConfig, *interfaces.WorkerConfig) (localmodels.CacheLayout, error) {
	return localmodels.CacheLayout{}, nil
}

func (p *cancelBlockingPullAssetPuller) InspectRuntimeCache(context.Context, *factoryconfig.LoadedFactoryConfig, string) (localmodels.RuntimeCacheInspection, error) {
	return localmodels.RuntimeCacheInspection{}, nil
}
