package controlplane_test

import (
	"context"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/factorysessions"
	"github.com/portpowered/infinite-you/pkg/factorysessions/controlplane"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

type syncPreflightTestHost struct {
	target      controlplane.SyncPreflightTarget
	targetErr   error
	backendID   string
	streamGenID string
	events      []factoryapi.FactoryEvent
}

func (h *syncPreflightTestHost) ResolveSyncPreflightTarget(_ string) (controlplane.SyncPreflightTarget, error) {
	return h.target, h.targetErr
}

func (h *syncPreflightTestHost) BackendScopeID() string {
	return h.backendID
}

func (h *syncPreflightTestHost) StreamGenerationID(_ *factorysessions.LiveSession) string {
	return h.streamGenID
}

func (h *syncPreflightTestHost) LiveSessionEvents(_ *factorysessions.LiveSession) []factoryapi.FactoryEvent {
	return h.events
}

func TestGetLiveFactorySessionSyncPreflight_DurableSessionReturnsNotFound(t *testing.T) {
	t.Parallel()

	response, err := controlplane.GetLiveFactorySessionSyncPreflight(
		context.Background(),
		&syncPreflightTestHost{},
		"dur-sess-js-run-001",
		nil,
	)
	if err != nil {
		t.Fatalf("GetLiveFactorySessionSyncPreflight: %v", err)
	}
	if response.ReasonCode != factoryapi.SessionNotFound {
		t.Fatalf("reasonCode = %q, want session_not_found", response.ReasonCode)
	}
}

func TestGetLiveFactorySessionSyncPreflight_RemappedDefaultReturnsLogicalSessionRemap(t *testing.T) {
	t.Parallel()

	session := factorysessions.NewLiveSession("session-successor", "", "", "", factorysessions.TargetRef{}, nil, false, "")
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
	)
	if err != nil {
		t.Fatalf("GetLiveFactorySessionSyncPreflight: %v", err)
	}
	if response.ReasonCode != factoryapi.LogicalSessionRemap {
		t.Fatalf("reasonCode = %q, want logical_session_remap", response.ReasonCode)
	}
	if response.FactorySessionId == nil || *response.FactorySessionId != "session-successor" {
		t.Fatalf("factorySessionId = %#v, want session-successor", response.FactorySessionId)
	}
}

func TestGetLiveFactorySessionSyncPreflight_StaleCursorReturnsCursorStale(t *testing.T) {
	t.Parallel()

	session := factorysessions.NewLiveSession("session-live", "", "", "", factorysessions.TargetRef{}, nil, false, "")
	host := &syncPreflightTestHost{
		target: controlplane.SyncPreflightTarget{Session: session},
		events: []factoryapi.FactoryEvent{{Id: "evt-1", Context: factoryapi.FactoryEventContext{Sequence: 1}}},
	}
	afterSequence := 99
	response, err := controlplane.GetLiveFactorySessionSyncPreflight(
		context.Background(),
		host,
		"session-live",
		&interfaces.FactoryEventReconnectCursor{AfterSequence: &afterSequence},
	)
	if err != nil {
		t.Fatalf("GetLiveFactorySessionSyncPreflight: %v", err)
	}
	if response.ReasonCode != factoryapi.CursorStale {
		t.Fatalf("reasonCode = %q, want cursor_stale", response.ReasonCode)
	}
}
