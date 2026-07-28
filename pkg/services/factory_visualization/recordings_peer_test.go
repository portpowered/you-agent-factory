package factory_visualization_test

import (
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingsservice "github.com/portpowered/infinite-you/pkg/services/recordings/service"
)

func TestRecordingsPeerFromProjectionServiceAdaptsLegacyProjectionPeer(t *testing.T) {
	t.Parallel()

	peer := recordingsservice.NewProjectionService()
	service, err := factoryvisualization.RecordingsPeerFromProjectionService(peer)
	if err != nil {
		t.Fatalf("RecordingsPeerFromProjectionService: %v", err)
	}
	if service == nil {
		t.Fatal("expected adapted recordings.Service, got nil")
	}
	if _, ok := peer.(recordings.Service); ok {
		t.Fatal("test setup: projection peer should not implement recordings.Service directly")
	}
}

type projectionOnlyPeer struct{}

func (projectionOnlyPeer) ReconstructFactoryWorldState(
	[]factorydefinitions.FactoryEvent,
	int,
) (factorydefinitions.FactoryWorldState, error) {
	return factorydefinitions.FactoryWorldState{}, nil
}

func (projectionOnlyPeer) SimpleDashboardRenderData(
	factorydefinitions.FactoryWorldState,
) recordings.SimpleDashboardRenderData {
	return recordings.SimpleDashboardRenderData{InFlightDispatchCount: 1}
}

func (projectionOnlyPeer) ProjectActiveThrottlePauses(
	factorydefinitions.InitialStructurePayload,
	[]factorydefinitions.ActiveThrottlePause,
) []factorydefinitions.FactoryWorldThrottlePause {
	return nil
}

func (projectionOnlyPeer) ProjectWorkstationRequests(
	factorydefinitions.FactoryWorldState,
) recordings.WorkstationFactoryWorldWorkstationRequestProjectionSlice {
	return recordings.WorkstationFactoryWorldWorkstationRequestProjectionSlice{}
}

func (projectionOnlyPeer) ValidateReconnectReplay(
	[]factorydefinitions.FactoryEvent,
	factorydefinitions.FactoryEventReconnectCursor,
	factorydefinitions.FactoryEventReconnectScope,
) error {
	return nil
}

func TestRecordingsPeerFromProjectionServiceWrapsProjectionOnlyPeer(t *testing.T) {
	t.Parallel()

	service, err := factoryvisualization.RecordingsPeerFromProjectionService(projectionOnlyPeer{})
	if err != nil {
		t.Fatalf("RecordingsPeerFromProjectionService: %v", err)
	}
	result, err := service.QuerySimpleDashboard(recordings.SimpleDashboardQueryRequest{
		WorldState: recordings.WorldStateView{
			SchemaVersion: recordings.WorldStateViewSchemaV1,
			Payload:       `{"topology":{}}`,
		},
	})
	if err != nil {
		t.Fatalf("QuerySimpleDashboard through adapted peer: %v", err)
	}
	if result.Data.InFlightDispatchCount != 1 {
		t.Fatalf("dashboard in-flight count = %d, want 1", result.Data.InFlightDispatchCount)
	}
}
