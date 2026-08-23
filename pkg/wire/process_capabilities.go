package wire

import (
	processcontract "github.com/portpowered/infinite-you/pkg/initializer/process"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

// provideRecordingsProjectionCapability carries the already-composed
// Recordings root across the neutral initializer boundary. The root package
// reifies only the projection methods needed by callers.
func provideRecordingsProjectionCapability(
	service recordings.Service,
) processcontract.RecordingsProjectionCapability {
	var projection recordings.ProjectionService
	if opening, ok := service.(recordings.RuntimeOpening); ok {
		projection = opening.Projection()
	}
	return recordingsProjectionCapability{service: service, projection: projection}
}

type recordingsProjectionCapability struct {
	service    recordings.Service
	projection recordings.ProjectionService
}

func (capability recordingsProjectionCapability) RecordingsProjection() any {
	return recordingsProjection{
		service:    capability.service,
		projection: capability.projection,
	}
}

type recordingsProjection struct {
	service    recordings.Service
	projection recordings.ProjectionService
}

func (projection recordingsProjection) ReconstructWorldState(
	request recordings.ReconstructWorldStateRequest,
) (recordings.ReconstructWorldStateResult, error) {
	return projection.service.ReconstructWorldState(request)
}

func (projection recordingsProjection) QueryWorkstationRequests(
	request recordings.WorkstationRequestsQueryRequest,
) (recordings.WorkstationRequestsQueryResult, error) {
	return projection.service.QueryWorkstationRequests(request)

}

func (projection recordingsProjection) ReconstructFactoryWorldState(
	events []recordings.FactoryEvent,
	selectedTick int,
) (recordings.FactoryWorldState, error) {
	return projection.projection.ReconstructFactoryWorldState(events, selectedTick)
}

func (projection recordingsProjection) ProjectWorkstationRequests(
	world recordings.FactoryWorldState,
) recordings.WorkstationFactoryWorldWorkstationRequestProjectionSlice {
	return projection.projection.ProjectWorkstationRequests(world)
}

// provideOperatorSettingsCapability carries the already-composed Operator
// Settings root across the neutral initializer boundary for public bindings.
func provideOperatorSettingsCapability(
	service operatorsettings.Service,
) processcontract.OperatorSettingsCapability {
	return operatorSettingsCapability{service: service}
}

type operatorSettingsCapability struct {
	service operatorsettings.Service
}

func (capability operatorSettingsCapability) OperatorSettings() any {
	return capability.service
}
