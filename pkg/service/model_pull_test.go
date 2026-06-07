package service

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/portpowered/infinite-you/pkg/apisurface"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/localmodels"
)

func TestRuntimeModelService_PullModel_RecordsManagedRuntimeMetrics(t *testing.T) {
	recorder := &capturingModelPullMetricsRecorder{}
	runtimeCfg, err := factoryconfig.NewLoadedFactoryConfig(t.TempDir(), &interfaces.FactoryConfig{
		Name: "factory",
		Workers: []interfaces.WorkerConfig{{
			Name:          "voice-local",
			Type:          interfaces.WorkerTypeModel,
			Model:         "OMNIVOICE_Q4_K_M",
			ModelLocality: interfaces.ModelLocalityLocal,
			Operations:    []interfaces.ModelOperation{{Name: "TTS"}},
		}},
		Resources: []interfaces.ResourceConfig{{
			Name:     "omnivoice-cache",
			Type:     interfaces.ResourceTypeModel,
			Capacity: 1,
			Model:    "OMNIVOICE_Q4_K_M",
		}},
	}, nil)
	if err != nil {
		t.Fatalf("NewLoadedFactoryConfig: %v", err)
	}

	svc := newModelService(modelServiceDependencies{
		runtimeConfig: func() *factoryconfig.LoadedFactoryConfig { return runtimeCfg },
		modelAssetPuller: func() modelAssetPuller {
			return &managedPullMetricsAssetPuller{
				result: apisurface.ModelPullResult{
					ModelName: "OMNIVOICE_Q4_K_M",
					Outcome:   "ALREADY_PRESENT",
					CachePath: "/tmp/cache",
					Revision:  "rev1",
				},
				inspection: localmodels.RuntimeCacheInspection{
					Supported: true,
					Installed: true,
					CachePath: "/tmp/cache",
					Revision:  "rev1",
				},
			}
		},
		modelPullMetrics: func() ModelPullMetricsRecorder { return recorder },
	})

	if _, err := svc.PullModel(context.Background(), "OMNIVOICE_Q4_K_M"); err != nil {
		t.Fatalf("PullModel: %v", err)
	}
	recorder.assertContainsMetric(t, modelPullMetricAttempts, map[string]string{"model_name": "OMNIVOICE_Q4_K_M"})
	recorder.assertContainsMetric(t, modelPullMetricSuccess, map[string]string{
		"model_name":      "OMNIVOICE_Q4_K_M",
		"pull_outcome":    "ALREADY_READY",
		"readiness_state": "READY",
	})
}

func TestRuntimeModelService_PullModel_RecordsSourceFailureMetric(t *testing.T) {
	recorder := &capturingModelPullMetricsRecorder{}
	runtimeCfg, err := factoryconfig.NewLoadedFactoryConfig(t.TempDir(), &interfaces.FactoryConfig{
		Name: "factory",
		Workers: []interfaces.WorkerConfig{{
			Name:          "voice-local",
			Type:          interfaces.WorkerTypeModel,
			Model:         "OMNIVOICE_Q4_K_M",
			ModelLocality: interfaces.ModelLocalityLocal,
			Operations:    []interfaces.ModelOperation{{Name: "TTS"}},
		}},
		Resources: []interfaces.ResourceConfig{{
			Name:     "omnivoice-cache",
			Type:     interfaces.ResourceTypeModel,
			Capacity: 1,
			Model:    "OMNIVOICE_Q4_K_M",
		}},
	}, nil)
	if err != nil {
		t.Fatalf("NewLoadedFactoryConfig: %v", err)
	}

	svc := newModelService(modelServiceDependencies{
		runtimeConfig: func() *factoryconfig.LoadedFactoryConfig { return runtimeCfg },
		modelAssetPuller: func() modelAssetPuller {
			return &managedPullMetricsAssetPuller{
				err: apisurface.ErrManagedRuntimeSourceFetchFailed,
			}
		},
		modelPullMetrics: func() ModelPullMetricsRecorder { return recorder },
	})

	if _, err := svc.PullModel(context.Background(), "OMNIVOICE_Q4_K_M"); !errors.Is(err, apisurface.ErrManagedRuntimeSourceFetchFailed) {
		t.Fatalf("PullModel error = %v, want source fetch failure", err)
	}
	recorder.assertContainsMetric(t, modelPullMetricFailure, map[string]string{
		"model_name":   "OMNIVOICE_Q4_K_M",
		"pull_outcome": "SOURCE_FETCH_FAILED",
	})
	recorder.assertContainsMetric(t, modelPullMetricSourceFailure, map[string]string{
		"model_name": "OMNIVOICE_Q4_K_M",
	})
}

type capturingModelPullMetricsRecorder struct {
	mu      sync.Mutex
	metrics []InvocationMetric
}

func (r *capturingModelPullMetricsRecorder) RecordModelPullMetric(metric InvocationMetric) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.metrics = append(r.metrics, metric)
}

func (r *capturingModelPullMetricsRecorder) assertContainsMetric(t *testing.T, name string, labels map[string]string) {
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

type managedPullMetricsAssetPuller struct {
	result     apisurface.ModelPullResult
	inspection localmodels.RuntimeCacheInspection
	cache      localmodels.CacheLayout
	err        error
}

func (p *managedPullMetricsAssetPuller) PullModel(context.Context, *factoryconfig.LoadedFactoryConfig, string) (apisurface.ModelPullResult, error) {
	return p.result, p.err
}

func (p *managedPullMetricsAssetPuller) EnsureModelAvailable(context.Context, *factoryconfig.LoadedFactoryConfig, *interfaces.WorkerConfig) error {
	return nil
}

func (p *managedPullMetricsAssetPuller) ResolveModelCache(context.Context, *factoryconfig.LoadedFactoryConfig, *interfaces.WorkerConfig) (localmodels.CacheLayout, error) {
	return p.cache, nil
}

func (p *managedPullMetricsAssetPuller) InspectRuntimeCache(context.Context, *factoryconfig.LoadedFactoryConfig, string) (localmodels.RuntimeCacheInspection, error) {
	return p.inspection, nil
}
