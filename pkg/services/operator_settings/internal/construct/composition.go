package construct

import (
	"fmt"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	operatorservice "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/service"
	settingsdocument "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/services/document"
	resolution "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/services/resolution"
)

// newServiceRoot is retained only for tests and legacy package migration. The
// canonical production constructor is operator_settings/wire.NewService,
// which supplies the complete dependency set directly.
func newServiceRoot(
	document operatorsettings.DocumentOwner,
	resolutionService resolution.Service,
	files operatorsettings.FileSystem,
	createTemp operatorsettings.CreateTemporaryFile,
	decoder operatorsettings.ConfigDecoder,
	encoder operatorsettings.ConfigEncoder,
	idGenerator operatorsettings.IDGenerator,
) (operatorsettings.Service, error) {
	if document == nil {
		return nil, fmt.Errorf("operator settings document owner is required")
	}
	if resolutionService == nil {
		return nil, fmt.Errorf("operator settings resolution service is required")
	}
	documentService, ok := document.(settingsdocument.Service)
	if !ok {
		return nil, fmt.Errorf("operator settings document owner must implement the document service")
	}
	return operatorservice.New(
		documentService,
		resolutionService,
		files,
		createTemp,
		decoder,
		encoder,
		idGenerator,
		nil,
	)
}
