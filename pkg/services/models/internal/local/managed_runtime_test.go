package local

import (
	"testing"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	managedruntime "github.com/portpowered/infinite-you/pkg/services/models/internal/managedruntime"
)

func TestBuildManagedRuntime_MapsCompatibilityFieldsToManagedContract(t *testing.T) {
	t.Parallel()
	summary := managedRuntimeSummary{
		name:       "OMNIVOICE_Q4_K_M",
		locality:   managedruntime.LocalityLocal,
		readiness:  managedruntime.ReadinessStateReady,
		lifecycle:  managedruntime.LifecycleStateNotInstalled,
		operations: []managedruntime.Operation{{Name: "TTS"}},
	}
	diagnostics := map[string]string{"statusReason": "ready"}

	managed := buildManagedRuntime(summary, diagnostics)

	if managed.Identity != summary.name {
		t.Fatalf("identity = %q, want %q", managed.Identity, summary.name)
	}
	if managed.ReadinessState != managedruntime.ReadinessStateMissing {
		t.Fatalf("readiness = %s, want MISSING", managed.ReadinessState)
	}
	if managed.LifecycleState != managedruntime.LifecycleStateNotInstalled {
		t.Fatalf("lifecycle = %s, want NOT_INSTALLED", managed.LifecycleState)
	}
	if managed.Locality != summary.locality {
		t.Fatalf("locality = %s, want %s", managed.Locality, summary.locality)
	}
	if len(managed.SupportedOperations) != 1 || managed.SupportedOperations[0].Name != "TTS" {
		t.Fatalf("supported operations = %#v, want one TTS operation", managed.SupportedOperations)
	}
	if managed.Diagnostics["readinessState"] != "MISSING" || managed.Diagnostics["statusReason"] != "ready" {
		t.Fatalf("diagnostics = %#v, want managed-runtime projections", managed.Diagnostics)
	}
}

func TestBuildManagedRuntimeProjection_RejectsReadyNotInstalledForManagedLocal(t *testing.T) {
	t.Parallel()

	runtime := buildManagedRuntime(managedRuntimeSummary{
		name:      "OMNIVOICE_Q4_K_M",
		locality:  managedruntime.LocalityLocal,
		readiness: managedruntime.ReadinessStateReady,
		lifecycle: managedruntime.LifecycleStateNotInstalled,
	}, nil)

	if runtime.ReadinessState != managedruntime.ReadinessStateMissing ||
		runtime.LifecycleState != managedruntime.LifecycleStateNotInstalled {
		t.Fatalf("runtime state = (%s, %s), want MISSING/NOT_INSTALLED",
			runtime.ReadinessState, runtime.LifecycleState)
	}
}

func TestListModels_PopulatesManagedRuntimeContract(t *testing.T) {
	t.Parallel()
	loaded := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))

	models, err := ListModels(loaded)
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if len(models.Results) != 1 {
		t.Fatalf("models count = %d, want 1", len(models.Results))
	}
	model := models.Results[0]
	if model.ManagedRuntime.Identity != "OMNIVOICE_Q4_K_M" {
		t.Fatalf("managed runtime identity = %q, want OMNIVOICE_Q4_K_M", model.ManagedRuntime.Identity)
	}
	if model.ManagedRuntime.ReadinessState != managedruntime.ReadinessStateMissing {
		t.Fatalf("managed readiness = %s, want MISSING", model.ManagedRuntime.ReadinessState)
	}
	if model.ManagedRuntime.LifecycleState != managedruntime.LifecycleStateNotInstalled {
		t.Fatalf("managed lifecycle = %s, want NOT_INSTALLED", model.ManagedRuntime.LifecycleState)
	}
}

func TestBuildManagedRuntimeProjection_ReportsLoadingAndFailedCacheStates(t *testing.T) {
	t.Parallel()
	loading := buildManagedRuntimeProjection(managedRuntimeProjection{
		summary: managedRuntimeSummary{
			name:     "OMNIVOICE_Q4_K_M",
			locality: managedruntime.LocalityLocal,
		},
		cacheInspection: &RuntimeCacheInspection{
			Supported:          true,
			Installed:          false,
			InstalledFileCount: 1,
			MissingAssets:      []string{"omnivoice-tokenizer-Q4_K_M.gguf"},
		},
	})
	if loading.ReadinessState != managedruntime.ReadinessStateLoading ||
		loading.LifecycleState != managedruntime.LifecycleStateInstalling {
		t.Fatalf("loading runtime = (%s, %s), want LOADING/INSTALLING", loading.ReadinessState, loading.LifecycleState)
	}

	failed := buildManagedRuntimeProjection(managedRuntimeProjection{
		summary: managedRuntimeSummary{
			name:     "OMNIVOICE_Q4_K_M",
			locality: managedruntime.LocalityLocal,
		},
		cacheInspection: &RuntimeCacheInspection{
			Supported:        true,
			Installed:        false,
			PartialArtifacts: true,
		},
	})
	if failed.ReadinessState != managedruntime.ReadinessStateFailed ||
		failed.LifecycleState != managedruntime.LifecycleStateNotInstalled {
		t.Fatalf("failed runtime = (%s, %s), want FAILED/NOT_INSTALLED", failed.ReadinessState, failed.LifecycleState)
	}
}

func TestBuildManagedRuntimeProjection_ReportsInstalledCacheState(t *testing.T) {
	t.Parallel()
	summary := managedRuntimeSummary{
		name:      "OMNIVOICE_Q4_K_M",
		locality:  managedruntime.LocalityLocal,
		readiness: managedruntime.ReadinessStateReady,
		lifecycle: managedruntime.LifecycleStateNotInstalled,
	}
	inspection := RuntimeCacheInspection{
		Supported:          true,
		Installed:          true,
		Revision:           "rev1",
		CachePath:          "/tmp/models/OMNIVOICE_Q4_K_M/rev1",
		InstalledFileCount: 2,
	}
	managed := buildManagedRuntimeProjection(managedRuntimeProjection{
		summary:         summary,
		baseDiagnostics: map[string]string{"statusReason": "ready"},
		cacheInspection: &inspection,
		includeInspect:  true,
	})
	if managed.LifecycleState != managedruntime.LifecycleStateInstalled {
		t.Fatalf("lifecycle = %s, want INSTALLED", managed.LifecycleState)
	}
	if managed.Diagnostics["revision"] != "rev1" {
		t.Fatalf("diagnostics = %#v, want revision detail", managed.Diagnostics)
	}
	if managed.CachePath == nil || *managed.CachePath != inspection.CachePath {
		t.Fatalf("cache path = %v, want %q", managed.CachePath, inspection.CachePath)
	}
}

func TestProjectManagedRuntimeState_UsesCacheAndHostFacts(t *testing.T) {
	t.Parallel()
	for _, tc := range managedRuntimeProjectionCases() {
		t.Run(tc.name, func(t *testing.T) {
			projection := managedruntime.ProjectManagedRuntimeState(tc.cache, tc.host)
			if projection.ReadinessState != tc.readiness || projection.LifecycleState != tc.lifecycle {
				t.Fatalf("projection = (%s, %s), want (%s, %s)",
					projection.ReadinessState, projection.LifecycleState, tc.readiness, tc.lifecycle)
			}
		})
	}
}

type managedRuntimeProjectionCase struct {
	name      string
	cache     models.ManagedRuntimeCacheFacts
	host      models.ManagedRuntimeHostFacts
	readiness models.ReadinessState
	lifecycle models.LifecycleState
}

func managedRuntimeProjectionCases() []managedRuntimeProjectionCase {
	return append(managedRuntimeProjectionMissingCases(), managedRuntimeProjectionHostCases()...)
}

func managedRuntimeProjectionMissingCases() []managedRuntimeProjectionCase {
	expected := managedRuntimeProjectionExpectedArtifacts()
	return []managedRuntimeProjectionCase{
		{
			name: "missing manifest and artifacts",
			cache: models.ManagedRuntimeCacheFacts{
				Locality: models.LocalityLocal, Supported: true, ExpectedArtifacts: expected,
			},
			readiness: models.ReadinessStateMissing, lifecycle: models.LifecycleStateNotInstalled,
		},
		{
			name: "active pull",
			cache: models.ManagedRuntimeCacheFacts{
				Locality: models.LocalityLocal, Supported: true, ActivePull: true,
				ManifestPresent: true, ManifestValid: true, ExpectedArtifacts: expected,
				ObservedArtifacts: []models.AssetArtifact{{Name: "weights.gguf", Bytes: 1}},
			},
			readiness: models.ReadinessStateLoading, lifecycle: models.LifecycleStateInstalling,
		},
		{
			name: "active pull while manifest is pending",
			cache: models.ManagedRuntimeCacheFacts{
				Locality: models.LocalityLocal, Supported: true, ActivePull: true,
				ExpectedArtifacts: expected, FailureReason: "managed cache manifest is invalid",
			},
			readiness: models.ReadinessStateLoading, lifecycle: models.LifecycleStateInstalling,
		},
		{
			name: "wrong-sized artifact",
			cache: models.ManagedRuntimeCacheFacts{
				Locality: models.LocalityLocal, Supported: true,
				ManifestPresent: true, ManifestValid: true, ExpectedArtifacts: expected,
				ObservedArtifacts: []models.AssetArtifact{
					{Name: "weights.gguf", Bytes: 3}, {Name: "tokenizer.gguf", Bytes: 2},
				},
			},
			readiness: models.ReadinessStateFailed, lifecycle: models.LifecycleStateNotInstalled,
		},
		{
			name: "corrupt cache",
			cache: models.ManagedRuntimeCacheFacts{
				Locality: models.LocalityLocal, Supported: true,
				ManifestPresent: true, ManifestValid: true, ExpectedArtifacts: expected,
				ObservedArtifacts: []models.AssetArtifact{
					{Name: "weights.gguf", Bytes: 4}, {Name: "tokenizer.gguf", Bytes: 2},
				},
				FailureReason: "asset integrity verification failed",
			},
			readiness: models.ReadinessStateFailed, lifecycle: models.LifecycleStateNotInstalled,
		},
	}
}

func managedRuntimeProjectionHostCases() []managedRuntimeProjectionCase {
	expected := managedRuntimeProjectionExpectedArtifacts()
	completeCache := func() models.ManagedRuntimeCacheFacts {
		return models.ManagedRuntimeCacheFacts{
			Locality: models.LocalityLocal, Supported: true,
			ManifestPresent: true, ManifestValid: true, ExpectedArtifacts: expected,
			ObservedArtifacts: []models.AssetArtifact{
				{Name: "weights.gguf", Bytes: 4}, {Name: "tokenizer.gguf", Bytes: 2},
			},
			IntegrityVerified: true,
		}
	}
	return []managedRuntimeProjectionCase{
		{
			name: "verified cache", cache: completeCache(),
			readiness: models.ReadinessStateReady, lifecycle: models.LifecycleStateInstalled,
		},
		{
			name: "host loading", cache: completeCache(),
			host: models.ManagedRuntimeHostFacts{
				Observed: true, ReadinessState: models.ReadinessStateLoading,
				LifecycleState: models.LifecycleStateLoading,
			},
			readiness: models.ReadinessStateLoading, lifecycle: models.LifecycleStateLoading,
		},
		{
			name: "host loaded", cache: completeCache(),
			host: models.ManagedRuntimeHostFacts{
				Observed: true, ReadinessState: models.ReadinessStateReady,
				LifecycleState: models.LifecycleStateLoaded,
			},
			readiness: models.ReadinessStateReady, lifecycle: models.LifecycleStateLoaded,
		},
	}
}

func managedRuntimeProjectionExpectedArtifacts() []models.AssetRequirement {
	return []models.AssetRequirement{
		{Name: "weights.gguf", Bytes: 4},
		{Name: "tokenizer.gguf", Bytes: 2},
	}
}

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
