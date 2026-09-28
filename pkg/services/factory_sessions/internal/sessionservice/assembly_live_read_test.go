package service

import (
	"context"
	"errors"
	"testing"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimebinding"
)

type projectionOwnerStub struct {
	status string
}

func (owner projectionOwnerStub) BuildSessionProjectionContext(_ context.Context, session *livesession.LiveSession) (factorysessions.ProjectionContext, error) {
	return factorysessions.ProjectionContext{
		FactorySessionID: session.ID,
		Session: &factorysessions.ScopedLiveSessionSummary{
			ID: session.ID, FactoryDir: session.FactoryDir, FolderPath: session.FolderPath,
			Project: session.Project, IsDefault: session.IsDefault,
			Runtime: &factorysessions.RuntimeProjection{},
		},
		LifecycleControlStatus: owner.status,
		BackendScopeID:         "backend-" + session.ID,
	}, nil
}

func TestAssemblyReadsFullProjectionFromCanonicalSessionRegistry(t *testing.T) {
	state := newWorkResolverSessionState()
	assembly := &Assembly{state: state, registry: state.Registry()}
	for _, record := range []struct {
		id, status string
		isDefault  bool
	}{
		{id: "session-second", status: "paused"},
		{id: "session-first", status: "running", isDefault: true},
	} {
		assembly.registry.Upsert(&livesession.LiveSession{
			ID:           record.id,
			SessionState: livesession.SessionState{FactoryDir: "/factory/" + record.id, FolderPath: "/project"},
			IsDefault:    record.isDefault,
			Runtime:      &factorysessions.LiveRuntime{},
			Handle:       &runtimebinding.SessionState{Owner: projectionOwnerStub{status: record.status}},
		}, record.isDefault)
	}

	projection, err := assembly.GetFactorySession(context.Background(), "session-second")
	if err != nil {
		t.Fatalf("GetFactorySession: %v", err)
	}
	if projection.Context.BackendScopeID != "backend-session-second" || projection.Runtime.LifecycleControlStatus == nil || *projection.Runtime.LifecycleControlStatus != "paused" {
		t.Fatalf("full projection was lost: %#v", projection)
	}

	got, err := assembly.Get(context.Background(), factorysessions.SessionGetRequest{Mode: factorysessions.SessionOperationModeLive, SessionID: "session-second"})
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Session.SessionID != "session-second" || got.Session.FactoryDir != "/factory/session-second" || !got.Session.RuntimeAvailable {
		t.Fatalf("canonical get lost live session data: %#v", got.Session)
	}

	listed, err := assembly.ListFactorySessions(context.Background())
	if err != nil {
		t.Fatalf("ListFactorySessions: %v", err)
	}
	if len(listed) != 2 || listed[0].Context.FactorySessionID != "session-first" || listed[1].Context.FactorySessionID != "session-second" {
		t.Fatalf("listed sessions = %#v", listed)
	}
	if !listed[0].RuntimeAvailable || !listed[1].RuntimeAvailable || listed[0].Context.BackendScopeID == listed[1].Context.BackendScopeID {
		t.Fatalf("list lost separate runtime projections: %#v", listed)
	}

	if _, err := assembly.GetFactorySession(context.Background(), "missing"); !errors.Is(err, factorysessions.ErrSessionNotFound) {
		t.Fatalf("missing session error = %v, want ErrSessionNotFound", err)
	}
}
