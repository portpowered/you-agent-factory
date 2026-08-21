package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	modelinference "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	contentcontract "github.com/portpowered/infinite-you/pkg/transports/mapping/workcontent"
)

// pkgmaintcheck:ignore-cyclomatic-complexity pre-existing baseline debt recorded 2026-08-08; refactor this code below the maintainability threshold and remove this exemption
func (service *rootService) Invoke(cfg InvokeConfig) error {
	if cfg.Context == nil {
		return fmt.Errorf("context is required")
	}
	if cfg.Output == nil {
		return fmt.Errorf("output writer is required")
	}
	modelName := strings.TrimSpace(cfg.ModelName)
	if modelName == "" {
		return fmt.Errorf("model name is required")
	}
	operation := strings.TrimSpace(cfg.Operation)
	if operation == "" {
		return fmt.Errorf("--operation is required")
	}
	text := strings.TrimSpace(cfg.Text)
	if text == "" {
		return fmt.Errorf("--text is required")
	}
	if strings.TrimSpace(cfg.Server) != "" {
		return fmt.Errorf("remote models invoke requires the composition-stable HTTP service")
	}
	if service.openInvokeScope == nil {
		return fmt.Errorf("models invoke runtime scope opener is required")
	}
	scope, err := service.openInvokeScope(cfg.Context, cfg)
	if err != nil {
		return mapModelsRootError(err)
	}
	if scope.Close != nil {
		defer func() {
			_ = scope.Close(cfg.Context)
		}()
	}
	return service.invokeInScope(cfg, scope.Scope, modelName, operation, text)
}

func (service *rootService) invokeInScope(
	cfg InvokeConfig,
	scope modelinference.RuntimeScopeRef,
	modelName string,
	operation string,
	text string,
) error {
	catalogResult, err := service.models.GetCatalogModel(cfg.Context, modelinference.GetModelRequest{
		Scope: scope, Name: modelName, Operation: operation,
	})
	if err != nil {
		if !cfg.JSON && strings.TrimSpace(cfg.OutputPath) == "" && errors.Is(err, modelinference.ErrUnsupportedOperation) {
			return fmt.Errorf("--output is required unless --json is set")
		}
		return mapModelsRootError(err)
	}
	if err := validateCLIOutputShape(cfg, catalogResult.Model, operation); err != nil {
		return err
	}
	if cfg.JSON || genericCLIInlineOutput(cfg, catalogResult.Model, operation) {
		joinedResult, joinedErr := service.models.InvokeModel(cfg.Context, joinedCLIInvocationRequest(
			scope, modelName, operation, text, catalogResult.Model,
		))
		if joinedErr == nil {
			if genericCLIJSONResult(cfg, catalogResult.Model, operation, joinedResult) {
				return json.NewEncoder(cfg.Output).Encode(genericInvocationResponseFromInferenceResult(joinedResult))
			}
			if genericCLIInlineOutput(cfg, catalogResult.Model, operation) {
				return writeGenericCLIOutput(cfg.Output, joinedResult)
			}
			response := modelInvocationResponseFromInferenceResult(joinedResult, catalogResult.Model, text)
			return json.NewEncoder(cfg.Output).Encode(response)
		}
		if !errors.Is(joinedErr, modelinference.ErrUnsupportedOperation) &&
			!errors.Is(joinedErr, modelinference.ErrModelReferenceUnknown) {
			return mapModelsRootError(joinedErr)
		}
	}
	return service.invokePreparedLease(cfg, scope, modelName, operation, text, catalogResult.Model)
}

func validateCLIOutputShape(
	cfg InvokeConfig,
	catalog modelinference.Detail,
	operation string,
) error {
	if cfg.JSON {
		return nil
	}
	selected, ok := catalogOperationForName(catalog, operation)
	if ok && len(selected.Outputs) > 1 {
		return fmt.Errorf(
			"multiple model outputs require --json or explicit output mappings: %s",
			genericOutputSlotNames(selected.Outputs),
		)
	}
	if strings.TrimSpace(cfg.OutputPath) != "" {
		return nil
	}
	if !ok || len(selected.Outputs) != 1 || !genericCLIInlineModality(selected.Outputs[0].Modality) {
		return fmt.Errorf("--output is required unless --json is set")
	}
	return nil
}

func genericCLIInlineOutput(cfg InvokeConfig, catalog modelinference.Detail, operation string) bool {
	if strings.TrimSpace(cfg.OutputPath) != "" {
		return false
	}
	selected, ok := catalogOperationForName(catalog, operation)
	return ok && len(selected.Outputs) == 1 && genericCLIInlineModality(selected.Outputs[0].Modality)
}

func genericCLIInlineModality(modality modelinference.Modality) bool {
	return modality == modelinference.ModalityText || modality == modelinference.ModalityJSON
}

func genericOutputSlotNames(outputs []modelinference.OperationSlot) string {
	names := make([]string, 0, len(outputs))
	for _, output := range outputs {
		name := strings.TrimSpace(output.Name)
		if name != "" {
			names = append(names, name)
		}
	}
	return strings.Join(names, ", ")
}

func genericCLIJSONResult(
	cfg InvokeConfig,
	catalog modelinference.Detail,
	operation string,
	result modelinference.InvokeModelResult,
) bool {
	if !cfg.JSON || len(result.Outputs) == 0 {
		return false
	}
	if len(result.Outputs) > 1 {
		return true
	}
	selected, ok := catalogOperationForName(catalog, operation)
	return ok && len(selected.Outputs) == 1 && genericCLIInlineModality(selected.Outputs[0].Modality)
}

func writeGenericCLIOutput(output interface{ Write([]byte) (int, error) }, result modelinference.InvokeModelResult) error {
	if len(result.Outputs) != 1 {
		return fmt.Errorf("multiple model outputs require --json or explicit output mappings")
	}
	value := result.Outputs[0].Content
	if value == "" {
		return fmt.Errorf("model invocation returned no inline output")
	}
	_, err := output.Write([]byte(value))
	return err
}

func (service *rootService) invokePreparedLease(
	cfg InvokeConfig,
	scope modelinference.RuntimeScopeRef,
	modelName string,
	operation string,
	text string,
	catalog modelinference.Detail,
) error {
	if runtime := catalog.ManagedRuntime; strings.TrimSpace(runtime.Identity) != "" {
		if err := runtime.InvocationError(); err != nil {
			return mapModelsRootError(err)
		}
	}
	leaseResult, err := service.models.AcquireModelLease(cfg.Context, modelinference.AcquireModelLeaseRequest{
		Scope: scope, Name: modelName, Holder: modelsCLIInvokeHolder,
	})
	if err != nil {
		return mapModelsRootError(err)
	}
	request := modelinference.InvokeModelRequest{
		Scope:     scope,
		Lease:     leaseResult.Lease.Lease,
		Holder:    modelsCLIInvokeHolder,
		ModelName: modelName,
		Operation: operation,
		Input: modelinference.InferenceInput{
			ContentType: "text/plain",
			Content:     text,
		},
	}
	if !cfg.JSON {
		mode := modelinference.ResponseModeAudioStream
		request.ResponseMode = mode
	}
	result, err := service.models.InvokeModelWithLease(cfg.Context, request)
	if err != nil {
		return mapModelsRootError(err)
	}
	if cfg.JSON {
		response := modelInvocationResponseFromInferenceResult(result, catalog, text)
		return json.NewEncoder(cfg.Output).Encode(response)
	}
	outputPath := strings.TrimSpace(cfg.OutputPath)
	streamFile, err := inferenceArtifactSourcePath(result)
	if err != nil {
		return mapModelsRootError(err)
	}
	if service.artifacts == nil {
		return fmt.Errorf("model invocation artifact exporter is required")
	}
	if err := service.artifacts.ExportInvocationArtifact(streamFile, outputPath); err != nil {
		return err
	}
	_, err = fmt.Fprintf(cfg.Output, "Wrote audio: %s\n", outputPath)
	return err
}

func joinedCLIInvocationRequest(
	scope modelinference.RuntimeScopeRef,
	modelName string,
	operation string,
	text string,
	catalog modelinference.Detail,
) modelinference.InvokeModelRequest {
	inputName := "input"
	modality := modelinference.ModalityText
	contentType := "text/plain"
	if selected, ok := catalogOperationForName(catalog, operation); ok && len(selected.Inputs) > 0 {
		inputName = selected.Inputs[0].Name
		if selected.Inputs[0].Modality != "" {
			modality = selected.Inputs[0].Modality
		}
		if len(selected.Inputs[0].MediaTypes) > 0 {
			contentType = selected.Inputs[0].MediaTypes[0]
		}
	}
	return modelinference.InvokeModelRequest{
		Scope: scope, Holder: modelsCLIInvokeHolder,
		Model: modelinference.ModelReference{NameOrURI: modelName}, Operation: operation,
		Inputs: []modelinference.InferenceInput{{
			Name: inputName, Modality: modality, ContentType: contentType, Content: text,
		}},
	}
}

func modelInvocationResponseFromInferenceResult(
	result modelinference.InvokeModelResult,
	catalog modelinference.Detail,
	inputText string,
) factoryapi.ModelInvocationResponse {
	worker, locality := catalogPresentationForOperation(catalog, result.Operation)
	bindings := resolvedPresentationBindings(catalog, result.Operation, inputText)
	content := contentcontract.GeneratedPtrFromParts(inferenceContentToWorkParts(result.Content))
	return factoryapi.ModelInvocationResponse{
		ModelName:        result.ModelName,
		Worker:           worker,
		Operation:        result.Operation,
		ProviderLocality: factoryapi.WorkerModelLocality(locality),
		Content:          derefGeneratedWorkContent(content),
		Bindings:         generatedResolvedModelInvocationBindings(bindings),
	}
}

func genericInvocationResponseFromInferenceResult(
	result modelinference.InvokeModelResult,
) factoryapi.GenericModelInvocationResponse {
	outputs := make([]factoryapi.ModelInvocationOutput, len(result.Outputs))
	for index, output := range result.Outputs {
		projected := factoryapi.ModelInvocationOutput{
			Name:     output.Name,
			Modality: factoryapi.ModelInvocationContentType(output.Modality),
		}
		projected.ContentType = genericCLIStringPointer(output.ContentType)
		projected.MediaType = genericCLIStringPointer(output.MediaType)
		projected.Content = genericCLIStringPointer(output.Content)
		if output.Artifact != nil && !output.Artifact.Artifact.IsZero() {
			artifact := factoryapi.ModelInvocationArtifact{ArtifactRef: output.Artifact.Artifact.String()}
			artifact.Name = genericCLIStringPointer(output.Artifact.Name)
			artifact.MediaType = genericCLIStringPointer(output.Artifact.MediaType)
			if output.Artifact.SizeBytes >= 0 {
				size := output.Artifact.SizeBytes
				artifact.SizeBytes = &size
			}
			if len(output.Artifact.Properties) > 0 {
				properties := make(factoryapi.StringMap, len(output.Artifact.Properties))
				for key, value := range output.Artifact.Properties {
					properties[key] = value
				}
				artifact.Properties = &properties
			}
			projected.Artifact = &artifact
		}
		outputs[index] = projected
	}
	return factoryapi.GenericModelInvocationResponse{Outputs: outputs}
}

func genericCLIStringPointer(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	copy := value
	return &copy
}

func catalogPresentationForOperation(catalog modelinference.Detail, operation string) (string, string) {
	for _, capability := range catalog.Capabilities {
		for _, catalogOperation := range capability.Operations {
			if catalogOperation.Name == operation {
				return capability.Worker, string(capability.ProviderLocality)
			}
		}
	}
	return "", string(catalog.ProviderLocality)
}

func resolvedPresentationBindings(
	catalog modelinference.Detail,
	operation string,
	inputText string,
) []modelinference.ResolvedModelOperationBinding {
	operationDetail, ok := catalogOperationForName(catalog, operation)
	if !ok {
		return []modelinference.ResolvedModelOperationBinding{}
	}
	for _, input := range operationDetail.Inputs {
		slot := strings.TrimSpace(input.Name)
		if slot == "" {
			continue
		}
		return []modelinference.ResolvedModelOperationBinding{{
			Slot:   slot,
			Source: "INPUT",
			Content: []work.WorkContentPart{{
				Type: work.WorkContentPartTypeText,
				Text: inputText,
			}},
		}}
	}
	return []modelinference.ResolvedModelOperationBinding{}
}

func catalogOperationForName(catalog modelinference.Detail, operation string) (modelinference.Operation, bool) {
	for _, catalogOperation := range catalog.Operations {
		if catalogOperation.Name == operation {
			return catalogOperation, true
		}
	}
	for _, capability := range catalog.Capabilities {
		for _, catalogOperation := range capability.Operations {
			if catalogOperation.Name == operation {
				return catalogOperation, true
			}
		}
	}
	return modelinference.Operation{}, false
}

func inferenceContentToWorkParts(content []modelinference.InferenceContent) []work.WorkContentPart {
	if len(content) == 0 {
		return nil
	}
	parts := make([]work.WorkContentPart, 0, len(content))
	for _, item := range content {
		parts = append(parts, inferenceContentToWorkPart(item))
	}
	return parts
}

func inferenceContentToWorkPart(item modelinference.InferenceContent) work.WorkContentPart {
	contentType := strings.TrimSpace(item.ContentType)
	value := strings.TrimSpace(item.Content)
	switch {
	case strings.HasPrefix(strings.ToLower(contentType), "audio/"):
		return work.WorkContentPart{
			Type:        work.WorkContentPartTypeAudio,
			File:        value,
			ContentType: contentType,
			Slot:        "audio",
		}
	case strings.HasPrefix(strings.ToLower(contentType), "image/"):
		return work.WorkContentPart{
			Type:        work.WorkContentPartTypeImage,
			URL:         value,
			ContentType: contentType,
			Slot:        "image",
		}
	case strings.EqualFold(contentType, "application/json"):
		return work.WorkContentPart{
			Type: work.WorkContentPartTypeJSON,
			JSON: json.RawMessage(value),
			Slot: "json",
		}
	default:
		if contentType == "" {
			contentType = "text/plain"
		}
		return work.WorkContentPart{
			Type:        work.WorkContentPartTypeText,
			Text:        value,
			ContentType: contentType,
			Slot:        "text",
		}
	}
}

func inferenceArtifactSourcePath(result modelinference.InvokeModelResult) (string, error) {
	for _, artifact := range result.Artifacts {
		if path := strings.TrimSpace(artifact.Artifact.String()); path != "" {
			return path, nil
		}
	}
	return "", fmt.Errorf("models invoke returned no streamed audio output")
}
