package invocation

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
)

type invocationEventReaderStub struct {
	liveEvents     []factorydefinitions.FactoryEvent
	liveHistories  [][]factorydefinitions.FactoryEvent
	liveStream     <-chan factorydefinitions.FactoryEvent
	liveReads      int
	durableEvents  []factorydefinitions.FactoryEvent
	liveSession    string
	durableSession string
}

func (stub *invocationEventReaderStub) SubscribeFactoryEventsForSession(
	_ context.Context,
	sessionID string,
	_ *factorydefinitions.FactoryEventReconnectCursor,
) (*factorydefinitions.FactoryEventStream, error) {
	stub.liveSession = sessionID
	history := stub.liveEvents
	if stub.liveReads < len(stub.liveHistories) {
		history = stub.liveHistories[stub.liveReads]
	}
	stub.liveReads++
	events := stub.liveStream
	if events == nil {
		closed := make(chan factorydefinitions.FactoryEvent)
		close(closed)
		events = closed
	}
	return &factorydefinitions.FactoryEventStream{History: history, Events: events}, nil
}

func (stub *invocationEventReaderStub) ReadDurableFactorySessionEventStream(
	_ context.Context,
	sessionID string,
	_ factorysessions.EventReconnectRequest,
) (*factorydefinitions.FactoryEventStream, error) {
	stub.durableSession = sessionID
	return &factorydefinitions.FactoryEventStream{History: stub.durableEvents}, nil
}

func TestReadInvocationFactoryEvents_UsesLiveHistoryForDefaultSession(t *testing.T) {
	events := invocationFactoryEventFixtures()
	reader := &invocationEventReaderStub{liveEvents: events}

	got, err := readInvocationFactoryEvents(
		context.Background(), reader, factorydefinitions.FactoryInvocationResult{},
	)
	if err != nil {
		t.Fatalf("read live invocation events: %v", err)
	}
	assertInvocationFactoryEventOrder(t, got)
	if reader.liveSession != factorysessions.DefaultSessionID || reader.durableSession != "" {
		t.Fatalf("reader calls = live %q durable %q", reader.liveSession, reader.durableSession)
	}

	got[0].Payload[0] = '['
	if string(events[0].Payload) != `{}` {
		t.Fatal("returned live event payload aliases canonical history")
	}
}

func TestReadInvocationFactoryEvents_UsesDurableHistoryForJavaScriptSession(t *testing.T) {
	events := invocationFactoryEventFixtures()
	reader := &invocationEventReaderStub{durableEvents: events}

	got, err := readInvocationFactoryEvents(
		context.Background(), reader,
		factorydefinitions.FactoryInvocationResult{SessionID: "session-javascript"},
	)
	if err != nil {
		t.Fatalf("read durable invocation events: %v", err)
	}
	assertInvocationFactoryEventOrder(t, got)
	if reader.durableSession != "session-javascript" || reader.liveSession != "" {
		t.Fatalf("reader calls = live %q durable %q", reader.liveSession, reader.durableSession)
	}
}

func TestLiveInvocationFactoryEvents_PresentsBeforeTerminalAndReconcilesFinalHistory(t *testing.T) {
	events := invocationFactoryEventFixtures()
	terminal := events[1].Clone()
	terminal.Id = "event-terminal"
	terminal.Context.Sequence = 6
	terminal.Context.SessionSequence = intPointer(6)
	allEvents := append(append([]factorydefinitions.FactoryEvent(nil), events...), terminal)
	liveStream := make(chan factorydefinitions.FactoryEvent, 1)
	reader := &invocationEventReaderStub{
		liveHistories: [][]factorydefinitions.FactoryEvent{events[:1], allEvents},
		liveStream:    liveStream,
	}

	presented := make(chan factorydefinitions.FactoryEvent, len(allEvents))
	live, err := startLiveInvocationFactoryEvents(
		context.Background(), reader,
		func(batch []factorydefinitions.FactoryEvent) {
			for _, event := range batch {
				presented <- event
			}
		},
	)
	if err != nil {
		t.Fatalf("start live Factory Event consumption: %v", err)
	}
	assertPresentedInvocationEvent(t, presented, events[0])

	liveStream <- events[1]
	assertPresentedInvocationEvent(t, presented, events[1])
	close(liveStream)
	if err := live.finish(
		context.Background(), reader, factorydefinitions.FactoryInvocationResult{},
	); err != nil {
		t.Fatalf("finish live Factory Event consumption: %v", err)
	}
	assertPresentedInvocationEvent(t, presented, terminal)
	select {
	case duplicate := <-presented:
		t.Fatalf("duplicate event presented during final reconciliation: %#v", duplicate)
	default:
	}
}

func assertPresentedInvocationEvent(
	t *testing.T,
	presented <-chan factorydefinitions.FactoryEvent,
	want factorydefinitions.FactoryEvent,
) {
	t.Helper()
	select {
	case got := <-presented:
		if got.Id != want.Id || got.Context.Sequence != want.Context.Sequence {
			t.Fatalf("presented event = %#v, want %#v", got, want)
		}
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for Factory Event %q", want.Id)
	}
}

func intPointer(value int) *int { return &value }

func invocationFactoryEventFixtures() []factorydefinitions.FactoryEvent {
	sessionID := "session-javascript"
	types := []factorydefinitions.FactoryEventType{
		factorydefinitions.FactoryEventTypeOrchestratorPhaseChanged,
		factorydefinitions.FactoryEventTypeOrchestratorCheckpointWritten,
	}
	events := make([]factorydefinitions.FactoryEvent, len(types))
	for i, eventType := range types {
		sequence := i + 4
		events[i] = factorydefinitions.FactoryEvent{
			Id: "event-" + string(rune('a'+i)), SchemaVersion: factorydefinitions.FactoryEventSchemaVersionV1,
			Type: eventType, Payload: json.RawMessage(`{}`),
			Context: factorydefinitions.FactoryEventContext{
				EventTime: time.Unix(int64(sequence), 0).UTC(), Sequence: sequence,
				SessionID: &sessionID, SessionSequence: &sequence,
			},
		}
	}
	return events
}

func assertInvocationFactoryEventOrder(t *testing.T, events []factorydefinitions.FactoryEvent) {
	t.Helper()
	if len(events) != 2 {
		t.Fatalf("events = %d, want 2", len(events))
	}
	if events[0].Type != factorydefinitions.FactoryEventTypeOrchestratorPhaseChanged ||
		events[1].Type != factorydefinitions.FactoryEventTypeOrchestratorCheckpointWritten {
		t.Fatalf("event order = %s, %s", events[0].Type, events[1].Type)
	}
	if events[0].Context.SessionSequence == nil || *events[0].Context.SessionSequence != 4 ||
		events[1].Context.SessionSequence == nil || *events[1].Context.SessionSequence != 5 {
		t.Fatalf("session sequences = %#v, %#v", events[0].Context.SessionSequence, events[1].Context.SessionSequence)
	}
}
