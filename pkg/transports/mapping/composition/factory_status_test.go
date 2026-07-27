package composition

import (
	"context"
	"testing"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
)

type factoryStatusRuntimeRole struct {
	factoryruntime.Service
	observation factoryruntime.Observation
}

func (role factoryStatusRuntimeRole) Observe(context.Context, factoryruntime.ObserveRequest) (factoryruntime.ObserveResult, error) {
	return factoryruntime.ObserveResult{Observation: role.observation}, nil
}

type factoryStatusSessionRole struct {
	observation factoryruntime.Observation
	sessionID   string
}

func (role *factoryStatusSessionRole) ObserveForSession(_ context.Context, sessionID string, _ factoryruntime.ObserveRequest) (factoryruntime.ObserveResult, error) {
	role.sessionID = sessionID
	return factoryruntime.ObserveResult{Observation: role.observation}, nil
}

func TestFactoryStatusAPIRoutesCurrentAndSessionObservationsThroughNeutralProjection(t *testing.T) {
	scopedObservation := factoryruntime.Observation{
		Status: factoryruntime.ObservationStatusActive,
		Progress: factoryruntime.ObservationProgress{
			TotalWorkCount: 2,
			WorkCategories: factoryruntime.ObservationWorkCategories{Processing: 1, Terminal: 1},
		},
		Health: factoryruntime.ObservationHealth{FactoryState: "SCOPED"},
	}
	sessions := &factoryStatusSessionRole{observation: scopedObservation}
	api := newFactoryStatusAPI(factoryStatusRuntimeRole{observation: factoryruntime.Observation{
		Status: factoryruntime.ObservationStatusActive,
		Progress: factoryruntime.ObservationProgress{
			TotalWorkCount: 3,
			WorkCategories: factoryruntime.ObservationWorkCategories{Processing: 2, Terminal: 1},
		},
		Resources: []factoryruntime.ObservationResourceView{{
			ResourceID: "worker-slot", InUseCount: 1, AvailableCount: 2,
		}},
		Health: factoryruntime.ObservationHealth{
			FactoryState: "CURRENT", LifecycleControlStatus: "RUNNING",
		},
	}}, sessions)

	got, err := api.ProjectFactoryStatus(context.Background(), "")
	if err != nil || got.FactoryState != "CURRENT" || got.RuntimeStatus != "ACTIVE" ||
		got.TotalTokens != 3 || got.Categories.Processing != 2 || len(got.Resources) != 1 ||
		got.Resources[0].Available != 2 || got.Resources[0].Total != 3 {
		t.Fatalf("current status = (%#v, %v), want root observation projection", got, err)
	}
	got, err = api.ProjectFactoryStatus(context.Background(), "session-beta")
	if err != nil || got.FactoryState != "SCOPED" || got.TotalTokens != 2 || sessions.sessionID != "session-beta" {
		t.Fatalf("scoped status = (%#v, %v), session = %q", got, err, sessions.sessionID)
	}
}
