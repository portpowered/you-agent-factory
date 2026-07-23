package runtime_test

import (
	"testing"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responsestream"
	sessionruntime "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/sessionregistry"
)

func newRuntimeTestResponseStream() *responsestream.SessionResponseStream {
	return responsestream.NewSessionResponseStream(platformclock.Real{})
}

func TestServiceRegistrationCompletesResponseEventsFromCanonicalFactoryEvent(t *testing.T) {
	t.Parallel()

	clock := platformclock.Real{}
	service := sessionruntime.New(
		sessionregistry.New(),
		responsestream.NewRegistry(newRuntimeTestResponseStream, clock),
		nil,
		clock,
		func() string { return "response-event-test-id" },
		func() string { return "session-test-id" },
	)
	var recorder func(interfaces.FactoryEventType)
	service.Register(sessionruntime.Registration{
		SessionID: "session-completion",
		Handle:    struct{}{},
		AddEventTypeRecorder: func(bound func(interfaces.FactoryEventType)) {
			recorder = bound
		},
	})
	session := service.Resolve("session-completion")
	if recorder == nil || session == nil || session.ResponseEvents == nil {
		t.Fatal("registration did not bind session-owned response-event completion")
	}

	recorder(interfaces.FactoryEventTypeSessionResultUpdated)
	if session.ResponseEvents.Completed() {
		t.Fatal("response events completed for non-terminal Factory event")
	}
	recorder(interfaces.FactoryEventTypeSessionCompleted)
	if !session.ResponseEvents.Completed() {
		t.Fatal("response events remain live after SESSION_COMPLETED")
	}
}

func TestServiceRegisterResolveAndUnregister(t *testing.T) {
	registry := sessionregistry.New()
	closed := ""
	clock := platformclock.Real{}
	responses := responsestream.NewRegistry(newRuntimeTestResponseStream, clock)
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
	if service.Resolve(livesession.CanonicalID(service.Default())) != service.Default() {
		t.Fatal("canonical runtime ID did not resolve")
	}
	service.Unregister(factorysessions.DefaultSessionID)
	if closed != id || registry.Count() != 0 {
		t.Fatalf("unregister = (closed %q, count %d), want (%q, 0)", closed, registry.Count(), id)
	}
}
