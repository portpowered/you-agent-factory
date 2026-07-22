package factorysessionsse

import (
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestFactorySessionSSEFixture_RetainedHistoryIsStableAndOrdered(t *testing.T) {
	fixture := NewFactorySessionSSEFixture(t)
	if len(fixture.Retained) < 3 {
		t.Fatalf("retained events = %d, want at least 3", len(fixture.Retained))
	}

	wantIDs := []string{
		factorySessionSSEFixtureRetainedEventOneID,
		factorySessionSSEFixtureRetainedEventTwoID,
		factorySessionSSEFixtureRetainedEventThreeID,
	}
	for index, wantID := range wantIDs {
		got := fixture.Retained[index]
		if got.Id != wantID {
			t.Fatalf("retained[%d].id = %q, want %q", index, got.Id, wantID)
		}
		if got.Context.Sequence != index {
			t.Fatalf("retained[%d].context.sequence = %d, want %d", index, got.Context.Sequence, index)
		}
		if got.Context.SessionId == nil || *got.Context.SessionId != fixture.SessionID {
			t.Fatalf("retained[%d].context.sessionId = %#v, want %q", index, got.Context.SessionId, fixture.SessionID)
		}
	}
}

func TestFactorySessionSSEHarness_ReadsRetainedThenLiveEventsWithinTimeout(t *testing.T) {
	fixture := NewFactorySessionSSEFixture(t)
	server := httptest.NewServer(newAPITestServer(fixture.WorkAPI()).Handler())
	defer server.Close()

	harness := newFactorySessionSSEHarness(t, 2*time.Second)
	stream := harness.Open(server.URL, fixture.SessionID, "")
	defer stream.Close()

	retained := stream.ReadEvents(len(fixture.Retained))
	for index, want := range fixture.Retained {
		if retained[index].Id != want.Id {
			t.Fatalf("retained[%d].id = %q, want %q", index, retained[index].Id, want.Id)
		}
		if retained[index].Context.Sequence != want.Context.Sequence {
			t.Fatalf(
				"retained[%d].context.sequence = %d, want %d",
				index,
				retained[index].Context.Sequence,
				want.Context.Sequence,
			)
		}
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
}

func TestFactorySessionSSEHarness_FailsClosedWhenTimeoutElapses(t *testing.T) {
	fixture := NewFactorySessionSSEFixture(t)
	server := httptest.NewServer(newAPITestServer(fixture.WorkAPI()).Handler())
	defer server.Close()

	readTimeout := 200 * time.Millisecond
	harness := newFactorySessionSSEHarness(t, readTimeout)
	stream := harness.Open(server.URL, fixture.SessionID, "")
	defer stream.Close()

	_ = stream.ReadEvents(len(fixture.Retained))
	_, err := stream.TryReadNextEvent(readTimeout)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if !errors.Is(err, errFactorySessionSSEHarnessTimeout) {
		t.Fatalf("error = %v, want %v", err, errFactorySessionSSEHarnessTimeout)
	}
}

func TestFactorySessionSSEHarness_DecodesPublicFactoryEventRecords(t *testing.T) {
	fixture := NewFactorySessionSSEFixture(t)
	server := httptest.NewServer(newAPITestServer(fixture.WorkAPI()).Handler())
	defer server.Close()

	harness := newFactorySessionSSEHarness(t, 2*time.Second)
	stream := harness.Open(server.URL, fixture.SessionID, "")
	defer stream.Close()

	event := stream.ReadNextEvent()
	if event.SchemaVersion != factoryapi.AgentFactoryEventV1 {
		t.Fatalf("schemaVersion = %q, want agent-factory.event.v1", event.SchemaVersion)
	}
	if event.Type != factoryapi.FactoryEventTypeRunRequest {
		t.Fatalf("type = %q, want RUN_REQUEST", event.Type)
	}
}
