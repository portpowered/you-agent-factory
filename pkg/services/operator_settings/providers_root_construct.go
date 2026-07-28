package operatorsettings

import (
	"fmt"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

// ProvidersRootConstructor builds the Providers service root used by transitional
// Settings composition paths.
type ProvidersRootConstructor func() (providers.Service, error)

var providersRootConstructor ProvidersRootConstructor

// ConfigureProvidersRootConstructor registers the Providers root constructor
// used when transitional servicewire composition assembles resolution behavior.
// Process composition supplies the concrete constructor from pkg/wire.
func ConfigureProvidersRootConstructor(constructor ProvidersRootConstructor) {
	providersRootConstructor = constructor
}

// ConstructProvidersRoot returns the configured Providers service root.
func ConstructProvidersRoot() (providers.Service, error) {
	if providersRootConstructor == nil {
		return nil, fmt.Errorf("providers root constructor is required")
	}
	return providersRootConstructor()
}
