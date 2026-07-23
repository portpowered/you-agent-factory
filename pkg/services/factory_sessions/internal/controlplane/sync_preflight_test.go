package controlplane_test

import (
	"context"
	"testing"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/controlplane"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

func validateReconnectCursor(
	recorded []interfaces.FactoryEvent,
	cursor interfaces.FactoryEventReconnectCursor,
	scope interfaces.FactoryEventReconnectScope,
) error {
	_ = recorded
	_ = scope
	if cursor.AfterSequence != nil {
		return recordings.ErrReconnectCursorNotFound
	}
	return nil
}

type syncPreflightTestHost struct {
	target      controlplane.SyncPreflightTarget
	targetErr   error
	backendID   string
	streamGenID string
	events      []interfaces.FactoryEvent
}

func (h *syncPreflightTestHost) ResolveSyncPreflightTarget(_ string, _ *interfaces.FactorySessionLogicalResolveHint) (controlplane.SyncPreflightTarget, error) {
	return h.target, h.targetErr
}

func (h *syncPreflightTestHost) BackendScopeID() string {
	return h.backendID
}

func (h *syncPreflightTestHost) LogicalSessionKeyID(session *livesession.LiveSession) string {
	return controlplane.LogicalSessionKeyID(session)
}

func (h *syncPreflightTestHost) StreamGenerationID(_ *livesession.LiveSession) string {
	return h.streamGenID
}

func (h *syncPreflightTestHost) LiveSessionEvents(_ *livesession.LiveSession) []interfaces.FactoryEvent {
	return h.events
}

func TestGetLiveFactorySessionSyncPreflight_DurableSessionReturnsNotFound(t *testing.T) {
	t.Parallel()

	response, err := controlplane.GetLiveFactorySessionSyncPreflight(
		context.Background(),
		&syncPreflightTestHost{},
		"dur-sess-js-run-001",
		nil,
		nil,
		validateReconnectCursor,
	)
	if err != nil {
		t.Fatalf("GetLiveFactorySessionSyncPreflight: %v", err)
	}
	if response.Reason != factorysessions.SyncPreflightReasonSessionNotFound {
		t.Fatalf("reason = %q, want session_not_found", response.Reason)
	}
}

func TestGetLiveFactorySessionSyncPreflight_RemappedDefaultReturnsLogicalSessionRemap(t *testing.T) {
	t.Parallel()

	session := livesession.New("session-successor", "", "", "", factorysessions.TargetRef{}, nil, false, "", platformclock.Real{}, func() string { return "session-test-id" }, func() string { return "response-event-test-id" })
	host := &syncPreflightTestHost{
		target: controlplane.SyncPreflightTarget{
			Session:  session,
			Remapped: true,
		},
		backendID:   "runtime-1",
		streamGenID: "runtime-1::session-successor",
	}
	response, err := controlplane.GetLiveFactorySessionSyncPreflight(
		context.Background(),
		host,
		factorysessions.DefaultSessionID,
		nil,
		nil,
		validateReconnectCursor,
	)
	if err != nil {
		t.Fatalf("GetLiveFactorySessionSyncPreflight: %v", err)
	}
	if response.Reason != factorysessions.SyncPreflightReasonLogicalSessionRemap {
		t.Fatalf("reason = %q, want logical_session_remap", response.Reason)
	}
	if response.FactorySessionID == nil || *response.FactorySessionID != "session-successor" {
		t.Fatalf("factorySessionId = %#v, want session-successor", response.FactorySessionID)
	}
}

func TestGetLiveFactorySessionSyncPreflight_StaleCursorReturnsCursorStale(t *testing.T) {
	t.Parallel()

	session := livesession.New("session-live", "", "", "", factorysessions.TargetRef{}, nil, false, "", platformclock.Real{}, func() string { return "session-test-id" }, func() string { return "response-event-test-id" })
	host := &syncPreflightTestHost{
		target: controlplane.SyncPreflightTarget{Session: session},
		events: []interfaces.FactoryEvent{{Id: "evt-1", Context: interfaces.FactoryEventContext{Sequence: 1}}},
	}
	afterSequence := 99
	response, err := controlplane.GetLiveFactorySessionSyncPreflight(
		context.Background(),
		host,
		"session-live",
		&interfaces.FactoryEventReconnectCursor{AfterSequence: &afterSequence},
		nil,
		validateReconnectCursor,
	)
	if err != nil {
		t.Fatalf("GetLiveFactorySessionSyncPreflight: %v", err)
	}
	if response.Reason != factorysessions.SyncPreflightReasonCursorStale {
		t.Fatalf("reason = %q, want cursor_stale", response.Reason)
	}
}

func TestLogicalSessionKeyID_UsesFolderAndTargetIdentity(t *testing.T) {
	t.Parallel()

	key := controlplane.LogicalSessionKeyID(&livesession.LiveSession{
		SessionState: livesession.SessionState{
			FolderPath: "/tmp/demo",
		},
		Target: factorysessions.TargetRef{
			Kind: factorysessions.TargetKindNamed,
			Name: "beta",
		},
	})
	if key == "" {
		t.Fatal("LogicalSessionKeyID = empty, want stable key")
	}
}
