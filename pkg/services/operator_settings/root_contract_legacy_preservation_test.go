package operatorsettings

import (
	"context"
	"errors"
	"sync"
	"testing"
)

func TestConfigDocumentServiceRetainsCompletedOwnerPolicy(t *testing.T) {
	t.Parallel()
	persistError := errors.New("injected persistence failure")
	owner := boundDocumentOwner{
		document:     Document{Defaults: DocumentDefaults{WorkerModelProvider: "CODEX", WorkerModel: "bound-model"}},
		persistError: persistError,
	}
	service := ConfigDocumentService{
		DocumentOwner:   owner,
		PersistenceLock: &sync.Mutex{},
	}
	loaded, err := service.Load("config.json")
	if err != nil || loaded.FileConfig().Defaults.WorkerModel != "bound-model" {
		t.Fatalf("Load() = %#v, %v, want bound model", loaded, err)
	}
	merged, err := service.MergeProviderModelDefaults(loaded, ProviderModelUpdate{})
	if err != nil || merged.FileConfig().Defaults != loaded.FileConfig().Defaults {
		t.Fatalf("MergeProviderModelDefaults() = %#v, %v, want bound defaults", merged, err)
	}
	updated, err := service.ConfigureProviderModel(context.Background(), "config.json", ProviderModelUpdate{})
	if err != nil || updated.FileConfig().Defaults != loaded.FileConfig().Defaults {
		t.Fatalf("ConfigureProviderModel() = %#v, %v, want bound defaults", updated, err)
	}
	if err := service.Persist(context.Background(), "config.json", loaded); !errors.Is(err, persistError) {
		t.Fatalf("Persist() = %v, want completed owner's persistence failure", err)
	}
}

func TestConfigDocumentServicePreservesCompletedOwnerFailures(t *testing.T) {
	t.Parallel()
	failure := DocumentFailure{Kind: DocumentFailureKindMalformed, Message: "controlled malformed document"}
	service := ConfigDocumentService{
		DocumentOwner:   boundDocumentOwner{operationError: failure},
		PersistenceLock: &sync.Mutex{},
	}
	for _, test := range []struct {
		name   string
		invoke func() (ConfigDocument, error)
	}{
		{name: "load", invoke: func() (ConfigDocument, error) { return service.Load("config.json") }},
		{name: "merge", invoke: func() (ConfigDocument, error) {
			return service.MergeProviderModelDefaults(ConfigDocument{}, ProviderModelUpdate{})
		}},
		{name: "update", invoke: func() (ConfigDocument, error) {
			return service.ConfigureProviderModel(context.Background(), "config.json", ProviderModelUpdate{})
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			document, err := test.invoke()
			if !errors.Is(err, ErrDocumentMalformed) || document.BackendScopeID() != "" || document.FileConfig().Defaults != (Defaults{}) {
				t.Fatalf("operation = %#v, %v, want empty result and malformed failure", document.FileConfig(), err)
			}
			var got DocumentFailure
			if !errors.As(err, &got) || got.Message != failure.Message {
				t.Fatalf("operation failure = %v, want unchanged typed owner failure", err)
			}
		})
	}
}

// The legacy hook deliberately supplies a different policy. The compatibility
// adapter must preserve the completed owner's results and errors instead.
type boundDocumentOwner struct {
	document       Document
	persistError   error
	operationError error
}

func (owner boundDocumentOwner) RebindDocumentOwner(
	FileSystem, CreateTemporaryFile, ConfigDecoder, ConfigEncoder, ProviderCatalog, ...ConfigDiagnosticsDecoder,
) DocumentOwner {
	return boundDocumentOwner{document: Document{Defaults: DocumentDefaults{WorkerModel: "rebound-model"}}}
}

func (owner boundDocumentOwner) RebindDocumentOwnerWithPreserver(
	FileSystem, CreateTemporaryFile, ConfigDecoder, ConfigEncoder, ProviderCatalog, ConfigDocumentPreserver, ...ConfigDiagnosticsDecoder,
) DocumentOwner {
	return boundDocumentOwner{document: Document{Defaults: DocumentDefaults{WorkerModel: "rebound-preserved-model"}}}
}

func (owner boundDocumentOwner) LoadDocument(LoadDocumentRequest) (LoadDocumentResult, error) {
	if owner.operationError != nil {
		return LoadDocumentResult{}, owner.operationError
	}
	return LoadDocumentResult{Document: owner.document}, nil
}

func (owner boundDocumentOwner) MergeDocumentProviderModel(Document, DocumentProviderModelUpdate) (Document, error) {
	if owner.operationError != nil {
		return Document{}, owner.operationError
	}
	return owner.document, nil
}

func (owner boundDocumentOwner) ApplyDocumentUpdate(ApplyDocumentUpdateRequest) (ApplyDocumentUpdateResult, error) {
	if owner.operationError != nil {
		return ApplyDocumentUpdateResult{}, owner.operationError
	}
	return ApplyDocumentUpdateResult{Document: owner.document}, nil
}

func (owner boundDocumentOwner) PersistDocument(context.Context, PersistDocumentRequest) error {
	return owner.persistError
}
