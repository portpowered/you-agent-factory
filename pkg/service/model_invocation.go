package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/logging"
	"github.com/portpowered/infinite-you/pkg/workcontent"
	"github.com/portpowered/infinite-you/pkg/workers"
	workerexecutor "github.com/portpowered/infinite-you/pkg/workers/executor"
)

const directModelInvocationTransitionID = "direct-model-invocation"

func (fs *FactoryService) InvokeModel(ctx context.Context, modelName string, request factoryapi.ModelInvocationRequest) (apisurface.ModelInvocationResult, error) {
	runtimeCfg := fs.currentRuntimeConfig()
	if runtimeCfg == nil {
		return apisurface.ModelInvocationResult{}, fmt.Errorf("factory service runtime is not available")
	}
	factoryCfg := runtimeCfg.FactoryConfig()
	if factoryCfg == nil {
		return apisurface.ModelInvocationResult{}, fmt.Errorf("factory config is not available")
	}

	workerDef, operation, err := selectModelInvocationWorker(runtimeCfg, modelName, request.Operation)
	if err != nil {
		return apisurface.ModelInvocationResult{}, err
	}
	if err := fs.modelAssetPuller().EnsureModelAvailable(ctx, runtimeCfg, workerDef); err != nil {
		return apisurface.ModelInvocationResult{}, err
	}

	inputContent := workcontent.PartsFromGenerated(request.Content)
	inputTokens := []interfaces.Token{{
		ID: "direct-model-invocation-input",
		Color: interfaces.TokenColor{
			Content: inputContent,
		},
	}}
	workstationDef := &interfaces.FactoryWorkstationConfig{
		Type:              interfaces.WorkstationTypeInvoke,
		Operation:         strings.TrimSpace(request.Operation),
		OperationBindings: modelInvocationBindingsFromGenerated(request.Bindings),
	}
	resolvedBindings, err := workerexecutor.ResolveModelOperationBindings(workstationDef, workerDef, inputTokens)
	if err != nil {
		return apisurface.ModelInvocationResult{}, err
	}

	executor, err := fs.modelInvocationExecutor(runtimeCfg, factoryCfg, workerDef.Name)
	if err != nil {
		return apisurface.ModelInvocationResult{}, err
	}
	selection := interfaces.ResolveRunnerSelection("", fs.factoryRunnerID(), workerDef.ModelProvider)
	workstationRequest := interfaces.WorkstationExecutionRequest{
		Dispatch: interfaces.WorkDispatch{
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
		UserMessage:           directModelInvocationUserMessage(request.Operation, inputContent, resolvedBindings),
	}

	result, err := executor.Execute(ctx, workstationRequest)
	if err != nil {
		return apisurface.ModelInvocationResult{}, fmt.Errorf("provider execution failed: %w", err)
	}
	if result.Outcome == interfaces.OutcomeFailed {
		return apisurface.ModelInvocationResult{}, fmt.Errorf("provider execution failed: %s", strings.TrimSpace(result.Error))
	}

	outputContent, err := directModelInvocationOutputContent(result.Output, operation)
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
		Bindings:          interfaces.CloneResolvedModelOperationBindings(resolvedBindings),
		StreamFile:        streamFile,
		StreamContentType: streamContentType,
	}, nil
}

func (fs *FactoryService) modelInvocationExecutor(runtimeCfg *factoryconfig.LoadedFactoryConfig, factoryCfg *interfaces.FactoryConfig, workerName string) (workers.WorkstationRequestExecutor, error) {
	if runtimeCfg == nil || factoryCfg == nil {
		return nil, fmt.Errorf("runtime config is required")
	}
	logger := logging.NewZapLogger(fs.logger, fs.cfg != nil && fs.cfg.Verbose)
	executor := buildWorkerExecutor(
		runtimeCfg,
		factoryCfg,
		workerName,
		fs.factoryRunnerID(),
		logger,
		fs.providerOverride(),
		fs.providerCommandRunnerOverride(),
		fs.commandRunnerOverride(),
		nil,
		nil,
		nil,
		time.Now,
		fs.modelResources,
		fs.localModels,
	)
	workstationExecutor, ok := executor.(*workerexecutor.WorkstationExecutor)
	if !ok || workstationExecutor.Executor == nil {
		return nil, fmt.Errorf("model worker %q does not support direct invocation", workerName)
	}
	return workstationExecutor.Executor, nil
}

func (fs *FactoryService) factoryRunnerID() string {
	if fs == nil || fs.cfg == nil {
		return ""
	}
	return fs.cfg.RunnerID
}

func (fs *FactoryService) providerOverride() workers.Provider {
	if fs == nil || fs.cfg == nil {
		return nil
	}
	return fs.cfg.ProviderOverride
}

func (fs *FactoryService) providerCommandRunnerOverride() workers.CommandRunner {
	if fs == nil || fs.cfg == nil {
		return nil
	}
	return fs.cfg.ProviderCommandRunnerOverride
}

func (fs *FactoryService) commandRunnerOverride() workers.CommandRunner {
	if fs == nil || fs.cfg == nil {
		return nil
	}
	return fs.cfg.CommandRunnerOverride
}

func selectModelInvocationWorker(runtimeCfg *factoryconfig.LoadedFactoryConfig, modelName, operationName string) (*interfaces.WorkerConfig, interfaces.ModelOperation, error) {
	if runtimeCfg == nil || runtimeCfg.FactoryConfig() == nil {
		return nil, interfaces.ModelOperation{}, fmt.Errorf("runtime config is not available")
	}
	modelKey := canonicalModelName(modelName)
	operationName = strings.TrimSpace(operationName)
	if modelKey == "" {
		return nil, interfaces.ModelOperation{}, fmt.Errorf("%w: empty model name", apisurface.ErrModelNotFound)
	}
	if operationName == "" {
		return nil, interfaces.ModelOperation{}, fmt.Errorf("operation is required")
	}

	var modelMatched bool
	for _, worker := range runtimeCfg.FactoryConfig().Workers {
		workerDef, ok := runtimeCfg.Worker(worker.Name)
		if !ok || workerDef == nil || workerDef.Type != interfaces.WorkerTypeModel {
			continue
		}
		if canonicalModelName(workerDef.Model) != modelKey {
			continue
		}
		modelMatched = true
		for _, operation := range workerDef.Operations {
			if strings.TrimSpace(operation.Name) == operationName {
				return workerDef, operation, nil
			}
		}
	}
	if modelMatched {
		return nil, interfaces.ModelOperation{}, fmt.Errorf("%w: model %q does not support operation %q", apisurface.ErrModelInvocationUnsupportedOperation, modelName, operationName)
	}
	return nil, interfaces.ModelOperation{}, fmt.Errorf("%w: %s", apisurface.ErrModelNotFound, modelName)
}

func modelInvocationBindingsFromGenerated(values *[]factoryapi.WorkstationOperationBinding) []interfaces.ModelOperationBinding {
	if values == nil || len(*values) == 0 {
		return nil
	}
	bindings := make([]interfaces.ModelOperationBinding, 0, len(*values))
	for _, binding := range *values {
		current := interfaces.ModelOperationBinding{
			Slot:           strings.TrimSpace(binding.Slot),
			Config:         workcontent.PartsFromGenerated(binding.Config),
			DefaultContent: workcontent.PartsFromGenerated(binding.DefaultContent),
		}
		if binding.Selector != nil {
			current.Selector = &interfaces.ModelOperationBindingSelector{
				Slot:  stringValue(binding.Selector.Slot),
				Label: stringValue(binding.Selector.Label),
				Type:  stringValue(binding.Selector.Type),
				Role:  stringValue(binding.Selector.Role),
			}
		}
		bindings = append(bindings, current)
	}
	return bindings
}

func directModelInvocationUserMessage(operation string, inputContent []interfaces.WorkContentPart, bindings []interfaces.ResolvedModelOperationBinding) string {
	payload := struct {
		Operation string                                     `json:"operation"`
		Input     []interfaces.WorkContentPart               `json:"input,omitempty"`
		Bindings  []interfaces.ResolvedModelOperationBinding `json:"bindings,omitempty"`
	}{
		Operation: strings.TrimSpace(operation),
		Input:     append([]interfaces.WorkContentPart(nil), inputContent...),
		Bindings:  interfaces.CloneResolvedModelOperationBindings(bindings),
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return strings.TrimSpace(operation)
	}
	return string(encoded)
}

func directModelInvocationOutputContent(raw string, operation interfaces.ModelOperation) ([]interfaces.WorkContentPart, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, nil
	}

	var content factoryapi.WorkContent
	if err := json.Unmarshal([]byte(trimmed), &content); err == nil {
		return workcontent.PartsFromGenerated(&content), nil
	}
	var envelope struct {
		Content factoryapi.WorkContent `json:"content"`
	}
	if err := json.Unmarshal([]byte(trimmed), &envelope); err == nil && envelope.Content != nil {
		return workcontent.PartsFromGenerated(&envelope.Content), nil
	}
	if modelOperationHasOnlyTextOutputs(operation) {
		return []interfaces.WorkContentPart{{
			Type: interfaces.WorkContentPartTypeText,
			Text: raw,
		}}, nil
	}
	return nil, fmt.Errorf("model invocation response is not valid WorkContent JSON for operation %q", strings.TrimSpace(operation.Name))
}

func directModelInvocationStream(content []interfaces.WorkContentPart, options *factoryapi.ModelInvocationOptions) (string, string, error) {
	if options == nil || options.ResponseMode == nil || *options.ResponseMode != factoryapi.AUDIOSTREAM {
		return "", "", nil
	}
	for _, part := range content {
		if part.Type.Normalized() != interfaces.WorkContentPartTypeAudio || strings.TrimSpace(part.File) == "" {
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

func modelOperationHasOnlyTextOutputs(operation interfaces.ModelOperation) bool {
	if len(operation.Outputs) == 0 {
		return true
	}
	for _, output := range operation.Outputs {
		if len(output.ContentTypes) == 0 {
			return false
		}
		for _, contentType := range output.ContentTypes {
			if strings.TrimSpace(contentType) != interfaces.ModelOperationContentTypeText {
				return false
			}
		}
	}
	return true
}

func stringValue[T ~string](value *T) string {
	if value == nil {
		return ""
	}
	return string(*value)
}
