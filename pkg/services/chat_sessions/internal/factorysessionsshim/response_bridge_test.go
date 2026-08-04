package factorysessionsshim

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
	"github.com/portpowered/infinite-you/pkg/services/events"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// fakeSequencer records every Sequence/AdvanceStreamHead call it receives and
// returns pre-scripted results/errors in call order, so a test can assert
// both what BridgeFactoryResponseEvents committed and the exact order it
// committed it in.
type fakeSequencer struct {
	sequenceCalls  []chatsessions.SequenceRequest
	sequenceResult []chatsessions.SequenceResult
	sequenceErr    []error

	advanceCalls  []chatsessions.AdvanceStreamHeadRequest
	advanceResult []chatsessions.AdvanceStreamHeadResult
	advanceErr    []error
}

func (f *fakeSequencer) Sequence(_ context.Context, req chatsessions.SequenceRequest) (chatsessions.SequenceResult, error) {
	i := len(f.sequenceCalls)
	f.sequenceCalls = append(f.sequenceCalls, req)
	var result chatsessions.SequenceResult
	if i < len(f.sequenceResult) {
		result = f.sequenceResult[i]
	}
	var err error
	if i < len(f.sequenceErr) {
		err = f.sequenceErr[i]
	}
	return result, err
}

func (f *fakeSequencer) AdvanceStreamHead(_ context.Context, req chatsessions.AdvanceStreamHeadRequest) (chatsessions.AdvanceStreamHeadResult, error) {
	i := len(f.advanceCalls)
	f.advanceCalls = append(f.advanceCalls, req)
	var result chatsessions.AdvanceStreamHeadResult
	if i < len(f.advanceResult) {
		result = f.advanceResult[i]
	}
	var err error
	if i < len(f.advanceErr) {
		err = f.advanceErr[i]
	}
	return result, err
}

// fakeSubscriber returns a pre-built cursor (or error) for the one
// SubscribeFactoryResponseEvents call BridgeFactoryResponseEvents makes.
type fakeSubscriber struct {
	req    factorysessions.ResponseEventSubscriptionRequest
	cursor *factorysessions.ResponseEventCursor
	err    error
}

func (f *fakeSubscriber) SubscribeFactoryResponseEvents(
	_ context.Context,
	req factorysessions.ResponseEventSubscriptionRequest,
) (*factorysessions.ResponseEventCursor, error) {
	f.req = req
	return f.cursor, f.err
}

// newBatchCursor returns a ResponseEventCursor whose Next calls return each
// element of batches in turn, then ErrResponseEventSubscriptionClosed.
func newBatchCursor(batches [][]factorysessions.FactoryResponseEvent) (*factorysessions.ResponseEventCursor, *int) {
	calls := 0
	return &factorysessions.ResponseEventCursor{
		NextEvents: func(ctx context.Context) ([]factorysessions.FactoryResponseEvent, error) {
			if calls < len(batches) {
				batch := batches[calls]
				calls++
				return batch, nil
			}
			return nil, factorysessions.ErrResponseEventSubscriptionClosed
		},
		DrainEvents:  func() ([]factorysessions.FactoryResponseEvent, error) { return nil, nil },
		DetachCursor: func() { calls = -1 },
	}, &calls
}

func TestBridgeFactoryResponseEvents_SequencesEventsInOrderAndRemapsParentItemID(t *testing.T) {
	parent := factorysessions.FactoryResponseEvent{
		FactorySessionID: "factory-1",
		EventID:          "evt-1",
		Sequence:         1,
		ItemID:           "factory-item-1",
		Kind:             workers.KindMessage,
		Phase:            workers.PhaseCompleted,
		Payload:          json.RawMessage(`{"role":"ASSISTANT"}`),
	}
	child := factorysessions.FactoryResponseEvent{
		FactorySessionID: "factory-1",
		EventID:          "evt-2",
		Sequence:         2,
		ItemID:           "factory-item-2",
		ParentItemID:     "factory-item-1",
		Kind:             workers.KindReasoning,
		Phase:            workers.PhaseCompleted,
		Payload:          json.RawMessage(`{"summary":"hi"}`),
	}

	cursor, _ := newBatchCursor([][]factorysessions.FactoryResponseEvent{{parent, child}})
	subscriber := &fakeSubscriber{cursor: cursor}
	sequencer := &fakeSequencer{
		sequenceResult: []chatsessions.SequenceResult{
			{ItemID: "chat-item-1", AggregateSequence: 10},
			{ItemID: "chat-item-2", AggregateSequence: 11},
		},
		advanceResult: []chatsessions.AdvanceStreamHeadResult{
			{Session: chatsessions.Session{Version: 6}},
			{Session: chatsessions.Session{Version: 7}},
		},
	}

	if err := BridgeFactoryResponseEvents(context.Background(), sequencer, subscriber, "chat-1", 5, "factory-1"); err != nil {
		t.Fatalf("BridgeFactoryResponseEvents() error = %v, want nil", err)
	}

	if subscriber.req.SessionID != "factory-1" {
		t.Errorf("subscribe SessionID = %q, want %q", subscriber.req.SessionID, "factory-1")
	}
	if len(sequencer.sequenceCalls) != 2 {
		t.Fatalf("Sequence calls = %d, want 2", len(sequencer.sequenceCalls))
	}

	first := sequencer.sequenceCalls[0]
	if first.SessionID != "chat-1" || first.SourceID != events.SourceID("factory-1") ||
		first.SourceSequence != events.SourceSequence(1) || first.SourceEventID != events.SourceEventID("evt-1") ||
		first.Kind != workers.KindMessage || first.Phase != workers.PhaseCompleted || first.ParentItemID != "" {
		t.Errorf("first Sequence call = %+v, unexpected fields", first)
	}

	second := sequencer.sequenceCalls[1]
	if second.ParentItemID != "chat-item-1" {
		t.Errorf("second Sequence call ParentItemID = %q, want remapped %q", second.ParentItemID, "chat-item-1")
	}

	if len(sequencer.advanceCalls) != 2 {
		t.Fatalf("AdvanceStreamHead calls = %d, want 2", len(sequencer.advanceCalls))
	}
	if sequencer.advanceCalls[0].ExpectedVersion != 5 {
		t.Errorf("first AdvanceStreamHead ExpectedVersion = %d, want 5 (initial sessionVersion)", sequencer.advanceCalls[0].ExpectedVersion)
	}
	if sequencer.advanceCalls[1].ExpectedVersion != 6 {
		t.Errorf("second AdvanceStreamHead ExpectedVersion = %d, want 6 (result of first advance)", sequencer.advanceCalls[1].ExpectedVersion)
	}
}

func TestBridgeFactoryResponseEvents_SubscribeFailureReturnsErrorWithoutSequencing(t *testing.T) {
	subscribeErr := errors.New("subscribe boom")
	subscriber := &fakeSubscriber{err: subscribeErr}
	sequencer := &fakeSequencer{}

	err := BridgeFactoryResponseEvents(context.Background(), sequencer, subscriber, "chat-1", 5, "factory-1")
	if !errors.Is(err, subscribeErr) {
		t.Fatalf("error = %v, want %v", err, subscribeErr)
	}
	if len(sequencer.sequenceCalls) != 0 {
		t.Errorf("Sequence calls = %d, want 0", len(sequencer.sequenceCalls))
	}
}

func TestBridgeFactoryResponseEvents_SequenceFailureStopsDrainImmediately(t *testing.T) {
	event := factorysessions.FactoryResponseEvent{
		FactorySessionID: "factory-1", EventID: "evt-1", Sequence: 1,
		Kind: workers.KindMessage, Phase: workers.PhaseCompleted,
		Payload: json.RawMessage(`{}`),
	}
	nextEvent := factorysessions.FactoryResponseEvent{
		FactorySessionID: "factory-1", EventID: "evt-2", Sequence: 2,
		Kind: workers.KindMessage, Phase: workers.PhaseCompleted,
		Payload: json.RawMessage(`{}`),
	}
	cursor, _ := newBatchCursor([][]factorysessions.FactoryResponseEvent{{event, nextEvent}})
	subscriber := &fakeSubscriber{cursor: cursor}
	sequenceErr := errors.New("sequence boom")
	sequencer := &fakeSequencer{sequenceErr: []error{sequenceErr}}

	err := BridgeFactoryResponseEvents(context.Background(), sequencer, subscriber, "chat-1", 5, "factory-1")
	if !errors.Is(err, sequenceErr) {
		t.Fatalf("error = %v, want %v", err, sequenceErr)
	}
	if len(sequencer.sequenceCalls) != 1 {
		t.Errorf("Sequence calls = %d, want 1 (drain must stop at the first failure)", len(sequencer.sequenceCalls))
	}
}

func TestBridgeFactoryResponseEvents_AdvanceStreamHeadFailureStopsDrain(t *testing.T) {
	event := factorysessions.FactoryResponseEvent{
		FactorySessionID: "factory-1", EventID: "evt-1", Sequence: 1,
		Kind: workers.KindMessage, Phase: workers.PhaseCompleted,
		Payload: json.RawMessage(`{}`),
	}
	cursor, _ := newBatchCursor([][]factorysessions.FactoryResponseEvent{{event}})
	subscriber := &fakeSubscriber{cursor: cursor}
	advanceErr := errors.New("advance boom")
	sequencer := &fakeSequencer{
		sequenceResult: []chatsessions.SequenceResult{{ItemID: "chat-item-1", AggregateSequence: 10}},
		advanceErr:     []error{advanceErr},
	}

	err := BridgeFactoryResponseEvents(context.Background(), sequencer, subscriber, "chat-1", 5, "factory-1")
	if !errors.Is(err, advanceErr) {
		t.Fatalf("error = %v, want %v", err, advanceErr)
	}
}

func TestBridgeFactoryResponseEvents_ContextCancellationPropagates(t *testing.T) {
	cursor := &factorysessions.ResponseEventCursor{
		NextEvents: func(ctx context.Context) ([]factorysessions.FactoryResponseEvent, error) {
			return nil, ctx.Err()
		},
		DrainEvents:  func() ([]factorysessions.FactoryResponseEvent, error) { return nil, nil },
		DetachCursor: func() {},
	}
	subscriber := &fakeSubscriber{cursor: cursor}
	sequencer := &fakeSequencer{}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := BridgeFactoryResponseEvents(ctx, sequencer, subscriber, "chat-1", 5, "factory-1")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}

func TestBridgeFactoryResponseEvents_DetachesCursorOnGracefulClose(t *testing.T) {
	detached := false
	cursor := &factorysessions.ResponseEventCursor{
		NextEvents: func(context.Context) ([]factorysessions.FactoryResponseEvent, error) {
			return nil, factorysessions.ErrResponseEventSubscriptionClosed
		},
		DrainEvents:  func() ([]factorysessions.FactoryResponseEvent, error) { return nil, nil },
		DetachCursor: func() { detached = true },
	}
	subscriber := &fakeSubscriber{cursor: cursor}
	sequencer := &fakeSequencer{}

	if err := BridgeFactoryResponseEvents(context.Background(), sequencer, subscriber, "chat-1", 5, "factory-1"); err != nil {
		t.Fatalf("BridgeFactoryResponseEvents() error = %v, want nil", err)
	}
	if !detached {
		t.Error("cursor was never detached")
	}
}

func TestRunWithResponseBridge_ReturnsInvokeResultAndErrorUnchanged(t *testing.T) {
	event := factorysessions.FactoryResponseEvent{
		FactorySessionID: "factory-1", EventID: "evt-1", Sequence: 1,
		Kind: workers.KindMessage, Phase: workers.PhaseCompleted,
		Payload: json.RawMessage(`{}`),
	}
	cursor, _ := newBatchCursor([][]factorysessions.FactoryResponseEvent{{event}})
	subscriber := &fakeSubscriber{cursor: cursor}
	sequencer := &fakeSequencer{
		sequenceResult: []chatsessions.SequenceResult{{ItemID: "chat-item-1", AggregateSequence: 10}},
		advanceResult:  []chatsessions.AdvanceStreamHeadResult{{Session: chatsessions.Session{Version: 6}}},
	}

	wantResult := factorysessions.InvocationResult{Status: factorysessions.InvocationTerminalStatusCompleted}
	wantErr := errors.New("invoke boom")
	invokeCalls := 0
	invoke := func(ctx context.Context) (factorysessions.InvocationResult, error) {
		invokeCalls++
		return wantResult, wantErr
	}

	result, err := RunWithResponseBridge(context.Background(), sequencer, subscriber, "chat-1", 5, "factory-1", nil, invoke)

	if invokeCalls != 1 {
		t.Fatalf("invoke called %d times, want 1", invokeCalls)
	}
	if result.Status != wantResult.Status {
		t.Errorf("result.Status = %v, want %v", result.Status, wantResult.Status)
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("error = %v, want %v", err, wantErr)
	}
}

func TestRunWithResponseBridge_BridgeFailureNeverPropagatesToInvokeResult(t *testing.T) {
	subscriber := &fakeSubscriber{err: errors.New("subscribe boom")}
	sequencer := &fakeSequencer{}

	wantResult := factorysessions.InvocationResult{Status: factorysessions.InvocationTerminalStatusCompleted}
	invoke := func(ctx context.Context) (factorysessions.InvocationResult, error) {
		return wantResult, nil
	}

	result, err := RunWithResponseBridge(context.Background(), sequencer, subscriber, "chat-1", 5, "factory-1", nil, invoke)
	if err != nil {
		t.Fatalf("error = %v, want nil (a bridge failure must never propagate)", err)
	}
	if result.Status != wantResult.Status {
		t.Errorf("result.Status = %v, want %v", result.Status, wantResult.Status)
	}
}

func TestRunWithResponseBridge_StopsBridgeAfterInvokeReturns(t *testing.T) {
	bridgeStopped := make(chan struct{})
	cursor := &factorysessions.ResponseEventCursor{
		NextEvents: func(ctx context.Context) ([]factorysessions.FactoryResponseEvent, error) {
			<-ctx.Done()
			close(bridgeStopped)
			return nil, ctx.Err()
		},
		DrainEvents:  func() ([]factorysessions.FactoryResponseEvent, error) { return nil, nil },
		DetachCursor: func() {},
	}
	subscriber := &fakeSubscriber{cursor: cursor}
	sequencer := &fakeSequencer{}

	invoke := func(ctx context.Context) (factorysessions.InvocationResult, error) {
		return factorysessions.InvocationResult{}, nil
	}

	_, _ = RunWithResponseBridge(context.Background(), sequencer, subscriber, "chat-1", 5, "factory-1", nil, invoke)

	select {
	case <-bridgeStopped:
	case <-time.After(time.Second):
		t.Fatal("bridge goroutine never observed cancellation after invoke returned; it may have leaked")
	}
}

// TestRunWithResponseBridge_RunsLiveDrainConcurrentlyWithInvoke proves
// liveDrain actually starts before invoke returns, not only after -- the
// genuine mid-generation delivery behavior this parameter exists for, as
// distinct from the pre-existing Factory response-event bridge which runs
// concurrently but on the producer side only.
func TestRunWithResponseBridge_RunsLiveDrainConcurrentlyWithInvoke(t *testing.T) {
	cursor := &factorysessions.ResponseEventCursor{
		NextEvents: func(ctx context.Context) ([]factorysessions.FactoryResponseEvent, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		},
		DrainEvents:  func() ([]factorysessions.FactoryResponseEvent, error) { return nil, nil },
		DetachCursor: func() {},
	}
	subscriber := &fakeSubscriber{cursor: cursor}
	sequencer := &fakeSequencer{}

	liveDrainStarted := make(chan struct{})
	liveDrain := func(ctx context.Context) {
		close(liveDrainStarted)
		<-ctx.Done()
	}

	invoke := func(ctx context.Context) (factorysessions.InvocationResult, error) {
		select {
		case <-liveDrainStarted:
		case <-time.After(time.Second):
			t.Error("liveDrain never started before invoke returned")
		}
		return factorysessions.InvocationResult{}, nil
	}

	if _, err := RunWithResponseBridge(context.Background(), sequencer, subscriber, "chat-1", 5, "factory-1", liveDrain, invoke); err != nil {
		t.Fatalf("RunWithResponseBridge() error = %v, want nil", err)
	}
}

// TestRunWithResponseBridge_StopsLiveDrainAfterInvokeReturns mirrors
// TestRunWithResponseBridge_StopsBridgeAfterInvokeReturns for the liveDrain
// goroutine: it must never outlive RunWithResponseBridge itself.
func TestRunWithResponseBridge_StopsLiveDrainAfterInvokeReturns(t *testing.T) {
	cursor := &factorysessions.ResponseEventCursor{
		NextEvents: func(ctx context.Context) ([]factorysessions.FactoryResponseEvent, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		},
		DrainEvents:  func() ([]factorysessions.FactoryResponseEvent, error) { return nil, nil },
		DetachCursor: func() {},
	}
	subscriber := &fakeSubscriber{cursor: cursor}
	sequencer := &fakeSequencer{}

	drainStopped := make(chan struct{})
	liveDrain := func(ctx context.Context) {
		<-ctx.Done()
		close(drainStopped)
	}

	invoke := func(ctx context.Context) (factorysessions.InvocationResult, error) {
		return factorysessions.InvocationResult{}, nil
	}

	_, _ = RunWithResponseBridge(context.Background(), sequencer, subscriber, "chat-1", 5, "factory-1", liveDrain, invoke)

	select {
	case <-drainStopped:
	case <-time.After(time.Second):
		t.Fatal("liveDrain goroutine never observed cancellation after invoke returned; it may have leaked")
	}
}
