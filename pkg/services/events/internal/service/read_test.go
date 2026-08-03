package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/events"
)

const readTestTopic events.Topic = "chat-session/read/events"

func appendN(t *testing.T, st *Store, ctx context.Context, topic events.Topic, n int) {
	t.Helper()
	for i := 1; i <= n; i++ {
		req := validAppendRequest()
		req.Topic = topic
		req.SourceSequence = events.SourceSequence(i)
		req.SourceEventID = events.SourceEventID("evt-" + itoa(i))
		if _, err := st.Append(ctx, req); err != nil {
			t.Fatalf("Append() [%d] error = %v", i, err)
		}
	}
}

func itoa(n int) string {
	// small helper to avoid pulling strconv into every call site; test-only.
	if n == 0 {
		return "0"
	}
	digits := ""
	for n > 0 {
		digits = string(rune('0'+n%10)) + digits
		n /= 10
	}
	return digits
}

func TestRead_ReturnsAtHeadOnEmptyTopic(t *testing.T) {
	st := New()
	ctx := context.Background()

	result, err := st.Read(ctx, events.ReadRequest{Topic: readTestTopic, From: events.Cursor{Topic: readTestTopic}, Limit: 10})
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if result.Outcome != events.ReadOutcomeAtHead {
		t.Fatalf("Outcome = %v, want ReadOutcomeAtHead", result.Outcome)
	}
	if err := result.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestRead_ProgressReturnsContiguousRecordsAndNextCursor(t *testing.T) {
	st := New()
	ctx := context.Background()
	appendN(t, st, ctx, readTestTopic, 5)

	result, err := st.Read(ctx, events.ReadRequest{Topic: readTestTopic, From: events.Cursor{Topic: readTestTopic}, Limit: 3})
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if err := result.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if result.Outcome != events.ReadOutcomeProgress {
		t.Fatalf("Outcome = %v, want ReadOutcomeProgress", result.Outcome)
	}
	if len(result.Records) != 3 {
		t.Fatalf("len(Records) = %d, want 3 (bounded by Limit)", len(result.Records))
	}
	for i, rec := range result.Records {
		if rec.ID.Position != events.AggregateSequence(i+1) {
			t.Fatalf("Records[%d].ID.Position = %d, want %d", i, rec.ID.Position, i+1)
		}
	}
	if result.Next.Position != 3 {
		t.Fatalf("Next.Position = %d, want 3", result.Next.Position)
	}
	if result.Retained.Earliest != 1 || result.Retained.Head != 5 {
		t.Fatalf("Retained = %+v, want Earliest=1 Head=5", result.Retained)
	}
}

func TestRead_AtHeadAfterConsumingEverything(t *testing.T) {
	st := New()
	ctx := context.Background()
	appendN(t, st, ctx, readTestTopic, 3)

	result, err := st.Read(ctx, events.ReadRequest{Topic: readTestTopic, From: events.Cursor{Topic: readTestTopic, Position: 3}, Limit: 10})
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if result.Outcome != events.ReadOutcomeAtHead {
		t.Fatalf("Outcome = %v, want ReadOutcomeAtHead", result.Outcome)
	}
	if err := result.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestRead_InvalidCursorAheadOfHead(t *testing.T) {
	st := New()
	ctx := context.Background()
	appendN(t, st, ctx, readTestTopic, 2)

	result, err := st.Read(ctx, events.ReadRequest{Topic: readTestTopic, From: events.Cursor{Topic: readTestTopic, Position: 99}, Limit: 10})
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if result.Outcome != events.ReadOutcomeInvalidCursor {
		t.Fatalf("Outcome = %v, want ReadOutcomeInvalidCursor", result.Outcome)
	}
	if err := result.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestRead_GapReportsEvictedPosition(t *testing.T) {
	st := NewWithRetention(2)
	ctx := context.Background()
	appendN(t, st, ctx, readTestTopic, 5) // retains only positions 4,5; evicts 1-3

	result, err := st.Read(ctx, events.ReadRequest{Topic: readTestTopic, From: events.Cursor{Topic: readTestTopic, Position: 1}, Limit: 10})
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if result.Outcome != events.ReadOutcomeGap {
		t.Fatalf("Outcome = %v, want ReadOutcomeGap", result.Outcome)
	}
	if err := result.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if result.Gap.Requested != 1 || result.Gap.EarliestRetained != 4 || result.Gap.Head != 5 {
		t.Fatalf("Gap = %+v, want Requested=1 EarliestRetained=4 Head=5", result.Gap)
	}
}

func TestRead_GapFromStartOfStreamWhenEarliestRecordEvicted(t *testing.T) {
	st := NewWithRetention(2)
	ctx := context.Background()
	appendN(t, st, ctx, readTestTopic, 5)

	// From position 0 (start-of-stream) also reports a gap once position 1
	// has been evicted: reading "from the start" would otherwise silently
	// skip evicted history instead of surfacing the loss.
	result, err := st.Read(ctx, events.ReadRequest{Topic: readTestTopic, From: events.Cursor{Topic: readTestTopic}, Limit: 10})
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if result.Outcome != events.ReadOutcomeGap {
		t.Fatalf("Outcome = %v, want ReadOutcomeGap", result.Outcome)
	}
	if err := result.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

// TestRead_FromExactlyBeforeEarliestRetainedIsNotAGap proves the earliest
// still-retained record is always reachable through a resumable cursor: a
// From naming exactly EarliestRetained-1 must return that record as
// ReadOutcomeProgress, not fabricate a gap for a position Events still
// retains (PR #1753 review finding 1, 2026-08-03T18:37:32Z).
func TestRead_FromExactlyBeforeEarliestRetainedIsNotAGap(t *testing.T) {
	st := NewWithRetention(2)
	ctx := context.Background()
	appendN(t, st, ctx, readTestTopic, 5) // retains only positions 4,5; evicts 1-3

	result, err := st.Read(ctx, events.ReadRequest{Topic: readTestTopic, From: events.Cursor{Topic: readTestTopic, Position: 3}, Limit: 10})
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if result.Outcome != events.ReadOutcomeProgress {
		t.Fatalf("Outcome = %v, want ReadOutcomeProgress (From=3 is EarliestRetained-1, not a gap)", result.Outcome)
	}
	if err := result.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if len(result.Records) != 2 || result.Records[0].ID.Position != 4 || result.Records[1].ID.Position != 5 {
		t.Fatalf("Records = %+v, want positions 4,5 (the earliest retained record must be reachable, not skipped)", result.Records)
	}
}

func TestRead_StartOfStreamIsNotAGapWhenNothingEvicted(t *testing.T) {
	st := NewWithRetention(10)
	ctx := context.Background()
	appendN(t, st, ctx, readTestTopic, 3)

	result, err := st.Read(ctx, events.ReadRequest{Topic: readTestTopic, From: events.Cursor{Topic: readTestTopic}, Limit: 10})
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if result.Outcome != events.ReadOutcomeProgress {
		t.Fatalf("Outcome = %v, want ReadOutcomeProgress (nothing evicted yet)", result.Outcome)
	}
	if len(result.Records) != 3 {
		t.Fatalf("len(Records) = %d, want 3", len(result.Records))
	}
}

func TestRead_RetentionEvictsOldestWithoutRenumberingHead(t *testing.T) {
	st := NewWithRetention(2)
	ctx := context.Background()
	appendN(t, st, ctx, readTestTopic, 4) // retains only positions 3,4; evicts 1,2

	result, err := st.Read(ctx, events.ReadRequest{Topic: readTestTopic, From: events.Cursor{Topic: readTestTopic, Position: 3}, Limit: 10})
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if result.Outcome != events.ReadOutcomeProgress {
		t.Fatalf("Outcome = %v, want ReadOutcomeProgress", result.Outcome)
	}
	if len(result.Records) != 1 {
		t.Fatalf("len(Records) = %d, want 1 (only position 4 is after From=3)", len(result.Records))
	}
	if result.Records[0].ID.Position != 4 {
		t.Fatalf("Records[0].ID.Position = %d, want 4", result.Records[0].ID.Position)
	}
	if result.Retained.Earliest != 3 || result.Retained.Head != 4 {
		t.Fatalf("Retained = %+v, want Earliest=3 Head=4 (head is never renumbered by eviction)", result.Retained)
	}
}

// TestRead_DuplicateIdentityIsAcceptedAsNewAfterEviction proves idempotency
// state is bounded by the same explicit retention policy as retained
// records: once a record's position has been evicted, ts.identity no longer
// holds its entry (see topicState.commitLocked), so repeating that identity
// is accepted as a new record instead of resolving as a duplicate forever.
// Retaining full accepted records/payloads in ts.identity indefinitely,
// unbounded by the declared retention policy, is the hidden second
// retention path this test guards against (PR #1753 review finding 2,
// 2026-08-03T18:37:32Z).
func TestRead_DuplicateIdentityIsAcceptedAsNewAfterEviction(t *testing.T) {
	st := NewWithRetention(1)
	ctx := context.Background()

	first := validAppendRequest()
	first.Topic = readTestTopic
	firstResult, err := st.Append(ctx, first)
	if err != nil {
		t.Fatalf("Append() first error = %v", err)
	}

	// Evict position 1 out of retention.
	second := validAppendRequest()
	second.Topic = readTestTopic
	second.SourceEventID = "evt-2"
	if _, err := st.Append(ctx, second); err != nil {
		t.Fatalf("Append() second error = %v", err)
	}

	repeat := first
	repeatResult, err := st.Append(ctx, repeat)
	if err != nil {
		t.Fatalf("Append() repeat error = %v", err)
	}
	if repeatResult.Outcome != events.AppendOutcomeAccepted {
		t.Fatalf("Outcome = %v, want AppendOutcomeAccepted (identity was evicted, so idempotency detection no longer applies)", repeatResult.Outcome)
	}
	if repeatResult.Record.ID == firstResult.Record.ID {
		t.Fatalf("repeat Record.ID = %+v, want a new position distinct from the evicted original %+v", repeatResult.Record.ID, firstResult.Record.ID)
	}
}

// TestRead_IdentityIndexStaysBoundedByRetentionPolicy proves the bounded
// retention policy applies to all retained state, not just the readable
// records slice: appending well beyond the retention cap must not leave
// ts.identity growing without bound (PR #1753 review finding 2,
// 2026-08-03T18:37:32Z).
func TestRead_IdentityIndexStaysBoundedByRetentionPolicy(t *testing.T) {
	const retentionCap = 5
	st := NewWithRetention(retentionCap)
	ctx := context.Background()
	appendN(t, st, ctx, readTestTopic, retentionCap*20)

	ts := st.topic(readTestTopic)
	ts.mu.Lock()
	recordCount := len(ts.records)
	identityCount := len(ts.identity)
	ts.mu.Unlock()

	if recordCount != retentionCap {
		t.Fatalf("len(records) = %d, want %d", recordCount, retentionCap)
	}
	if identityCount != retentionCap {
		t.Fatalf("len(identity) = %d, want %d (identity must stay bounded by the same retention policy as records, not grow with every append)", identityCount, retentionCap)
	}
}

func TestRead_IndependentCursorsDoNotAffectEachOther(t *testing.T) {
	st := New()
	ctx := context.Background()
	appendN(t, st, ctx, readTestTopic, 5)

	readerA, err := st.Read(ctx, events.ReadRequest{Topic: readTestTopic, From: events.Cursor{Topic: readTestTopic, Position: 1}, Limit: 1})
	if err != nil {
		t.Fatalf("Read(A) error = %v", err)
	}
	readerB, err := st.Read(ctx, events.ReadRequest{Topic: readTestTopic, From: events.Cursor{Topic: readTestTopic, Position: 4}, Limit: 1})
	if err != nil {
		t.Fatalf("Read(B) error = %v", err)
	}

	if readerA.Records[0].ID.Position != 2 {
		t.Fatalf("reader A Position = %d, want 2", readerA.Records[0].ID.Position)
	}
	if readerB.Records[0].ID.Position != 5 {
		t.Fatalf("reader B Position = %d, want 5", readerB.Records[0].ID.Position)
	}

	// Re-reading from A's original cursor must still yield the same next
	// record: B's read must not have advanced A's position or mutated the
	// topic's records.
	readerAAgain, err := st.Read(ctx, events.ReadRequest{Topic: readTestTopic, From: events.Cursor{Topic: readTestTopic, Position: 1}, Limit: 1})
	if err != nil {
		t.Fatalf("Read(A again) error = %v", err)
	}
	if readerAAgain.Records[0].ID.Position != 2 {
		t.Fatalf("reader A (again) Position = %d, want 2", readerAAgain.Records[0].ID.Position)
	}
}

func TestRead_RejectsMalformedRequestBeforeAnyLogOrStateChange(t *testing.T) {
	st := New()
	ctx := context.Background()

	_, err := st.Read(ctx, events.ReadRequest{Topic: readTestTopic, From: events.Cursor{Topic: readTestTopic}, Limit: 0})
	if !errors.Is(err, events.ErrInvalidReadLimit) {
		t.Fatalf("Read() error = %v, want ErrInvalidReadLimit", err)
	}

	// The topic must still behave as never touched: the first valid append
	// still lands at position 1.
	result, err := st.Append(ctx, validAppendRequest())
	if err != nil {
		t.Fatalf("Append() error = %v", err)
	}
	if result.Record.ID.Position != 1 {
		t.Fatalf("Position = %d, want 1 (rejected Read must not change aggregate state)", result.Record.ID.Position)
	}
}

func TestRead_RejectsCanceledContextBeforeAnyStateChange(t *testing.T) {
	st := New()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := st.Read(ctx, events.ReadRequest{Topic: readTestTopic, From: events.Cursor{Topic: readTestTopic}, Limit: 10})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Read() error = %v, want context.Canceled", err)
	}
}

func TestRead_RejectedAfterClose(t *testing.T) {
	st := New()
	ctx := context.Background()

	if err := st.Close(ctx); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	_, err := st.Read(ctx, events.ReadRequest{Topic: readTestTopic, From: events.Cursor{Topic: readTestTopic}, Limit: 10})
	if !errors.Is(err, events.ErrClosed) {
		t.Fatalf("Read() after Close error = %v, want ErrClosed", err)
	}
}

func TestRead_RejectedAfterCloseForATopicCreatedAfterwards(t *testing.T) {
	st := New()
	ctx := context.Background()

	if err := st.Close(ctx); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	topic := events.Topic("chat-session/never-seen-before/events")
	if _, err := st.Read(ctx, events.ReadRequest{Topic: topic, From: events.Cursor{Topic: topic}, Limit: 10}); !errors.Is(err, events.ErrClosed) {
		t.Fatalf("Read() on a brand-new topic after Close error = %v, want ErrClosed", err)
	}
}

func TestRead_ReturnedRecordsAreDetached(t *testing.T) {
	st := New()
	ctx := context.Background()
	req := validAppendRequest()
	req.Topic = readTestTopic
	if _, err := st.Append(ctx, req); err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	result, err := st.Read(ctx, events.ReadRequest{Topic: readTestTopic, From: events.Cursor{Topic: readTestTopic}, Limit: 10})
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	result.Records[0].Payload[2] = 'X'

	second, err := st.Read(ctx, events.ReadRequest{Topic: readTestTopic, From: events.Cursor{Topic: readTestTopic}, Limit: 10})
	if err != nil {
		t.Fatalf("Read() second error = %v", err)
	}
	want := json.RawMessage(`{"tool":"grep","status":"ok"}`)
	if string(second.Records[0].Payload) != string(want) {
		t.Fatalf("Records[0].Payload = %s, want unaffected by mutation of a prior returned Record", second.Records[0].Payload)
	}
}
