package service

import (
	"github.com/portpowered/infinite-you/pkg/apisurface"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/localmodels"
	modelsservice "github.com/portpowered/infinite-you/pkg/models/service"
	"github.com/portpowered/infinite-you/pkg/modelhost"
	"go.uber.org/zap"
)

// modelServiceHost adapts FactoryService runtime seams for pkg/models/service wiring.
type modelServiceHost struct {
	*FactoryService
}

var _ modelsservice.Host = modelServiceHost{}

func (h modelServiceHost) RuntimeConfig() func() *factoryconfig.LoadedFactoryConfig {
	if h.FactoryService == nil {
		return func() *factoryconfig.LoadedFactoryConfig { return nil }
	}
	return h.FactoryService.currentRuntimeConfig
}

func (h modelServiceHost) ModelHost() func() modelhost.Host {
	if h.FactoryService == nil {
		return func() modelhost.Host { return nil }
	}
	return h.FactoryService.modelHost
}

func (h modelServiceHost) ModelAssetPuller() func() localmodels.AssetPuller {
	if h.FactoryService == nil {
		return func() localmodels.AssetPuller { return nil }
	}
	return h.FactoryService.modelAssetPuller
}

func (h modelServiceHost) Logger() func() *zap.Logger {
	if h.FactoryService == nil {
		return func() *zap.Logger { return nil }
	}
	return func() *zap.Logger { return h.FactoryService.logger }
}

func (h modelServiceHost) ModelPullMetrics() func() modelsservice.PullMetricsRecorder {
	if h.FactoryService == nil {
		return func() modelsservice.PullMetricsRecorder { return nil }
	}
	return func() modelsservice.PullMetricsRecorder {
		recorder := h.FactoryService.modelPullMetricsRecorder()
		if recorder == nil {
			return nil
		}
		return modelPullMetricsHostAdapter{inner: recorder}
	}
}

func (h modelServiceHost) ModelInvocationExecutor() modelsservice.ModelInvocationExecutor {
	if h.FactoryService == nil {
		return nil
	}
	return h.FactoryService.modelInvocationExecutor
}

func (h modelServiceHost) FactoryRunnerID() func() string {
	if h.FactoryService == nil {
		return func() string { return "" }
	}
	return h.FactoryService.factoryRunnerID
}

type modelPullMetricsHostAdapter struct {
	inner ModelPullMetricsRecorder
}

func (a modelPullMetricsHostAdapter) RecordModelPullMetric(metric modelsservice.PullMetric) {
	a.inner.RecordModelPullMetric(InvocationMetric{
		Name:   metric.Name,
		Labels: metric.Labels,
	})
}

func wireModelServiceCollaborator(fs *FactoryService, cfg *FactoryServiceConfig) apisurface.ModelAPI {
	if cfg != nil && cfg.ModelAPI != nil {
		return cfg.ModelAPI
	}
	return modelsservice.NewFromHost(modelServiceHost{FactoryService: fs})
}

// ProvideModelServiceCollaborator constructs the model-domain collaborator for a
// built FactoryService shell.
func ProvideModelServiceCollaborator(
	shell FactoryServiceShell,
	cfg *FactoryServiceConfig,
) apisurface.ModelAPI {
	return wireModelServiceCollaborator(shell.Service, cfg)
}

// AttachModelServiceCollaborator assigns the model-domain collaborator on the
// service shell and returns the assembled FactoryService.
func AttachModelServiceCollaborator(
	shell FactoryServiceShell,
	modelAPI apisurface.ModelAPI,
) *FactoryService {
	if shell.Service != nil {
		shell.Service.modelService = modelAPI
	}
	return shell.Service
}
