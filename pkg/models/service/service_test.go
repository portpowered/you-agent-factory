package service

import (
	"context"
	"testing"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/localmodels"
	"github.com/portpowered/infinite-you/pkg/modelhost"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/workers"
	"go.uber.org/zap"
)

func TestNewRetainsExplicitDependencies(t *testing.T) {
	t.Parallel()

	runtimeCfg := mustConstructionRuntimeConfig(t)
	puller := localmodels.NewAssetPuller(t.TempDir())
	logger := zap.NewNop()
	metrics := &constructionPullMetrics{}
	executor := func(
		*factoryconfig.LoadedFactoryConfig,
		*interfaces.FactoryConfig,
		string,
	) (workers.WorkstationRequestExecutor, error) {
		return nil, nil
	}

	svc := New(Dependencies{
		RuntimeConfig:           func() *factoryconfig.LoadedFactoryConfig { return runtimeCfg },
		ModelHost:               func() modelhost.Host { return nil },
		ModelAssetPuller:        func() localmodels.AssetPuller { return puller },
		Logger:                  func() *zap.Logger { return logger },
		ModelPullMetrics:        func() PullMetricsRecorder { return metrics },
		ModelInvocationExecutor: executor,
		FactoryRunnerID:         func() string { return "runner-a" },
	})

	if svc.runtimeConfig() != runtimeCfg {
		t.Fatal("runtime config accessor did not return the supplied config")
	}
	if svc.modelAssetPuller() != puller {
		t.Fatal("asset puller accessor did not return the supplied puller")
	}
	if svc.logger() != logger {
		t.Fatal("logger accessor did not return the supplied logger")
	}
	if svc.deps.ModelPullMetrics() != metrics {
		t.Fatal("metrics accessor did not return the supplied recorder")
	}
	if svc.deps.ModelInvocationExecutor == nil {
		t.Fatal("invocation executor was not retained")
	}
	if got := svc.factoryRunnerID(); got != "runner-a" {
		t.Fatalf("factory runner ID = %q, want runner-a", got)
	}
}

func TestNewAllowsOptionalDependenciesToBeOmitted(t *testing.T) {
	t.Parallel()

	runtimeCfg := mustConstructionRuntimeConfig(t)
	svc := New(Dependencies{
		RuntimeConfig: func() *factoryconfig.LoadedFactoryConfig { return runtimeCfg },
	})

	models, err := svc.ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels with optional dependencies omitted: %v", err)
	}
	if models.Results == nil {
		t.Fatal("ListModels results = nil, want initialized empty results")
	}
	if svc.logger() != nil {
		t.Fatal("logger = non-nil, want nil when omitted")
	}
	if svc.factoryRunnerID() != "" {
		t.Fatal("factory runner ID = non-empty, want empty when omitted")
	}
	if svc.modelAssetPuller() == nil {
		t.Fatal("asset puller = nil, want nil-safe default puller")
	}
}

func TestNewMissingRuntimeDependencyReturnsErrorWithoutPanic(t *testing.T) {
	t.Parallel()

	svc := New(Dependencies{})
	if svc == nil {
		t.Fatal("New returned nil")
	}
	if _, err := svc.ListModels(context.Background()); err == nil {
		t.Fatal("ListModels error = nil, want unavailable runtime error")
	}
	if _, err := svc.InvokeModel(context.Background(), "missing", modelInvocationRequest()); err == nil {
		t.Fatal("InvokeModel error = nil, want unavailable runtime error")
	}
}

type constructionPullMetrics struct{}

func (*constructionPullMetrics) RecordModelPullMetric(PullMetric) {}

func mustConstructionRuntimeConfig(t *testing.T) *factoryconfig.LoadedFactoryConfig {
	t.Helper()
	runtimeCfg, err := factoryconfig.NewLoadedFactoryConfig(t.TempDir(), &interfaces.FactoryConfig{
		Name: "construction-test",
	}, nil)
	if err != nil {
		t.Fatalf("NewLoadedFactoryConfig: %v", err)
	}
	return runtimeCfg
}

func modelInvocationRequest() factoryapi.ModelInvocationRequest {
	return factoryapi.ModelInvocationRequest{Operation: "TTS"}
}
