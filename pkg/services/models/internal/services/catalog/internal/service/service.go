package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	localmodels "github.com/portpowered/infinite-you/pkg/services/models/internal/local"
	catalog "github.com/portpowered/infinite-you/pkg/services/models/internal/services/catalog"
	runtimescopes "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_scopes"
)

type service struct {
	scopes    runtimescopes.Service
	readiness catalog.ReadinessQuery
}

var _ catalog.Service = (*service)(nil)

// New constructs an inert catalog over the Models-owned Runtime Scopes
// authority.
func New(scopes runtimescopes.Service, readiness catalog.ReadinessQuery) catalog.Service {
	return &service{scopes: scopes, readiness: readiness}
}

func (s *service) ListCatalog(
	ctx context.Context,
	request models.ListModelsRequest,
) (models.ListModelsResult, error) {
	scopeConfig, err := s.resolveScopeConfig(ctx, request.Scope)
	if err != nil {
		return models.ListModelsResult{}, err
	}

	entries, err := effectiveCatalog(scopeConfig)
	if err != nil {
		return models.ListModelsResult{}, err
	}
	result := models.ListModelsResult{Models: make([]models.Summary, 0, len(entries))}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return models.ListModelsResult{}, err
		}
		summary := stableSummary(entry.Summary)
		if s.readiness != nil {
			current, readinessErr := s.readiness(
				ctx,
				request.Scope,
				scopeConfig.Clone(),
				entry.Detail.Clone(),
			)
			if readinessErr != nil {
				if contextError := ctx.Err(); contextError != nil {
					return models.ListModelsResult{}, contextError
				}
				if errors.Is(readinessErr, context.Canceled) || errors.Is(readinessErr, context.DeadlineExceeded) {
					return models.ListModelsResult{}, readinessErr
				}
				return models.ListModelsResult{}, models.ErrUnavailable
			}
			summary.ManagedRuntime = overlayResolvedRuntime(summary.ManagedRuntime, current)
		}
		result.Models = append(result.Models, summary)
	}
	sort.Slice(result.Models, func(i, j int) bool {
		return localmodels.CanonicalModelName(result.Models[i].Name) <
			localmodels.CanonicalModelName(result.Models[j].Name)
	})
	return result, nil
}

// overlayResolvedRuntime applies the Models-owned readiness observation to a
// catalog projection while retaining the stable catalog identity and shape.
// Collection and detail callers must consume the same resolved state; copying
// only cache facts would leave the collection projection at its static
// MISSING/NOT_INSTALLED baseline.
func overlayResolvedRuntime(base, current models.Runtime) models.Runtime {
	projected := base.Clone()
	current = current.Clone()
	if current.Identity != "" {
		projected.Identity = current.Identity
	}
	if current.Locality != "" {
		projected.Locality = current.Locality
	}
	if current.ReadinessState != "" {
		projected.ReadinessState = current.ReadinessState
	}
	if current.LifecycleState != "" {
		projected.LifecycleState = current.LifecycleState
	}
	if current.SupportedOperations != nil {
		projected.SupportedOperations = current.SupportedOperations
	}
	if current.Revision != nil {
		projected.Revision = current.Revision
	}
	if current.CachePath != nil {
		projected.CachePath = current.CachePath
	}
	if current.CacheBytes != nil {
		projected.CacheBytes = current.CacheBytes
	}
	projected.Diagnostics = mergeDiagnostics(projected.Diagnostics, current.Diagnostics)
	if projected.Diagnostics == nil {
		projected.Diagnostics = map[string]string{}
	}
	if projected.ReadinessState != "" {
		projected.Diagnostics["readinessState"] = string(projected.ReadinessState)
	}
	if projected.LifecycleState != "" {
		projected.Diagnostics["lifecycleState"] = string(projected.LifecycleState)
	}
	return projected
}

func (s *service) GetCatalogModel(
	ctx context.Context,
	request models.GetModelRequest,
) (models.GetModelResult, error) {
	scopeConfig, err := s.resolveScopeConfig(ctx, request.Scope)
	if err != nil {
		return models.GetModelResult{}, err
	}
	detail, err := catalogDetail(scopeConfig, request.Name, request.Operation)
	if err != nil {
		return models.GetModelResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return models.GetModelResult{}, err
	}
	if s.readiness != nil {
		current, readinessErr := s.readiness(
			ctx,
			request.Scope,
			scopeConfig.Clone(),
			detail.Clone(),
		)
		if readinessErr != nil {
			if contextError := ctx.Err(); contextError != nil {
				return models.GetModelResult{}, contextError
			}
			if errors.Is(readinessErr, context.Canceled) || errors.Is(readinessErr, context.DeadlineExceeded) {
				return models.GetModelResult{}, readinessErr
			}
			return models.GetModelResult{}, models.ErrUnavailable
		}
		detail.Summary.ManagedRuntime = overlayResolvedRuntime(detail.Summary.ManagedRuntime, current)
		// Detail diagnostics historically includes the managed-runtime state for
		// compatibility. Keep that duplicate projection sourced from the same
		// resolved runtime as the canonical managedRuntime object.
		detail.Diagnostics = mergeDiagnostics(detail.Diagnostics, detail.Summary.ManagedRuntime.Diagnostics)
	}
	return models.GetModelResult{Model: detail}, nil
}

func (s *service) GetModelReadiness(
	ctx context.Context,
	request models.GetModelReadinessRequest,
) (models.GetModelReadinessResult, error) {
	scopeConfig, err := s.resolveScopeConfig(ctx, request.Scope)
	if err != nil {
		return models.GetModelReadinessResult{}, err
	}
	detail, err := catalogDetail(scopeConfig, request.Name, request.Operation)
	if err != nil {
		return models.GetModelReadinessResult{}, err
	}
	if s.readiness == nil {
		return models.GetModelReadinessResult{}, models.ErrUnavailable
	}
	// Effective definitions are discovery entries, not Factory-declared
	// invocation workers. Keep the direct invocation preflight at its stable
	// missing baseline; catalog list/detail still query the durable cache so
	// operators can see a pulled built-in model as READY.
	if detail.Diagnostics["catalogSource"] == "EFFECTIVE_DEFINITION" {
		return models.GetModelReadinessResult{
			ModelName: detail.Name,
			Readiness: stableReadiness(detail, detail.ManagedRuntime),
		}, nil
	}
	readiness, err := s.readiness(ctx, request.Scope, scopeConfig.Clone(), detail.Clone())
	if err != nil {
		if contextError := ctx.Err(); contextError != nil {
			return models.GetModelReadinessResult{}, contextError
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return models.GetModelReadinessResult{}, err
		}
		return models.GetModelReadinessResult{}, models.ErrUnavailable
	}
	if err := ctx.Err(); err != nil {
		return models.GetModelReadinessResult{}, err
	}
	return models.GetModelReadinessResult{
		ModelName: detail.Name,
		Readiness: stableReadiness(detail, readiness),
	}, nil
}

func catalogDetail(
	scopeConfig models.RuntimeScopeConfig,
	name string,
	operation string,
) (models.Detail, error) {
	request := models.GetModelRequest{Name: name, Operation: operation}
	if err := request.Validate(); err != nil {
		return models.Detail{}, err
	}
	entries, err := effectiveCatalog(scopeConfig)
	if err != nil {
		return models.Detail{}, err
	}
	entry, ok := entries[localmodels.CanonicalModelName(name)]
	if !ok {
		return models.Detail{}, fmt.Errorf("%w: %s", models.ErrNotFound, name)
	}
	detail := stableDetail(entry.Detail)
	if operation != "" && !supportsOperation(detail, operation) {
		return models.Detail{}, fmt.Errorf(
			"%w: model %q does not support operation %q",
			models.ErrUnsupportedOperation,
			detail.Name,
			operation,
		)
	}
	return detail, nil
}

func (s *service) resolveScopeConfig(
	ctx context.Context,
	scope models.RuntimeScopeRef,
) (models.RuntimeScopeConfig, error) {
	if err := ctx.Err(); err != nil {
		return models.RuntimeScopeConfig{}, err
	}
	if scope.IsZero() {
		return models.RuntimeScopeConfig{}, models.ErrRuntimeScopeInvalid
	}
	if s == nil || s.scopes == nil {
		return models.RuntimeScopeConfig{}, models.ErrUnavailable
	}

	binding, err := s.scopes.Resolve(runtimescopes.Reference(scope.String()))
	if err != nil {
		return models.RuntimeScopeConfig{}, catalogScopeError(err)
	}
	if binding.RuntimeConfig == nil {
		return models.RuntimeScopeConfig{}, models.ErrUnavailable
	}
	runtimeConfig := binding.RuntimeConfig()
	if runtimeConfig == nil {
		return models.RuntimeScopeConfig{}, models.ErrUnavailable
	}
	return models.RuntimeScopeConfig{
		CacheDirectory: binding.CacheDirectory,
		Runtime:        *runtimeConfig,
		OperatorModels: binding.OperatorModels,
	}.Clone(), nil
}

// effectiveCatalog keeps the Factory catalog as the highest-precedence
// discovery source, then projects the existing operator overlay and built-in
// definition policy for names that the Factory did not declare. The local
// catalog builder intentionally remains Factory-only because it is also used
// by lower-level asset and runtime helpers with different scope semantics.
func effectiveCatalog(scopeConfig models.RuntimeScopeConfig) (map[string]catalog.Entry, error) {
	entries := localmodels.BuildCatalogWithRuntime(
		&scopeConfig.Runtime,
		nil,
		localmodels.DefaultManagedRuntimeSourceResolver(),
	)

	overlayNames := make([]string, 0, len(scopeConfig.OperatorModels))
	for name := range scopeConfig.OperatorModels {
		overlayNames = append(overlayNames, name)
	}
	sort.Strings(overlayNames)
	seenOverlays := make(map[string]string, len(overlayNames))
	for _, rawName := range overlayNames {
		canonicalName := strings.ToLower(strings.TrimSpace(rawName))
		if !safeCatalogModelName(canonicalName) {
			return nil, catalogConfigurationFailure(
				rawName,
				"name",
				"must contain only letters, digits, dots, hyphens, or underscores",
			)
		}
		if previous, exists := seenOverlays[canonicalName]; exists {
			return nil, catalogConfigurationFailure(
				rawName,
				"name",
				fmt.Sprintf("duplicates another entry after case and whitespace normalization (%q)", previous),
			)
		}
		seenOverlays[canonicalName] = rawName

		overlay := scopeConfig.OperatorModels[rawName].Clone()
		base, builtIn := (models.BuiltInCatalog{}).ModelDefinitionFor(canonicalName)
		if !builtIn {
			base = models.ModelDefinition{Name: canonicalName}
		}
		if err := validateCatalogOverlay(rawName, overlay, builtIn); err != nil {
			return nil, err
		}
		applyCatalogOverlay(&base, overlay)
		base.Name = canonicalName

		// An authored Factory definition is already an effective catalog entry;
		// do not add an operator or built-in duplicate over that key.
		key := localmodels.CanonicalModelName(canonicalName)
		if _, exists := entries[key]; exists {
			continue
		}
		entries[key] = catalogEntryFromDefinition(base, catalogSourceKind(base.Source, false))
	}

	for _, definition := range (models.BuiltInCatalog{}).ModelDefinitions() {
		key := localmodels.CanonicalModelName(definition.Name)
		if _, exists := entries[key]; exists {
			continue
		}
		entries[key] = catalogEntryFromDefinition(definition, catalogSourceKind(definition.Source, true))
	}
	return entries, nil
}

func validateCatalogOverlay(name string, overlay models.ModelOverlay, builtIn bool) error {
	if overlay.Source != nil && strings.TrimSpace(*overlay.Source) == "" {
		return catalogConfigurationFailure(name, "source", "must be omitted or non-empty")
	}
	if overlay.Backend != nil && strings.TrimSpace(*overlay.Backend) == "" {
		return catalogConfigurationFailure(name, "backend", "must be omitted or non-empty")
	}
	if overlay.LoadPolicy != nil {
		loadPolicy := strings.ToUpper(strings.TrimSpace(string(*overlay.LoadPolicy)))
		if loadPolicy != string(models.LoadPolicyOnDemand) && loadPolicy != string(models.LoadPolicyKeepWarm) {
			return catalogConfigurationFailure(name, "loadPolicy", "must be ON_DEMAND or KEEP_WARM")
		}
	}
	if overlay.Operations != nil && len(overlay.Operations) == 0 {
		return catalogConfigurationFailure(name, "operations", "must contain at least one operation")
	}
	for _, operation := range overlay.Operations {
		if _, ok := (models.GenericOperationCatalog{}).GenericOperationContract(operation); !ok {
			return catalogConfigurationFailure(name, "operations", fmt.Sprintf("unsupported operation %q", operation))
		}
	}
	if !builtIn {
		for _, field := range []struct {
			name    string
			present bool
		}{
			{name: "source", present: overlay.Source != nil},
			{name: "backend", present: overlay.Backend != nil},
			{name: "loadPolicy", present: overlay.LoadPolicy != nil},
			{name: "operations", present: overlay.Operations != nil},
		} {
			if !field.present {
				return catalogConfigurationFailure(name, field.name, "is required for a new model entry")
			}
		}
	}
	return nil
}

func applyCatalogOverlay(definition *models.ModelDefinition, overlay models.ModelOverlay) {
	if overlay.Source != nil {
		definition.Source = strings.TrimSpace(*overlay.Source)
	}
	if overlay.Backend != nil {
		definition.Backend = strings.TrimSpace(*overlay.Backend)
	}
	if overlay.LoadPolicy != nil {
		definition.LoadPolicy = models.LoadPolicy(strings.ToUpper(strings.TrimSpace(string(*overlay.LoadPolicy))))
	}
	if overlay.Operations != nil {
		definition.Operations = make([]models.Operation, 0, len(overlay.Operations))
		for _, name := range overlay.Operations {
			operation, ok := (models.GenericOperationCatalog{}).GenericOperationContract(name)
			if ok {
				definition.Operations = append(definition.Operations, operation)
			}
		}
	}
}

func catalogEntryFromDefinition(
	definition models.ModelDefinition,
	sourceKind string,
) catalog.Entry {
	definition = definition.Clone()
	operations := make([]models.Operation, len(definition.Operations))
	for index, operation := range definition.Operations {
		operations[index] = operation.Clone()
	}
	diagnostics := map[string]string{
		"catalogSource": "EFFECTIVE_DEFINITION",
		"sourceKind":    sourceKind,
		"sourceId":      definition.Source,
	}
	if revision := sourceRevision(definition.Source); revision != "" {
		diagnostics["revision"] = revision
	}
	runtime := models.Runtime{
		Identity:            definition.Name,
		ReadinessState:      models.ReadinessStateMissing,
		LifecycleState:      models.LifecycleStateNotInstalled,
		Locality:            models.LocalityLocal,
		SupportedOperations: operations,
		Diagnostics:         cloneCatalogDiagnostics(diagnostics),
	}
	summary := models.Summary{
		Name:             definition.Name,
		ProviderLocality: models.LocalityLocal,
		Status:           models.StatusReady,
		LoadState:        models.LoadStateUnloaded,
		Operations:       operations,
		Modalities:       catalogModalities(operations),
		ManagedRuntime:   runtime,
	}
	return catalog.Entry{
		Summary: summary,
		Detail: models.Detail{
			Summary:     summary.Clone(),
			Sources:     sourceMetadata(diagnostics),
			Diagnostics: diagnostics,
		},
	}
}

func catalogModalities(operations []models.Operation) []string {
	seen := make(map[string]struct{})
	for _, operation := range operations {
		for _, slots := range [][]models.OperationSlot{operation.Inputs, operation.Outputs} {
			for _, slot := range slots {
				modality := strings.TrimSpace(string(slot.Modality))
				if modality == "" {
					for _, contentType := range slot.ContentTypes {
						modality = strings.TrimSpace(contentType)
						if modality != "" {
							break
						}
					}
				}
				if modality != "" {
					seen[modality] = struct{}{}
				}
			}
		}
	}
	modalities := make([]string, 0, len(seen))
	for modality := range seen {
		modalities = append(modalities, modality)
	}
	sort.Strings(modalities)
	return modalities
}

func catalogSourceKind(source string, builtIn bool) string {
	if builtIn || strings.HasPrefix(strings.ToLower(strings.TrimSpace(source)), "hf://") {
		return string(models.ModelReferenceSourceHuggingFace)
	}
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(source)), "file://") {
		return string(models.ModelReferenceSourceFileURI)
	}
	return string(models.ModelReferenceSourceLocalPath)
}

func sourceRevision(source string) string {
	if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(source)), "hf://") {
		return ""
	}
	separator := strings.LastIndex(source, "@")
	if separator < 0 || separator == len(source)-1 {
		return ""
	}
	return strings.TrimSpace(source[separator+1:])
}

func cloneCatalogDiagnostics(values map[string]string) map[string]string {
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func catalogConfigurationFailure(name, field, message string) error {
	return models.ModelConfigurationFailure{ModelName: name, Field: field, Message: message}
}

func safeCatalogModelName(value string) bool {
	for index, character := range value {
		if (character >= 'a' && character <= 'z') ||
			(character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') ||
			(index > 0 && (character == '.' || character == '_' || character == '-')) {
			continue
		}
		return false
	}
	return value != ""
}

func catalogScopeError(err error) error {
	switch {
	case errors.Is(err, runtimescopes.ErrScopeForeign):
		return fmt.Errorf("%w: %v", models.ErrRuntimeScopeForeign, err)
	case errors.Is(err, runtimescopes.ErrScopeClosed):
		return fmt.Errorf("%w: %v", models.ErrRuntimeScopeClosed, err)
	case errors.Is(err, runtimescopes.ErrScopeUnknown):
		return fmt.Errorf("%w: %v", models.ErrRuntimeScopeStale, err)
	default:
		return models.ErrUnavailable
	}
}

func stableRuntime(runtime models.Runtime) models.Runtime {
	runtime = runtime.Clone()
	sortOperations(runtime.SupportedOperations)
	return runtime
}

func stableReadiness(detail models.Detail, current models.Runtime) models.Runtime {
	current = current.Clone()
	current.Identity = detail.Name
	if current.Locality == "" {
		current.Locality = detail.ProviderLocality
	}
	current.SupportedOperations = detail.ManagedRuntime.Clone().SupportedOperations
	current.Diagnostics = mergeDiagnostics(
		detail.ManagedRuntime.Diagnostics,
		current.Diagnostics,
	)
	return stableRuntime(current)
}

func mergeDiagnostics(base, current map[string]string) map[string]string {
	merged := make(map[string]string, len(base)+len(current))
	for key, value := range base {
		merged[key] = value
	}
	for key, value := range current {
		merged[key] = value
	}
	return merged
}

func stableSummary(summary models.Summary) models.Summary {
	summary = summary.Clone()
	sortOperations(summary.Operations)
	sort.Strings(summary.Modalities)
	sort.Slice(summary.Resources, func(i, j int) bool {
		return summary.Resources[i].Name < summary.Resources[j].Name
	})
	sortOperations(summary.ManagedRuntime.SupportedOperations)
	return summary
}

func stableDetail(detail models.Detail) models.Detail {
	detail = detail.Clone()
	detail.Summary = stableSummary(detail.Summary)
	if len(detail.Sources) == 0 {
		detail.Sources = sourceMetadata(detail.ManagedRuntime.Diagnostics)
	}
	for i := range detail.Capabilities {
		sortOperations(detail.Capabilities[i].Operations)
		sort.Strings(detail.Capabilities[i].ResourceNames)
	}
	sort.Slice(detail.Capabilities, func(i, j int) bool {
		return detail.Capabilities[i].Worker < detail.Capabilities[j].Worker
	})
	sort.Slice(detail.Sources, func(i, j int) bool {
		left, right := detail.Sources[i], detail.Sources[j]
		if left.Provider != right.Provider {
			return left.Provider < right.Provider
		}
		if left.Reference != right.Reference {
			return left.Reference < right.Reference
		}
		return left.Revision < right.Revision
	})
	return detail
}

func sourceMetadata(diagnostics map[string]string) []models.SourceMetadata {
	provider := diagnostics["sourceKind"]
	reference := diagnostics["sourceId"]
	revision := diagnostics["revision"]
	if provider == "" && reference == "" && revision == "" {
		return nil
	}
	return []models.SourceMetadata{{
		Provider:  provider,
		Reference: reference,
		Revision:  revision,
	}}
}

func supportsOperation(detail models.Detail, requested string) bool {
	for _, operation := range detail.Operations {
		if operation.Name == requested {
			return true
		}
	}
	for _, capability := range detail.Capabilities {
		for _, operation := range capability.Operations {
			if operation.Name == requested {
				return true
			}
		}
	}
	return false
}

func sortOperations(operations []models.Operation) {
	for i := range operations {
		sortOperationSlots(operations[i].Inputs)
		sortOperationSlots(operations[i].Outputs)
	}
	sort.Slice(operations, func(i, j int) bool {
		return operations[i].Name < operations[j].Name
	})
}

func sortOperationSlots(slots []models.OperationSlot) {
	for i := range slots {
		sort.Strings(slots[i].ContentTypes)
	}
	sort.Slice(slots, func(i, j int) bool {
		return slots[i].Name < slots[j].Name
	})
}
