package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	modelhost "github.com/portpowered/infinite-you/pkg/models/host"
	localmodels "github.com/portpowered/infinite-you/pkg/models/local"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/workers"
	"go.uber.org/zap"
)

func TestNewServiceRetainsExplicitDependencies(t *testing.T) {
	t.Parallel()

	runtimeCfg := mustConstructionRuntimeConfig(t)
	puller := localmodels.NewAssetPuller(t.TempDir())
	logger := zap.NewNop()
	metrics := &constructionPullMetrics{}
	executor := constructionInvocationExecutor()
	host := constructionModelHost{}

	svc, err := NewService(Dependencies{
		RuntimeConfig:           func() *factoryconfig.LoadedFactoryConfig { return runtimeCfg },
		ModelHost:               host,
		ModelAssetPuller:        puller,
		Logger:                  logger,
		Clock:                   func() time.Time { return time.Unix(123, 0) },
		ModelPullMetrics:        metrics,
		ModelInvocationExecutor: executor,
		FactoryRunnerID:         "runner-a",
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	if svc.runtimeConfig() != runtimeCfg || svc.modelHost() != host || svc.modelAssetPuller() != puller {
		t.Fatal("NewService did not retain required dependencies")
	}
	if svc.logger() != logger || svc.deps.ModelPullMetrics != metrics || svc.deps.ModelInvocationExecutor == nil {
		t.Fatal("NewService did not retain optional or invocation dependencies")
	}
	if got := svc.factoryRunnerID(); got != "runner-a" {
		t.Fatalf("factory runner ID = %q, want runner-a", got)
	}
}

func TestNewServiceAppliesModelLocalDefaults(t *testing.T) {
	t.Parallel()

	runtimeCfg := mustConstructionRuntimeConfig(t)
	svc, err := NewService(Dependencies{
		RuntimeConfig:           func() *factoryconfig.LoadedFactoryConfig { return runtimeCfg },
		ModelHost:               constructionModelHost{},
		ModelAssetPuller:        localmodels.NewAssetPuller(t.TempDir()),
		ModelInvocationExecutor: constructionInvocationExecutor(),
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	if svc.logger() == nil || svc.deps.Clock == nil || svc.deps.ModelPullMetrics == nil {
		t.Fatal("NewService did not apply logger, clock, and metrics defaults")
	}
	if got := svc.now(); got.IsZero() {
		t.Fatal("default clock returned the zero time")
	}
	// Exercise the no-op metrics default through the same emission boundary used
	// by pull operations. Omitted optional collaborators must remain usable, not
	// merely non-nil.
	svc.recordModelPullMetric(modelPullMetricAttempts, map[string]string{"model": "test"})
}

func TestServiceNilReceiverPreservesUnavailableRuntimeErrors(t *testing.T) {
	t.Parallel()

	var svc *Service
	assertUnavailable := func(operation string, err error) {
		t.Helper()
		if err == nil || !strings.Contains(err.Error(), "runtime is not available") {
			t.Fatalf("%s error = %v, want runtime unavailable", operation, err)
		}
	}

	_, err := svc.ListModels(context.Background())
	assertUnavailable("ListModels", err)
	_, err = svc.GetModel(context.Background(), "OMNIVOICE_Q4_K_M")
	assertUnavailable("GetModel", err)
	_, err = svc.PullModel(context.Background(), "OMNIVOICE_Q4_K_M")
	assertUnavailable("PullModel", err)
	_, err = svc.InvokeModel(context.Background(), "OMNIVOICE_Q4_K_M", factoryapi.ModelInvocationRequest{Operation: "TTS"})
	assertUnavailable("InvokeModel", err)
	if _, err := svc.modelInvocationExecutor(nil, nil, "worker"); err == nil {
		t.Fatal("nil service modelInvocationExecutor() succeeded")
	}
	if svc.factoryRunnerID() != "" || svc.logger() != nil || svc.modelHost() != nil {
		t.Fatal("nil service accessors returned configured collaborators")
	}
}

func TestNewServiceRejectsMissingRequiredDependencies(t *testing.T) {
	t.Parallel()

	runtimeCfg := mustConstructionRuntimeConfig(t)
	valid := Dependencies{
		RuntimeConfig:           func() *factoryconfig.LoadedFactoryConfig { return runtimeCfg },
		ModelHost:               constructionModelHost{},
		ModelAssetPuller:        localmodels.NewAssetPuller(t.TempDir()),
		ModelInvocationExecutor: constructionInvocationExecutor(),
	}
	tests := []struct {
		name       string
		dependency string
		mutate     func(*Dependencies)
	}{
		{name: "runtime lookup", dependency: "runtime configuration lookup", mutate: func(deps *Dependencies) { deps.RuntimeConfig = nil }},
		{name: "runtime value", dependency: "runtime configuration", mutate: func(deps *Dependencies) {
			deps.RuntimeConfig = func() *factoryconfig.LoadedFactoryConfig { return nil }
		}},
		{name: "model host", dependency: "model host", mutate: func(deps *Dependencies) { deps.ModelHost = nil }},
		{name: "typed nil model host", dependency: "model host", mutate: func(deps *Dependencies) { var host *constructionModelHost; deps.ModelHost = host }},
		{name: "asset puller", dependency: "model asset puller", mutate: func(deps *Dependencies) { deps.ModelAssetPuller = nil }},
		{name: "invocation executor", dependency: "model invocation executor", mutate: func(deps *Dependencies) { deps.ModelInvocationExecutor = nil }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			deps := valid
			test.mutate(&deps)
			svc, err := NewService(deps)
			if svc != nil {
				t.Fatalf("NewService service = %#v, want nil", svc)
			}
			if !errors.Is(err, ErrInvalidDependencies) || !strings.Contains(err.Error(), test.dependency) {
				t.Fatalf("NewService error = %v, want invalid-dependencies error naming %q", err, test.dependency)
			}
		})
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

func constructionInvocationExecutor() ModelInvocationExecutor {
	return func(
		*factoryconfig.LoadedFactoryConfig,
		*interfaces.FactoryConfig,
		string,
	) (workers.WorkstationRequestExecutor, error) {
		return nil, nil
	}
}

type constructionModelHost struct{}

func (constructionModelHost) ResolveIdentity(context.Context, *factoryconfig.LoadedFactoryConfig, string) (modelhost.Identity, error) {
	return modelhost.Identity{}, nil
}

func (constructionModelHost) InspectReadiness(context.Context, *factoryconfig.LoadedFactoryConfig, string) (modelhost.ReadinessSnapshot, error) {
	return modelhost.ReadinessSnapshot{}, nil
}

func (constructionModelHost) Pull(context.Context, *factoryconfig.LoadedFactoryConfig, string) (modelhost.PullSnapshot, error) {
	return modelhost.PullSnapshot{}, nil
}

func (constructionModelHost) AcquireLease(context.Context, *factoryconfig.LoadedFactoryConfig, string, modelhost.LeaseOptions) (modelhost.Lease, error) {
	return modelhost.Lease{}, nil
}

func (constructionModelHost) ReleaseLease(context.Context, string) error { return nil }

func (constructionModelHost) Unload(context.Context, *factoryconfig.LoadedFactoryConfig, string) error {
	return nil
}
