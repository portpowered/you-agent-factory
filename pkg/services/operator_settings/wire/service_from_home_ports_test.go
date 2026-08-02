package wire_test

import (
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

func TestNewServiceFromHomePortsRequiresFilesystem(t *testing.T) {
	t.Parallel()

	_, err := settingswire.NewServiceFromHomePorts(nil, globalconfigmapping.Decode, internaltestproviders.StandardCatalog(), testIDGenerator())
	if err == nil || !strings.Contains(err.Error(), "filesystem is required") {
		t.Fatalf("NewServiceFromHomePorts(nil, decode) error = %v, want filesystem required", err)
	}
}

func TestNewServiceFromHomePortsRequiresDecoder(t *testing.T) {
	t.Parallel()

	_, err := settingswire.NewServiceFromHomePorts(platformfilesystem.Local{}, nil, internaltestproviders.StandardCatalog(), testIDGenerator())
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
	)
	if err == nil || err.Error() != "operator settings ID generator is required" {
		t.Fatalf("NewServiceFromHomePorts() error = %v, want missing ID generator", err)
	}
}
