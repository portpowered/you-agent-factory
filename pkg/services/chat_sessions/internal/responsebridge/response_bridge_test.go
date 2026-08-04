package responsebridge

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sync"
	"testing"

	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
	"github.com/portpowered/infinite-you/pkg/services/events"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

type bridgeSequencer struct {
	mu        sync.Mutex
	sequences []chatsessions.SequenceRequest
	advances  []chatsessions.AdvanceStreamHeadRequest
	didFirst  chan struct{}
}

func (s *bridgeSequencer) Sequence(_ context.Context, req chatsessions.SequenceRequest) (chatsessions.SequenceResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sequences = append(s.sequences, req)
	if len(s.sequences) == 1 {
		close(s.didFirst)
	}
	return chatsessions.SequenceResult{ItemID: "chat-item-" + string(rune('0'+len(s.sequences))), AggregateSequence: events.AggregateSequence(len(s.sequences))}, nil
}

func (s *bridgeSequencer) AdvanceStreamHead(_ context.Context, req chatsessions.AdvanceStreamHeadRequest) (chatsessions.AdvanceStreamHeadResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.advances = append(s.advances, req)
	return chatsessions.AdvanceStreamHeadResult{Session: chatsessions.Session{Version: req.ExpectedVersion + 1}}, nil
}

type bridgeTarget struct {
	factorysessions.TargetExecutionService
	cursor *factorysessions.ResponseEventCursor
	err    error
}

func (t bridgeTarget) SubscribeFactoryResponseEvents(_ context.Context, _ factorysessions.ResponseEventSubscriptionRequest) (*factorysessions.ResponseEventCursor, error) {
	return t.cursor, t.err
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
		DetachCursor: func() {},
	}

	want := factorysessions.InvocationResult{RequestID: "turn-1", Status: factorysessions.InvocationTerminalStatusCompleted}
	got, err := New(sequencer).Run(context.Background(), bridgeTarget{cursor: cursor}, "chat-1", 4, "factory-1", nil,
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

func TestServiceRunDoesNotReplaceInvokeFailureWhenSubscriptionFails(t *testing.T) {
	wantErr := errors.New("invoke failed")
	want := factorysessions.InvocationResult{Status: factorysessions.InvocationTerminalStatusFailed}
	got, err := New(&bridgeSequencer{didFirst: make(chan struct{})}).Run(
		context.Background(), bridgeTarget{err: errors.New("subscribe failed")}, "chat-1", 1, "factory-1", nil,
		func(context.Context) (factorysessions.InvocationResult, error) { return want, wantErr },
	)
	if !errors.Is(err, wantErr) || !reflect.DeepEqual(got, want) {
		t.Fatalf("Run() = (%+v, %v), want (%+v, %v)", got, err, want, wantErr)
	}
}
