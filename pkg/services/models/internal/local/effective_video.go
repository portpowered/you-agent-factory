package local

import (
	"path"
	"strings"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	scopedassets "github.com/portpowered/infinite-you/pkg/services/models/internal/services/assets"
)

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
