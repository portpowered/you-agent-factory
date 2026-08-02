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
	settingswire "github.com/portpowered/infinite-you/pkg/services/operator_settings/wire"
	globalconfigmapping "github.com/portpowered/infinite-you/pkg/services/operator_settings/transports/globalconfig"
)

// Fold-preservation proofs for pss-cln-set-legacy-packages-004 (and the earlier
// pss-cln-set-fold-servicewire-005 baseline). Every case constructs Operator
// Settings exclusively through operator_settings/wire and exercises observable
// document, identity, resolution, and config outcomes on the published
// operatorsettings.Service root.

func TestWireFoldPreservesDocumentIdentityResolutionAndConfigBehavior(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	configPath := operatorsettings.DefaultConfigPath(homeDir)
	const backendScopeID = "local-00000000-0000-4000-8000-000000000010"
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(config dir): %v", err)
	}
	if err := os.WriteFile(configPath, []byte(`{
		"backendScopeID": "`+backendScopeID+`",
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
	var service operatorsettings.Service = root

	loaded, err := service.LoadDocument(operatorsettings.LoadDocumentRequest{
		Path:            configPath,
		RequireExisting: true,
	})
	if err != nil {
		t.Fatalf("LoadDocument() error = %v", err)
	}
	if !loaded.Found || loaded.Document.BackendScopeID != backendScopeID {
		t.Fatalf("LoadDocument() = %#v, want found document with backend scope", loaded)
	}
	if loaded.Document.Defaults.WorkerModelProvider != "codex" {
		t.Fatalf("loaded provider = %q, want codex", loaded.Document.Defaults.WorkerModelProvider)
	}

	provider := "gemini"
	model := "gemini-pro"
	updated, err := service.ApplyDocumentUpdate(operatorsettings.ApplyDocumentUpdateRequest{
		Path:                 configPath,
		ExpectedBackendScope: backendScopeID,
		ProviderModel: operatorsettings.DocumentProviderModelUpdate{
			Provider: &provider,
			Model:    &model,
		},
	})
	if err != nil {
		t.Fatalf("ApplyDocumentUpdate() error = %v", err)
	}
	if !updated.Persisted ||
		updated.Document.Defaults.WorkerModelProvider != "GEMINI" ||
		updated.Document.Defaults.WorkerModel != "gemini-pro" {
		t.Fatalf("ApplyDocumentUpdate() = %#v, want persisted GEMINI/gemini-pro", updated)
	}

	unsupported := "unsupported-provider"
	_, err = service.ApplyDocumentUpdate(operatorsettings.ApplyDocumentUpdateRequest{
		Path: configPath,
		ProviderModel: operatorsettings.DocumentProviderModelUpdate{
			Provider: &unsupported,
		},
	})
	if !errors.Is(err, operatorsettings.ErrDocumentUnsupported) {
		t.Fatalf("unsupported provider error = %v, want ErrDocumentUnsupported", err)
	}

	resolved, err := service.ResolveEffective(operatorsettings.ResolveEffectiveRequest{
		DocumentBaseline: loaded.Document.Defaults,
		InvocationOverrides: operatorsettings.EffectiveOverrideFacts{
			WorkerModelProvider: "gemini",
			WorkerModel:         "flag-model",
		},
		ConfigPath: configPath,
	})
	if err != nil {
		t.Fatalf("ResolveEffective() error = %v", err)
	}
	if resolved.Selection.WorkerModelProvider != "GEMINI" ||
		resolved.Selection.WorkerModel != "flag-model" ||
		resolved.Selection.ConfigPath != configPath {
		t.Fatalf("ResolveEffective() = %#v", resolved.Selection)
	}

	_, err = service.ResolveEffective(operatorsettings.ResolveEffectiveRequest{
		InvocationOverrides: operatorsettings.EffectiveOverrideFacts{
			WorkerModelProvider: "unsupported-provider",
		},
		ConfigPath: configPath,
	})
	if !errors.Is(err, operatorsettings.ErrResolutionUnsupportedOverride) {
		t.Fatalf("unsupported override error = %v, want ErrResolutionUnsupportedOverride", err)
	}
}

func TestWireFoldPreservesDefaultsResolutionFromHomeOwnershipPath(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	configPath := operatorsettings.DefaultConfigPath(homeDir)
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(config dir): %v", err)
	}
	if err := os.WriteFile(configPath, []byte(`{
		"defaults": {
			"workerModelProvider": "openai",
			"workerModel": "file-model"
		}
	}`), 0o600); err != nil {
		t.Fatalf("WriteFile(config): %v", err)
	}

	root, err := settingswire.NewServiceFromHomePorts(
		platformfilesystem.Local{},
		globalconfigmapping.Decode,
		internaltestproviders.StandardCatalog(),
		testIDGenerator(),
	)
	if err != nil {
		t.Fatalf("NewServiceFromHomePorts() error = %v", err)
	}
	resolved, err := root.ResolveFromHomeWithEnvironment(
		homeDir,
		operatorsettings.Defaults{},
		operatorsettings.FlagOverrides{},
	)
	if err != nil {
		t.Fatalf("ResolveFromHomeWithEnvironment() error = %v", err)
	}
	if resolved.WorkerModelProvider != "CODEX" {
		t.Fatalf("provider = %q, want CODEX alias canonicalization through wire ownership", resolved.WorkerModelProvider)
	}
	if resolved.WorkerModel != "file-model" {
		t.Fatalf("model = %q, want file-model", resolved.WorkerModel)
	}
	if resolved.ConfigPath != configPath {
		t.Fatalf("config path = %q, want %q", resolved.ConfigPath, configPath)
	}
}

func TestWireFoldDefaultsResolutionFromHomeRejectsMissingFilesystemPorts(t *testing.T) {
	t.Parallel()

	_, err := settingswire.NewServiceFromHomePorts(
		nil,
		globalconfigmapping.Decode,
		internaltestproviders.StandardCatalog(),
		testIDGenerator(),
	)
	if err == nil || !strings.Contains(err.Error(), "filesystem is required") {
		t.Fatalf("NewServiceFromHomePorts() error = %v, want home-port construction failure", err)
	}
}
