package service

import (
	"context"
	"fmt"
	"strings"

	workerrunner "github.com/portpowered/infinite-you/pkg/workers/runner"

	factorytoken "github.com/portpowered/infinite-you/pkg/factory/token"
	workerconfig "github.com/portpowered/infinite-you/pkg/workers/config"

	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"

	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"github.com/portpowered/infinite-you/pkg/work"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"

	"go.uber.org/zap"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	modelhost "github.com/portpowered/infinite-you/pkg/models/host"
	localmodels "github.com/portpowered/infinite-you/pkg/models/local"
	contentcontract "github.com/portpowered/infinite-you/pkg/transports/mapping/workcontent"
	workerinferencemapping "github.com/portpowered/infinite-you/pkg/transports/mapping/workerinference"
	"github.com/portpowered/infinite-you/pkg/workers"
	workerinference "github.com/portpowered/infinite-you/pkg/workers/inference"
)

const directModelInvocationTransitionID = "direct-model-invocation"

// ModelInvocationExecutor builds one workstation executor for direct model invocation.
type ModelInvocationExecutor func(
	runtimeCfg *factoryconfig.LoadedFactoryConfig,
	factoryCfg *interfaces.FactoryConfig,
	workerName string,
) (workers.WorkstationRequestExecutor, error)

// InvokeModel executes one direct model invocation through the configured worker path.
func (s *Service) InvokeModel(
	ctx context.Context,
	modelName string,
	request factoryapi.ModelInvocationRequest,
) (apisurface.ModelInvocationResult, error) {
	runtimeCfg := s.runtimeConfig()
	if runtimeCfg == nil {
		return apisurface.ModelInvocationResult{}, fmt.Errorf("factory service runtime is not available")
	}
	factoryCfg := runtimeCfg.FactoryConfig()
	if factoryCfg == nil {
		return apisurface.ModelInvocationResult{}, fmt.Errorf("factory config is not available")
	}

	workerDef, operation, err := localmodels.SelectInvocationWorker(runtimeCfg, modelName, request.Operation)
	failureContext := apisurface.InferenceFailureContext{
		ModelName: strings.TrimSpace(modelName),
		Operation: strings.TrimSpace(request.Operation),
	}
	if err != nil {
		if failure, ok := apisurface.ClassifyInferenceFailure(err, failureContext); ok {
			return apisurface.ModelInvocationResult{}, failure
		}
		return apisurface.ModelInvocationResult{}, err
	}
	failureContext.WorkerName = workerDef.Name
	failureContext.ModelName = workerDef.Model
	managed, readinessErr := s.ensureInvocationReady(ctx, runtimeCfg, workerDef.Model)
	s.recordManagedRuntimeInvocationReadiness(modelName, managed, readinessErr)
	if readinessErr != nil {
		if failure, ok := apisurface.ClassifyInferenceFailure(readinessErr, failureContext); ok {
			return apisurface.ModelInvocationResult{}, failure
		}
		return apisurface.ModelInvocationResult{}, readinessErr
	}

	inputContent := contentcontract.PartsFromGenerated(request.Content)
	inputTokens := []factorytoken.Token{{
		ID: "direct-model-invocation-input",
		Color: factorytoken.Color{
			Content: inputContent,
		},
	}}
	workstationDef := workerinference.DirectInferenceWorkstationConfig(
		request.Operation,
		workerinferencemapping.OperationBindingsFromGenerated(request.Bindings),
	)
	resolvedBindings, err := workerinference.ResolveInferenceOperationBindings(workstationDef, workerDef, inputTokens)
	if err != nil {
		return apisurface.ModelInvocationResult{}, err
	}

	executor, err := s.modelInvocationExecutor(runtimeCfg, factoryCfg, workerDef.Name)
	if err != nil {
		return apisurface.ModelInvocationResult{}, err
	}
	workstationRequest := directModelInvocationWorkstationRequest(workerDef, request, inputTokens, resolvedBindings, s.factoryRunnerID())

	result, err := executor.Execute(ctx, workstationRequest)
	if err != nil {
		if failure, ok := apisurface.ClassifyInferenceFailure(err, failureContext); ok {
			return apisurface.ModelInvocationResult{}, failure
		}
		return apisurface.ModelInvocationResult{}, err
	}
	if result.Outcome == workerexecution.OutcomeFailed {
		if failure, ok := apisurface.ClassifyInferenceWorkResultFailure(result, failureContext); ok {
			return apisurface.ModelInvocationResult{}, failure
		}
		return apisurface.ModelInvocationResult{}, fmt.Errorf("provider execution failed: %s", strings.TrimSpace(result.Error))
	}

	outputContent, err := workerinference.WorkContentFromInferenceOutput(result.Output, operation)
	if err != nil {
		return apisurface.ModelInvocationResult{}, err
	}
	streamFile, streamContentType, err := directModelInvocationStream(outputContent, request.Options)
	if err != nil {
		return apisurface.ModelInvocationResult{}, err
	}

	return apisurface.ModelInvocationResult{
		ModelName:         workerDef.Model,
		Worker:            workerDef.Name,
		Operation:         strings.TrimSpace(request.Operation),
		ProviderLocality:  workerDef.ModelLocality,
		Content:           outputContent,
		Bindings:          workerexecution.CloneResolvedModelOperationBindings(resolvedBindings),
		StreamFile:        streamFile,
		StreamContentType: streamContentType,
	}, nil
}

func (s *Service) ensureInvocationReady(
	ctx context.Context,
	runtimeCfg *factoryconfig.LoadedFactoryConfig,
	modelName string,
) (factoryapi.ManagedRuntime, error) {
	if runtimeCfg == nil {
		return factoryapi.ManagedRuntime{}, fmt.Errorf("runtime config is not available")
	}
	host := s.modelHost()
	if host == nil {
		return localmodels.EnsureManagedRuntimeReadyForInvocation(runtimeCfg, modelName, catalogDiscoveryOptions())
	}
	snapshot, err := host.InspectReadiness(ctx, runtimeCfg, modelName)
	if err != nil {
		return factoryapi.ManagedRuntime{}, err
	}
	managed := apisurface.ManagedRuntimeToAPI(modelhost.ManagedRuntimeFromSnapshot(snapshot))
	if invocationErr := apisurface.InvocationErrorFromManagedRuntime(managed); invocationErr != nil {
		return managed, invocationErr
	}
	return managed, nil
}

func directModelInvocationWorkstationRequest(
	workerDef *workerconfig.Config,
	request factoryapi.ModelInvocationRequest,
	inputTokens []factorytoken.Token,
	resolvedBindings []workerexecution.ResolvedModelOperationBinding,
	factoryRunnerID string,
) workerexecution.WorkstationExecutionRequest {
	selection := workerrunner.ResolveRunnerSelection("", factoryRunnerID, workerDef.ModelProvider)
	inputContent := contentcontract.PartsFromGenerated(request.Content)
	return workerexecution.WorkstationExecutionRequest{
		Dispatch: work.WorkDispatch{
			DispatchID:      directModelInvocationTransitionID,
			TransitionID:    directModelInvocationTransitionID,
			WorkerType:      workerDef.Name,
			WorkstationName: directModelInvocationTransitionID,
			InputTokens:     workers.InputTokens(inputTokens...),
		},
		WorkerType:            workerDef.Name,
		WorkstationType:       directModelInvocationTransitionID,
		RunnerID:              selection.RunnerID,
		RunnerSelectionSource: selection.Source,
		InputTokens:           workers.InputTokens(inputTokens...),
		ModelOperation:        strings.TrimSpace(request.Operation),
		ModelBindings:         resolvedBindings,
		SystemPrompt:          workerDef.Body,
		UserMessage:           workerinference.InferenceOperationUserMessage(request.Operation, inputContent, resolvedBindings),
	}
}

func directModelInvocationStream(content []work.WorkContentPart, options *factoryapi.ModelInvocationOptions) (string, string, error) {
	if options == nil || options.ResponseMode == nil || *options.ResponseMode != factoryapi.AUDIOSTREAM {
		return "", "", nil
	}
	for _, part := range content {
		if part.Type.Normalized() != work.WorkContentPartTypeAudio || strings.TrimSpace(part.File) == "" {
			continue
		}
		contentType := strings.TrimSpace(part.ContentType)
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		return part.File, contentType, nil
	}
	return "", "", fmt.Errorf("%w: invocation did not produce audio output", apisurface.ErrModelInvocationUnsupportedMode)
}

func (s *Service) modelInvocationExecutor(
	runtimeCfg *factoryconfig.LoadedFactoryConfig,
	factoryCfg *interfaces.FactoryConfig,
	workerName string,
) (workers.WorkstationRequestExecutor, error) {
	if s == nil || s.deps.ModelInvocationExecutor == nil {
		return nil, fmt.Errorf("model invocation executor is not configured")
	}
	return s.deps.ModelInvocationExecutor(runtimeCfg, factoryCfg, workerName)
}

func (s *Service) factoryRunnerID() string {
	if s == nil {
		return ""
	}
	return s.deps.FactoryRunnerID
}

func (s *Service) recordManagedRuntimeInvocationReadiness(
	modelName string,
	managed factoryapi.ManagedRuntime,
	err error,
) {
	logger := s.logger()
	if logger == nil {
		return
	}
	fields := []zap.Field{
		zap.String("model_name", modelName),
		zap.String("managed_runtime_identity", managed.Identity),
		zap.String("readiness_state", string(managed.ReadinessState)),
		zap.String("lifecycle_state", string(managed.LifecycleState)),
	}
	if err != nil {
		logger.Warn("managed runtime invocation blocked", append(fields, zap.Error(err))...)
		return
	}
	logger.Info("managed runtime invocation readiness satisfied", fields...)
}
