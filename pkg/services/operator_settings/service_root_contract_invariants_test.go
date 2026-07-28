package operatorsettings_test

import (
	"errors"
	"strings"
	"testing"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
)

type documentPeerFake struct {
	documents map[string]operatorsettings.Document
}

func newDocumentPeerFake(entries map[string]operatorsettings.Document) *documentPeerFake {
	copied := make(map[string]operatorsettings.Document, len(entries))
	for path, document := range entries {
		copied[path] = document.Clone()
	}
	return &documentPeerFake{documents: copied}
}

func (fake *documentPeerFake) LoadDocument(
	request operatorsettings.LoadDocumentRequest,
) (operatorsettings.LoadDocumentResult, error) {
	if err := request.Validate(); err != nil {
		return operatorsettings.LoadDocumentResult{}, err
	}
	path := strings.TrimSpace(request.Path)
	document, found := fake.documents[path]
	if !found {
		if request.RequireExisting {
			return operatorsettings.LoadDocumentResult{}, operatorsettings.DocumentFailure{
				Kind: operatorsettings.DocumentFailureKindNotFound,
				Path: path,
			}
		}
		return operatorsettings.LoadDocumentResult{
			Document: operatorsettings.EmptyDocument(),
			Path:     path,
			Found:    false,
		}, nil
	}
	return operatorsettings.LoadDocumentResult{
		Document: document.Clone(),
		Path:     path,
		Found:    true,
	}, nil
}

func (fake *documentPeerFake) ApplyDocumentUpdate(
	request operatorsettings.ApplyDocumentUpdateRequest,
) (operatorsettings.ApplyDocumentUpdateResult, error) {
	if err := request.Validate(); err != nil {
		return operatorsettings.ApplyDocumentUpdateResult{}, err
	}
	path := strings.TrimSpace(request.Path)
	document, found := fake.documents[path]
	if !found {
		document = operatorsettings.EmptyDocument()
	}
	expected := strings.TrimSpace(request.ExpectedBackendScope)
	if expected != "" && document.BackendScopeID != expected {
		return operatorsettings.ApplyDocumentUpdateResult{}, operatorsettings.DocumentFailure{
			Kind:    operatorsettings.DocumentFailureKindConflict,
			Message: "backend scope mismatch",
			Path:    path,
		}
	}
	updated, err := fake.mergeProviderModelUpdate(document, request.ProviderModel)
	if err != nil {
		return operatorsettings.ApplyDocumentUpdateResult{}, err
	}
	fake.documents[path] = updated
	return operatorsettings.ApplyDocumentUpdateResult{
		Document:  updated.Clone(),
		Path:      path,
		Persisted: true,
	}, nil
}

func (fake *documentPeerFake) mergeProviderModelUpdate(
	document operatorsettings.Document,
	update operatorsettings.DocumentProviderModelUpdate,
) (operatorsettings.Document, error) {
	if update.Provider != nil {
		provider := strings.TrimSpace(*update.Provider)
		if provider == "" {
			return operatorsettings.Document{}, operatorsettings.DocumentFailure{
				Kind:    operatorsettings.DocumentFailureKindMalformed,
				Message: "worker model provider is required",
			}
		}
		if provider == "unsupported-provider" {
			return operatorsettings.Document{}, operatorsettings.DocumentFailure{
				Kind:    operatorsettings.DocumentFailureKindUnsupported,
				Message: provider,
			}
		}
		document.Defaults.WorkerModelProvider = provider
	}
	if update.Model != nil {
		document.Defaults.WorkerModel = strings.TrimSpace(*update.Model)
	}
	return document, nil
}

type resolutionPeerFake struct{}

func newResolutionPeerFake() *resolutionPeerFake {
	return &resolutionPeerFake{}
}

func (fake *resolutionPeerFake) ResolveEffective(
	request operatorsettings.ResolveEffectiveRequest,
) (operatorsettings.ResolveEffectiveResult, error) {
	if err := request.Validate(); err != nil {
		return operatorsettings.ResolveEffectiveResult{}, err
	}

	providerRaw, providerSource := fake.winningLayerValue(
		request.DocumentBaseline.WorkerModelProvider,
		request.EnvironmentOverrides.WorkerModelProvider,
		request.InvocationOverrides.WorkerModelProvider,
	)
	modelRaw, modelSource := fake.winningLayerValue(
		request.DocumentBaseline.WorkerModel,
		request.EnvironmentOverrides.WorkerModel,
		request.InvocationOverrides.WorkerModel,
	)

	resolvedProvider, err := fake.resolveWorkerModelProvider(
		providerRaw,
		providerSource,
		request,
	)
	if err != nil {
		return operatorsettings.ResolveEffectiveResult{}, err
	}

	return operatorsettings.ResolveEffectiveResult{
		Selection: operatorsettings.EffectiveSelection{
			WorkerModelProvider:       resolvedProvider,
			WorkerModel:               strings.TrimSpace(modelRaw),
			WorkerModelProviderSource: providerSource,
			WorkerModelSource:         modelSource,
			ConfigPath:                strings.TrimSpace(request.ConfigPath),
		},
	}, nil
}

func (fake *resolutionPeerFake) winningLayerValue(
	baselineValue, envValue, flagValue string,
) (string, operatorsettings.EffectiveLayerSource) {
	switch {
	case strings.TrimSpace(flagValue) != "":
		return strings.TrimSpace(flagValue), operatorsettings.EffectiveLayerSourceFlag
	case strings.TrimSpace(envValue) != "":
		return strings.TrimSpace(envValue), operatorsettings.EffectiveLayerSourceEnv
	case strings.TrimSpace(baselineValue) != "":
		return strings.TrimSpace(baselineValue), operatorsettings.EffectiveLayerSourceFile
	default:
		return "", ""
	}
}

func (fake *resolutionPeerFake) resolveWorkerModelProvider(
	raw string,
	winningSource operatorsettings.EffectiveLayerSource,
	request operatorsettings.ResolveEffectiveRequest,
) (string, error) {
	if raw == "" {
		return "", nil
	}

	canonical, ok := fake.canonicalizeProvider(raw)
	if !ok {
		return "", operatorsettings.ResolutionFailure{
			Kind:    operatorsettings.ResolutionFailureKindUnsupportedOverride,
			Message: raw,
			Field:   "workerModelProvider",
		}
	}
	if canonical != "DEFAULT" {
		return canonical, nil
	}

	concreteRaw := fake.concreteProviderBelowSource(winningSource, request)
	if concreteRaw == "" {
		return "", operatorsettings.ResolutionFailure{
			Kind:    operatorsettings.ResolutionFailureKindInvalidInput,
			Message: "symbolic DEFAULT requires a concrete provider from file or environment",
			Field:   "workerModelProvider",
		}
	}
	concreteCanonical, ok := fake.canonicalizeProvider(concreteRaw)
	if !ok || concreteCanonical == "DEFAULT" {
		return "", operatorsettings.ResolutionFailure{
			Kind:    operatorsettings.ResolutionFailureKindInvalidInput,
			Message: "symbolic DEFAULT requires a concrete provider from file or environment",
			Field:   "workerModelProvider",
		}
	}
	return concreteCanonical, nil
}

func (fake *resolutionPeerFake) concreteProviderBelowSource(
	winningSource operatorsettings.EffectiveLayerSource,
	request operatorsettings.ResolveEffectiveRequest,
) string {
	type layer struct {
		source operatorsettings.EffectiveLayerSource
		value  string
	}
	layers := []layer{
		{source: operatorsettings.EffectiveLayerSourceFile, value: request.DocumentBaseline.WorkerModelProvider},
		{source: operatorsettings.EffectiveLayerSourceEnv, value: request.EnvironmentOverrides.WorkerModelProvider},
		{source: operatorsettings.EffectiveLayerSourceFlag, value: request.InvocationOverrides.WorkerModelProvider},
	}

	below := make([]layer, 0, 2)
	for _, layer := range layers {
		if layer.source == winningSource {
			break
		}
		below = append(below, layer)
	}
	for i := len(below) - 1; i >= 0; i-- {
		value := strings.TrimSpace(below[i].value)
		if value == "" || strings.EqualFold(value, "DEFAULT") {
			continue
		}
		return value
	}
	return ""
}

func (fake *resolutionPeerFake) canonicalizeProvider(raw string) (string, bool) {
	value := strings.TrimSpace(raw)
	switch strings.ToLower(value) {
	case "", "default":
		return "DEFAULT", true
	case "codex", "openai":
		return "CODEX", true
	case "claude", "anthropic":
		return "CLAUDE", true
	case "gemini":
		return "GEMINI", true
	default:
		if strings.Contains(value, ".") {
			return value, true
		}
		return "", false
	}
}

type servicePeerFake struct {
	document   *documentPeerFake
	resolution *resolutionPeerFake
}

var _ operatorsettings.Service = (*servicePeerFake)(nil)

func newServicePeerFake(entries map[string]operatorsettings.Document) *servicePeerFake {
	return &servicePeerFake{
		document:   newDocumentPeerFake(entries),
		resolution: newResolutionPeerFake(),
	}
}

func (fake *servicePeerFake) LoadDocument(
	request operatorsettings.LoadDocumentRequest,
) (operatorsettings.LoadDocumentResult, error) {
	return fake.document.LoadDocument(request)
}

func (fake *servicePeerFake) ApplyDocumentUpdate(
	request operatorsettings.ApplyDocumentUpdateRequest,
) (operatorsettings.ApplyDocumentUpdateResult, error) {
	return fake.document.ApplyDocumentUpdate(request)
}

func (fake *servicePeerFake) ResolveEffective(
	request operatorsettings.ResolveEffectiveRequest,
) (operatorsettings.ResolveEffectiveResult, error) {
	return fake.resolution.ResolveEffective(request)
}

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
