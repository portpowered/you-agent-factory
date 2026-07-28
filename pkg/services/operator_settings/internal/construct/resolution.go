package construct

import (
	"fmt"

	settingsinternal "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal"
	resolution "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/services/resolution"
	resolutionwire "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/services/resolution/wire"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

// ConstructResolutionService builds the parent-private resolution capability used
// by transitional home-port and config-document composition paths.
func ConstructResolutionService() (resolution.Service, error) {
	return constructResolutionService()
}

var constructResolutionService = defaultResolutionService

var (
	constructProvidersRoot  = settingsinternal.ConstructProvidersRoot
	constructResolutionWire = resolutionwire.NewService
)

func defaultResolutionService() (resolution.Service, error) {
	providersRoot, err := constructProvidersRoot()
	if err != nil {
		return nil, fmt.Errorf("construct providers root: %w", err)
	}
	resolutionService, err := constructResolutionWire(providersRoot)
	if err != nil {
		return nil, fmt.Errorf("construct resolution service: %w", err)
	}
	return resolutionService, nil
}

// SetConstructResolutionServiceForTests replaces resolution construction for tests.
func SetConstructResolutionServiceForTests(
	constructor func() (resolution.Service, error),
) func() {
	previous := constructResolutionService
	constructResolutionService = constructor
	return func() { constructResolutionService = previous }
}

// SetConstructProvidersRootForTests replaces providers-root construction for tests.
func SetConstructProvidersRootForTests(
	constructor func() (providers.Service, error),
) func() {
	previous := constructProvidersRoot
	constructProvidersRoot = constructor
	return func() { constructProvidersRoot = previous }
}

// SetConstructResolutionWireForTests replaces resolution wire construction for tests.
func SetConstructResolutionWireForTests(
	constructor func(providers.Service) (resolution.Service, error),
) func() {
	previous := constructResolutionWire
	constructResolutionWire = constructor
	return func() { constructResolutionWire = previous }
}
