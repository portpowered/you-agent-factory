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
	snapshot  *factoryruntime.LegacyEngineObservation
	sessionID string
}

func (role *factoryStatusSessionRole) GetEngineStateSnapshotForSession(_ context.Context, sessionID string) (*factoryruntime.LegacyEngineObservation, error) {
	role.sessionID = sessionID
	return role.snapshot, nil
}

type recordingFactoryStatusProjector struct {
	seen *factoryruntime.LegacyEngineObservation
	want factoryruntime.FactoryStatus
}

func (projector *recordingFactoryStatusProjector) ProjectFactoryStatus(snapshot *factoryruntime.LegacyEngineObservation) factoryruntime.FactoryStatus {
	projector.seen = snapshot
	return projector.want
}

func TestFactoryStatusAPIRoutesCurrentAndSessionSnapshotsThroughInjectedOwnerProjection(t *testing.T) {
	scoped := &factoryruntime.LegacyEngineObservation{FactoryState: "SCOPED"}
	sessions := &factoryStatusSessionRole{snapshot: scoped}
	projector := &recordingFactoryStatusProjector{want: factoryruntime.FactoryStatus{FactoryState: "DETACHED"}}
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
	}}, sessions, projector)

	got, err := api.ProjectFactoryStatus(context.Background(), "")
	if err != nil || got.FactoryState != "CURRENT" || got.RuntimeStatus != "ACTIVE" ||
		got.TotalTokens != 3 || got.Categories.Processing != 2 || len(got.Resources) != 1 ||
		got.Resources[0].Available != 2 || got.Resources[0].Total != 3 {
		t.Fatalf("current status = (%#v, %v), want root observation projection", got, err)
	}
	got, err = api.ProjectFactoryStatus(context.Background(), "session-beta")
	if err != nil || got.FactoryState != "DETACHED" || projector.seen != scoped || sessions.sessionID != "session-beta" {
		t.Fatalf("scoped status = (%#v, %v), projected snapshot = %p, session = %q", got, err, projector.seen, sessions.sessionID)
	}
}
