package wire

import (
	"testing"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	settingswire "github.com/portpowered/infinite-you/pkg/services/operator_settings/wire"
	globalconfigmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/globalconfig"
)

func TestOperatorSettingsHomePortCompositionUsesProcessProviderRoot(t *testing.T) {
	t.Parallel()

	service, err := settingswire.NewServiceFromHomePorts(platformfilesystem.Local{}, globalconfigmapping.Decode)
	if err != nil {
		t.Fatalf("NewServiceFromHomePorts() error = %v", err)
	}
	if service == nil {
		t.Fatal("NewServiceFromHomePorts() = nil, want Operator Settings root")
	}
}
