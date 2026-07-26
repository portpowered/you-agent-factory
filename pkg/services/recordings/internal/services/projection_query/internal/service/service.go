package service

import (
	"fmt"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingevents "github.com/portpowered/infinite-you/pkg/services/recordings/events"
	projectionquery "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/projection_query"
	"github.com/portpowered/infinite-you/pkg/services/recordings/projections"
	dashboardprojections "github.com/portpowered/infinite-you/pkg/services/recordings/projections/dashboard"
)

// Service keeps the canonical reducer and derived-query implementation behind
// the Recordings-owned projection-query capability.
type Service struct{}

var _ projectionquery.Service = (*Service)(nil)

// New constructs the stateless projection-query service.
func New() *Service {
	return &Service{}
}

func (*Service) ReconstructFactoryWorldState(
	events []factorydefinitions.FactoryEvent,
	selectedTick int,
) (factorydefinitions.FactoryWorldState, error) {
	if selectedTick < 0 {
		return factorydefinitions.FactoryWorldState{}, recordings.ErrInvalidProjectionInput
	}
	state, err := projections.ReconstructCanonicalFactoryWorldState(events, selectedTick)
	if err != nil {
		return factorydefinitions.FactoryWorldState{}, fmt.Errorf(
			"%w: %v",
			recordings.ErrInvalidProjectionInput,
			err,
		)
	}
	return state, nil
}

func (*Service) SimpleDashboardRenderData(
	state factorydefinitions.FactoryWorldState,
) recordings.SimpleDashboardRenderData {
	return dashboardprojections.SimpleDashboardRenderDataFromWorldState(state)
}

func (*Service) ProjectActiveThrottlePauses(
	topology factorydefinitions.InitialStructurePayload,
	pauses []factorydefinitions.ActiveThrottlePause,
) []factorydefinitions.FactoryWorldThrottlePause {
	return projections.ProjectActiveThrottlePauses(topology, pauses)
}

func (*Service) ProjectWorkstationRequests(
	state factorydefinitions.FactoryWorldState,
) recordings.WorkstationFactoryWorldWorkstationRequestProjectionSlice {
	return recordings.BuildFactoryWorldWorkstationRequestProjectionSlice(state)
}

func (*Service) ValidateReconnectReplay(
	events []factorydefinitions.FactoryEvent,
	cursor factorydefinitions.FactoryEventReconnectCursor,
	scope factorydefinitions.FactoryEventReconnectScope,
) error {
	_, err := recordingevents.BuildCanonicalReconnectReplay(events, cursor, scope)
	return err
}
