package runtimeopening

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func ProjectModelsRuntimeConfig(source factorydefinitions.RuntimeConfigLookup) *models.RuntimeConfig {
	if source == nil || source.FactoryConfig() == nil {
		return nil
	}
	factory := source.FactoryConfig()
	projected := &models.RuntimeConfig{
		FactoryDirectory: source.FactoryDir(),
		BaseDirectory:    source.RuntimeBaseDir(),
		Workers:          make([]models.RuntimeWorker, len(factory.Workers)),
		Resources:        projectModelsRuntimeResources(factory.Resources),
	}
	for i := range factory.Workers {
		worker := factory.Workers[i]
		projected.Workers[i] = models.RuntimeWorker{
			Name:          worker.Name,
			Type:          worker.Type,
			Model:         worker.Model,
			ModelProvider: worker.ModelProvider,
			ModelLocality: worker.ModelLocality,
			Command:       worker.Command,
			Args:          append([]string(nil), worker.Args...),
			Operations:    projectModelsRuntimeOperations(worker.Operations),
			Resources:     projectModelsRuntimeResources(worker.Resources),
		}
	}
	return projected
}

func projectModelsRuntimeResources(resources []factorydefinitions.ResourceConfig) []models.RuntimeResource {
	projected := make([]models.RuntimeResource, len(resources))
	for i := range resources {
		resource := resources[i]
		projected[i] = models.RuntimeResource{
			ID:         resource.ID,
			Name:       resource.Name,
			Type:       resource.Type,
			Capacity:   resource.Capacity,
			Model:      resource.Model,
			Backend:    resource.Backend,
			LoadPolicy: resource.LoadPolicy,
			Provider:   resource.Provider,
		}
	}
	return projected
}

func projectModelsRuntimeOperations(operations []factorydefinitions.ModelOperation) []models.RuntimeOperation {
	projected := make([]models.RuntimeOperation, len(operations))
	for i := range operations {
		projected[i] = models.RuntimeOperation{
			Name:    operations[i].Name,
			Inputs:  projectModelsRuntimeOperationSlots(operations[i].Inputs),
			Outputs: projectModelsRuntimeOperationSlots(operations[i].Outputs),
		}
	}
	return projected
}

func projectModelsRuntimeOperationSlots(slots []factorydefinitions.ModelOperationSlot) []models.RuntimeOperationSlot {
	projected := make([]models.RuntimeOperationSlot, len(slots))
	for i := range slots {
		projected[i] = models.RuntimeOperationSlot{
			Name:         slots[i].Name,
			ContentTypes: append([]string(nil), slots[i].ContentTypes...),
			Required:     slots[i].Required,
		}
	}
	return projected
}

const directModelInvocationDispatchID = "direct-model-invocation"

// RuntimeModelInvokerConfig carries the already-opened, session-owned
// capabilities needed by one direct model operation. It contains no mutable
// execution state; each call constructs a detached request for Workers while
// Models remains the authority for scoped readiness.
type RuntimeModelInvokerConfig struct {
	Models           models.Service
	Scope            models.RuntimeScopeRef
	Sessions         factorysessions.Service
	Workers          workers.Service
	RuntimeID        string
	GenerationID     string
	FactoryDirectory string
	WorkingDirectory string
}

type runtimeModelInvoker struct {
	config RuntimeModelInvokerConfig
}

// NewRuntimeModelInvoker binds direct model invocation to one opened runtime.
// The opened Models scope is used for readiness, while the attempt itself
// enters the same request-scoped Workers path as Factory Runtime execution.
func NewRuntimeModelInvoker(config RuntimeModelInvokerConfig) workers.ModelInvoker {
	return &runtimeModelInvoker{config: config}
}

func (invoker *runtimeModelInvoker) InvokeModel(
	ctx context.Context,
	modelName string,
	request models.Request,
) (models.Result, error) {
	failureContext := workers.InferenceFailureContext{
		ModelName: strings.TrimSpace(modelName),
		Operation: strings.TrimSpace(request.Operation),
	}
	if invoker == nil || invoker.config.Models == nil {
		return models.Result{}, classifyRuntimeModelError(
			fmt.Errorf("Models service is not available"), failureContext,
		)
	}
	if invoker.config.Sessions == nil {
		return models.Result{}, classifyRuntimeModelError(
			fmt.Errorf("Factory Session service is not available"), failureContext,
		)
	}
	if invoker.config.Scope.IsZero() {
		return models.Result{}, models.ErrRuntimeScopeInvalid
	}
	projection, err := invoker.config.Sessions.GetFactorySession(
		ctx, factorysessions.DefaultSessionID,
	)
	if err != nil {
		return models.Result{}, err
	}
	worker, operation, err := directRuntimeModelWorker(
		projection.Context.FactoryCfg, modelName, request.Operation,
	)
	if err != nil {
		return models.Result{}, classifyRuntimeModelError(err, failureContext)
	}
	failureContext.ModelName = worker.Model
	failureContext.WorkerName = worker.Name
	readiness, err := invoker.config.Models.GetModelReadiness(ctx, models.GetModelReadinessRequest{
		Scope: invoker.config.Scope, Name: worker.Model, Operation: request.Operation,
	})
	if err != nil {
		return models.Result{}, classifyRuntimeModelError(err, failureContext)
	}
	if err := readiness.Readiness.InvocationError(); err != nil {
		return models.Result{}, classifyRuntimeModelError(err, failureContext)
	}
	bindings, err := resolveRuntimeModelBindings(worker, request)
	if err != nil {
		return models.Result{}, classifyRuntimeModelError(err, failureContext)
	}

	raw, err := invoker.invokeRuntimeModel(ctx, worker, request, bindings)
	if err != nil {
		return models.Result{}, classifyRuntimeModelError(err, failureContext)
	}

	content, err := runtimeModelContent(raw, operation)
	if err != nil {
		return models.Result{}, classifyRuntimeModelError(err, failureContext)
	}
	streamFile, streamContentType, err := runtimeModelStream(content, request.Options)
	if err != nil {
		return models.Result{}, classifyRuntimeModelError(err, failureContext)
	}
	return models.Result{
		ModelName:         worker.Model,
		Worker:            worker.Name,
		Operation:         strings.TrimSpace(request.Operation),
		ProviderLocality:  worker.ModelLocality,
		Content:           content,
		Bindings:          bindings,
		StreamFile:        streamFile,
		StreamContentType: streamContentType,
	}, nil
}

func (invoker *runtimeModelInvoker) invokeRuntimeModel(
	ctx context.Context,
	worker *factorydefinitions.FactoryWorkerConfig,
	request models.Request,
	bindings []models.ResolvedModelOperationBinding,
) (string, error) {
	if invoker == nil || invoker.config.Workers == nil {
		return "", fmt.Errorf("Workers service is not available")
	}
	selection := workers.ResolveRunnerSelection("", "", worker.ModelProvider)
	provider := strings.TrimSpace(worker.ModelProvider)
	if provider == "" {
		provider = selection.RunnerID
	}
	dispatchID := directModelInvocationDispatchID
	runtimeID := firstRuntimeModelValue(invoker.config.RuntimeID, dispatchID+"-runtime")
	generationID := firstRuntimeModelValue(invoker.config.GenerationID, dispatchID+"-generation")
	requestID := dispatchID + "-request"
	traceID := dispatchID + "-trace"
	workingDirectory := strings.TrimSpace(invoker.config.WorkingDirectory)
	factoryDirectory := strings.TrimSpace(invoker.config.FactoryDirectory)
	userMessage := runtimeModelUserMessage(request.Operation, request.Content, bindings)
	executeResult, err := invoker.config.Workers.Execute(ctx, workers.ExecuteRequest{
		Correlation: workers.ExecutionCorrelation{
			FactorySessionID: factorysessions.DefaultSessionID,
			RuntimeID:        runtimeID,
			GenerationID:     generationID,
			DispatchID:       dispatchID,
			AttemptID:        dispatchID + "-attempt",
			RequestID:        requestID,
			TraceID:          traceID,
		},
		Target: workers.ExecutionTarget{
			WorkerName:       worker.Name,
			WorkerType:       worker.Type,
			WorkstationName:  dispatchID,
			RunnerID:         selection.RunnerID,
			ExecutorProvider: worker.ExecutorProvider,
			FactoryDirectory: factoryDirectory,
			Provider:         workers.ProviderReference{ID: provider},
			Model: workers.ModelReference{
				Name:            worker.Model,
				Provider:        provider,
				ReasoningEffort: worker.ReasoningEffort,
				Locality:        worker.ModelLocality,
			},
			Prompt: workers.PromptPolicy{
				SystemPrompt: worker.Body,
				UserMessage:  userMessage,
			},
			Output: workers.OutputPolicy{StopToken: worker.StopToken},
			Environment: workers.EnvironmentPolicy{
				WorkingDirectory:    workingDirectory,
				WorkingDirectorySet: workingDirectory != "",
			},
			Workspace: workers.WorkspacePolicy{
				WorkingDirectory: workingDirectory,
				FactoryDirectory: factoryDirectory,
			},
			Permissions: workers.PermissionPolicy{SkipPermissions: worker.SkipPermissions},
			Timeout:     worker.TimeoutDuration(),
		},
		Input: workers.ExecutionInput{
			Dispatch: work.WorkDispatch{
				DispatchID:      dispatchID,
				TransitionID:    dispatchID,
				WorkerType:      worker.Name,
				WorkstationName: dispatchID,
				Execution: work.ExecutionMetadata{
					RequestID: requestID,
					TraceID:   traceID,
				},
			},
			ModelBindings:  runtimeWorkerBindings(bindings),
			ModelOperation: strings.TrimSpace(request.Operation),
			WorkflowContext: &workers.Context{
				FactoryDirectory: factoryDirectory,
				WorkDirectory:    workingDirectory,
				SessionID:        factorysessions.DefaultSessionID,
			},
		},
		Attempt: workers.AttemptContext{Number: 1},
	})
	if err != nil {
		return "", err
	}
	if executeResult.Outcome != workers.ExecutionOutcomeAccepted {
		message := fmt.Sprintf("Workers execution outcome %q", executeResult.Outcome)
		if executeResult.Failure != nil && strings.TrimSpace(executeResult.Failure.Message) != "" {
			message = executeResult.Failure.Message
		}
		return "", fmt.Errorf("%s", message)
	}
	return runtimeModelOutput(executeResult), nil
}

func runtimeModelOutput(result workers.ExecuteResult) string {
	if len(result.Output.Primary) == 1 &&
		result.Output.Primary[0].Type.Normalized() == work.WorkContentPartTypeText {
		return result.Output.Primary[0].Text
	}
	if len(result.Output.Primary) == 0 {
		return ""
	}
	encoded, err := json.Marshal(result.Output.Primary)
	if err != nil {
		return ""
	}
	return string(encoded)
}

func runtimeModelUserMessage(
	operation string,
	content []work.WorkContentPart,
	bindings []models.ResolvedModelOperationBinding,
) string {
	payload := struct {
		Operation string                                 `json:"operation"`
		Input     []work.WorkContentPart                 `json:"input,omitempty"`
		Bindings  []models.ResolvedModelOperationBinding `json:"bindings,omitempty"`
	}{
		Operation: strings.TrimSpace(operation),
		Input:     work.CloneWorkContentParts(content),
		Bindings:  append([]models.ResolvedModelOperationBinding(nil), bindings...),
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return strings.TrimSpace(operation)
	}
	return string(encoded)
}

func runtimeWorkerBindings(
	bindings []models.ResolvedModelOperationBinding,
) []workers.ResolvedModelOperationBinding {
	if len(bindings) == 0 {
		return nil
	}
	result := make([]workers.ResolvedModelOperationBinding, len(bindings))
	for index, binding := range bindings {
		result[index] = workers.ResolvedModelOperationBinding{
			Slot: binding.Slot,
			Source: workers.ModelOperationBindingSource(
				strings.TrimSpace(binding.Source),
			),
			Content: work.CloneWorkContentParts(binding.Content),
		}
	}
	return result
}

func firstRuntimeModelValue(value, fallback string) string {
	if value = strings.TrimSpace(value); value != "" {
		return value
	}
	return fallback
}

func directRuntimeModelWorker(
	config *factorydefinitions.FactoryConfig,
	modelName, operationName string,
) (*factorydefinitions.FactoryWorkerConfig, factorydefinitions.ModelOperation, error) {
	if config == nil {
		return nil, factorydefinitions.ModelOperation{}, fmt.Errorf("factory config is not available")
	}
	modelKey := strings.ToUpper(strings.TrimSpace(modelName))
	operationName = strings.TrimSpace(operationName)
	if modelKey == "" {
		return nil, factorydefinitions.ModelOperation{}, fmt.Errorf(
			"%w: empty model name", models.ErrNotFound,
		)
	}
	if operationName == "" {
		return nil, factorydefinitions.ModelOperation{}, fmt.Errorf("operation is required")
	}
	var matchedWorker string
	for index := range config.Workers {
		worker := &config.Workers[index]
		if !factorydefinitions.IsInferenceWorkerType(worker.Type) ||
			strings.ToUpper(strings.TrimSpace(worker.Model)) != modelKey {
			continue
		}
		matchedWorker = worker.Name
		for _, operation := range worker.Operations {
			if strings.TrimSpace(operation.Name) == operationName {
				return worker, operation, nil
			}
		}
	}
	if matchedWorker != "" {
		return nil, factorydefinitions.ModelOperation{}, fmt.Errorf(
			"%w: worker %q for model %q does not support operation %q",
			models.ErrUnsupportedOperation, matchedWorker, modelName, operationName,
		)
	}
	return nil, factorydefinitions.ModelOperation{}, fmt.Errorf("%w: %s", models.ErrNotFound, modelName)
}

func resolveRuntimeModelBindings(
	worker *factorydefinitions.FactoryWorkerConfig,
	request models.Request,
) ([]models.ResolvedModelOperationBinding, error) {
	var operation factorydefinitions.ModelOperation
	for _, candidate := range worker.Operations {
		if strings.TrimSpace(candidate.Name) == strings.TrimSpace(request.Operation) {
			operation = candidate
			break
		}
	}
	authored := make(map[string]models.ModelOperationBinding, len(request.Bindings))
	for _, binding := range request.Bindings {
		if slot := strings.TrimSpace(binding.Slot); slot != "" {
			authored[slot] = binding
		}
	}
	resolved := make([]models.ResolvedModelOperationBinding, 0, len(operation.Inputs))
	for _, input := range operation.Inputs {
		binding, ok := authored[input.Name]
		if !ok {
			binding = models.ModelOperationBinding{
				Slot:     input.Name,
				Selector: &models.ModelOperationBindingSelector{Slot: input.Name},
			}
		}
		current, err := resolveRuntimeModelBinding(input, binding, request)
		if err != nil {
			return nil, err
		}
		resolved = append(resolved, current)
	}
	return resolved, nil
}

func resolveRuntimeModelBinding(
	input factorydefinitions.ModelOperationSlot,
	binding models.ModelOperationBinding,
	request models.Request,
) (models.ResolvedModelOperationBinding, error) {
	current := models.ResolvedModelOperationBinding{Slot: binding.Slot, Source: "OMITTED"}
	if !runtimeModelSelectorEmpty(binding.Selector) {
		current.Content = firstRuntimeModelInput(request.Content, binding.Selector)
		if len(current.Content) > 0 {
			current.Source = "INPUT"
		}
	}
	if len(current.Content) == 0 && len(binding.Config) > 0 {
		current.Source = "CONFIG"
		current.Content = work.CloneWorkContentParts(binding.Config)
	}
	if len(current.Content) == 0 && len(binding.DefaultContent) > 0 {
		current.Source = "DEFAULT"
		current.Content = work.CloneWorkContentParts(binding.DefaultContent)
	}
	if len(current.Content) == 0 && input.Required {
		return models.ResolvedModelOperationBinding{}, fmt.Errorf(
			"required slot %q could not be resolved for operation %q",
			input.Name, request.Operation,
		)
	}
	return current, nil
}

func firstRuntimeModelInput(
	content []work.WorkContentPart,
	selector *models.ModelOperationBindingSelector,
) []work.WorkContentPart {
	for _, part := range content {
		if runtimeModelSelectorMatches(part, selector) {
			return []work.WorkContentPart{part}
		}
	}
	return nil
}

func runtimeModelSelectorEmpty(selector *models.ModelOperationBindingSelector) bool {
	return selector == nil || (strings.TrimSpace(selector.Slot) == "" &&
		strings.TrimSpace(selector.Label) == "" && strings.TrimSpace(selector.Type) == "" &&
		strings.TrimSpace(selector.Role) == "")
}

func runtimeModelSelectorMatches(
	part work.WorkContentPart,
	selector *models.ModelOperationBindingSelector,
) bool {
	if selector == nil {
		return false
	}
	if value := strings.TrimSpace(selector.Slot); value != "" && strings.TrimSpace(part.Slot) != value {
		return false
	}
	if value := strings.TrimSpace(selector.Label); value != "" && strings.TrimSpace(part.Label) != value {
		return false
	}
	if value := strings.TrimSpace(selector.Type); value != "" && runtimeModelContentType(part) != value {
		return false
	}
	return strings.TrimSpace(selector.Role) == "" || strings.TrimSpace(part.Role) == strings.TrimSpace(selector.Role)
}

func runtimeModelContentType(part work.WorkContentPart) string {
	switch part.Type.Normalized() {
	case work.WorkContentPartTypeText:
		return factorydefinitions.ModelOperationContentTypeText
	case work.WorkContentPartTypeImage:
		return factorydefinitions.ModelOperationContentTypeImage
	case work.WorkContentPartTypeAudio:
		return factorydefinitions.ModelOperationContentTypeAudio
	case work.WorkContentPartTypeJSON:
		return factorydefinitions.ModelOperationContentTypeJSON
	case work.WorkContentPartTypeBinary:
		return factorydefinitions.ModelOperationContentTypeBinary
	default:
		return strings.TrimSpace(string(part.Type))
	}
}

func runtimeModelContent(
	raw string,
	operation factorydefinitions.ModelOperation,
) ([]work.WorkContentPart, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, nil
	}
	var content []work.WorkContentPart
	if err := json.Unmarshal([]byte(trimmed), &content); err == nil {
		return work.SupportedContentParts(content), nil
	}
	var envelope struct {
		Content []work.WorkContentPart `json:"content"`
	}
	if err := json.Unmarshal([]byte(trimmed), &envelope); err == nil && envelope.Content != nil {
		return work.SupportedContentParts(envelope.Content), nil
	}
	if runtimeModelOnlyTextOutputs(operation) {
		return []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: raw}}, nil
	}
	return nil, fmt.Errorf(
		"inference response is not valid WorkContent JSON for operation %q", operation.Name,
	)
}

func runtimeModelOnlyTextOutputs(operation factorydefinitions.ModelOperation) bool {
	if len(operation.Outputs) == 0 {
		return true
	}
	for _, output := range operation.Outputs {
		if len(output.ContentTypes) == 0 {
			return false
		}
		for _, contentType := range output.ContentTypes {
			if strings.TrimSpace(contentType) != factorydefinitions.ModelOperationContentTypeText {
				return false
			}
		}
	}
	return true
}

func runtimeModelStream(
	content []work.WorkContentPart,
	options *models.Options,
) (string, string, error) {
	if options == nil || options.ResponseMode != models.ResponseModeAudioStream {
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
	return "", "", fmt.Errorf(
		"%w: invocation did not produce audio output", models.ErrUnsupportedResponseMode,
	)
}

func classifyRuntimeModelError(err error, context workers.InferenceFailureContext) error {
	if failure, ok := workers.ClassifyInferenceFailure(err, context); ok {
		return failure
	}
	return err
}
