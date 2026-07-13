package api

import (
	"github.com/portpowered/infinite-you/pkg/api/moveprojection"
	"github.com/portpowered/infinite-you/pkg/api/workstationprojection"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// BuildFactoryWorldWorkstationRequestProjectionSlice delegates to the
// workstationprojection subpackage while preserving the historical pkg/api entrypoint.
func BuildFactoryWorldWorkstationRequestProjectionSlice(
	state interfaces.FactoryWorldState,
) factoryapi.FactoryWorldWorkstationRequestProjectionSlice {
	return workstationprojection.BuildFactoryWorldWorkstationRequestProjectionSlice(state)
}

// BuildFactoryWorldWorkMoveOperationProjectionSlice delegates to the
// moveprojection subpackage while preserving the historical pkg/api entrypoint.
func BuildFactoryWorldWorkMoveOperationProjectionSlice(
	state interfaces.FactoryWorldState,
) factoryapi.FactoryWorldWorkMoveOperationProjectionSlice {
	return moveprojection.BuildFactoryWorldWorkMoveOperationProjectionSlice(state)
}
