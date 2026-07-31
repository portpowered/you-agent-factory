package operatorsettings_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	internaltestlink "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/testlink"
	internaltestproviders "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/testproviders"
	settingswire "github.com/portpowered/infinite-you/pkg/services/operator_settings/wire"
	globalconfigmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/globalconfig"
)

const rootWireIdentityFixtureRelative = "pkg/services/operator_settings/internal/services/document/identityinventory/testdata/fixtures/valid/existing-scope.json"

// TestRootWireBehavioralBoundary_PublishedServicePreservesObservables constructs
// Operator Settings exclusively through operator_settings/wire and proves
// LoadDocument, ApplyDocumentUpdate, and ResolveEffective preserve observable
// outcomes and typed failures on the published operatorsettings.Service peer
// surface after CLN-SET-CONTRACT-ROOTS seals the thin root.
func TestRootWireBehavioralBoundary_PublishedServicePreservesObservables(t *testing.T) {
	internaltestlink.RegisterComposition()

	t.Run("document persist and effective resolution success", func(t *testing.T) {
		t.Parallel()

		homeDir := t.TempDir()
		configPath := filepath.Join(homeDir, "config.json")
		if err := os.WriteFile(configPath, []byte(`{
			"defaults": {
				"workerModelProvider": "codex",
				"workerModel": "gpt-5"
			}
		}`), 0o600); err != nil {
			t.Fatalf("WriteFile(config): %v", err)
		}

		root := newRootWireBehavioralService(t)

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
	})

	t.Run("backend scope identity and conflict", func(t *testing.T) {
		t.Parallel()

		const scopeID = "local-aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee"
		configPath := writeRootWireIdentityFixtureToTemp(t)

		root, err := settingswire.NewServiceFromConfigDocument(rootWireConfigDocumentService())
		if err != nil {
			t.Fatalf("NewServiceFromConfigDocument() = %v", err)
		}
		var service operatorsettings.Service = root

		loaded, err := service.LoadDocument(operatorsettings.LoadDocumentRequest{
			Path:            configPath,
			RequireExisting: true,
		})
		if err != nil {
			t.Fatalf("LoadDocument() = %v", err)
		}
		if !loaded.Found || loaded.Document.BackendScopeID != scopeID {
			t.Fatalf("LoadDocument() = %#v, want found document with scope %q", loaded, scopeID)
		}

		model := "gpt-5.2"
		updated, err := service.ApplyDocumentUpdate(operatorsettings.ApplyDocumentUpdateRequest{
			Path:                 configPath,
			ExpectedBackendScope: scopeID,
			ProviderModel: operatorsettings.DocumentProviderModelUpdate{
				Model: &model,
			},
		})
		if err != nil {
			t.Fatalf("ApplyDocumentUpdate() = %v", err)
		}
		if !updated.Persisted || updated.Document.BackendScopeID != scopeID {
			t.Fatalf("ApplyDocumentUpdate() = %#v, want persisted document with scope %q", updated, scopeID)
		}

		_, err = service.ApplyDocumentUpdate(operatorsettings.ApplyDocumentUpdateRequest{
			Path:                 configPath,
			ExpectedBackendScope: "local-stale-scope",
			ProviderModel: operatorsettings.DocumentProviderModelUpdate{
				Model: &model,
			},
		})
		if !errors.Is(err, operatorsettings.ErrDocumentConflict) {
			t.Fatalf("stale scope error = %v, want ErrDocumentConflict", err)
		}
	})

	t.Run("typed failures", func(t *testing.T) {
		t.Parallel()

		homeDir := t.TempDir()
		configPath := filepath.Join(homeDir, "config.json")
		if err := os.WriteFile(configPath, []byte(`{
			"defaults": {
				"workerModelProvider": "codex",
				"workerModel": "gpt-5"
			}
		}`), 0o600); err != nil {
			t.Fatalf("WriteFile(config): %v", err)
		}

		root := newRootWireBehavioralService(t)

		_, err := root.LoadDocument(operatorsettings.LoadDocumentRequest{})
		if !errors.Is(err, operatorsettings.ErrDocumentMalformed) {
			t.Fatalf("empty load path error = %v, want ErrDocumentMalformed", err)
		}

		_, err = root.LoadDocument(operatorsettings.LoadDocumentRequest{
			Path:            filepath.Join(homeDir, "missing.json"),
			RequireExisting: true,
		})
		if !errors.Is(err, operatorsettings.ErrDocumentNotFound) {
			t.Fatalf("missing required document error = %v, want ErrDocumentNotFound", err)
		}

		unsupported := "unsupported-provider"
		_, err = root.ApplyDocumentUpdate(operatorsettings.ApplyDocumentUpdateRequest{
			Path: configPath,
			ProviderModel: operatorsettings.DocumentProviderModelUpdate{
				Provider: &unsupported,
			},
		})
		if !errors.Is(err, operatorsettings.ErrDocumentUnsupported) {
			t.Fatalf("unsupported provider error = %v, want ErrDocumentUnsupported", err)
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

		_, err = root.ResolveEffective(operatorsettings.ResolveEffectiveRequest{
			InvocationOverrides: operatorsettings.EffectiveOverrideFacts{
				WorkerModelProvider: "DEFAULT",
			},
			ConfigPath: configPath,
		})
		if !errors.Is(err, operatorsettings.ErrResolutionInvalidInput) {
			t.Fatalf("unresolved DEFAULT error = %v, want ErrResolutionInvalidInput", err)
		}

		expected := operatorsettings.DocumentDefaults{
			WorkerModelProvider: "codex",
			WorkerModel:         "gpt-5",
		}
		stale := operatorsettings.DocumentDefaults{
			WorkerModelProvider: "claude",
			WorkerModel:         "gpt-5",
		}
		_, err = root.ResolveEffective(operatorsettings.ResolveEffectiveRequest{
			DocumentBaseline:         stale,
			ExpectedDocumentBaseline: &expected,
			ConfigPath:               configPath,
		})
		if !errors.Is(err, operatorsettings.ErrResolutionConflict) {
			t.Fatalf("baseline conflict error = %v, want ErrResolutionConflict", err)
		}
	})
}

func newRootWireBehavioralService(t *testing.T) operatorsettings.Service {
	t.Helper()

	providersRoot := internaltestproviders.StandardCatalog()
	service, err := settingswire.NewService(
		platformfilesystem.Local{},
		func(dir, pattern string) (operatorsettings.TemporaryFile, error) {
			return os.CreateTemp(dir, pattern)
		},
		globalconfigmapping.Decode,
		globalconfigmapping.Encode,
		rootWireProviderCatalog,
		providersRoot,
	)
	if err != nil {
		t.Fatalf("NewService() = %v", err)
	}
	if service == nil {
		t.Fatal("NewService() returned nil service")
	}
	return service
}

func rootWireProviderCatalog(value string) (string, bool) {
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

func rootWireConfigDocumentService() operatorsettings.ConfigDocumentService {
	return settingswire.NewConfigDocumentService(
		platformfilesystem.Local{},
		func(dir, pattern string) (operatorsettings.TemporaryFile, error) {
			return os.CreateTemp(dir, pattern)
		},
		globalconfigmapping.Decode,
		globalconfigmapping.Encode,
		rootWireProviderCatalog,
		&sync.Mutex{},
	)
}

func writeRootWireIdentityFixtureToTemp(t *testing.T) string {
	t.Helper()

	fixturePath := testutil.MustRepoPath(t, rootWireIdentityFixtureRelative)
	data, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read fixture %s: %v", fixturePath, err)
	}

	configPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(configPath, data, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return configPath
}
