package construct

import (
	"fmt"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	settingsdocumentwire "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/services/document/wire"
)

// NewServiceFromHomePorts constructs the accepted Settings root from the
// filesystem and decoder ports Wire already injects for defaults resolution.
func NewServiceFromHomePorts(
	files operatorsettings.FileSystem,
	decode operatorsettings.ConfigDecoder,
) (operatorsettings.Service, error) {
	if files == nil {
		return nil, fmt.Errorf("operator settings filesystem is required")
	}
	if decode == nil {
		return nil, fmt.Errorf("operator settings decoder is required")
	}
	documentOwner := settingsdocumentwire.NewService(files, nil, decode, nil, nil)
	resolutionService, err := constructResolutionService()
	if err != nil {
		return nil, err
	}
	return newServiceRoot(
		documentOwner,
		resolutionService,
		files,
		nil,
		decode,
		nil,
		nil,
	)
}
