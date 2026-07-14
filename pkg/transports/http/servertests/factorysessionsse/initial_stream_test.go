package factorysessionsse

import (
	"net/http/httptest"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

const (
	factorySessionSSEBackendScopeHeader      = "X-Factory-Session-Backend-Scope-Id"
	factorySessionSSELogicalSessionKeyHeader = "X-Factory-Session-Logical-Session-Key-Id"
	factorySessionSSEFactorySessionHeader    = "X-Factory-Session-Factory-Session-Id"
	factorySessionSSEStreamGenerationHeader  = "X-Factory-Session-Stream-Generation-Id"
)

func TestFactorySessionSSEInitialStream_NoReconnectCursorReturnsEventStream(t *testing.T) {
	fixture := NewFactorySessionSSEFixture(t)
	server := httptest.NewServer(newAPITestServer(fixture.RootMockFactory()).Handler())
	defer server.Close()

	harness := NewFactorySessionSSEHarness(t, 2*time.Second)
	stream := harness.Open(server.URL, fixture.SessionID, "")
	defer stream.Close()

	if got := stream.Response.Header.Get("Content-Type"); got == "" {
		t.Fatal("Content-Type header is empty, want text/event-stream")
	}
}

func TestFactorySessionSSEInitialStream_DeliversRetainedHistoryAsValidFactoryEventsInOrder(t *testing.T) {
	fixture := NewFactorySessionSSEFixture(t)
	server := httptest.NewServer(newAPITestServer(fixture.RootMockFactory()).Handler())
	defer server.Close()

	harness := NewFactorySessionSSEHarness(t, 2*time.Second)
	stream := harness.Open(server.URL, fixture.SessionID, "")
	defer stream.Close()

	retained := stream.ReadEvents(len(fixture.Retained))
	seenIDs := make(map[string]struct{}, len(retained))
	var previousTick int
	var previousSequence int
	for index, event := range retained {
		if event.SchemaVersion != factoryapi.AgentFactoryEventV1 {
			t.Fatalf("retained[%d].schemaVersion = %q, want agent-factory.event.v1", index, event.SchemaVersion)
		}
		if event.Id == "" {
			t.Fatalf("retained[%d].id is empty, want public FactoryEvent id", index)
		}
		if _, duplicate := seenIDs[event.Id]; duplicate {
			t.Fatalf("retained[%d].id = %q duplicated on initial connection", index, event.Id)
		}
		seenIDs[event.Id] = struct{}{}

		want := fixture.Retained[index]
		if event.Id != want.Id {
			t.Fatalf("retained[%d].id = %q, want %q", index, event.Id, want.Id)
		}
		if event.Context.Sequence != want.Context.Sequence {
			t.Fatalf(
				"retained[%d].context.sequence = %d, want %d",
				index,
				event.Context.Sequence,
				want.Context.Sequence,
			)
		}
		if event.Context.Tick < previousTick {
			t.Fatalf(
				"retained[%d].context.tick = %d, want >= previous tick %d",
				index,
				event.Context.Tick,
				previousTick,
			)
		}
		if event.Context.Sequence < previousSequence {
			t.Fatalf(
				"retained[%d].context.sequence = %d, want >= previous sequence %d",
				index,
				event.Context.Sequence,
				previousSequence,
			)
		}
		previousTick = event.Context.Tick
		previousSequence = event.Context.Sequence
	}
}

func TestFactorySessionSSEInitialStream_ContinuesWithLiveEventWithoutRetainedReplay(t *testing.T) {
	fixture := NewFactorySessionSSEFixture(t)
	server := httptest.NewServer(newAPITestServer(fixture.RootMockFactory()).Handler())
	defer server.Close()

	harness := NewFactorySessionSSEHarness(t, 2*time.Second)
	stream := harness.Open(server.URL, fixture.SessionID, "")
	defer stream.Close()

	retained := stream.ReadEvents(len(fixture.Retained))
	retainedIDs := make(map[string]struct{}, len(retained))
	for _, event := range retained {
		retainedIDs[event.Id] = struct{}{}
	}

	live := fixture.LiveDispatchEvent(t)
	fixture.PublishLive(live)
	gotLive := stream.ReadNextEvent()
	if gotLive.Id != live.Id {
		t.Fatalf("live event id = %q, want %q", gotLive.Id, live.Id)
	}
	if gotLive.Context.Sequence != live.Context.Sequence {
		t.Fatalf("live event sequence = %d, want %d", gotLive.Context.Sequence, live.Context.Sequence)
	}
	if _, replayed := retainedIDs[gotLive.Id]; replayed {
		t.Fatalf("live event id %q was replayed from retained history", gotLive.Id)
	}

	_, err := stream.TryReadNextEvent(150 * time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout after live event, got another SSE frame")
	}
}

func TestFactorySessionSSEInitialStream_WritesSessionIdentityHandshakeHeaders(t *testing.T) {
	fixture := NewFactorySessionSSEFixture(t)
	server := httptest.NewServer(newAPITestServer(fixture.RootMockFactory()).Handler())
	defer server.Close()

	harness := NewFactorySessionSSEHarness(t, 2*time.Second)
	stream := harness.Open(server.URL, fixture.SessionID, "")
	defer stream.Close()

	assertFactorySessionSSEHandshakeHeader(
		t,
		stream.Response.Header.Get(factorySessionSSEBackendScopeHeader),
		factorySessionSSEBackendScopeHeader,
		factorySessionSSEFixtureBackendScopeID,
	)
	assertFactorySessionSSEHandshakeHeader(
		t,
		stream.Response.Header.Get(factorySessionSSELogicalSessionKeyHeader),
		factorySessionSSELogicalSessionKeyHeader,
		factorySessionSSEFixtureLogicalSessionKey,
	)
	assertFactorySessionSSEHandshakeHeader(
		t,
		stream.Response.Header.Get(factorySessionSSEFactorySessionHeader),
		factorySessionSSEFactorySessionHeader,
		fixture.SessionID,
	)
	assertFactorySessionSSEHandshakeHeader(
		t,
		stream.Response.Header.Get(factorySessionSSEStreamGenerationHeader),
		factorySessionSSEStreamGenerationHeader,
		factorySessionSSEFixtureStreamGenerationID,
	)
}

func assertFactorySessionSSEHandshakeHeader(t *testing.T, got, headerName, want string) {
	t.Helper()
	if got != want {
		t.Fatalf("%s = %q, want %q", headerName, got, want)
	}
}
