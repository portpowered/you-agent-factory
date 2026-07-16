package controlplane_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factorysessions "github.com/portpowered/infinite-you/pkg/factory/sessions"
	"github.com/portpowered/infinite-you/pkg/factory/sessions/controlplane"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/orchestrators/petri"
)

type readTestHost struct {
	sessionIDs      []string
	sessions        map[string]*factorysessions.LiveSession
	projectionErr   error
	requireSessionE error
	snapshot        *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]
}

type defaultIdentityTestHost struct {
	*readTestHost
	session *factorysessions.LiveSession
}

func (h *defaultIdentityTestHost) ResolveSyncPreflightTarget(
	_ string,
	_ *interfaces.FactorySessionLogicalResolveHint,
) (controlplane.SyncPreflightTarget, error) {
	return controlplane.SyncPreflightTarget{Session: h.session}, nil
}

func (h *defaultIdentityTestHost) BackendScopeID() string {
	return "backend-default-identity-test"
}

func (h *defaultIdentityTestHost) StreamGenerationID(*factorysessions.LiveSession) string {
	return "stream-default-identity-test"
}

func (h *defaultIdentityTestHost) LiveSessionEvents(*factorysessions.LiveSession) []interfaces.FactoryEvent {
	return nil
}

func (h *readTestHost) DiscoverTargets(string) ([]factorysessions.Target, error) {
	return nil, nil
}

func (h *readTestHost) InitializeFactoryScaffold(string) error {
	return nil
}

func (h *readTestHost) OpenLiveSessionForTarget(context.Context, factorysessions.Target) (string, error) {
	return "", nil
}

func (h *readTestHost) ListLiveSessionIDs() []string {
	return h.sessionIDs
}

func (h *readTestHost) GetLiveSession(sessionID string) *factorysessions.LiveSession {
	if h.sessions == nil {
		return nil
	}
	return h.sessions[sessionID]
}

func (h *readTestHost) RequireSession(sessionID string) (*factorysessions.LiveSession, error) {
	if h.requireSessionE != nil {
		return nil, h.requireSessionE
	}
	session := h.GetLiveSession(sessionID)
	if session == nil {
		return nil, fmt.Errorf("%w: %s", factorysessions.ErrNotFound, sessionID)
	}
	return session, nil
}

func (h *readTestHost) BuildSessionProjectionContext(
	_ context.Context,
	session *factorysessions.LiveSession,
) (factorysessions.ProjectionContext, error) {
	if h.projectionErr != nil {
		return factorysessions.ProjectionContext{}, h.projectionErr
	}
	return factorysessions.ProjectionContext{Session: session, Snapshot: h.snapshot}, nil
}

func TestIsDurableExecutionSessionID(t *testing.T) {
	t.Parallel()

	if !controlplane.IsDurableExecutionSessionID("dur-sess-js-run-n-001") {
		t.Fatal("expected durable session id")
	}
	if controlplane.IsDurableExecutionSessionID("~default") {
		t.Fatal("did not expect live session id to be durable")
	}
}

func TestListLiveFactorySessions_OrdersDefaultFirst(t *testing.T) {
	t.Parallel()

	host := &readTestHost{
		sessionIDs: []string{"beta", "~default"},
		sessions: map[string]*factorysessions.LiveSession{
			"beta": {
				ID: "beta",
				SessionState: factorysessions.SessionState{
					FactoryDir: "/tmp/beta",
				},
			},
			"~default": {
				ID:        "~default",
				IsDefault: true,
				SessionState: factorysessions.SessionState{
					FactoryDir: "/tmp/default",
				},
			},
		},
	}
	response, err := controlplane.ListLiveFactorySessions(context.Background(), host)
	if err != nil {
		t.Fatalf("ListLiveFactorySessions: %v", err)
	}
	if len(response) != 2 {
		t.Fatalf("sessions = %d, want 2", len(response))
	}
	if session := response[0].Context.Session; !session.IsDefault || session.ID != "~default" {
		t.Fatalf("first session = %#v, want default first", session)
	}
}

func TestDefaultSessionSelectorResolvesConsistentRuntimeIdentity(t *testing.T) {
	t.Parallel()

	const allocatedSessionID = "11111111-1111-4111-8111-111111111111"
	if allocatedSessionID == factorysessions.DefaultSessionID || !factorysessions.IsUUIDFactorySessionID(allocatedSessionID) {
		t.Fatalf("allocated session id %q must be a UUID distinct from the default selector", allocatedSessionID)
	}

	defaultSession := &factorysessions.LiveSession{
		ID:                      factorysessions.DefaultSessionID,
		IsDefault:               true,
		RuntimeFactorySessionID: allocatedSessionID,
		SessionState: factorysessions.SessionState{
			FactoryDir: "/tmp/default/factory",
			FolderPath: "/tmp/default",
		},
		Target: factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault},
	}
	host := &defaultIdentityTestHost{
		readTestHost: &readTestHost{
			sessionIDs: []string{factorysessions.DefaultSessionID},
			sessions: map[string]*factorysessions.LiveSession{
				factorysessions.DefaultSessionID: defaultSession,
			},
			snapshot: &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
				Dispatches: map[string]*interfaces.DispatchEntry{
					"dispatch-underlying": {
						DispatchID:   "dispatch-underlying",
						TransitionID: "transition-underlying",
					},
				},
			},
		},
		session: defaultSession,
	}
	listed, err := controlplane.ListLiveFactorySessions(context.Background(), host)
	if err != nil {
		t.Fatalf("ListLiveFactorySessions: %v", err)
	}
	assertDefaultSessionListProjection(t, listed, allocatedSessionID)

	got, err := controlplane.GetLiveFactorySession(context.Background(), host, factorysessions.DefaultSessionID)
	if err != nil {
		t.Fatalf("GetLiveFactorySession(%q): %v", factorysessions.DefaultSessionID, err)
	}
	assertDefaultSessionDetailProjection(t, got, allocatedSessionID)

	preflight, err := controlplane.GetLiveFactorySessionSyncPreflight(
		context.Background(),
		host,
		factorysessions.DefaultSessionID,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("GetLiveFactorySessionSyncPreflight(%q): %v", factorysessions.DefaultSessionID, err)
	}
	assertDefaultSessionPreflightIdentity(t, preflight, allocatedSessionID)
	assertResolvedSessionIDsAgree(t, listed, got, preflight)
}

func assertDefaultSessionListProjection(
	t *testing.T,
	listed []factorysessions.ReadProjection,
	allocatedSessionID string,
) {
	t.Helper()
	if len(listed) != 1 {
		t.Fatalf("listed sessions = %d, want 1", len(listed))
	}
	if session := listed[0].Context.Session; factorysessions.CanonicalFactorySessionID(session) != allocatedSessionID || !session.IsDefault {
		t.Fatalf("listed default session = %#v, want id %q and isDefault true", session, allocatedSessionID)
	}
	if !listed[0].RuntimeAvailable {
		t.Fatal("listed runtime is unavailable")
	}
}

func assertDefaultSessionDetailProjection(
	t *testing.T,
	got factorysessions.ProjectionContext,
	allocatedSessionID string,
) {
	t.Helper()
	if gotID := factorysessions.CanonicalFactorySessionID(got.Session); gotID != allocatedSessionID {
		t.Fatalf("get-by-alias session id = %q, want %q", gotID, allocatedSessionID)
	}
}

func assertDefaultSessionPreflightIdentity(
	t *testing.T,
	preflight factorysessions.SyncPreflightResult,
	allocatedSessionID string,
) {
	t.Helper()
	if preflight.RequestedSessionID != factorysessions.DefaultSessionID {
		t.Fatalf("requestedSessionId = %q, want %q", preflight.RequestedSessionID, factorysessions.DefaultSessionID)
	}
	if preflight.FactorySessionID == nil || *preflight.FactorySessionID != allocatedSessionID {
		t.Fatalf("factorySessionId = %#v, want %q", preflight.FactorySessionID, allocatedSessionID)
	}
}

func assertResolvedSessionIDsAgree(
	t *testing.T,
	listed []factorysessions.ReadProjection,
	got factorysessions.ProjectionContext,
	preflight factorysessions.SyncPreflightResult,
) {
	t.Helper()
	listedID := factorysessions.CanonicalFactorySessionID(listed[0].Context.Session)
	gotID := factorysessions.CanonicalFactorySessionID(got.Session)
	if listedID != gotID || gotID != *preflight.FactorySessionID {
		t.Fatalf(
			"resolved ids differ: list=%q get=%q preflight=%q",
			listedID,
			gotID,
			*preflight.FactorySessionID,
		)
	}
}

func TestListLiveFactorySessions_FallsBackWhenProjectionFails(t *testing.T) {
	t.Parallel()

	host := &readTestHost{
		sessionIDs: []string{"sess-1"},
		sessions: map[string]*factorysessions.LiveSession{
			"sess-1": {ID: "sess-1"},
		},
		projectionErr: errors.New("projection unavailable"),
	}
	response, err := controlplane.ListLiveFactorySessions(context.Background(), host)
	if err != nil {
		t.Fatalf("ListLiveFactorySessions: %v", err)
	}
	if len(response) != 1 || response[0].Context.Session.ID != "sess-1" {
		t.Fatalf("sessions = %#v, want sess-1 summary fallback", response)
	}
	if response[0].RuntimeAvailable {
		t.Fatal("runtime available = true, want summary fallback")
	}
}

func TestGetLiveFactorySession_RejectsDurableIDs(t *testing.T) {
	t.Parallel()

	_, err := controlplane.GetLiveFactorySession(context.Background(), &readTestHost{}, "dur-sess-js-run-n-001")
	if err == nil || !errors.Is(err, factorysessions.ErrNotFound) {
		t.Fatalf("GetLiveFactorySession error = %v, want not found", err)
	}
}

func TestGetLiveFactorySession_ReturnsProjectedSession(t *testing.T) {
	t.Parallel()

	host := &readTestHost{
		sessions: map[string]*factorysessions.LiveSession{
			"sess-1": {
				ID: "sess-1",
				SessionState: factorysessions.SessionState{
					FactoryDir: "/tmp/factory",
				},
			},
		},
	}
	session, err := controlplane.GetLiveFactorySession(context.Background(), host, "sess-1")
	if err != nil {
		t.Fatalf("GetLiveFactorySession: %v", err)
	}
	if session.Session.ID != "sess-1" {
		t.Fatalf("session id = %q, want sess-1", session.Session.ID)
	}
}
