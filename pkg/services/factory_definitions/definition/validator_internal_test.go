package factorydefinition

import (
	factorydefinitionfixtures "github.com/portpowered/infinite-you/internal/testutil/factorydefinitionfixtures"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation/impl"
	factorymapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
)

func testFactoryDefinitionValidator() *factoryvalidation.Service {
	return factoryvalidation.New(nil)
}

func testCanonicalFactoryLoader(
	payload []byte,
	_ factorydefinitions.WorkstationLoader,
) (factorydefinitions.MutableLoadedFactorySource, error) {
	config, err := factorymapping.NewFactoryConfigMapper().Expand(payload)
	if err != nil {
		return nil, err
	}
	return factorydefinitionfixtures.NewLoadedSource("", config, nil, nil)
}
