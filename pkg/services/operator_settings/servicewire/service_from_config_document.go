package operatorsettingsservicewire

import (
	"fmt"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
)

// NewServiceFromConfigDocument constructs the accepted Settings root from the
// ConfigDocumentService ports Wire already injects for configure composition.
func NewServiceFromConfigDocument(
	service operatorsettings.ConfigDocumentService,
) (operatorsettings.Service, error) {
	documentOwner := service.DocumentOwner
	if documentOwner == nil {
		if service.Files == nil || service.Decoder == nil {
			return nil, fmt.Errorf("operator settings document ports are required")
		}
		documentOwner = NewDocumentOwner(
			service.Files,
			service.CreateTemp,
			service.Decoder,
			service.Encoder,
			service.Providers,
		)
	}
	resolutionService, err := newResolutionService()
	if err != nil {
		return nil, err
	}
	return newCompositionRoot(documentOwner, resolutionService)
}
