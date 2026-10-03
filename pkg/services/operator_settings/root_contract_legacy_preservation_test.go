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
		DocumentOwner:         owner,
		Files:                 bindingFileSystem{},
		CreateTemp:            func(string, string) (TemporaryFile, error) { return nil, persistError },
		PersistenceLock:       &sync.Mutex{},
		PreserveUnknownFields: func(_, encoded []byte) ([]byte, error) { return encoded, nil },
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

// The legacy hook deliberately supplies a different policy. The compatibility
// adapter must preserve the completed owner's results and errors instead.
type boundDocumentOwner struct {
	document     Document
	persistError error
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
	return LoadDocumentResult{Document: owner.document}, nil
}

func (owner boundDocumentOwner) MergeDocumentProviderModel(Document, DocumentProviderModelUpdate) (Document, error) {
	return owner.document, nil
}

func (owner boundDocumentOwner) ApplyDocumentUpdate(ApplyDocumentUpdateRequest) (ApplyDocumentUpdateResult, error) {
	return ApplyDocumentUpdateResult{Document: owner.document}, nil
}

func (owner boundDocumentOwner) PersistDocument(context.Context, PersistDocumentRequest) error {
	return owner.persistError
}

// No filesystem operation is needed for an adapter with a completed owner.
type bindingFileSystem struct{ FileSystem }
