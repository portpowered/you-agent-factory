package service

import (
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	projectionquery "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/projection_query"
)

func TestServiceProvidesProjectionQueryCapability(t *testing.T) {
	t.Parallel()

	var capability projectionquery.Service = New()
	state, err := capability.ReconstructFactoryWorldState(nil, 0)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState: %v", err)
	}
	if state.Tick != 0 {
		t.Fatalf("world state tick = %d, want 0", state.Tick)
	}

	dashboard := capability.SimpleDashboardRenderData(state)
	if dashboard.Session.HasData {
		t.Fatalf("dashboard session = %#v, want no projected data", dashboard.Session)
	}
	if pauses := capability.ProjectActiveThrottlePauses(
		factorydefinitions.InitialStructurePayload{},
		nil,
	); len(pauses) != 0 {
		t.Fatalf("active throttle pauses = %#v, want empty", pauses)
	}
	requests := capability.ProjectWorkstationRequests(state)
	if requests.WorkstationRequestsByDispatchId != nil {
		t.Fatalf("workstation request projection = %#v, want empty", requests)
	}
	if err := capability.ValidateReconnectReplay(
		nil,
		factorydefinitions.FactoryEventReconnectCursor{},
		factorydefinitions.FactoryEventReconnectScope{},
	); err != nil {
		t.Fatalf("ValidateReconnectReplay: %v", err)
	}
}
