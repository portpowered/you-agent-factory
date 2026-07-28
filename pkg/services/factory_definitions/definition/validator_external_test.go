package factorydefinition_test

import (
	"context"

	factorydefinitionfixtures "github.com/portpowered/infinite-you/internal/testutil/factorydefinitionfixtures"
	factoryeditable "github.com/portpowered/infinite-you/pkg/services/factory_definitions/editable"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/validation"
	factorymapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/validationentry"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func testFactoryDefinitionValidator() *factoryvalidation.Service {
	return factoryvalidation.New(nil)
}

func validateEditableFactorySnapshotForTest(
	ctx context.Context,
	snapshot *factorydefinitions.FactorySnapshot,
	loader factorydefinitions.WorkstationLoader,
) error {
	return factoryeditable.ValidateSnapshot(
		ctx,
		snapshot,
		loader,
		func(snapshot *factorydefinitions.FactorySnapshot, loader factorydefinitions.WorkstationLoader) (factorydefinitions.DefinitionValidationRequest, error) {
			return validationentry.MapEditableFactorySnapshot(snapshot, loader, testCanonicalFactoryLoader)
		},
		testFactoryDefinitionValidator(),
	)
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
