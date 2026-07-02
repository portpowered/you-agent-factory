package controlplane_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/portpowered/infinite-you/pkg/apisurface"
	"github.com/portpowered/infinite-you/pkg/factorysessions"
	"github.com/portpowered/infinite-you/pkg/factorysessions/controlplane"
)

type readTestHost struct {
	sessionIDs      []string
	sessions        map[string]*factorysessions.LiveSession
	projectionErr   error
	requireSessionE error
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
		return nil, fmt.Errorf("%w: %s", apisurface.ErrFactorySessionNotFound, sessionID)
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
	return factorysessions.ProjectionContext{Session: session}, nil
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
	if len(response.Sessions) != 2 {
		t.Fatalf("sessions = %d, want 2", len(response.Sessions))
	}
	if !response.Sessions[0].IsDefault || response.Sessions[0].Id != "~default" {
		t.Fatalf("first session = %#v, want default first", response.Sessions[0])
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
	if len(response.Sessions) != 1 || response.Sessions[0].Id != "sess-1" {
		t.Fatalf("sessions = %#v, want sess-1 summary fallback", response.Sessions)
	}
	if response.Sessions[0].Runtime != nil {
		t.Fatalf("runtime = %#v, want nil fallback summary", response.Sessions[0].Runtime)
	}
}

func TestGetLiveFactorySession_RejectsDurableIDs(t *testing.T) {
	t.Parallel()

	_, err := controlplane.GetLiveFactorySession(context.Background(), &readTestHost{}, "dur-sess-js-run-n-001")
	if err == nil || !errors.Is(err, apisurface.ErrFactorySessionNotFound) {
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
	if session.Id != "sess-1" {
		t.Fatalf("session id = %q, want sess-1", session.Id)
	}
}
