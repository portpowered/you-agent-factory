package wire_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	internaltestproviders "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/testproviders"
	settingswire "github.com/portpowered/infinite-you/pkg/services/operator_settings/wire"
	globalconfigmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/globalconfig"
)

const identityInventoryFixturesRelativeDir = "pkg/services/operator_settings/internal/services/document/identityinventory/testdata/fixtures"

// TestWireLegacyPackagesFoldPreservesExistingBackendScopeIdentity seals
// pss-cln-set-legacy-packages-004: document-backed backend scope identity from
// the relocated identityinventory fixtures still loads and enforces optimistic
// concurrency through operator_settings/wire on the published Service root.
func TestWireLegacyPackagesFoldPreservesExistingBackendScopeIdentity(t *testing.T) {
	t.Parallel()

	const scopeID = "local-aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee"
	configPath := writeIdentityInventoryFixtureToTemp(t, "valid/existing-scope.json")

	root, err := settingswire.NewServiceFromConfigDocument(
		testConfigDocumentService(),
		internaltestproviders.StandardCatalog(),
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
		t.Fatalf("ApplyDocumentUpdate() error = %v", err)
	}
	if !updated.Persisted || updated.Document.BackendScopeID != scopeID {
		t.Fatalf("ApplyDocumentUpdate() = %#v, want persisted document with scope %q", updated, scopeID)
	}
	if updated.Document.Defaults.WorkerModel != model {
		t.Fatalf("updated model = %q, want %q", updated.Document.Defaults.WorkerModel, model)
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
}

// TestWireLegacyPackagesFoldPreservesRootBehaviorWithRelocatedTestHelpers seals
// pss-cln-set-legacy-packages-004: wire construction through internal/testproviders
// after the testlink/testproviders fold still serves document and resolve-effective
// outcomes on the published root.
func TestWireLegacyPackagesFoldPreservesRootBehaviorWithRelocatedTestHelpers(t *testing.T) {
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
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	var root operatorsettings.Service = service

	loaded, err := root.LoadDocument(operatorsettings.LoadDocumentRequest{Path: configPath})
	if err != nil {
		t.Fatalf("LoadDocument() error = %v", err)
	}
	if loaded.Document.Defaults.WorkerModelProvider != "codex" {
		t.Fatalf("loaded provider = %q, want codex", loaded.Document.Defaults.WorkerModelProvider)
	}

	resolved, err := root.ResolveEffective(operatorsettings.ResolveEffectiveRequest{
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
		resolved.Selection.WorkerModel != "flag-model" {
		t.Fatalf("ResolveEffective() = %#v", resolved.Selection)
	}
}

func writeIdentityInventoryFixtureToTemp(t *testing.T, rel string) string {
	t.Helper()

	fixturePath := testutil.MustRepoPath(t, filepath.ToSlash(filepath.Join(identityInventoryFixturesRelativeDir, rel)))
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
