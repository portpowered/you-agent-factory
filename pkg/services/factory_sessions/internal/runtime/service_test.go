package runtime_test

import (
	"errors"
	"sync"
	"testing"
	"time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responseeventstore"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responsestream"
	sessionruntime "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtime"
	responseowner "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/response_stream"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/sessionregistry"
)

func newRuntimeTestResponseStream() *responsestream.SessionResponseStream {
	return responsestream.NewSessionResponseStream(platformclock.Real{})
}

func TestServiceRegistrationResponseEventsArmsAfterHistoricalReplay(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		historical []interfaces.FactoryEventType
	}{
		{name: "fresh session", historical: nil},
		{name: "one restored terminal", historical: []interfaces.FactoryEventType{
			interfaces.FactoryEventTypeSessionCompleted,
		}},
		{name: "multiple restored terminals", historical: []interfaces.FactoryEventType{
			interfaces.FactoryEventTypeSessionCompleted,
			interfaces.FactoryEventTypeSessionCompleted,
			interfaces.FactoryEventTypeSessionCompleted,
		}},
	}

	for _, testCase := range cases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
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
			var replayed []interfaces.FactoryEventType
			service.Register(sessionruntime.Registration{
				SessionID: "session-completion",
				Handle:    struct{}{},
				AddEventTypeRecorderWithReady: func(bound func(interfaces.FactoryEventType), ready func()) {
					recorder = bound
					for _, eventType := range testCase.historical {
						replayed = append(replayed, eventType)
						bound(eventType)
					}
					ready()
				},
			})
			session := service.Resolve("session-completion")
			if recorder == nil || session == nil || session.ResponseEvents == nil {
				t.Fatal("registration did not bind session-owned response-event completion")
			}
			if len(replayed) != len(testCase.historical) {
				t.Fatalf("replayed historical event count = %d, want %d", len(replayed), len(testCase.historical))
			}
			if session.ResponseEvents.Completed() {
				t.Fatal("restored terminal history completed the successor response events")
			}

			recorder(interfaces.FactoryEventTypeSessionResultUpdated)
			if session.ResponseEvents.Completed() {
				t.Fatal("response events completed for non-terminal Factory event")
			}
			recorder(interfaces.FactoryEventTypeSessionCompleted)
			if !session.ResponseEvents.Completed() {
				t.Fatal("response events remain live after successor SESSION_COMPLETED")
			}
		})
	}
}

func TestServiceRegistrationResponseEventsConcurrentLiveTailAfterAttachment(t *testing.T) {
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
	ready := make(chan struct{})
	allowAppend := make(chan struct{})
	registered := make(chan string, 1)
	go func() {
		registered <- service.Register(sessionruntime.Registration{
			SessionID: "session-concurrent-completion",
			Handle:    struct{}{},
			AddEventTypeRecorderWithReady: func(bound func(interfaces.FactoryEventType), arm func()) {
				recorder = bound
				bound(interfaces.FactoryEventTypeSessionCompleted)
				arm()
				close(ready)
				<-allowAppend
				// This append occurs after the source has armed live-tail delivery
				// but before the registrar returns to the caller.
				bound(interfaces.FactoryEventTypeSessionStarted)
				bound(interfaces.FactoryEventTypeSessionCompleted)
			},
		})
	}()

	select {
	case <-ready:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for live-tail arm")
	}
	close(allowAppend)

	var sessionID string
	select {
	case sessionID = <-registered:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for registration")
	}
	if sessionID == "" || recorder == nil {
		t.Fatal("concurrent registration did not return a recorder")
	}
	session := service.Resolve(sessionID)
	if session == nil || session.ResponseEvents == nil {
		t.Fatal("concurrent registration did not create response events")
	}
	if !session.ResponseEvents.Completed() {
		t.Fatal("terminal appended during the registration handoff did not complete response events")
	}

	const concurrentTerminals = 32
	start := make(chan struct{})
	var workers sync.WaitGroup
	workers.Add(concurrentTerminals)
	for range concurrentTerminals {
		go func() {
			defer workers.Done()
			<-start
			recorder(interfaces.FactoryEventTypeSessionCompleted)
		}()
	}
	close(start)
	workers.Wait()

	if !session.ResponseEvents.Completed() {
		t.Fatal("concurrent successor terminal observation did not complete response events")
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

type authorityResponses struct {
	responseowner.Service
	ids     []string
	clocks  []factoryruntime.Clock
	closed  []*responseeventstore.SessionResponseEventStore
	failure error
}

func (o *authorityResponses) NewEventStore(id string, clock factoryruntime.Clock) (*responseeventstore.SessionResponseEventStore, error) {
	o.ids = append(o.ids, id)
	o.clocks = append(o.clocks, clock)
	if o.failure != nil {
		return nil, o.failure
	}
	return responseeventstore.NewSessionResponseEventStore(id, clock, func() string { return "selected-event" }), nil
}
func (o *authorityResponses) Close(store *responseeventstore.SessionResponseEventStore) {
	o.closed = append(o.closed, store)
	store.Close()
}
func TestSessionAuthorityUsesInjectedRegistryAndResponseOwner(t *testing.T) {
	t.Parallel()
	registry := sessionregistry.New()
	clock := platformclock.Real{}
	responses := responsestream.NewRegistry(newRuntimeTestResponseStream, clock)
	owner := &authorityResponses{}
	state := sessionruntime.NewWithResponseService(registry, responses, nil, clock, func() string { return "event" }, func() string { return "selected-session" }, owner)
	id := state.Register(sessionruntime.Registration{SessionID: factorysessions.DefaultSessionID, Default: true, AllocateDefaultID: true, Handle: struct{}{}})
	session := state.Resolve(id)
	if id != "selected-session" || session == nil || registry.Get(id) != session || len(owner.ids) != 1 || owner.ids[0] != livesession.CanonicalID(session) || owner.clocks[0] != clock {
		t.Fatalf("registration did not use selected owners: id=%s, allocations=%v", id, owner.ids)
	}
	owner.failure = errors.New("store allocation failed")
	if got := state.Register(sessionruntime.Registration{SessionID: "failed", Handle: struct{}{}}); got != "" || registry.Get("failed") != nil {
		t.Fatalf("failed allocation registered %q", got)
	}
	state.Unregister(id)
	if registry.Get(id) != nil || len(owner.closed) != 1 || owner.closed[0] != session.ResponseEvents {
		t.Fatal("unregister did not retire selected response owner")
	}
}
func TestSessionAuthorityPreservesOptionalCloseAndResponseRetirement(t *testing.T) {
	t.Parallel()
	for _, withClose := range []bool{false, true} {
		clock := platformclock.Real{}
		registry := sessionregistry.New()
		responses := responsestream.NewRegistry(newRuntimeTestResponseStream, clock)
		owner := &authorityResponses{}
		calls := 0
		var closeSession func(*livesession.LiveSession)
		if withClose {
			closeSession = func(*livesession.LiveSession) { calls++ }
		}
		state := sessionruntime.NewWithResponseService(registry, responses, closeSession, clock, func() string { return "event" }, func() string { return "session" }, owner)
		state.Register(sessionruntime.Registration{SessionID: "session", Handle: struct{}{}})
		old := responses.Streams("session")
		state.RotateResponseStreams(state.Resolve("session"))
		if responses.Streams("session") == old {
			t.Fatal("rotation retained retired streams")
		}
		state.Unregister("session")
		if withClose && calls != 1 || !withClose && calls != 0 || len(owner.closed) != 2 {
			t.Fatalf("cleanup calls=%d response closes=%d", calls, len(owner.closed))
		}
		if _, err := responses.Streams("session").Subscribe("late", 0); err != responsestream.ErrSubscriptionClosed {
			t.Fatalf("retired stream error=%v", err)
		}
	}
}

func TestSessionAuthorityRetirementPreservesReplacementResponseStreams(t *testing.T) {
	t.Parallel()
	clock := platformclock.Real{}
	registry := sessionregistry.New()
	responses := responsestream.NewRegistry(newRuntimeTestResponseStream, clock)
	var state *sessionruntime.Service
	var replacement *livesession.LiveSession
	state = sessionruntime.New(registry, responses, func(old *livesession.LiveSession) {
		responses.Rotate(old.ID)
		state.Register(sessionruntime.Registration{SessionID: old.ID, Handle: struct{}{}})
		replacement = state.Resolve(old.ID)
		responses.Streams(old.ID)
	}, clock, func() string { return "event" }, func() string { return "session" })
	state.Register(sessionruntime.Registration{SessionID: "a", Handle: struct{}{}})
	old := state.Resolve("a")
	oldStreams := responses.Streams("a")
	if !state.UnregisterGeneration(old) {
		t.Fatal("current generation did not retire")
	}
	if replacement == nil || state.Resolve("a") != replacement || replacement.ResponseEvents.Completed() {
		t.Fatal("old cleanup removed or completed replacement response events")
	}
	if _, err := oldStreams.Subscribe("old", 0); !errors.Is(err, responsestream.ErrSubscriptionClosed) {
		t.Fatalf("retired stream subscription = %v", err)
	}
	if _, err := responses.Streams("a").Subscribe("next", 0); err != nil {
		t.Fatalf("replacement stream subscription = %v", err)
	}
	if state.UnregisterGeneration(old) || state.Resolve("a") != replacement {
		t.Fatal("stale retirement removed the replacement")
	}
	state.CloseResponseStreams(replacement)
}
