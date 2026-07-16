package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	factory_context "github.com/portpowered/infinite-you/pkg/factory/context"
	"github.com/portpowered/infinite-you/pkg/factory/runtime"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/logging"
	modelhost "github.com/portpowered/infinite-you/pkg/models/host"
	localmodels "github.com/portpowered/infinite-you/pkg/models/local"
	modelsservice "github.com/portpowered/infinite-you/pkg/models/service"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/transports/mapping"
	"github.com/portpowered/infinite-you/pkg/workers"
	workerapplication "github.com/portpowered/infinite-you/pkg/workers/application"
	workerexecutor "github.com/portpowered/infinite-you/pkg/workers/executor"
)

func (fs *FactoryService) requireModelService() apisurface.ModelAPI {
	if fs == nil || fs.modelService == nil {
		return unavailableModelService{}
	}
	return fs.modelService
}

func (fs *FactoryService) ListModels(ctx context.Context) (factoryapi.ListModelsResponse, error) {
	return fs.requireModelService().ListModels(ctx)
}

func (fs *FactoryService) GetModel(ctx context.Context, modelName string) (factoryapi.ModelDetail, error) {
	return fs.requireModelService().GetModel(ctx, modelName)
}

type modelAssetPuller = localmodels.AssetPuller

func (fs *FactoryService) PullModel(ctx context.Context, modelName string) (apisurface.ModelPullResult, error) {
	return fs.requireModelService().PullModel(ctx, modelName)
}

func (fs *FactoryService) InvokeModel(ctx context.Context, modelName string, request factoryapi.ModelInvocationRequest) (apisurface.ModelInvocationResult, error) {
	return fs.requireModelService().InvokeModel(ctx, modelName, request)
}

var errModelServiceUnavailable = errors.New("model service is not attached to the factory service")

type unavailableModelService struct{}

func (unavailableModelService) ListModels(context.Context) (factoryapi.ListModelsResponse, error) {
	return factoryapi.ListModelsResponse{}, errModelServiceUnavailable
}

func (unavailableModelService) GetModel(context.Context, string) (factoryapi.ModelDetail, error) {
	return factoryapi.ModelDetail{}, errModelServiceUnavailable
}

func (unavailableModelService) PullModel(context.Context, string) (apisurface.ModelPullResult, error) {
	return apisurface.ModelPullResult{}, errModelServiceUnavailable
}

func (unavailableModelService) InvokeModel(context.Context, string, factoryapi.ModelInvocationRequest) (apisurface.ModelInvocationResult, error) {
	return apisurface.ModelInvocationResult{}, errModelServiceUnavailable
}

// CurrentModelRuntimeConfig returns the active runtime configuration used by
// the model service. The lookup remains dynamic across Current Factory changes.
func (fs *FactoryService) CurrentModelRuntimeConfig() *factoryconfig.LoadedFactoryConfig {
	return fs.currentRuntimeConfig()
}

// BuildModelInvocationExecutor adapts the compatibility service shell to the
// explicit model-service invocation boundary assembled by pkg/wire.
func (fs *FactoryService) BuildModelInvocationExecutor(
	runtimeCfg *factoryconfig.LoadedFactoryConfig,
	factoryCfg *interfaces.FactoryConfig,
	workerName string,
) (workers.WorkstationRequestExecutor, error) {
	return fs.modelInvocationExecutor(runtimeCfg, factoryCfg, workerName)
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

func modelPullMetricsRecorderForService(recorder ModelPullMetricsRecorder) modelsservice.PullMetricsRecorder {
	if recorder == nil {
		return nil
	}
	return modelPullMetricsHostAdapter{inner: recorder}
}

// ModelServiceDependencies adapts a built compatibility shell to the canonical
// model-package construction contract without constructing the service.
func ModelServiceDependencies(shell FactoryServiceShell) (modelsservice.Dependencies, error) {
	if shell.Service == nil {
		return modelsservice.Dependencies{}, fmt.Errorf("construct model service: factory service shell is required")
	}
	fs := shell.Service
	var now func() time.Time
	if fs.clock != nil {
		now = fs.clock.Now
	}
	return modelsservice.Dependencies{
		RuntimeConfig:           fs.currentRuntimeConfig,
		ModelHost:               fs.modelHost(),
		ModelAssetPuller:        fs.modelAssets,
		Logger:                  fs.logger,
		Clock:                   now,
		ModelPullMetrics:        modelPullMetricsRecorderForService(fs.modelPullMetricsRecorder()),
		ModelInvocationExecutor: fs.modelInvocationExecutor,
		FactoryRunnerID:         fs.factoryRunnerID(),
	}, nil
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

func (fs *FactoryService) modelHost() modelhost.Host {
	if fs == nil {
		return nil
	}
	if core := fs.core; core != nil {
		if host := core.ModelHost(); host != nil {
			return host
		}
	}
	if bundle := fs.currentRuntimeBundle(); bundle != nil && bundle.ModelHost != nil {
		return bundle.ModelHost
	}
	return nil
}

func cloneMetricLabels(labels map[string]string) map[string]string {
	if len(labels) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(labels))
	for key, value := range labels {
		cloned[key] = value
	}
	return cloned
}

func (fs *FactoryService) modelInvocationExecutor(runtimeCfg *factoryconfig.LoadedFactoryConfig, factoryCfg *interfaces.FactoryConfig, workerName string) (workers.WorkstationRequestExecutor, error) {
	if runtimeCfg == nil || factoryCfg == nil {
		return nil, fmt.Errorf("runtime config is required")
	}
	logger := logging.NewZapLogger(fs.logger, fs != nil && fs.coordinatorPolicy().verbose)
	bundle := fs.currentRuntimeBundle()
	var modelDomain localModelDomain
	var workflowContext *factory_context.FactoryContext
	if bundle != nil {
		modelDomain = LocalModelDomain{
			Resources:      bundle.ModelResources,
			Assets:         bundle.ModelAssets,
			Runtime:        bundle.LocalModelRuntime,
			Host:           bundle.ModelHost,
			Manager:        bundle.LocalModels,
			LeaseExecution: bundle.LeaseExecution,
		}
		workflowContext = runtime.WorkflowContext(bundle.Factory)
	}
	workerApplication, err := fs.workerApplication()
	if err != nil {
		return nil, err
	}
	executor, err := buildWorkerExecutor(
		runtimeCfg,
		factoryCfg,
		workerName,
		fs.factoryRunnerID(),
		workflowContext,
		logger,
		fs.invocationSkipPermissionsOverride(),
		fs.providerOverride(),
		nil,
		workerApplication,
		nil,
		nil,
		nil,
		nil,
		time.Now,
		modelDomain,
	)
	if err != nil {
		return nil, fmt.Errorf("construct model worker %q: %w", workerName, err)
	}
	workstationExecutor, ok := executor.(*workerexecutor.WorkstationExecutor)
	if !ok || workstationExecutor.Executor == nil {
		return nil, fmt.Errorf("model worker %q does not support direct invocation", workerName)
	}
	return workstationExecutor.Executor, nil
}

func (fs *FactoryService) workerApplication() (workerapplication.Components, error) {
	if fs != nil && fs.cfg != nil && fs.cfg.WorkerApplication.Valid() {
		return fs.cfg.WorkerApplication, nil
	}
	return workerapplication.Components{}, fmt.Errorf("factory service worker application is required")
}

func (fs *FactoryService) factoryRunnerID() string {
	if fs == nil {
		return ""
	}
	return fs.coordinatorPolicy().runnerID
}

func (fs *FactoryService) providerOverride() workers.Provider {
	if fs == nil {
		return nil
	}
	return fs.coordinatorPolicy().providerOverride
}

func (fs *FactoryService) invocationSkipPermissionsOverride() *bool {
	if fs == nil || fs.cfg == nil {
		return nil
	}
	return fs.cfg.InvocationSkipPermissionsOverride
}

func stringValue[T ~string](value *T) string {
	if value == nil {
		return ""
	}
	return string(*value)
}
