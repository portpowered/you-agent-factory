package local

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	workertaxonomy "github.com/portpowered/infinite-you/pkg/workers/taxonomy"

	factoryresource "github.com/portpowered/infinite-you/pkg/factory/resource"
	workerconfig "github.com/portpowered/infinite-you/pkg/workers/config"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	managedruntime "github.com/portpowered/infinite-you/pkg/models/managedruntime"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
)

// CatalogEntry holds API summary and detail for one discovered model.
type CatalogEntry struct {
	Summary factoryapi.ModelSummary
	Detail  factoryapi.ModelDetail
}

// CatalogOptions configures runtime-aware managed-runtime projection for discovery.
type CatalogOptions struct {
	RuntimeCacheInspector RuntimeCacheInspector
	SourceResolver        ManagedRuntimeSourceResolver
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
	return BuildCatalogWithOptions(runtimeCfg, CatalogOptions{})
}

// BuildCatalogWithOptions projects configured model workers into API catalog entries
// with optional runtime cache and source resolver inputs.
func BuildCatalogWithOptions(runtimeCfg *factoryconfig.LoadedFactoryConfig, opts CatalogOptions) map[string]CatalogEntry {
	if runtimeCfg == nil || runtimeCfg.FactoryConfig() == nil {
		return map[string]CatalogEntry{}
	}

	factoryCfg := runtimeCfg.FactoryConfig()
	resourceByName := make(map[string]factoryresource.Config, len(factoryCfg.Resources))
	for _, resource := range factoryCfg.Resources {
		resourceByName[resource.Name] = resource
	}

	aggregates := make(map[string]*catalogAggregate)
	for _, worker := range factoryCfg.Workers {
		if !catalogWorkerIncludesModel(worker) {
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
			case workerconfig.ModelLocalityLocal:
				aggregate.localCount++
			case workerconfig.ModelLocalityCloud:
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
		detailDiagnostics := modelDiagnostics(*aggregate, summary)
		listManagedRuntime := buildCatalogManagedRuntime(runtimeCfg, factoryCfg, *aggregate, summary, detailDiagnostics, opts, false)
		summary.ManagedRuntime = listManagedRuntime
		inspectManagedRuntime := buildCatalogManagedRuntime(runtimeCfg, factoryCfg, *aggregate, summary, detailDiagnostics, opts, true)
		inspectDiagnostics := mergeInspectDiagnostics(detailDiagnostics, inspectManagedRuntime)
		catalog[key] = CatalogEntry{
			Summary: summary,
			Detail: factoryapi.ModelDetail{
				Name:             summary.Name,
				ManagedRuntime:   inspectManagedRuntime,
				ProviderLocality: summary.ProviderLocality,
				Status:           summary.Status,
				LoadState:        summary.LoadState,
				Operations:       summary.Operations,
				Modalities:       summary.Modalities,
				Resources:        summary.Resources,
				Capabilities:     aggregate.capabilities,
				Diagnostics:      inspectDiagnostics,
			},
		}
	}
	return catalog
}

func catalogWorkerIncludesModel(worker workerconfig.Config) bool {
	if workertaxonomy.IsInferenceWorkerType(worker.Type) {
		return true
	}
	return workertaxonomy.IsAgentWorkerType(worker.Type) &&
		strings.TrimSpace(worker.ModelLocality) == workerconfig.ModelLocalityLocal &&
		strings.TrimSpace(worker.Model) != ""
}

func capabilityFromWorker(worker workerconfig.Config) factoryapi.ModelCapability {
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

func mergeAggregateOperation(aggregate *catalogAggregate, operation workerconfig.ModelOperation) {
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
	worker workerconfig.Config,
	resourceByName map[string]factoryresource.Config,
	allResources []factoryresource.Config,
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
	if primaryModelLocality(aggregate) == workerconfig.ModelLocalityLocal {
		return factoryapi.UNLOADED
	}
	return factoryapi.NOTAPPLICABLE
}

func primaryModelLocality(aggregate catalogAggregate) string {
	switch {
	case aggregate.locality != "":
		return aggregate.locality
	case aggregate.localCount > 0:
		return workerconfig.ModelLocalityLocal
	case aggregate.cloudCount > 0:
		return workerconfig.ModelLocalityCloud
	default:
		return workerconfig.ModelLocalityCloud
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
		diagnostics["statusReason"] = "managed runtime assets are missing or not yet installed"
	} else {
		diagnostics["statusReason"] = "managed runtime is discoverable with declared operations and resources"
	}
	return diagnostics
}

func generatedModelOperation(operation workerconfig.ModelOperation) factoryapi.ModelOperation {
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

func generatedModelOperationSlot(slot workerconfig.ModelOperationSlot) factoryapi.ModelOperationSlot {
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

func generatedModelResourceSummary(resource factoryresource.Config) factoryapi.ModelResourceSummary {
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

func buildCatalogManagedRuntime(
	runtimeCfg *factoryconfig.LoadedFactoryConfig,
	factoryCfg *interfaces.FactoryConfig,
	aggregate catalogAggregate,
	summary factoryapi.ModelSummary,
	diagnostics factoryapi.StringMap,
	opts CatalogOptions,
	forInspect bool,
) factoryapi.ManagedRuntime {
	domainSummary := managedRuntimeSummary{
		name:       summary.Name,
		locality:   managedruntime.Locality(summary.ProviderLocality),
		readiness:  managedRuntimeReadiness(summary.Status),
		lifecycle:  managedRuntimeLifecycle(summary.LoadState),
		operations: managedRuntimeOperations(summary.Operations),
	}
	projection := managedRuntimeProjection{
		summary:         domainSummary,
		baseDiagnostics: diagnostics,
		includeInspect:  forInspect,
	}
	if resource := primaryModelScopedResource(aggregate, factoryCfg); resource != nil {
		if opts.SourceResolver != nil {
			resolution := opts.SourceResolver.Resolve(aggregate.name, resource)
			projection.sourceResolution = &resolution
		}
		if opts.RuntimeCacheInspector != nil && aggregate.localCount > 0 {
			inspection, err := opts.RuntimeCacheInspector.InspectRuntimeCache(context.Background(), runtimeCfg, aggregate.name)
			if err == nil {
				projection.cacheInspection = &inspection
			}
		}
	}
	return apisurface.ManagedRuntimeToAPI(buildManagedRuntimeProjection(projection))
}

func managedRuntimeReadiness(status factoryapi.ModelStatus) managedruntime.ReadinessState {
	switch status {
	case factoryapi.ModelStatusREADY:
		return managedruntime.ReadinessStateReady
	case factoryapi.ModelStatusUNAVAILABLE:
		return managedruntime.ReadinessStateMissing
	default:
		return managedruntime.ReadinessStateUnsupported
	}
}

func managedRuntimeLifecycle(loadState factoryapi.ModelLoadState) managedruntime.LifecycleState {
	if loadState == factoryapi.UNLOADED {
		return managedruntime.LifecycleStateNotInstalled
	}
	return managedruntime.LifecycleStateNotApplicable
}

func managedRuntimeOperations(operations []factoryapi.ModelOperation) []managedruntime.Operation {
	result := make([]managedruntime.Operation, 0, len(operations))
	for _, operation := range operations {
		item := managedruntime.Operation{Name: operation.Name}
		if operation.Inputs != nil {
			item.Inputs = managedRuntimeOperationSlots(*operation.Inputs)
		}
		if operation.Outputs != nil {
			item.Outputs = managedRuntimeOperationSlots(*operation.Outputs)
		}
		result = append(result, item)
	}
	return result
}

func managedRuntimeOperationSlots(slots []factoryapi.ModelOperationSlot) []managedruntime.OperationSlot {
	result := make([]managedruntime.OperationSlot, 0, len(slots))
	for _, slot := range slots {
		contentTypes := make([]string, 0, len(slot.ContentTypes))
		for _, contentType := range slot.ContentTypes {
			contentTypes = append(contentTypes, string(contentType))
		}
		result = append(result, managedruntime.OperationSlot{
			Name: slot.Name, ContentTypes: contentTypes, Required: slot.Required,
		})
	}
	return result
}

func managedRuntimeForCatalog(
	runtimeCfg *factoryconfig.LoadedFactoryConfig,
	modelName string,
	opts CatalogOptions,
) (managedruntime.Runtime, error) {
	catalog := BuildCatalogWithOptions(runtimeCfg, opts)
	key := CanonicalModelName(modelName)
	entry, ok := catalog[key]
	if !ok {
		return managedruntime.Runtime{}, fmt.Errorf("%w: %s", managedruntime.ErrNotFound, modelName)
	}
	managed := entry.Summary.ManagedRuntime
	return managedruntime.Runtime{
		Identity:            managed.Identity,
		ReadinessState:      managedruntime.ReadinessState(managed.ReadinessState),
		LifecycleState:      managedruntime.LifecycleState(managed.LifecycleState),
		Locality:            managedruntime.Locality(managed.Locality),
		SupportedOperations: managedRuntimeOperations(managed.SupportedOperations),
		Diagnostics:         cloneManagedRuntimeDiagnostics(managed.Diagnostics),
	}, nil
}

func cloneManagedRuntimeDiagnostics(values *factoryapi.StringMap) map[string]string {
	result := map[string]string{}
	if values == nil {
		return result
	}
	for key, value := range *values {
		result[key] = value
	}
	return result
}

func mergeInspectDiagnostics(base factoryapi.StringMap, managed factoryapi.ManagedRuntime) factoryapi.StringMap {
	merged := factoryapi.StringMap{}
	for key, value := range base {
		merged[key] = value
	}
	if managed.Diagnostics == nil {
		return merged
	}
	for key, value := range *managed.Diagnostics {
		merged[key] = value
	}
	return merged
}

// ListModels builds the list-models API response from runtime config.
func ListModels(runtimeCfg *factoryconfig.LoadedFactoryConfig) (factoryapi.ListModelsResponse, error) {
	return ListModelsWithOptions(runtimeCfg, CatalogOptions{})
}

// ListModelsWithOptions builds the list-models API response with runtime-aware projection.
func ListModelsWithOptions(runtimeCfg *factoryconfig.LoadedFactoryConfig, opts CatalogOptions) (factoryapi.ListModelsResponse, error) {
	if runtimeCfg == nil {
		return factoryapi.ListModelsResponse{}, fmt.Errorf("factory service runtime is not available")
	}
	catalog := BuildCatalogWithOptions(runtimeCfg, opts)
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
	return GetModelWithOptions(runtimeCfg, modelName, CatalogOptions{})
}

// GetModelWithOptions returns model detail with runtime-aware managed-runtime projection.
func GetModelWithOptions(runtimeCfg *factoryconfig.LoadedFactoryConfig, modelName string, opts CatalogOptions) (factoryapi.ModelDetail, error) {
	if runtimeCfg == nil {
		return factoryapi.ModelDetail{}, fmt.Errorf("factory service runtime is not available")
	}
	catalog := BuildCatalogWithOptions(runtimeCfg, opts)
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

// PullOptions configures runtime-aware managed-runtime projection for pull.
type PullOptions struct {
	RuntimeCacheInspector RuntimeCacheInspector
	SourceResolver        ManagedRuntimeSourceResolver
}

// PullModel validates catalog locality and delegates asset pull to the injected puller.
func PullModel(
	puller AssetPuller,
	ctx context.Context,
	runtimeCfg *factoryconfig.LoadedFactoryConfig,
	modelName string,
) (apisurface.ModelPullResult, error) {
	return PullModelWithOptions(puller, ctx, runtimeCfg, modelName, PullOptions{})
}

// PullModelWithOptions validates catalog locality, resolves backend source
// diagnostics, delegates asset pull, and projects managed readiness outcomes.
func PullModelWithOptions(
	puller AssetPuller,
	ctx context.Context,
	runtimeCfg *factoryconfig.LoadedFactoryConfig,
	modelName string,
	opts PullOptions,
) (apisurface.ModelPullResult, error) {
	if runtimeCfg == nil {
		return apisurface.ModelPullResult{}, fmt.Errorf("factory service runtime is not available")
	}
	if puller == nil {
		return apisurface.ModelPullResult{}, fmt.Errorf("model asset puller is not available")
	}
	catalog := BuildCatalog(runtimeCfg)
	key := CanonicalModelName(modelName)
	if key == "" {
		return apisurface.ModelPullResult{}, fmt.Errorf("%w: empty model name", apisurface.ErrModelNotFound)
	}
	entry, ok := catalog[key]
	if !ok {
		return apisurface.ModelPullResult{}, fmt.Errorf("%w: %s", apisurface.ErrModelNotFound, modelName)
	}
	if entry.Summary.ProviderLocality != factoryapi.WorkerModelLocalityLocal {
		return apisurface.ModelPullResult{}, fmt.Errorf("%w: model %q is not a local model", apisurface.ErrModelPullUnsupported, modelName)
	}

	var resolution ManagedRuntimeSourceResolution
	if resource := modelScopedResource(runtimeCfg.FactoryConfig(), modelName); resource != nil && opts.SourceResolver != nil {
		resolution = opts.SourceResolver.Resolve(modelName, resource)
	}

	result, err := puller.PullModel(ctx, runtimeCfg, modelName)
	if err != nil {
		switch {
		case errors.Is(err, apisurface.ErrModelNotFound), errors.Is(err, apisurface.ErrModelPullUnsupported):
			return apisurface.ModelPullResult{}, err
		default:
			pullOutcome, readiness := ClassifyPullFailure(err)
			failureResult := apisurface.ModelPullResult{
				ModelName:          strings.TrimSpace(entry.Summary.Name),
				ProviderLocality:   string(entry.Summary.ProviderLocality),
				ManagedPullOutcome: pullOutcome,
				ReadinessState:     readiness,
				LifecycleState:     managedLifecycleNotInstalled,
				SourceKind:         strings.TrimSpace(resolution.SourceKind),
				SourceID:           strings.TrimSpace(resolution.SourceID),
				ResolverNotes:      strings.TrimSpace(resolution.ResolverNotes),
			}
			return failureResult, &apisurface.ManagedRuntimePullError{Result: failureResult, Cause: err}
		}
	}

	inspection := RuntimeCacheInspection{}
	if opts.RuntimeCacheInspector != nil {
		inspected, inspectErr := opts.RuntimeCacheInspector.InspectRuntimeCache(ctx, runtimeCfg, modelName)
		if inspectErr == nil {
			inspection = inspected
		}
	}
	return EnrichPullResult(result, inspection, resolution), nil
}

func modelScopedResource(factoryCfg *interfaces.FactoryConfig, modelName string) *factoryresource.Config {
	if factoryCfg == nil {
		return nil
	}
	key := canonicalModelName(modelName)
	for _, resource := range factoryCfg.Resources {
		if strings.TrimSpace(resource.Type) != factoryresource.TypeModel {
			continue
		}
		if canonicalModelName(resource.Model) != key {
			continue
		}
		copied := resource
		return &copied
	}
	return nil
}

// SelectInvocationWorker resolves the model worker and operation for direct invocation.
func SelectInvocationWorker(
	runtimeCfg *factoryconfig.LoadedFactoryConfig,
	modelName, operationName string,
) (*workerconfig.Config, workerconfig.ModelOperation, error) {
	if runtimeCfg == nil || runtimeCfg.FactoryConfig() == nil {
		return nil, workerconfig.ModelOperation{}, fmt.Errorf("runtime config is not available")
	}
	modelKey := CanonicalModelName(modelName)
	operationName = strings.TrimSpace(operationName)
	if modelKey == "" {
		return nil, workerconfig.ModelOperation{}, fmt.Errorf("%w: empty model name", apisurface.ErrModelNotFound)
	}
	if operationName == "" {
		return nil, workerconfig.ModelOperation{}, fmt.Errorf("operation is required")
	}

	var modelMatched bool
	var matchedWorkerName string
	for _, worker := range runtimeCfg.FactoryConfig().Workers {
		workerDef, ok := runtimeCfg.Worker(worker.Name)
		if !ok || workerDef == nil || !workertaxonomy.IsInferenceWorkerType(workerDef.Type) {
			continue
		}
		if CanonicalModelName(workerDef.Model) != modelKey {
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
		return nil, workerconfig.ModelOperation{}, fmt.Errorf("%w: worker %q for model %q does not support operation %q", apisurface.ErrModelInvocationUnsupportedOperation, matchedWorkerName, modelName, operationName)
	}
	return nil, workerconfig.ModelOperation{}, fmt.Errorf("%w: %s", apisurface.ErrModelNotFound, modelName)
}

const (
	legacyPullOutcomePulled         = "PULLED"
	legacyPullOutcomeAlreadyPresent = "ALREADY_PRESENT"

	managedPullOutcomeAlreadyReady          = "ALREADY_READY"
	managedPullOutcomeInstalledSuccessfully = "INSTALLED_SUCCESSFULLY"
	managedPullOutcomeAlreadyPresent        = "ALREADY_PRESENT"
	managedPullOutcomeStillLoading          = "STILL_LOADING"
	managedPullOutcomeTimedOut              = "TIMED_OUT"
	managedPullOutcomeSourceFetchFailed     = "SOURCE_FETCH_FAILED"
	managedPullOutcomeUnsupportedRuntime    = "UNSUPPORTED_RUNTIME"

	managedReadinessReady       = "READY"
	managedReadinessMissing     = "MISSING"
	managedReadinessLoading     = "LOADING"
	managedReadinessFailed      = "FAILED"
	managedReadinessUnsupported = "UNSUPPORTED"

	managedLifecycleInstalling   = "INSTALLING"
	managedLifecycleInstalled    = "INSTALLED"
	managedLifecycleNotInstalled = "NOT_INSTALLED"
)

// EnrichPullResult projects a service-owned pull result into managed-runtime
// readiness, lifecycle, and source diagnostics using post-pull cache inspection.
func EnrichPullResult(
	result apisurface.ModelPullResult,
	inspection RuntimeCacheInspection,
	resolution ManagedRuntimeSourceResolution,
) apisurface.ModelPullResult {
	outcome, readiness, lifecycle := classifySuccessfulPull(result, inspection)
	result.ManagedPullOutcome = outcome
	result.ReadinessState = readiness
	result.LifecycleState = lifecycle
	result.SourceKind = strings.TrimSpace(resolution.SourceKind)
	result.SourceID = strings.TrimSpace(resolution.SourceID)
	result.ResolverNotes = strings.TrimSpace(resolution.ResolverNotes)
	return result
}

// ClassifyPullFailure maps pull errors to managed-runtime pull outcomes and
// readiness states for logging, metrics, and stable customer-facing vocabulary.
func ClassifyPullFailure(err error) (pullOutcome string, readiness string) {
	if err == nil {
		return "", ""
	}
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return managedPullOutcomeTimedOut, managedReadinessFailed
	case errors.Is(err, apisurface.ErrModelPullUnsupported):
		return managedPullOutcomeUnsupportedRuntime, managedReadinessUnsupported
	case errors.Is(err, apisurface.ErrManagedRuntimeSourceFetchFailed):
		return managedPullOutcomeSourceFetchFailed, managedReadinessFailed
	case isSourceFetchFailureMessage(err.Error()):
		return managedPullOutcomeSourceFetchFailed, managedReadinessFailed
	default:
		return managedPullOutcomeSourceFetchFailed, managedReadinessFailed
	}
}

func classifySuccessfulPull(result apisurface.ModelPullResult, inspection RuntimeCacheInspection) (pullOutcome, readiness, lifecycle string) {
	legacyOutcome := strings.ToUpper(strings.TrimSpace(result.Outcome))
	switch legacyOutcome {
	case legacyPullOutcomePulled:
		pullOutcome = managedPullOutcomeInstalledSuccessfully
	case legacyPullOutcomeAlreadyPresent:
		pullOutcome = managedPullOutcomeAlreadyPresent
	default:
		pullOutcome = managedPullOutcomeUnsupportedRuntime
	}

	if inspection.Supported {
		if inspection.Installed {
			readiness = managedReadinessReady
			lifecycle = managedLifecycleInstalled
			if pullOutcome == managedPullOutcomeAlreadyPresent {
				pullOutcome = managedPullOutcomeAlreadyReady
			}
			return pullOutcome, readiness, lifecycle
		}
		readiness = managedReadinessMissing
		lifecycle = managedLifecycleNotInstalled
		if pullOutcome == managedPullOutcomeInstalledSuccessfully {
			readiness = managedReadinessLoading
			lifecycle = managedLifecycleInstalling
			pullOutcome = managedPullOutcomeStillLoading
		}
		return pullOutcome, readiness, lifecycle
	}

	readiness = managedReadinessReady
	lifecycle = managedLifecycleInstalled
	if pullOutcome == managedPullOutcomeAlreadyPresent {
		pullOutcome = managedPullOutcomeAlreadyReady
	}
	return pullOutcome, readiness, lifecycle
}

func isSourceFetchFailureMessage(message string) bool {
	trimmed := strings.ToLower(strings.TrimSpace(message))
	if trimmed == "" {
		return false
	}
	for _, fragment := range []string{
		"pull model manifest",
		"download model asset",
		"model asset request failed",
		"checksum verification",
	} {
		if strings.Contains(trimmed, fragment) {
			return true
		}
	}
	return false
}
