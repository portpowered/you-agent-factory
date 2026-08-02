package events

import (
	"context"
	"errors"
	"testing"
)

// fakeAppendService is a minimal detached Service consumer fixture. It proves
// the published contract is satisfiable and that AppendResult's commit-order
// and duplicate-identity guarantees are coherent, without introducing any
// production implementation, persistence, or wire construction.
type fakeAppendService struct {
	byKey   map[IdempotencyKey]Record
	byTopic map[TopicID][]Record
	nextSeq map[TopicID]AggregateSequence
	genID   StreamGeneration
}

var _ Service = (*fakeAppendService)(nil)

func newFakeAppendService() *fakeAppendService {
	return &fakeAppendService{
		byKey:   make(map[IdempotencyKey]Record),
		byTopic: make(map[TopicID][]Record),
		nextSeq: make(map[TopicID]AggregateSequence),
		genID:   1,
	}
}

func (f *fakeAppendService) Append(_ context.Context, req AppendRequest) (AppendResult, error) {
	if err := req.Validate(); err != nil {
		return AppendResult{}, err
	}

	key := req.Key()
	if existing, ok := f.byKey[key]; ok {
		return AppendResult{Record: existing, Outcome: AppendOutcomeDuplicate}, nil
	}

	f.nextSeq[req.Topic]++
	record := Record{
		Topic:             req.Topic,
		SourceType:        req.SourceType,
		SourceID:          req.SourceID,
		SourceSequence:    req.SourceSequence,
		SourceEventID:     req.SourceEventID,
		Schema:            req.Schema,
		AggregateSequence: f.nextSeq[req.Topic],
		Generation:        f.genID,
		Payload:           req.Payload,
	}
	f.byKey[key] = record
	f.byTopic[req.Topic] = append(f.byTopic[req.Topic], record)
	return AppendResult{Record: record, Outcome: AppendOutcomeCommitted}, nil
}

func TestServiceAppendCommitsInOrderAndSuppressesDuplicates(t *testing.T) {
	svc := newFakeAppendService()
	ctx := context.Background()
	topic := TopicID("factory-session/s1/response-events")

	first, err := svc.Append(ctx, AppendRequest{
		Topic:          topic,
		SourceType:     "factory_session",
		SourceID:       "s1",
		SourceSequence: 1,
		SourceEventID:  "evt-1",
		Schema:         "factory_session.response_event.v1",
		Payload:        "first",
	})
	if err != nil {
		t.Fatalf("unexpected error on first append: %v", err)
	}
	if first.Outcome != AppendOutcomeCommitted {
		t.Fatalf("expected AppendOutcomeCommitted, got %v", first.Outcome)
	}
	if first.Record.AggregateSequence != 1 {
		t.Fatalf("expected AggregateSequence 1, got %d", first.Record.AggregateSequence)
	}

	second, err := svc.Append(ctx, AppendRequest{
		Topic:          topic,
		SourceType:     "factory_session",
		SourceID:       "s1",
		SourceSequence: 2,
		SourceEventID:  "evt-2",
		Schema:         "factory_session.response_event.v1",
		Payload:        "second",
	})
	if err != nil {
		t.Fatalf("unexpected error on second append: %v", err)
	}
	if second.Record.AggregateSequence != 2 {
		t.Fatalf("expected commit-order AggregateSequence 2, got %d", second.Record.AggregateSequence)
	}

	duplicate, err := svc.Append(ctx, AppendRequest{
		Topic:          topic,
		SourceType:     "factory_session",
		SourceID:       "s1",
		SourceSequence: 1,
		SourceEventID:  "evt-1",
		Schema:         "factory_session.response_event.v1",
		Payload:        "resent with a different payload value",
	})
	if err != nil {
		t.Fatalf("unexpected error on duplicate append: %v", err)
	}
	if duplicate.Outcome != AppendOutcomeDuplicate {
		t.Fatalf("expected AppendOutcomeDuplicate for a repeated idempotency tuple, got %v", duplicate.Outcome)
	}
	if duplicate.Record.AggregateSequence != first.Record.AggregateSequence {
		t.Fatalf("expected duplicate delivery to return the original stable AggregateSequence %d, got %d",
			first.Record.AggregateSequence, duplicate.Record.AggregateSequence)
	}
}

func TestServiceAppendRejectsInvalidEnvelope(t *testing.T) {
	svc := newFakeAppendService()

	_, err := svc.Append(context.Background(), AppendRequest{
		Topic:          "",
		SourceType:     "factory_session",
		SourceID:       "s1",
		SourceSequence: 1,
		SourceEventID:  "evt-1",
		Schema:         "factory_session.response_event.v1",
	})
	if !errors.Is(err, ErrInvalidTopic) {
		t.Fatalf("expected errors.Is(err, ErrInvalidTopic), got %v", err)
	}
}
