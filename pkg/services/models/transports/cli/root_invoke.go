package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	modelinference "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	contentcontract "github.com/portpowered/infinite-you/pkg/transports/mapping/workcontent"
)

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
	if !cfg.JSON && strings.TrimSpace(cfg.OutputPath) == "" {
		return fmt.Errorf("--output is required unless --json is set")
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
	catalogResult, err := service.models.GetCatalogModel(cfg.Context, modelinference.GetModelRequest{
		Scope: scope.Scope, Name: modelName, Operation: operation,
	})
	if err != nil {
		return mapModelsRootError(err)
	}
	leaseResult, err := service.models.AcquireModelLease(cfg.Context, modelinference.AcquireModelLeaseRequest{
		Scope: scope.Scope, Name: modelName, Holder: modelsCLIInvokeHolder,
	})
	if err != nil {
		return mapModelsRootError(err)
	}
	request := modelinference.InvokeModelRequest{
		Scope:     scope.Scope,
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
		response := modelInvocationResponseFromInferenceResult(result, catalogResult.Model, text)
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
