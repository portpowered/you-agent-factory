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
	service := sessionruntime.New(registry, responses, func(session *livesession.LiveSession) { closed = session.ID }, clock, func() string { return "response-event-test-id" }, func() string { return "session-test-id" })

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

func TestServiceRegisterDefaultPreservesCanonicalRuntimeID(t *testing.T) {
	t.Parallel()

	clock := platformclock.Real{}
	registry := sessionregistry.New()
	service := sessionruntime.New(
		registry,
		responsestream.NewRegistry(newRuntimeTestResponseStream, clock),
		nil,
		clock,
		func() string { return "response-event-test-id" },
		func() string { return "replacement-session-id" },
	)
	canonicalID := "550e8400-e29b-41d4-a716-446655440000"
	existing := livesession.NewWithRuntimeID(
		factorysessions.DefaultSessionID,
		"factory-dir",
		"folder-path",
		"execution-base",
		factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault},
		struct{}{},
		true,
		"project",
		clock,
		func() string { return "existing-session-id" },
		func() string { return "existing-response-event-id" },
		canonicalID,
	)
	if existing == nil {
		t.Fatal("construct existing default session")
	}
	existing.RetainedRuntimeMetricsSessionIDs = []string{canonicalID, "source-runtime-id"}
	registry.Upsert(existing, true)

	gotID := service.Register(sessionruntime.Registration{
		SessionID:         factorysessions.DefaultSessionID,
		Default:           true,
		AllocateDefaultID: true,
		Handle:            struct{}{},
	})
	if gotID != factorysessions.DefaultSessionID {
		t.Fatalf("replacement default ID = %q, want public alias %q", gotID, factorysessions.DefaultSessionID)
	}
	got := service.Default()
	if got == nil {
		t.Fatal("replacement default session is nil")
	}
	if got.RuntimeFactorySessionID != canonicalID {
		t.Fatalf("replacement canonical runtime ID = %q, want %q", got.RuntimeFactorySessionID, canonicalID)
	}
	if service.Resolve(canonicalID) != got || got.ResponseEvents == nil || got.ResponseEvents.FactorySessionID() != canonicalID {
		t.Fatalf("replacement canonical session resolution/events = (%v, %q), want canonical session", service.Resolve(canonicalID) == got, responseEventSessionID(got))
	}
	if len(got.RetainedRuntimeMetricsSessionIDs) != 2 ||
		got.RetainedRuntimeMetricsSessionIDs[0] != canonicalID ||
		got.RetainedRuntimeMetricsSessionIDs[1] != "source-runtime-id" {
		t.Fatalf("replacement retained metrics IDs = %#v, want successor and source lineage", got.RetainedRuntimeMetricsSessionIDs)
	}
}

func responseEventSessionID(session *livesession.LiveSession) string {
	if session == nil || session.ResponseEvents == nil {
		return ""
	}
	return session.ResponseEvents.FactorySessionID()
}
