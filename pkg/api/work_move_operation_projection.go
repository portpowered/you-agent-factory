package api

import (
	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/api/moveprojection"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

// BuildFactoryWorldWorkMoveOperationProjectionSlice delegates to the
// moveprojection subpackage while preserving the historical pkg/api entrypoint.
func BuildFactoryWorldWorkMoveOperationProjectionSlice(
	state interfaces.FactoryWorldState,
) factoryapi.FactoryWorldWorkMoveOperationProjectionSlice {
	return moveprojection.BuildFactoryWorldWorkMoveOperationProjectionSlice(state)
}
