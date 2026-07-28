package operatorsettings

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestConfigDocumentServiceRoutesLoadUpdatePersistThroughNestedDocumentOwner(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	service := persistedConfigService(testFiles, testCreateTemp)

	loaded, err := service.Load(path)
	if err != nil {
		t.Fatalf("Load() = %v", err)
	}
	if loaded.BackendScopeID() != "" || loaded.FileConfig().Defaults != (Defaults{}) {
		t.Fatalf("missing load = %#v, want empty valid document", loaded.FileConfig())
	}

	provider, model := "codex", "routed-model"
	updated, err := service.ConfigureProviderModel(context.Background(), path, ProviderModelUpdate{
		Provider: &provider,
		Model:    &model,
	})
	if err != nil {
		t.Fatalf("ConfigureProviderModel() = %v", err)
	}
	if got := updated.FileConfig().Defaults; got != (Defaults{WorkerModelProvider: "CODEX", WorkerModel: "routed-model"}) {
		t.Fatalf("updated defaults = %#v", got)
	}

	reloaded, err := service.Load(path)
	if err != nil {
		t.Fatalf("Load() after persist = %v", err)
	}
	if reloaded.FileConfig().Defaults != updated.FileConfig().Defaults {
		t.Fatalf("reloaded defaults = %#v, want %#v", reloaded.FileConfig().Defaults, updated.FileConfig().Defaults)
	}
}

func TestConfigDocumentServiceRoutesMalformedConflictAndUnsupportedThroughNestedOwner(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	service := ConfigDocumentService{
		Files:           testFiles,
		CreateTemp:      testCreateTemp,
		Providers:       controlledProviderCatalog,
		Decoder:         decodeTestConfig,
		Encoder:         encodeTestConfig,
		PersistenceLock: &sync.Mutex{},
	}

	if writeErr := os.WriteFile(filepath.Join(dir, "malformed.json"), []byte(`{"defaults":`), 0o600); writeErr != nil {
		t.Fatalf("WriteFile() = %v", writeErr)
	}
	_, err := service.Load(filepath.Join(dir, "malformed.json"))
	if !errors.Is(err, ErrDocumentMalformed) {
		t.Fatalf("Load(malformed) = %v, want ErrDocumentMalformed", err)
	}

	scope := "local-11111111-1111-4111-8111-111111111111"
	if writeErr := os.WriteFile(path, []byte(`{"backendScopeID":"`+scope+`","defaults":{"workerModelProvider":"CODEX"}}`), 0o600); writeErr != nil {
		t.Fatalf("WriteFile() = %v", writeErr)
	}
	unsupported := "unsupported-provider"
	_, err = service.ConfigureProviderModel(context.Background(), path, ProviderModelUpdate{Provider: &unsupported})
	if !errors.Is(err, ErrDocumentUnsupported) {
		t.Fatalf("ConfigureProviderModel(unsupported) = %v, want ErrDocumentUnsupported", err)
	}

	wrongScope := "local-22222222-2222-4222-8222-222222222222"
	owner, ownerErr := service.resolvedDocumentOwner()
	if ownerErr != nil {
		t.Fatalf("resolvedDocumentOwner() = %v", ownerErr)
	}
	_, err = owner.ApplyDocumentUpdate(ApplyDocumentUpdateRequest{
		Path:                 path,
		ExpectedBackendScope: wrongScope,
		ProviderModel:        DocumentProviderModelUpdate{Model: strPtr("next")},
	})
	if !errors.Is(err, ErrDocumentConflict) {
		t.Fatalf("ApplyDocumentUpdate(conflict) = %v, want ErrDocumentConflict", err)
	}
}

func strPtr(value string) *string {
	return &value
}
