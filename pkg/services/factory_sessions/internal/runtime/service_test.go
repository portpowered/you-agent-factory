package runtime_test

import (
	"testing"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	sessionruntime "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/sessionregistry"
)

func newRuntimeTestResponseStream() *factorysessions.SessionResponseStream {
	return factorysessions.NewSessionResponseStream(platformclock.Real{})
}

func TestServiceRegisterResolveAndUnregister(t *testing.T) {
	registry := sessionregistry.New()
	closed := ""
	clock := platformclock.Real{}
	responses := factorysessions.NewResponseStreamRegistry(newRuntimeTestResponseStream, clock)
	service := sessionruntime.New(registry, responses, func(session *factorysessions.LiveSession) { closed = session.ID }, clock, func() string { return "response-event-test-id" }, func() string { return "session-test-id" })

	id := service.Register(sessionruntime.Registration{
		SessionID: factorysessions.DefaultSessionID, Handle: struct{}{}, Default: true,
		AllocateDefaultID: true, Select: true,
	})
	if id == "" || id == factorysessions.DefaultSessionID {
		t.Fatalf("default registration ID = %q, want allocated runtime ID", id)
	}
	if service.Current() != service.Default() || service.Resolve(factorysessions.DefaultSessionID) != service.Default() {
		t.Fatal("default and selected session did not resolve to one registration")
	}
	if service.Resolve(factorysessions.CanonicalFactorySessionID(service.Default())) != service.Default() {
		t.Fatal("canonical runtime ID did not resolve")
	}
	service.Unregister(factorysessions.DefaultSessionID)
	if closed != id || registry.Count() != 0 {
		t.Fatalf("unregister = (closed %q, count %d), want (%q, 0)", closed, registry.Count(), id)
	}
}
