package responsebridge

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sync"
	"testing"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
	"github.com/portpowered/infinite-you/pkg/services/events"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

type bridgeSequencer struct {
	mu                      sync.Mutex
	sequences               []chatsessions.SequenceRequest
	advances                []chatsessions.AdvanceStreamHeadRequest
	sequenceErr             error
	advanceErr              error
	sequenceSawCancelledCtx bool
	didFirst                chan struct{}
}

func (s *bridgeSequencer) Sequence(ctx context.Context, req chatsessions.SequenceRequest) (chatsessions.SequenceResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ctx.Err() != nil {
		s.sequenceSawCancelledCtx = true
		return chatsessions.SequenceResult{}, ctx.Err()
	}
	if s.sequenceErr != nil {
		return chatsessions.SequenceResult{}, s.sequenceErr
	}
	s.sequences = append(s.sequences, req)
	if len(s.sequences) == 1 {
		close(s.didFirst)
	}
	return chatsessions.SequenceResult{ItemID: "chat-item-" + string(rune('0'+len(s.sequences))), AggregateSequence: events.AggregateSequence(len(s.sequences))}, nil
}

func (s *bridgeSequencer) AdvanceStreamHead(_ context.Context, req chatsessions.AdvanceStreamHeadRequest) (chatsessions.AdvanceStreamHeadResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.advanceErr != nil {
		return chatsessions.AdvanceStreamHeadResult{}, s.advanceErr
	}
	s.advances = append(s.advances, req)
	return chatsessions.AdvanceStreamHeadResult{Session: chatsessions.Session{Version: req.ExpectedVersion + 1}}, nil
}

type bridgeTarget struct {
	factorysessions.TargetExecutionService
	cursor              *factorysessions.ResponseEventCursor
	err                 error
	factoryEvents       *factorydefinitions.FactoryEventStream
	factoryEventsErr    error
	factoryEventStreams []*factorydefinitions.FactoryEventStream
	factoryEventIndex   *int
}

func (t bridgeTarget) SubscribeFactoryResponseEvents(_ context.Context, _ factorysessions.ResponseEventSubscriptionRequest) (*factorysessions.ResponseEventCursor, error) {
	return t.cursor, t.err
}

func (t bridgeTarget) SubscribeFactoryEventsForSession(_ context.Context, _ string, _ *factorydefinitions.FactoryEventReconnectCursor) (*factorydefinitions.FactoryEventStream, error) {
	if len(t.factoryEventStreams) > 0 {
		index := 0
		if t.factoryEventIndex != nil {
			index = *t.factoryEventIndex
			*t.factoryEventIndex = *t.factoryEventIndex + 1
		}
		if index >= len(t.factoryEventStreams) {
			index = len(t.factoryEventStreams) - 1
		}
		return t.factoryEventStreams[index], nil
	}
	return t.factoryEvents, t.factoryEventsErr
}

func TestServiceRunFallsBackToInvokeWhenBridgeIsUnconfigured(t *testing.T) {
	want := factorysessions.InvocationResult{RequestID: "turn-1", Status: factorysessions.InvocationTerminalStatusCompleted}
	invoke := func(context.Context) (factorysessions.InvocationResult, error) {
		return want, nil
	}

	var nilService *Service
	got, err := nilService.Run(context.Background(), "chat-1", 1, "factory-1", nil, invoke)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("nil Service.Run() = (%+v, %v), want (%+v, nil)", got, err, want)
	}

	got, err = New(nil, nil, nil, logging.NoopLogger{}).Run(context.Background(), "chat-1", 1, "factory-1", nil, invoke)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("unconfigured Service.Run() = (%+v, %v), want (%+v, nil)", got, err, want)
	}
}

func TestServiceRunSequencesFactoryEventsAndPreservesInvokeResult(t *testing.T) {
	sequencer := &bridgeSequencer{didFirst: make(chan struct{})}
	parent := factorysessions.FactoryResponseEvent{FactorySessionID: "factory-1", EventID: "parent", Sequence: 1, ItemID: "source-parent", Kind: workers.KindMessage, Phase: workers.PhaseCompleted, Payload: json.RawMessage(`{"role":"ASSISTANT"}`)}
	child := factorysessions.FactoryResponseEvent{FactorySessionID: "factory-1", EventID: "child", Sequence: 2, ItemID: "source-child", ParentItemID: "source-parent", Kind: workers.KindReasoning, Phase: workers.PhaseCompleted, Payload: json.RawMessage(`{"summary":"reasoning"}`)}
	nextCalls := 0
	cursor := &factorysessions.ResponseEventCursor{
		NextEvents: func(context.Context) ([]factorysessions.FactoryResponseEvent, error) {
			nextCalls++
			if nextCalls == 1 {
				return []factorysessions.FactoryResponseEvent{parent, child}, nil
			}
			return nil, factorysessions.ErrResponseEventSubscriptionClosed
		},
		DrainEvents: func() ([]factorysessions.FactoryResponseEvent, error) {
			return nil, factorysessions.ErrResponseEventSubscriptionClosed
		},
		DetachCursor: func() {},
	}

	want := factorysessions.InvocationResult{RequestID: "turn-1", Status: factorysessions.InvocationTerminalStatusCompleted}
	got, err := New(sequencer, bridgeTarget{cursor: cursor}, nil, logging.NoopLogger{}).Run(context.Background(), "chat-1", 4, "factory-1", nil,
		func(context.Context) (factorysessions.InvocationResult, error) {
			<-sequencer.didFirst
			return want, nil
		},
	)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("Run() = (%+v, %v), want (%+v, nil)", got, err, want)
	}
	if len(sequencer.sequences) != 2 || len(sequencer.advances) != 2 {
		t.Fatalf("sequence/advance counts = %d/%d, want 2/2", len(sequencer.sequences), len(sequencer.advances))
	}
	if sequencer.sequences[1].ParentItemID != "chat-item-1" {
		t.Fatalf("child ParentItemID = %q, want remapped chat-item-1", sequencer.sequences[1].ParentItemID)
	}
	if sequencer.advances[0].ExpectedVersion != 4 || sequencer.advances[1].ExpectedVersion != 5 {
		t.Fatalf("advance versions = %d/%d, want 4/5", sequencer.advances[0].ExpectedVersion, sequencer.advances[1].ExpectedVersion)
	}
}

func TestServiceRunWithoutBridgeDependenciesDelegatesInvoke(t *testing.T) {
	want := factorysessions.InvocationResult{
		RequestID: "turn-without-bridge",
		Status:    factorysessions.InvocationTerminalStatusCompleted,
	}
	called := false
	got, err := New(nil, nil, nil, logging.NoopLogger{}).Run(
		context.Background(), "chat-1", 1, "factory-1", nil,
		func(context.Context) (factorysessions.InvocationResult, error) {
			called = true
			return want, nil
		},
	)
	if err != nil {
		t.Fatalf("Run() error = %v, want nil", err)
	}
	if !called {
		t.Fatal("invoke callback was not called")
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Run() result = %+v, want %+v", got, want)
	}
}

// TestServiceRunDrainsTerminalTailWithNonCancelledContext proves the
// invocation-return handoff cannot silently lose response events published at
// the terminal boundary: the live reader stops first, then Drain returns the
// retained tail and its sequencing context remains usable after the invoke
// context has been cancelled.
func TestServiceRunDrainsTerminalTailWithNonCancelledContext(t *testing.T) {
	sequencer := &bridgeSequencer{didFirst: make(chan struct{})}
	nextStarted := make(chan struct{})
	invokeReturned := make(chan struct{})
	tail := factorysessions.FactoryResponseEvent{
		FactorySessionID: "factory-1", EventID: "tail", Sequence: 1,
		Kind: workers.KindMessage, Phase: workers.PhaseCompleted,
		Payload: json.RawMessage(`{"role":"ASSISTANT","content":[{"type":"text","text":"tail"}]}`),
	}
	cursor := &factorysessions.ResponseEventCursor{
		NextEvents: func(ctx context.Context) ([]factorysessions.FactoryResponseEvent, error) {
			close(nextStarted)
			<-ctx.Done()
			return nil, ctx.Err()
		},
		DrainEvents: func() ([]factorysessions.FactoryResponseEvent, error) {
			select {
			case <-invokeReturned:
				return []factorysessions.FactoryResponseEvent{tail}, nil
			default:
				t.Fatal("Drain ran before invoke returned")
				return nil, nil
			}
		},
		DetachCursor: func() {},
	}

	want := factorysessions.InvocationResult{RequestID: "turn-1", Status: factorysessions.InvocationTerminalStatusCompleted}
	got, err := New(sequencer, bridgeTarget{cursor: cursor}, nil, logging.NoopLogger{}).Run(context.Background(), "chat-1", 4, "factory-1", nil,
		func(context.Context) (factorysessions.InvocationResult, error) {
			<-nextStarted
			close(invokeReturned)
			return want, nil
		},
	)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("Run() = (%+v, %v), want (%+v, nil)", got, err, want)
	}
	if len(sequencer.sequences) != 1 || len(sequencer.advances) != 1 {
		t.Fatalf("sequence/advance counts = %d/%d, want 1/1", len(sequencer.sequences), len(sequencer.advances))
	}
	if sequencer.sequenceSawCancelledCtx {
		t.Fatal("tail Sequence received a cancelled context, want a delivery context that survives invoke completion")
	}
}

// TestServiceRunReturnsTerminalTailSequencingFailure proves a tail that fails
// to commit makes the prompt fail safely instead of returning a successful
// invocation result with silently missing canonical output.
func TestServiceRunReturnsTerminalTailSequencingFailure(t *testing.T) {
	wantErr := errors.New("sequence tail")
	sequencer := &bridgeSequencer{didFirst: make(chan struct{}), sequenceErr: wantErr}
	nextStarted := make(chan struct{})
	tail := factorysessions.FactoryResponseEvent{FactorySessionID: "factory-1", EventID: "tail", Sequence: 1, Kind: workers.KindUsage, Phase: workers.PhaseUpdated, Payload: json.RawMessage(`{"inputTokens":1}`)}
	cursor := &factorysessions.ResponseEventCursor{
		NextEvents: func(ctx context.Context) ([]factorysessions.FactoryResponseEvent, error) {
			close(nextStarted)
			<-ctx.Done()
			return nil, ctx.Err()
		},
		DrainEvents: func() ([]factorysessions.FactoryResponseEvent, error) {
			return []factorysessions.FactoryResponseEvent{tail}, nil
		},
		DetachCursor: func() {},
	}

	want := factorysessions.InvocationResult{RequestID: "turn-1", Status: factorysessions.InvocationTerminalStatusCompleted}
	got, err := New(sequencer, bridgeTarget{cursor: cursor}, nil, logging.NoopLogger{}).Run(context.Background(), "chat-1", 4, "factory-1", nil,
		func(context.Context) (factorysessions.InvocationResult, error) {
			<-nextStarted
			return want, nil
		},
	)
	if !errors.Is(err, wantErr) {
		t.Fatalf("Run() error = %v, want wrapped %v", err, wantErr)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Run() result = %+v, want %+v", got, want)
	}
}

func TestServiceRunDoesNotReplaceInvokeFailureWhenSubscriptionFails(t *testing.T) {
	wantErr := errors.New("invoke failed")
	want := factorysessions.InvocationResult{Status: factorysessions.InvocationTerminalStatusFailed}
	got, err := New(&bridgeSequencer{didFirst: make(chan struct{})}, bridgeTarget{err: errors.New("subscribe failed")}, nil, logging.NoopLogger{}).Run(
		context.Background(), "chat-1", 1, "factory-1", nil,
		func(context.Context) (factorysessions.InvocationResult, error) { return want, wantErr },
	)
	if !errors.Is(err, wantErr) || !reflect.DeepEqual(got, want) {
		t.Fatalf("Run() = (%+v, %v), want (%+v, %v)", got, err, want, wantErr)
	}
}

// bridgeWorkerEvents models a Worker Session topic with a real aggregate
// cursor: retained tests read it only during the terminal sweep, while live
// tests consume the same records from Subscribe before the sweep sees head.
// It keeps the response bridge test focused on the producer boundary without
// importing Events' private Store implementation across its package boundary.
type bridgeWorkerEvents struct {
	events.Service

	mu              sync.Mutex
	records         map[events.Topic][]events.Record
	deliverLive     bool
	liveDelivered   chan struct{}
	liveDeliveredMu sync.Once
}

var _ events.Service = (*bridgeWorkerEvents)(nil)

func (f *bridgeWorkerEvents) Read(_ context.Context, req events.ReadRequest) (events.ReadResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	records := f.records[req.Topic]
	head := events.AggregateSequence(len(records))
	if req.From.Position >= head {
		return events.ReadResult{
			Outcome:  events.ReadOutcomeAtHead,
			Next:     events.Cursor{Topic: req.Topic, Position: head},
			Retained: events.RetainedRange{Topic: req.Topic, Earliest: 1, Head: head},
		}, nil
	}
	start := int(req.From.Position)
	end := start + req.Limit
	if end > len(records) {
		end = len(records)
	}
	page := append([]events.Record(nil), records[start:end]...)
	return events.ReadResult{
		Records:  page,
		Next:     events.Cursor{Topic: req.Topic, Position: page[len(page)-1].ID.Position},
		Outcome:  events.ReadOutcomeProgress,
		Retained: events.RetainedRange{Topic: req.Topic, Earliest: 1, Head: head},
	}, nil
}

func (f *bridgeWorkerEvents) Subscribe(_ context.Context, req events.SubscribeRequest) (events.Subscription, error) {
	position := req.From.Position
	return func(ctx context.Context) events.Delivery {
		if f.deliverLive {
			f.mu.Lock()
			records := f.records[req.Topic]
			if position < events.AggregateSequence(len(records)) {
				record := records[int(position)]
				position = record.ID.Position
				last := position == events.AggregateSequence(len(records))
				f.mu.Unlock()
				if last && f.liveDelivered != nil {
					f.liveDeliveredMu.Do(func() { close(f.liveDelivered) })
				}
				return events.Delivery{Kind: events.DeliveryRecord, Record: record, Cursor: events.Cursor{Topic: req.Topic, Position: position}}
			}
			f.mu.Unlock()
		}
		<-ctx.Done()
		return events.Delivery{Kind: events.DeliveryCanceled}
	}, nil
}

func workerLifecycleRecord(t *testing.T, workerSessionID string, position events.AggregateSequence, phase workers.Phase, status string) events.Record {
	t.Helper()
	return workerDraftRecord(t, workerSessionID, position, workers.Draft{
		Kind: workers.KindSession, Phase: phase,
		Payload: json.RawMessage(`{"status":"` + status + `"}`),
	})
}

func workerDraftRecord(t *testing.T, workerSessionID string, position events.AggregateSequence, draft workers.Draft) events.Record {
	t.Helper()
	payload, err := json.Marshal(draft)
	if err != nil {
		t.Fatalf("marshal worker draft: %v", err)
	}
	topic := workersessions.Topic(workerSessionID)
	return events.Record{
		ID:             events.RecordID{Topic: topic, Position: position},
		SourceType:     "worker_session",
		SourceID:       events.SourceID(workerSessionID),
		SourceSequence: events.SourceSequence(position),
		SourceEventID:  events.SourceEventID("worker-event-" + string(rune('0'+position))),
		SchemaID:       "workers.draft.v1",
		Payload:        payload,
	}
}

func workerAssociationStream(t *testing.T, dispatchID, workerSessionID string) *factorydefinitions.FactoryEventStream {
	t.Helper()
	payload, err := json.Marshal(factorydefinitions.DispatchWorkerSessionAssociationEventPayload{WorkerSessionID: workerSessionID})
	if err != nil {
		t.Fatalf("marshal worker association: %v", err)
	}
	closed := make(chan factorydefinitions.FactoryEvent)
	close(closed)
	return &factorydefinitions.FactoryEventStream{
		History: []factorydefinitions.FactoryEvent{{
			Type:    factorydefinitions.FactoryEventTypeDispatchWorkerSessionAssoc,
			Context: factorydefinitions.FactoryEventContext{DispatchID: &dispatchID},
			Payload: payload,
		}},
		Events: closed,
	}
}

func closedResponseCursor() *factorysessions.ResponseEventCursor {
	return &factorysessions.ResponseEventCursor{
		NextEvents: func(context.Context) ([]factorysessions.FactoryResponseEvent, error) {
			return nil, factorysessions.ErrResponseEventSubscriptionClosed
		},
		DrainEvents: func() ([]factorysessions.FactoryResponseEvent, error) {
			return nil, factorysessions.ErrResponseEventSubscriptionClosed
		},
		DetachCursor: func() {},
	}
}

func TestServiceRunSequencesAssociatedWorkerLifecycleFromRetainedTail(t *testing.T) {
	const workerSessionID = "worker-session-1"
	sequencer := &bridgeSequencer{didFirst: make(chan struct{})}
	workerEvents := &bridgeWorkerEvents{records: map[events.Topic][]events.Record{
		workersessions.Topic(workerSessionID): {
			workerLifecycleRecord(t, workerSessionID, 1, workers.PhaseStarted, "STARTING"),
			workerLifecycleRecord(t, workerSessionID, 2, workers.PhaseUpdated, "RUNNING"),
			workerLifecycleRecord(t, workerSessionID, 3, workers.PhaseCompleted, "COMPLETED"),
		},
	}}
	target := bridgeTarget{cursor: closedResponseCursor(), factoryEvents: workerAssociationStream(t, "dispatch-1", workerSessionID)}

	got, err := New(sequencer, target, workerEvents, logging.NoopLogger{}).Run(
		context.Background(), "chat-1", 4, "factory-1", nil,
		func(context.Context) (factorysessions.InvocationResult, error) {
			return factorysessions.InvocationResult{Status: factorysessions.InvocationTerminalStatusCompleted}, nil
		},
	)
	if err != nil || got.Status != factorysessions.InvocationTerminalStatusCompleted {
		t.Fatalf("Run() = (%+v, %v), want completed result and nil", got, err)
	}
	if len(sequencer.sequences) != 3 || len(sequencer.advances) != 3 {
		t.Fatalf("sequence/advance counts = %d/%d, want all three retained worker records", len(sequencer.sequences), len(sequencer.advances))
	}
	assertAssociatedWorkerLifecycle(t, sequencer.sequences, workerSessionID)
}

func TestServiceRunSequencesAssociatedWorkerContentRecordsWithOpeningParent(t *testing.T) {
	const workerSessionID = "worker-session-content"
	sequencer := &bridgeSequencer{didFirst: make(chan struct{})}
	workerEvents := &bridgeWorkerEvents{records: map[events.Topic][]events.Record{
		workersessions.Topic(workerSessionID): {
			workerLifecycleRecord(t, workerSessionID, 1, workers.PhaseStarted, "STARTING"),
			workerDraftRecord(t, workerSessionID, 2, workers.Draft{Kind: workers.KindMessage, Phase: workers.PhaseDelta, Payload: json.RawMessage(`{"contentBlockIndex":0,"contentBlockKind":"TEXT","textDelta":"answer"}`)}),
			workerDraftRecord(t, workerSessionID, 3, workers.Draft{Kind: workers.KindTool, Phase: workers.PhaseDelta, Payload: json.RawMessage(`{"toolCallId":"native-tool","outputDelta":"tool output"}`)}),
			workerDraftRecord(t, workerSessionID, 4, workers.Draft{Kind: workers.KindProgress, Phase: workers.PhaseUpdated, Payload: json.RawMessage(`{"label":"working"}`)}),
			workerDraftRecord(t, workerSessionID, 5, workers.Draft{Kind: workers.KindError, Phase: workers.PhaseUpdated, Payload: json.RawMessage(`{"code":"retry","message":"temporary"}`)}),
			workerLifecycleRecord(t, workerSessionID, 6, workers.PhaseCompleted, "COMPLETED"),
		},
	}}
	target := bridgeTarget{cursor: closedResponseCursor(), factoryEvents: workerAssociationStream(t, "dispatch-content", workerSessionID)}

	got, err := New(sequencer, target, workerEvents, logging.NoopLogger{}).Run(
		context.Background(), "chat-1", 4, "factory-1", nil,
		func(context.Context) (factorysessions.InvocationResult, error) {
			return factorysessions.InvocationResult{Status: factorysessions.InvocationTerminalStatusCompleted}, nil
		},
	)
	if err != nil || got.Status != factorysessions.InvocationTerminalStatusCompleted {
		t.Fatalf("Run() = (%+v, %v), want completed result and nil", got, err)
	}
	if len(sequencer.sequences) != 6 || len(sequencer.advances) != 6 {
		t.Fatalf("sequence/advance counts = %d/%d, want every associated Worker record", len(sequencer.sequences), len(sequencer.advances))
	}
	assertAssociatedWorkerLifecycle(t, sequencer.sequences, workerSessionID)
	for index, want := range []workers.Kind{
		workers.KindSession, workers.KindMessage, workers.KindTool, workers.KindProgress, workers.KindError, workers.KindSession,
	} {
		if got := sequencer.sequences[index].Kind; got != want {
			t.Fatalf("sequence %d kind = %q, want %q", index, got, want)
		}
	}
}

func TestServiceRunSequencesAssociatedWorkerLifecycleLiveBeforeInvokeReturns(t *testing.T) {
	const workerSessionID = "worker-session-live"
	sequencer := &bridgeSequencer{didFirst: make(chan struct{})}
	liveDelivered := make(chan struct{})
	workerEvents := &bridgeWorkerEvents{
		deliverLive: true, liveDelivered: liveDelivered,
		records: map[events.Topic][]events.Record{workersessions.Topic(workerSessionID): {
			workerLifecycleRecord(t, workerSessionID, 1, workers.PhaseStarted, "STARTING"),
			workerLifecycleRecord(t, workerSessionID, 2, workers.PhaseUpdated, "RUNNING"),
			workerLifecycleRecord(t, workerSessionID, 3, workers.PhaseFailed, "FAILED"),
		}},
	}
	target := bridgeTarget{cursor: closedResponseCursor(), factoryEvents: workerAssociationStream(t, "dispatch-live", workerSessionID)}

	_, err := New(sequencer, target, workerEvents, logging.NoopLogger{}).Run(
		context.Background(), "chat-1", 4, "factory-1", nil,
		func(context.Context) (factorysessions.InvocationResult, error) {
			<-liveDelivered
			return factorysessions.InvocationResult{Status: factorysessions.InvocationTerminalStatusCompleted}, nil
		},
	)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(sequencer.sequences) != 3 || len(sequencer.advances) != 3 {
		t.Fatalf("sequence/advance counts = %d/%d, want exactly the live worker records", len(sequencer.sequences), len(sequencer.advances))
	}
	assertAssociatedWorkerLifecycle(t, sequencer.sequences, workerSessionID)
	if sequencer.sequences[2].Phase != workers.PhaseFailed {
		t.Fatalf("terminal phase = %q, want FAILED", sequencer.sequences[2].Phase)
	}
}

func TestServiceRunDefersUnknownDispatchResponseUntilWorkerAssociation(t *testing.T) {
	const dispatchID = "dispatch-race"
	const workerSessionID = "worker-race"

	sequencer := &bridgeSequencer{didFirst: make(chan struct{})}
	liveResponseSeen := make(chan struct{})
	var liveResponseSeenOnce sync.Once
	response := factorysessions.FactoryResponseEvent{
		FactorySessionID: "factory-1", DispatchID: dispatchID, EventID: "child-response", Sequence: 1,
		Kind: workers.KindMessage, Phase: workers.PhaseDelta,
		Payload: json.RawMessage(`{"text":"child output"}`),
	}
	terminal := factorysessions.FactoryResponseEvent{
		FactorySessionID: "factory-1", EventID: "terminal-response", Sequence: 2,
		Kind: workers.KindMessage, Phase: workers.PhaseCompleted,
		Payload: json.RawMessage(`{"text":"merged result"}`),
	}
	cursor := &factorysessions.ResponseEventCursor{
		NextEvents: func(ctx context.Context) ([]factorysessions.FactoryResponseEvent, error) {
			liveResponseSeenOnce.Do(func() { close(liveResponseSeen) })
			if response.EventID != "" {
				event := response
				response.EventID = ""
				// The associated child response and the legitimate top-level
				// terminal event are one live batch. The bridge must defer only
				// the child copy rather than losing the batch remainder when the
				// cursor closes at the terminal boundary.
				return []factorysessions.FactoryResponseEvent{event, terminal}, nil
			}
			<-ctx.Done()
			return nil, ctx.Err()
		},
		DrainEvents: func() ([]factorysessions.FactoryResponseEvent, error) {
			return nil, factorysessions.ErrResponseEventSubscriptionClosed
		},
		DetachCursor: func() {},
	}
	closed := make(chan factorydefinitions.FactoryEvent)
	close(closed)
	association := workerAssociationStream(t, dispatchID, workerSessionID)
	firstFactoryStream := &factorydefinitions.FactoryEventStream{Events: closed}
	target := bridgeTarget{
		cursor:              cursor,
		factoryEventStreams: []*factorydefinitions.FactoryEventStream{firstFactoryStream, association},
		factoryEventIndex:   new(int),
	}
	workerEvents := &bridgeWorkerEvents{records: map[events.Topic][]events.Record{}}

	_, err := New(sequencer, target, workerEvents, logging.NoopLogger{}).Run(
		context.Background(), "chat-1", 1, "factory-1", nil,
		func(context.Context) (factorysessions.InvocationResult, error) {
			<-liveResponseSeen
			return factorysessions.InvocationResult{Status: factorysessions.InvocationTerminalStatusCompleted}, nil
		},
	)
	if err != nil {
		t.Fatalf("Run() error = %v, want nil", err)
	}
	if len(sequencer.sequences) != 1 {
		t.Fatalf("top-level response sequences = %d, want only the terminal event", len(sequencer.sequences))
	}
	if got := sequencer.sequences[0].SourceEventID; got != events.SourceEventID(terminal.EventID) {
		t.Fatalf("top-level response event = %q, want %q", got, terminal.EventID)
	}
	if sequencer.sequences[0].WorkerSessionAssociation != nil {
		t.Fatalf("terminal response association = %#v, want no Worker association", sequencer.sequences[0].WorkerSessionAssociation)
	}
}

func assertAssociatedWorkerLifecycle(t *testing.T, sequences []chatsessions.SequenceRequest, workerSessionID string) {
	t.Helper()
	for index, sequence := range sequences {
		if sequence.WorkerSessionAssociation == nil || sequence.WorkerSessionAssociation.WorkerSessionID != workerSessionID {
			t.Fatalf("sequence %d association = %#v, want canonical worker %q", index, sequence.WorkerSessionAssociation, workerSessionID)
		}
		if sequence.SourceType != workerEventSourceType || sequence.SourceID != events.SourceID(workerSessionID) {
			t.Fatalf("sequence %d source = (%q, %q), want worker topic identity", index, sequence.SourceType, sequence.SourceID)
		}
	}
	if sequences[0].Phase != workers.PhaseStarted || sequences[0].ParentItemID != "" {
		t.Fatalf("opening sequence = %#v, want top-level STARTED", sequences[0])
	}
	for index := 1; index < len(sequences); index++ {
		if sequences[index].ParentItemID != "chat-item-1" {
			t.Fatalf("sequence %d ParentItemID = %q, want stored opening identity chat-item-1", index, sequences[index].ParentItemID)
		}
	}
}

type bridgeLogCall struct {
	level string
	msg   string
	kv    []any
}

type bridgeRecordingLogger struct{ calls []bridgeLogCall }

func (l *bridgeRecordingLogger) Debug(msg string, kv ...any) {
	l.calls = append(l.calls, bridgeLogCall{level: "debug", msg: msg, kv: append([]any(nil), kv...)})
}
func (l *bridgeRecordingLogger) Info(msg string, kv ...any) {
	l.calls = append(l.calls, bridgeLogCall{level: "info", msg: msg, kv: append([]any(nil), kv...)})
}
func (*bridgeRecordingLogger) Warn(string, ...any)    {}
func (*bridgeRecordingLogger) Error(string, ...any)   {}
func (*bridgeRecordingLogger) Verbose(string, ...any) {}

func TestServiceRunLogsSafeSuccessfulOutcome(t *testing.T) {
	logger := &bridgeRecordingLogger{}
	cursor := &factorysessions.ResponseEventCursor{
		NextEvents: func(context.Context) ([]factorysessions.FactoryResponseEvent, error) {
			return nil, factorysessions.ErrResponseEventSubscriptionClosed
		},
		DrainEvents: func() ([]factorysessions.FactoryResponseEvent, error) {
			return nil, factorysessions.ErrResponseEventSubscriptionClosed
		},
		DetachCursor: func() {},
	}
	_, err := New(
		&bridgeSequencer{didFirst: make(chan struct{})},
		bridgeTarget{cursor: cursor},
		nil,
		logger,
	).Run(context.Background(), "chat-1", 1, "factory-1", nil,
		func(context.Context) (factorysessions.InvocationResult, error) {
			return factorysessions.InvocationResult{Status: factorysessions.InvocationTerminalStatusCompleted}, nil
		},
	)
	if err != nil {
		t.Fatalf("Run() error = %v, want nil", err)
	}
	if len(logger.calls) != 2 {
		t.Fatalf("log calls = %+v, want start and outcome", logger.calls)
	}
	if logger.calls[0].level != "debug" || logger.calls[0].msg != "chat_sessions response bridge start" {
		t.Fatalf("start log = %+v, want debug bridge start", logger.calls[0])
	}
	if logger.calls[1].level != "info" || logger.calls[1].msg != "chat_sessions response bridge outcome" {
		t.Fatalf("outcome log = %+v, want info bridge outcome", logger.calls[1])
	}
	assertBridgeLogValue(t, logger.calls[0], "op", responseBridgeOperation)
	assertBridgeLogValue(t, logger.calls[0], "chat_session_id", "chat-1")
	assertBridgeLogValue(t, logger.calls[0], "factory_session_id", "factory-1")
	assertBridgeLogValue(t, logger.calls[1], "terminal_status", string(factorysessions.InvocationTerminalStatusCompleted))
	assertBridgeLogValue(t, logger.calls[1], "error_class", "")
}

func TestServiceRunLogsSafeFailedOutcome(t *testing.T) {
	const unsafeSubscriptionError = "provider api key=secret"
	logger := &bridgeRecordingLogger{}
	_, err := New(
		&bridgeSequencer{didFirst: make(chan struct{})},
		bridgeTarget{err: errors.New(unsafeSubscriptionError)},
		nil,
		logger,
	).Run(context.Background(), "chat-1", 1, "factory-1", nil,
		func(context.Context) (factorysessions.InvocationResult, error) {
			return factorysessions.InvocationResult{Status: factorysessions.InvocationTerminalStatusCompleted}, nil
		},
	)
	if err == nil {
		t.Fatal("Run() error = nil, want subscription failure")
	}
	if len(logger.calls) != 2 {
		t.Fatalf("log calls = %+v, want start and outcome", logger.calls)
	}
	assertBridgeLogValue(t, logger.calls[1], "terminal_status", string(factorysessions.InvocationTerminalStatusCompleted))
	assertBridgeLogValue(t, logger.calls[1], "error_class", "response_event_subscription")
	for _, call := range logger.calls {
		if call.msg == unsafeSubscriptionError {
			t.Fatalf("log message leaked unsafe subscription error %q", unsafeSubscriptionError)
		}
		for i := 1; i < len(call.kv); i += 2 {
			if call.kv[i] == unsafeSubscriptionError {
				t.Fatalf("log fields leaked unsafe subscription error %q: %+v", unsafeSubscriptionError, call)
			}
		}
	}
}

func assertBridgeLogValue(t *testing.T, call bridgeLogCall, key string, want any) {
	t.Helper()
	for i := 0; i+1 < len(call.kv); i += 2 {
		if call.kv[i] == key && reflect.DeepEqual(call.kv[i+1], want) {
			return
		}
	}
	t.Fatalf("log %+v missing %q=%#v", call, key, want)
}
