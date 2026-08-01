package observation

import (
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
)

const statusObserveScope = factoryruntime.ObservationScopeFull

func statusResponseFromObservation(observation factoryruntime.Observation) factoryapi.StatusResponse {
	return apisurface.FactoryStatusToAPI(factoryruntime.FactoryStatusFromObservation(observation))
}
