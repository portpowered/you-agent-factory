package internal

import (
	"context"
	"fmt"
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	modelinference "github.com/portpowered/infinite-you/pkg/services/models"

	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"

	"github.com/portpowered/infinite-you/pkg/services/work"

	"go.uber.org/zap"

	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerinference "github.com/portpowered/infinite-you/pkg/services/workers/services/inference"
)

const directModelInvocationTransitionID = "direct-model-invocation"

func selectInvocationWorker(
	runtimeCfg interfaces.RuntimeConfigLookup,
	modelName string,
	operationName string,
) (*interfaces.FactoryWorkerConfig, interfaces.ModelOperation, error) {
	if runtimeCfg == nil || runtimeCfg.FactoryConfig() == nil {
		return nil, interfaces.ModelOperation{}, fmt.Errorf("runtime config is not available")
	}
	modelKey := canonicalModelName(modelName)
	operationName = strings.TrimSpace(operationName)
	if modelKey == "" {
		return nil, interfaces.ModelOperation{}, fmt.Errorf("%w: empty model name", modelinference.ErrNotFound)
	}
	if operationName == "" {
		return nil, interfaces.ModelOperation{}, fmt.Errorf("operation is required")
	}

	var modelMatched bool
	var matchedWorkerName string
	for _, worker := range runtimeCfg.FactoryConfig().Workers {
		workerDef, ok := runtimeCfg.Worker(worker.Name)
		if !ok || workerDef == nil || !isInferenceWorkerType(workerDef.Type) {
			continue
		}
		if canonicalModelName(workerDef.Model) != modelKey {
			continue
		}
		modelMatched = true
		matchedWorkerName = workerDef.Name
		for _, operation := range workerDef.Operations {
			if strings.TrimSpace(operation.Name) == operationName {
				return workerDef, operation, nil
			}
		}
	}
	if modelMatched {
		return nil, interfaces.ModelOperation{}, fmt.Errorf(
			"%w: worker %q for model %q does not support operation %q",
			modelinference.ErrUnsupportedOperation,
			matchedWorkerName,
			modelName,
			operationName,
		)
	}
	return nil, interfaces.ModelOperation{}, fmt.Errorf("%w: %s", modelinference.ErrNotFound, modelName)
}

func canonicalModelName(value string) string {
	return strings.ToUpper(strings.TrimSpace(value))
}

func isInferenceWorkerType(value string) bool {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case interfaces.WorkerTypeInference, interfaces.WorkerTypeModel:
		return true
	default:
		return false
	}
}

// InvokeModel executes one direct model invocation through the configured worker path.
func (s *Service) InvokeModel(
	ctx context.Context,
	modelName string,
	request modelinference.Request,
) (modelinference.Result, error) {
	failureContext := workers.InferenceFailureContext{
		ModelName: strings.TrimSpace(modelName),
		Operation: strings.TrimSpace(request.Operation),
	}
	runtimeCfg := s.CurrentModelRuntimeConfig()
	if runtimeCfg == nil {
		return modelinference.Result{}, classifyModelInvocationError(fmt.Errorf("factory service runtime is not available"), failureContext)
	}
	factoryCfg := runtimeCfg.FactoryConfig()
	if factoryCfg == nil {
		return modelinference.Result{}, classifyModelInvocationError(fmt.Errorf("factory config is not available"), failureContext)
	}

	workerDef, operation, err := selectInvocationWorker(runtimeCfg, modelName, request.Operation)
	if err != nil {
		return modelinference.Result{}, classifyModelInvocationError(err, failureContext)
	}
	failureContext.WorkerName = workerDef.Name
	failureContext.ModelName = workerDef.Model
	readinessErr := s.ensureAndRecordInvocationReady(
		ctx, runtimeCfg, workerDef.Model, request.Operation,
	)
	if readinessErr != nil {
		return modelinference.Result{}, classifyModelInvocationError(readinessErr, failureContext)
	}

	inputContent := request.Content
	inputTokens := []workerexecution.Token{{
		ID: "direct-model-invocation-input",
		Color: workerexecution.Color{
			Content: inputContent,
		},
	}}
	workstationDef := workerinference.DirectInferenceWorkstationConfig(
		request.Operation,
		factoryDefinitionBindings(request.Bindings),
	)
	resolvedBindings, err := workerinference.ResolveInferenceOperationBindings(workstationDef, workerDef, inputTokens)
	if err != nil {
		return modelinference.Result{}, classifyModelInvocationError(err, failureContext)
	}

	executor, err := s.modelInvocationExecutor(runtimeCfg, factoryCfg, workerDef.Name)
	if err != nil {
		return modelinference.Result{}, classifyModelInvocationError(err, failureContext)
	}
	selection, err := resolveRuntimeRunnerSelection(
		s.providerRegistry,
		"",
		s.factoryRunnerID,
		workerDef.ModelProvider,
	)
	if err != nil {
		return modelinference.Result{}, classifyModelInvocationError(err, failureContext)
	}
	workstationRequest := directModelInvocationWorkstationRequest(
		workerDef,
		request,
		inputTokens,
		resolvedBindings,
		selection,
	)

	result, err := executor.Execute(ctx, workstationRequest)
	if err != nil {
		return modelinference.Result{}, classifyModelInvocationError(err, failureContext)
	}
	if result.Outcome == workerexecution.OutcomeFailed {
		if failure, ok := workers.ClassifyInferenceWorkResultFailure(result, failureContext); ok {
			return modelinference.Result{}, failure
		}
		return modelinference.Result{}, classifyModelInvocationError(fmt.Errorf("provider execution failed: %s", strings.TrimSpace(result.Error)), failureContext)
	}

	outputContent, err := workerinference.WorkContentFromInferenceOutput(result.Output, operation)
	if err != nil {
		return modelinference.Result{}, classifyModelInvocationError(err, failureContext)
	}
	streamFile, streamContentType, err := directModelInvocationStream(outputContent, request.Options)
	if err != nil {
		return modelinference.Result{}, classifyModelInvocationError(err, failureContext)
	}

	return modelinference.Result{
		ModelName:         workerDef.Model,
		Worker:            workerDef.Name,
		Operation:         strings.TrimSpace(request.Operation),
		ProviderLocality:  workerDef.ModelLocality,
		Content:           outputContent,
		Bindings:          modelInvocationBindings(resolvedBindings),
		StreamFile:        streamFile,
		StreamContentType: streamContentType,
	}, nil
}

func classifyModelInvocationError(err error, context workers.InferenceFailureContext) error {
	failure, ok := workers.ClassifyInferenceFailure(err, context)
	if !ok {
		return err
	}
	return failure
}

func factoryDefinitionBindings(
	bindings []modelinference.ModelOperationBinding,
) []interfaces.ModelOperationBinding {
	result := make([]interfaces.ModelOperationBinding, len(bindings))
	for i := range bindings {
		result[i] = interfaces.ModelOperationBinding{
			Slot:           bindings[i].Slot,
			Config:         work.CloneWorkContentParts(bindings[i].Config),
			DefaultContent: work.CloneWorkContentParts(bindings[i].DefaultContent),
		}
		if bindings[i].Selector != nil {
			result[i].Selector = &interfaces.ModelOperationBindingSelector{
				Slot:  bindings[i].Selector.Slot,
				Label: bindings[i].Selector.Label,
				Type:  bindings[i].Selector.Type,
				Role:  bindings[i].Selector.Role,
			}
		}
	}
	return result
}

func modelInvocationBindings(
	bindings []workerexecution.ResolvedModelOperationBinding,
) []modelinference.ResolvedModelOperationBinding {
	if len(bindings) == 0 {
		return nil
	}
	result := make([]modelinference.ResolvedModelOperationBinding, 0, len(bindings))
	for _, binding := range bindings {
		result = append(result, modelinference.ResolvedModelOperationBinding{
			Slot:    binding.Slot,
			Source:  string(binding.Source),
			Content: work.CloneWorkContentParts(binding.Content),
		})
	}
	return result
}

func (s *Service) ensureAndRecordInvocationReady(
	ctx context.Context,
	runtimeCfg interfaces.RuntimeConfigLookup,
	modelName string,
	operation string,
) error {
	managed, err := s.ensureInvocationReady(ctx, runtimeCfg, modelName, operation)
	s.recordManagedRuntimeInvocationReadiness(modelName, managed, err)
	return err
}

func (s *Service) ensureInvocationReady(
	ctx context.Context,
	runtimeCfg interfaces.RuntimeConfigLookup,
	modelName string,
	operation string,
) (modelinference.Runtime, error) {
	if runtimeCfg == nil {
		return modelinference.Runtime{}, fmt.Errorf("runtime config is not available")
	}
	if s == nil || s.models == nil {
		return modelinference.Runtime{}, fmt.Errorf("Models service is not available")
	}
	if s.modelsScope.IsZero() {
		return s.models.InspectRuntime(ctx, modelName)
	}
	readiness, err := s.models.GetModelReadiness(ctx, modelinference.GetModelReadinessRequest{
		Scope:     s.modelsScope,
		Name:      modelName,
		Operation: operation,
	})
	return readiness.Readiness, err
}

func directModelInvocationWorkstationRequest(
	workerDef *interfaces.FactoryWorkerConfig,
	request modelinference.Request,
	inputTokens []workerexecution.Token,
	resolvedBindings []workerexecution.ResolvedModelOperationBinding,
	selection workerexecution.ResolvedRunnerSelection,
) workerexecution.WorkstationExecutionRequest {
	inputContent := request.Content
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

func directModelInvocationStream(content []work.WorkContentPart, options *modelinference.Options) (string, string, error) {
	if options == nil || options.ResponseMode != modelinference.ResponseModeAudioStream {
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
	return "", "", fmt.Errorf("%w: invocation did not produce audio output", modelinference.ErrUnsupportedResponseMode)
}

func (s *Service) recordManagedRuntimeInvocationReadiness(
	modelName string,
	managed modelinference.Runtime,
	err error,
) {
	var logger *zap.Logger
	if s != nil {
		logger = s.logger
	}
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
