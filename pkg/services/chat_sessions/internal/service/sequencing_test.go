package service

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"sync"
	"testing"
	"time"

	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
	"github.com/portpowered/infinite-you/pkg/services/events"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// fakeEventsAppender is a minimal, concurrency-safe EventsAppender+EventsReader double that
// dedups by identity like the real Events store, since Sequence depends on observing
// AppendOutcomeDuplicate for a repeated source identity tuple.
type fakeEventsAppender struct {
	mu     sync.Mutex
	topics map[events.Topic]*fakeTopicState
}

type fakeTopicState struct {
	head     events.AggregateSequence
	identity map[events.AppendIdentity]events.Record
	commits  []events.AppendRequest
	// records holds every still-retained Record in commit order, used by
	// Read. retentionLimit, when nonzero, caps how many records this fake
	// keeps: an Append past the cap evicts the oldest records and advances
	// the topic's earliest-retained position, simulating Events' own
	// bounded-retention eviction deterministically (no wall-clock or
	// wait-for-eviction needed in a test).
	records        []events.Record
	retentionLimit int
}

func newFakeEventsAppender() *fakeEventsAppender {
	return &fakeEventsAppender{topics: make(map[events.Topic]*fakeTopicState)}
}

func (f *fakeEventsAppender) Append(ctx context.Context, req events.AppendRequest) (events.AppendResult, error) {
	if err := ctx.Err(); err != nil {
		return events.AppendResult{}, err
	}
	if err := req.Validate(); err != nil {
		return events.AppendResult{}, err
	}
	detached := req.Detached()
	identity := detached.Identity()

	f.mu.Lock()
	defer f.mu.Unlock()

	ts, ok := f.topics[detached.Topic]
	if !ok {
		ts = &fakeTopicState{identity: make(map[events.AppendIdentity]events.Record)}
		f.topics[detached.Topic] = ts
	}
	if existing, ok := ts.identity[identity]; ok {
		return events.AppendResult{Record: existing.Detached(), Outcome: events.AppendOutcomeDuplicate}, nil
	}

	ts.head++
	record := events.Record{
		ID:             events.RecordID{Topic: detached.Topic, Position: ts.head},
		SourceType:     detached.SourceType,
		SourceID:       detached.SourceID,
		SourceSequence: detached.SourceSequence,
		SourceEventID:  detached.SourceEventID,
		SchemaID:       detached.SchemaID,
		Payload:        detached.Payload,
	}.Detached()
	ts.identity[identity] = record
	ts.commits = append(ts.commits, detached)
	ts.records = append(ts.records, record)
	if ts.retentionLimit > 0 && len(ts.records) > ts.retentionLimit {
		evict := len(ts.records) - ts.retentionLimit
		ts.records = ts.records[evict:]
	}
	return events.AppendResult{Record: record.Detached(), Outcome: events.AppendOutcomeAccepted}, nil
}

// Read mirrors the real Events Store's outcome contract (Gap/AtHead/InvalidCursor/Progress)
// closely enough for AcknowledgeAttachment's gap-detection tests.
func (f *fakeEventsAppender) Read(_ context.Context, req events.ReadRequest) (events.ReadResult, error) {
	if err := req.Validate(); err != nil {
		return events.ReadResult{}, err
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	ts, ok := f.topics[req.Topic]
	if !ok {
		return events.ReadResult{}, events.ErrUnknownTopic
	}

	var earliest events.AggregateSequence
	if len(ts.records) > 0 {
		earliest = ts.records[0].ID.Position
	}
	head := ts.head
	from := req.From.Position

	switch {
	case from > head:
		return events.ReadResult{Outcome: events.ReadOutcomeInvalidCursor}, nil
	case earliest > 0 && from+1 < earliest:
		return events.ReadResult{
			Outcome: events.ReadOutcomeGap,
			Gap: &events.GapFacts{
				Topic:            req.Topic,
				Requested:        from + 1,
				EarliestRetained: earliest,
				Head:             head,
			},
		}, nil
	case from == head:
		return events.ReadResult{
			Outcome:  events.ReadOutcomeAtHead,
			Next:     events.Cursor{Topic: req.Topic, Position: head},
			Retained: events.RetainedRange{Topic: req.Topic, Earliest: earliest, Head: head},
		}, nil
	default:
		startIdx := max(int(from-earliest+1), 0)
		end := min(startIdx+req.Limit, len(ts.records))
		recs := make([]events.Record, end-startIdx)
		copy(recs, ts.records[startIdx:end])
		last := recs[len(recs)-1]
		return events.ReadResult{
			Records:  recs,
			Next:     events.Cursor{Topic: req.Topic, Position: last.ID.Position},
			Retained: events.RetainedRange{Topic: req.Topic, Earliest: earliest, Head: head},
			Outcome:  events.ReadOutcomeProgress,
		}, nil
	}
}

// stubAppender delegates every Append call to fn, used to force Sequence's less common
// outcome and error paths without teaching fakeEventsAppender about injected failures.
type stubAppender struct {
	fn func(context.Context, events.AppendRequest) (events.AppendResult, error)
}

func (s stubAppender) Append(ctx context.Context, req events.AppendRequest) (events.AppendResult, error) {
	return s.fn(ctx, req)
}

func (f *fakeEventsAppender) setRetentionLimit(topic events.Topic, limit int) *fakeEventsAppender {
	f.mu.Lock()
	defer f.mu.Unlock()
	ts, ok := f.topics[topic]
	if !ok {
		ts = &fakeTopicState{identity: make(map[events.AppendIdentity]events.Record)}
		f.topics[topic] = ts
	}
	ts.retentionLimit = limit
	return f
}

func (f *fakeEventsAppender) commitCount(topic events.Topic) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	ts, ok := f.topics[topic]
	if !ok {
		return 0
	}
	return len(ts.commits)
}

func newSequencingTestSession(t *testing.T) (*Store, chatsessions.Session, *fakeEventsAppender) {
	t.Helper()
	appender := newFakeEventsAppender()
	store := NewStore(sequentialIDs("id"), fixedClock(time.Now()), appender, appender)
	created, err := store.CreateSession(context.Background(), validCreateRequest())
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	return store, created.Session, appender
}

func sequenceRequest(sessionID string, sourceSeq events.SourceSequence, parentItemID string) chatsessions.SequenceRequest {
	return chatsessions.SequenceRequest{
		SessionID:      sessionID,
		SourceType:     "worker",
		SourceID:       "worker-1",
		SourceSequence: sourceSeq,
		SourceEventID:  events.SourceEventID("event-" + strconv.FormatUint(uint64(sourceSeq), 10)),
		SchemaID:       "worker.output.v1",
		Kind:           workers.KindMessage,
		ParentItemID:   parentItemID,
		Payload:        json.RawMessage(`{"text":"hello"}`),
	}
}

func TestStore_Sequence_AssignsStableItemIDAndPersistsEnvelope(t *testing.T) {
	ctx := context.Background()
	store, session, appender := newSequencingTestSession(t)

	result, err := store.Sequence(ctx, sequenceRequest(session.ID, 1, ""))
	if err != nil {
		t.Fatalf("Sequence: %v", err)
	}
	if result.ItemID == "" {
		t.Fatal("Sequence: got empty ItemID, want a stable non-empty identity")
	}
	if result.Outcome != chatsessions.SequenceOutcomeAccepted {
		t.Fatalf("Sequence: got outcome %v, want SequenceOutcomeAccepted", result.Outcome)
	}
	if result.AggregateSequence != 1 {
		t.Fatalf("Sequence: got aggregate sequence %d, want 1", result.AggregateSequence)
	}

	topic := chatsessions.EventsTopic(session.ID)
	if appender.commitCount(topic) != 1 {
		t.Fatalf("commit count = %d, want exactly 1", appender.commitCount(topic))
	}
	committed := appender.topics[topic].commits[0]
	if committed.Topic != topic {
		t.Fatalf("committed topic = %q, want %q", committed.Topic, topic)
	}
	var envelope chatsessions.SequencedItem
	if err := json.Unmarshal(committed.Payload, &envelope); err != nil {
		t.Fatalf("unmarshal committed envelope: %v", err)
	}
	if envelope.ItemID != result.ItemID {
		t.Fatalf("committed envelope ItemID = %q, want %q (the exact identity returned to the caller)", envelope.ItemID, result.ItemID)
	}
	if envelope.Kind != workers.KindMessage {
		t.Fatalf("committed envelope Kind = %q, want %q", envelope.Kind, workers.KindMessage)
	}
}

func TestStore_Sequence_PreservesSourceIdentityAndPayload(t *testing.T) {
	ctx := context.Background()
	store, session, appender := newSequencingTestSession(t)

	req := sequenceRequest(session.ID, 7, "")
	if _, err := store.Sequence(ctx, req); err != nil {
		t.Fatalf("Sequence: %v", err)
	}

	topic := chatsessions.EventsTopic(session.ID)
	committed := appender.topics[topic].commits[0]
	if committed.SourceType != req.SourceType {
		t.Errorf("SourceType = %q, want %q", committed.SourceType, req.SourceType)
	}
	if committed.SourceID != req.SourceID {
		t.Errorf("SourceID = %q, want %q", committed.SourceID, req.SourceID)
	}
	if committed.SourceSequence != req.SourceSequence {
		t.Errorf("SourceSequence = %d, want %d", committed.SourceSequence, req.SourceSequence)
	}
	if committed.SourceEventID != req.SourceEventID {
		t.Errorf("SourceEventID = %q, want %q", committed.SourceEventID, req.SourceEventID)
	}
	if committed.SchemaID != req.SchemaID {
		t.Errorf("SchemaID = %q, want %q", committed.SchemaID, req.SchemaID)
	}
	var envelope chatsessions.SequencedItem
	if err := json.Unmarshal(committed.Payload, &envelope); err != nil {
		t.Fatalf("unmarshal committed envelope: %v", err)
	}
	if string(envelope.Payload) != string(req.Payload) {
		t.Errorf("envelope Payload = %s, want %s", envelope.Payload, req.Payload)
	}
}

// A naive json.Marshal(item) on a struct embedding a json.RawMessage field runs that
// field's bytes through Go's stdlib JSON compaction and HTML-escaping before Events ever
// sees it -- this test would fail against that implementation.
func TestStore_Sequence_PreservesPayloadBytesVerbatimIncludingWhitespaceAndHTMLCharacters(t *testing.T) {
	ctx := context.Background()
	store, session, appender := newSequencingTestSession(t)

	req := sequenceRequest(session.ID, 1, "")
	req.Payload = json.RawMessage("{\n  \"text\": \"<script>alert('a' & 'b')</script>\"\n}")
	if _, err := store.Sequence(ctx, req); err != nil {
		t.Fatalf("Sequence: %v", err)
	}

	topic := chatsessions.EventsTopic(session.ID)
	committed := appender.topics[topic].commits[0]
	var envelope chatsessions.SequencedItem
	if err := json.Unmarshal(committed.Payload, &envelope); err != nil {
		t.Fatalf("unmarshal committed envelope: %v", err)
	}
	if string(envelope.Payload) != string(req.Payload) {
		t.Fatalf("committed envelope Payload = %q, want byte-for-byte %q (whitespace/HTML-escaping must not change)", envelope.Payload, req.Payload)
	}
}

func TestStore_Sequence_ChildAcceptedOnlyAfterParentSequenced(t *testing.T) {
	ctx := context.Background()
	store, session, appender := newSequencingTestSession(t)
	topic := chatsessions.EventsTopic(session.ID)

	if _, err := store.Sequence(ctx, sequenceRequest(session.ID, 1, "not-yet-sequenced")); !errors.Is(err, chatsessions.ErrNotFound) {
		t.Fatalf("Sequence with unsequenced parent: got %v, want ErrNotFound", err)
	}
	if got := appender.commitCount(topic); got != 0 {
		t.Fatalf("commit count after rejected parent reference = %d, want 0 (no partial state)", got)
	}

	parent, err := store.Sequence(ctx, sequenceRequest(session.ID, 2, ""))
	if err != nil {
		t.Fatalf("Sequence parent: %v", err)
	}

	child, err := store.Sequence(ctx, sequenceRequest(session.ID, 3, parent.ItemID))
	if err != nil {
		t.Fatalf("Sequence child referencing sequenced parent: %v", err)
	}
	if child.ParentItemID != parent.ItemID {
		t.Fatalf("child.ParentItemID = %q, want %q", child.ParentItemID, parent.ItemID)
	}
	if child.AggregateSequence <= parent.AggregateSequence {
		t.Fatalf("child aggregate sequence %d must be strictly after parent's %d", child.AggregateSequence, parent.AggregateSequence)
	}
}

func TestStore_Sequence_ParentFromAnotherSessionIsRejected(t *testing.T) {
	ctx := context.Background()
	store, sessionA, _ := newSequencingTestSession(t)
	createdB, err := store.CreateSession(ctx, chatsessions.CreateSessionRequest{
		RequestID:     chatsessions.RequestIdentity{Kind: chatsessions.RequestIdentityKindJSONRPCString, ConnectionID: "conn-2", JSONRPCStringID: "req-2"},
		WorkingRoot:   "/workspace/project-b",
		InitialTarget: chatsessions.ChatTargetRef{Kind: chatsessions.ChatTargetKindFactory, Ref: "factory:@you/review"},
	})
	if err != nil {
		t.Fatalf("CreateSession (B): %v", err)
	}

	inA, err := store.Sequence(ctx, sequenceRequest(sessionA.ID, 1, ""))
	if err != nil {
		t.Fatalf("Sequence in session A: %v", err)
	}

	if _, err := store.Sequence(ctx, sequenceRequest(createdB.Session.ID, 1, inA.ItemID)); !errors.Is(err, chatsessions.ErrNotFound) {
		t.Fatalf("Sequence in session B referencing session A's item: got %v, want ErrNotFound", err)
	}
}

func TestStore_Sequence_UnknownSessionReportsNotFound(t *testing.T) {
	ctx := context.Background()
	appender := newFakeEventsAppender()
	store := NewStore(sequentialIDs("id"), fixedClock(time.Now()), appender, nil)

	if _, err := store.Sequence(ctx, sequenceRequest("does-not-exist", 1, "")); !errors.Is(err, chatsessions.ErrNotFound) {
		t.Fatalf("Sequence(unknown session): got %v, want ErrNotFound", err)
	}
}

func TestStore_Sequence_InvalidRequestIsRejected(t *testing.T) {
	ctx := context.Background()

	base := func(session string) chatsessions.SequenceRequest {
		return sequenceRequest(session, 1, "")
	}

	tests := []struct {
		name    string
		mutate  func(chatsessions.SequenceRequest) chatsessions.SequenceRequest
		wantErr error
	}{
		{
			name: "blank session id",
			mutate: func(r chatsessions.SequenceRequest) chatsessions.SequenceRequest {
				r.SessionID = ""
				return r
			},
			wantErr: chatsessions.ErrRequiredValue,
		},
		{
			name: "blank source type",
			mutate: func(r chatsessions.SequenceRequest) chatsessions.SequenceRequest {
				r.SourceType = ""
				return r
			},
			wantErr: events.ErrEmptySourceType,
		},
		{
			name: "unknown kind",
			mutate: func(r chatsessions.SequenceRequest) chatsessions.SequenceRequest {
				r.Kind = "BOGUS"
				return r
			},
			wantErr: chatsessions.ErrUnknownEnumValue,
		},
		{
			name: "empty payload",
			mutate: func(r chatsessions.SequenceRequest) chatsessions.SequenceRequest {
				r.Payload = nil
				return r
			},
			wantErr: chatsessions.ErrRequiredValue,
		},
		{
			name: "malformed payload",
			mutate: func(r chatsessions.SequenceRequest) chatsessions.SequenceRequest {
				r.Payload = json.RawMessage(`{not-json`)
				return r
			},
			wantErr: chatsessions.ErrMalformedValue,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store, session, appender := newSequencingTestSession(t)
			req := tt.mutate(base(session.ID))
			if _, err := store.Sequence(ctx, req); !errors.Is(err, tt.wantErr) {
				t.Fatalf("Sequence(%s): got %v, want %v", tt.name, err, tt.wantErr)
			}
			topic := chatsessions.EventsTopic(session.ID)
			if got := appender.commitCount(topic); got != 0 {
				t.Fatalf("commit count after rejected request (%s) = %d, want 0", tt.name, got)
			}
		})
	}
}

func TestStore_Sequence_DuplicateSourceTupleReturnsOriginalIdentity(t *testing.T) {
	ctx := context.Background()
	store, session, appender := newSequencingTestSession(t)
	topic := chatsessions.EventsTopic(session.ID)

	req := sequenceRequest(session.ID, 1, "")
	first, err := store.Sequence(ctx, req)
	if err != nil {
		t.Fatalf("Sequence (first): %v", err)
	}
	second, err := store.Sequence(ctx, req)
	if err != nil {
		t.Fatalf("Sequence (duplicate): %v", err)
	}
	if second.Outcome != chatsessions.SequenceOutcomeDuplicate {
		t.Fatalf("duplicate outcome = %v, want SequenceOutcomeDuplicate", second.Outcome)
	}
	if second.ItemID != first.ItemID || second.AggregateSequence != first.AggregateSequence {
		t.Fatalf("duplicate result = %+v, want identity matching original %+v", second, first)
	}
	if got := appender.commitCount(topic); got != 1 {
		t.Fatalf("commit count after duplicate = %d, want exactly 1", got)
	}
}

// TestStore_Sequence_ContradictoryDuplicateIsRejected table-drives every way a reused
// source identity tuple can disagree with the originally committed record.
func TestStore_Sequence_ContradictoryDuplicateIsRejected(t *testing.T) {
	ctx := context.Background()
	topic := func(session chatsessions.Session) events.Topic { return chatsessions.EventsTopic(session.ID) }

	tests := []struct {
		name      string
		mutate    func(chatsessions.SequenceRequest) chatsessions.SequenceRequest
		wantField string
	}{
		{
			name: "contradictory parent",
			mutate: func(r chatsessions.SequenceRequest) chatsessions.SequenceRequest {
				r.ParentItemID = "some-other-parent"
				return r
			},
			wantField: "ParentItemID",
		},
		{
			name: "contradictory kind",
			mutate: func(r chatsessions.SequenceRequest) chatsessions.SequenceRequest {
				r.Kind = workers.KindTool
				return r
			},
			wantField: "Kind",
		},
		{
			name: "contradictory schema",
			mutate: func(r chatsessions.SequenceRequest) chatsessions.SequenceRequest {
				r.SchemaID = "worker.output.v2"
				return r
			},
			wantField: "SchemaID",
		},
		{
			name: "contradictory payload",
			mutate: func(r chatsessions.SequenceRequest) chatsessions.SequenceRequest {
				r.Payload = json.RawMessage(`{"text":"goodbye"}`)
				return r
			},
			wantField: "Payload",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store, session, appender := newSequencingTestSession(t)

			original := sequenceRequest(session.ID, 1, "")
			first, err := store.Sequence(ctx, original)
			if err != nil {
				t.Fatalf("Sequence (first): %v", err)
			}

			retry := tt.mutate(sequenceRequest(session.ID, 1, ""))
			if tt.wantField == "ParentItemID" {
				// The contradictory ParentItemID must itself already name a
				// sequenced item in this session, otherwise the earlier
				// parent-lookup check (a distinct rejection path) would fire
				// instead of the contradiction check this test targets.
				altParent, err := store.Sequence(ctx, sequenceRequest(session.ID, 2, ""))
				if err != nil {
					t.Fatalf("Sequence (alternate parent): %v", err)
				}
				retry.ParentItemID = altParent.ItemID
			}

			_, err = store.Sequence(ctx, retry)
			var conflict *chatsessions.SequencedIdentityConflictError
			if !errors.As(err, &conflict) {
				t.Fatalf("Sequence(%s): got %v, want *SequencedIdentityConflictError", tt.name, err)
			}
			if !errors.Is(err, chatsessions.ErrSequencedIdentityContradiction) {
				t.Fatalf("Sequence(%s): got %v, want errors.Is ErrSequencedIdentityContradiction", tt.name, err)
			}
			if conflict.Field != tt.wantField {
				t.Fatalf("conflict.Field = %q, want %q", conflict.Field, tt.wantField)
			}

			if got := appender.commitCount(topic(session)); got < 1 {
				t.Fatalf("commit count = %d, want at least 1 (original record untouched)", got)
			}
			storedRecord := appender.topics[topic(session)].identity[events.AppendIdentity{
				SourceType:     original.SourceType,
				SourceID:       original.SourceID,
				SourceSequence: original.SourceSequence,
				SourceEventID:  original.SourceEventID,
			}]
			var storedItem chatsessions.SequencedItem
			if err := json.Unmarshal(storedRecord.Payload, &storedItem); err != nil {
				t.Fatalf("unmarshal stored envelope: %v", err)
			}
			if storedItem.ItemID != first.ItemID {
				t.Fatalf("stored envelope ItemID = %q, want original %q (not replaced)", storedItem.ItemID, first.ItemID)
			}
		})
	}
}

// 9007199254740992 vs 9007199254740993 are distinct valid JSON integers that collapse to
// the same float64 -- equalJSON must compare exact decimal text, not decode through float64.
func TestStore_Sequence_ContradictoryLargeIntegerPayloadIsRejected(t *testing.T) {
	ctx := context.Background()
	store, session, appender := newSequencingTestSession(t)
	topic := chatsessions.EventsTopic(session.ID)

	original := sequenceRequest(session.ID, 1, "")
	original.Payload = json.RawMessage(`{"value":9007199254740992}`)
	first, err := store.Sequence(ctx, original)
	if err != nil {
		t.Fatalf("Sequence (first): %v", err)
	}

	retry := sequenceRequest(session.ID, 1, "")
	retry.Payload = json.RawMessage(`{"value":9007199254740993}`)
	_, err = store.Sequence(ctx, retry)
	var conflict *chatsessions.SequencedIdentityConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("Sequence(adjacent large integer payload): got %v, want *SequencedIdentityConflictError", err)
	}
	if conflict.Field != "Payload" {
		t.Fatalf("conflict.Field = %q, want %q", conflict.Field, "Payload")
	}

	if got := appender.commitCount(topic); got != 1 {
		t.Fatalf("commit count after contradictory retry = %d, want exactly 1 (original untouched, no second record)", got)
	}
	storedRecord := appender.topics[topic].identity[events.AppendIdentity{
		SourceType:     original.SourceType,
		SourceID:       original.SourceID,
		SourceSequence: original.SourceSequence,
		SourceEventID:  original.SourceEventID,
	}]
	var storedItem chatsessions.SequencedItem
	if err := json.Unmarshal(storedRecord.Payload, &storedItem); err != nil {
		t.Fatalf("unmarshal stored envelope: %v", err)
	}
	if storedItem.ItemID != first.ItemID {
		t.Fatalf("stored envelope ItemID = %q, want original %q (not replaced)", storedItem.ItemID, first.ItemID)
	}
}

func countingIDs(prefix string) (IDGenerator, func() int) {
	n := 0
	gen := func() string {
		n++
		return prefix + "-" + strconv.Itoa(n)
	}
	return gen, func() int { return n }
}

// Before this test's fix, Sequence called s.newID() while building the candidate envelope
// before Append, discarding that minted identity on every duplicate/rejected branch.
func TestStore_Sequence_DuplicateAndContradictoryRetriesNeverConsumeGenerator(t *testing.T) {
	ctx := context.Background()
	appender := newFakeEventsAppender()
	gen, callCount := countingIDs("id")
	store := NewStore(gen, fixedClock(time.Now()), appender, appender)
	created, err := store.CreateSession(ctx, validCreateRequest())
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	baseline := callCount() // CreateSession itself consumed exactly one identity.

	if _, err := store.Sequence(ctx, sequenceRequest(created.Session.ID, 1, "")); err != nil {
		t.Fatalf("Sequence (first): %v", err)
	}
	afterFirst := callCount()
	if afterFirst != baseline+1 {
		t.Fatalf("generator calls after first accepted Sequence = %d, want %d", afterFirst, baseline+1)
	}

	if _, err := store.Sequence(ctx, sequenceRequest(created.Session.ID, 1, "")); err != nil {
		t.Fatalf("Sequence (duplicate retry): %v", err)
	}
	if got := callCount(); got != afterFirst {
		t.Fatalf("generator calls after duplicate retry = %d, want unchanged %d (duplicate must not mint an ItemID)", got, afterFirst)
	}

	contradictory := sequenceRequest(created.Session.ID, 1, "")
	contradictory.Payload = json.RawMessage(`{"text":"goodbye"}`)
	var conflict *chatsessions.SequencedIdentityConflictError
	if _, err := store.Sequence(ctx, contradictory); !errors.As(err, &conflict) {
		t.Fatalf("Sequence (contradictory retry): got %v, want *SequencedIdentityConflictError", err)
	}
	if got := callCount(); got != afterFirst {
		t.Fatalf("generator calls after contradictory retry = %d, want unchanged %d (rejected retry must not mint an ItemID)", got, afterFirst)
	}

	next, err := store.Sequence(ctx, sequenceRequest(created.Session.ID, 2, ""))
	if err != nil {
		t.Fatalf("Sequence (next new record): %v", err)
	}
	wantNextItemID := "id-" + strconv.Itoa(baseline+2)
	if next.ItemID != wantNextItemID {
		t.Fatalf("next accepted ItemID = %q, want %q (unaffected by prior retries)", next.ItemID, wantNextItemID)
	}
	if got := callCount(); got != baseline+2 {
		t.Fatalf("generator calls after next accepted record = %d, want %d", got, baseline+2)
	}
}

func TestStore_Sequence_ConcurrentFirstDeliveryAndRetryAgree(t *testing.T) {
	ctx := context.Background()
	store, session, appender := newSequencingTestSession(t)
	topic := chatsessions.EventsTopic(session.ID)
	req := sequenceRequest(session.ID, 1, "")

	start := make(chan struct{})
	var wg sync.WaitGroup
	var firstResult, secondResult chatsessions.SequenceResult
	var firstErr, secondErr error

	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		firstResult, firstErr = store.Sequence(ctx, req)
	}()
	go func() {
		defer wg.Done()
		<-start
		secondResult, secondErr = store.Sequence(ctx, req)
	}()
	close(start)
	wg.Wait()

	if firstErr != nil {
		t.Fatalf("first Sequence: %v", firstErr)
	}
	if secondErr != nil {
		t.Fatalf("second Sequence: %v", secondErr)
	}
	if firstResult.ItemID != secondResult.ItemID {
		t.Fatalf("ItemID diverged across the race: %q vs %q", firstResult.ItemID, secondResult.ItemID)
	}
	if firstResult.AggregateSequence != secondResult.AggregateSequence {
		t.Fatalf("AggregateSequence diverged across the race: %d vs %d", firstResult.AggregateSequence, secondResult.AggregateSequence)
	}
	acceptedCount := 0
	if firstResult.Outcome == chatsessions.SequenceOutcomeAccepted {
		acceptedCount++
	}
	if secondResult.Outcome == chatsessions.SequenceOutcomeAccepted {
		acceptedCount++
	}
	if acceptedCount != 1 {
		t.Fatalf("accepted outcome count = %d, want exactly 1 (one winner, one duplicate)", acceptedCount)
	}
	if got := appender.commitCount(topic); got != 1 {
		t.Fatalf("commit count after racing identical retry = %d, want exactly 1", got)
	}
}

func TestStore_Sequence_ConcurrentDistinctRecordsCommitContiguousPositions(t *testing.T) {
	const n = 64
	ctx := context.Background()
	store, session, _ := newSequencingTestSession(t)

	start := make(chan struct{})
	var wg sync.WaitGroup
	var mu sync.Mutex
	positions := make(map[events.AggregateSequence]bool, n)
	itemIDs := make(map[string]bool, n)

	for i := range n {
		wg.Add(1)
		go func(seq int) {
			defer wg.Done()
			<-start
			result, err := store.Sequence(ctx, sequenceRequest(session.ID, events.SourceSequence(seq+1), ""))
			if err != nil {
				t.Errorf("Sequence(seq=%d): %v", seq, err)
				return
			}
			mu.Lock()
			positions[result.AggregateSequence] = true
			itemIDs[result.ItemID] = true
			mu.Unlock()
		}(i)
	}
	close(start)
	wg.Wait()

	if len(positions) != n {
		t.Fatalf("distinct aggregate positions = %d, want %d (no duplicate or skipped position)", len(positions), n)
	}
	for i := 1; i <= n; i++ {
		if !positions[events.AggregateSequence(i)] {
			t.Fatalf("position %d missing from committed set, want contiguous 1..%d", i, n)
		}
	}
	if len(itemIDs) != n {
		t.Fatalf("distinct item ids = %d, want %d (every concurrent record got its own stable identity)", len(itemIDs), n)
	}
}

// Sequence's session lookup, parent check, Events append, and sequencedItemIDs update all
// run under one Store-wide critical section, so only two outcomes are ever possible: the
// child loses the race (*NotFoundError) or wins against a fully consistent parent.
func TestStore_Sequence_ConcurrentChildNeverObservesTornParentState(t *testing.T) {
	ctx := context.Background()
	store, session, _ := newSequencingTestSession(t)

	// sequentialIDs("id") issued "id-1" to CreateSession's session, so the
	// very next minted identity -- the parent's ItemID -- is deterministically
	// predictable as "id-2".
	const predictedParentItemID = "id-2"

	start := make(chan struct{})
	var wg sync.WaitGroup
	var parentResult chatsessions.SequenceResult
	var parentErr error
	var childResult chatsessions.SequenceResult
	var childErr error

	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		parentResult, parentErr = store.Sequence(ctx, sequenceRequest(session.ID, 1, ""))
	}()
	go func() {
		defer wg.Done()
		<-start
		childResult, childErr = store.Sequence(ctx, sequenceRequest(session.ID, 2, predictedParentItemID))
	}()
	close(start)
	wg.Wait()

	if parentErr != nil {
		t.Fatalf("parent Sequence: %v", parentErr)
	}
	if parentResult.ItemID != predictedParentItemID {
		t.Fatalf("parent ItemID = %q, want predicted %q", parentResult.ItemID, predictedParentItemID)
	}

	switch {
	case childErr == nil:
		if childResult.ParentItemID != predictedParentItemID {
			t.Fatalf("child won the race but ParentItemID = %q, want %q", childResult.ParentItemID, predictedParentItemID)
		}
	case errors.Is(childErr, chatsessions.ErrNotFound):
		// Child lost the race: the parent had not been sequenced yet when the
		// child's Sequence call ran, which is the expected, safe outcome.
	default:
		t.Fatalf("child Sequence: got %v, want nil (won the race) or ErrNotFound (lost the race)", childErr)
	}
}

func TestStore_Sequence_CancelledContextIsRejected(t *testing.T) {
	store, session, appender := newSequencingTestSession(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := store.Sequence(ctx, sequenceRequest(session.ID, 1, "")); !errors.Is(err, context.Canceled) {
		t.Fatalf("Sequence(cancelled ctx): got %v, want context.Canceled", err)
	}
	if got := appender.commitCount(chatsessions.EventsTopic(session.ID)); got != 0 {
		t.Fatalf("commit count after cancelled context = %d, want 0", got)
	}
}

func TestStore_Sequence_AssignedItemFailingValidationIsRejected(t *testing.T) {
	ctx := context.Background()
	appender := newFakeEventsAppender()
	calls := 0
	blankAfterFirst := func() string {
		calls++
		if calls == 1 {
			return "session-1"
		}
		return ""
	}
	store := NewStore(blankAfterFirst, fixedClock(time.Now()), appender, nil)
	created, err := store.CreateSession(ctx, validCreateRequest())
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	if _, err := store.Sequence(ctx, sequenceRequest(created.Session.ID, 1, "")); !errors.Is(err, chatsessions.ErrRequiredValue) {
		t.Fatalf("Sequence(blank minted ItemID): got %v, want ErrRequiredValue", err)
	}
	if got := appender.commitCount(chatsessions.EventsTopic(created.Session.ID)); got != 0 {
		t.Fatalf("commit count after invalid assigned item = %d, want 0", got)
	}
}

func TestStore_Sequence_AppendFailurePropagatesWithoutIndexingItem(t *testing.T) {
	ctx := context.Background()
	appendErr := errors.New("events store unavailable")
	appender := stubAppender{fn: func(context.Context, events.AppendRequest) (events.AppendResult, error) {
		return events.AppendResult{}, appendErr
	}}
	store := NewStore(sequentialIDs("id"), fixedClock(time.Now()), appender, nil)
	created, err := store.CreateSession(ctx, validCreateRequest())
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	if _, err := store.Sequence(ctx, sequenceRequest(created.Session.ID, 1, "")); !errors.Is(err, appendErr) {
		t.Fatalf("Sequence(append failure): got %v, want %v", err, appendErr)
	}
}

func TestStore_Sequence_UnexpectedAppendOutcomeIsReportedAsError(t *testing.T) {
	ctx := context.Background()
	const bogusOutcome = events.AppendOutcome(99)
	appender := stubAppender{fn: func(_ context.Context, req events.AppendRequest) (events.AppendResult, error) {
		return events.AppendResult{
			Record: events.Record{
				ID:             events.RecordID{Topic: req.Topic, Position: 1},
				SourceType:     req.SourceType,
				SourceID:       req.SourceID,
				SourceSequence: req.SourceSequence,
				SourceEventID:  req.SourceEventID,
				SchemaID:       req.SchemaID,
				Payload:        req.Payload,
			},
			Outcome: bogusOutcome,
		}, nil
	}}
	store := NewStore(sequentialIDs("id"), fixedClock(time.Now()), appender, nil)
	created, err := store.CreateSession(ctx, validCreateRequest())
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	if _, err := store.Sequence(ctx, sequenceRequest(created.Session.ID, 1, "")); err == nil {
		t.Fatal("Sequence(unexpected outcome): got nil error, want a reported error")
	}
}

func TestEqualJSON(t *testing.T) {
	tests := []struct {
		name string
		a, b json.RawMessage
		want bool
	}{
		{
			name: "identical bytes",
			a:    json.RawMessage(`{"text":"hello"}`),
			b:    json.RawMessage(`{"text":"hello"}`),
			want: true,
		},
		{
			name: "structurally equal despite whitespace and key order",
			a:    json.RawMessage(`{"a":1,"b":2}`),
			b:    json.RawMessage(`{ "b": 2, "a": 1 }`),
			want: true,
		},
		{
			name: "structurally different",
			a:    json.RawMessage(`{"text":"hello"}`),
			b:    json.RawMessage(`{"text":"goodbye"}`),
			want: false,
		},
		{
			name: "a malformed falls back to byte comparison (equal bytes)",
			a:    json.RawMessage(`{not-json`),
			b:    json.RawMessage(`{not-json`),
			want: true,
		},
		{
			name: "a malformed falls back to byte comparison (different bytes)",
			a:    json.RawMessage(`{not-json`),
			b:    json.RawMessage(`{"text":"hello"}`),
			want: false,
		},
		{
			name: "b malformed falls back to byte comparison",
			a:    json.RawMessage(`{"text":"hello"}`),
			b:    json.RawMessage(`{also-not-json`),
			want: false,
		},
		{
			name: "map with fewer keys is not equal",
			a:    json.RawMessage(`{"a":1}`),
			b:    json.RawMessage(`{"a":1,"b":2}`),
			want: false,
		},
		{
			name: "equal nested arrays",
			a:    json.RawMessage(`{"items":[1,2,3]}`),
			b:    json.RawMessage(`{"items":[1,2,3]}`),
			want: true,
		},
		{
			name: "arrays of different length are not equal",
			a:    json.RawMessage(`{"items":[1,2]}`),
			b:    json.RawMessage(`{"items":[1,2,3]}`),
			want: false,
		},
		{
			name: "arrays with a differing element are not equal",
			a:    json.RawMessage(`{"items":[1,2,3]}`),
			b:    json.RawMessage(`{"items":[1,2,9]}`),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := equalJSON(tt.a, tt.b); got != tt.want {
				t.Fatalf("equalJSON(%s, %s) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}
