package operatorsettingsservicewire_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	operatorsettingsservicewire "github.com/portpowered/infinite-you/pkg/services/operator_settings/servicewire"
	globalconfigmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/globalconfig"
)

func TestNewServiceFromHomePortsRequiresFilesystem(t *testing.T) {
	t.Parallel()

	_, err := operatorsettingsservicewire.NewServiceFromHomePorts(nil, globalconfigmapping.Decode)
	if err == nil || !strings.Contains(err.Error(), "filesystem is required") {
		t.Fatalf("NewServiceFromHomePorts(nil, decode) error = %v, want filesystem required", err)
	}
}

func TestNewServiceFromHomePortsRequiresDecoder(t *testing.T) {
	t.Parallel()

	_, err := operatorsettingsservicewire.NewServiceFromHomePorts(platformfilesystem.Local{}, nil)
	if err == nil || !strings.Contains(err.Error(), "decoder is required") {
		t.Fatalf("NewServiceFromHomePorts(files, nil) error = %v, want decoder required", err)
	}
}

func TestNewServiceFromHomePortsConstructsAcceptedSettingsRoot(t *testing.T) {
	t.Parallel()

	root, err := operatorsettingsservicewire.NewServiceFromHomePorts(platformfilesystem.Local{}, globalconfigmapping.Decode)
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
		t.Fatalf("provider = %q, want CODEX alias canonicalization", resolved.WorkerModelProvider)
	}
	if resolved.WorkerModel != "file-model" {
		t.Fatalf("model = %q, want file-model", resolved.WorkerModel)
	}
}

func TestResolveFromHomeViaSettingsCLIRejectsMissingFilesystemPorts(t *testing.T) {
	t.Parallel()

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
