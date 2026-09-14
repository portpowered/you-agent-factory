package local

import (
	"testing"

	models "github.com/portpowered/infinite-you/pkg/services/models"
)

func TestProjectorEffectiveVideoOmitsOnlyVideoWhenProjectorMissing(t *testing.T) {
	t.Parallel()

	definition, ok := (models.BuiltInCatalog{}).ModelDefinitionFor(models.BuiltInModelNameLLM)
	if !ok {
		t.Fatal("built-in LLM definition is missing")
	}
	effective, diagnostics, unavailable := ProjectEffectiveVideoDefinition(
		definition,
		RuntimeCacheInspection{Supported: true},
	)
	if !unavailable || diagnostics[VideoReadinessDiagnostic] != VideoReadinessMissing {
		t.Fatalf("missing-projector projection = %#v, %#v, unavailable=%t", effective, diagnostics, unavailable)
	}
	operation := findOperation(t, effective.Operations, models.OperationOMNI)
	if hasVideoSlot(operation) {
		t.Fatalf("effective OMNI inputs = %#v, want VIDEO omitted", operation.Inputs)
	}
	if !hasNamedSlot(operation, "prompt") {
		t.Fatalf("effective OMNI inputs = %#v, want prompt retained", operation.Inputs)
	}
}

func TestProjectorEffectiveVideoRejectsCorruptProjector(t *testing.T) {
	t.Parallel()

	definition, _ := (models.BuiltInCatalog{}).ModelDefinitionFor(models.BuiltInModelNameLLM)
	inspection := verifiedProjectorInspection()
	inspection.ObservedArtifacts[1].SHA256 = "corrupt"
	effective, diagnostics := ProjectEffectiveVideoOperations(
		models.BuiltInModelNameLLM, definition.Operations, inspection,
	)
	if diagnostics[VideoReadinessDiagnostic] != VideoReadinessMissing {
		t.Fatalf("corrupt projector diagnostics = %#v, want stable missing reason", diagnostics)
	}
	if hasVideoSlot(findOperation(t, effective, models.OperationOMNI)) {
		t.Fatal("corrupt projector retained VIDEO capability")
	}
}

func TestProjectorEffectiveVideoRestoresVideoForVerifiedProjector(t *testing.T) {
	t.Parallel()

	definition, _ := (models.BuiltInCatalog{}).ModelDefinitionFor(models.BuiltInModelNameLLM)
	effective, diagnostics := ProjectEffectiveVideoOperations(
		models.BuiltInModelNameLLM, definition.Operations, verifiedProjectorInspection(),
	)
	if diagnostics[VideoReadinessDiagnostic] != VideoReadinessVerified ||
		diagnostics[EffectiveOperationsDiagnostic] != "verified-runtime-assets" {
		t.Fatalf("verified projector diagnostics = %#v", diagnostics)
	}
	if !hasVideoSlot(findOperation(t, effective, models.OperationOMNI)) {
		t.Fatal("verified projector omitted VIDEO capability")
	}
}

func TestProjectorEffectiveVideoLeavesNonLLMOperationUnchanged(t *testing.T) {
	t.Parallel()

	definition, _ := (models.BuiltInCatalog{}).ModelDefinitionFor(models.BuiltInModelNameASR)
	effective, diagnostics := ProjectEffectiveVideoOperations(
		models.BuiltInModelNameASR, definition.Operations, RuntimeCacheInspection{},
	)
	if diagnostics != nil {
		t.Fatalf("non-LLM diagnostics = %#v, want nil", diagnostics)
	}
	if len(effective) != len(definition.Operations) {
		t.Fatalf("non-LLM operations = %#v, want %#v", effective, definition.Operations)
	}
}

func verifiedProjectorInspection() RuntimeCacheInspection {
	model := models.AssetRequirement{Name: "gemma-4-E4B-it-Q4_K_M.gguf", Bytes: 10, SHA256: "model"}
	projector := models.AssetRequirement{Name: "mmproj-F16.gguf", Bytes: 20, SHA256: "projector"}
	return RuntimeCacheInspection{
		Supported: true, Installed: true, ManifestPresent: true, ManifestValid: true,
		IntegrityVerified: true,
		ExpectedArtifacts: []models.AssetRequirement{model, projector},
		ObservedArtifacts: []models.AssetArtifact{
			{Kind: models.AssetArtifactKindModel, Name: model.Name, Bytes: model.Bytes, SHA256: model.SHA256},
			{Kind: models.AssetArtifactKindModel, Name: projector.Name, Bytes: projector.Bytes, SHA256: projector.SHA256},
		},
	}
}

func findOperation(t *testing.T, operations []models.Operation, name string) models.Operation {
	t.Helper()
	for _, operation := range operations {
		if operation.Name == name {
			return operation
		}
	}
	t.Fatalf("operation %q missing from %#v", name, operations)
	return models.Operation{}
}

func hasVideoSlot(operation models.Operation) bool {
	for _, slot := range operation.Inputs {
		if slot.Modality == models.ModalityVideo || slot.Name == "video" {
			return true
		}
	}
	return false
}

func hasNamedSlot(operation models.Operation, name string) bool {
	for _, slot := range operation.Inputs {
		if slot.Name == name {
			return true
		}
	}
	return false
}
