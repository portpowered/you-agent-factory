package service_test

import (
	"context"
	"encoding/json"
	"strconv"
	"sync"
	"testing"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	"github.com/portpowered/infinite-you/pkg/services/events"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// TestPublishRecord_RetryOfOlderAcceptedIdentity_AfterNewerSequenceAccepted_ResolvesAsDuplicate
// proves that once a higher SourceSequence has been accepted for a source, an
// exact retry of an earlier identity that source already had accepted still
// resolves to the original record as a duplicate rather than being rejected
// as out of order: Events retains every accepted identity permanently for
// dedup, so a retry must stay idempotent regardless of publication order
// since. It also proves the retry produces neither a new committed position
// nor a live subscription delivery of its own.
// pkgmaintcheck:ignore-cyclomatic-complexity pre-existing baseline debt recorded 2026-08-08; refactor this code below the maintainability threshold and remove this exemption
func TestPublishRecord_RetryOfOlderAcceptedIdentity_AfterNewerSequenceAccepted_ResolvesAsDuplicate(t *testing.T) {
	eventsSvc := newEventsAppender()
	topic := workersessions.Topic("worker-1")

	var svc workersessions.Service
	var first, second, retry workersessions.PublishRecordResult

	execution := &fakeExecution{
		dispatch: func(ctx context.Context, req workers.WorkstationDispatchRequest) (workers.WorkstationDispatchResult, error) {
			var err error
			if first, err = svc.PublishRecord(ctx, validPublishRecordRequest("worker-1", 1, toolDraft("tc-1"))); err != nil {
				t.Fatalf("PublishRecord() seq=1 error = %v, want nil", err)
			}
			if second, err = svc.PublishRecord(ctx, validPublishRecordRequest("worker-1", 2, toolDraft("tc-2"))); err != nil {
				t.Fatalf("PublishRecord() seq=2 error = %v, want nil", err)
			}

			readResult, err := eventsSvc.Read(ctx, events.ReadRequest{Topic: topic, From: events.Cursor{Topic: topic}, Limit: 100})
			if err != nil {
				t.Fatalf("Read() error = %v, want nil", err)
			}
			if readResult.Outcome != events.ReadOutcomeProgress || len(readResult.Records) != 3 {
				t.Fatalf("Read() = %+v, want Progress with 3 records (opening, seq=1, seq=2)", readResult)
			}
			sub, err := eventsSvc.Subscribe(ctx, events.SubscribeRequest{Topic: topic, From: readResult.Next, Limit: 10})
			if err != nil {
				t.Fatalf("Subscribe() error = %v, want nil", err)
			}

			if retry, err = svc.PublishRecord(ctx, validPublishRecordRequest("worker-1", 1, toolDraft("tc-1"))); err != nil {
				t.Fatalf("retry PublishRecord() seq=1 error = %v, want nil", err)
			}
			if _, err := svc.PublishRecord(ctx, validPublishRecordRequest("worker-1", 3, toolDraft("tc-3"))); err != nil {
				t.Fatalf("PublishRecord() seq=3 error = %v, want nil", err)
			}

			delivery := sub.Next(ctx)
			if delivery.Kind != events.DeliveryRecord || delivery.Cursor.Position != 4 {
				t.Fatalf("Subscription.Next() = %+v, want the seq=3 record delivered directly at position 4 -- the seq=1 retry must not have produced a delivery of its own", delivery)
			}

			return workers.WorkstationDispatchResult{
				DispatchID: req.Execution.Dispatch.DispatchID,
				Result: workers.WorkResult{
					DispatchID: req.Execution.Dispatch.DispatchID,
					Outcome:    workers.OutcomeAccepted,
				},
			}, nil
		},
	}
	var err error
	svc, err = newService(executionBoundary{execution: execution}, eventsSvc, nil)
	if err != nil {
		t.Fatalf("service.New() error = %v, want nil", err)
	}

	if _, err := svc.InvokeSession(context.Background(), validStartRequest("worker-1", "dispatch-1")); err != nil {
		t.Fatalf("Start() error = %v, want nil", err)
	}
	if second.Outcome != workersessions.PublishOutcomeAccepted {
		t.Fatalf("PublishRecord() seq=2 outcome = %v, want PublishOutcomeAccepted", second.Outcome)
	}
	if retry.Outcome != workersessions.PublishOutcomeDuplicate {
		t.Fatalf("retry PublishRecord() seq=1 outcome = %v, want PublishOutcomeDuplicate", retry.Outcome)
	}
	if retry.AggregateSequence != first.AggregateSequence {
		t.Fatalf("retry PublishRecord() aggregate sequence = %d, want %d (original seq=1 record, unchanged)", retry.AggregateSequence, first.AggregateSequence)
	}

	committed := readAllDrafts(t, eventsSvc, topic)
	if len(committed) != 5 {
		t.Fatalf("committed record count = %d, want 5 (opening, seq=1, seq=2, seq=3, terminal; the retry of seq=1 must not create a sixth position)", len(committed))
	}
}

// TestPublishRecord_ConcurrentPublishesForOneSource_NeverCommitOutOfOrder
// stress-tests the ordering guarantee under real concurrency: several
// goroutines race to publish distinct SourceSequence values for the same
// source, released simultaneously. Whichever calls are accepted must appear
// in the topic in strictly increasing SourceSequence order -- the ordering
// enforcement must hold regardless of goroutine scheduling, not just for a
// hand-sequenced call order.
func TestPublishRecord_ConcurrentPublishesForOneSource_NeverCommitOutOfOrder(t *testing.T) {
	eventsSvc := newEventsAppender()
	const n = 10

	var svc workersessions.Service
	execution := &fakeExecution{
		dispatch: func(ctx context.Context, req workers.WorkstationDispatchRequest) (workers.WorkstationDispatchResult, error) {
			var wg sync.WaitGroup
			ready := make(chan struct{})
			wg.Add(n)
			for i := 1; i <= n; i++ {
				go func(seq int) {
					defer wg.Done()
					<-ready
					_, _ = svc.PublishRecord(ctx, validPublishRecordRequest("worker-1", events.SourceSequence(seq), progressDraft(strconv.Itoa(seq))))
				}(i)
			}
			close(ready)
			wg.Wait()
			return workers.WorkstationDispatchResult{
				DispatchID: req.Execution.Dispatch.DispatchID,
				Result: workers.WorkResult{
					DispatchID: req.Execution.Dispatch.DispatchID,
					Outcome:    workers.OutcomeAccepted,
				},
			}, nil
		},
	}
	var err error
	svc, err = newService(executionBoundary{execution: execution}, eventsSvc, nil)
	if err != nil {
		t.Fatalf("service.New() error = %v, want nil", err)
	}

	if _, err := svc.InvokeSession(context.Background(), validStartRequest("worker-1", "dispatch-1")); err != nil {
		t.Fatalf("Start() error = %v, want nil", err)
	}

	committed := readAllDrafts(t, eventsSvc, workersessions.Topic("worker-1"))
	if len(committed) < 3 {
		t.Fatalf("committed record count = %d, want at least 3 (opening, >=1 published, terminal)", len(committed))
	}
	lastSeq := 0
	for _, draft := range committed[1 : len(committed)-1] {
		var payload workers.ProgressPayload
		if err := json.Unmarshal(draft.Payload, &payload); err != nil {
			t.Fatalf("unmarshal progress payload error = %v", err)
		}
		seq, err := strconv.Atoi(payload.Label)
		if err != nil {
			t.Fatalf("parse sequence from label %q error = %v", payload.Label, err)
		}
		if seq <= lastSeq {
			t.Fatalf("committed progress records out of order: sequence %d committed after %d", seq, lastSeq)
		}
		lastSeq = seq
	}
}

// TestPublishRecord_SourceIdentityTupleMembersRemainDistinct proves that
// changing any single member of the (SourceType, SourceID, SourceSequence,
// SourceEventID) tuple, while holding the other three fixed, is treated as a
// wholly distinct record rather than a duplicate of the base identity.
func TestPublishRecord_SourceIdentityTupleMembersRemainDistinct(t *testing.T) {
	eventsSvc := newEventsAppender()
	base := workersessions.PublishRecordRequest{
		SessionID:      "worker-1",
		Draft:          toolDraft("tc-base"),
		SourceType:     "worker_provider",
		SourceID:       "src-1",
		SourceSequence: 1,
		SourceEventID:  "evt-1",
		SchemaID:       "workers.draft.v1",
	}
	// Published in an order that keeps each distinct (SourceType, SourceID)
	// key's own SourceSequence non-decreasing: withSourceEventID shares
	// base's key at the same SourceSequence (allowed; only SourceEventID
	// differs), and withSourceSequence -- the one variant that advances
	// SourceSequence on base's key -- is published last so it can never
	// look like a regression relative to an already-accepted higher
	// SourceSequence on that same key.
	variants := []workersessions.PublishRecordRequest{base}
	withSourceType := base
	withSourceType.SourceType = "worker_provider_alt"
	variants = append(variants, withSourceType)
	withSourceID := base
	withSourceID.SourceID = "src-2"
	variants = append(variants, withSourceID)
	withSourceEventID := base
	withSourceEventID.SourceEventID = "evt-2"
	variants = append(variants, withSourceEventID)
	withSourceSequence := base
	withSourceSequence.SourceSequence = 2
	variants = append(variants, withSourceSequence)

	seen := make(map[events.AggregateSequence]bool, len(variants))
	var svc workersessions.Service
	execution := &fakeExecution{
		dispatch: func(ctx context.Context, req workers.WorkstationDispatchRequest) (workers.WorkstationDispatchResult, error) {
			for i, r := range variants {
				result, err := svc.PublishRecord(ctx, r)
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
			return workers.WorkstationDispatchResult{
				DispatchID: req.Execution.Dispatch.DispatchID,
				Result: workers.WorkResult{
					DispatchID: req.Execution.Dispatch.DispatchID,
					Outcome:    workers.OutcomeAccepted,
				},
			}, nil
		},
	}
	var err error
	svc, err = newService(executionBoundary{execution: execution}, eventsSvc, nil)
	if err != nil {
		t.Fatalf("service.New() error = %v, want nil", err)
	}

	if _, err := svc.InvokeSession(context.Background(), validStartRequest("worker-1", "dispatch-1")); err != nil {
		t.Fatalf("Start() error = %v, want nil", err)
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

	var svc workersessions.Service
	execution := &fakeExecution{
		dispatch: func(ctx context.Context, req workers.WorkstationDispatchRequest) (workers.WorkstationDispatchResult, error) {
			result, err := svc.PublishRecord(ctx, identicalTuple(req.Execution.Dispatch.DispatchID))
			if err != nil {
				t.Fatalf("PublishRecord() error = %v, want nil", err)
			}
			if result.Outcome != workersessions.PublishOutcomeAccepted {
				t.Fatalf("PublishRecord() outcome = %v, want Accepted (a different Worker Session topic must not collapse an identical tuple into a duplicate)", result.Outcome)
			}
			return workers.WorkstationDispatchResult{
				DispatchID: req.Execution.Dispatch.DispatchID,
				Result: workers.WorkResult{
					DispatchID: req.Execution.Dispatch.DispatchID,
					Outcome:    workers.OutcomeAccepted,
				},
			}, nil
		},
	}
	var err error
	svc, err = newService(executionBoundary{execution: execution}, eventsSvc, nil)
	if err != nil {
		t.Fatalf("service.New() error = %v, want nil", err)
	}
	ctx := context.Background()

	if _, err := svc.InvokeSession(ctx, validStartRequest("worker-1", "worker-1")); err != nil {
		t.Fatalf("Start(worker-1) error = %v, want nil", err)
	}
	if _, err := svc.InvokeSession(ctx, validStartRequest("worker-2", "worker-2")); err != nil {
		t.Fatalf("Start(worker-2) error = %v, want nil", err)
	}

	for _, sessionID := range []string{"worker-1", "worker-2"} {
		committed := readAllDrafts(t, eventsSvc, workersessions.Topic(sessionID))
		if len(committed) != 3 {
			t.Fatalf("session %s committed record count = %d, want 3 (opening, published, terminal)", sessionID, len(committed))
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
	const published = 5

	var svc workersessions.Service
	execution := &fakeExecution{
		dispatch: func(ctx context.Context, req workers.WorkstationDispatchRequest) (workers.WorkstationDispatchResult, error) {
			for i := 1; i <= published; i++ {
				pubReq := validPublishRecordRequest("worker-1", events.SourceSequence(i), progressDraft("step"))
				if _, err := svc.PublishRecord(ctx, pubReq); err != nil {
					t.Fatalf("PublishRecord() [%d] error = %v, want nil", i, err)
				}
			}
			return workers.WorkstationDispatchResult{
				DispatchID: req.Execution.Dispatch.DispatchID,
				Result: workers.WorkResult{
					DispatchID: req.Execution.Dispatch.DispatchID,
					Outcome:    workers.OutcomeAccepted,
				},
			}, nil
		},
	}
	var err error
	svc, err = newService(executionBoundary{execution: execution}, eventsSvc, nil)
	if err != nil {
		t.Fatalf("service.New() error = %v, want nil", err)
	}
	ctx := context.Background()

	if _, err := svc.InvokeSession(ctx, validStartRequest("worker-1", "dispatch-1")); err != nil {
		t.Fatalf("Start() error = %v, want nil", err)
	}
	const wantTotal = published + 2 // + the opening record and the terminal record

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
// Every step runs from inside the dispatch callback, on the same goroutine
// Start blocks on, since publication is only accepted during that window.
func TestPublishRecord_SubscriptionFromLastReadCursorDeliversOnlyLaterRecords(t *testing.T) {
	eventsSvc := newEventsAppender()
	topic := workersessions.Topic("worker-1")

	var svc workersessions.Service
	execution := &fakeExecution{
		dispatch: func(ctx context.Context, req workers.WorkstationDispatchRequest) (workers.WorkstationDispatchResult, error) {
			if _, err := svc.PublishRecord(ctx, validPublishRecordRequest("worker-1", 1, toolDraft("tc-1"))); err != nil {
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

			if _, err := svc.PublishRecord(ctx, validPublishRecordRequest("worker-1", 2, toolDraft("tc-2"))); err != nil {
				t.Fatalf("PublishRecord() [2] error = %v, want nil", err)
			}
			if _, err := svc.PublishRecord(ctx, validPublishRecordRequest("worker-1", 3, toolDraft("tc-3"))); err != nil {
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

			return workers.WorkstationDispatchResult{
				DispatchID: req.Execution.Dispatch.DispatchID,
				Result: workers.WorkResult{
					DispatchID: req.Execution.Dispatch.DispatchID,
					Outcome:    workers.OutcomeAccepted,
				},
			}, nil
		},
	}
	var err error
	svc, err = newService(executionBoundary{execution: execution}, eventsSvc, nil)
	if err != nil {
		t.Fatalf("service.New() error = %v, want nil", err)
	}

	if _, err := svc.InvokeSession(context.Background(), validStartRequest("worker-1", "dispatch-1")); err != nil {
		t.Fatalf("Start() error = %v, want nil", err)
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

	var svc workersessions.Service
	var accepted int
	var positions map[events.AggregateSequence]bool
	execution := &fakeExecution{
		dispatch: func(ctx context.Context, req workers.WorkstationDispatchRequest) (workers.WorkstationDispatchResult, error) {
			pubReq := validPublishRecordRequest("worker-1", 1, toolDraft("tc-1"))

			var wg sync.WaitGroup
			results := make(chan workersessions.PublishRecordResult, goroutines)
			for range goroutines {
				wg.Go(func() {
					result, err := svc.PublishRecord(ctx, pubReq)
					if err != nil {
						t.Errorf("PublishRecord() error = %v, want nil", err)
						return
					}
					results <- result
				})
			}
			wg.Wait()
			close(results)

			positions = make(map[events.AggregateSequence]bool)
			for result := range results {
				if result.Outcome == workersessions.PublishOutcomeAccepted {
					accepted++
				}
				positions[result.AggregateSequence] = true
			}

			return workers.WorkstationDispatchResult{
				DispatchID: req.Execution.Dispatch.DispatchID,
				Result: workers.WorkResult{
					DispatchID: req.Execution.Dispatch.DispatchID,
					Outcome:    workers.OutcomeAccepted,
				},
			}, nil
		},
	}
	var err error
	svc, err = newService(executionBoundary{execution: execution}, eventsSvc, nil)
	if err != nil {
		t.Fatalf("service.New() error = %v, want nil", err)
	}

	if _, err := svc.InvokeSession(context.Background(), validStartRequest("worker-1", "dispatch-1")); err != nil {
		t.Fatalf("Start() error = %v, want nil", err)
	}

	if accepted != 1 {
		t.Fatalf("accepted count = %d, want exactly 1 across %d concurrent racers", accepted, goroutines)
	}
	if len(positions) != 1 {
		t.Fatalf("observed %d distinct aggregate sequences, want exactly 1: every racer must resolve to the same committed record", len(positions))
	}

	committed := readAllDrafts(t, eventsSvc, workersessions.Topic("worker-1"))
	if len(committed) != 3 {
		t.Fatalf("committed record count = %d, want exactly 3 (opening, one published record, terminal)", len(committed))
	}
}

func TestControlHistory_OrdersPauseResumeBracketsBeforeTerminalAndDeduplicatesReplay(t *testing.T) {
	eventsSvc := newEventsAppender()
	boundary := newControlledBoundary()
	registry, err := newService(boundary, eventsSvc, logging.NoopLogger{})
	if err != nil {
		t.Fatalf("service.New() error = %v", err)
	}
	started := startControlledSession(t, registry, boundary, "worker-1", "dispatch-1")
	reference := providers.SessionRef{
		Provider: providers.IDCodex,
		Kind:     providers.SessionIDKind,
		ID:       "provider-session-control-history",
	}
	if _, err := registry.AssociateProviderSession(context.Background(), workersessions.ProviderSessionAssociationRequest{
		WorkerSessionID: "worker-1",
		DispatchID:      "dispatch-1",
		Reference:       reference,
	}); err != nil {
		t.Fatalf("AssociateProviderSession() error = %v", err)
	}
	boundary.setCancel(func(_ context.Context, request workers.WorkstationDispatchCancelRequest) (workers.WorkstationDispatchCancelResult, error) {
		boundary.complete(canceledDispatchResult(request.DispatchID), workers.ErrWorkstationDispatchCanceled)
		return workers.WorkstationDispatchCancelResult{
			DispatchID: request.DispatchID,
			Outcome:    workers.WorkstationDispatchCancelOutcomeCanceled,
		}, nil
	})

	paused, err := registry.Pause(context.Background(), workersessions.ControlRequest{ID: "worker-1", RequestID: "pause-1"})
	if err != nil || paused.Outcome != workersessions.ControlOutcomeApplied || paused.Session.State != workersessions.StatePaused {
		t.Fatalf("Pause() = %#v, %v, want applied PAUSED", paused, err)
	}
	if repeated, err := registry.Pause(context.Background(), workersessions.ControlRequest{ID: "worker-1", RequestID: "pause-1"}); err != nil || repeated.Outcome != workersessions.ControlOutcomeNoop {
		t.Fatalf("replayed Pause() = %#v, %v, want NOOP without a second history bracket", repeated, err)
	}

	resumed, err := registry.Resume(context.Background(), workersessions.ControlRequest{ID: "worker-1", RequestID: "resume-1"})
	if err != nil || resumed.Outcome != workersessions.ControlOutcomeApplied || resumed.Session.State != workersessions.StateRunning {
		t.Fatalf("Resume() = %#v, %v, want applied RUNNING", resumed, err)
	}
	resumedResult := completedDispatchResult(resumed.DispatchID)
	resumedResult.Result.Continuation = continuationFromProviderMetadata(&providers.SessionMetadata{
		Provider: reference.Provider.String(),
		Kind:     reference.Kind,
		ID:       reference.ID,
	})
	boundary.complete(resumedResult, nil)
	if result := <-started; result.Session.State != workersessions.StateCompleted {
		t.Fatalf("InvokeSession() result = %#v, want COMPLETED", result)
	}
	if repeated, err := registry.Resume(context.Background(), workersessions.ControlRequest{ID: "worker-1", RequestID: "resume-1"}); err != nil || repeated.Outcome != workersessions.ControlOutcomeNoop {
		t.Fatalf("replayed terminal Resume() = %#v, %v, want NOOP without a second history bracket", repeated, err)
	}

	topic := workersessions.Topic("worker-1")
	read := readControlHistory(t, eventsSvc, topic, 20)
	if len(read.Records) != 7 {
		t.Fatalf("control history record count = %d, want opening + pause bracket + resume attempt + resume bracket + terminal", len(read.Records))
	}
	assertPauseResumeControlHistory(t, read, resumed.DispatchID)
	assertPortableControlHistory(t, "worker-1", topic, read)
}

func TestControlHistory_NaturalCompletionWinsAfterControlRequestAndBeforeTerminalRecord(t *testing.T) {
	eventsSvc := newEventsAppender()
	boundary := newControlledBoundary()
	registry, err := newService(boundary, eventsSvc, logging.NoopLogger{})
	if err != nil {
		t.Fatalf("service.New() error = %v", err)
	}
	started := startControlledSession(t, registry, boundary, "worker-natural", "dispatch-natural")
	if _, err := registry.AssociateProviderSession(context.Background(), workersessions.ProviderSessionAssociationRequest{
		WorkerSessionID: "worker-natural",
		DispatchID:      "dispatch-natural",
		Reference: providers.SessionRef{
			Provider: providers.IDCodex,
			Kind:     providers.SessionIDKind,
			ID:       "provider-session-natural-race",
		},
	}); err != nil {
		t.Fatalf("AssociateProviderSession() error = %v", err)
	}
	cancelEntered := make(chan struct{})
	releaseCancel := make(chan struct{})
	boundary.setCancel(func(_ context.Context, request workers.WorkstationDispatchCancelRequest) (workers.WorkstationDispatchCancelResult, error) {
		close(cancelEntered)
		<-releaseCancel
		return workers.WorkstationDispatchCancelResult{DispatchID: request.DispatchID}, nil
	})

	controlResult := make(chan struct {
		result workersessions.ControlResult
		err    error
	}, 1)
	go func() {
		result, err := registry.Pause(context.Background(), workersessions.ControlRequest{ID: "worker-natural", RequestID: "natural-race-control"})
		controlResult <- struct {
			result workersessions.ControlResult
			err    error
		}{result: result, err: err}
	}()
	<-cancelEntered

	boundary.complete(completedDispatchResult("dispatch-natural"), nil)
	close(releaseCancel)
	control := <-controlResult
	if control.err != nil || control.result.Outcome != workersessions.ControlOutcomeNoop || control.result.Session.State != workersessions.StateCompleted {
		t.Fatalf("racing Pause() = %#v, %v, want natural COMPLETED NOOP", control.result, control.err)
	}
	if result := <-started; result.Session.State != workersessions.StateCompleted {
		t.Fatalf("natural completion result = %#v, want COMPLETED", result)
	}

	topic := workersessions.Topic("worker-natural")
	assertNaturalControlHistory(t, eventsSvc, topic)
}

func TestControlHistory_RecordsInterruptBracketBeforeSourceTerminal(t *testing.T) {
	eventsSvc := newEventsAppender()
	boundary := newControlledBoundary()
	registry, err := newService(boundary, eventsSvc, logging.NoopLogger{})
	if err != nil {
		t.Fatalf("service.New() error = %v", err)
	}
	sourceResult := startControlledSession(t, registry, boundary, "source-interrupt", "dispatch-source-interrupt")
	reference := providers.SessionRef{
		Provider: providers.IDCodex,
		Kind:     providers.SessionIDKind,
		ID:       "provider-session-interrupt-history",
	}
	if _, err := registry.AssociateProviderSession(context.Background(), workersessions.ProviderSessionAssociationRequest{
		WorkerSessionID: "source-interrupt",
		DispatchID:      "dispatch-source-interrupt",
		Reference:       reference,
	}); err != nil {
		t.Fatalf("AssociateProviderSession() error = %v", err)
	}
	boundary.setCancel(func(_ context.Context, request workers.WorkstationDispatchCancelRequest) (workers.WorkstationDispatchCancelResult, error) {
		boundary.complete(canceledDispatchResult(request.DispatchID), workers.ErrWorkstationDispatchCanceled)
		return workers.WorkstationDispatchCancelResult{
			DispatchID: request.DispatchID,
			Outcome:    workers.WorkstationDispatchCancelOutcomeCanceled,
		}, nil
	})

	interrupted, err := registry.Interrupt(context.Background(), workersessions.InterruptRequest{
		RequestID:                "interrupt-history-1",
		SourceWorkerSessionID:    "source-interrupt",
		SuccessorWorkerSessionID: "successor-interrupt",
		ReplacementMessage:       "replacement",
	})
	if err != nil || !interrupted.Accepted {
		t.Fatalf("Interrupt() = %#v, %v, want accepted", interrupted, err)
	}
	if result := <-sourceResult; result.Session.State != workersessions.StateCanceled {
		t.Fatalf("source InvokeSession() = %#v, want CANCELED", result.Session)
	}

	topic := workersessions.Topic("source-interrupt")
	assertInterruptControlHistory(t, eventsSvc, topic)

	boundary.complete(completedDispatchResult(interrupted.Successor.ProviderSessionAssociation.DispatchID), nil)
}
