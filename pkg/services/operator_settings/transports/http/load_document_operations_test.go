package http

import (
	"context"
	"errors"
	"testing"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
)

func TestAdapter_LoadDocumentInvokesFakeRootAndEncodesSuccess(t *testing.T) {
	t.Parallel()

	configPath := "/home/operator/.you-agent-factory/config.json"
	scopeID := "local-00000000-0000-4000-8000-000000000010"
	var invoked bool
	fake := &rootFake{
		loadDocument: func(
			request operatorsettings.LoadDocumentRequest,
		) (operatorsettings.LoadDocumentResult, error) {
			invoked = true
			if request.Path != configPath || !request.RequireExisting {
				t.Fatalf("LoadDocumentRequest = %#v, want path %q with RequireExisting", request, configPath)
			}
			return operatorsettings.LoadDocumentResult{
				Found: true,
				Path:  configPath,
				Document: operatorsettings.Document{
					BackendScopeID: scopeID,
					Defaults: operatorsettings.DocumentDefaults{
						WorkerModelProvider: "codex",
						WorkerModel:         "gpt-5",
					},
					Runtime: operatorsettings.EmptyDocument().Runtime,
				},
			}, nil
		},
	}
	adapter := NewAdapterFromRoot(RootBinding{Settings: fake})

	response, err := adapter.LoadDocument(context.Background(), LoadDocumentInput{
		Path:            configPath,
		RequireExisting: true,
	})
	if !invoked {
		t.Fatal("LoadDocument did not invoke the injected Settings root")
	}
	if err != nil {
		t.Fatalf("LoadDocument error = %v", err)
	}
	if !response.Found || response.Path != configPath {
		t.Fatalf("response metadata = found=%v path=%q, want found at %q", response.Found, response.Path, configPath)
	}
	if response.Document.BackendScopeID == nil || *response.Document.BackendScopeID != scopeID {
		t.Fatalf("response.Document.BackendScopeID = %#v, want %q", response.Document.BackendScopeID, scopeID)
	}
	if response.Document.Defaults == nil ||
		response.Document.Defaults.WorkerModelProvider == nil ||
		*response.Document.Defaults.WorkerModelProvider != "codex" {
		t.Fatalf("response.Document.Defaults = %#v, want codex/gpt-5 defaults", response.Document.Defaults)
	}
}

func TestAdapter_LoadDocumentRejectsInvalidInputBeforeFakeRoot(t *testing.T) {
	t.Parallel()

	fake := &rootFake{
		loadDocument: func(operatorsettings.LoadDocumentRequest) (operatorsettings.LoadDocumentResult, error) {
			t.Fatal("fake root must not be invoked for invalid load input")
			return operatorsettings.LoadDocumentResult{}, nil
		},
	}
	adapter := NewAdapterFromRoot(RootBinding{Settings: fake})

	_, err := adapter.LoadDocument(context.Background(), LoadDocumentInput{RequireExisting: true})
	if err == nil || !IsLoadDocumentBadRequest(err) {
		t.Fatalf("LoadDocument error = %v, want typed bad request", err)
	}
}

func TestAdapter_LoadDocumentPropagatesTypedRootFailures(t *testing.T) {
	t.Parallel()

	fake := &rootFake{
		loadDocument: func(
			operatorsettings.LoadDocumentRequest,
		) (operatorsettings.LoadDocumentResult, error) {
			return operatorsettings.LoadDocumentResult{}, operatorsettings.ErrDocumentNotFound
		},
	}
	adapter := NewAdapterFromRoot(RootBinding{Settings: fake})

	_, err := adapter.LoadDocument(context.Background(), LoadDocumentInput{
		Path:            "/tmp/missing.json",
		RequireExisting: true,
	})
	if err == nil || !errors.Is(err, operatorsettings.ErrDocumentNotFound) {
		t.Fatalf("LoadDocument error = %v, want ErrDocumentNotFound", err)
	}
}
