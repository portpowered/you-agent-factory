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

// servicePeerFake implements the full Operator Settings Service using only root
// contracts and in-memory state. It does not import filesystem, codec, CLI, UI,
// Wire, or Initializer packages.
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
		Runtime: operatorsettings.EmptyDocument().Runtime,
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
