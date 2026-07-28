package root_composition_test

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	settingswire "github.com/portpowered/infinite-you/pkg/services/operator_settings/wire"
	globalconfigmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/globalconfig"
)

// TestWireCompositionServesDocumentAndResolutionOperations exercises published
// Settings wire surfaces through the functional lane so transitional composition
// hooks (construct/testlink/testproviders) retain coverage after servicewire/
// retargeting.
func TestWireCompositionServesDocumentAndResolutionOperations(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	configPath := operatorsettings.DefaultConfigPath(homeDir)
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(config dir): %v", err)
	}
	if err := os.WriteFile(configPath, []byte(`{
		"defaults": {
			"workerModelProvider": "openai",
			"workerModel": "gpt-5"
		}
	}`), 0o600); err != nil {
		t.Fatalf("WriteFile(config): %v", err)
	}

	root, err := settingswire.NewServiceFromConfigDocument(
		settingswire.NewConfigDocumentService(
			platformfilesystem.Local{},
			func(dir, pattern string) (operatorsettings.TemporaryFile, error) {
				return os.CreateTemp(dir, pattern)
			},
			globalconfigmapping.Decode,
			globalconfigmapping.Encode,
			wireCompositionProviderCatalog,
			&sync.Mutex{},
		),
	)
	if err != nil {
		t.Fatalf("NewServiceFromConfigDocument() error = %v", err)
	}

	loaded, err := root.LoadDocument(operatorsettings.LoadDocumentRequest{Path: configPath})
	if err != nil {
		t.Fatalf("LoadDocument() error = %v", err)
	}
	if loaded.Document.Defaults.WorkerModelProvider != "openai" {
		t.Fatalf("loaded provider = %q, want openai from file", loaded.Document.Defaults.WorkerModelProvider)
	}

	resolved, err := root.ResolveEffective(operatorsettings.ResolveEffectiveRequest{
		DocumentBaseline: loaded.Document.Defaults,
		InvocationOverrides: operatorsettings.EffectiveOverrideFacts{
			WorkerModelProvider: "claude",
			WorkerModel:         "claude-sonnet",
		},
		ConfigPath: configPath,
	})
	if err != nil {
		t.Fatalf("ResolveEffective() error = %v", err)
	}
	if resolved.Selection.WorkerModelProvider != "CLAUDE" || resolved.Selection.WorkerModel != "claude-sonnet" {
		t.Fatalf("ResolveEffective() = %#v", resolved.Selection)
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
}

func TestWireCompositionFromHomePortsConstructsSettingsRoot(t *testing.T) {
	t.Parallel()

	root, err := settingswire.NewServiceFromHomePorts(platformfilesystem.Local{}, globalconfigmapping.Decode)
	if err != nil {
		t.Fatalf("NewServiceFromHomePorts() error = %v", err)
	}
	if root == nil {
		t.Fatal("NewServiceFromHomePorts() = nil, want Settings root")
	}
}

func TestWireCompositionFromHomePortsRejectsMissingPorts(t *testing.T) {
	t.Parallel()

	_, err := settingswire.NewServiceFromHomePorts(nil, globalconfigmapping.Decode)
	if err == nil || !strings.Contains(err.Error(), "filesystem is required") {
		t.Fatalf("NewServiceFromHomePorts(nil, decode) error = %v, want filesystem required", err)
	}

	_, err = settingswire.NewServiceFromHomePorts(platformfilesystem.Local{}, nil)
	if err == nil || !strings.Contains(err.Error(), "decoder is required") {
		t.Fatalf("NewServiceFromHomePorts(files, nil) error = %v, want decoder required", err)
	}
}

func TestWireCompositionRegisterDefaultsResolutionFromHomeRestoresAdapterOwnership(t *testing.T) {
	t.Parallel()

	operatorsettings.ConfigureDefaultsResolutionFromHome(nil)
	settingswire.RegisterDefaultsResolutionFromHome()
}

func wireCompositionProviderCatalog(value string) (string, bool) {
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
