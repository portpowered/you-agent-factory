package modelcatalog

import (
	modelcatalog "github.com/portpowered/infinite-you/pkg/models/catalog"
	managedruntime "github.com/portpowered/infinite-you/pkg/models/managedruntime"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
)

func SummaryToGenerated(summary modelcatalog.Summary) factoryapi.ModelSummary {
	return factoryapi.ModelSummary{
		Name:             summary.Name,
		ProviderLocality: factoryapi.WorkerModelLocality(summary.ProviderLocality),
		Status:           factoryapi.ModelStatus(summary.Status),
		LoadState:        factoryapi.ModelLoadState(summary.LoadState),
		Operations:       operationsToGenerated(summary.Operations),
		Modalities:       modalitiesToGenerated(summary.Modalities),
		Resources:        resourcesToGenerated(summary.Resources),
		ManagedRuntime:   apisurface.ManagedRuntimeToAPI(summary.ManagedRuntime),
	}
}

func ListToGenerated(models modelcatalog.List) factoryapi.ListModelsResponse {
	results := make([]factoryapi.ModelSummary, 0, len(models.Results))
	for _, summary := range models.Results {
		results = append(results, SummaryToGenerated(summary))
	}
	return factoryapi.ListModelsResponse{Results: results}
}

func DetailToGenerated(detail modelcatalog.Detail) factoryapi.ModelDetail {
	return factoryapi.ModelDetail{
		Name:             detail.Name,
		ProviderLocality: factoryapi.WorkerModelLocality(detail.ProviderLocality),
		Status:           factoryapi.ModelStatus(detail.Status),
		LoadState:        factoryapi.ModelLoadState(detail.LoadState),
		Operations:       operationsToGenerated(detail.Operations),
		Modalities:       modalitiesToGenerated(detail.Modalities),
		Resources:        resourcesToGenerated(detail.Resources),
		ManagedRuntime:   apisurface.ManagedRuntimeToAPI(detail.ManagedRuntime),
		Capabilities:     capabilitiesToGenerated(detail.Capabilities),
		Diagnostics:      cloneDiagnostics(detail.Diagnostics),
	}
}

func operationsToGenerated(operations []managedruntime.Operation) []factoryapi.ModelOperation {
	converted := make([]factoryapi.ModelOperation, 0, len(operations))
	for _, operation := range operations {
		item := factoryapi.ModelOperation{Name: operation.Name}
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

func slotsToGenerated(slots []managedruntime.OperationSlot) []factoryapi.ModelOperationSlot {
	converted := make([]factoryapi.ModelOperationSlot, 0, len(slots))
	for _, slot := range slots {
		contentTypes := make([]factoryapi.ModelOperationContentType, 0, len(slot.ContentTypes))
		for _, contentType := range slot.ContentTypes {
			contentTypes = append(contentTypes, factoryapi.ModelOperationContentType(contentType))
		}
		converted = append(converted, factoryapi.ModelOperationSlot{
			Name: slot.Name, ContentTypes: contentTypes, Required: slot.Required,
		})
	}
	return converted
}

func modalitiesToGenerated(modalities []string) []factoryapi.ModelOperationContentType {
	converted := make([]factoryapi.ModelOperationContentType, 0, len(modalities))
	for _, modality := range modalities {
		converted = append(converted, factoryapi.ModelOperationContentType(modality))
	}
	return converted
}

func resourcesToGenerated(resources []modelcatalog.ResourceSummary) []factoryapi.ModelResourceSummary {
	converted := make([]factoryapi.ModelResourceSummary, 0, len(resources))
	for _, resource := range resources {
		converted = append(converted, factoryapi.ModelResourceSummary{
			Name: resource.Name, Type: factoryapi.ResourceType(resource.Type), Capacity: resource.Capacity,
			Model: resource.Model, Backend: resource.Backend, LoadPolicy: resource.LoadPolicy, Provider: resource.Provider,
		})
	}
	return converted
}

func capabilitiesToGenerated(capabilities []modelcatalog.Capability) []factoryapi.ModelCapability {
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
