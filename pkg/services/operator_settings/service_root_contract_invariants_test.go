package operatorsettings_test

import (
	"errors"
	"testing"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
)

// TestRootContractInvariants_AllSlicesThroughSingularService seals the
// Operator Settings root-contract packet: document operations and effective
// resolution are reachable through one named operatorsettings.Service, a
// peer-shaped fake exercises success and typed-failure paths using only the
// root package, and no second peer-facing Operator Settings authority is
// required.
func TestRootContractInvariants_AllSlicesThroughSingularService(t *testing.T) {
	t.Parallel()

	scopeID := "local-00000000-0000-4000-8000-000000000010"
	configPath := "/home/operator/.you-agent-factory/config.json"
	initial := operatorsettings.Document{
		BackendScopeID: scopeID,
		Defaults: operatorsettings.DocumentDefaults{
			WorkerModelProvider: "codex",
			WorkerModel:         "gpt-5",
		},
		Runtime: operatorsettings.EmptyDocument().Runtime,
	}
	service := newServicePeerFake(map[string]operatorsettings.Document{
		configPath: initial,
	})
	var root operatorsettings.Service = service

	assertSealDocumentSuccess(t, root, configPath, scopeID)
	assertSealDocumentFailures(t, root, configPath)
	assertSealResolutionSuccess(t, root, initial.Defaults, configPath)
	assertSealResolutionFailures(t, root)
}

func assertSealDocumentSuccess(
	t *testing.T,
	service operatorsettings.Service,
	configPath string,
	scopeID string,
) {
	t.Helper()

	loaded, err := service.LoadDocument(operatorsettings.LoadDocumentRequest{
		Path:            configPath,
		RequireExisting: true,
	})
	if err != nil {
		t.Fatalf("LoadDocument() = %v", err)
	}
	if !loaded.Found || loaded.Document.BackendScopeID != scopeID {
		t.Fatalf("LoadDocument() = %#v", loaded)
	}

	nextModel := "gpt-5.2"
	updated, err := service.ApplyDocumentUpdate(operatorsettings.ApplyDocumentUpdateRequest{
		Path:                 configPath,
		ExpectedBackendScope: scopeID,
		ProviderModel: operatorsettings.DocumentProviderModelUpdate{
			Model: &nextModel,
		},
	})
	if err != nil {
		t.Fatalf("ApplyDocumentUpdate() = %v", err)
	}
	if !updated.Persisted || updated.Document.Defaults.WorkerModel != nextModel {
		t.Fatalf("ApplyDocumentUpdate() = %#v", updated)
	}
}

func assertSealDocumentFailures(t *testing.T, service operatorsettings.Service, configPath string) {
	t.Helper()

	_, err := service.LoadDocument(operatorsettings.LoadDocumentRequest{})
	if !errors.Is(err, operatorsettings.ErrDocumentMalformed) {
		t.Fatalf("empty load path error = %v, want ErrDocumentMalformed", err)
	}

	unsupported := "unsupported-provider"
	_, err = service.ApplyDocumentUpdate(operatorsettings.ApplyDocumentUpdateRequest{
		Path: configPath,
		ProviderModel: operatorsettings.DocumentProviderModelUpdate{
			Provider: &unsupported,
		},
	})
	if !errors.Is(err, operatorsettings.ErrDocumentUnsupported) {
		t.Fatalf("unsupported provider error = %v, want ErrDocumentUnsupported", err)
	}
}

func assertSealResolutionSuccess(
	t *testing.T,
	service operatorsettings.Service,
	baseline operatorsettings.DocumentDefaults,
	configPath string,
) {
	t.Helper()

	resolved, err := service.ResolveEffective(operatorsettings.ResolveEffectiveRequest{
		DocumentBaseline: baseline,
		InvocationOverrides: operatorsettings.EffectiveOverrideFacts{
			WorkerModelProvider: "gemini",
			WorkerModel:         "flag-model",
		},
		ConfigPath: configPath,
	})
	if err != nil {
		t.Fatalf("ResolveEffective() = %v", err)
	}
	if resolved.Selection.WorkerModelProvider != "GEMINI" ||
		resolved.Selection.WorkerModel != "flag-model" ||
		resolved.Selection.ConfigPath != configPath {
		t.Fatalf("ResolveEffective() = %#v", resolved.Selection)
	}
}

func assertSealResolutionFailures(t *testing.T, service operatorsettings.Service) {
	t.Helper()

	_, err := service.ResolveEffective(operatorsettings.ResolveEffectiveRequest{
		InvocationOverrides: operatorsettings.EffectiveOverrideFacts{
			WorkerModelProvider: "unsupported-provider",
		},
		ConfigPath: "/tmp/config.json",
	})
	if !errors.Is(err, operatorsettings.ErrResolutionUnsupportedOverride) {
		t.Fatalf("unsupported override error = %v, want ErrResolutionUnsupportedOverride", err)
	}

	_, err = service.ResolveEffective(operatorsettings.ResolveEffectiveRequest{
		InvocationOverrides: operatorsettings.EffectiveOverrideFacts{
			WorkerModelProvider: "DEFAULT",
		},
		ConfigPath: "/tmp/config.json",
	})
	if !errors.Is(err, operatorsettings.ErrResolutionInvalidInput) {
		t.Fatalf("unresolved DEFAULT error = %v, want ErrResolutionInvalidInput", err)
	}
}

func TestRootContract_ContractValuesStayInertWhenHeld(t *testing.T) {
	t.Parallel()

	document := operatorsettings.Document{
		BackendScopeID: "scope-inert",
		Defaults: operatorsettings.DocumentDefaults{
			WorkerModelProvider: "codex",
			WorkerModel:         "gpt-5",
		},
		Runtime: operatorsettings.EmptyDocument().Runtime,
	}
	clonedDocument := document.Clone()
	document.Defaults.WorkerModel = "mutated"
	if clonedDocument.Defaults.WorkerModel == "mutated" {
		t.Fatal("Document.Clone() shares mutable defaults state")
	}

	selection := operatorsettings.EffectiveSelection{
		BackendScopeID:            "local-00000000-0000-4000-8000-000000000010",
		WorkerPresets:             []operatorsettings.DocumentWorkerPreset{{ID: "research", ModelProvider: "CODEX"}},
		WorkerModelProvider:       "CODEX",
		WorkerModel:               "gpt-5",
		WorkerModelProviderSource: operatorsettings.EffectiveLayerSourceFlag,
		WorkerModelSource:         operatorsettings.EffectiveLayerSourceFile,
		ConfigPath:                "/tmp/config.json",
	}
	clonedSelection := selection.Clone()
	selection.WorkerModel = "mutated"
	if clonedSelection.WorkerModel == "mutated" {
		t.Fatal("EffectiveSelection.Clone() shares mutable model state")
	}
	selection.WorkerPresets[0].ID = "mutated"
	if clonedSelection.WorkerPresets[0].ID == "mutated" {
		t.Fatal("EffectiveSelection.Clone() shares mutable worker preset state")
	}

	overrides := operatorsettings.EffectiveOverrideFacts{
		WorkerModelProvider: "gemini",
		WorkerModel:         "gemini-pro",
	}
	clonedOverrides := overrides.Clone()
	overrides.WorkerModel = "mutated"
	if clonedOverrides.WorkerModel == "mutated" {
		t.Fatal("EffectiveOverrideFacts.Clone() shares mutable override state")
	}

	loadRequest := operatorsettings.LoadDocumentRequest{Path: "/tmp/config.json"}
	if err := loadRequest.Validate(); err != nil {
		t.Fatalf("LoadDocumentRequest.Validate() = %v", err)
	}
	loadRequest.Path = "mutated"
	if err := (operatorsettings.LoadDocumentRequest{Path: ""}).Validate(); !errors.Is(
		err, operatorsettings.ErrDocumentMalformed,
	) {
		t.Fatalf("empty LoadDocumentRequest.Validate() = %v, want ErrDocumentMalformed", err)
	}

	model := "gpt-5"
	updateRequest := operatorsettings.ApplyDocumentUpdateRequest{
		Path: "/tmp/config.json",
		ProviderModel: operatorsettings.DocumentProviderModelUpdate{
			Model: &model,
		},
	}
	if err := updateRequest.Validate(); err != nil {
		t.Fatalf("ApplyDocumentUpdateRequest.Validate() = %v", err)
	}

	baseline := operatorsettings.DocumentDefaults{
		WorkerModelProvider: "codex",
		WorkerModel:         "gpt-5",
	}
	resolutionRequest := operatorsettings.ResolveEffectiveRequest{
		DocumentBaseline: baseline,
		ExpectedDocumentBaseline: &operatorsettings.DocumentDefaults{
			WorkerModelProvider: "mutated",
			WorkerModel:         "gpt-5",
		},
	}
	if err := resolutionRequest.Validate(); !errors.Is(err, operatorsettings.ErrResolutionConflict) {
		t.Fatalf("baseline mismatch Validate() = %v, want ErrResolutionConflict", err)
	}

	// Holding contract values must not require a Service implementation or
	// perform filesystem, atomic persist, Wire, or Initializer work.
	var (
		_ operatorsettings.Document
		_ operatorsettings.DocumentDefaults
		_ operatorsettings.EffectiveSelection
		_ operatorsettings.EffectiveOverrideFacts
		_ operatorsettings.LoadDocumentRequest
		_ operatorsettings.LoadDocumentResult
		_ operatorsettings.ApplyDocumentUpdateRequest
		_ operatorsettings.ApplyDocumentUpdateResult
		_ operatorsettings.ResolveEffectiveRequest
		_ operatorsettings.ResolveEffectiveResult
		_ operatorsettings.DocumentFailure
		_ operatorsettings.ResolutionFailure
	)
}

func TestRootContract_FakePeerConstructionIsInert(t *testing.T) {
	t.Parallel()

	fake := newServicePeerFake(nil)
	if fake.document == nil || fake.resolution == nil {
		t.Fatal("fake peer construction returned nil slice collaborators")
	}
	if fake.document.documents == nil {
		t.Fatal("fake peer construction returned nil document store")
	}
	if len(fake.document.documents) != 0 {
		t.Fatalf("fake peer construction initialized documents = %d, want 0", len(fake.document.documents))
	}

	var service operatorsettings.Service = fake
	if service == nil {
		t.Fatal("constructed Service is nil")
	}
}
