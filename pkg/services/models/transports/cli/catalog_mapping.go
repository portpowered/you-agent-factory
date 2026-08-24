package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	contentcontract "github.com/portpowered/infinite-you/pkg/transports/mapping/workcontent"
)

func summaryToGenerated(summary models.Summary) factoryapi.ModelSummary {
	return factoryapi.ModelSummary{
		Name: summary.Name, ProviderLocality: factoryapi.WorkerModelLocality(summary.ProviderLocality),
		Status: factoryapi.ModelStatus(summary.Status), LoadState: factoryapi.ModelLoadState(summary.LoadState),
		Operations: operationsToGenerated(summary.Operations), Modalities: modalitiesToGenerated(summary.Modalities),
		Resources: resourcesToGenerated(summary.Resources), ManagedRuntime: managedRuntimeToGenerated(summary.ManagedRuntime),
	}
}

func listToGenerated(list models.List) factoryapi.ListModelsResponse {
	results := make([]factoryapi.ModelSummary, 0, len(list.Results))
	for _, summary := range list.Results {
		results = append(results, summaryToGenerated(summary))
	}
	return factoryapi.ListModelsResponse{Results: results}
}

func detailToGenerated(detail models.Detail) factoryapi.ModelDetail {
	return factoryapi.ModelDetail{
		Name: detail.Name, ProviderLocality: factoryapi.WorkerModelLocality(detail.ProviderLocality),
		Status: factoryapi.ModelStatus(detail.Status), LoadState: factoryapi.ModelLoadState(detail.LoadState),
		Operations: operationsToGenerated(detail.Operations), Modalities: modalitiesToGenerated(detail.Modalities),
		Resources: resourcesToGenerated(detail.Resources), ManagedRuntime: managedRuntimeToGenerated(detail.ManagedRuntime),
		Capabilities: capabilitiesToGenerated(detail.Capabilities), Diagnostics: cloneDiagnostics(detail.Diagnostics),
	}
}

func operationsToGenerated(operations []models.Operation) []factoryapi.ModelInvocationOperation {
	converted := make([]factoryapi.ModelInvocationOperation, 0, len(operations))
	for _, operation := range operations {
		item := factoryapi.ModelInvocationOperation{Name: operation.Name}
		if operation.Inputs != nil {
			inputs := slotsToGenerated(operation.Inputs)
			item.Inputs = &inputs
		}
		if operation.Outputs != nil {
			outputs := slotsToGenerated(operation.Outputs)
			item.Outputs = &outputs
		}
		converted = append(converted, item)
	}
	return converted
}

func slotsToGenerated(slots []models.OperationSlot) []factoryapi.ModelInvocationSlot {
	converted := make([]factoryapi.ModelInvocationSlot, 0, len(slots))
	for _, slot := range slots {
		contentTypes := make([]factoryapi.ModelInvocationContentType, 0, len(slot.ContentTypes))
		for _, contentType := range slot.ContentTypes {
			contentTypes = append(contentTypes, factoryapi.ModelInvocationContentType(contentType))
		}
		converted = append(converted, factoryapi.ModelInvocationSlot{
			Name:         slot.Name,
			ContentTypes: contentTypes,
			Modality:     modelOperationModality(slot.Modality),
			Required:     slot.Required,
			Repeatable:   modelOperationRepeatable(slot),
			MediaTypes:   modelOperationMediaTypes(slot.MediaTypes),
		})
	}
	return converted
}

func modelOperationModality(modality models.Modality) *factoryapi.ModelInvocationContentType {
	if modality == "" {
		return nil
	}
	value := factoryapi.ModelInvocationContentType(modality)
	return &value
}

func modelOperationRepeatable(slot models.OperationSlot) *bool {
	if slot.Modality == "" && slot.MediaTypes == nil {
		return nil
	}
	value := slot.Repeatable
	return &value
}

func modelOperationMediaTypes(mediaTypes []string) *[]string {
	if mediaTypes == nil {
		return nil
	}
	copied := append([]string(nil), mediaTypes...)
	return &copied
}

func modalitiesToGenerated(modalities []string) []factoryapi.ModelInvocationContentType {
	converted := make([]factoryapi.ModelInvocationContentType, 0, len(modalities))
	for _, modality := range modalities {
		converted = append(converted, factoryapi.ModelInvocationContentType(modality))
	}
	return converted
}

func resourcesToGenerated(resources []models.ResourceSummary) []factoryapi.ModelResourceSummary {
	converted := make([]factoryapi.ModelResourceSummary, 0, len(resources))
	for _, resource := range resources {
		converted = append(converted, factoryapi.ModelResourceSummary{
			Name: resource.Name, Type: factoryapi.ResourceType(resource.Type), Capacity: resource.Capacity,
			Model: resource.Model, Backend: resource.Backend, LoadPolicy: resource.LoadPolicy, Provider: resource.Provider,
		})
	}
	return converted
}

func capabilitiesToGenerated(capabilities []models.Capability) []factoryapi.ModelCapability {
	converted := make([]factoryapi.ModelCapability, 0, len(capabilities))
	for _, capability := range capabilities {
		var provider *factoryapi.WorkerModelProvider
		if capability.ModelProvider != nil {
			value := factoryapi.WorkerModelProvider(*capability.ModelProvider)
			provider = &value
		}
		converted = append(converted, factoryapi.ModelCapability{
			Worker: capability.Worker, ProviderLocality: factoryapi.WorkerModelLocality(capability.ProviderLocality),
			ModelProvider: provider, Operations: operationsToGenerated(capability.Operations),
			ResourceNames: append([]string(nil), capability.ResourceNames...),
		})
	}
	return converted
}

func cloneDiagnostics(values map[string]string) factoryapi.StringMap {
	cloned := make(factoryapi.StringMap, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func managedRuntimeToGenerated(runtime models.Runtime) factoryapi.ManagedRuntime {
	diagnostics := factoryapi.StringMap{}
	for key, value := range runtime.Diagnostics {
		diagnostics[key] = value
	}
	return factoryapi.ManagedRuntime{
		Identity:            runtime.Identity,
		Revision:            runtime.Revision,
		CachePath:           runtime.CachePath,
		CacheBytes:          runtime.CacheBytes,
		ReadinessState:      factoryapi.ManagedRuntimeReadinessState(runtime.ReadinessState),
		LifecycleState:      factoryapi.ManagedRuntimeLifecycleState(runtime.LifecycleState),
		Locality:            factoryapi.WorkerModelLocality(runtime.Locality),
		SupportedOperations: operationsToGenerated(runtime.SupportedOperations),
		Diagnostics:         &diagnostics,
	}
}

func modelInvocationResponseFromInferenceResult(
	result models.InvokeModelResult,
	catalog models.Detail,
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
	result models.InvokeModelResult,
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

func catalogPresentationForOperation(catalog models.Detail, operation string) (string, string) {
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
	catalog models.Detail,
	operation string,
	inputText string,
) []models.ResolvedModelOperationBinding {
	operationDetail, ok := catalogOperationForName(catalog, operation)
	if !ok {
		return []models.ResolvedModelOperationBinding{}
	}
	for _, input := range operationDetail.Inputs {
		slot := strings.TrimSpace(input.Name)
		if slot == "" {
			continue
		}
		return []models.ResolvedModelOperationBinding{{
			Slot:   slot,
			Source: "INPUT",
			Content: []work.WorkContentPart{{
				Type: work.WorkContentPartTypeText,
				Text: inputText,
			}},
		}}
	}
	return []models.ResolvedModelOperationBinding{}
}

func catalogOperationForName(catalog models.Detail, operation string) (models.Operation, bool) {
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
	return models.Operation{}, false
}
func catalogCapabilityOperationForName(catalog models.Detail, operation string) (models.Operation, bool) {
	for _, capability := range catalog.Capabilities {
		for _, catalogOperation := range capability.Operations {
			if catalogOperation.Name == operation {
				return catalogOperation, true
			}
		}
	}
	return models.Operation{}, false
}
func effectiveCLIInvocationOperation(catalog models.Detail, operation string) string {
	if strings.TrimSpace(operation) != "" {
		return strings.TrimSpace(operation)
	}
	if len(catalog.Operations) == 1 {
		return catalog.Operations[0].Name
	}
	return ""
}

func inferenceContentToWorkParts(content []models.InferenceContent) []work.WorkContentPart {
	if len(content) == 0 {
		return nil
	}
	parts := make([]work.WorkContentPart, 0, len(content))
	for _, item := range content {
		parts = append(parts, inferenceContentToWorkPart(item))
	}
	return parts
}

func inferenceContentToWorkPart(item models.InferenceContent) work.WorkContentPart {
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

func inferenceArtifactSourcePath(result models.InvokeModelResult) (string, error) {
	for _, artifact := range result.Artifacts {
		if path := strings.TrimSpace(artifact.Artifact.String()); path != "" {
			return path, nil
		}
	}
	return "", fmt.Errorf("models invoke returned no streamed audio output")
}
