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
	byKey    map[IdempotencyKey]Record
	byTopic  map[TopicID][]Record
	nextSeq  map[TopicID]AggregateSequence
	earliest map[TopicID]AggregateSequence
	genID    StreamGeneration

	attachments   map[fakeAttachmentKey]fakeAttachment
	nextAttachSeq int

	retention map[TopicID]RetentionLimits
	completed map[TopicID]bool
}

var _ Service = (*fakeAppendService)(nil)

func newFakeAppendService() *fakeAppendService {
	return &fakeAppendService{
		byKey:       make(map[IdempotencyKey]Record),
		byTopic:     make(map[TopicID][]Record),
		nextSeq:     make(map[TopicID]AggregateSequence),
		earliest:    make(map[TopicID]AggregateSequence),
		genID:       1,
		attachments: make(map[fakeAttachmentKey]fakeAttachment),
		retention:   make(map[TopicID]RetentionLimits),
		completed:   make(map[TopicID]bool),
	}
}

// complete marks topic as reaching a defined completion, the fixture
// condition fakeSubscription reports as SubscriptionTerminalCompleted once
// every committed Record has been delivered. It is test-fixture behavior
// only; Events defines no production topic-completion policy.
func (f *fakeAppendService) complete(topic TopicID) {
	f.completed[topic] = true
}

// setRetention bounds topic to limits, evicting from the front of its
// currently retained Records whenever a subsequent Append grows past
// limits.MaxRecords. It is test-fixture behavior only, proving the Gap and
// RetainedRange contract is satisfiable; it defines no production retention
// policy.
func (f *fakeAppendService) setRetention(topic TopicID, limits RetentionLimits) {
	f.retention[topic] = limits
}

func (f *fakeAppendService) evict(topic TopicID) {
	limits, ok := f.retention[topic]
	if !ok {
		return
	}
	records := f.byTopic[topic]
	if int64(len(records)) <= limits.MaxRecords {
		return
	}
	excess := int64(len(records)) - limits.MaxRecords
	f.byTopic[topic] = records[excess:]
	f.earliest[topic] = f.byTopic[topic][0].AggregateSequence
}

func (f *fakeAppendService) earliestOf(topic TopicID) AggregateSequence {
	if earliest, ok := f.earliest[topic]; ok {
		return earliest
	}
	return 1
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
	f.evict(req.Topic)
	return AppendResult{Record: record, Outcome: AppendOutcomeCommitted}, nil
}

func (f *fakeAppendService) Read(_ context.Context, req ReadRequest) (ReadResult, error) {
	if err := req.Validate(); err != nil {
		return ReadResult{}, err
	}

	if _, known := f.nextSeq[req.Topic]; !known {
		return ReadResult{}, &ValidationError{Field: "topic", Err: ErrTopicNotFound}
	}

	head := f.nextSeq[req.Topic]
	if status := req.After.ClassifyAgainst(req.Topic, f.genID); status == CursorStatusStaleGeneration {
		return ReadResult{}, &ValidationError{Field: "after.generation", Err: ErrCursorStaleGeneration}
	}

	earliest := f.earliestOf(req.Topic)
	retained := RetainedRange{Topic: req.Topic, Earliest: earliest, Head: head}

	var startAfter AggregateSequence
	switch req.After.Mode {
	case CursorBeginning:
		startAfter = 0
	case CursorAt:
		startAfter = req.After.At
	case CursorLiveHead:
		startAfter = head
	}

	if startAfter < earliest-1 {
		gap := &Gap{Topic: req.Topic, From: startAfter + 1, To: earliest - 1, ResumeAt: earliest}
		return ReadResult{
			Retained: retained,
			Outcome:  ReadOutcomeGap,
			Gap:      gap,
		}, nil
	}

	var matching []Record
	for _, record := range f.byTopic[req.Topic] {
		if record.AggregateSequence > startAfter {
			matching = append(matching, record)
		}
	}

	if len(matching) > req.Limit {
		truncated := matching[:req.Limit]
		last := truncated[len(truncated)-1]
		return ReadResult{
			Records:    truncated,
			NextCursor: Cursor{Topic: req.Topic, Generation: f.genID, Mode: CursorAt, At: last.AggregateSequence},
			Retained:   retained,
			Outcome:    ReadOutcomeTruncated,
		}, nil
	}

	return ReadResult{
		Records:    matching,
		NextCursor: Cursor{Topic: req.Topic, Generation: f.genID, Mode: CursorLiveHead},
		Retained:   retained,
		Outcome:    ReadOutcomeComplete,
	}, nil
}

// fakeSubscription is a minimal detached Subscription fixture proving
// SubscribeRequest/SubscriptionDelivery is satisfiable: it classifies every
// published SubscriptionTerminalReason and hands off retained backlog into
// subsequent live commits without blocking or losing a Record silently. It
// polls fakeAppendService's in-memory state rather than genuinely blocking
// on new commits; that is fixture behavior only, not a mandated Subscribe
// implementation shape.
type fakeSubscription struct {
	svc       *fakeAppendService
	topic     TopicID
	capacity  int
	start     Cursor
	delivered AggregateSequence
	validated bool
	done      bool
}

func (f *fakeAppendService) Subscribe(_ context.Context, req SubscribeRequest) (SubscribeResult, error) {
	if err := req.Validate(); err != nil {
		return SubscribeResult{}, err
	}

	sub := &fakeSubscription{svc: f, topic: req.Topic, capacity: req.Capacity, start: req.Start}
	switch req.Start.Mode {
	case CursorAt:
		sub.delivered = req.Start.At
	case CursorLiveHead:
		sub.delivered = f.nextSeq[req.Topic]
	}
	return SubscribeResult{Subscription: sub.Next}, nil
}

func (s *fakeSubscription) cursor() Cursor {
	if s.delivered == 0 {
		return Cursor{Topic: s.topic, Generation: s.svc.genID, Mode: CursorBeginning}
	}
	return Cursor{Topic: s.topic, Generation: s.svc.genID, Mode: CursorAt, At: s.delivered}
}

func (s *fakeSubscription) terminal(reason SubscriptionTerminalReason, gap *Gap) SubscriptionDelivery {
	s.done = true
	return SubscriptionDelivery{
		Outcome:  SubscriptionOutcomeTerminal,
		Terminal: &SubscriptionTerminal{Reason: reason, Cursor: s.cursor(), Gap: gap},
	}
}

func (s *fakeSubscription) Next(ctx context.Context) SubscriptionDelivery {
	if s.done {
		return s.terminal(SubscriptionTerminalCanceled, nil)
	}
	select {
	case <-ctx.Done():
		return s.terminal(SubscriptionTerminalCanceled, nil)
	default:
	}

	if !s.validated {
		s.validated = true
		if _, known := s.svc.nextSeq[s.topic]; !known {
			return s.terminal(SubscriptionTerminalInvalidCursor, nil)
		}
		if status := s.start.ClassifyAgainst(s.topic, s.svc.genID); status == CursorStatusStaleGeneration {
			return s.terminal(SubscriptionTerminalInvalidCursor, nil)
		}
		earliest := s.svc.earliestOf(s.topic)
		if s.delivered < earliest-1 {
			return s.terminal(SubscriptionTerminalGap, &Gap{
				Topic: s.topic, From: s.delivered + 1, To: earliest - 1, ResumeAt: earliest,
			})
		}
	}

	var matching []Record
	for _, record := range s.svc.byTopic[s.topic] {
		if record.AggregateSequence > s.delivered {
			matching = append(matching, record)
		}
	}

	if len(matching) > s.capacity {
		return s.terminal(SubscriptionTerminalBackpressure, nil)
	}

	if len(matching) == 0 {
		if s.svc.completed[s.topic] {
			return s.terminal(SubscriptionTerminalCompleted, nil)
		}
		return SubscriptionDelivery{Outcome: SubscriptionOutcomeRecords, NextCursor: s.cursor()}
	}

	s.delivered = matching[len(matching)-1].AggregateSequence
	return SubscriptionDelivery{Outcome: SubscriptionOutcomeRecords, Records: matching, NextCursor: s.cursor()}
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

func appendTestRecord(t *testing.T, svc *fakeAppendService, topic TopicID, sourceSeq SourceSequence) Record {
	t.Helper()
	result, err := svc.Append(context.Background(), AppendRequest{
		Topic:          topic,
		SourceType:     "factory_session",
		SourceID:       "s1",
		SourceSequence: sourceSeq,
		SourceEventID:  SourceEventID(fmt.Sprintf("evt-%d", sourceSeq)),
		Schema:         "factory_session.response_event.v1",
		Payload:        fmt.Sprintf("payload-%d", sourceSeq),
	})
	if err != nil {
		t.Fatalf("unexpected error appending fixture record: %v", err)
	}
	return result.Record
}

func TestServiceReadReturnsOrderedRecordsAndReportsCompletion(t *testing.T) {
	svc := newFakeAppendService()
	ctx := context.Background()
	topic := TopicID("factory-session/s1/response-events")

	appendTestRecord(t, svc, topic, 1)
	second := appendTestRecord(t, svc, topic, 2)

	result, err := svc.Read(ctx, ReadRequest{
		Topic: topic,
		After: Cursor{Topic: topic, Generation: 1, Mode: CursorBeginning},
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Outcome != ReadOutcomeComplete {
		t.Fatalf("expected ReadOutcomeComplete, got %v", result.Outcome)
	}
	if len(result.Records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(result.Records))
	}
	if result.Records[0].AggregateSequence != 1 || result.Records[1].AggregateSequence != 2 {
		t.Fatalf("expected records in commit order, got %+v", result.Records)
	}
	if result.NextCursor.Mode != CursorLiveHead {
		t.Fatalf("expected a live-head NextCursor after reaching head, got %+v", result.NextCursor)
	}
	if result.Retained.Head != second.AggregateSequence {
		t.Fatalf("expected retained head %d, got %d", second.AggregateSequence, result.Retained.Head)
	}

	// Reading again from the live head, with nothing new committed, is a
	// normal empty-at-head completion, not an error or a gap.
	again, err := svc.Read(ctx, ReadRequest{Topic: topic, After: result.NextCursor, Limit: 10})
	if err != nil {
		t.Fatalf("unexpected error on empty-at-head read: %v", err)
	}
	if again.Outcome != ReadOutcomeComplete {
		t.Fatalf("expected ReadOutcomeComplete for empty-at-head read, got %v", again.Outcome)
	}
	if len(again.Records) != 0 {
		t.Fatalf("expected no records past the live head, got %d", len(again.Records))
	}
}

func TestServiceReadTruncatesAtLimitAndResumes(t *testing.T) {
	svc := newFakeAppendService()
	ctx := context.Background()
	topic := TopicID("factory-session/s1/response-events")

	appendTestRecord(t, svc, topic, 1)
	appendTestRecord(t, svc, topic, 2)
	appendTestRecord(t, svc, topic, 3)

	first, err := svc.Read(ctx, ReadRequest{
		Topic: topic,
		After: Cursor{Topic: topic, Generation: 1, Mode: CursorBeginning},
		Limit: 2,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if first.Outcome != ReadOutcomeTruncated {
		t.Fatalf("expected ReadOutcomeTruncated, got %v", first.Outcome)
	}
	if len(first.Records) != 2 {
		t.Fatalf("expected 2 records within the limit, got %d", len(first.Records))
	}
	if first.NextCursor.Mode != CursorAt || first.NextCursor.At != 2 {
		t.Fatalf("expected a resumable NextCursor at aggregate sequence 2, got %+v", first.NextCursor)
	}

	rest, err := svc.Read(ctx, ReadRequest{Topic: topic, After: first.NextCursor, Limit: 10})
	if err != nil {
		t.Fatalf("unexpected error resuming from NextCursor: %v", err)
	}
	if rest.Outcome != ReadOutcomeComplete {
		t.Fatalf("expected ReadOutcomeComplete after resuming to head, got %v", rest.Outcome)
	}
	if len(rest.Records) != 1 || rest.Records[0].AggregateSequence != 3 {
		t.Fatalf("expected exactly the remaining record 3, got %+v", rest.Records)
	}
}

func TestServiceReadReportsGapAfterRetentionEviction(t *testing.T) {
	svc := newFakeAppendService()
	ctx := context.Background()
	topic := TopicID("factory-session/s1/response-events")
	svc.setRetention(topic, RetentionLimits{MaxRecords: 1, MaxBytes: 1024})

	appendTestRecord(t, svc, topic, 1) // evicted once record 2 commits
	appendTestRecord(t, svc, topic, 2)

	result, err := svc.Read(ctx, ReadRequest{
		Topic: topic,
		After: Cursor{Topic: topic, Generation: 1, Mode: CursorBeginning},
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Outcome != ReadOutcomeGap {
		t.Fatalf("expected ReadOutcomeGap, got %v", result.Outcome)
	}
	if result.Gap == nil {
		t.Fatalf("expected a non-nil Gap for ReadOutcomeGap")
	}
	if result.Gap.From != 1 || result.Gap.To != 1 || result.Gap.ResumeAt != 2 {
		t.Fatalf("expected Gap{From:1, To:1, ResumeAt:2}, got %+v", result.Gap)
	}
	if err := result.Gap.Validate(); err != nil {
		t.Fatalf("expected the reported Gap to be well-formed, got %v", err)
	}
}

func TestServiceReadRejectsInvalidRequest(t *testing.T) {
	svc := newFakeAppendService()
	topic := TopicID("factory-session/s1/response-events")
	appendTestRecord(t, svc, topic, 1)

	_, err := svc.Read(context.Background(), ReadRequest{
		Topic: topic,
		After: Cursor{Topic: topic, Generation: 1, Mode: CursorBeginning},
		Limit: 0,
	})
	if !errors.Is(err, ErrInvalidReadLimit) {
		t.Fatalf("expected errors.Is(err, ErrInvalidReadLimit), got %v", err)
	}
}

func TestServiceReadRejectsForeignCursorTopic(t *testing.T) {
	svc := newFakeAppendService()
	topic := TopicID("factory-session/s1/response-events")
	appendTestRecord(t, svc, topic, 1)

	_, err := svc.Read(context.Background(), ReadRequest{
		Topic: topic,
		After: Cursor{Topic: "factory-session/other/response-events", Generation: 1, Mode: CursorBeginning},
		Limit: 10,
	})
	if !errors.Is(err, ErrCursorForeignTopic) {
		t.Fatalf("expected errors.Is(err, ErrCursorForeignTopic), got %v", err)
	}
}

func TestServiceReadRejectsStaleCursorGeneration(t *testing.T) {
	svc := newFakeAppendService()
	topic := TopicID("factory-session/s1/response-events")
	appendTestRecord(t, svc, topic, 1)

	_, err := svc.Read(context.Background(), ReadRequest{
		Topic: topic,
		After: Cursor{Topic: topic, Generation: 2, Mode: CursorBeginning},
		Limit: 10,
	})
	if !errors.Is(err, ErrCursorStaleGeneration) {
		t.Fatalf("expected errors.Is(err, ErrCursorStaleGeneration), got %v", err)
	}
}

func TestServiceReadRejectsUnknownTopic(t *testing.T) {
	svc := newFakeAppendService()
	topic := TopicID("factory-session/never-appended/response-events")

	_, err := svc.Read(context.Background(), ReadRequest{
		Topic: topic,
		After: Cursor{Topic: topic, Generation: 1, Mode: CursorBeginning},
		Limit: 10,
	})
	if !errors.Is(err, ErrTopicNotFound) {
		t.Fatalf("expected errors.Is(err, ErrTopicNotFound), got %v", err)
	}
}

func TestServiceSubscribeDeliversRetainedBacklogThenLiveHandoff(t *testing.T) {
	svc := newFakeAppendService()
	ctx := context.Background()
	topic := TopicID("factory-session/s1/response-events")

	appendTestRecord(t, svc, topic, 1)
	appendTestRecord(t, svc, topic, 2)

	result, err := svc.Subscribe(ctx, SubscribeRequest{
		Topic:    topic,
		Start:    Cursor{Topic: topic, Generation: 1, Mode: CursorBeginning},
		Capacity: 10,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	first := result.Subscription.Next(ctx)
	if first.Outcome != SubscriptionOutcomeRecords {
		t.Fatalf("expected SubscriptionOutcomeRecords for retained backlog, got %v", first.Outcome)
	}
	if len(first.Records) != 2 || first.Records[0].AggregateSequence != 1 || first.Records[1].AggregateSequence != 2 {
		t.Fatalf("expected the 2 retained records in commit order, got %+v", first.Records)
	}

	// Nothing new has committed yet: the next observation still reports
	// SubscriptionOutcomeRecords (the subscription remains open), just with
	// no new Records.
	idle := result.Subscription.Next(ctx)
	if idle.Outcome != SubscriptionOutcomeRecords || len(idle.Records) != 0 {
		t.Fatalf("expected an open, empty observation while caught up, got %+v", idle)
	}

	third := appendTestRecord(t, svc, topic, 3)
	live := result.Subscription.Next(ctx)
	if live.Outcome != SubscriptionOutcomeRecords {
		t.Fatalf("expected SubscriptionOutcomeRecords for the live handoff, got %v", live.Outcome)
	}
	if len(live.Records) != 1 || live.Records[0].AggregateSequence != third.AggregateSequence {
		t.Fatalf("expected exactly the newly committed record %d, got %+v", third.AggregateSequence, live.Records)
	}
}

func TestServiceSubscribeRejectsInvalidRequest(t *testing.T) {
	svc := newFakeAppendService()

	_, err := svc.Subscribe(context.Background(), SubscribeRequest{
		Topic:    "factory-session/s1/response-events",
		Start:    Cursor{Topic: "factory-session/s1/response-events", Generation: 1, Mode: CursorBeginning},
		Capacity: 0,
	})
	if !errors.Is(err, ErrInvalidSubscribeCapacity) {
		t.Fatalf("expected errors.Is(err, ErrInvalidSubscribeCapacity), got %v", err)
	}
}

func TestServiceSubscribeTerminatesOnUnknownTopic(t *testing.T) {
	svc := newFakeAppendService()
	ctx := context.Background()
	topic := TopicID("factory-session/never-appended/response-events")

	result, err := svc.Subscribe(ctx, SubscribeRequest{
		Topic:    topic,
		Start:    Cursor{Topic: topic, Generation: 1, Mode: CursorBeginning},
		Capacity: 10,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	delivery := result.Subscription.Next(ctx)
	if delivery.Outcome != SubscriptionOutcomeTerminal {
		t.Fatalf("expected SubscriptionOutcomeTerminal, got %v", delivery.Outcome)
	}
	if delivery.Terminal.Reason != SubscriptionTerminalInvalidCursor {
		t.Fatalf("expected SubscriptionTerminalInvalidCursor, got %v", delivery.Terminal.Reason)
	}
}

func TestServiceSubscribeTerminatesOnStaleCursorGeneration(t *testing.T) {
	svc := newFakeAppendService()
	ctx := context.Background()
	topic := TopicID("factory-session/s1/response-events")
	appendTestRecord(t, svc, topic, 1)

	result, err := svc.Subscribe(ctx, SubscribeRequest{
		Topic:    topic,
		Start:    Cursor{Topic: topic, Generation: 2, Mode: CursorBeginning},
		Capacity: 10,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	delivery := result.Subscription.Next(ctx)
	if delivery.Outcome != SubscriptionOutcomeTerminal || delivery.Terminal.Reason != SubscriptionTerminalInvalidCursor {
		t.Fatalf("expected SubscriptionTerminalInvalidCursor, got %+v", delivery)
	}
}

func TestServiceSubscribeTerminatesWithGapAfterRetentionEviction(t *testing.T) {
	svc := newFakeAppendService()
	ctx := context.Background()
	topic := TopicID("factory-session/s1/response-events")
	svc.setRetention(topic, RetentionLimits{MaxRecords: 1, MaxBytes: 1024})

	appendTestRecord(t, svc, topic, 1) // evicted once record 2 commits
	appendTestRecord(t, svc, topic, 2)

	result, err := svc.Subscribe(ctx, SubscribeRequest{
		Topic:    topic,
		Start:    Cursor{Topic: topic, Generation: 1, Mode: CursorBeginning},
		Capacity: 10,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	delivery := result.Subscription.Next(ctx)
	if delivery.Outcome != SubscriptionOutcomeTerminal {
		t.Fatalf("expected SubscriptionOutcomeTerminal, got %v", delivery.Outcome)
	}
	if delivery.Terminal.Reason != SubscriptionTerminalGap {
		t.Fatalf("expected SubscriptionTerminalGap, got %v", delivery.Terminal.Reason)
	}
	if delivery.Terminal.Gap == nil {
		t.Fatalf("expected a non-nil Gap for SubscriptionTerminalGap")
	}
	if delivery.Terminal.Gap.From != 1 || delivery.Terminal.Gap.To != 1 || delivery.Terminal.Gap.ResumeAt != 2 {
		t.Fatalf("expected Gap{From:1, To:1, ResumeAt:2}, got %+v", delivery.Terminal.Gap)
	}
	if err := delivery.Terminal.Gap.Validate(); err != nil {
		t.Fatalf("expected the reported Gap to be well-formed, got %v", err)
	}
}

func TestServiceSubscribeTerminatesOnBackpressureWhenCapacityIsExceeded(t *testing.T) {
	svc := newFakeAppendService()
	ctx := context.Background()
	topic := TopicID("factory-session/s1/response-events")

	result, err := svc.Subscribe(ctx, SubscribeRequest{
		Topic:    topic,
		Start:    Cursor{Topic: topic, Generation: 1, Mode: CursorLiveHead},
		Capacity: 2,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Simulate a slow consumer: 3 records commit before the subscriber ever
	// calls Next, exceeding the bounded Capacity of 2.
	appendTestRecord(t, svc, topic, 1)
	appendTestRecord(t, svc, topic, 2)
	appendTestRecord(t, svc, topic, 3) // 3 undelivered records now exceed Capacity 2

	delivery := result.Subscription.Next(ctx)
	if delivery.Outcome != SubscriptionOutcomeTerminal {
		t.Fatalf("expected SubscriptionOutcomeTerminal, got %v", delivery.Outcome)
	}
	if delivery.Terminal.Reason != SubscriptionTerminalBackpressure {
		t.Fatalf("expected SubscriptionTerminalBackpressure, got %v", delivery.Terminal.Reason)
	}
}

func TestServiceSubscribeTerminatesOnCompletionAfterDraining(t *testing.T) {
	svc := newFakeAppendService()
	ctx := context.Background()
	topic := TopicID("factory-session/s1/response-events")

	appendTestRecord(t, svc, topic, 1)
	svc.complete(topic)

	result, err := svc.Subscribe(ctx, SubscribeRequest{
		Topic:    topic,
		Start:    Cursor{Topic: topic, Generation: 1, Mode: CursorBeginning},
		Capacity: 10,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	drained := result.Subscription.Next(ctx)
	if drained.Outcome != SubscriptionOutcomeRecords || len(drained.Records) != 1 {
		t.Fatalf("expected the single retained record before completion, got %+v", drained)
	}

	delivery := result.Subscription.Next(ctx)
	if delivery.Outcome != SubscriptionOutcomeTerminal || delivery.Terminal.Reason != SubscriptionTerminalCompleted {
		t.Fatalf("expected SubscriptionTerminalCompleted, got %+v", delivery)
	}
}

func TestServiceSubscribeTerminatesOnCallerCancellation(t *testing.T) {
	svc := newFakeAppendService()
	topic := TopicID("factory-session/s1/response-events")
	appendTestRecord(t, svc, topic, 1)

	result, err := svc.Subscribe(context.Background(), SubscribeRequest{
		Topic:    topic,
		Start:    Cursor{Topic: topic, Generation: 1, Mode: CursorBeginning},
		Capacity: 10,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()

	delivery := result.Subscription.Next(canceled)
	if delivery.Outcome != SubscriptionOutcomeTerminal || delivery.Terminal.Reason != SubscriptionTerminalCanceled {
		t.Fatalf("expected SubscriptionTerminalCanceled, got %+v", delivery)
	}
}
