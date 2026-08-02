package events

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

// fakeAppendService is a minimal detached Service consumer fixture. It proves
// the published contract is satisfiable and that AppendResult's commit-order
// and duplicate-identity guarantees are coherent, without introducing any
// production implementation, persistence, or wire construction.
type fakeAttachmentKey struct {
	DestinationTopic TopicID
	SourceTopic      TopicID
	SourceType       SourceType
	SourceID         SourceID
}

type fakeAttachment struct {
	ID    AttachmentID
	Start AttachStartPosition
}

type fakeAppendService struct {
	byKey   map[IdempotencyKey]Record
	byTopic map[TopicID][]Record
	nextSeq map[TopicID]AggregateSequence
	genID   StreamGeneration

	attachments   map[fakeAttachmentKey]fakeAttachment
	nextAttachSeq int
}

var _ Service = (*fakeAppendService)(nil)

func newFakeAppendService() *fakeAppendService {
	return &fakeAppendService{
		byKey:       make(map[IdempotencyKey]Record),
		byTopic:     make(map[TopicID][]Record),
		nextSeq:     make(map[TopicID]AggregateSequence),
		genID:       1,
		attachments: make(map[fakeAttachmentKey]fakeAttachment),
	}
}

func (f *fakeAppendService) AttachSource(_ context.Context, req AttachSourceRequest) (AttachSourceResult, error) {
	if err := req.Validate(); err != nil {
		return AttachSourceResult{}, err
	}

	key := fakeAttachmentKey{
		DestinationTopic: req.DestinationTopic,
		SourceTopic:      req.SourceTopic,
		SourceType:       req.SourceType,
		SourceID:         req.SourceID,
	}
	if existing, ok := f.attachments[key]; ok {
		if existing.Start != req.Start {
			return AttachSourceResult{AttachmentID: existing.ID, Outcome: AttachOutcomeConflict}, nil
		}
		return AttachSourceResult{AttachmentID: existing.ID, Outcome: AttachOutcomeAlreadyAttached}, nil
	}

	f.nextAttachSeq++
	attachment := fakeAttachment{
		ID:    AttachmentID(fmt.Sprintf("attachment-%d", f.nextAttachSeq)),
		Start: req.Start,
	}
	f.attachments[key] = attachment
	return AttachSourceResult{AttachmentID: attachment.ID, Outcome: AttachOutcomeAttached}, nil
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

func TestServiceAttachSourceIsIdempotentAndDetectsConflict(t *testing.T) {
	svc := newFakeAppendService()
	ctx := context.Background()

	req := AttachSourceRequest{
		DestinationTopic: "chat-session/c1/events",
		SourceTopic:      "factory-session/s1/response-events",
		SourceType:       "factory_session",
		SourceID:         "s1",
		Start:            AttachStartPosition{Mode: AttachStartBeginning},
	}

	first, err := svc.AttachSource(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error on first attach: %v", err)
	}
	if first.Outcome != AttachOutcomeAttached {
		t.Fatalf("expected AttachOutcomeAttached, got %v", first.Outcome)
	}
	if first.AttachmentID == "" {
		t.Fatalf("expected a non-empty AttachmentID")
	}

	repeat, err := svc.AttachSource(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error on repeated attach: %v", err)
	}
	if repeat.Outcome != AttachOutcomeAlreadyAttached {
		t.Fatalf("expected AttachOutcomeAlreadyAttached for a repeated identical request, got %v", repeat.Outcome)
	}
	if repeat.AttachmentID != first.AttachmentID {
		t.Fatalf("expected repeated attach to return the original AttachmentID %v, got %v", first.AttachmentID, repeat.AttachmentID)
	}

	conflicting := req
	conflicting.Start = AttachStartPosition{Mode: AttachStartLiveHead}
	conflict, err := svc.AttachSource(ctx, conflicting)
	if err != nil {
		t.Fatalf("unexpected error on conflicting attach: %v", err)
	}
	if conflict.Outcome != AttachOutcomeConflict {
		t.Fatalf("expected AttachOutcomeConflict for a differing Start on the same source relationship, got %v", conflict.Outcome)
	}
	if conflict.AttachmentID != first.AttachmentID {
		t.Fatalf("expected conflict outcome to still identify the original AttachmentID %v, got %v", first.AttachmentID, conflict.AttachmentID)
	}
}

func TestServiceAttachSourceRejectsInvalidRequest(t *testing.T) {
	svc := newFakeAppendService()

	_, err := svc.AttachSource(context.Background(), AttachSourceRequest{
		DestinationTopic: "chat-session/c1/events",
		SourceTopic:      "chat-session/c1/events",
		SourceType:       "factory_session",
		SourceID:         "s1",
		Start:            AttachStartPosition{Mode: AttachStartBeginning},
	})
	if !errors.Is(err, ErrSelfAttachment) {
		t.Fatalf("expected errors.Is(err, ErrSelfAttachment), got %v", err)
	}
}
