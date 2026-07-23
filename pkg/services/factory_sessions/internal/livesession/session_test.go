package livesession_test

import (
	"fmt"
	"sync/atomic"
	"testing"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
)

var responseEventIdentity atomic.Uint64

func responseEventID() string {
	return fmt.Sprintf("response-event-test-%d", responseEventIdentity.Add(1))
}

func sessionID() string { return "00000000-0000-4000-8000-000000000001" }

func TestNewOwnsCanonicalResponseEventStore(t *testing.T) {
	t.Parallel()

	session := newSession(t, factorysessions.DefaultSessionID, platformclock.Real{})
	if session.ResponseEvents == nil {
		t.Fatal("ResponseEvents = nil, want session-owned store")
	}
	if got := session.ResponseEvents.FactorySessionID(); got != factorysessions.CanonicalFactorySessionID(session) {
		t.Fatalf("response event store session ID = %q, want %q", got, factorysessions.CanonicalFactorySessionID(session))
	}
}

func TestNewRequiresExplicitClock(t *testing.T) {
	t.Parallel()

	if session := livesession.New(
		"session-missing-clock", "/factories/default", "/workspace", "/workspace",
		factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault}, nil, true, "default",
		nil, sessionID, responseEventID,
	); session != nil {
		t.Fatalf("New without clock = %#v, want nil", session)
	}
}

func TestNewUUIDKeepsRegistryIdentity(t *testing.T) {
	t.Parallel()

	want := sessionID()
	session := newSession(t, want, platformclock.Real{})
	if got := factorysessions.CanonicalFactorySessionID(session); got != want {
		t.Fatalf("canonical session ID = %q, want registry UUID %q", got, want)
	}
	if got := session.ResponseEvents.FactorySessionID(); got != want {
		t.Fatalf("response event store session ID = %q, want registry UUID %q", got, want)
	}
}

func newSession(t *testing.T, id string, clock factoryruntime.Clock) *factorysessions.LiveSession {
	t.Helper()
	session := livesession.New(
		id, "/factories/default", "/workspace", "/workspace",
		factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault}, nil, true, "default",
		clock, sessionID, responseEventID,
	)
	if session == nil {
		t.Fatal("New returned nil")
	}
	return session
}
