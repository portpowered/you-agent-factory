package localmodels

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

// CatalogEntry holds API summary and detail for one discovered model.
type CatalogEntry struct {
	Summary factoryapi.ModelSummary
	Detail  factoryapi.ModelDetail
}

type catalogAggregate struct {
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

// BuildCatalog projects configured model workers into API catalog entries.
func BuildCatalog(runtimeCfg *factoryconfig.LoadedFactoryConfig) map[string]CatalogEntry {
	if runtimeCfg == nil || runtimeCfg.FactoryConfig() == nil {
		return map[string]CatalogEntry{}
	}

	factoryCfg := runtimeCfg.FactoryConfig()
	resourceByName := make(map[string]interfaces.ResourceConfig, len(factoryCfg.Resources))
	for _, resource := range factoryCfg.Resources {
		resourceByName[resource.Name] = resource
	}

	aggregates := make(map[string]*catalogAggregate)
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
			aggregate = &catalogAggregate{
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

	catalog := make(map[string]CatalogEntry, len(aggregates))
	for key, aggregate := range aggregates {
		sort.Strings(aggregate.workerNames)
		sort.Slice(aggregate.capabilities, func(i, j int) bool {
			return aggregate.capabilities[i].Worker < aggregate.capabilities[j].Worker
		})
		summary := buildModelSummary(*aggregate)
		catalog[key] = CatalogEntry{
			Summary: summary,
			Detail: factoryapi.ModelDetail{
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

func mergeAggregateOperation(aggregate *catalogAggregate, operation interfaces.ModelOperation) {
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
	aggregate *catalogAggregate,
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

func buildModelSummary(aggregate catalogAggregate) factoryapi.ModelSummary {
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

func modelStatus(aggregate catalogAggregate) factoryapi.ModelStatus {
	if aggregate.localCount > 0 && !aggregate.hasModelScoped {
		return factoryapi.ModelStatusUNAVAILABLE
	}
	return factoryapi.ModelStatusREADY
}

func modelLoadState(aggregate catalogAggregate) factoryapi.ModelLoadState {
	if primaryModelLocality(aggregate) == interfaces.ModelLocalityLocal {
		return factoryapi.UNLOADED
	}
	return factoryapi.NOTAPPLICABLE
}

func primaryModelLocality(aggregate catalogAggregate) string {
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

func modelDiagnostics(aggregate catalogAggregate, summary factoryapi.ModelSummary) factoryapi.StringMap {
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

// ResourceSummary maps a factory resource config to the API model-resource summary shape.
func ResourceSummary(resource interfaces.ResourceConfig) factoryapi.ModelResourceSummary {
	return generatedModelResourceSummary(resource)
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


// ListModels builds the list-models API response from runtime config.
func ListModels(runtimeCfg *factoryconfig.LoadedFactoryConfig) (factoryapi.ListModelsResponse, error) {
	if runtimeCfg == nil {
		return factoryapi.ListModelsResponse{}, fmt.Errorf("factory service runtime is not available")
	}
	catalog := BuildCatalog(runtimeCfg)
	results := make([]factoryapi.ModelSummary, 0, len(catalog))
	for _, entry := range catalog {
		results = append(results, entry.Summary)
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].Name < results[j].Name
	})
	return factoryapi.ListModelsResponse{Results: results}, nil
}

// GetModel returns model detail for a catalog model name.
func GetModel(runtimeCfg *factoryconfig.LoadedFactoryConfig, modelName string) (factoryapi.ModelDetail, error) {
	if runtimeCfg == nil {
		return factoryapi.ModelDetail{}, fmt.Errorf("factory service runtime is not available")
	}
	catalog := BuildCatalog(runtimeCfg)
	key := CanonicalModelName(modelName)
	if key == "" {
		return factoryapi.ModelDetail{}, fmt.Errorf("%w: empty model name", apisurface.ErrModelNotFound)
	}
	entry, ok := catalog[key]
	if !ok {
		return factoryapi.ModelDetail{}, fmt.Errorf("%w: %s", apisurface.ErrModelNotFound, modelName)
	}
	return entry.Detail, nil
}

// CanonicalModelName normalizes model identifiers for catalog lookup.
func CanonicalModelName(model string) string {
	return canonicalModelName(model)
}