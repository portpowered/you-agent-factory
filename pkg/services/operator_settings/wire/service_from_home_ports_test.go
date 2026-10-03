package wire_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	internaltestproviders "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/testproviders"
	globalconfigmapping "github.com/portpowered/infinite-you/pkg/services/operator_settings/transports/globalconfig"
	settingswire "github.com/portpowered/infinite-you/pkg/services/operator_settings/wire"
)

func testIDGenerator() operatorsettings.IDGenerator {
	return func() string { return "00000000-0000-4000-8000-000000000001" }
}

func TestNewServiceFromHomePortsRequiresFilesystem(t *testing.T) {
	t.Parallel()

	_, err := settingswire.NewServiceFromHomePorts(nil, globalconfigmapping.Decode, internaltestproviders.StandardCatalog(), testIDGenerator(), logging.NoopLogger{})
	if err == nil || !strings.Contains(err.Error(), "filesystem is required") {
		t.Fatalf("NewServiceFromHomePorts(nil, decode) error = %v, want filesystem required", err)
	}
}

func TestNewServiceFromHomePortsRequiresDecoder(t *testing.T) {
	t.Parallel()

	_, err := settingswire.NewServiceFromHomePorts(platformfilesystem.Local{}, nil, internaltestproviders.StandardCatalog(), testIDGenerator(), logging.NoopLogger{})
	if err == nil || !strings.Contains(err.Error(), "decoder is required") {
		t.Fatalf("NewServiceFromHomePorts(files, nil) error = %v, want decoder required", err)
	}
}

func TestNewServiceFromHomePortsConstructsAcceptedSettingsRoot(t *testing.T) {
	t.Parallel()

	root, err := settingswire.NewServiceFromHomePorts(
		platformfilesystem.Local{},
		globalconfigmapping.Decode,
		internaltestproviders.StandardCatalog(),
		testIDGenerator(),
		logging.NoopLogger{},
	)
	if err != nil {
		t.Fatalf("NewServiceFromHomePorts() error = %v", err)
	}
	if root == nil {
		t.Fatal("NewServiceFromHomePorts() = nil, want Settings root")
	}
}

func TestResolveFromHomeViaSettingsCLIAdapterOwnershipPath(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	configPath := operatorsettings.DefaultConfigPath(homeDir)
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(config): %v", err)
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
		logging.NoopLogger{},
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
		t.Fatalf("provider = %q, want CODEX alias canonicalization", resolved.WorkerModelProvider)
	}
	if resolved.WorkerModel != "file-model" {
		t.Fatalf("model = %q, want file-model", resolved.WorkerModel)
	}
}

func TestResolveFromHomeViaSettingsCLIRejectsMissingFilesystemPorts(t *testing.T) {
	t.Parallel()

	_, err := settingswire.NewServiceFromHomePorts(
		nil,
		globalconfigmapping.Decode,
		internaltestproviders.StandardCatalog(),
		testIDGenerator(),
		logging.NoopLogger{},
	)
	if err == nil || !strings.Contains(err.Error(), "filesystem is required") {
		t.Fatalf("NewServiceFromHomePorts() error = %v, want home-port construction failure", err)
	}
}

func TestNewServiceFromHomePortsRejectsMissingIDGenerator(t *testing.T) {
	t.Parallel()

	_, err := settingswire.NewServiceFromHomePorts(
		platformfilesystem.Local{},
		globalconfigmapping.Decode,
		internaltestproviders.StandardCatalog(),
		nil,
		logging.NoopLogger{},
	)
	if err == nil || err.Error() != "operator settings ID generator is required" {
		t.Fatalf("NewServiceFromHomePorts() error = %v, want missing ID generator", err)
	}
}

// TestResolveFromHomeFallbackPreservesAcceptedSemantics proves defaults
// resolution through the published Operator Settings root contract preserves
// accepted environment-over-file semantics when adapter ownership is unset.
func TestResolveFromHomeFallbackPreservesAcceptedSemantics(t *testing.T) {
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

	providersRoot := internaltestproviders.StandardCatalog()
	settingsRoot, err := settingswire.NewServiceFromHomePorts(
		platformfilesystem.Local{},
		globalconfigmapping.Decode,
		providersRoot,
		func() string { return "00000000-0000-4000-8000-000000000001" },
		logging.NoopLogger{},
	)
	if err != nil {
		t.Fatalf("NewServiceFromHomePorts() error = %v", err)
	}
	resolved, err := settingsRoot.ResolveFromHomeWithEnvironment(
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
