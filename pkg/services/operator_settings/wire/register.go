package wire

import (
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	settingsinternal "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	providerswire "github.com/portpowered/infinite-you/pkg/services/providers/wire"

	_ "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/identityinputinventory"
	_ "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/services/document"
	_ "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/services/resolution/defaults"
)

func init() {
	operatorsettings.ConfigureDocumentOwnerConstructor(NewDocumentOwner)
	settingsinternal.ConfigureProvidersRootConstructor(func() (providers.Service, error) {
		return providerswire.NewService()
	})
}
