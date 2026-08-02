package settingsdocument_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	settingsconstruct "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/construct"
	settingsdocument "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/services/document"
	globalconfigmapping "github.com/portpowered/infinite-you/pkg/services/operator_settings/transports/globalconfig"
)

func TestConfigDocumentServiceRoutesLoadUpdatePersistThroughNestedDocumentOwner(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	service := persistedConfigService(platformfilesystem.Local{}, testCreateTemp)

	loaded, err := service.Load(path)
	if err != nil {
		t.Fatalf("Load() = %v", err)
	}
	if loaded.BackendScopeID() != "" || loaded.FileConfig().Defaults != (operatorsettings.Defaults{}) {
		t.Fatalf("missing load = %#v, want empty valid document", loaded.FileConfig())
	}

	provider, model := "codex", "routed-model"
	updated, err := service.ConfigureProviderModel(context.Background(), path, operatorsettings.ProviderModelUpdate{
		Provider: &provider,
		Model:    &model,
	})
	if err != nil {
		t.Fatalf("ConfigureProviderModel() = %v", err)
	}
	if got := updated.FileConfig().Defaults; got != (operatorsettings.Defaults{WorkerModelProvider: "CODEX", WorkerModel: "routed-model"}) {
		t.Fatalf("updated defaults = %#v", got)
	}

	reloaded, err := service.Load(path)
	if err != nil {
		t.Fatalf("Load() after persist = %v", err)
	}
	if reloaded.FileConfig().Defaults != updated.FileConfig().Defaults {
		t.Fatalf("reloaded defaults = %#v, want %#v", reloaded.FileConfig().Defaults, updated.FileConfig().Defaults)
	}

	withACP, err := service.ConfigureACPIntegrationAdd(context.Background(), path, operatorsettings.ACPIntegration{
		ID: "entry-1", Name: "cursor-acp", Transport: "stdio", Command: "cursor-agent acp",
	})
	if err != nil {
		t.Fatalf("ConfigureACPIntegrationAdd() = %v", err)
	}
	if got := withACP.FileConfig().Workers.ACP.Integrations; len(got) != 1 || got[0].Name != "cursor-acp" {
		t.Fatalf("ACP integrations after add = %#v", got)
	}
	withoutACP, err := service.ConfigureACPIntegrationDelete(context.Background(), path, "cursor-acp")
	if err != nil {
		t.Fatalf("ConfigureACPIntegrationDelete() = %v", err)
	}
	if got := withoutACP.FileConfig().Workers.ACP.Integrations; len(got) != 0 {
		t.Fatalf("ACP integrations after delete = %#v, want empty", got)
	}
}

func TestConfigDocumentServiceRoutesMalformedConflictAndUnsupportedThroughNestedOwner(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	service := operatorsettings.ConfigDocumentService{
		Files:      platformfilesystem.Local{},
		CreateTemp: testCreateTemp,
		Providers:  controlledProviderCatalog,
		Decoder:    globalconfigmapping.Decode,
		Encoder:    globalconfigmapping.Encode,
		DocumentOwner: settingsconstruct.NewDocumentOwner(
			platformfilesystem.Local{},
			testCreateTemp,
			globalconfigmapping.Decode,
			globalconfigmapping.Encode,
			controlledProviderCatalog,
		),
		PersistenceLock: &sync.Mutex{},
	}

	if writeErr := os.WriteFile(filepath.Join(dir, "malformed.json"), []byte(`{"defaults":`), 0o600); writeErr != nil {
		t.Fatalf("WriteFile() = %v", writeErr)
	}
	_, err := service.Load(filepath.Join(dir, "malformed.json"))
	if !errors.Is(err, operatorsettings.ErrDocumentMalformed) {
		t.Fatalf("Load(malformed) = %v, want ErrDocumentMalformed", err)
	}

	scope := "local-11111111-1111-4111-8111-111111111111"
	if writeErr := os.WriteFile(path, []byte(`{"backendScopeID":"`+scope+`","defaults":{"workerModelProvider":"CODEX"}}`), 0o600); writeErr != nil {
		t.Fatalf("WriteFile() = %v", writeErr)
	}
	unsupported := "unsupported-provider"
	_, err = service.ConfigureProviderModel(context.Background(), path, operatorsettings.ProviderModelUpdate{Provider: &unsupported})
	if !errors.Is(err, operatorsettings.ErrDocumentUnsupported) {
		t.Fatalf("ConfigureProviderModel(unsupported) = %v, want ErrDocumentUnsupported", err)
	}

	wrongScope := "local-22222222-2222-4222-8222-222222222222"
	owner, ok := service.DocumentOwner.(settingsdocument.Service)
	if !ok {
		t.Fatalf("DocumentOwner = %T, want private document service", service.DocumentOwner)
	}
	_, err = owner.ApplyDocumentUpdate(operatorsettings.ApplyDocumentUpdateRequest{
		Path:                 path,
		ExpectedBackendScope: wrongScope,
		ProviderModel:        operatorsettings.DocumentProviderModelUpdate{Model: strPtr("next")},
	})
	if !errors.Is(err, operatorsettings.ErrDocumentConflict) {
		t.Fatalf("ApplyDocumentUpdate(conflict) = %v, want ErrDocumentConflict", err)
	}
}

func persistedConfigService(
	files operatorsettings.FileSystem,
	create operatorsettings.CreateTemporaryFile,
) operatorsettings.ConfigDocumentService {
	return settingsconstruct.NewConfigDocumentService(
		files,
		create,
		globalconfigmapping.Decode,
		globalconfigmapping.Encode,
		controlledProviderCatalog,
		&sync.Mutex{},
	)
}

var testCreateTemp operatorsettings.CreateTemporaryFile = func(dir, pattern string) (operatorsettings.TemporaryFile, error) {
	return os.CreateTemp(dir, pattern)
}

func controlledProviderCatalog(value string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "codex", "openai":
		return "CODEX", true
	case "claude", "anthropic":
		return "CLAUDE", true
	case "gemini":
		return "GEMINI", true
	default:
		return "", false
	}
}

func strPtr(value string) *string {
	return &value
}
