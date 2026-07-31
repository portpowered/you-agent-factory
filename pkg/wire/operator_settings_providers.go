package wire

import (
	settingswire "github.com/portpowered/infinite-you/pkg/services/operator_settings/wire"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	providerswire "github.com/portpowered/infinite-you/pkg/services/providers/wire"
)

func init() {
	settingswire.RegisterProvidersRootConstructor(func() (providers.Service, error) {
		return providerswire.NewService()
	})
}
