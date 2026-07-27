package operatorsettings_test

import (
	"errors"
	"strings"
	"testing"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
)

// documentPeerFake implements document operations using only Operator Settings
// root contracts and in-memory state. It does not import filesystem, codec,
// CLI, UI, Wire, or Initializer packages.
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

func TestDocumentContract_Characterization_LoadAndUpdateSuccess(t *testing.T) {
	t.Parallel()

	scopeID := "local-00000000-0000-4000-8000-000000000001"
	initial := operatorsettings.Document{
		BackendScopeID: scopeID,
		Defaults: operatorsettings.DocumentDefaults{
			WorkerModelProvider: "codex",
			WorkerModel:         "gpt-5",
		},
		Runtime: operatorsettings.EmptyDocument().Runtime,
		WorkerPresets: []operatorsettings.DocumentWorkerPreset{{
			ID:            "reviewer",
			ModelProvider: "codex",
			Model:         "gpt-5",
		}},
	}
	fake := newDocumentPeerFake(map[string]operatorsettings.Document{
		"/home/operator/.you-agent-factory/config.json": initial,
	})

	loaded, err := fake.LoadDocument(operatorsettings.LoadDocumentRequest{
		Path:            "/home/operator/.you-agent-factory/config.json",
		RequireExisting: true,
	})
	if err != nil {
		t.Fatalf("LoadDocument() = %v", err)
	}
	if !loaded.Found ||
		loaded.Document.BackendScopeID != scopeID ||
		loaded.Document.Defaults.WorkerModelProvider != "codex" ||
		len(loaded.Document.WorkerPresets) != 1 {
		t.Fatalf("LoadDocument() = %#v", loaded)
	}

	nextModel := "gpt-5.2"
	updated, err := fake.ApplyDocumentUpdate(operatorsettings.ApplyDocumentUpdateRequest{
		Path:                 "/home/operator/.you-agent-factory/config.json",
		ExpectedBackendScope: scopeID,
		ProviderModel: operatorsettings.DocumentProviderModelUpdate{
			Model: &nextModel,
		},
	})
	if err != nil {
		t.Fatalf("ApplyDocumentUpdate() = %v", err)
	}
	if !updated.Persisted ||
		updated.Document.Defaults.WorkerModel != nextModel ||
		updated.Document.Defaults.WorkerModelProvider != "codex" {
		t.Fatalf("ApplyDocumentUpdate() = %#v", updated)
	}
}

func TestDocumentContract_Characterization_TypedFailures(t *testing.T) {
	t.Parallel()

	fake := newDocumentPeerFake(nil)

	_, err := fake.LoadDocument(operatorsettings.LoadDocumentRequest{})
	if !errors.Is(err, operatorsettings.ErrDocumentMalformed) {
		t.Fatalf("empty load path error = %v, want ErrDocumentMalformed", err)
	}

	_, err = fake.LoadDocument(operatorsettings.LoadDocumentRequest{
		Path:            "/missing/config.json",
		RequireExisting: true,
	})
	if !errors.Is(err, operatorsettings.ErrDocumentNotFound) {
		t.Fatalf("missing required document error = %v, want ErrDocumentNotFound", err)
	}

	unsupported := "unsupported-provider"
	_, err = fake.ApplyDocumentUpdate(operatorsettings.ApplyDocumentUpdateRequest{
		Path: "/home/operator/.you-agent-factory/config.json",
		ProviderModel: operatorsettings.DocumentProviderModelUpdate{
			Provider: &unsupported,
		},
	})
	if !errors.Is(err, operatorsettings.ErrDocumentUnsupported) {
		t.Fatalf("unsupported provider error = %v, want ErrDocumentUnsupported", err)
	}

	scopeID := "local-00000000-0000-4000-8000-000000000002"
	fake = newDocumentPeerFake(map[string]operatorsettings.Document{
		"/home/operator/.you-agent-factory/config.json": {
			BackendScopeID: scopeID,
			Runtime:        operatorsettings.EmptyDocument().Runtime,
		},
	})
	model := "gpt-5"
	_, err = fake.ApplyDocumentUpdate(operatorsettings.ApplyDocumentUpdateRequest{
		Path:                 "/home/operator/.you-agent-factory/config.json",
		ExpectedBackendScope: "local-stale-scope",
		ProviderModel: operatorsettings.DocumentProviderModelUpdate{
			Model: &model,
		},
	})
	if !errors.Is(err, operatorsettings.ErrDocumentConflict) {
		t.Fatalf("backend scope conflict error = %v, want ErrDocumentConflict", err)
	}

	var failure operatorsettings.DocumentFailure
	if !errors.As(err, &failure) || failure.Kind != operatorsettings.DocumentFailureKindConflict {
		t.Fatalf("conflict error = %#v, want DocumentFailureKindConflict", err)
	}
}

func TestDocumentContract_Characterization_ValueConstruction(t *testing.T) {
	t.Parallel()

	document := operatorsettings.Document{
		BackendScopeID: "local-00000000-0000-4000-8000-000000000003",
		Defaults: operatorsettings.DocumentDefaults{
			WorkerModelProvider: "codex",
			WorkerModel:         "gpt-5",
		},
		Runtime: operatorsettings.DocumentRuntimeSettings{
			Logging: operatorsettings.DocumentRuntimeArtifactSettings{
				MaxSizeMB:  100,
				MaxBackups: 20,
				MaxAgeDays: 30,
			},
			Metrics: operatorsettings.DocumentRuntimeArtifactSettings{
				MaxSizeMB:  100,
				MaxBackups: 20,
				MaxAgeDays: 30,
			},
		},
		WorkerPresets: []operatorsettings.DocumentWorkerPreset{{
			ID:              "reviewer",
			ModelProvider:   "codex",
			Model:           "gpt-5",
			ReasoningEffort: "medium",
		}},
	}
	cloned := document.Clone()
	cloned.Defaults.WorkerModel = "mutated"
	if document.Defaults.WorkerModel == "mutated" {
		t.Fatalf("Clone() did not detach defaults: %#v", document.Defaults)
	}
	if len(cloned.WorkerPresets) != 1 || cloned.WorkerPresets[0].ID != "reviewer" {
		t.Fatalf("Clone() worker presets = %#v", cloned.WorkerPresets)
	}
}
