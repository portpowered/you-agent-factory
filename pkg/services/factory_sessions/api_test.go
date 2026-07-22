package factorysessions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

var factorySessionTestClock = platformclock.Real{}
var factorySessionResponseEventIdentity atomic.Uint64

func factorySessionTestResponseEventID() string {
	return fmt.Sprintf("response-event-test-%d", factorySessionResponseEventIdentity.Add(1))
}

func factorySessionTestID() string { return "00000000-0000-4000-8000-000000000001" }

func TestSessionErrorsMatchStableBoundarySentinels(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		domain error
		legacy error
	}{
		{name: "not found", domain: ErrNotFound, legacy: errors.New("factory session not found")},
		{name: "result unavailable", domain: ErrResultUnavailable, legacy: errors.New("factory session result unavailable")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if !errors.Is(fmt.Errorf("read session: %w", test.domain), test.legacy) {
				t.Fatalf("wrapped %v did not match stable boundary sentinel %v", test.domain, test.legacy)
			}
		})
	}
}

func TestNewSessionResponseEventStoreAlias(t *testing.T) {
	t.Parallel()

	store := NewSessionResponseEventStore("session-alias", factorySessionTestClock, factorySessionTestResponseEventID)
	if store == nil {
		t.Fatal("NewSessionResponseEventStore returned nil")
	}
	if got := store.FactorySessionID(); got != "session-alias" {
		t.Fatalf("FactorySessionID() = %q, want session-alias", got)
	}
}

func TestSubscribeFactoryResponseEvents_OwnsCursorDispatchKindAndOrderingPolicy(t *testing.T) {
	t.Parallel()

	store := NewSessionResponseEventStore("session-policy", factorySessionTestClock, factorySessionTestResponseEventID)
	publishResponseEventForSubscriptionTest(t, store, ResponseEventKindMessage, "dispatch-alpha")
	wantFirst := publishResponseEventForSubscriptionTest(t, store, ResponseEventKindProgress, "dispatch-alpha")
	publishResponseEventForSubscriptionTest(t, store, ResponseEventKindProgress, "dispatch-beta")
	wantSecond := publishResponseEventForSubscriptionTest(t, store, ResponseEventKindProgress, "dispatch-alpha")
	store.Complete()

	cursor, err := SubscribeFactoryResponseEvents(
		context.Background(),
		&LiveSession{ID: "session-policy", ResponseEvents: store},
		ResponseEventSubscriptionRequest{
			SessionID:     "session-policy",
			AfterSequence: 1,
			DispatchID:    "dispatch-alpha",
			Kinds:         []ResponseEventKind{ResponseEventKindProgress},
		},
	)
	if err != nil {
		t.Fatalf("SubscribeFactoryResponseEvents: %v", err)
	}
	defer cursor.Detach()

	events, err := cursor.Next(context.Background())
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if len(events) != 2 || events[0].Sequence != wantFirst.Sequence || events[1].Sequence != wantSecond.Sequence {
		t.Fatalf("events = %#v, want ordered progress sequences [%d %d]", events, wantFirst.Sequence, wantSecond.Sequence)
	}
}

func TestSubscribeFactoryResponseEvents_RejectsInvalidOwnerPolicyInputs(t *testing.T) {
	t.Parallel()

	session := &LiveSession{
		ID:             "session-policy-validation",
		ResponseEvents: NewSessionResponseEventStore("session-policy-validation", factorySessionTestClock, factorySessionTestResponseEventID),
	}
	tests := []struct {
		name    string
		request ResponseEventSubscriptionRequest
		want    error
	}{
		{
			name:    "negative reconnect sequence",
			request: ResponseEventSubscriptionRequest{SessionID: session.ID, AfterSequence: -1},
			want:    ErrInvalidResponseEventCursor,
		},
		{
			name: "unknown kind",
			request: ResponseEventSubscriptionRequest{
				SessionID: session.ID,
				Kinds:     []ResponseEventKind{"INTERNAL_ONLY"},
			},
			want: ErrInvalidResponseEventFilter,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := SubscribeFactoryResponseEvents(context.Background(), session, test.request)
			if !errors.Is(err, test.want) {
				t.Fatalf("SubscribeFactoryResponseEvents error = %v, want %v", err, test.want)
			}
		})
	}
}

func publishResponseEventForSubscriptionTest(
	t *testing.T,
	store *SessionResponseEventStore,
	kind ResponseEventKind,
	dispatchID string,
) FactoryResponseEvent {
	t.Helper()

	phase := ResponseEventPhaseDelta
	payload := json.RawMessage(`{"contentBlockIndex":0,"contentBlockKind":"TEXT","textDelta":"hello"}`)
	if kind == ResponseEventKindProgress {
		phase = ResponseEventPhaseUpdated
		payload = json.RawMessage(`{"label":"compile","message":"building"}`)
	}
	event, err := store.Publish(FactoryResponseEvent{
		RunID:      "run-policy",
		DispatchID: dispatchID,
		Kind:       kind,
		Phase:      phase,
		Provenance: ResponseEventProvenance{
			Provider:        "example-provider",
			NativeEventType: "fixture",
			Delivery:        ResponseEventDeliveryNativeStream,
			Representation:  ResponseEventRepresentationDelta,
			Fidelity:        ResponseEventFidelityLossless,
		},
		Payload: payload,
	})
	if err != nil {
		t.Fatalf("Publish(%s, %s): %v", kind, dispatchID, err)
	}
	return event
}

func TestNewLiveSessionOwnsCanonicalResponseEventStore(t *testing.T) {
	t.Parallel()

	session := NewLiveSession(
		DefaultSessionID,
		"/factories/default",
		"/workspace",
		"/workspace",
		TargetRef{Kind: TargetKindDefault},
		nil,
		true,
		"default",
		factorySessionTestClock,
		factorySessionTestID,
		factorySessionTestResponseEventID,
	)
	if session.ResponseEvents == nil {
		t.Fatal("ResponseEvents = nil, want session-owned store")
	}
	if got := session.ResponseEvents.FactorySessionID(); got != CanonicalFactorySessionID(session) {
		t.Fatalf("response event store session ID = %q, want %q", got, CanonicalFactorySessionID(session))
	}
}

func TestNewLiveSessionRequiresExplicitClock(t *testing.T) {
	t.Parallel()

	if session := NewLiveSession(
		"session-missing-clock",
		"/factories/default",
		"/workspace",
		"/workspace",
		TargetRef{Kind: TargetKindDefault},
		nil,
		true,
		"default",
		nil,
		factorySessionTestID,
		factorySessionTestResponseEventID,
	); session != nil {
		t.Fatalf("NewLiveSession without clock = %#v, want nil", session)
	}
}

func TestNewLiveSessionDefaultUUIDKeepsRegistryIdentity(t *testing.T) {
	t.Parallel()

	sessionID := factorySessionTestID()
	session := NewLiveSession(
		sessionID,
		"/factories/default",
		"/workspace",
		"/workspace",
		TargetRef{Kind: TargetKindDefault},
		nil,
		true,
		"default",
		factorySessionTestClock,
		factorySessionTestID,
		factorySessionTestResponseEventID,
	)
	if got := CanonicalFactorySessionID(session); got != sessionID {
		t.Fatalf("canonical session ID = %q, want registry UUID %q", got, sessionID)
	}
	if got := session.ResponseEvents.FactorySessionID(); got != sessionID {
		t.Fatalf("response event store session ID = %q, want registry UUID %q", got, sessionID)
	}
}

func TestBindResponseEventCompletion_UsesCanonicalFactoryEventTypes(t *testing.T) {
	t.Parallel()

	session := NewLiveSession(
		"session-completion",
		"/factories/default",
		"/workspace",
		"/workspace",
		TargetRef{Kind: TargetKindDefault},
		nil,
		true,
		"default",
		factorySessionTestClock,
		factorySessionTestID,
		factorySessionTestResponseEventID,
	)
	var recorder func(interfaces.FactoryEventType)
	BindResponseEventCompletion(session, func(bound func(interfaces.FactoryEventType)) {
		recorder = bound
	})
	if recorder == nil {
		t.Fatal("completion recorder = nil, want canonical Factory event callback")
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
