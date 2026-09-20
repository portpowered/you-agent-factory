package local

import (
	"context"
	"fmt"
	"path"
	"strings"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	managedruntime "github.com/portpowered/infinite-you/pkg/services/models/internal/managedruntime"
	scopedassets "github.com/portpowered/infinite-you/pkg/services/models/internal/services/assets"
)

type managedRuntimeProjection struct {
	summary          managedRuntimeSummary
	baseDiagnostics  map[string]string
	cacheInspection  *RuntimeCacheInspection
	sourceResolution *ManagedRuntimeSourceResolution
	includeInspect   bool
}

type managedRuntimeSummary struct {
	name       string
	locality   managedruntime.Locality
	readiness  managedruntime.ReadinessState
	lifecycle  managedruntime.LifecycleState
	operations []managedruntime.Operation
}

func buildManagedRuntime(summary managedRuntimeSummary, diagnostics map[string]string) managedruntime.Runtime {
	return buildManagedRuntimeProjection(managedRuntimeProjection{
		summary:         summary,
		baseDiagnostics: diagnostics,
	})
}

func buildManagedRuntimeProjection(input managedRuntimeProjection) managedruntime.Runtime {
	readiness, lifecycle := managedRuntimeStates(input)
	managedDiagnostics := managedRuntimeDiagnostics(input.summary, input.baseDiagnostics, readiness, lifecycle)
	for key, value := range managedRuntimeSourceDiagnostics(managedRuntimeSourceResolutionValue(input)) {
		managedDiagnostics[key] = value
	}
	if input.cacheInspection != nil {
		for key, value := range runtimeCacheInspectDiagnostics(*input.cacheInspection, input.includeInspect) {
			managedDiagnostics[key] = value
		}
	}
	var revision *string
	var cachePath *string
	var cacheBytes *int64
	if input.cacheInspection != nil && input.cacheInspection.Supported && input.cacheInspection.Installed {
		if value := strings.TrimSpace(input.cacheInspection.Revision); value != "" {
			revision = &value
		}
		if value := strings.TrimSpace(input.cacheInspection.CachePath); value != "" {
			cachePath = &value
		}
		if input.cacheInspection.CacheBytes >= 0 {
			value := input.cacheInspection.CacheBytes
			cacheBytes = &value
		}
	}
	return managedruntime.Runtime{
		Identity:            input.summary.name,
		ReadinessState:      readiness,
		LifecycleState:      lifecycle,
		Locality:            input.summary.locality,
		Revision:            revision,
		CachePath:           cachePath,
		CacheBytes:          cacheBytes,
		SupportedOperations: input.summary.operations,
		Diagnostics:         managedDiagnostics,
	}
}

func managedRuntimeStates(input managedRuntimeProjection) (managedruntime.ReadinessState, managedruntime.LifecycleState) {
	if input.cacheInspection != nil {
		inspection := *input.cacheInspection
		projection := managedruntime.ProjectManagedRuntimeState(
			managedRuntimeCacheFacts(input.summary.locality, inspection),
			models.ManagedRuntimeHostFacts{},
		)
		return projection.ReadinessState, projection.LifecycleState
	}
	if input.summary.locality == managedruntime.LocalityLocal {
		projection := managedruntime.ProjectManagedRuntimeState(
			models.ManagedRuntimeCacheFacts{
				Locality:  models.LocalityLocal,
				Supported: true,
			},
			models.ManagedRuntimeHostFacts{},
		)
		return projection.ReadinessState, projection.LifecycleState
	}
	readiness, lifecycle := managedruntime.NormalizeManagedRuntimeState(
		models.Locality(input.summary.locality), input.summary.readiness, input.summary.lifecycle,
	)
	return readiness, lifecycle
}

func managedRuntimeCacheFacts(
	locality managedruntime.Locality,
	inspection RuntimeCacheInspection,
) models.ManagedRuntimeCacheFacts {
	return models.ManagedRuntimeCacheFacts{
		Locality:           models.Locality(locality),
		Supported:          inspection.Supported,
		Installed:          inspection.Installed,
		ManifestPresent:    inspection.ManifestPresent,
		ManifestValid:      inspection.ManifestValid,
		ExpectedArtifacts:  append([]models.AssetRequirement(nil), inspection.ExpectedArtifacts...),
		ObservedArtifacts:  append([]models.AssetArtifact(nil), inspection.ObservedArtifacts...),
		InstalledFileCount: inspection.InstalledFileCount,
		PartialArtifacts:   inspection.PartialArtifacts,
		ActivePull:         inspection.ActivePull,
		IntegrityVerified:  inspection.IntegrityVerified,
		FailureReason:      inspection.FailureReason,
	}
}

func managedRuntimeSourceResolutionValue(input managedRuntimeProjection) ManagedRuntimeSourceResolution {
	if input.sourceResolution != nil {
		return *input.sourceResolution
	}
	return ManagedRuntimeSourceResolution{}
}

func managedRuntimeDiagnostics(
	summary managedRuntimeSummary,
	diagnostics map[string]string,
	readiness managedruntime.ReadinessState,
	lifecycle managedruntime.LifecycleState,
) map[string]string {
	result := map[string]string{
		"readinessState": string(readiness),
		"lifecycleState": string(lifecycle),
		"locality":       string(summary.locality),
	}
	for key, value := range diagnostics {
		result[key] = value
	}
	if readiness == managedruntime.ReadinessStateFailed && result["failureReason"] == "" {
		result["failureReason"] = "managed runtime cache or host state is not usable"
	}
	return result
}

func primaryModelScopedResource(aggregate catalogAggregate, factoryCfg *models.RuntimeConfig) *models.RuntimeResource {
	if factoryCfg == nil || !aggregate.hasModelScoped {
		return nil
	}
	for _, resource := range factoryCfg.Resources {
		if canonicalModelName(resource.Model) != canonicalModelName(aggregate.name) {
			continue
		}
		if strings.TrimSpace(resource.Type) != models.RuntimeResourceTypeModel {
			continue
		}
		copied := resource
		return &copied
	}
	return nil
}

// ManagedRuntimeReadinessForFactory returns the canonical managed-runtime readiness projection
// for one factory dependency identity using the same catalog path for packaged and authored factories.
func ManagedRuntimeReadinessForFactory(
	runtimeCfg *models.RuntimeConfig,
	modelName string,
	runtimeCacheInspector RuntimeCacheInspector,
	sourceResolver ManagedRuntimeSourceResolver,
) (managedruntime.Runtime, error) {
	if runtimeCfg == nil {
		return managedruntime.Runtime{}, fmt.Errorf("runtime config is not available")
	}
	key := CanonicalModelName(modelName)
	if key == "" {
		return managedruntime.Runtime{}, fmt.Errorf("%w: empty model name", managedruntime.ErrNotFound)
	}
	return managedRuntimeForCatalog(runtimeCfg, modelName, runtimeCacheInspector, sourceResolver)
}

// ManagedRuntimeReadinessForFactoryContext returns the current readiness
// projection while preserving cancellation across the cache fact boundary.
func ManagedRuntimeReadinessForFactoryContext(
	ctx context.Context,
	runtimeCfg *models.RuntimeConfig,
	modelName string,
	runtimeCacheInspector RuntimeCacheInspector,
	sourceResolver ManagedRuntimeSourceResolver,
) (managedruntime.Runtime, error) {
	if err := ctx.Err(); err != nil {
		return managedruntime.Runtime{}, err
	}
	baseline, err := ManagedRuntimeReadinessForFactory(
		runtimeCfg,
		modelName,
		nil,
		sourceResolver,
	)
	if err != nil {
		return managedruntime.Runtime{}, err
	}
	if runtimeCacheInspector == nil {
		return baseline, nil
	}
	inspection, err := runtimeCacheInspector.InspectRuntimeCache(ctx, runtimeCfg, modelName)
	if err != nil {
		return managedruntime.Runtime{}, err
	}
	if err := ctx.Err(); err != nil {
		return managedruntime.Runtime{}, err
	}
	detail, err := GetModelWithRuntime(
		runtimeCfg,
		modelName,
		fixedRuntimeCacheInspector{inspection: inspection},
		sourceResolver,
	)
	return detail.ManagedRuntime, err
}

// ManagedRuntimeReadinessForEffectiveDefinitionContext applies current cache
// facts to a catalog entry that came from an effective definition rather than
// a Factory worker/resource projection. Built-in and operator model entries
// use this path when a catalog scope has no authored runtime resources.
func ManagedRuntimeReadinessForEffectiveDefinitionContext(
	ctx context.Context,
	baseline models.Runtime,
	runtimeCfg *models.RuntimeConfig,
	modelName string,
	runtimeCacheInspector RuntimeCacheInspector,
) (models.Runtime, error) {
	if err := ctx.Err(); err != nil {
		return models.Runtime{}, err
	}
	if runtimeCacheInspector == nil {
		return baseline.Clone(), nil
	}
	inspection, err := runtimeCacheInspector.InspectRuntimeCache(ctx, runtimeCfg, modelName)
	if err != nil {
		return models.Runtime{}, err
	}
	if err := ctx.Err(); err != nil {
		return models.Runtime{}, err
	}
	return projectManagedRuntimeCacheInspection(baseline, inspection), nil
}

func projectManagedRuntimeCacheInspection(
	baseline models.Runtime,
	inspection RuntimeCacheInspection,
) models.Runtime {
	operations, capabilityDiagnostics := ProjectEffectiveVideoOperations(
		baseline.Identity,
		baseline.SupportedOperations,
		inspection,
	)
	baseDiagnostics := baseline.Diagnostics
	if len(capabilityDiagnostics) > 0 {
		baseDiagnostics = mergeRuntimeDiagnostics(baseDiagnostics, capabilityDiagnostics)
	}
	projection := buildManagedRuntimeProjection(managedRuntimeProjection{
		summary: managedRuntimeSummary{
			name:       baseline.Identity,
			locality:   managedruntime.Locality(baseline.Locality),
			readiness:  managedruntime.ReadinessState(baseline.ReadinessState),
			lifecycle:  managedruntime.LifecycleState(baseline.LifecycleState),
			operations: operations,
		},
		baseDiagnostics: baseDiagnostics,
		cacheInspection: &inspection,
		includeInspect:  true,
	})
	return models.Runtime{
		Identity:            projection.Identity,
		ReadinessState:      projection.ReadinessState,
		LifecycleState:      projection.LifecycleState,
		Locality:            projection.Locality,
		Revision:            projection.Revision,
		CachePath:           projection.CachePath,
		CacheBytes:          projection.CacheBytes,
		SupportedOperations: projection.SupportedOperations,
		Diagnostics:         projection.Diagnostics,
	}
}

func mergeRuntimeDiagnostics(
	base map[string]string,
	current map[string]string,
) map[string]string {
	merged := make(map[string]string, len(base)+len(current))
	for key, value := range base {
		merged[key] = value
	}
	for key, value := range current {
		merged[key] = value
	}
	return merged
}

type fixedRuntimeCacheInspector struct {
	inspection RuntimeCacheInspection
}

func (inspector fixedRuntimeCacheInspector) InspectRuntimeCache(
	context.Context,
	*models.RuntimeConfig,
	string,
) (RuntimeCacheInspection, error) {
	return inspector.inspection, nil
}

// EnsureManagedRuntimeReadyForInvocation classifies one managed runtime using
// the same catalog readiness projection as discovery and inspect before
// invocation proceeds.
func EnsureManagedRuntimeReadyForInvocation(
	runtimeCfg *models.RuntimeConfig,
	modelName string,
	runtimeCacheInspector RuntimeCacheInspector,
	sourceResolver ManagedRuntimeSourceResolver,
) (managedruntime.Runtime, error) {
	managed, err := ManagedRuntimeReadinessForFactory(
		runtimeCfg, modelName, runtimeCacheInspector, sourceResolver,
	)
	if err != nil {
		return managedruntime.Runtime{}, err
	}
	if invocationErr := managed.InvocationError(); invocationErr != nil {
		return managed, invocationErr
	}
	return managed, nil
}

const (
	// EffectiveOperationsDiagnostic identifies a runtime-backed operation
	// projection. It is intentionally diagnostic-only; the public model shape
	// remains unchanged.
	EffectiveOperationsDiagnostic = "effectiveOperations"
	// VideoReadinessDiagnostic is the stable safe explanation for an omitted
	// VIDEO input capability.
	VideoReadinessDiagnostic = "videoReadiness"
	VideoReadinessVerified   = "verified-projector"
	VideoReadinessMissing    = "required projector artifact is missing or invalid"

	projectorArtifactMarker = "mmproj"
)

// ProjectEffectiveVideoDefinition applies verified runtime-artifact facts to
// a model definition. The built-in LLM's VIDEO input is effective only when
// the durable cache contains an intact projector artifact. Text and all other
// operation inputs remain available when VIDEO is omitted.
func ProjectEffectiveVideoDefinition(
	definition models.ModelDefinition,
	inspection RuntimeCacheInspection,
) (models.ModelDefinition, map[string]string, bool) {
	definition = definition.Clone()
	operations, diagnostics := ProjectEffectiveVideoOperations(
		definition.Name, definition.Operations, inspection,
	)
	definition.Operations = operations
	return definition, diagnostics, diagnostics[VideoReadinessDiagnostic] == VideoReadinessMissing
}

// ProjectEffectiveVideoOperations removes only the VIDEO slot from the
// built-in LLM OMNI contract when the required projector is not verified.
// The returned operations and diagnostics are detached.
func ProjectEffectiveVideoOperations(
	modelName string,
	operations []models.Operation,
	inspection RuntimeCacheInspection,
) ([]models.Operation, map[string]string) {
	cloned := cloneOperations(operations)
	if !videoProjectorRequired(modelName, cloned) {
		return cloned, nil
	}

	diagnostics := map[string]string{
		EffectiveOperationsDiagnostic: "verified-runtime-assets",
	}
	if verifiedProjectorArtifact(inspection) {
		diagnostics[VideoReadinessDiagnostic] = VideoReadinessVerified
		return cloned, diagnostics
	}

	diagnostics[VideoReadinessDiagnostic] = VideoReadinessMissing
	for index := range cloned {
		if !strings.EqualFold(strings.TrimSpace(cloned[index].Name), models.OperationOMNI) {
			continue
		}
		cloned[index] = withoutVideoSlots(cloned[index])
	}
	return cloned, diagnostics
}

// VideoProjectorRequired reports whether the named definition has the
// projector-backed built-in LLM VIDEO capability.
func VideoProjectorRequired(modelName string, operations []models.Operation) bool {
	return videoProjectorRequired(modelName, operations)
}

// RuntimeCacheInspectionFromScoped converts the parent-private Assets fact
// into the local compatibility fact without leaking the nested service type
// through the Models public contract.
func RuntimeCacheInspectionFromScoped(
	inspection scopedassets.RuntimeCacheInspection,
) RuntimeCacheInspection {
	return RuntimeCacheInspection{
		Supported:          inspection.Supported,
		Installed:          inspection.Installed,
		Revision:           inspection.Revision,
		CachePath:          inspection.CachePath,
		CacheBytes:         inspection.CacheBytes,
		InstalledFileCount: inspection.InstalledFileCount,
		MissingAssets:      append([]string(nil), inspection.MissingAssets...),
		PartialArtifacts:   inspection.PartialArtifacts,
		ManifestPresent:    inspection.ManifestPresent,
		ManifestValid:      inspection.ManifestValid,
		ExpectedArtifacts:  append([]models.AssetRequirement(nil), inspection.ExpectedArtifacts...),
		ObservedArtifacts:  append([]models.AssetArtifact(nil), inspection.ObservedArtifacts...),
		ActivePull:         inspection.ActivePull,
		IntegrityVerified:  inspection.IntegrityVerified,
		FailureReason:      inspection.FailureReason,
	}
}

func videoProjectorRequired(modelName string, operations []models.Operation) bool {
	if CanonicalModelName(modelName) != CanonicalModelName(models.BuiltInModelNameLLM) {
		return false
	}
	for _, operation := range operations {
		if !strings.EqualFold(strings.TrimSpace(operation.Name), models.OperationOMNI) {
			continue
		}
		for _, slot := range operation.Inputs {
			if isVideoSlot(slot) {
				return true
			}
		}
	}
	return false
}

func verifiedProjectorArtifact(inspection RuntimeCacheInspection) bool {
	if !inspection.Supported || !inspection.Installed ||
		!inspection.ManifestPresent || !inspection.ManifestValid ||
		!inspection.IntegrityVerified {
		return false
	}
	var expected *models.AssetRequirement
	for index := range inspection.ExpectedArtifacts {
		if isProjectorArtifact(inspection.ExpectedArtifacts[index].Name) {
			candidate := inspection.ExpectedArtifacts[index]
			expected = &candidate
			break
		}
	}
	if expected == nil {
		return false
	}
	for _, observed := range inspection.ObservedArtifacts {
		if !strings.EqualFold(path.Clean(strings.TrimSpace(observed.Name)), path.Clean(strings.TrimSpace(expected.Name))) {
			continue
		}
		if expected.Bytes > 0 && observed.Bytes != expected.Bytes {
			return false
		}
		if digest := strings.TrimSpace(expected.SHA256); digest != "" &&
			!strings.EqualFold(digest, strings.TrimSpace(observed.SHA256)) {
			return false
		}
		return true
	}
	return false
}

func isProjectorArtifact(name string) bool {
	return strings.Contains(strings.ToLower(path.Base(strings.TrimSpace(name))), projectorArtifactMarker)
}

func isVideoSlot(slot models.OperationSlot) bool {
	if slot.Modality == models.ModalityVideo ||
		strings.EqualFold(strings.TrimSpace(slot.Name), "video") {
		return true
	}
	for _, contentType := range slot.ContentTypes {
		if strings.EqualFold(strings.TrimSpace(contentType), string(models.ModalityVideo)) ||
			strings.HasPrefix(strings.ToLower(strings.TrimSpace(contentType)), "video/") {
			return true
		}
	}
	return false
}

func withoutVideoSlots(operation models.Operation) models.Operation {
	cloned := operation.Clone()
	inputs := make([]models.OperationSlot, 0, len(cloned.Inputs))
	for _, slot := range cloned.Inputs {
		if !isVideoSlot(slot) {
			inputs = append(inputs, slot)
		}
	}
	cloned.Inputs = inputs
	return cloned
}

func cloneOperations(operations []models.Operation) []models.Operation {
	if operations == nil {
		return nil
	}
	cloned := make([]models.Operation, len(operations))
	for index, operation := range operations {
		cloned[index] = operation.Clone()
	}
	return cloned
}
