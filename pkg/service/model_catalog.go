package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
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

func (fs *FactoryService) ListModels(_ context.Context) (factoryapi.ListModelsResponse, error) {
	catalog, err := fs.currentModelCatalog()
	if err != nil {
		return factoryapi.ListModelsResponse{}, err
	}
	results := make([]factoryapi.ModelSummary, 0, len(catalog))
	for _, entry := range catalog {
		results = append(results, entry.summary)
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].Name < results[j].Name
	})
	return factoryapi.ListModelsResponse{Results: results}, nil
}

func (fs *FactoryService) GetModel(_ context.Context, modelName string) (factoryapi.ModelDetail, error) {
	catalog, err := fs.currentModelCatalog()
	if err != nil {
		return factoryapi.ModelDetail{}, err
	}
	key := canonicalModelName(modelName)
	if key == "" {
		return factoryapi.ModelDetail{}, fmt.Errorf("%w: empty model name", apisurface.ErrModelNotFound)
	}
	entry, ok := catalog[key]
	if !ok {
		return factoryapi.ModelDetail{}, fmt.Errorf("%w: %s", apisurface.ErrModelNotFound, modelName)
	}
	return entry.detail, nil
}

type discoveredModelCatalogEntry struct {
	summary factoryapi.ModelSummary
	detail  factoryapi.ModelDetail
}

type discoveredModelAggregate struct {
	name           string
	locality       string
	localities     map[string]struct{}
	workerCount    int
	localCount     int
	cloudCount     int
	operations     map[string]factoryapi.ModelOperation
	modalities     map[factoryapi.ModelOperationContentType]struct{}
	resources      map[string]factoryapi.ModelResourceSummary
	capabilities   []factoryapi.ModelCapability
	workerNames    []string
	hasAnyResource bool
	hasModelScoped bool
}

func (fs *FactoryService) currentModelCatalog() (map[string]discoveredModelCatalogEntry, error) {
	if fs == nil {
		return nil, fmt.Errorf("factory service is required")
	}
	runtimeCfg := fs.currentRuntimeConfig()
	if runtimeCfg == nil {
		return nil, fmt.Errorf("factory service runtime is not available")
	}
	return buildModelCatalog(runtimeCfg), nil
}

func buildModelCatalog(runtimeCfg *factoryconfig.LoadedFactoryConfig) map[string]discoveredModelCatalogEntry {
	if runtimeCfg == nil || runtimeCfg.FactoryConfig() == nil {
		return map[string]discoveredModelCatalogEntry{}
	}

	factoryCfg := runtimeCfg.FactoryConfig()
	resourceByName := make(map[string]interfaces.ResourceConfig, len(factoryCfg.Resources))
	for _, resource := range factoryCfg.Resources {
		resourceByName[resource.Name] = resource
	}

	aggregates := make(map[string]*discoveredModelAggregate)
	for _, worker := range factoryCfg.Workers {
		if strings.TrimSpace(worker.Type) != interfaces.WorkerTypeModel {
			continue
		}
		key := canonicalModelName(worker.Model)
		if key == "" {
			continue
		}
		aggregate := aggregates[key]
		if aggregate == nil {
			aggregate = &discoveredModelAggregate{
				name:       strings.TrimSpace(worker.Model),
				locality:   strings.TrimSpace(worker.ModelLocality),
				localities: make(map[string]struct{}),
				operations: make(map[string]factoryapi.ModelOperation),
				modalities: make(map[factoryapi.ModelOperationContentType]struct{}),
				resources:  make(map[string]factoryapi.ModelResourceSummary),
			}
			aggregates[key] = aggregate
		}
		aggregate.workerCount++
		aggregate.workerNames = append(aggregate.workerNames, worker.Name)
		if locality := strings.TrimSpace(worker.ModelLocality); locality != "" {
			aggregate.localities[locality] = struct{}{}
			if aggregate.locality == "" {
				aggregate.locality = locality
			}
			switch locality {
			case interfaces.ModelLocalityLocal:
				aggregate.localCount++
			case interfaces.ModelLocalityCloud:
				aggregate.cloudCount++
			}
		}

		aggregate.capabilities = append(aggregate.capabilities, capabilityFromWorker(worker))
		for _, operation := range worker.Operations {
			mergeAggregateOperation(aggregate, operation)
		}
		collectAggregateResources(aggregate, worker, resourceByName, factoryCfg.Resources)
	}

	catalog := make(map[string]discoveredModelCatalogEntry, len(aggregates))
	for key, aggregate := range aggregates {
		sort.Strings(aggregate.workerNames)
		sort.Slice(aggregate.capabilities, func(i, j int) bool {
			return aggregate.capabilities[i].Worker < aggregate.capabilities[j].Worker
		})
		summary := buildModelSummary(*aggregate)
		catalog[key] = discoveredModelCatalogEntry{
			summary: summary,
			detail: factoryapi.ModelDetail{
				Name:             summary.Name,
				ProviderLocality: summary.ProviderLocality,
				Status:           summary.Status,
				LoadState:        summary.LoadState,
				Operations:       summary.Operations,
				Modalities:       summary.Modalities,
				Resources:        summary.Resources,
				Capabilities:     aggregate.capabilities,
				Diagnostics:      modelDiagnostics(*aggregate, summary),
			},
		}
	}
	return catalog
}

func capabilityFromWorker(worker interfaces.WorkerConfig) factoryapi.ModelCapability {
	operations := make([]factoryapi.ModelOperation, 0, len(worker.Operations))
	for _, operation := range worker.Operations {
		operations = append(operations, generatedModelOperation(operation))
	}
	sort.Slice(operations, func(i, j int) bool {
		return operations[i].Name < operations[j].Name
	})
	resourceNames := make([]string, 0, len(worker.Resources))
	for _, resource := range worker.Resources {
		if name := strings.TrimSpace(resource.Name); name != "" {
			resourceNames = append(resourceNames, name)
		}
	}
	sort.Strings(resourceNames)

	capability := factoryapi.ModelCapability{
		Worker:           worker.Name,
		ProviderLocality: factoryapi.WorkerModelLocality(strings.TrimSpace(worker.ModelLocality)),
		Operations:       operations,
		ResourceNames:    resourceNames,
	}
	if provider := strings.TrimSpace(worker.ModelProvider); provider != "" {
		generatedProvider := factoryapi.WorkerModelProvider(provider)
		capability.ModelProvider = &generatedProvider
	}
	return capability
}

func mergeAggregateOperation(aggregate *discoveredModelAggregate, operation interfaces.ModelOperation) {
	if aggregate == nil {
		return
	}
	key := strings.TrimSpace(operation.Name)
	if key == "" {
		return
	}
	generated := generatedModelOperation(operation)
	if existing, ok := aggregate.operations[key]; ok {
		generated = mergeGeneratedOperations(existing, generated)
	}
	aggregate.operations[key] = generated
	for _, slot := range operation.Inputs {
		for _, contentType := range slot.ContentTypes {
			if normalized := strings.TrimSpace(contentType); normalized != "" {
				aggregate.modalities[factoryapi.ModelOperationContentType(normalized)] = struct{}{}
			}
		}
	}
	for _, slot := range operation.Outputs {
		for _, contentType := range slot.ContentTypes {
			if normalized := strings.TrimSpace(contentType); normalized != "" {
				aggregate.modalities[factoryapi.ModelOperationContentType(normalized)] = struct{}{}
			}
		}
	}
}

func mergeGeneratedOperations(left, right factoryapi.ModelOperation) factoryapi.ModelOperation {
	merged := left
	merged.Inputs = mergeGeneratedOperationSlots(left.Inputs, right.Inputs)
	merged.Outputs = mergeGeneratedOperationSlots(left.Outputs, right.Outputs)
	return merged
}

func mergeGeneratedOperationSlots(left, right *[]factoryapi.ModelOperationSlot) *[]factoryapi.ModelOperationSlot {
	slotByName := make(map[string]factoryapi.ModelOperationSlot)
	order := make([]string, 0)
	appendSlots := func(slots *[]factoryapi.ModelOperationSlot) {
		if slots == nil {
			return
		}
		for _, slot := range *slots {
			existing, ok := slotByName[slot.Name]
			if !ok {
				slotByName[slot.Name] = slot
				order = append(order, slot.Name)
				continue
			}
			slotByName[slot.Name] = mergeGeneratedOperationSlot(existing, slot)
		}
	}
	appendSlots(left)
	appendSlots(right)
	if len(order) == 0 {
		return nil
	}
	merged := make([]factoryapi.ModelOperationSlot, 0, len(order))
	for _, name := range order {
		merged = append(merged, slotByName[name])
	}
	return &merged
}

func mergeGeneratedOperationSlot(left, right factoryapi.ModelOperationSlot) factoryapi.ModelOperationSlot {
	merged := left
	typeSet := make(map[factoryapi.ModelOperationContentType]struct{}, len(left.ContentTypes)+len(right.ContentTypes))
	ordered := make([]factoryapi.ModelOperationContentType, 0, len(left.ContentTypes)+len(right.ContentTypes))
	for _, contentType := range left.ContentTypes {
		if _, ok := typeSet[contentType]; ok {
			continue
		}
		typeSet[contentType] = struct{}{}
		ordered = append(ordered, contentType)
	}
	for _, contentType := range right.ContentTypes {
		if _, ok := typeSet[contentType]; ok {
			continue
		}
		typeSet[contentType] = struct{}{}
		ordered = append(ordered, contentType)
	}
	merged.ContentTypes = ordered
	if merged.Required == nil {
		merged.Required = right.Required
	} else if right.Required != nil && *right.Required {
		required := true
		merged.Required = &required
	}
	return merged
}

func collectAggregateResources(
	aggregate *discoveredModelAggregate,
	worker interfaces.WorkerConfig,
	resourceByName map[string]interfaces.ResourceConfig,
	allResources []interfaces.ResourceConfig,
) {
	if aggregate == nil {
		return
	}
	for _, requirement := range worker.Resources {
		resource, ok := resourceByName[requirement.Name]
		if !ok {
			continue
		}
		aggregate.hasAnyResource = true
		if canonicalModelName(resource.Model) == canonicalModelName(worker.Model) && canonicalModelName(resource.Model) != "" {
			aggregate.hasModelScoped = true
		}
		aggregate.resources[resource.Name] = generatedModelResourceSummary(resource)
	}
	for _, resource := range allResources {
		if canonicalModelName(resource.Model) != canonicalModelName(worker.Model) {
			continue
		}
		aggregate.hasAnyResource = true
		aggregate.hasModelScoped = true
		aggregate.resources[resource.Name] = generatedModelResourceSummary(resource)
	}
}

func buildModelSummary(aggregate discoveredModelAggregate) factoryapi.ModelSummary {
	operations := make([]factoryapi.ModelOperation, 0, len(aggregate.operations))
	for _, operation := range aggregate.operations {
		operations = append(operations, operation)
	}
	sort.Slice(operations, func(i, j int) bool {
		return operations[i].Name < operations[j].Name
	})

	modalities := make([]factoryapi.ModelOperationContentType, 0, len(aggregate.modalities))
	for modality := range aggregate.modalities {
		modalities = append(modalities, modality)
	}
	sort.Slice(modalities, func(i, j int) bool {
		return modalities[i] < modalities[j]
	})

	resources := make([]factoryapi.ModelResourceSummary, 0, len(aggregate.resources))
	for _, resource := range aggregate.resources {
		resources = append(resources, resource)
	}
	sort.Slice(resources, func(i, j int) bool {
		return resources[i].Name < resources[j].Name
	})

	return factoryapi.ModelSummary{
		Name:             aggregate.name,
		ProviderLocality: factoryapi.WorkerModelLocality(primaryModelLocality(aggregate)),
		Status:           modelStatus(aggregate),
		LoadState:        modelLoadState(aggregate),
		Operations:       operations,
		Modalities:       modalities,
		Resources:        resources,
	}
}

func modelStatus(aggregate discoveredModelAggregate) factoryapi.ModelStatus {
	if aggregate.localCount > 0 && !aggregate.hasModelScoped {
		return factoryapi.ModelStatusUNAVAILABLE
	}
	return factoryapi.ModelStatusREADY
}

func modelLoadState(aggregate discoveredModelAggregate) factoryapi.ModelLoadState {
	if primaryModelLocality(aggregate) == interfaces.ModelLocalityLocal {
		return factoryapi.UNLOADED
	}
	return factoryapi.NOTAPPLICABLE
}

func primaryModelLocality(aggregate discoveredModelAggregate) string {
	switch {
	case aggregate.locality != "":
		return aggregate.locality
	case aggregate.localCount > 0:
		return interfaces.ModelLocalityLocal
	case aggregate.cloudCount > 0:
		return interfaces.ModelLocalityCloud
	default:
		return interfaces.ModelLocalityCloud
	}
}

func modelDiagnostics(aggregate discoveredModelAggregate, summary factoryapi.ModelSummary) factoryapi.StringMap {
	diagnostics := factoryapi.StringMap{
		"workerCount":      strconv.Itoa(aggregate.workerCount),
		"localWorkerCount": strconv.Itoa(aggregate.localCount),
		"cloudWorkerCount": strconv.Itoa(aggregate.cloudCount),
		"resourceCount":    strconv.Itoa(len(summary.Resources)),
		"workers":          strings.Join(aggregate.workerNames, ","),
		"mixedLocality":    strconv.FormatBool(len(aggregate.localities) > 1),
	}
	if summary.Status == factoryapi.ModelStatusUNAVAILABLE {
		diagnostics["statusReason"] = "local model workers require a matching MODEL resource declaration for readiness"
	} else {
		diagnostics["statusReason"] = "declared worker capabilities and resources are discoverable"
	}
	return diagnostics
}

func generatedModelOperation(operation interfaces.ModelOperation) factoryapi.ModelOperation {
	generated := factoryapi.ModelOperation{
		Name: strings.TrimSpace(operation.Name),
	}
	if len(operation.Inputs) > 0 {
		inputs := make([]factoryapi.ModelOperationSlot, 0, len(operation.Inputs))
		for _, slot := range operation.Inputs {
			inputs = append(inputs, generatedModelOperationSlot(slot))
		}
		sort.Slice(inputs, func(i, j int) bool {
			return inputs[i].Name < inputs[j].Name
		})
		generated.Inputs = &inputs
	}
	if len(operation.Outputs) > 0 {
		outputs := make([]factoryapi.ModelOperationSlot, 0, len(operation.Outputs))
		for _, slot := range operation.Outputs {
			outputs = append(outputs, generatedModelOperationSlot(slot))
		}
		sort.Slice(outputs, func(i, j int) bool {
			return outputs[i].Name < outputs[j].Name
		})
		generated.Outputs = &outputs
	}
	return generated
}

func generatedModelOperationSlot(slot interfaces.ModelOperationSlot) factoryapi.ModelOperationSlot {
	contentTypes := make([]factoryapi.ModelOperationContentType, 0, len(slot.ContentTypes))
	for _, contentType := range slot.ContentTypes {
		if normalized := strings.TrimSpace(contentType); normalized != "" {
			contentTypes = append(contentTypes, factoryapi.ModelOperationContentType(normalized))
		}
	}
	sort.Slice(contentTypes, func(i, j int) bool {
		return contentTypes[i] < contentTypes[j]
	})
	generated := factoryapi.ModelOperationSlot{
		Name:         strings.TrimSpace(slot.Name),
		ContentTypes: contentTypes,
	}
	if slot.Required {
		required := true
		generated.Required = &required
	}
	return generated
}

func generatedModelResourceSummary(resource interfaces.ResourceConfig) factoryapi.ModelResourceSummary {
	summary := factoryapi.ModelResourceSummary{
		Name:     resource.Name,
		Type:     factoryapi.ResourceType(strings.TrimSpace(resource.Type)),
		Capacity: resource.Capacity,
	}
	if value := strings.TrimSpace(resource.Model); value != "" {
		summary.Model = &value
	}
	if value := strings.TrimSpace(resource.Backend); value != "" {
		summary.Backend = &value
	}
	if value := strings.TrimSpace(resource.LoadPolicy); value != "" {
		summary.LoadPolicy = &value
	}
	if value := strings.TrimSpace(resource.Provider); value != "" {
		summary.Provider = &value
	}
	return summary
}

func canonicalModelName(model string) string {
	return strings.ToUpper(strings.TrimSpace(model))
}

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
