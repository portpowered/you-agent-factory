package cli

import (
	"github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
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
			Name: slot.Name, ContentTypes: contentTypes, Modality: modelOperationModality(slot.Modality),
			Required: slot.Required, Repeatable: modelOperationRepeatable(slot), MediaTypes: modelOperationMediaTypes(slot.MediaTypes),
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

// genericCLIModelDetailFromGenerated projects the remote catalog detail back
// into the Models-owned operation vocabulary used by the shared CLI input
// preparation helpers. It intentionally maps only detached catalog facts; the
// remote runtime remains authoritative for scope, readiness, and invocation.
func genericCLIModelDetailFromGenerated(model factoryapi.ModelDetail) models.Detail {
	detail := models.Detail{Summary: models.Summary{
		Name:             model.Name,
		ProviderLocality: models.Locality(model.ProviderLocality),
		Status:           models.Status(model.Status),
		LoadState:        models.LoadState(model.LoadState),
	}}
	detail.Operations = genericCLIOperationsFromGenerated(model.Operations)
	if len(detail.Operations) == 0 {
		detail.Operations = genericCLIOperationsFromGenerated(model.ManagedRuntime.SupportedOperations)
	}
	detail.Capabilities = make([]models.Capability, 0, len(model.Capabilities))
	for _, capability := range model.Capabilities {
		detail.Capabilities = append(detail.Capabilities, models.Capability{
			Worker:           capability.Worker,
			ProviderLocality: models.Locality(capability.ProviderLocality),
			Operations:       genericCLIOperationsFromGenerated(capability.Operations),
			ResourceNames:    append([]string(nil), capability.ResourceNames...),
		})
	}
	return detail
}

func genericCLIOperationsFromGenerated(
	operations []factoryapi.ModelInvocationOperation,
) []models.Operation {
	converted := make([]models.Operation, 0, len(operations))
	for _, operation := range operations {
		item := models.Operation{Name: operation.Name}
		if operation.Inputs != nil {
			item.Inputs = genericCLISlotsFromGenerated(*operation.Inputs)
		}
		if operation.Outputs != nil {
			item.Outputs = genericCLISlotsFromGenerated(*operation.Outputs)
		}
		converted = append(converted, item)
	}
	return converted
}

func genericCLISlotsFromGenerated(
	slots []factoryapi.ModelInvocationSlot,
) []models.OperationSlot {
	converted := make([]models.OperationSlot, 0, len(slots))
	for _, slot := range slots {
		item := models.OperationSlot{
			Name:         slot.Name,
			Modality:     models.Modality(stringValue(slot.Modality)),
			Required:     slot.Required,
			ContentTypes: make([]string, 0, len(slot.ContentTypes)),
		}
		for _, contentType := range slot.ContentTypes {
			item.ContentTypes = append(item.ContentTypes, string(contentType))
		}
		if slot.Repeatable != nil {
			item.Repeatable = *slot.Repeatable
		}
		if slot.MediaTypes != nil {
			item.MediaTypes = append([]string(nil), (*slot.MediaTypes)...)
		}
		converted = append(converted, item)
	}
	return converted
}

func stringValue(value *factoryapi.ModelInvocationContentType) string {
	if value == nil {
		return ""
	}
	return string(*value)
}

func managedRuntimeToGenerated(runtime models.Runtime) factoryapi.ManagedRuntime {
	diagnostics := factoryapi.StringMap{}
	for key, value := range runtime.Diagnostics {
		diagnostics[key] = value
	}
	return factoryapi.ManagedRuntime{
		Identity: runtime.Identity, Revision: runtime.Revision, CachePath: runtime.CachePath,
		CacheBytes:          runtime.CacheBytes,
		ReadinessState:      factoryapi.ManagedRuntimeReadinessState(runtime.ReadinessState),
		LifecycleState:      factoryapi.ManagedRuntimeLifecycleState(runtime.LifecycleState),
		Locality:            factoryapi.WorkerModelLocality(runtime.Locality),
		SupportedOperations: operationsToGenerated(runtime.SupportedOperations), Diagnostics: &diagnostics,
	}
}
