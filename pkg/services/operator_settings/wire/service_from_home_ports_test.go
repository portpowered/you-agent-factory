package wire_test

import (
	"strings"
	"testing"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	settingswire "github.com/portpowered/infinite-you/pkg/services/operator_settings/wire"
	globalconfigmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/globalconfig"
)

func TestNewServiceFromHomePortsRequiresFilesystem(t *testing.T) {
	t.Parallel()

	_, err := settingswire.NewServiceFromHomePorts(nil, globalconfigmapping.Decode)
	if err == nil || !strings.Contains(err.Error(), "filesystem is required") {
		t.Fatalf("NewServiceFromHomePorts(nil, decode) error = %v, want filesystem required", err)
	}
}

func TestNewServiceFromHomePortsRequiresDecoder(t *testing.T) {
	t.Parallel()

	_, err := settingswire.NewServiceFromHomePorts(platformfilesystem.Local{}, nil)
	if err == nil || !strings.Contains(err.Error(), "decoder is required") {
		t.Fatalf("NewServiceFromHomePorts(files, nil) error = %v, want decoder required", err)
	}
}

func TestNewServiceFromHomePortsConstructsAcceptedSettingsRoot(t *testing.T) {
	t.Parallel()

	root, err := settingswire.NewServiceFromHomePorts(platformfilesystem.Local{}, globalconfigmapping.Decode)
	if err != nil {
		t.Fatalf("NewServiceFromHomePorts() error = %v", err)
	}
	if root == nil {
		t.Fatal("NewServiceFromHomePorts() = nil, want Settings root")
	}
}
