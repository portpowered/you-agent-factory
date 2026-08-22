package events

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/recordings/internal/projections"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestFactoryEventHistory_Subscribe_InvalidReconnectCursorDoesNotRegisterStream(t *testing.T) {
	history := newTestFactoryEventHistory(eventHistoryProjectionNet(), func() time.Time { return time.Unix(0, 0).UTC() })
	history.RecordInitialStructure()

	_, err := history.Subscribe(context.Background(), &interfaces.FactoryEventReconnectCursor{
		AfterEventID: "factory-event/missing",
	}, interfaces.FactoryEventReconnectScope{})
	if !errors.Is(err, ErrReconnectCursorNotFound) {
		t.Fatalf("Subscribe error = %v, want %v", err, ErrReconnectCursorNotFound)
	}

	history.mu.RLock()
	streamCount := len(history.streams)
	history.mu.RUnlock()
	if streamCount != 0 {
		t.Fatalf("stream count = %d, want 0 after invalid reconnect cursor", streamCount)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stream, err := history.Subscribe(ctx, nil, interfaces.FactoryEventReconnectScope{})
	if err != nil {
		t.Fatalf("valid Subscribe: %v", err)
	}

	history.RecordFactoryStateChange(1, interfaces.FactoryStateIdle, interfaces.FactoryStateRunning, "after-invalid-reconnect", time.Unix(1, 0).UTC())

	select {
	case event := <-stream.Events:
		if event.Type != interfaces.FactoryEventType(factoryapi.FactoryEventTypeFactoryStateResponse) {
			t.Fatalf("live event type = %s, want %s", event.Type, factoryapi.FactoryEventTypeFactoryStateResponse)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for live event on valid subscriber after invalid reconnect attempt")
	}
}

func TestFactoryEventHistory_Subscribe_DoesNotMissEventsDuringRegistration(t *testing.T) {
	history := newTestFactoryEventHistory(eventHistoryProjectionNet(), func() time.Time { return time.Unix(0, 0).UTC() })
	history.RecordInitialStructure()

	const attempts = 64
	var wg sync.WaitGroup
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func(attempt int) {
			defer wg.Done()

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			stream, err := history.Subscribe(ctx, nil, interfaces.FactoryEventReconnectScope{})
			if err != nil {
				t.Errorf("Subscribe attempt %d: %v", attempt, err)
				return
			}

			eventID := fmt.Sprintf("factory-event/factory-state-change/%d/%s", attempt, interfaces.FactoryStateRunning)
			history.RecordFactoryStateChange(
				attempt,
				interfaces.FactoryStateIdle,
				interfaces.FactoryStateRunning,
				fmt.Sprintf("subscribe-race-%d", attempt),
				time.Unix(int64(attempt+1), 0).UTC(),
			)

			received := make(map[string]struct{}, len(stream.History)+attempts)
			for _, event := range stream.History {
				received[event.Id] = struct{}{}
			}
			if _, ok := received[eventID]; ok {
				return
			}

			deadline := time.After(time.Second)
			for {
				select {
				case event, ok := <-stream.Events:
					if !ok {
						t.Errorf("attempt %d: stream closed before event %s arrived", attempt, eventID)
						return
					}
					received[event.Id] = struct{}{}
					if _, found := received[eventID]; found {
						return
					}
				case <-deadline:
					t.Errorf("attempt %d: missed event %s between replay snapshot and live delivery", attempt, eventID)
					return
				}
			}
		}(i)
	}
	wg.Wait()
}

func TestBuildReconnectReplay_AfterEventIDReturnsOnlyNewerEvents(t *testing.T) {
	events := reconnectFixtureEvents(t)

	replay, err := BuildCanonicalReconnectReplay(events, interfaces.FactoryEventReconnectCursor{
		AfterEventID: "dispatch-queued/dispatch-js-1",
	}, interfaces.FactoryEventReconnectScope{SessionID: "session-js"})
	if err != nil {
		t.Fatalf("BuildReconnectReplay: %v", err)
	}
	if len(replay) != 2 {
		t.Fatalf("replay = %d events, want interrupted and reconciled only", len(replay))
	}
	if replay[0].Type != interfaces.FactoryEventTypeDispatchInterrupted {
		t.Fatalf("first replay event = %q, want DISPATCH_INTERRUPTED", replay[0].Type)
	}
	if replay[1].Type != interfaces.FactoryEventTypeDispatchReconciled {
		t.Fatalf("second replay event = %q, want DISPATCH_RECONCILED", replay[1].Type)
	}
}

func TestBuildReconnectReplay_AfterSessionSequenceReturnsOnlyNewerSessionEvents(t *testing.T) {
	events := reconnectFixtureEvents(t)
	sequence := 0

	replay, err := BuildCanonicalReconnectReplay(events, interfaces.FactoryEventReconnectCursor{
		AfterSequence: &sequence,
	}, interfaces.FactoryEventReconnectScope{SessionID: "session-js"})
	if err != nil {
		t.Fatalf("BuildReconnectReplay: %v", err)
	}
	if len(replay) != 2 {
		t.Fatalf("replay = %d events, want two newer session events", len(replay))
	}
}

func TestBuildReconnectReplay_AfterSequenceFallsBackWhenSessionSequenceIsAbsent(t *testing.T) {
	events := reconnectFixtureEvents(t)
	events[1].Context.SessionSequence = nil
	sequence := events[1].Context.Sequence

	replay, err := BuildCanonicalReconnectReplay(events, interfaces.FactoryEventReconnectCursor{
		AfterSequence: &sequence,
	}, interfaces.FactoryEventReconnectScope{SessionID: "session-js"})
	if err != nil {
		t.Fatalf("BuildReconnectReplay: %v", err)
	}
	if len(replay) != 1 || replay[0].Id != events[2].Id {
		t.Fatalf("replay = %#v, want only event %q", replay, events[2].Id)
	}
}

func TestBuildReconnectReplay_ReconstructsDispatchStateWithoutSessionCompleted(t *testing.T) {
	events := reconnectFixtureEvents(t)
	replay, err := BuildCanonicalReconnectReplay(events, interfaces.FactoryEventReconnectCursor{
		AfterEventID: "dispatch-queued/dispatch-js-1",
	}, interfaces.FactoryEventReconnectScope{SessionID: "session-js"})
	if err != nil {
		t.Fatalf("BuildReconnectReplay: %v", err)
	}

	worldState, err := projections.ReconstructCanonicalFactoryWorldState(replay, 3)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState: %v", err)
	}
	if worldState.SessionBracket != nil && worldState.SessionBracket.Terminal {
		t.Fatalf("session bracket = %#v, want reconnect replay without SESSION_COMPLETED", worldState.SessionBracket)
	}
	if worldState.JavaScriptRuntime == nil {
		t.Fatalf("javascript runtime = nil, want reconnect replay dispatch state")
	}
	if worldState.JavaScriptRuntime.CompletedDispatches != 0 {
		t.Fatalf("javascript runtime completed dispatches = %d, want zero after interrupted dispatch suppresses late reconciliation", worldState.JavaScriptRuntime.CompletedDispatches)
	}
	if len(worldState.JavaScriptRuntime.Dispatches) != 1 {
		t.Fatalf("javascript runtime dispatches = %#v, want one interrupted dispatch", worldState.JavaScriptRuntime.Dispatches)
	}
	dispatch := worldState.JavaScriptRuntime.Dispatches[0]
	if dispatch.Status != string(factoryapi.FactoryDispatchStatusINTERRUPTED) {
		t.Fatalf("dispatch status = %q, want INTERRUPTED after replay suppression", dispatch.Status)
	}
	if dispatch.FailureDetail == nil || dispatch.FailureDetail.Message != "provider disconnected" {
		t.Fatalf("dispatch failure detail = %#v, want provider disconnected interruption reason", dispatch.FailureDetail)
	}
}

func TestBuildReconnectReplay_AllowsAcknowledgementPrefixWithoutJavaScriptRuntime(t *testing.T) {
	history := newTestFactoryEventHistory(nil, func() time.Time { return time.Unix(0, 0).UTC() })
	history.RecordRunRequest()
	prefix := history.CanonicalEvents()
	if len(prefix) != 1 {
		t.Fatalf("run request prefix = %#v, want one event", prefix)
	}

	phase := domainFactoryEvent(
		interfaces.FactoryEventTypeJavaScriptPhaseChange,
		"javascript-phase/review",
		interfaces.FactoryEventContext{Tick: 1, Sequence: 1, EventTime: time.Unix(1, 0).UTC()},
		interfaces.JavaScriptPhaseChangeEventPayload{
			ChildDispatchCounts: interfaces.FactorySessionChildDispatchCounts{},
			Phase:               "review",
			Phases:              []string{"review"},
			ScriptStatus:        interfaces.FactorySessionJavaScriptScriptStatusRunning,
		},
	)
	events := append(append([]interfaces.FactoryEvent(nil), prefix...), phase)

	replay, err := BuildCanonicalReconnectReplay(events, interfaces.FactoryEventReconnectCursor{
		AfterEventID: prefix[0].Id,
	}, interfaces.FactoryEventReconnectScope{})
	if err != nil {
		t.Fatalf("BuildReconnectReplay: %v", err)
	}
	if len(replay) != 1 || replay[0].Id != phase.Id {
		t.Fatalf("replay = %#v, want the JavaScript phase event after the acknowledgement", replay)
	}
}

func TestBuildReconnectReplay_IdempotentWhenNoNewEvents(t *testing.T) {
	events := reconnectFixtureEvents(t)
	cursor := interfaces.FactoryEventReconnectCursor{AfterEventID: "dispatch-reconciled/dispatch-js-1"}

	first, err := BuildCanonicalReconnectReplay(events, cursor, interfaces.FactoryEventReconnectScope{SessionID: "session-js"})
	if err != nil {
		t.Fatalf("first BuildReconnectReplay: %v", err)
	}
	second, err := BuildCanonicalReconnectReplay(events, cursor, interfaces.FactoryEventReconnectScope{SessionID: "session-js"})
	if err != nil {
		t.Fatalf("second BuildReconnectReplay: %v", err)
	}
	if len(first) != 0 || len(second) != 0 {
		t.Fatalf("idempotent replay = %#v and %#v, want empty slices", first, second)
	}
}

func reconnectFixtureEvents(t *testing.T) []interfaces.FactoryEvent {
	t.Helper()
	t0 := time.Date(2026, 6, 9, 15, 0, 0, 0, time.UTC)
	sessionID := "session-js"
	kind := factoryapi.JAVASCRIPT
	dispatchID := "dispatch-js-1"
	generated := []factoryapi.FactoryEvent{
		reconnectFixtureEvent(factoryapi.FactoryEventTypeDispatchQueued, "dispatch-queued/"+dispatchID, 1, t0, 0, factoryapi.FactoryEventContext{
			Sequence:         0,
			SessionId:        &sessionID,
			SessionSequence:  intPtr(0),
			OrchestratorKind: &kind,
			DispatchId:       stringPtr(dispatchID),
		}, factoryapi.DispatchQueuedEventPayload{
			DispatchKind: factoryapi.FactoryDispatchKindJAVASCRIPTAGENT,
		}),
		reconnectFixtureEvent(factoryapi.FactoryEventTypeDispatchInterrupted, "dispatch-interrupted/"+dispatchID, 2, t0.Add(time.Second), 1, factoryapi.FactoryEventContext{
			Sequence:         1,
			SessionId:        &sessionID,
			SessionSequence:  intPtr(1),
			OrchestratorKind: &kind,
			DispatchId:       stringPtr(dispatchID),
		}, factoryapi.DispatchInterruptedEventPayload{
			Reason:         "provider disconnected",
			ObservedStatus: factoryapi.FactoryDispatchStatusFAILED,
			InterruptedAt:  t0.Add(time.Second),
		}),
		reconnectFixtureEvent(factoryapi.FactoryEventTypeDispatchReconciled, "dispatch-reconciled/"+dispatchID, 3, t0.Add(2*time.Second), 2, factoryapi.FactoryEventContext{
			Sequence:         2,
			SessionId:        &sessionID,
			SessionSequence:  intPtr(2),
			OrchestratorKind: &kind,
			DispatchId:       stringPtr(dispatchID),
		}, factoryapi.DispatchReconciledEventPayload{
			ReconciledStatus:     factoryapi.FactoryDispatchStatusCOMPLETED,
			ReconciliationSource: factoryapi.PROVIDERSESSION,
			Replayed:             false,
		}),
	}
	events := make([]interfaces.FactoryEvent, len(generated))
	for index, event := range generated {
		canonical, err := interfaces.NewFactoryEvent(event)
		if err != nil {
			t.Fatalf("canonical reconnect fixture event %d: %v", index, err)
		}
		events[index] = canonical
	}
	return events
}

func reconnectFixtureEvent(
	eventType factoryapi.FactoryEventType,
	id string,
	tick int,
	eventTime time.Time,
	sequence int,
	context factoryapi.FactoryEventContext,
	payload any,
) factoryapi.FactoryEvent {
	context.Tick = tick
	context.EventTime = eventTime
	context.Sequence = sequence
	event := factoryapi.FactoryEvent{
		Type:    eventType,
		Id:      id,
		Context: context,
		Payload: reconnectFixturePayload(payload),
	}
	event.SchemaVersion = factoryapi.AgentFactoryEventV1
	return event
}

func reconnectFixturePayload(payload any) factoryapi.FactoryEvent_Payload {
	var out factoryapi.FactoryEvent_Payload
	var err error
	switch typed := payload.(type) {
	case factoryapi.DispatchQueuedEventPayload:
		err = out.FromDispatchQueuedEventPayload(typed)
	case factoryapi.DispatchInterruptedEventPayload:
		err = out.FromDispatchInterruptedEventPayload(typed)
	case factoryapi.DispatchReconciledEventPayload:
		err = out.FromDispatchReconciledEventPayload(typed)
	default:
		panic(fmt.Sprintf("unsupported reconnect fixture payload %T", payload))
	}
	if err != nil {
		panic(fmt.Sprintf("encode reconnect fixture payload %T: %v", payload, err))
	}
	return out
}

func intPtr(value int) *int {
	return &value
}

func TestFactoryEventHistory_NilSubscribeReturnsClosedStream(t *testing.T) {
	stream, err := (*FactoryEventHistory)(nil).Subscribe(context.Background(), nil, interfaces.FactoryEventReconnectScope{})
	if err != nil || stream.Events == nil {
		t.Fatalf("Subscribe() = %#v, %v; want a closed event stream", stream, err)
	}
	if _, ok := <-stream.Events; ok {
		t.Fatal("nil history returned an open event stream")
	}
}

func TestFactoryEventHistory_CloseLiveSubscriptionsDeliversQueuedTerminalEvent(t *testing.T) {
	history := newTestFactoryEventHistory(nil, func() time.Time { return time.Unix(0, 0).UTC() })
	stream, err := history.Subscribe(context.Background(), nil, interfaces.FactoryEventReconnectScope{})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	history.RecordRunResponse(1, interfaces.FactoryStateCompleted, "", time.Unix(1, 0).UTC())
	canonical := history.CanonicalEvents()
	if len(canonical) != 1 || canonical[0].Type != interfaces.FactoryEventTypeRunResponse {
		t.Fatalf("canonical events = %#v, want one RUN_RESPONSE", canonical)
	}

	history.mu.RLock()
	subscription := history.streams[0]
	history.mu.RUnlock()
	if subscription == nil {
		t.Fatal("live subscription was not registered")
	}

	closeDone := make(chan struct{})
	go func() {
		history.CloseLiveSubscriptions()
		close(closeDone)
	}()
	<-subscription.terminal

	event, ok := <-stream.Events
	if !ok {
		t.Fatal("live stream closed before queued RUN_RESPONSE")
	}
	if event.Type != interfaces.FactoryEventTypeRunResponse {
		t.Fatalf("live event type = %s, want %s", event.Type, interfaces.FactoryEventTypeRunResponse)
	}
	if _, ok := <-stream.Events; ok {
		t.Fatal("live stream delivered an event after RUN_RESPONSE")
	}
	<-closeDone

	late, err := history.Subscribe(context.Background(), nil, interfaces.FactoryEventReconnectScope{})
	if err != nil {
		t.Fatalf("late Subscribe: %v", err)
	}
	if _, ok := <-late.Events; ok {
		t.Fatal("late live subscription remained open after terminal close")
	}
}

func TestFactoryEventHistory_CloseLiveSubscriptionsBoundsUnreadSubscriber(t *testing.T) {
	history := newTestFactoryEventHistory(nil, func() time.Time { return time.Unix(0, 0).UTC() })
	_, err := history.Subscribe(context.Background(), nil, interfaces.FactoryEventReconnectScope{})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	history.mu.RLock()
	subscription := history.streams[0]
	history.mu.RUnlock()
	if subscription == nil {
		t.Fatal("live subscription was not registered")
	}

	history.RecordRunResponse(1, interfaces.FactoryStateCompleted, "", time.Unix(1, 0).UTC())
	closeDone := make(chan struct{})
	go func() {
		history.CloseLiveSubscriptions()
		close(closeDone)
	}()

	deadline := time.NewTimer(eventHistoryCloseDrainTimeout + 250*time.Millisecond)
	defer deadline.Stop()
	select {
	case <-closeDone:
	case <-subscription.overflow:
		select {
		case <-closeDone:
		case <-deadline.C:
			t.Fatal("CloseLiveSubscriptions did not return after releasing the unread subscriber")
		}
	case <-deadline.C:
		t.Fatal("CloseLiveSubscriptions blocked on a subscriber that stopped reading")
	}
}
