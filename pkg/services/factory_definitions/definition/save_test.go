package factorydefinition

import (
	"context"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryeditable "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability/editable"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/transports/mapping/validationentry"
)

func validateDefinitionSnapshotForTest(
	ctx context.Context,
	snapshot *factorydefinitions.FactorySnapshot,
	loader factorydefinitions.WorkstationLoader,
) error {
	return factoryeditable.ValidateSnapshot(
		ctx,
		snapshot,
		loader,
		func(snapshot *factorydefinitions.FactorySnapshot, loader factorydefinitions.WorkstationLoader) (factorydefinitions.DefinitionValidationRequest, error) {
			return validationentry.MapEditableFactorySnapshot(snapshot, loader)
		},
		testFactoryDefinitionValidator(),
	)
}
