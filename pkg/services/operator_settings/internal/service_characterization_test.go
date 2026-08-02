package internal_test

import (
	"context"
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
			Document: operatorsettings.EmptyDocument,
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
		document = operatorsettings.EmptyDocument
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

func (fake *servicePeerFake) DefaultConfigPath(string) string { return "" }

func (fake *servicePeerFake) LoadFileConfig(string) (operatorsettings.Config, error) {
	return operatorsettings.Config{}, errors.New("characterization fake does not implement file config")
}

func (fake *servicePeerFake) ResolveFromHomeWithEnvironment(
	string,
	operatorsettings.Defaults,
	operatorsettings.FlagOverrides,
) (operatorsettings.ResolvedDefaults, error) {
	return operatorsettings.ResolvedDefaults{}, errors.New("characterization fake does not implement defaults")
}

func (fake *servicePeerFake) EnsureLocalBackendScope(string) (operatorsettings.ResolvedBackendScope, error) {
	return operatorsettings.ResolvedBackendScope{}, errors.New("characterization fake does not implement identity")
}

func (fake *servicePeerFake) ProjectInputInventory() operatorsettings.InputInventory {
	return operatorsettings.InputInventory{}
}

func (fake *servicePeerFake) DeriveProviderBackendScopeID(string, string, string) string { return "" }

func (fake *servicePeerFake) IsLocalBackendScopeID(string) bool { return false }

func (fake *servicePeerFake) ConfigureACPIntegrationAdd(
	context.Context,
	string,
	operatorsettings.ACPIntegration,
) (operatorsettings.Document, error) {
	return operatorsettings.Document{}, errors.New("characterization fake does not implement ACP")
}

func (fake *servicePeerFake) ConfigureACPIntegrationDelete(
	context.Context,
	string,
	string,
) (operatorsettings.Document, error) {
	return operatorsettings.Document{}, errors.New("characterization fake does not implement ACP")
}

func (fake *servicePeerFake) EnsurePackagedACPIntegrations(
	context.Context,
	string,
	[]operatorsettings.ACPIntegration,
) (operatorsettings.Document, error) {
	return operatorsettings.Document{}, errors.New("characterization fake does not implement ACP")
}

func (fake *servicePeerFake) ResolveACPAgentProfile(
	request operatorsettings.ResolveACPAgentProfileRequest,
) (operatorsettings.ResolveACPAgentProfileResult, error) {
	if request.AuthoredProfile == nil {
		return operatorsettings.ResolveACPAgentProfileResult{Profile: operatorsettings.BuiltInACPAgentProfile()}, nil
	}
	profile, err := operatorsettings.NormalizeACPAgentProfile(
		request.AuthoredProfile.DefaultFactoryReference,
		request.AuthoredProfile.Allowlist,
	)
	if err != nil {
		return operatorsettings.ResolveACPAgentProfileResult{}, err
	}
	return operatorsettings.ResolveACPAgentProfileResult{Profile: profile}, nil
}

func (fake *servicePeerFake) UpdateACPAgentProfile(
	context.Context,
	operatorsettings.UpdateACPAgentProfileRequest,
) (operatorsettings.UpdateACPAgentProfileResult, error) {
	return operatorsettings.UpdateACPAgentProfileResult{}, errors.New("characterization fake does not implement ACP")
}

func TestService_Characterization_FakeImplementsSingularSeam(t *testing.T) {
	t.Parallel()

	scopeID := "local-00000000-0000-4000-8000-000000000010"
	configPath := "/home/operator/.you-agent-factory/config.json"
	initial := operatorsettings.Document{
		BackendScopeID: scopeID,
		Defaults: operatorsettings.DocumentDefaults{
			WorkerModelProvider: "codex",
			WorkerModel:         "gpt-5",
		},
		Runtime: operatorsettings.EmptyDocument.Runtime,
	}
	fake := newServicePeerFake(map[string]operatorsettings.Document{
		configPath: initial,
	})
	var svc operatorsettings.Service = fake

	loaded, err := svc.LoadDocument(operatorsettings.LoadDocumentRequest{
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
	updated, err := svc.ApplyDocumentUpdate(operatorsettings.ApplyDocumentUpdateRequest{
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

	resolved, err := svc.ResolveEffective(operatorsettings.ResolveEffectiveRequest{
		DocumentBaseline: loaded.Document.Defaults,
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

func TestService_Characterization_TypedFailures(t *testing.T) {
	t.Parallel()

	fake := newServicePeerFake(nil)
	var svc operatorsettings.Service = fake

	_, err := svc.LoadDocument(operatorsettings.LoadDocumentRequest{})
	if !errors.Is(err, operatorsettings.ErrDocumentMalformed) {
		t.Fatalf("empty load path error = %v, want ErrDocumentMalformed", err)
	}

	unsupported := "unsupported-provider"
	_, err = svc.ApplyDocumentUpdate(operatorsettings.ApplyDocumentUpdateRequest{
		Path: "/home/operator/.you-agent-factory/config.json",
		ProviderModel: operatorsettings.DocumentProviderModelUpdate{
			Provider: &unsupported,
		},
	})
	if !errors.Is(err, operatorsettings.ErrDocumentUnsupported) {
		t.Fatalf("unsupported provider error = %v, want ErrDocumentUnsupported", err)
	}

	_, err = svc.ResolveEffective(operatorsettings.ResolveEffectiveRequest{
		InvocationOverrides: operatorsettings.EffectiveOverrideFacts{
			WorkerModelProvider: "unsupported-provider",
		},
		ConfigPath: "/tmp/config.json",
	})
	if !errors.Is(err, operatorsettings.ErrResolutionUnsupportedOverride) {
		t.Fatalf("unsupported override error = %v, want ErrResolutionUnsupportedOverride", err)
	}

	_, err = svc.ResolveEffective(operatorsettings.ResolveEffectiveRequest{
		InvocationOverrides: operatorsettings.EffectiveOverrideFacts{
			WorkerModelProvider: "DEFAULT",
		},
		ConfigPath: "/tmp/config.json",
	})
	if !errors.Is(err, operatorsettings.ErrResolutionInvalidInput) {
		t.Fatalf("unresolved DEFAULT error = %v, want ErrResolutionInvalidInput", err)
	}
}
