package wire

import (
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	providerswire "github.com/portpowered/infinite-you/pkg/services/providers/wire"
)

func init() {
	operatorsettings.ConfigureProvidersRootConstructor(func() (providers.Service, error) {
		return providerswire.NewService()
	})
}
