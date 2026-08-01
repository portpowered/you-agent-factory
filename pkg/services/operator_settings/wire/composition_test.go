package wire_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	internaltestproviders "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/testproviders"
	settingswire "github.com/portpowered/infinite-you/pkg/services/operator_settings/wire"
	globalconfigmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/globalconfig"
)

func TestNewServiceFromConfigDocumentRequiresDocumentPorts(t *testing.T) {
	t.Parallel()

	_, err := settingswire.NewServiceFromConfigDocument(
		operatorsettings.ConfigDocumentService{},
		internaltestproviders.StandardCatalog(),
		testIDGenerator(),
	)
	if err == nil || !strings.Contains(err.Error(), "operator settings document ports are required") {
		t.Fatalf("NewServiceFromConfigDocument() error = %v, want document ports required", err)
	}
}

func TestNewServiceFromConfigDocumentConstructsFromPorts(t *testing.T) {
	t.Parallel()

	root, err := settingswire.NewServiceFromConfigDocument(
		testConfigDocumentService(),
		internaltestproviders.StandardCatalog(),
		testIDGenerator(),
	)
	if err != nil {
		t.Fatalf("NewServiceFromConfigDocument() error = %v", err)
	}
	if root == nil {
		t.Fatal("NewServiceFromConfigDocument() = nil, want Settings root")
	}
}

func TestNewServiceFromConfigDocumentUsesInjectedDocumentOwner(t *testing.T) {
	t.Parallel()

	service := testConfigDocumentService()
	service.DocumentOwner = settingswire.NewDocumentOwner(
		service.Files,
		service.CreateTemp,
		service.Decoder,
		service.Encoder,
		service.Providers,
	)

	root, err := settingswire.NewServiceFromConfigDocument(
		service,
		internaltestproviders.StandardCatalog(),
		testIDGenerator(),
	)
	if err != nil {
		t.Fatalf("NewServiceFromConfigDocument() error = %v", err)
	}
	if root == nil {
		t.Fatal("NewServiceFromConfigDocument() = nil, want Settings root")
	}
}

func TestNewConfigDocumentServiceConstructsDocumentOwner(t *testing.T) {
	t.Parallel()

	service := settingswire.NewConfigDocumentService(
		platformfilesystem.Local{},
		func(dir, pattern string) (operatorsettings.TemporaryFile, error) {
			return os.CreateTemp(dir, pattern)
		},
		globalconfigmapping.Decode,
		globalconfigmapping.Encode,
		testProviderCatalog,
		&sync.Mutex{},
	)
	if service.Files == nil || service.Decoder == nil || service.DocumentOwner == nil {
		t.Fatalf("NewConfigDocumentService() = %#v, want populated document ports", service)
	}
}

func TestWireCompositionDelegatesDocumentAndResolutionOperations(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	configPath := operatorsettings.DefaultConfigPath(homeDir)
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(config dir): %v", err)
	}
	if err := os.WriteFile(configPath, []byte(`{
		"defaults": {
			"workerModelProvider": "codex",
			"workerModel": "gpt-5"
		}
	}`), 0o600); err != nil {
		t.Fatalf("WriteFile(config): %v", err)
	}

	root, err := settingswire.NewServiceFromConfigDocument(
		testConfigDocumentService(),
		internaltestproviders.StandardCatalog(),
		testIDGenerator(),
	)
	if err != nil {
		t.Fatalf("NewServiceFromConfigDocument() error = %v", err)
	}

	loaded, err := root.LoadDocument(operatorsettings.LoadDocumentRequest{Path: configPath})
	if err != nil {
		t.Fatalf("LoadDocument() error = %v", err)
	}
	if loaded.Document.Defaults.WorkerModelProvider != "codex" {
		t.Fatalf("loaded provider = %q, want codex", loaded.Document.Defaults.WorkerModelProvider)
	}

	provider := "gemini"
	model := "gemini-pro"
	updated, err := root.ApplyDocumentUpdate(operatorsettings.ApplyDocumentUpdateRequest{
		Path: configPath,
		ProviderModel: operatorsettings.DocumentProviderModelUpdate{
			Provider: &provider,
			Model:    &model,
		},
	})
	if err != nil {
		t.Fatalf("ApplyDocumentUpdate() error = %v", err)
	}
	if updated.Document.Defaults.WorkerModelProvider != "GEMINI" || updated.Document.Defaults.WorkerModel != "gemini-pro" {
		t.Fatalf("updated defaults = %#v, want GEMINI/gemini-pro", updated.Document.Defaults)
	}

	resolved, err := root.ResolveEffective(operatorsettings.ResolveEffectiveRequest{
		DocumentBaseline: operatorsettings.DocumentDefaults{
			WorkerModelProvider: "CODEX",
			WorkerModel:         "gpt-5",
		},
		InvocationOverrides: operatorsettings.EffectiveOverrideFacts{
			WorkerModelProvider: "gemini",
			WorkerModel:         "flag-model",
		},
		ConfigPath: configPath,
	})
	if err != nil {
		t.Fatalf("ResolveEffective() error = %v", err)
	}
	if resolved.Selection.WorkerModelProvider != "GEMINI" || resolved.Selection.WorkerModel != "flag-model" {
		t.Fatalf("ResolveEffective() = %#v", resolved.Selection)
	}

	_, err = root.ResolveEffective(operatorsettings.ResolveEffectiveRequest{
		InvocationOverrides: operatorsettings.EffectiveOverrideFacts{
			WorkerModelProvider: "unsupported-provider",
		},
		ConfigPath: configPath,
	})
	if !errors.Is(err, operatorsettings.ErrResolutionUnsupportedOverride) {
		t.Fatalf("unsupported override error = %v, want ErrResolutionUnsupportedOverride", err)
	}
}

func testConfigDocumentService() operatorsettings.ConfigDocumentService {
	return settingswire.NewConfigDocumentService(
		platformfilesystem.Local{},
		func(dir, pattern string) (operatorsettings.TemporaryFile, error) {
			return os.CreateTemp(dir, pattern)
		},
		globalconfigmapping.Decode,
		globalconfigmapping.Encode,
		testProviderCatalog,
		&sync.Mutex{},
	)
}

func testProviderCatalog(value string) (string, bool) {
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
