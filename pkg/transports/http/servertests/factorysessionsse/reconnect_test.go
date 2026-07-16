package factorysessionsse

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestFactorySessionSSEReconnect_AfterEventIDSkipsAcknowledgedRetainedHistory(t *testing.T) {
	fixture := NewFactorySessionSSEFixture(t)
	server := httptest.NewServer(newAPITestServer(fixture.RootMockFactory()).Handler())
	defer server.Close()

	acknowledged := fixture.Retained[1]
	wantSuffix := []string{fixture.Retained[2].Id}

	harness := NewFactorySessionSSEHarness(t, 2*time.Second)
	stream := harness.OpenFromCheckpoint(server.URL, fixture.SessionID, FactorySessionSSECheckpoint{
		AfterEventID: acknowledged.Id,
	})
	defer stream.Close()

	assertFactorySessionSSEReconnectSuffix(t, stream, wantSuffix, []string{
		fixture.Retained[0].Id,
		acknowledged.Id,
	})
}

func TestFactorySessionSSEReconnect_AfterSequenceSkipsAcknowledgedRetainedHistory(t *testing.T) {
	fixture := NewFactorySessionSSEFixture(t)
	server := httptest.NewServer(newAPITestServer(fixture.RootMockFactory()).Handler())
	defer server.Close()

	acknowledgedSequence := 1
	wantSuffix := []string{fixture.Retained[2].Id}

	harness := NewFactorySessionSSEHarness(t, 2*time.Second)
	stream := harness.OpenFromCheckpoint(server.URL, fixture.SessionID, FactorySessionSSECheckpoint{
		AfterSequence: &acknowledgedSequence,
	})
	defer stream.Close()

	assertFactorySessionSSEReconnectSuffix(t, stream, wantSuffix, []string{
		fixture.Retained[0].Id,
		fixture.Retained[1].Id,
	})
	if got := fixture.Retained[1].Context.SessionSequence; got == nil || *got != acknowledgedSequence {
		t.Fatalf("fixture retained[1].context.sessionSequence = %#v, want %d", got, acknowledgedSequence)
	}
}

func TestFactorySessionSSEReconnect_AfterSequenceFallsBackToCanonicalSequence(t *testing.T) {
	fixture := NewFactorySessionSSEFixture(t)
	fixture.Retained[1].Context.SessionSequence = nil
	server := httptest.NewServer(newAPITestServer(fixture.RootMockFactory()).Handler())
	defer server.Close()

	acknowledgedSequence := fixture.Retained[1].Context.Sequence
	harness := NewFactorySessionSSEHarness(t, 2*time.Second)
	stream := harness.OpenFromCheckpoint(server.URL, fixture.SessionID, FactorySessionSSECheckpoint{
		AfterSequence: &acknowledgedSequence,
	})
	defer stream.Close()

	assertFactorySessionSSEReconnectSuffix(t, stream, []string{fixture.Retained[2].Id}, []string{
		fixture.Retained[0].Id,
		fixture.Retained[1].Id,
	})
}

func TestFactorySessionSSEReconnect_AfterEventIDTakesPrecedenceOverSequence(t *testing.T) {
	fixture := NewFactorySessionSSEFixture(t)
	server := httptest.NewServer(newAPITestServer(fixture.RootMockFactory()).Handler())
	defer server.Close()

	conflictingSequence := *fixture.Retained[1].Context.SessionSequence
	harness := NewFactorySessionSSEHarness(t, 2*time.Second)
	stream := harness.OpenFromCheckpoint(server.URL, fixture.SessionID, FactorySessionSSECheckpoint{
		AfterEventID:  fixture.Retained[0].Id,
		AfterSequence: &conflictingSequence,
	})
	defer stream.Close()

	assertFactorySessionSSEReconnectSuffix(t, stream, []string{
		fixture.Retained[1].Id,
		fixture.Retained[2].Id,
	}, []string{fixture.Retained[0].Id})
}

func TestFactorySessionSSEReconnect_SecondConnectFromSameCursorYieldsDeterministicSuffix(t *testing.T) {
	fixture := NewFactorySessionSSEFixture(t)
	server := httptest.NewServer(newAPITestServer(fixture.RootMockFactory()).Handler())
	defer server.Close()

	wantSuffix := []string{fixture.Retained[2].Id}
	checkpoint := FactorySessionSSECheckpoint{AfterEventID: fixture.Retained[1].Id}

	firstSuffix := readFactorySessionSSEReconnectSuffix(t, server.URL, fixture.SessionID, checkpoint)
	secondSuffix := readFactorySessionSSEReconnectSuffix(t, server.URL, fixture.SessionID, checkpoint)
	if len(firstSuffix) != len(wantSuffix) || len(secondSuffix) != len(wantSuffix) {
		t.Fatalf(
			"reconnect suffix lengths = [%d %d], want %d each",
			len(firstSuffix),
			len(secondSuffix),
			len(wantSuffix),
		)
	}
	for index, wantID := range wantSuffix {
		if firstSuffix[index].Id != wantID {
			t.Fatalf("first reconnect[%d].id = %q, want %q", index, firstSuffix[index].Id, wantID)
		}
		if secondSuffix[index].Id != wantID {
			t.Fatalf("second reconnect[%d].id = %q, want %q", index, secondSuffix[index].Id, wantID)
		}
	}
}

func TestFactorySessionSSEReconnect_KeepsEventStreamFramingAndTargetSession(t *testing.T) {
	fixture := NewFactorySessionSSEFixture(t)
	otherSessionID := "b08-sse-other-session"
	otherEventID := "b08-sse-other-session/only-event"
	root := fixture.RootMockFactory()
	otherSessionIDCopy := otherSessionID
	root.SessionFactories[otherSessionID] = &testutil.MockFactory{
		FactoryEventStream: &interfaces.FactoryEventStream{
			StreamGenerationID:  "b08-sse-other-stream-gen",
			BackendScopeID:      "b08-sse-other-backend-scope",
			LogicalSessionKeyID: "b08-sse-other-logical-key",
			FactorySessionID:    otherSessionID,
			History: []interfaces.FactoryEvent{
				testutil.FactoryEvent(t, testAPIFactoryEvent(
					t,
					factoryapi.FactoryEventTypeRunRequest,
					otherEventID,
					factoryapi.FactoryEventContext{
						Tick:            0,
						Sequence:        0,
						SessionSequence: factorySessionSSESessionSequencePointer(0),
						EventTime:       factorySessionSSEFixtureEventTime,
						SessionId:       &otherSessionIDCopy,
					},
					factoryapi.RunRequestEventPayload{
						RecordedAt: factorySessionSSEFixtureEventTime,
						Factory:    factoryapi.Factory{Name: "other-session-factory"},
					},
				)),
			},
			Events: make(chan interfaces.FactoryEvent, 1),
		},
	}

	server := httptest.NewServer(newAPITestServer(root).Handler())
	defer server.Close()

	harness := NewFactorySessionSSEHarness(t, 2*time.Second)
	stream := harness.OpenFromCheckpoint(server.URL, fixture.SessionID, FactorySessionSSECheckpoint{
		AfterEventID: fixture.Retained[0].Id,
	})
	defer stream.Close()

	assertFactorySessionSSEHandshakeHeader(
		t,
		stream.Response.Header.Get(factorySessionSSEBackendScopeHeader),
		factorySessionSSEBackendScopeHeader,
		factorySessionSSEFixtureBackendScopeID,
	)
	assertFactorySessionSSEHandshakeHeader(
		t,
		stream.Response.Header.Get(factorySessionSSEFactorySessionHeader),
		factorySessionSSEFactorySessionHeader,
		fixture.SessionID,
	)

	wantSuffix := []string{fixture.Retained[1].Id, fixture.Retained[2].Id}
	assertFactorySessionSSEReconnectSuffix(t, stream, wantSuffix, []string{otherEventID})
}

func readFactorySessionSSEReconnectSuffix(
	t *testing.T,
	serverURL, sessionID string,
	checkpoint FactorySessionSSECheckpoint,
) []factoryapi.FactoryEvent {
	t.Helper()

	harness := NewFactorySessionSSEHarness(t, 2*time.Second)
	stream := harness.OpenFromCheckpoint(serverURL, sessionID, checkpoint)
	defer stream.Close()

	events := make([]factoryapi.FactoryEvent, 0, 4)
	for {
		event, err := stream.TryReadNextEvent(300 * time.Millisecond)
		if err != nil {
			break
		}
		events = append(events, event)
	}
	return events
}

func assertFactorySessionSSEReconnectSuffix(
	t *testing.T,
	stream *FactorySessionSSEStream,
	wantSuffix, forbiddenIDs []string,
) {
	t.Helper()

	forbidden := make(map[string]struct{}, len(forbiddenIDs))
	for _, id := range forbiddenIDs {
		forbidden[id] = struct{}{}
	}

	got := make([]factoryapi.FactoryEvent, 0, len(wantSuffix))
	for len(got) < len(wantSuffix) {
		got = append(got, stream.ReadNextEvent())
	}
	for index, wantID := range wantSuffix {
		if got[index].Id != wantID {
			t.Fatalf("reconnect suffix[%d].id = %q, want %q", index, got[index].Id, wantID)
		}
		if _, replayed := forbidden[got[index].Id]; replayed {
			t.Fatalf("reconnect suffix[%d].id = %q replayed forbidden acknowledged history", index, got[index].Id)
		}
	}
	for _, event := range got {
		if _, replayed := forbidden[event.Id]; replayed {
			t.Fatalf("reconnect replayed forbidden event id %q", event.Id)
		}
	}

	_, err := stream.TryReadNextEvent(150 * time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout after reconnect suffix, got another SSE frame")
	}
}
