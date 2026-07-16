package runtimehost

import (
	"context"
	"errors"
	"fmt"
	"time"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	factory_context "github.com/portpowered/infinite-you/pkg/factory/context"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	"github.com/portpowered/infinite-you/pkg/factory/runtime"
	modelhost "github.com/portpowered/infinite-you/pkg/models/host"
	localmodels "github.com/portpowered/infinite-you/pkg/models/local"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"github.com/portpowered/infinite-you/pkg/workers"
	workerexecutor "github.com/portpowered/infinite-you/pkg/workers/executor"
	"github.com/portpowered/infinite-you/pkg/workers/skippermissions"
)

func (fs *Host) requireModelService() apisurface.ModelAPI {
	if fs == nil || fs.modelService == nil {
		return unavailableModelService{}
	}
	return fs.modelService
}

func (fs *Host) ListModels(ctx context.Context) (factoryapi.ListModelsResponse, error) {
	return fs.requireModelService().ListModels(ctx)
}

func (fs *Host) GetModel(ctx context.Context, modelName string) (factoryapi.ModelDetail, error) {
	return fs.requireModelService().GetModel(ctx, modelName)
}

type modelAssetPuller = localmodels.AssetPuller

func (fs *Host) PullModel(ctx context.Context, modelName string) (apisurface.ModelPullResult, error) {
	return fs.requireModelService().PullModel(ctx, modelName)
}

func (fs *Host) InvokeModel(ctx context.Context, modelName string, request factoryapi.ModelInvocationRequest) (apisurface.ModelInvocationResult, error) {
	return fs.requireModelService().InvokeModel(ctx, modelName, request)
}

var errModelServiceUnavailable = errors.New("model service is not attached to the runtime host")

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

// ModelService returns the canonical model-domain collaborator used by the
// compatibility methods on Host.
func (fs *Host) ModelService() apisurface.ModelAPI {
	return fs.requireModelService()
}

// CurrentModelRuntimeConfig returns the active runtime configuration used by
// model catalog and invocation operations. The callback remains dynamic so a
// factory switch is visible without reconstructing the model service.
func (fs *Host) CurrentModelRuntimeConfig() *factoryconfig.LoadedFactoryConfig {
	return fs.currentRuntimeConfig()
}

// BuildModelInvocationExecutor constructs the worker execution collaborator
// used by one direct model invocation.
func (fs *Host) BuildModelInvocationExecutor(
	runtimeCfg *factoryconfig.LoadedFactoryConfig,
	factoryCfg *interfaces.FactoryConfig,
	workerName string,
) (workers.WorkstationRequestExecutor, error) {
	return fs.modelInvocationExecutor(runtimeCfg, factoryCfg, workerName)
}

func (fs *Host) modelHost() modelhost.Host {
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

func (fs *Host) modelInvocationExecutor(runtimeCfg *factoryconfig.LoadedFactoryConfig, factoryCfg *interfaces.FactoryConfig, workerName string) (workers.WorkstationRequestExecutor, error) {
	if runtimeCfg == nil || factoryCfg == nil {
		return nil, fmt.Errorf("runtime config is required")
	}
	logger := logging.NewZapLogger(fs.logger, fs != nil && fs.coordinatorPolicy().verbose)
	workerDef, ok := runtimeCfg.Worker(workerName)
	if !ok || workerDef == nil {
		return nil, fmt.Errorf("worker %q is not configured", workerName)
	}
	if err := skippermissions.ValidateInvocationSkipPermissionsForWorker(workerDef, fs.invocationSkipPermissionsOverride()); err != nil {
		return nil, fmt.Errorf("worker %q: %w", workerName, err)
	}
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
	if fs == nil || fs.cfg == nil || !fs.cfg.WorkerApplication.Valid() {
		return nil, fmt.Errorf("runtime host worker application is required")
	}
	executor := buildWorkerExecutor(
		runtimeCfg,
		factoryCfg,
		workerName,
		fs.factoryRunnerID(),
		workflowContext,
		logger,
		fs.invocationSkipPermissionsOverride(),
		fs.providerOverride(),
		nil,
		fs.cfg.WorkerApplication.ProviderCommandRunner,
		fs.cfg.WorkerApplication.ScriptCommandRunner,
		nil,
		nil,
		nil,
		nil,
		time.Now,
		modelDomain,
	)
	workstationExecutor, ok := executor.(*workerexecutor.WorkstationExecutor)
	if !ok || workstationExecutor.Executor == nil {
		return nil, fmt.Errorf("model worker %q does not support direct invocation", workerName)
	}
	return workstationExecutor.Executor, nil
}

func (fs *Host) factoryRunnerID() string {
	if fs == nil {
		return ""
	}
	return fs.coordinatorPolicy().runnerID
}

func (fs *Host) providerOverride() workers.Provider {
	if fs == nil {
		return nil
	}
	return fs.coordinatorPolicy().providerOverride
}

func (fs *Host) invocationSkipPermissionsOverride() *bool {
	if fs == nil || fs.cfg == nil {
		return nil
	}
	return fs.cfg.InvocationSkipPermissionsOverride
}
