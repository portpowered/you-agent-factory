package factorysessions_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
)

// peerResponseStreamFake exercises the published response-stream root slice
// through the singular Service. It compiles against only the Sessions root
// package and never imports factory_sessions/internal or private response-stream
// store/manager types. It does not publish a nested stream interface for peer
// import.
type peerResponseStreamFake struct {
	*peerRootServiceFake
	eventsBySession map[string][]factorysessions.ResponseStreamEvent
	staleCursors    map[string]bool
	closedSessions  map[string]bool
}

func newPeerResponseStreamFake() *peerResponseStreamFake {
	return &peerResponseStreamFake{
		peerRootServiceFake: newPeerRootServiceFake(),
		eventsBySession:     make(map[string][]factorysessions.ResponseStreamEvent),
		staleCursors:        make(map[string]bool),
		closedSessions:      make(map[string]bool),
	}
}

var _ factorysessions.Service = (*peerResponseStreamFake)(nil)

func (fake *peerResponseStreamFake) SubscribeFactoryResponseEvents(
	_ context.Context,
	request factorysessions.ResponseStreamSubscriptionRequest,
) (*factorysessions.ResponseStreamCursor, error) {
	sessionID := request.SessionID
	if fake.closedSessions[sessionID] {
		return nil, factorysessions.ErrResponseStreamSubscriptionClosed
	}
	if _, ok := fake.eventsBySession[sessionID]; !ok {
		return nil, factorysessions.ErrSessionNotFound
	}
	if fake.staleCursors[sessionID] || request.AfterSequence < 0 {
		return nil, factorysessions.ErrResponseStreamStaleCursor
	}

	after := request.AfterSequence
	detached := false
	nextBatch := func() ([]factorysessions.ResponseStreamEvent, error) {
		if detached || fake.closedSessions[sessionID] {
			return nil, factorysessions.ErrResponseStreamSubscriptionClosed
		}
		retained := fake.eventsBySession[sessionID]
		out := make([]factorysessions.ResponseStreamEvent, 0, len(retained))
		for _, event := range retained {
			if event.Sequence > after {
				out = append(out, event)
			}
		}
		return out, nil
	}

	return &factorysessions.ResponseStreamCursor{
		NextEvents: func(context.Context) ([]factorysessions.ResponseStreamEvent, error) {
			return nextBatch()
		},
		DrainEvents: func() ([]factorysessions.ResponseStreamEvent, error) {
			return nextBatch()
		},
		DetachCursor: func() {
			detached = true
		},
	}, nil
}

func TestResponseStreamRootContract_SubscriberReceivesOnlyNewerEvents(t *testing.T) {
	t.Parallel()

	fake := newPeerResponseStreamFake()
	sessionID := "sess-response-alpha"
	dispatchID := "dispatch-1"
	fake.eventsBySession[sessionID] = []factorysessions.ResponseStreamEvent{
		{
			FactorySessionID: sessionID,
			DispatchID:       dispatchID,
			EventID:          "evt-1",
			Sequence:         1,
			Kind:             factorysessions.ResponseEventKindProgress,
			Phase:            factorysessions.ResponseEventPhaseUpdated,
			SchemaVersion:    factorysessions.ResponseEventSchemaVersionV1,
			Payload:          json.RawMessage(`{}`),
		},
		{
			FactorySessionID: sessionID,
			DispatchID:       dispatchID,
			EventID:          "evt-2",
			Sequence:         2,
			Kind:             factorysessions.ResponseEventKindMessage,
			Phase:            factorysessions.ResponseEventPhaseDelta,
			SchemaVersion:    factorysessions.ResponseEventSchemaVersionV1,
			Payload:          json.RawMessage(`{}`),
		},
		{
			FactorySessionID: sessionID,
			DispatchID:       dispatchID,
			EventID:          "evt-3",
			Sequence:         3,
			Kind:             factorysessions.ResponseStreamCompletionKind,
			Phase:            factorysessions.ResponseStreamCompletionPhase,
			SchemaVersion:    factorysessions.ResponseEventSchemaVersionV1,
			Payload:          json.RawMessage(`{}`),
		},
	}

	var service factorysessions.Service = fake
	cursor, err := service.SubscribeFactoryResponseEvents(context.Background(), factorysessions.ResponseStreamSubscriptionRequest{
		SessionID:     sessionID,
		AfterSequence: 1,
		DispatchID:    dispatchID,
	})
	if err != nil {
		t.Fatalf("SubscribeFactoryResponseEvents: %v", err)
	}
	if cursor == nil {
		t.Fatal("SubscribeFactoryResponseEvents returned nil cursor")
	}

	events, err := cursor.Drain()
	if err != nil {
		t.Fatalf("Drain: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("Drain events = %#v, want only sequences > 1", events)
	}
	if events[0].Sequence != 2 || events[1].Sequence != 3 {
		t.Fatalf("Drain sequences = [%d %d], want [2 3]", events[0].Sequence, events[1].Sequence)
	}
	completion := events[1]
	if completion.Kind != factorysessions.ResponseStreamCompletionKind ||
		completion.Phase != factorysessions.ResponseStreamCompletionPhase {
		t.Fatalf("completion event = %#v, want published completion vocabulary", completion)
	}

	// Gap payload remains expressible on the published root slice without store types.
	gap := factorysessions.ResponseStreamGap{FirstAvailableSequence: 2, Reason: "compaction"}
	if gap.FirstAvailableSequence != 2 {
		t.Fatalf("ResponseStreamGap = %#v, want firstAvailableSequence=2", gap)
	}
}

func progressStreamEvent(sessionID, eventID string, sequence int64) factorysessions.ResponseStreamEvent {
	return factorysessions.ResponseStreamEvent{
		FactorySessionID: sessionID,
		EventID:          eventID,
		Sequence:         sequence,
		Kind:             factorysessions.ResponseEventKindProgress,
		Phase:            factorysessions.ResponseEventPhaseUpdated,
		SchemaVersion:    factorysessions.ResponseEventSchemaVersionV1,
		Payload:          json.RawMessage(`{}`),
	}
}

func seedGapStreamEvent(t *testing.T, fake *peerResponseStreamFake, sessionID string) {
	t.Helper()
	gapPayload, err := json.Marshal(factorysessions.ResponseStreamGap{
		FromSequence:           1,
		ToSequence:             3,
		FirstAvailableSequence: 4,
		Reason:                 "compaction",
	})
	if err != nil {
		t.Fatalf("marshal gap: %v", err)
	}
	fake.eventsBySession[sessionID] = []factorysessions.ResponseStreamEvent{{
		FactorySessionID: sessionID,
		EventID:          "evt-gap",
		Sequence:         4,
		Kind:             factorysessions.ResponseStreamKindGap,
		Phase:            factorysessions.ResponseEventPhaseUpdated,
		SchemaVersion:    factorysessions.ResponseEventSchemaVersionV1,
		Payload:          gapPayload,
	}}
}

func TestResponseStreamRootContract_TypedStaleCursorGapAndCancelFailures(t *testing.T) {
	t.Parallel()

	fake := newPeerResponseStreamFake()
	staleSession, gapSession, cancelSession := "sess-stale", "sess-gap", "sess-cancel"
	fake.eventsBySession[staleSession] = []factorysessions.ResponseStreamEvent{
		progressStreamEvent(staleSession, "evt-stale", 5),
	}
	fake.staleCursors[staleSession] = true
	seedGapStreamEvent(t, fake, gapSession)
	fake.eventsBySession[cancelSession] = []factorysessions.ResponseStreamEvent{
		progressStreamEvent(cancelSession, "evt-cancel", 1),
	}

	var service factorysessions.Service = fake
	_, err := service.SubscribeFactoryResponseEvents(context.Background(), factorysessions.ResponseStreamSubscriptionRequest{
		SessionID: staleSession, AfterSequence: 1,
	})
	if !errors.Is(err, factorysessions.ErrResponseStreamStaleCursor) {
		t.Fatalf("stale cursor = %v, want ErrResponseStreamStaleCursor", err)
	}
	if !errors.Is(err, factorysessions.ErrInvalidResponseEventCursor) {
		t.Fatalf("ErrResponseStreamStaleCursor must alias ErrInvalidResponseEventCursor, got %v", err)
	}

	cursor, err := service.SubscribeFactoryResponseEvents(context.Background(), factorysessions.ResponseStreamSubscriptionRequest{
		SessionID: gapSession, AfterSequence: 0,
	})
	if err != nil {
		t.Fatalf("gap SubscribeFactoryResponseEvents: %v", err)
	}
	events, err := cursor.Next(context.Background())
	if err != nil {
		t.Fatalf("gap Next: %v", err)
	}
	if len(events) != 1 || events[0].Kind != factorysessions.ResponseStreamKindGap {
		t.Fatalf("gap events = %#v, want one ResponseStreamKindGap outcome", events)
	}
	var gap factorysessions.ResponseStreamGap
	if err := json.Unmarshal(events[0].Payload, &gap); err != nil {
		t.Fatalf("unmarshal gap payload: %v", err)
	}
	if gap.FirstAvailableSequence != 4 || gap.FromSequence != 1 || gap.ToSequence != 3 {
		t.Fatalf("gap payload = %#v, want retention gap shape", gap)
	}

	cancelCursor, err := service.SubscribeFactoryResponseEvents(context.Background(), factorysessions.ResponseStreamSubscriptionRequest{
		SessionID: cancelSession, AfterSequence: 0,
	})
	if err != nil {
		t.Fatalf("cancel SubscribeFactoryResponseEvents: %v", err)
	}
	cancelCursor.Detach()
	_, err = cancelCursor.Next(context.Background())
	if !errors.Is(err, factorysessions.ErrResponseStreamSubscriptionClosed) {
		t.Fatalf("cancelled subscription = %v, want ErrResponseStreamSubscriptionClosed", err)
	}
	if !errors.Is(err, factorysessions.ErrResponseEventSubscriptionClosed) {
		t.Fatalf("ErrResponseStreamSubscriptionClosed must alias ErrResponseEventSubscriptionClosed, got %v", err)
	}
	if errors.Is(err, factorysessions.ErrResponseStreamStaleCursor) {
		t.Fatal("cancelled subscription must stay distinct from stale cursor")
	}
}
