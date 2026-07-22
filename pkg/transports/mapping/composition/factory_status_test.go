package composition

import (
	"context"
	"testing"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
)

type factoryStatusRuntimeRole struct {
	factoryruntime.APIFactory
	snapshot *factoryruntime.StateSnapshot
}

func (role factoryStatusRuntimeRole) GetEngineStateSnapshot(context.Context) (*factoryruntime.StateSnapshot, error) {
	return role.snapshot, nil
}

type factoryStatusSessionRole struct {
	snapshot  *factoryruntime.StateSnapshot
	sessionID string
}

func (role *factoryStatusSessionRole) GetEngineStateSnapshotForSession(_ context.Context, sessionID string) (*factoryruntime.StateSnapshot, error) {
	role.sessionID = sessionID
	return role.snapshot, nil
}

type recordingFactoryStatusProjector struct {
	seen *factoryruntime.StateSnapshot
	want factoryruntime.FactoryStatus
}

func (projector *recordingFactoryStatusProjector) ProjectFactoryStatus(snapshot *factoryruntime.StateSnapshot) factoryruntime.FactoryStatus {
	projector.seen = snapshot
	return projector.want
}

func TestFactoryStatusAPIRoutesCurrentAndSessionSnapshotsThroughInjectedOwnerProjection(t *testing.T) {
	current := &factoryruntime.StateSnapshot{FactoryState: "CURRENT"}
	scoped := &factoryruntime.StateSnapshot{FactoryState: "SCOPED"}
	sessions := &factoryStatusSessionRole{snapshot: scoped}
	projector := &recordingFactoryStatusProjector{want: factoryruntime.FactoryStatus{FactoryState: "DETACHED"}}
	api := newFactoryStatusAPI(factoryStatusRuntimeRole{snapshot: current}, sessions, projector)

	got, err := api.ProjectFactoryStatus(context.Background(), "")
	if err != nil || got.FactoryState != "DETACHED" || projector.seen != current {
		t.Fatalf("current status = (%#v, %v), projected snapshot = %p, want detached result from %p", got, err, projector.seen, current)
	}
	got, err = api.ProjectFactoryStatus(context.Background(), "session-beta")
	if err != nil || got.FactoryState != "DETACHED" || projector.seen != scoped || sessions.sessionID != "session-beta" {
		t.Fatalf("scoped status = (%#v, %v), projected snapshot = %p, session = %q", got, err, projector.seen, sessions.sessionID)
	}
}
