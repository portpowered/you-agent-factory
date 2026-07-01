package service

import (
	"context"
	"testing"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/localmodels"
	"github.com/portpowered/infinite-you/pkg/modelhost"
	"go.uber.org/zap"
)

func TestNewFromHost_WiresRuntimeConfigIntoService(t *testing.T) {
	t.Parallel()

	runtimeCfg, err := factoryconfig.NewLoadedFactoryConfig(t.TempDir(), &interfaces.FactoryConfig{
		Name: "factory",
		Workers: []interfaces.WorkerConfig{{
			Name:          "voice-local",
			Type:          interfaces.WorkerTypeModel,
			Model:         "OMNIVOICE_Q4_K_M",
			ModelLocality: interfaces.ModelLocalityLocal,
			Operations:    []interfaces.ModelOperation{{Name: "TTS"}},
		}},
	}, nil)
	if err != nil {
		t.Fatalf("NewLoadedFactoryConfig: %v", err)
	}

	svc := NewFromHost(stubModelServiceHost{runtimeCfg: runtimeCfg})

	models, err := svc.ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if len(models.Results) != 1 || models.Results[0].Name != "OMNIVOICE_Q4_K_M" {
		t.Fatalf("models = %#v, want one OMNIVOICE summary", models.Results)
	}
}

type stubModelServiceHost struct {
	runtimeCfg *factoryconfig.LoadedFactoryConfig
}

func (h stubModelServiceHost) RuntimeConfig() func() *factoryconfig.LoadedFactoryConfig {
	return func() *factoryconfig.LoadedFactoryConfig { return h.runtimeCfg }
}

func (h stubModelServiceHost) ModelHost() func() modelhost.Host {
	return func() modelhost.Host { return nil }
}

func (h stubModelServiceHost) ModelAssetPuller() func() localmodels.AssetPuller {
	return func() localmodels.AssetPuller { return nil }
}

func (h stubModelServiceHost) Logger() func() *zap.Logger {
	return func() *zap.Logger { return nil }
}

func (h stubModelServiceHost) ModelPullMetrics() func() PullMetricsRecorder {
	return func() PullMetricsRecorder { return nil }
}

func (h stubModelServiceHost) ModelInvocationExecutor() ModelInvocationExecutor {
	return nil
}

func (h stubModelServiceHost) FactoryRunnerID() func() string {
	return func() string { return "" }
}
