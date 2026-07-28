package operatorsettingsservicewire

import (
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	settingsconstruct "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/construct"
)

// NewServiceFromHomePorts constructs the accepted Settings root from the
// filesystem and decoder ports Wire already injects for defaults resolution.
func NewServiceFromHomePorts(
	files operatorsettings.FileSystem,
	decode operatorsettings.ConfigDecoder,
) (operatorsettings.Service, error) {
	return settingsconstruct.NewServiceFromHomePorts(files, decode)
}
