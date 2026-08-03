package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/events"
)

func appendFixture(st *Store, ctx context.Context, t *testing.T, topic events.Topic, seq events.SourceSequence, eventID events.SourceEventID) events.Record {
	t.Helper()
	result, err := st.Append(ctx, events.AppendRequest{
		Topic:          topic,
		SourceType:     "worker.tool",
		SourceID:       "worker-1",
		SourceSequence: seq,
		SourceEventID:  eventID,
		SchemaID:       "worker.output.v1",
		Payload:        json.RawMessage(`{"ok":true}`),
	})
	if err != nil {
		t.Fatalf("Append(%s, seq=%d) error = %v", topic, seq, err)
	}
	return result.Record
}

func readAll(st *Store, ctx context.Context, t *testing.T, topic events.Topic) []events.Record {
	t.Helper()
	result, err := st.Read(ctx, events.ReadRequest{Topic: topic, From: events.Cursor{Topic: topic}, Limit: 1000})
	if err != nil {
		t.Fatalf("Read(%s) error = %v", topic, err)
	}
	if result.Outcome == events.ReadOutcomeAtHead {
		return nil
	}
	if result.Outcome != events.ReadOutcomeProgress {
		t.Fatalf("Read(%s).Outcome = %v, want ReadOutcomeProgress or ReadOutcomeAtHead", topic, result.Outcome)
	}
	return result.Records
}

func TestAttachSource_RetainedThenLiveForwardsBacklogThenLiveCommits(t *testing.T) {
	st := New()
	ctx := context.Background()
	source := events.Topic("factory-session/abc/response-events")
	destination := events.Topic("chat-session/abc/events")

	appendFixture(st, ctx, t, source, 1, "evt-1")
	appendFixture(st, ctx, t, source, 2, "evt-2")

	result, err := st.AttachSource(ctx, events.AttachSourceRequest{
		Destination: destination,
		Source:      source,
		StartAt:     events.Cursor{Topic: source},
		Mode:        events.AttachModeRetainedThenLive,
	})
	if err != nil {
		t.Fatalf("AttachSource() error = %v", err)
	}
	if result.Outcome != events.AttachOutcomeAccepted {
		t.Fatalf("Outcome = %v, want AttachOutcomeAccepted", result.Outcome)
	}

	appendFixture(st, ctx, t, source, 3, "evt-3")

	destRecords := readAll(st, ctx, t, destination)
	if len(destRecords) != 3 {
		t.Fatalf("destination has %d records, want 3 (2 retained + 1 live)", len(destRecords))
	}
	for i, rec := range destRecords {
		wantSeq := events.SourceSequence(i + 1)
		if rec.SourceSequence != wantSeq {
			t.Fatalf("destination record[%d].SourceSequence = %d, want %d", i, rec.SourceSequence, wantSeq)
		}
		if rec.ID.Position != events.AggregateSequence(i+1) {
			t.Fatalf("destination record[%d].ID.Position = %d, want %d (own contiguous ordering)", i, rec.ID.Position, i+1)
		}
		if rec.ID.Topic != destination {
			t.Fatalf("destination record[%d].ID.Topic = %v, want %v", i, rec.ID.Topic, destination)
		}
		if string(rec.Payload) != `{"ok":true}` {
			t.Fatalf("destination record[%d].Payload = %s, want the source payload preserved", i, rec.Payload)
		}
	}
}

func TestAttachSource_RetainedThenLiveFromMidpointSkipsEarlierRecords(t *testing.T) {
	st := New()
	ctx := context.Background()
	source := events.Topic("factory-session/mid/response-events")
	destination := events.Topic("chat-session/mid/events")

	appendFixture(st, ctx, t, source, 1, "evt-1")
	second := appendFixture(st, ctx, t, source, 2, "evt-2")
	appendFixture(st, ctx, t, source, 3, "evt-3")

	result, err := st.AttachSource(ctx, events.AttachSourceRequest{
		Destination: destination,
		Source:      source,
		StartAt:     events.Cursor{Topic: source, Position: second.ID.Position},
		Mode:        events.AttachModeRetainedThenLive,
	})
	if err != nil {
		t.Fatalf("AttachSource() error = %v", err)
	}
	if result.Outcome != events.AttachOutcomeAccepted {
		t.Fatalf("Outcome = %v, want AttachOutcomeAccepted", result.Outcome)
	}

	destRecords := readAll(st, ctx, t, destination)
	if len(destRecords) != 1 {
		t.Fatalf("destination has %d records, want 1 (only the record after StartAt)", len(destRecords))
	}
	if destRecords[0].SourceSequence != 3 {
		t.Fatalf("destination record.SourceSequence = %d, want 3 (the only record after StartAt=2)", destRecords[0].SourceSequence)
	}
}

func TestAttachSource_LiveOnlyForwardsNoRetainedHistory(t *testing.T) {
	st := New()
	ctx := context.Background()
	source := events.Topic("factory-session/live/response-events")
	destination := events.Topic("chat-session/live/events")

	appendFixture(st, ctx, t, source, 1, "evt-1")
	appendFixture(st, ctx, t, source, 2, "evt-2")

	result, err := st.AttachSource(ctx, events.AttachSourceRequest{
		Destination: destination,
		Source:      source,
		StartAt:     events.Cursor{Topic: source},
		Mode:        events.AttachModeLiveOnly,
	})
	if err != nil {
		t.Fatalf("AttachSource() error = %v", err)
	}
	if result.StartAt.Position != 2 {
		t.Fatalf("StartAt.Position = %d, want 2 (the observed live head at attach time)", result.StartAt.Position)
	}

	if destRecords := readAll(st, ctx, t, destination); len(destRecords) != 0 {
		t.Fatalf("destination has %d records immediately after a live-only attach, want 0", len(destRecords))
	}

	appendFixture(st, ctx, t, source, 3, "evt-3")

	destRecords := readAll(st, ctx, t, destination)
	if len(destRecords) != 1 {
		t.Fatalf("destination has %d records, want 1 (only the record committed after attach)", len(destRecords))
	}
	if destRecords[0].SourceSequence != 3 {
		t.Fatalf("destination record.SourceSequence = %d, want 3", destRecords[0].SourceSequence)
	}
}

func TestAttachSource_IdempotentAttachmentDoesNotForwardTwice(t *testing.T) {
	st := New()
	ctx := context.Background()
	source := events.Topic("factory-session/idem/response-events")
	destination := events.Topic("chat-session/idem/events")

	req := events.AttachSourceRequest{
		Destination: destination,
		Source:      source,
		StartAt:     events.Cursor{Topic: source},
		Mode:        events.AttachModeRetainedThenLive,
	}

	first, err := st.AttachSource(ctx, req)
	if err != nil {
		t.Fatalf("AttachSource() first error = %v", err)
	}
	if first.Outcome != events.AttachOutcomeAccepted {
		t.Fatalf("first Outcome = %v, want AttachOutcomeAccepted", first.Outcome)
	}

	second, err := st.AttachSource(ctx, req)
	if err != nil {
		t.Fatalf("AttachSource() second error = %v", err)
	}
	if second.Outcome != events.AttachOutcomeAlreadyAttached {
		t.Fatalf("second Outcome = %v, want AttachOutcomeAlreadyAttached", second.Outcome)
	}
	if second.ID != first.ID {
		t.Fatalf("second.ID = %+v, want the original %+v", second.ID, first.ID)
	}
	if second.StartAt != first.StartAt {
		t.Fatalf("second.StartAt = %+v, want the original %+v", second.StartAt, first.StartAt)
	}

	appendFixture(st, ctx, t, source, 1, "evt-1")

	destRecords := readAll(st, ctx, t, destination)
	if len(destRecords) != 1 {
		t.Fatalf("destination has %d records, want exactly 1: a repeated attachment must not create a second forwarding path", len(destRecords))
	}
}

func TestAttachSource_ValidationBeforeEffects(t *testing.T) {
	st := New()
	source := events.Topic("factory-session/val/response-events")
	destination := events.Topic("chat-session/val/events")
	appendFixture(st, context.Background(), t, source, 1, "evt-1")

	tests := []struct {
		name    string
		req     events.AttachSourceRequest
		ctx     context.Context
		wantErr error
	}{
		{
			name: "malformed request",
			req: events.AttachSourceRequest{
				Destination: "", Source: source,
				StartAt: events.Cursor{Topic: source}, Mode: events.AttachModeRetainedThenLive,
			},
			ctx:     context.Background(),
			wantErr: events.ErrEmptyTopic,
		},
		{
			name: "self attachment",
			req: events.AttachSourceRequest{
				Destination: source, Source: source,
				StartAt: events.Cursor{Topic: source}, Mode: events.AttachModeRetainedThenLive,
			},
			ctx:     context.Background(),
			wantErr: events.ErrSelfAttachment,
		},
		{
			name: "incompatible cursor",
			req: events.AttachSourceRequest{
				Destination: destination, Source: source,
				StartAt: events.Cursor{Topic: destination}, Mode: events.AttachModeRetainedThenLive,
			},
			ctx:     context.Background(),
			wantErr: events.ErrIncompatibleAttachmentCursor,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := st.AttachSource(tt.ctx, tt.req)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("AttachSource() error = %v, want %v", err, tt.wantErr)
			}
			if destRecords := readAll(st, context.Background(), t, destination); len(destRecords) != 0 {
				t.Fatalf("destination has %d records after a rejected AttachSource, want 0 (no partial attachment/forwarding)", len(destRecords))
			}
		})
	}

	// A rejected request must not register an attachment: a valid attach for
	// the same pair afterward must still report AttachOutcomeAccepted, not
	// AttachOutcomeAlreadyAttached.
	result, err := st.AttachSource(context.Background(), events.AttachSourceRequest{
		Destination: destination, Source: source,
		StartAt: events.Cursor{Topic: source}, Mode: events.AttachModeRetainedThenLive,
	})
	if err != nil {
		t.Fatalf("AttachSource() after rejections error = %v", err)
	}
	if result.Outcome != events.AttachOutcomeAccepted {
		t.Fatalf("Outcome = %v, want AttachOutcomeAccepted (no attachment should have been registered by any rejected call)", result.Outcome)
	}
}

func TestAttachSource_CanceledContextRejectsBeforeEffects(t *testing.T) {
	st := New()
	source := events.Topic("factory-session/cancel/response-events")
	destination := events.Topic("chat-session/cancel/events")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := st.AttachSource(ctx, events.AttachSourceRequest{
		Destination: destination, Source: source,
		StartAt: events.Cursor{Topic: source}, Mode: events.AttachModeRetainedThenLive,
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("AttachSource() error = %v, want context.Canceled", err)
	}

	result, err := st.AttachSource(context.Background(), events.AttachSourceRequest{
		Destination: destination, Source: source,
		StartAt: events.Cursor{Topic: source}, Mode: events.AttachModeRetainedThenLive,
	})
	if err != nil {
		t.Fatalf("AttachSource() after canceled call error = %v", err)
	}
	if result.Outcome != events.AttachOutcomeAccepted {
		t.Fatalf("Outcome = %v, want AttachOutcomeAccepted (a canceled call must not have registered an attachment)", result.Outcome)
	}
}

func TestAttachSource_StartAtBeyondHeadIsRejected(t *testing.T) {
	st := New()
	ctx := context.Background()
	source := events.Topic("factory-session/stale/response-events")
	destination := events.Topic("chat-session/stale/events")
	appendFixture(st, ctx, t, source, 1, "evt-1")

	_, err := st.AttachSource(ctx, events.AttachSourceRequest{
		Destination: destination,
		Source:      source,
		StartAt:     events.Cursor{Topic: source, Position: 5},
		Mode:        events.AttachModeRetainedThenLive,
	})
	if !errors.Is(err, events.ErrUnresolvableCursor) {
		t.Fatalf("AttachSource() error = %v, want ErrUnresolvableCursor", err)
	}
	if destRecords := readAll(st, ctx, t, destination); len(destRecords) != 0 {
		t.Fatalf("destination has %d records after a rejected AttachSource, want 0", len(destRecords))
	}
}

func TestAttachSource_EvictedStartAtIsRejectedWithoutPartialForwarding(t *testing.T) {
	st := NewWithRetention(2)
	ctx := context.Background()
	source := events.Topic("factory-session/gap/response-events")
	destination := events.Topic("chat-session/gap/events")

	appendFixture(st, ctx, t, source, 1, "evt-1") // evicted once seq 4 commits
	appendFixture(st, ctx, t, source, 2, "evt-2")
	appendFixture(st, ctx, t, source, 3, "evt-3")
	appendFixture(st, ctx, t, source, 4, "evt-4") // retention cap 2 -> records 3,4 retained

	_, err := st.AttachSource(ctx, events.AttachSourceRequest{
		Destination: destination,
		Source:      source,
		StartAt:     events.Cursor{Topic: source, Position: 1},
		Mode:        events.AttachModeRetainedThenLive,
	})
	if !errors.Is(err, events.ErrUnresolvableCursor) {
		t.Fatalf("AttachSource() error = %v, want ErrUnresolvableCursor (a StartAt naming an evicted position must be rejected, not silently recovered)", err)
	}

	if destRecords := readAll(st, ctx, t, destination); len(destRecords) != 0 {
		t.Fatalf("destination has %d records after a rejected AttachSource, want 0 (no partial attachment/forwarding)", len(destRecords))
	}

	// A rejected request must not register an attachment: a valid attach for
	// the same pair afterward must still report AttachOutcomeAccepted.
	result, err := st.AttachSource(ctx, events.AttachSourceRequest{
		Destination: destination,
		Source:      source,
		StartAt:     events.Cursor{Topic: source, Position: 4},
		Mode:        events.AttachModeRetainedThenLive,
	})
	if err != nil {
		t.Fatalf("AttachSource() after rejection error = %v", err)
	}
	if result.Outcome != events.AttachOutcomeAccepted {
		t.Fatalf("Outcome = %v, want AttachOutcomeAccepted (no attachment should have been registered by the rejected call)", result.Outcome)
	}
}

func TestAttachSource_IndirectCycleIsRejectedWithoutDeadlock(t *testing.T) {
	st := New()
	ctx := context.Background()
	a := events.Topic("factory-session/cycle-a/response-events")
	b := events.Topic("factory-session/cycle-b/response-events")
	c := events.Topic("factory-session/cycle-c/response-events")

	// a -> b -> c is a valid acyclic chain.
	if _, err := st.AttachSource(ctx, events.AttachSourceRequest{
		Destination: b, Source: a, StartAt: events.Cursor{Topic: a}, Mode: events.AttachModeRetainedThenLive,
	}); err != nil {
		t.Fatalf("AttachSource(a->b) error = %v", err)
	}
	if _, err := st.AttachSource(ctx, events.AttachSourceRequest{
		Destination: c, Source: b, StartAt: events.Cursor{Topic: b}, Mode: events.AttachModeRetainedThenLive,
	}); err != nil {
		t.Fatalf("AttachSource(b->c) error = %v", err)
	}

	// c -> a would close the cycle a -> b -> c -> a and must be rejected.
	_, err := st.AttachSource(ctx, events.AttachSourceRequest{
		Destination: a, Source: c, StartAt: events.Cursor{Topic: c}, Mode: events.AttachModeRetainedThenLive,
	})
	if !errors.Is(err, events.ErrSelfAttachment) {
		t.Fatalf("AttachSource(c->a) error = %v, want ErrSelfAttachment (indirect cycle)", err)
	}

	// Proves no deadlock was introduced: an append cascading a -> b -> c must
	// still complete and observe the accepted chain, not the rejected edge.
	done := make(chan struct{})
	go func() {
		appendFixture(st, ctx, t, a, 1, "evt-1")
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Append cascading through the attachment chain did not complete, want no deadlock")
	}

	if recs := readAll(st, ctx, t, c); len(recs) != 1 {
		t.Fatalf("destination c has %d records, want 1 (forwarded through the accepted a->b->c chain)", len(recs))
	}
	if recs := readAll(st, ctx, t, a); len(recs) != 1 {
		t.Fatalf("source a has %d records, want 1 (the rejected c->a edge must not have forwarded anything back)", len(recs))
	}
}

func TestAttachSource_ClosedSourceTopicIsRejected(t *testing.T) {
	st := New()
	ctx := context.Background()
	source := events.Topic("factory-session/closed/response-events")
	destination := events.Topic("chat-session/closed/events")
	appendFixture(st, ctx, t, source, 1, "evt-1")

	if err := st.Close(ctx); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	_, err := st.AttachSource(ctx, events.AttachSourceRequest{
		Destination: destination,
		Source:      source,
		StartAt:     events.Cursor{Topic: source},
		Mode:        events.AttachModeRetainedThenLive,
	})
	if !errors.Is(err, events.ErrOperationFailed) {
		t.Fatalf("AttachSource() error = %v, want ErrOperationFailed", err)
	}
	// Read itself also rejects after Close (story 005 shutdown scope), so
	// the destination's record count is confirmed by inspecting the topic
	// directly rather than through the now-rejecting public Read.
	destTS := st.topic(destination)
	destTS.mu.Lock()
	destCount := len(destTS.records)
	destTS.mu.Unlock()
	if destCount != 0 {
		t.Fatalf("destination has %d records after AttachSource on a closed source, want 0", destCount)
	}
}

func TestAttachSource_CloseTearsDownActiveForwarding(t *testing.T) {
	st := New()
	ctx := context.Background()
	source := events.Topic("factory-session/teardown/response-events")
	destination := events.Topic("chat-session/teardown/events")

	appendFixture(st, ctx, t, source, 1, "evt-1")
	if _, err := st.AttachSource(ctx, events.AttachSourceRequest{
		Destination: destination,
		Source:      source,
		StartAt:     events.Cursor{Topic: source},
		Mode:        events.AttachModeRetainedThenLive,
	}); err != nil {
		t.Fatalf("AttachSource() error = %v", err)
	}

	before := readAll(st, ctx, t, destination)
	if len(before) != 1 {
		t.Fatalf("destination has %d records before Close, want 1", len(before))
	}

	if err := st.Close(ctx); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	// White-box: prove the source topic's outgoing attachment registration
	// was itself torn down by Close, independent of the fact that Append
	// also now rejects everything after Close (see
	// TestAppend_RejectedAfterClose) -- this is what actually stops a later
	// commit from ever reaching forwardRecord, not merely that Append never
	// gets that far.
	ts := st.topic(source)
	ts.mu.Lock()
	attachmentCount := len(ts.attachments)
	ts.mu.Unlock()
	if attachmentCount != 0 {
		t.Fatalf("source topic has %d attachment registration(s) after Close, want 0 (forwarding must be torn down)", attachmentCount)
	}

	// Append itself is now rejected after Close (story 005 shutdown scope),
	// so a later source commit cannot reach forwarding at all.
	if _, err := st.Append(ctx, events.AppendRequest{
		Topic:          source,
		SourceType:     "worker.tool",
		SourceID:       "worker-1",
		SourceSequence: 2,
		SourceEventID:  "evt-2",
		SchemaID:       "worker.output.v1",
		Payload:        json.RawMessage(`{"ok":true}`),
	}); !errors.Is(err, events.ErrClosed) {
		t.Fatalf("Append() after Close error = %v, want ErrClosed", err)
	}

	// Read itself also rejects after Close (story 005 shutdown scope), so
	// the destination's still-1-record state is confirmed by inspecting the
	// topic directly rather than through the now-rejecting public Read.
	destTS := st.topic(destination)
	destTS.mu.Lock()
	destCount := len(destTS.records)
	destTS.mu.Unlock()
	if destCount != 1 {
		t.Fatalf("destination has %d records after Close, want still 1 (forwarding must stop once the source topic is closed)", destCount)
	}
}
