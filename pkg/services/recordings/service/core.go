package service

import (
	"context"
	"strings"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingevents "github.com/portpowered/infinite-you/pkg/services/recordings/events"
	"github.com/portpowered/infinite-you/pkg/services/recordings/projections"
	dashboardprojections "github.com/portpowered/infinite-you/pkg/services/recordings/projections/dashboard"
)

type projectionService struct{}

func NewProjectionService() recordings.ProjectionService {
	return projectionService{}
}

func (projectionService) ReconstructFactoryWorldState(
	events []factorydefinitions.FactoryEvent,
	selectedTick int,
) (factorydefinitions.FactoryWorldState, error) {
	return projections.ReconstructCanonicalFactoryWorldState(events, selectedTick)
}

func (projectionService) SimpleDashboardRenderData(
	state factorydefinitions.FactoryWorldState,
) recordings.SimpleDashboardRenderData {
	return dashboardprojections.SimpleDashboardRenderDataFromWorldState(state)
}

func (projectionService) ProjectActiveThrottlePauses(
	topology factorydefinitions.InitialStructurePayload,
	pauses []factorydefinitions.ActiveThrottlePause,
) []factorydefinitions.FactoryWorldThrottlePause {
	return projections.ProjectActiveThrottlePauses(topology, pauses)
}

func (projectionService) ProjectWorkstationRequests(
	state factorydefinitions.FactoryWorldState,
) recordings.WorkstationFactoryWorldWorkstationRequestProjectionSlice {
	return recordings.BuildFactoryWorldWorkstationRequestProjectionSlice(state)
}

func (projectionService) ValidateReconnectReplay(
	recorded []factorydefinitions.FactoryEvent,
	cursor factorydefinitions.FactoryEventReconnectCursor,
	scope factorydefinitions.FactoryEventReconnectScope,
) error {
	_, err := recordingevents.BuildCanonicalReconnectReplay(recorded, cursor, scope)
	return err
}

type combinedService struct {
	recordings.Ledger
	recordings.ProjectionService
}

var _ recordings.Service = (*combinedService)(nil)

func (service *combinedService) Append(
	request recordings.AppendRecordedEventRequest,
) recordings.AppendRecordedEventResult {
	service.AppendRecordedEvent(request.Event)
	return recordings.AppendRecordedEventResult{}
}

func (service *combinedService) SubscribeFrom(
	ctx context.Context,
	request recordings.SubscribeRequest,
) (recordings.SubscribeResult, error) {
	if request.Scope.SessionID != "" && strings.TrimSpace(request.Scope.SessionID) == "" {
		return recordings.SubscribeResult{}, recordings.ErrInvalidSubscribeScope
	}
	stream, err := service.Subscribe(ctx, request.Cursor, request.Scope)
	if err != nil {
		return recordings.SubscribeResult{}, err
	}
	return recordings.SubscribeResult{Stream: stream}, nil
}

func NewService(
	ledger recordings.Ledger,
	projection recordings.ProjectionService,
) recordings.Service {
	if ledger == nil || projection == nil {
		return nil
	}
	return &combinedService{Ledger: ledger, ProjectionService: projection}
}

func NewRuntimeLedger(
	topology recordings.InitialStructureSource,
	now func() time.Time,
	streamGenerationID string,
	definitions factorydefinitions.RuntimeDefinitionLookup,
) recordings.RuntimeEventLedger {
	return recordingevents.NewRuntimeLedger(topology, now, streamGenerationID, definitions)
}
