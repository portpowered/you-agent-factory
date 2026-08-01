package http

import (
	"context"
	"errors"
	"testing"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
)

func TestAdapter_BindsSettingsRootViaFakeRootSeam(t *testing.T) {
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
				Document: operatorsettings.Document{
					BackendScopeID: scopeID,
					Defaults: operatorsettings.DocumentDefaults{
						WorkerModelProvider: "codex",
						WorkerModel:         "gpt-5",
					},
					Runtime: operatorsettings.EmptyDocument.Runtime,
				},
			}, nil
		},
	}

	adapter := NewAdapterFromRoot(RootBinding{Settings: fake})
	if adapter.Root() != fake {
		t.Fatal("adapter must expose the injected Settings root")
	}

	result, err := adapter.invokeLoadDocument(context.Background(), operatorsettings.LoadDocumentRequest{
		Path:            configPath,
		RequireExisting: true,
	})
	if !invoked {
		t.Fatal("adapter-owned operation did not invoke the injected Settings root")
	}
	if err != nil {
		t.Fatalf("invokeLoadDocument error = %v", err)
	}
	if !result.Found || result.Document.BackendScopeID != scopeID {
		t.Fatalf("LoadDocumentResult = %#v, want found document for %q", result, scopeID)
	}
}

func TestNewAdapter_RejectsNilRoot(t *testing.T) {
	t.Parallel()

	if NewAdapter(nil) != nil {
		t.Fatal("NewAdapter(nil) must return nil")
	}
	if NewAdapterFromRoot(RootBinding{}) != nil {
		t.Fatal("NewAdapterFromRoot with nil Settings must return nil")
	}
}

func TestAdapter_PropagatesTypedRootFailures(t *testing.T) {
	t.Parallel()

	fake := &rootFake{
		loadDocument: func(
			operatorsettings.LoadDocumentRequest,
		) (operatorsettings.LoadDocumentResult, error) {
			return operatorsettings.LoadDocumentResult{}, operatorsettings.ErrDocumentNotFound
		},
	}
	adapter := NewAdapterFromRoot(RootBinding{Settings: fake})

	_, err := adapter.invokeLoadDocument(context.Background(), operatorsettings.LoadDocumentRequest{
		Path:            "/tmp/missing.json",
		RequireExisting: true,
	})
	if !errors.Is(err, operatorsettings.ErrDocumentNotFound) {
		t.Fatalf("invokeLoadDocument error = %v, want ErrDocumentNotFound", err)
	}
}

func TestAdapter_RequiresInjectedRoot(t *testing.T) {
	t.Parallel()

	var adapter *Adapter

	_, err := adapter.invokeLoadDocument(context.Background(), operatorsettings.LoadDocumentRequest{})
	if err == nil {
		t.Fatal("invokeLoadDocument on nil adapter = nil, want error")
	}
}
