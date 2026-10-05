package wire

import (
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	definitionsinternal "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal"
)

type RuntimeSnapshotMaterializer = definitionsinternal.RuntimeSnapshotMaterializer

func NewRuntimeSnapshotMaterializer(loader factorydefinitions.LoadedFactorySourceFactory) *RuntimeSnapshotMaterializer {
	return definitionsinternal.NewRuntimeSnapshotMaterializer(loader)
}
