package servicewire

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

func TestServiceWireCompositionRootServesDocumentAndResolutionOperations(t *testing.T) {
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
			providerCatalog,
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

func TestServiceFromHomePortsConstructsSettingsRoot(t *testing.T) {
	root, err := settingswire.NewServiceFromHomePorts(platformfilesystem.Local{}, globalconfigmapping.Decode)
	if err != nil {
		t.Fatalf("NewServiceFromHomePorts() error = %v", err)
	}
	if root == nil {
		t.Fatal("NewServiceFromHomePorts() = nil, want Settings root")
	}
}

func TestServiceFromHomePortsRejectsMissingPorts(t *testing.T) {
	_, err := settingswire.NewServiceFromHomePorts(nil, globalconfigmapping.Decode)
	if err == nil || !strings.Contains(err.Error(), "filesystem is required") {
		t.Fatalf("NewServiceFromHomePorts(nil, decode) error = %v, want filesystem required", err)
	}

	_, err = settingswire.NewServiceFromHomePorts(platformfilesystem.Local{}, nil)
	if err == nil || !strings.Contains(err.Error(), "decoder is required") {
		t.Fatalf("NewServiceFromHomePorts(files, nil) error = %v, want decoder required", err)
	}
}

func TestServiceFromConfigDocumentConstructsFromDocumentPorts(t *testing.T) {
	root, err := settingswire.NewServiceFromConfigDocument(operatorsettings.ConfigDocumentService{
		Files:     platformfilesystem.Local{},
		Decoder:   globalconfigmapping.Decode,
		Encoder:   globalconfigmapping.Encode,
		Providers: providerCatalog,
		CreateTemp: func(dir, pattern string) (operatorsettings.TemporaryFile, error) {
			return os.CreateTemp(dir, pattern)
		},
	})
	if err != nil {
		t.Fatalf("NewServiceFromConfigDocument() error = %v", err)
	}
	if root == nil {
		t.Fatal("NewServiceFromConfigDocument() = nil, want Settings root")
	}
}

func TestServiceFromConfigDocumentRejectsMissingDocumentPorts(t *testing.T) {
	_, err := settingswire.NewServiceFromConfigDocument(operatorsettings.ConfigDocumentService{})
	if err == nil || !strings.Contains(err.Error(), "operator settings document ports are required") {
		t.Fatalf("NewServiceFromConfigDocument() error = %v, want document ports required", err)
	}
}

func TestResolveFromHomeRejectsMissingFilesystemPorts(t *testing.T) {
	_, err := operatorsettings.ResolveFromHomeWithEnvironment(
		nil,
		globalconfigmapping.Decode,
		t.TempDir(),
		operatorsettings.Defaults{},
		operatorsettings.FlagOverrides{},
	)
	if err == nil || !strings.Contains(err.Error(), "resolve operator defaults") {
		t.Fatalf("ResolveFromHomeWithEnvironment() error = %v, want home-port construction failure", err)
	}
}

func TestRegisterDefaultsResolutionFromHomeRestoresAdapterOwnership(t *testing.T) {
	operatorsettings.ConfigureDefaultsResolutionFromHome(nil)
	settingswire.RegisterDefaultsResolutionFromHome()
}

func TestResolveFromHomeUsesSettingsAdapterOwnershipPath(t *testing.T) {
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

	resolved, err := operatorsettings.ResolveFromHomeWithEnvironment(
		platformfilesystem.Local{},
		globalconfigmapping.Decode,
		homeDir,
		operatorsettings.Defaults{},
		operatorsettings.FlagOverrides{},
	)
	if err != nil {
		t.Fatalf("ResolveFromHomeWithEnvironment() error = %v", err)
	}
	if resolved.WorkerModelProvider != "CODEX" {
		t.Fatalf("provider = %q, want CODEX from adapter ownership path", resolved.WorkerModelProvider)
	}
}

func TestResolveFromHomeFallbackPreservesAcceptedSemantics(t *testing.T) {
	operatorsettings.ConfigureDefaultsResolutionFromHome(nil)
	t.Cleanup(settingswire.RegisterDefaultsResolutionFromHome)

	homeDir := t.TempDir()
	configPath := operatorsettings.DefaultConfigPath(homeDir)
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(config dir): %v", err)
	}
	if err := os.WriteFile(configPath, []byte(`{
		"defaults": {
			"workerModelProvider": "claude",
			"workerModel": "file-model"
		}
	}`), 0o600); err != nil {
		t.Fatalf("WriteFile(config): %v", err)
	}
	t.Setenv(operatorsettings.EnvDefaultWorkerModelProvider, "codex")
	t.Setenv(operatorsettings.EnvDefaultWorkerModel, "env-model")

	resolved, err := operatorsettings.ResolveFromHomeWithEnvironment(
		platformfilesystem.Local{},
		globalconfigmapping.Decode,
		homeDir,
		operatorsettings.Defaults{
			WorkerModelProvider: os.Getenv(operatorsettings.EnvDefaultWorkerModelProvider),
			WorkerModel:         os.Getenv(operatorsettings.EnvDefaultWorkerModel),
		},
		operatorsettings.FlagOverrides{},
	)
	if err != nil {
		t.Fatalf("ResolveFromHomeWithEnvironment() error = %v", err)
	}
	if resolved.WorkerModelProvider != "CODEX" {
		t.Fatalf("provider = %q, want CODEX from fallback path", resolved.WorkerModelProvider)
	}
	if resolved.WorkerModel != "env-model" {
		t.Fatalf("model = %q, want env-model", resolved.WorkerModel)
	}
	if resolved.ConfigPath != configPath {
		t.Fatalf("config path = %q, want %q", resolved.ConfigPath, configPath)
	}
}

func providerCatalog(value string) (string, bool) {
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
