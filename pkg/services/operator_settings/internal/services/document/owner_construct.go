package settingsdocument

import (
	"fmt"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
)

var ownerConstructor operatorsettings.DocumentOwnerConstructor

// ConfigureOwnerConstructor registers the nested document owner constructor
// used when ConfigDocumentService.DocumentOwner is unset. Wire and servicewire
// call this during process composition.
func ConfigureOwnerConstructor(constructor operatorsettings.DocumentOwnerConstructor) {
	ownerConstructor = constructor
}

// ResolvedDocumentOwner resolves the nested document owner for one
// ConfigDocumentService, using the injected owner or registered constructor.
func ResolvedDocumentOwner(service operatorsettings.ConfigDocumentService) (operatorsettings.DocumentOwner, error) {
	if service.DocumentOwner != nil {
		return service.DocumentOwner, nil
	}
	return newDocumentOwnerFromConstructor(service)
}

func newDocumentOwnerFromConstructor(service operatorsettings.ConfigDocumentService) (operatorsettings.DocumentOwner, error) {
	if ownerConstructor == nil {
		return nil, fmt.Errorf("operator settings document owner is required")
	}
	return ownerConstructor(
		service.Files,
		service.CreateTemp,
		service.Decoder,
		service.Encoder,
		service.Providers,
	), nil
}
