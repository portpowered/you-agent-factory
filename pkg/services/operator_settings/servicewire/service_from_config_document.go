package operatorsettingsservicewire

import (
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	settingsconstruct "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/construct"
)

// NewServiceFromConfigDocument constructs the accepted Settings root from the
// ConfigDocumentService ports Wire already injects for configure composition.
func NewServiceFromConfigDocument(
	service operatorsettings.ConfigDocumentService,
) (operatorsettings.Service, error) {
	return settingsconstruct.NewServiceFromConfigDocument(service)
}
