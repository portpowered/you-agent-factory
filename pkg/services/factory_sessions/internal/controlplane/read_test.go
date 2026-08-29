package controlplane_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/controlplane"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
)

type readTestHost struct {
	sessionIDs      []string
	sessions        map[string]*livesession.LiveSession
	projectionErr   error
	requireSessionE error
	snapshot        *interfaces.EngineStateSnapshot[factoryruntime.PetriMarkingSnapshot, *factoryruntime.Net]
}

type defaultIdentityTestHost struct {
	*readTestHost
	session *livesession.LiveSession
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

func (h *defaultIdentityTestHost) LogicalSessionKeyID(session *livesession.LiveSession) string {
	return controlplane.LogicalSessionKeyID(session)
}

func (h *defaultIdentityTestHost) StreamGenerationID(*livesession.LiveSession) string {
	return "stream-default-identity-test"
}

func (h *defaultIdentityTestHost) LiveSessionEvents(*livesession.LiveSession) []interfaces.FactoryEvent {
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

func (h *readTestHost) GetLiveSession(sessionID string) *livesession.LiveSession {
	if h.sessions == nil {
		return nil
	}
	return h.sessions[sessionID]
}

func (h *readTestHost) RequireSession(sessionID string) (*livesession.LiveSession, error) {
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
	session *livesession.LiveSession,
) (factorysessions.ProjectionContext, error) {
	if h.projectionErr != nil {
		return factorysessions.ProjectionContext{}, h.projectionErr
	}
	return factorysessions.ProjectionContext{
		Session: &factorysessions.ScopedLiveSessionSummary{
			ID: livesession.CanonicalID(session), FactoryDir: session.FactoryDir,
			FolderPath: session.FolderPath, Project: session.Project,
			IsDefault: session.IsDefault, Target: session.Target,
		},
		FactorySessionID: livesession.CanonicalID(session), Snapshot: h.snapshot,
	}, nil
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
		sessions: map[string]*livesession.LiveSession{
			"beta": {
				ID: "beta",
				SessionState: livesession.SessionState{
					FactoryDir: "/tmp/beta",
				},
			},
			"~default": {
				ID:        "~default",
				IsDefault: true,
				SessionState: livesession.SessionState{
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
	if allocatedSessionID == factorysessions.DefaultSessionID || !livesession.IsUUIDID(allocatedSessionID) {
		t.Fatalf("allocated session id %q must be a UUID distinct from the default selector", allocatedSessionID)
	}

	defaultSession := &livesession.LiveSession{
		ID:                      factorysessions.DefaultSessionID,
		IsDefault:               true,
		RuntimeFactorySessionID: allocatedSessionID,
		RetainedRuntimeMetricsSessionIDs: []string{
			allocatedSessionID,
			"550e8400-e29b-41d4-a716-446655440000",
		},
		SessionState: livesession.SessionState{
			FactoryDir: "/tmp/default/factory",
			FolderPath: "/tmp/default",
		},
		Target: factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault},
	}
	host := &defaultIdentityTestHost{
		readTestHost: &readTestHost{
			sessionIDs: []string{factorysessions.DefaultSessionID},
			sessions: map[string]*livesession.LiveSession{
				factorysessions.DefaultSessionID: defaultSession,
			},
			snapshot: &interfaces.EngineStateSnapshot[factoryruntime.PetriMarkingSnapshot, *factoryruntime.Net]{
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
	if got := listed[0].Runtime.RetainedMetricsSessionIDs; len(got) != 2 ||
		got[0] != allocatedSessionID || got[1] != "550e8400-e29b-41d4-a716-446655440000" {
		t.Fatalf("listed retained metrics IDs = %#v, want successor/source lineage", got)
	}

	got, err := controlplane.GetLiveFactorySession(context.Background(), host, factorysessions.DefaultSessionID)
	if err != nil {
		t.Fatalf("GetLiveFactorySession(%q): %v", factorysessions.DefaultSessionID, err)
	}
	assertDefaultSessionDetailProjection(t, got.Context, allocatedSessionID)
	if retained := got.Runtime.RetainedMetricsSessionIDs; len(retained) != 2 ||
		retained[0] != allocatedSessionID || retained[1] != "550e8400-e29b-41d4-a716-446655440000" {
		t.Fatalf("detail retained metrics IDs = %#v, want successor/source lineage", retained)
	}

	preflight, err := controlplane.GetLiveFactorySessionSyncPreflight(
		context.Background(),
		host,
		factorysessions.DefaultSessionID,
		nil,
		nil,
		validateReconnectCursor,
	)
	if err != nil {
		t.Fatalf("GetLiveFactorySessionSyncPreflight(%q): %v", factorysessions.DefaultSessionID, err)
	}
	assertDefaultSessionPreflightIdentity(t, preflight, allocatedSessionID)
	assertResolvedSessionIDsAgree(t, listed, got.Context, preflight)
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
	if session := listed[0].Context.Session; listed[0].Context.FactorySessionID != allocatedSessionID || !session.IsDefault {
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
	if gotID := got.FactorySessionID; gotID != allocatedSessionID {
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
	listedID := listed[0].Context.FactorySessionID
	gotID := got.FactorySessionID
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
		sessions: map[string]*livesession.LiveSession{
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

func TestGetLiveFactorySession_PrefersRegisteredLiveDurablePrefixedID(t *testing.T) {
	t.Parallel()

	const sessionID = "dur-sess-live-owned-001"
	host := &readTestHost{
		sessions: map[string]*livesession.LiveSession{
			sessionID: {ID: sessionID},
		},
	}
	got, err := controlplane.GetLiveFactorySession(context.Background(), host, sessionID)
	if err != nil {
		t.Fatalf("GetLiveFactorySession: %v", err)
	}
	if got.Context.Session == nil || got.Context.Session.ID != sessionID {
		t.Fatalf("session = %#v, want live session %q", got.Context.Session, sessionID)
	}
}

func TestGetLiveFactorySession_ReturnsProjectedSession(t *testing.T) {
	t.Parallel()

	host := &readTestHost{
		sessions: map[string]*livesession.LiveSession{
			"sess-1": {
				ID: "sess-1",
				SessionState: livesession.SessionState{
					FactoryDir: "/tmp/factory",
				},
			},
		},
	}
	session, err := controlplane.GetLiveFactorySession(context.Background(), host, "sess-1")
	if err != nil {
		t.Fatalf("GetLiveFactorySession: %v", err)
	}
	if session.Context.Session.ID != "sess-1" {
		t.Fatalf("session id = %q, want sess-1", session.Context.Session.ID)
	}
}
