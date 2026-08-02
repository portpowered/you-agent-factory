package wire_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	internaltestproviders "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/testproviders"
	globalconfigmapping "github.com/portpowered/infinite-you/pkg/services/operator_settings/transports/globalconfig"
	settingswire "github.com/portpowered/infinite-you/pkg/services/operator_settings/wire"
)

// TestNewServicePreservesDocumentPersistAndEffectiveResolution proves document
// load/update/atomic persist and effective resolution keep the same Settings-
// observable outcomes after the Settings→Providers consumer cut when the wire
// composes resolution against an injected providers.Service root.
func TestNewServicePreservesDocumentPersistAndEffectiveResolution(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	configPath := filepath.Join(homeDir, "config.json")
	if err := os.WriteFile(configPath, []byte(`{
		"defaults": {
			"workerModelProvider": "codex",
			"workerModel": "gpt-5"
		}
	}`), 0o600); err != nil {
		t.Fatalf("WriteFile(config) = %v", err)
	}

	root := newPreservationWireService(t)

	loaded, err := root.LoadDocument(operatorsettings.LoadDocumentRequest{Path: configPath})
	if err != nil {
		t.Fatalf("LoadDocument() = %v", err)
	}
	if loaded.Document.Defaults.WorkerModelProvider != "codex" ||
		loaded.Document.Defaults.WorkerModel != "gpt-5" {
		t.Fatalf("LoadDocument() defaults = %#v, want codex/gpt-5", loaded.Document.Defaults)
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
		t.Fatalf("ApplyDocumentUpdate() = %v", err)
	}
	if !updated.Persisted {
		t.Fatal("ApplyDocumentUpdate() Persisted = false, want true after atomic persist")
	}
	if updated.Document.Defaults.WorkerModelProvider != "GEMINI" ||
		updated.Document.Defaults.WorkerModel != "gemini-pro" {
		t.Fatalf("ApplyDocumentUpdate() defaults = %#v, want GEMINI/gemini-pro", updated.Document.Defaults)
	}

	reloaded, err := root.LoadDocument(operatorsettings.LoadDocumentRequest{Path: configPath})
	if err != nil {
		t.Fatalf("LoadDocument() after persist = %v", err)
	}
	if reloaded.Document.Defaults != updated.Document.Defaults {
		t.Fatalf("reloaded defaults = %#v, want %#v", reloaded.Document.Defaults, updated.Document.Defaults)
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
		t.Fatalf("ResolveEffective() = %v", err)
	}
	if resolved.Selection.WorkerModelProvider != "GEMINI" ||
		resolved.Selection.WorkerModel != "flag-model" ||
		resolved.Selection.ConfigPath != configPath {
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

func newPreservationWireService(t *testing.T) operatorsettings.Service {
	t.Helper()

	providersRoot := internaltestproviders.StandardCatalog()
	service, err := settingswire.NewService(
		platformfilesystem.Local{},
		func(dir, pattern string) (operatorsettings.TemporaryFile, error) {
			return os.CreateTemp(dir, pattern)
		},
		globalconfigmapping.Decode,
		globalconfigmapping.Encode,
		preservationProviderCatalog,
		providersRoot,
		testIDGenerator(),
		nil,
	)
	if err != nil {
		t.Fatalf("NewService() = %v", err)
	}
	if service == nil {
		t.Fatal("NewService() returned nil service")
	}
	return service
}

func preservationProviderCatalog(value string) (string, bool) {
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
