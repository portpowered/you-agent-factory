package local

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	managedruntime "github.com/portpowered/infinite-you/pkg/services/models/internal/managedruntime"
	pullsupport "github.com/portpowered/infinite-you/pkg/services/models/internal/pullsupport"
	modelcatalog "github.com/portpowered/infinite-you/pkg/services/models/internal/services/catalog"
)

type CatalogEntry = modelcatalog.Entry

type catalogAggregate struct {
	name           string
	locality       string
	localities     map[string]struct{}
	workerCount    int
	localCount     int
	cloudCount     int
	operations     map[string]managedruntime.Operation
	modalities     map[string]struct{}
	resources      map[string]modelcatalog.ResourceSummary
	capabilities   []modelcatalog.Capability
	workerNames    []string
	hasAnyResource bool
	hasModelScoped bool
}

// BuildCatalog projects configured model workers into API catalog entries.
func BuildCatalog(runtimeCfg *models.RuntimeConfig) map[string]CatalogEntry {
	return BuildCatalogWithRuntime(runtimeCfg, nil, nil)
}

// BuildCatalogWithRuntime projects configured model workers with direct
// runtime cache and source-resolution collaborators.
func BuildCatalogWithRuntime(
	runtimeCfg *models.RuntimeConfig,
	runtimeCacheInspector RuntimeCacheInspector,
	sourceResolver ManagedRuntimeSourceResolver,
) map[string]CatalogEntry {
	if runtimeCfg == nil {
		return map[string]CatalogEntry{}
	}

	factoryCfg := runtimeCfg
	resourceByName := make(map[string]models.RuntimeResource, len(factoryCfg.Resources))
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
				operations: make(map[string]managedruntime.Operation),
				modalities: make(map[string]struct{}),
				resources:  make(map[string]modelcatalog.ResourceSummary),
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
			case models.RuntimeModelLocalityLocal:
				aggregate.localCount++
			case models.RuntimeModelLocalityCloud:
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
		listManagedRuntime := buildCatalogManagedRuntime(runtimeCfg, factoryCfg, *aggregate, summary, detailDiagnostics, runtimeCacheInspector, sourceResolver, false)
		summary.ManagedRuntime = listManagedRuntime
		inspectManagedRuntime := buildCatalogManagedRuntime(runtimeCfg, factoryCfg, *aggregate, summary, detailDiagnostics, runtimeCacheInspector, sourceResolver, true)
		inspectDiagnostics := mergeInspectDiagnostics(detailDiagnostics, inspectManagedRuntime)
		catalog[key] = modelcatalog.Entry{
			Summary: summary,
			Detail: modelcatalog.Detail{
				Summary: modelcatalog.Summary{
					Name: summary.Name, ManagedRuntime: inspectManagedRuntime,
					ProviderLocality: summary.ProviderLocality, Status: summary.Status,
					LoadState: summary.LoadState, Operations: summary.Operations,
					Modalities: summary.Modalities, Resources: summary.Resources,
				},
				Capabilities: aggregate.capabilities,
				Diagnostics:  inspectDiagnostics,
			},
		}
	}
	return catalog
}

func catalogWorkerIncludesModel(worker models.RuntimeWorker) bool {
	if (models.RuntimeWorker{Type: worker.Type}).IsInference() {
		return true
	}
	return (models.RuntimeWorker{Type: worker.Type}).IsAgent() &&
		strings.TrimSpace(worker.ModelLocality) == models.RuntimeModelLocalityLocal &&
		strings.TrimSpace(worker.Model) != ""
}

func capabilityFromWorker(worker models.RuntimeWorker) modelcatalog.Capability {
	operations := make([]managedruntime.Operation, 0, len(worker.Operations))
	for _, operation := range worker.Operations {
		operations = append(operations, catalogModelOperation(operation))
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

	capability := modelcatalog.Capability{
		Worker:           worker.Name,
		ProviderLocality: managedruntime.Locality(strings.TrimSpace(worker.ModelLocality)),
		Operations:       operations,
		ResourceNames:    resourceNames,
	}
	if provider := strings.TrimSpace(worker.ModelProvider); provider != "" {
		capability.ModelProvider = &provider
	}
	return capability
}

func mergeAggregateOperation(aggregate *catalogAggregate, operation models.RuntimeOperation) {
	if aggregate == nil {
		return
	}
	key := strings.TrimSpace(operation.Name)
	if key == "" {
		return
	}
	merged := catalogModelOperationForModel(aggregate.name, operation)
	if existing, ok := aggregate.operations[key]; ok {
		merged = mergeCatalogOperations(existing, merged)
	}
	aggregate.operations[key] = merged
	for _, slot := range operation.Inputs {
		for _, contentType := range slot.ContentTypes {
			if normalized := strings.TrimSpace(contentType); normalized != "" {
				aggregate.modalities[normalized] = struct{}{}
			}
		}
	}
	for _, slot := range operation.Outputs {
		for _, contentType := range slot.ContentTypes {
			if normalized := strings.TrimSpace(contentType); normalized != "" {
				aggregate.modalities[normalized] = struct{}{}
			}
		}
	}
}

// catalogModelOperationForModel keeps the canonical provider-neutral shape
// for built-in model names when an authored worker supplies the host and
// resource binding. Authored worker fields still overlay the operation, but
// the worker cannot accidentally remove built-in repeatability or a modality
// that the Models-owned contract already publishes.
func catalogModelOperationForModel(modelName string, operation models.RuntimeOperation) managedruntime.Operation {
	authored := catalogModelOperation(operation)
	definition, ok := (models.BuiltInCatalog{}).ModelDefinitionFor(modelName)
	if !ok {
		return authored
	}
	for _, builtinOperation := range definition.Operations {
		if !strings.EqualFold(strings.TrimSpace(builtinOperation.Name), strings.TrimSpace(operation.Name)) {
			continue
		}
		return mergeCatalogOperations(builtinOperation, authored)
	}
	return authored
}

func mergeCatalogOperations(left, right managedruntime.Operation) managedruntime.Operation {
	merged := left
	merged.Inputs = mergeCatalogOperationSlots(left.Inputs, right.Inputs)
	merged.Outputs = mergeCatalogOperationSlots(left.Outputs, right.Outputs)
	return merged
}

func mergeCatalogOperationSlots(left, right []managedruntime.OperationSlot) []managedruntime.OperationSlot {
	slotByName := make(map[string]managedruntime.OperationSlot)
	order := make([]string, 0)
	appendSlots := func(slots []managedruntime.OperationSlot) {
		for _, slot := range slots {
			existing, ok := slotByName[slot.Name]
			if !ok {
				slotByName[slot.Name] = slot
				order = append(order, slot.Name)
				continue
			}
			slotByName[slot.Name] = mergeCatalogOperationSlot(existing, slot)
		}
	}
	appendSlots(left)
	appendSlots(right)
	if len(order) == 0 {
		return nil
	}
	merged := make([]managedruntime.OperationSlot, 0, len(order))
	for _, name := range order {
		merged = append(merged, slotByName[name])
	}
	return merged
}

func mergeCatalogOperationSlot(left, right managedruntime.OperationSlot) managedruntime.OperationSlot {
	merged := left
	typeSet := make(map[string]struct{}, len(left.ContentTypes)+len(right.ContentTypes))
	ordered := make([]string, 0, len(left.ContentTypes)+len(right.ContentTypes))
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
	worker models.RuntimeWorker,
	resourceByName map[string]models.RuntimeResource,
	allResources []models.RuntimeResource,
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
		aggregate.resources[resource.Name] = catalogModelResourceSummary(resource)
	}
	for _, resource := range allResources {
		if canonicalModelName(resource.Model) != canonicalModelName(worker.Model) {
			continue
		}
		aggregate.hasAnyResource = true
		aggregate.hasModelScoped = true
		aggregate.resources[resource.Name] = catalogModelResourceSummary(resource)
	}
}

func buildModelSummary(aggregate catalogAggregate) modelcatalog.Summary {
	operations := make([]managedruntime.Operation, 0, len(aggregate.operations))
	for _, operation := range aggregate.operations {
		operations = append(operations, operation)
	}
	sort.Slice(operations, func(i, j int) bool {
		return operations[i].Name < operations[j].Name
	})

	modalities := make([]string, 0, len(aggregate.modalities))
	for modality := range aggregate.modalities {
		modalities = append(modalities, modality)
	}
	sort.Slice(modalities, func(i, j int) bool {
		return modalities[i] < modalities[j]
	})

	resources := make([]modelcatalog.ResourceSummary, 0, len(aggregate.resources))
	for _, resource := range aggregate.resources {
		resources = append(resources, resource)
	}
	sort.Slice(resources, func(i, j int) bool {
		return resources[i].Name < resources[j].Name
	})

	return modelcatalog.Summary{
		Name:             aggregate.name,
		ProviderLocality: managedruntime.Locality(primaryModelLocality(aggregate)),
		Status:           modelStatus(aggregate),
		LoadState:        modelLoadState(aggregate),
		Operations:       operations,
		Modalities:       modalities,
		Resources:        resources,
	}
}

func modelStatus(aggregate catalogAggregate) modelcatalog.Status {
	if aggregate.localCount > 0 && !aggregate.hasModelScoped {
		return modelcatalog.StatusUnavailable
	}
	return modelcatalog.StatusReady
}

func modelLoadState(aggregate catalogAggregate) modelcatalog.LoadState {
	if primaryModelLocality(aggregate) == models.RuntimeModelLocalityLocal {
		return modelcatalog.LoadStateUnloaded
	}
	return modelcatalog.LoadStateNotApplicable
}

func primaryModelLocality(aggregate catalogAggregate) string {
	switch {
	case aggregate.locality != "":
		return aggregate.locality
	case aggregate.localCount > 0:
		return models.RuntimeModelLocalityLocal
	case aggregate.cloudCount > 0:
		return models.RuntimeModelLocalityCloud
	default:
		return models.RuntimeModelLocalityCloud
	}
}

func modelDiagnostics(aggregate catalogAggregate, summary modelcatalog.Summary) map[string]string {
	diagnostics := map[string]string{
		"workerCount":      strconv.Itoa(aggregate.workerCount),
		"localWorkerCount": strconv.Itoa(aggregate.localCount),
		"cloudWorkerCount": strconv.Itoa(aggregate.cloudCount),
		"resourceCount":    strconv.Itoa(len(summary.Resources)),
		"workers":          strings.Join(aggregate.workerNames, ","),
		"mixedLocality":    strconv.FormatBool(len(aggregate.localities) > 1),
	}
	if summary.Status == modelcatalog.StatusUnavailable {
		diagnostics["statusReason"] = "managed runtime assets are missing or not yet installed"
	} else {
		diagnostics["statusReason"] = "managed runtime is discoverable with declared operations and resources"
	}
	return diagnostics
}

func catalogModelOperation(operation models.RuntimeOperation) managedruntime.Operation {
	result := managedruntime.Operation{
		Name: strings.TrimSpace(operation.Name),
	}
	if len(operation.Inputs) > 0 {
		inputs := make([]managedruntime.OperationSlot, 0, len(operation.Inputs))
		for _, slot := range operation.Inputs {
			inputs = append(inputs, catalogModelOperationSlot(slot))
		}
		sort.Slice(inputs, func(i, j int) bool {
			return inputs[i].Name < inputs[j].Name
		})
		result.Inputs = inputs
	}
	if len(operation.Outputs) > 0 {
		outputs := make([]managedruntime.OperationSlot, 0, len(operation.Outputs))
		for _, slot := range operation.Outputs {
			outputs = append(outputs, catalogModelOperationSlot(slot))
		}
		sort.Slice(outputs, func(i, j int) bool {
			return outputs[i].Name < outputs[j].Name
		})
		result.Outputs = outputs
	}
	return result
}

func catalogModelOperationSlot(slot models.RuntimeOperationSlot) managedruntime.OperationSlot {
	contentTypes := make([]string, 0, len(slot.ContentTypes))
	for _, contentType := range slot.ContentTypes {
		if normalized := strings.TrimSpace(contentType); normalized != "" {
			contentTypes = append(contentTypes, normalized)
		}
	}
	sort.Slice(contentTypes, func(i, j int) bool {
		return contentTypes[i] < contentTypes[j]
	})
	mediaTypes := make([]string, 0, len(contentTypes))
	for _, contentType := range contentTypes {
		if mediaType := catalogOperationMediaType(contentType); mediaType != "" &&
			!containsCatalogOperationMediaType(mediaTypes, mediaType) {
			mediaTypes = append(mediaTypes, mediaType)
		}
	}
	result := managedruntime.OperationSlot{
		Name:         strings.TrimSpace(slot.Name),
		ContentTypes: contentTypes,
		MediaTypes:   mediaTypes,
	}
	if len(contentTypes) > 0 {
		result.Modality = models.Modality(strings.ToUpper(contentTypes[0]))
	}
	if slot.Required {
		required := true
		result.Required = &required
	}
	return result
}

func catalogOperationMediaType(contentType string) string {
	switch strings.ToUpper(strings.TrimSpace(contentType)) {
	case string(models.ModalityText):
		return "text/plain"
	case string(models.ModalityJSON):
		return "application/json"
	case string(models.ModalityAudio):
		return "audio/*"
	case string(models.ModalityImage):
		return "image/*"
	case string(models.ModalityVideo):
		return "video/*"
	case string(models.ModalityBinary):
		return "application/octet-stream"
	default:
		if strings.Contains(strings.TrimSpace(contentType), "/") {
			return strings.TrimSpace(contentType)
		}
		return ""
	}
}

func containsCatalogOperationMediaType(mediaTypes []string, candidate string) bool {
	for _, mediaType := range mediaTypes {
		if strings.EqualFold(mediaType, candidate) {
			return true
		}
	}
	return false
}

func catalogModelResourceSummary(resource models.RuntimeResource) modelcatalog.ResourceSummary {
	summary := modelcatalog.ResourceSummary{
		Name:     resource.Name,
		Type:     strings.TrimSpace(resource.Type),
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
	runtimeCfg *models.RuntimeConfig,
	factoryCfg *models.RuntimeConfig,
	aggregate catalogAggregate,
	summary modelcatalog.Summary,
	diagnostics map[string]string,
	runtimeCacheInspector RuntimeCacheInspector,
	sourceResolver ManagedRuntimeSourceResolver,
	forInspect bool,
) managedruntime.Runtime {
	domainSummary := managedRuntimeSummary{
		name:       summary.Name,
		locality:   managedruntime.Locality(summary.ProviderLocality),
		readiness:  managedRuntimeReadiness(summary.Status),
		lifecycle:  managedRuntimeLifecycle(summary.LoadState),
		operations: summary.Operations,
	}
	projection := managedRuntimeProjection{
		summary:         domainSummary,
		baseDiagnostics: diagnostics,
		includeInspect:  forInspect,
	}
	if resource := primaryModelScopedResource(aggregate, factoryCfg); resource != nil {
		if sourceResolver != nil {
			resolution := sourceResolver.Resolve(aggregate.name, resource)
			projection.sourceResolution = &resolution
		}
		if runtimeCacheInspector != nil && aggregate.localCount > 0 {
			inspection, err := runtimeCacheInspector.InspectRuntimeCache(context.Background(), runtimeCfg, aggregate.name)
			if err == nil {
				projection.cacheInspection = &inspection
			}
		}
	}
	return buildManagedRuntimeProjection(projection)
}

func managedRuntimeReadiness(status modelcatalog.Status) managedruntime.ReadinessState {
	switch status {
	case modelcatalog.StatusReady:
		return managedruntime.ReadinessStateReady
	case modelcatalog.StatusUnavailable:
		return managedruntime.ReadinessStateMissing
	default:
		return managedruntime.ReadinessStateUnsupported
	}
}

func managedRuntimeLifecycle(loadState modelcatalog.LoadState) managedruntime.LifecycleState {
	if loadState == modelcatalog.LoadStateUnloaded {
		return managedruntime.LifecycleStateNotInstalled
	}
	return managedruntime.LifecycleStateNotApplicable
}

func managedRuntimeForCatalog(
	runtimeCfg *models.RuntimeConfig,
	modelName string,
	runtimeCacheInspector RuntimeCacheInspector,
	sourceResolver ManagedRuntimeSourceResolver,
) (managedruntime.Runtime, error) {
	catalog := BuildCatalogWithRuntime(runtimeCfg, runtimeCacheInspector, sourceResolver)
	key := CanonicalModelName(modelName)
	entry, ok := catalog[key]
	if !ok {
		return managedruntime.Runtime{}, fmt.Errorf("%w: %s", managedruntime.ErrNotFound, modelName)
	}
	return entry.Summary.ManagedRuntime, nil
}

func mergeInspectDiagnostics(base map[string]string, managed managedruntime.Runtime) map[string]string {
	merged := map[string]string{}
	for key, value := range base {
		merged[key] = value
	}
	for key, value := range managed.Diagnostics {
		merged[key] = value
	}
	return merged
}

// ListModels returns the model-owned discovery projection from runtime config.
func ListModels(runtimeCfg *models.RuntimeConfig) (modelcatalog.List, error) {
	return ListModelsWithRuntime(runtimeCfg, nil, nil)
}

// ListModelsWithRuntime returns runtime-aware model-owned discovery projections.
func ListModelsWithRuntime(
	runtimeCfg *models.RuntimeConfig,
	runtimeCacheInspector RuntimeCacheInspector,
	sourceResolver ManagedRuntimeSourceResolver,
) (modelcatalog.List, error) {
	if runtimeCfg == nil {
		return modelcatalog.List{}, fmt.Errorf("factory service runtime is not available")
	}
	catalog := BuildCatalogWithRuntime(runtimeCfg, runtimeCacheInspector, sourceResolver)
	results := make([]modelcatalog.Summary, 0, len(catalog))
	for _, entry := range catalog {
		results = append(results, entry.Summary)
	}
	sort.Slice(results, func(i, j int) bool { return results[i].Name < results[j].Name })
	return modelcatalog.List{Results: results}, nil
}

// GetModel returns the model-owned detail projection for a catalog name.
func GetModel(runtimeCfg *models.RuntimeConfig, modelName string) (modelcatalog.Detail, error) {
	return GetModelWithRuntime(runtimeCfg, modelName, nil, nil)
}

// GetModelWithRuntime returns runtime-aware model-owned detail.
func GetModelWithRuntime(
	runtimeCfg *models.RuntimeConfig,
	modelName string,
	runtimeCacheInspector RuntimeCacheInspector,
	sourceResolver ManagedRuntimeSourceResolver,
) (modelcatalog.Detail, error) {
	if runtimeCfg == nil {
		return modelcatalog.Detail{}, fmt.Errorf("factory service runtime is not available")
	}
	catalog := BuildCatalogWithRuntime(runtimeCfg, runtimeCacheInspector, sourceResolver)
	key := CanonicalModelName(modelName)
	if key == "" {
		return modelcatalog.Detail{}, fmt.Errorf("%w: empty model name", managedruntime.ErrNotFound)
	}
	entry, ok := catalog[key]
	if !ok {
		return modelcatalog.Detail{}, fmt.Errorf("%w: %s", managedruntime.ErrNotFound, modelName)
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
	// ResolvedReference is supplied only by the Models root after its
	// canonical resolver has accepted a name that is absent from the
	// Factory-scoped catalog. It lets the existing pull projection continue
	// without turning the lower-level Factory catalog into a second resolver.
	ResolvedReference *models.ResolvedModelReference
}

// PullModel validates catalog locality and delegates asset pull to the injected puller.
func PullModel(
	puller AssetPuller,
	ctx context.Context,
	runtimeCfg *models.RuntimeConfig,
	modelName string,
) (models.PullResult, error) {
	return PullModelWithOptions(puller, ctx, runtimeCfg, modelName, PullOptions{})
}

// PullModelWithOptions validates catalog locality, resolves backend source
// diagnostics, delegates asset pull, and projects managed readiness outcomes.
func PullModelWithOptions(
	puller AssetPuller,
	ctx context.Context,
	runtimeCfg *models.RuntimeConfig,
	modelName string,
	opts PullOptions,
) (models.PullResult, error) {
	entry, err := pullCatalogEntry(puller, runtimeCfg, modelName, opts.ResolvedReference)
	if err != nil {
		return models.PullResult{}, err
	}
	resolution := resolvePullSource(runtimeCfg, modelName, opts.SourceResolver)
	if err := pullContextError(ctx); err != nil {
		return models.PullResult{}, err
	}
	result, err := puller.PullModel(ctx, runtimeCfg, modelName)
	if opts.ResolvedReference != nil && strings.TrimSpace(entry.Summary.Name) != "" {
		// Generic asset preparation reports the source identity for a named
		// reference (for example, its Hugging Face repository). Pull's public
		// identity remains the resolved model name.
		result.ModelName = entry.Summary.Name
	}
	if err != nil {
		if errors.Is(err, managedruntime.ErrNotFound) || errors.Is(err, models.ErrPullUnsupported) {
			return models.PullResult{}, err
		}
		return classifiedPullFailure(
			entry.Summary.Name, entry.Summary.ProviderLocality, resolution, result, err,
		)
	}
	if err := pullContextError(ctx); err != nil {
		return classifiedPullFailure(
			entry.Summary.Name, entry.Summary.ProviderLocality, resolution, result, err,
		)
	}

	inspection, err := inspectPullCache(ctx, runtimeCfg, modelName, opts.RuntimeCacheInspector)
	if err != nil {
		return classifiedPullFailure(
			entry.Summary.Name, entry.Summary.ProviderLocality, resolution, result, err,
		)
	}
	return EnrichPullResult(result, inspection, resolution), nil
}

func pullCatalogEntry(
	puller AssetPuller,
	runtimeCfg *models.RuntimeConfig,
	modelName string,
	resolved *models.ResolvedModelReference,
) (CatalogEntry, error) {
	if runtimeCfg == nil {
		return CatalogEntry{}, fmt.Errorf("factory service runtime is not available")
	}
	if puller == nil {
		return CatalogEntry{}, fmt.Errorf("model asset puller is not available")
	}
	key := CanonicalModelName(modelName)
	if key == "" {
		return CatalogEntry{}, fmt.Errorf("%w: empty model name", managedruntime.ErrNotFound)
	}
	entry, ok := BuildCatalog(runtimeCfg)[key]
	if !ok {
		if fallback := resolvedPullCatalogEntry(key, modelName, resolved); fallback != nil {
			return *fallback, nil
		}
		return CatalogEntry{}, fmt.Errorf("%w: %s", managedruntime.ErrNotFound, modelName)
	}
	if entry.Summary.ProviderLocality != managedruntime.LocalityLocal {
		return CatalogEntry{}, fmt.Errorf(
			"%w: model %q is not a local model", models.ErrPullUnsupported, modelName,
		)
	}
	return entry, nil
}

func resolvedPullCatalogEntry(
	key string,
	modelName string,
	resolved *models.ResolvedModelReference,
) *CatalogEntry {
	if resolved == nil {
		return nil
	}
	definition := resolved.Definition
	name := strings.TrimSpace(definition.Name)
	if name == "" {
		name = strings.TrimSpace(modelName)
	}
	if CanonicalModelName(name) != key {
		return nil
	}
	return &CatalogEntry{Summary: modelcatalog.Summary{
		Name:             name,
		ProviderLocality: managedruntime.LocalityLocal,
	}}
}

func resolvePullSource(
	runtimeCfg *models.RuntimeConfig,
	modelName string,
	resolver ManagedRuntimeSourceResolver,
) ManagedRuntimeSourceResolution {
	resource := modelScopedResource(runtimeCfg, modelName)
	if resource == nil || resolver == nil {
		return ManagedRuntimeSourceResolution{}
	}
	return resolver.Resolve(modelName, resource)
}

func pullContextError(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}

func inspectPullCache(
	ctx context.Context,
	runtimeCfg *models.RuntimeConfig,
	modelName string,
	inspector RuntimeCacheInspector,
) (RuntimeCacheInspection, error) {
	if inspector == nil {
		return RuntimeCacheInspection{}, nil
	}
	inspection, err := inspector.InspectRuntimeCache(ctx, runtimeCfg, modelName)
	if err != nil {
		stage := pullsupport.PullStageForError(err)
		if stage == "" {
			stage = models.PullStageReadinessEvaluation
		}
		return RuntimeCacheInspection{}, pullsupport.WrapPullStage(
			stage, modelName, "inspect managed runtime cache", "", err,
		)
	}
	if err := pullContextError(ctx); err != nil {
		return RuntimeCacheInspection{}, err
	}
	return inspection, nil
}

func classifiedPullFailure(
	modelName string,
	providerLocality managedruntime.Locality,
	resolution ManagedRuntimeSourceResolution,
	base models.PullResult,
	cause error,
) (models.PullResult, error) {
	pullOutcome, readiness := ClassifyPullFailure(cause)
	result := base
	stage := pullsupport.PullStageForError(cause)
	operation := "classify pull failure"
	if stage == models.PullStageSourceResolution {
		operation = "resolve model source"
	}
	diagnostics := pullsupport.MergePullDiagnostics(
		result.PullDiagnostics,
		pullsupport.PullDiagnosticsFromError(cause),
	).WithDefaults(
		modelName, result.SourceID, result.Revision, "", operation,
	)
	if strings.TrimSpace(result.ModelName) == "" {
		result.ModelName = strings.TrimSpace(modelName)
	}
	if strings.TrimSpace(result.ProviderLocality) == "" {
		result.ProviderLocality = string(providerLocality)
	}
	result.Outcome = legacyPullOutcomeFailed
	result.ManagedPullOutcome = pullOutcome
	result.ReadinessState = readiness
	result.LifecycleState = managedLifecycleNotInstalled
	result.SourceKind = strings.TrimSpace(resolution.SourceKind)
	result.SourceID = strings.TrimSpace(resolution.SourceID)
	result.ResolverNotes = strings.TrimSpace(resolution.ResolverNotes)
	result.FailureStage = stage
	if result.FailureStage == "" &&
		pullOutcome == managedPullOutcomeAssetPreparationFailed &&
		!errors.Is(cause, context.Canceled) && !errors.Is(cause, context.DeadlineExceeded) {
		result.FailureStage = models.PullStageAssembly
	}
	result.PullDiagnostics = diagnostics.WithDefaults(
		result.ModelName, result.SourceID, result.Revision, "", diagnostics.Operation,
	)
	return result, &models.PullError{Result: result, Cause: cause}
}

func modelScopedResource(factoryCfg *models.RuntimeConfig, modelName string) *models.RuntimeResource {
	if factoryCfg == nil {
		return nil
	}
	key := canonicalModelName(modelName)
	for _, resource := range factoryCfg.Resources {
		if strings.TrimSpace(resource.Type) != models.RuntimeResourceTypeModel {
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
	runtimeCfg *models.RuntimeConfig,
	modelName, operationName string,
) (*models.RuntimeWorker, models.RuntimeOperation, error) {
	if runtimeCfg == nil {
		return nil, models.RuntimeOperation{}, fmt.Errorf("runtime config is not available")
	}
	modelKey := CanonicalModelName(modelName)
	operationName = strings.TrimSpace(operationName)
	if modelKey == "" {
		return nil, models.RuntimeOperation{}, fmt.Errorf("%w: empty model name", managedruntime.ErrNotFound)
	}
	if operationName == "" {
		return nil, models.RuntimeOperation{}, fmt.Errorf("operation is required")
	}

	var modelMatched bool
	var matchedWorkerName string
	for _, worker := range runtimeCfg.Workers {
		workerDef, ok := runtimeCfg.Worker(worker.Name)
		if !ok || workerDef == nil || !workerDef.IsInference() {
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
		return nil, models.RuntimeOperation{}, fmt.Errorf("%w: worker %q for model %q does not support operation %q", modelcatalog.ErrUnsupportedOperation, matchedWorkerName, modelName, operationName)
	}
	return nil, models.RuntimeOperation{}, fmt.Errorf("%w: %s", managedruntime.ErrNotFound, modelName)
}
