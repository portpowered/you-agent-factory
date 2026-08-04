package service_test

import (
	"context"
	"sync"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/events"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/worker_sessions/internal/service"
)

// TestPublishRecord_SourceIdentityTupleMembersRemainDistinct proves that
// changing any single member of the (SourceType, SourceID, SourceSequence,
// SourceEventID) tuple, while holding the other three fixed, is treated as a
// wholly distinct record rather than a duplicate of the base identity.
func TestPublishRecord_SourceIdentityTupleMembersRemainDistinct(t *testing.T) {
	eventsSvc := newEventsAppender()
	registry, err := service.New(succeedingExecution(), eventsSvc, nil)
	if err != nil {
		t.Fatalf("service.New() error = %v, want nil", err)
	}
	ctx := context.Background()

	base := workersessions.PublishRecordRequest{
		SessionID:      "worker-1",
		Draft:          toolDraft("tc-base"),
		SourceType:     "worker_provider",
		SourceID:       "src-1",
		SourceSequence: 1,
		SourceEventID:  "evt-1",
		SchemaID:       "workers.draft.v1",
	}

	variants := []workersessions.PublishRecordRequest{base}
	withSourceType := base
	withSourceType.SourceType = "worker_provider_alt"
	variants = append(variants, withSourceType)
	withSourceID := base
	withSourceID.SourceID = "src-2"
	variants = append(variants, withSourceID)
	withSourceSequence := base
	withSourceSequence.SourceSequence = 2
	variants = append(variants, withSourceSequence)
	withSourceEventID := base
	withSourceEventID.SourceEventID = "evt-2"
	variants = append(variants, withSourceEventID)

	seen := make(map[events.AggregateSequence]bool, len(variants))
	for i, req := range variants {
		result, err := registry.PublishRecord(ctx, req)
		if err != nil {
			t.Fatalf("PublishRecord() [%d] error = %v, want nil", i, err)
		}
		if result.Outcome != workersessions.PublishOutcomeAccepted {
			t.Fatalf("PublishRecord() [%d] outcome = %v, want Accepted (a distinct tuple member must never be treated as a duplicate)", i, result.Outcome)
		}
		if seen[result.AggregateSequence] {
			t.Fatalf("PublishRecord() [%d] aggregate sequence %d was already assigned to a different variant", i, result.AggregateSequence)
		}
		seen[result.AggregateSequence] = true
	}
	if len(seen) != len(variants) {
		t.Fatalf("observed %d distinct aggregate sequences, want %d", len(seen), len(variants))
	}
}

// TestPublishRecord_IdenticalTupleAcrossSessionTopics_DoesNotCollapse proves
// that presenting the identical (SourceType, SourceID, SourceSequence,
// SourceEventID) tuple against two different Worker Session topics is
// accepted independently on each: Events' idempotency dedup is scoped per
// topic, not globally across every Worker Session.
func TestPublishRecord_IdenticalTupleAcrossSessionTopics_DoesNotCollapse(t *testing.T) {
	eventsSvc := newEventsAppender()
	registry, err := service.New(succeedingExecution(), eventsSvc, nil)
	if err != nil {
		t.Fatalf("service.New() error = %v, want nil", err)
	}
	ctx := context.Background()

	identicalTuple := func(sessionID string) workersessions.PublishRecordRequest {
		return workersessions.PublishRecordRequest{
			SessionID:      sessionID,
			Draft:          toolDraft("tc-1"),
			SourceType:     "worker_provider",
			SourceID:       "shared-src",
			SourceSequence: 1,
			SourceEventID:  "shared-evt",
			SchemaID:       "workers.draft.v1",
		}
	}

	first, err := registry.PublishRecord(ctx, identicalTuple("worker-1"))
	if err != nil {
		t.Fatalf("first PublishRecord() error = %v, want nil", err)
	}
	if first.Outcome != workersessions.PublishOutcomeAccepted {
		t.Fatalf("first PublishRecord() outcome = %v, want Accepted", first.Outcome)
	}

	second, err := registry.PublishRecord(ctx, identicalTuple("worker-2"))
	if err != nil {
		t.Fatalf("second PublishRecord() error = %v, want nil", err)
	}
	if second.Outcome != workersessions.PublishOutcomeAccepted {
		t.Fatalf("second PublishRecord() outcome = %v, want Accepted (a different Worker Session topic must not collapse an identical tuple into a duplicate)", second.Outcome)
	}

	for _, sessionID := range []string{"worker-1", "worker-2"} {
		committed := readAllDrafts(t, eventsSvc, workersessions.Topic(sessionID))
		if len(committed) != 1 {
			t.Fatalf("session %s committed record count = %d, want 1", sessionID, len(committed))
		}
	}
}

// TestPublishRecord_PagedReadDeliversRecordsExactlyOnceInContiguousOrder
// proves that reading a Worker Session topic in bounded pages smaller than
// its total record count, and resuming from each returned cursor, delivers
// the opening record and every published Worker record exactly once, in
// contiguous commit order, with no gap or duplicate across the page
// boundary.
func TestPublishRecord_PagedReadDeliversRecordsExactlyOnceInContiguousOrder(t *testing.T) {
	eventsSvc := newEventsAppender()
	registry, err := service.New(succeedingExecution(), eventsSvc, nil)
	if err != nil {
		t.Fatalf("service.New() error = %v, want nil", err)
	}
	ctx := context.Background()

	if _, err := registry.Start(ctx, validStartRequest("worker-1", "dispatch-1")); err != nil {
		t.Fatalf("Start() error = %v, want nil", err)
	}
	const published = 5
	for i := 1; i <= published; i++ {
		req := validPublishRecordRequest("worker-1", events.SourceSequence(i), progressDraft("step"))
		if _, err := registry.PublishRecord(ctx, req); err != nil {
			t.Fatalf("PublishRecord() [%d] error = %v, want nil", i, err)
		}
	}
	const wantTotal = published + 1 // + the W3-001 opening record

	topic := workersessions.Topic("worker-1")
	var page events.ReadResult
	var all []events.Record
	cursor := events.Cursor{Topic: topic}
	for pages := 0; ; pages++ {
		if pages > wantTotal {
			t.Fatalf("Read() paging did not reach ReadOutcomeAtHead within %d pages", wantTotal)
		}
		page, err = eventsSvc.Read(ctx, events.ReadRequest{Topic: topic, From: cursor, Limit: 2})
		if err != nil {
			t.Fatalf("Read() error = %v, want nil", err)
		}
		if page.Outcome == events.ReadOutcomeAtHead {
			break
		}
		if page.Outcome != events.ReadOutcomeProgress {
			t.Fatalf("Read() outcome = %v, want Progress or AtHead", page.Outcome)
		}
		all = append(all, page.Records...)
		cursor = page.Next
	}

	if len(all) != wantTotal {
		t.Fatalf("paged Read() delivered %d records, want %d", len(all), wantTotal)
	}
	for i, rec := range all {
		wantPosition := events.AggregateSequence(i + 1)
		if rec.ID.Position != wantPosition {
			t.Fatalf("record[%d] position = %d, want %d (contiguous, no gap or duplicate)", i, rec.ID.Position, wantPosition)
		}
	}
}

// TestPublishRecord_SubscriptionFromLastReadCursorDeliversOnlyLaterRecords
// proves read-to-subscribe continuation: a subscription opened from the
// cursor a prior Read already fully consumed resumes with only records
// published after that point, never re-delivering what was already read.
func TestPublishRecord_SubscriptionFromLastReadCursorDeliversOnlyLaterRecords(t *testing.T) {
	eventsSvc := newEventsAppender()
	registry, err := service.New(succeedingExecution(), eventsSvc, nil)
	if err != nil {
		t.Fatalf("service.New() error = %v, want nil", err)
	}
	ctx := context.Background()
	topic := workersessions.Topic("worker-1")

	if _, err := registry.Start(ctx, validStartRequest("worker-1", "dispatch-1")); err != nil {
		t.Fatalf("Start() error = %v, want nil", err)
	}
	if _, err := registry.PublishRecord(ctx, validPublishRecordRequest("worker-1", 1, toolDraft("tc-1"))); err != nil {
		t.Fatalf("PublishRecord() [1] error = %v, want nil", err)
	}

	readResult, err := eventsSvc.Read(ctx, events.ReadRequest{Topic: topic, From: events.Cursor{Topic: topic}, Limit: 100})
	if err != nil {
		t.Fatalf("Read() error = %v, want nil", err)
	}
	if readResult.Outcome != events.ReadOutcomeProgress || len(readResult.Records) != 2 {
		t.Fatalf("Read() = %+v, want Progress with 2 records (opening + published)", readResult)
	}
	lastReadCursor := readResult.Next

	sub, err := eventsSvc.Subscribe(ctx, events.SubscribeRequest{Topic: topic, From: lastReadCursor, Limit: 10})
	if err != nil {
		t.Fatalf("Subscribe() error = %v, want nil", err)
	}

	if _, err := registry.PublishRecord(ctx, validPublishRecordRequest("worker-1", 2, toolDraft("tc-2"))); err != nil {
		t.Fatalf("PublishRecord() [2] error = %v, want nil", err)
	}
	if _, err := registry.PublishRecord(ctx, validPublishRecordRequest("worker-1", 3, toolDraft("tc-3"))); err != nil {
		t.Fatalf("PublishRecord() [3] error = %v, want nil", err)
	}

	first := sub.Next(ctx)
	if first.Kind != events.DeliveryRecord || first.Cursor.Position != 3 {
		t.Fatalf("first Subscription.Next() = %+v, want DeliveryRecord at position 3 (only later records, never re-delivering the already-read positions 1-2)", first)
	}
	second := sub.Next(ctx)
	if second.Kind != events.DeliveryRecord || second.Cursor.Position != 4 {
		t.Fatalf("second Subscription.Next() = %+v, want DeliveryRecord at position 4", second)
	}
}

// TestPublishRecord_ConcurrentDuplicateDeliveryConvergesOnOneRecord proves
// that many goroutines racing the identical PublishRecord call converge on
// exactly one accepted record: every other racer resolves to
// PublishOutcomeDuplicate naming the same AggregateSequence, never a second
// committed position.
func TestPublishRecord_ConcurrentDuplicateDeliveryConvergesOnOneRecord(t *testing.T) {
	const goroutines = 50

	eventsSvc := newEventsAppender()
	registry, err := service.New(succeedingExecution(), eventsSvc, nil)
	if err != nil {
		t.Fatalf("service.New() error = %v, want nil", err)
	}
	ctx := context.Background()
	req := validPublishRecordRequest("worker-1", 1, toolDraft("tc-1"))

	var wg sync.WaitGroup
	results := make(chan workersessions.PublishRecordResult, goroutines)
	for range goroutines {
		wg.Go(func() {
			result, err := registry.PublishRecord(ctx, req)
			if err != nil {
				t.Errorf("PublishRecord() error = %v, want nil", err)
				return
			}
			results <- result
		})
	}
	wg.Wait()
	close(results)

	accepted := 0
	positions := make(map[events.AggregateSequence]bool)
	for result := range results {
		if result.Outcome == workersessions.PublishOutcomeAccepted {
			accepted++
		}
		positions[result.AggregateSequence] = true
	}
	if accepted != 1 {
		t.Fatalf("accepted count = %d, want exactly 1 across %d concurrent racers", accepted, goroutines)
	}
	if len(positions) != 1 {
		t.Fatalf("observed %d distinct aggregate sequences, want exactly 1: every racer must resolve to the same committed record", len(positions))
	}

	committed := readAllDrafts(t, eventsSvc, workersessions.Topic("worker-1"))
	if len(committed) != 1 {
		t.Fatalf("committed record count = %d, want exactly 1", len(committed))
	}
}
