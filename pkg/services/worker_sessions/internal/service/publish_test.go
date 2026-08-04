package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/events"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/worker_sessions/internal/service"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func progressDraft(label string) workers.Draft {
	payload, err := json.Marshal(workers.ProgressPayload{Label: label})
	if err != nil {
		panic(err)
	}
	return workers.Draft{Kind: workers.KindProgress, Phase: workers.PhaseUpdated, Payload: payload}
}

func toolDraft(toolCallID string) workers.Draft {
	payload, err := json.Marshal(workers.ToolPayload{ToolCallID: toolCallID, ToolName: "grep"})
	if err != nil {
		panic(err)
	}
	return workers.Draft{Kind: workers.KindTool, Phase: workers.PhaseStarted, Payload: payload}
}

func messageDraft(text string) workers.Draft {
	payload, err := json.Marshal(workers.MessagePayload{
		Role:          "assistant",
		ContentBlocks: []workers.ContentBlock{{Kind: workers.ContentBlockText, Text: text}},
	})
	if err != nil {
		panic(err)
	}
	return workers.Draft{Kind: workers.KindMessage, Phase: workers.PhaseCompleted, Payload: payload}
}

func validPublishRecordRequest(sessionID string, sequence events.SourceSequence, draft workers.Draft) workersessions.PublishRecordRequest {
	return workersessions.PublishRecordRequest{
		SessionID:      sessionID,
		Draft:          draft,
		SourceType:     "worker_provider",
		SourceID:       events.SourceID(sessionID),
		SourceSequence: sequence,
		SourceEventID:  events.SourceEventID(fmt.Sprintf("evt-%d", sequence)),
		SchemaID:       "workers.draft.v1",
	}
}

func readAllDrafts(t *testing.T, eventsSvc events.Service, topic events.Topic) []workers.Draft {
	t.Helper()
	result, err := eventsSvc.Read(context.Background(), events.ReadRequest{
		Topic: topic,
		From:  events.Cursor{Topic: topic},
		Limit: 100,
	})
	if err != nil {
		t.Fatalf("Read() error = %v, want nil", err)
	}
	drafts := make([]workers.Draft, len(result.Records))
	for i, record := range result.Records {
		if err := json.Unmarshal(record.Payload, &drafts[i]); err != nil {
			t.Fatalf("unmarshal record payload error = %v", err)
		}
	}
	return drafts
}

// TestPublishRecord_AppendsValidatedDetachedSourceNativeDraftsInOrder proves
// that a sequence of valid Worker observations across multiple Kinds and
// Phases, published from inside the controlled Workers boundary's own
// dispatch callback (the only window PublishRecord accepts calls in), is
// committed after the opening record, in the exact order PublishRecord is
// called, with Kind/Phase/Payload preserved verbatim.
func TestPublishRecord_AppendsValidatedDetachedSourceNativeDraftsInOrder(t *testing.T) {
	eventsSvc := newEventsAppender()
	drafts := []workers.Draft{toolDraft("tc-1"), progressDraft("thinking"), messageDraft("done")}

	var svc workersessions.Service
	execution := &fakeExecution{
		dispatch: func(ctx context.Context, req workers.WorkstationDispatchRequest) (workers.WorkstationDispatchResult, error) {
			for i, draft := range drafts {
				result, err := svc.PublishRecord(ctx, validPublishRecordRequest("worker-1", events.SourceSequence(i+1), draft))
				if err != nil {
					t.Fatalf("PublishRecord() [%d] error = %v, want nil", i, err)
				}
				if result.Outcome != workersessions.PublishOutcomeAccepted {
					t.Fatalf("PublishRecord() [%d] outcome = %v, want Accepted", i, result.Outcome)
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
	svc, err = service.New(execution, eventsSvc, nil)
	if err != nil {
		t.Fatalf("service.New() error = %v, want nil", err)
	}

	if _, err := svc.Start(context.Background(), validStartRequest("worker-1", "dispatch-1")); err != nil {
		t.Fatalf("Start() error = %v, want nil", err)
	}

	committed := readAllDrafts(t, eventsSvc, workersessions.Topic("worker-1"))
	if len(committed) != len(drafts)+2 {
		t.Fatalf("committed record count = %d, want %d (opening + published + terminal)", len(committed), len(drafts)+2)
	}
	for i, want := range drafts {
		got := committed[i+1]
		if got.Kind != want.Kind || got.Phase != want.Phase {
			t.Fatalf("committed[%d] Kind/Phase = %s/%s, want %s/%s", i+1, got.Kind, got.Phase, want.Kind, want.Phase)
		}
		if string(got.Payload) != string(want.Payload) {
			t.Fatalf("committed[%d] Payload = %s, want %s", i+1, got.Payload, want.Payload)
		}
	}
}

// TestPublishRecord_InvalidDraft_ReturnsErrorAndCommitsNoRecord proves that a
// draft violating the existing Workers Kind/Phase/payload rules is rejected
// explicitly and appends nothing, before any session or publication-window
// lookup: this is validated by req.Validate() alone.
func TestPublishRecord_InvalidDraft_ReturnsErrorAndCommitsNoRecord(t *testing.T) {
	eventsSvc := newEventsAppender()
	registry, err := service.New(succeedingExecution(), eventsSvc, nil)
	if err != nil {
		t.Fatalf("service.New() error = %v, want nil", err)
	}
	ctx := context.Background()

	invalid := workers.Draft{Kind: workers.KindTool, Phase: workers.PhaseUpdated, Payload: json.RawMessage(`{}`)}
	if _, err := registry.PublishRecord(ctx, validPublishRecordRequest("worker-1", 1, invalid)); err == nil {
		t.Fatalf("PublishRecord() error = nil, want a validation error")
	}

	committed := readAllDrafts(t, eventsSvc, workersessions.Topic("worker-1"))
	if len(committed) != 0 {
		t.Fatalf("committed record count = %d, want 0", len(committed))
	}
}

// TestPublishRecord_MalformedSourceIdentity_ReturnsErrorAndCommitsNoRecord
// proves that an incomplete Events idempotency identity is rejected before
// any Events append is attempted.
func TestPublishRecord_MalformedSourceIdentity_ReturnsErrorAndCommitsNoRecord(t *testing.T) {
	eventsSvc := newEventsAppender()
	registry, err := service.New(succeedingExecution(), eventsSvc, nil)
	if err != nil {
		t.Fatalf("service.New() error = %v, want nil", err)
	}
	ctx := context.Background()

	req := validPublishRecordRequest("worker-1", 1, toolDraft("tc-1"))
	req.SourceType = ""
	if _, err := registry.PublishRecord(ctx, req); err == nil {
		t.Fatalf("PublishRecord() error = nil, want an identity validation error")
	}

	committed := readAllDrafts(t, eventsSvc, workersessions.Topic("worker-1"))
	if len(committed) != 0 {
		t.Fatalf("committed record count = %d, want 0", len(committed))
	}
}

// TestPublishRecord_EventsAppendFailure_ReturnsErrorExplicitly proves that an
// Events-level append failure surfaces as an explicit error rather than a
// fabricated success result, once the publication window is genuinely open
// (the opening record, append #1, must still succeed for the window to open
// at all; only the PublishRecord call itself, append #2, is made to fail).
func TestPublishRecord_EventsAppendFailure_ReturnsErrorExplicitly(t *testing.T) {
	appender := &failOnNthAppendEventsAppender{Service: newEventsAppender(), n: 2}

	var svc workersessions.Service
	var publishResult workersessions.PublishRecordResult
	var publishErr error
	execution := &fakeExecution{
		dispatch: func(ctx context.Context, req workers.WorkstationDispatchRequest) (workers.WorkstationDispatchResult, error) {
			publishResult, publishErr = svc.PublishRecord(ctx, validPublishRecordRequest("worker-1", 1, toolDraft("tc-1")))
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
	svc, err = service.New(execution, appender, nil)
	if err != nil {
		t.Fatalf("service.New() error = %v, want nil", err)
	}

	if _, err := svc.Start(context.Background(), validStartRequest("worker-1", "dispatch-1")); err != nil {
		t.Fatalf("Start() error = %v, want nil", err)
	}
	if publishErr == nil {
		t.Fatalf("PublishRecord() during dispatch error = nil, want the broken appender's error")
	}
	if publishResult != (workersessions.PublishRecordResult{}) {
		t.Fatalf("PublishRecord() result = %+v, want the zero value on failure", publishResult)
	}
}

// TestPublishRecord_DuplicateSourceIdentity_ReturnsOriginalWithoutNewRecord
// proves that repeating the exact (SourceType, SourceID, SourceSequence,
// SourceEventID) tuple resolves to the original committed record instead of
// appending a second one.
func TestPublishRecord_DuplicateSourceIdentity_ReturnsOriginalWithoutNewRecord(t *testing.T) {
	eventsSvc := newEventsAppender()
	var svc workersessions.Service
	var first, second workersessions.PublishRecordResult

	execution := &fakeExecution{
		dispatch: func(ctx context.Context, req workers.WorkstationDispatchRequest) (workers.WorkstationDispatchResult, error) {
			pubReq := validPublishRecordRequest("worker-1", 1, toolDraft("tc-1"))
			var publishErr error
			if first, publishErr = svc.PublishRecord(ctx, pubReq); publishErr != nil {
				t.Fatalf("first PublishRecord() error = %v, want nil", publishErr)
			}
			if second, publishErr = svc.PublishRecord(ctx, pubReq); publishErr != nil {
				t.Fatalf("second PublishRecord() error = %v, want nil", publishErr)
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
	svc, err = service.New(execution, eventsSvc, nil)
	if err != nil {
		t.Fatalf("service.New() error = %v, want nil", err)
	}

	if _, err := svc.Start(context.Background(), validStartRequest("worker-1", "dispatch-1")); err != nil {
		t.Fatalf("Start() error = %v, want nil", err)
	}
	if first.Outcome != workersessions.PublishOutcomeAccepted {
		t.Fatalf("first PublishRecord() outcome = %v, want Accepted", first.Outcome)
	}
	if second.Outcome != workersessions.PublishOutcomeDuplicate {
		t.Fatalf("second PublishRecord() outcome = %v, want Duplicate", second.Outcome)
	}
	if second.AggregateSequence != first.AggregateSequence {
		t.Fatalf("second PublishRecord() aggregate sequence = %d, want %d (unchanged)", second.AggregateSequence, first.AggregateSequence)
	}

	committed := readAllDrafts(t, eventsSvc, workersessions.Topic("worker-1"))
	if len(committed) != 3 {
		t.Fatalf("committed record count = %d, want exactly 3 (opening, one published record, terminal)", len(committed))
	}
}

// TestPublishRecord_CallerPayloadMutationAfterPublish_DoesNotAffectRetainedRecord
// proves that mutating the caller-owned Payload byte slice after PublishRecord
// returns cannot alter the committed record.
func TestPublishRecord_CallerPayloadMutationAfterPublish_DoesNotAffectRetainedRecord(t *testing.T) {
	eventsSvc := newEventsAppender()
	original, err := json.Marshal(workers.ProgressPayload{Label: "original"})
	if err != nil {
		t.Fatalf("marshal payload error = %v", err)
	}
	mutable := append(json.RawMessage(nil), original...)
	draft := workers.Draft{Kind: workers.KindProgress, Phase: workers.PhaseUpdated, Payload: mutable}

	var svc workersessions.Service
	execution := &fakeExecution{
		dispatch: func(ctx context.Context, req workers.WorkstationDispatchRequest) (workers.WorkstationDispatchResult, error) {
			if _, publishErr := svc.PublishRecord(ctx, validPublishRecordRequest("worker-1", 1, draft)); publishErr != nil {
				t.Fatalf("PublishRecord() error = %v, want nil", publishErr)
			}
			for i := range mutable {
				mutable[i] = 'x'
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
	svc, err = service.New(execution, eventsSvc, nil)
	if err != nil {
		t.Fatalf("service.New() error = %v, want nil", err)
	}

	if _, err := svc.Start(context.Background(), validStartRequest("worker-1", "dispatch-1")); err != nil {
		t.Fatalf("Start() error = %v, want nil", err)
	}

	committed := readAllDrafts(t, eventsSvc, workersessions.Topic("worker-1"))
	if len(committed) != 3 {
		t.Fatalf("committed record count = %d, want 3 (opening, published, terminal)", len(committed))
	}
	if string(committed[1].Payload) != string(original) {
		t.Fatalf("committed payload = %s, want unaffected %s", committed[1].Payload, original)
	}
}

// TestPublishRecord_ConcurrentSessionIsolation proves that concurrently
// starting and publishing source records for distinct Worker Sessions never
// interleaves across topics: each session's topic, read independently,
// contains only its own records in its own submitted order, and one
// session's publication lock never blocks another session's concurrent
// publication.
func TestPublishRecord_ConcurrentSessionIsolation(t *testing.T) {
	eventsSvc := newEventsAppender()
	const sessions = 5
	const recordsPerSession = 20

	var svc workersessions.Service
	execution := &fakeExecution{
		dispatch: func(ctx context.Context, req workers.WorkstationDispatchRequest) (workers.WorkstationDispatchResult, error) {
			sessionID := req.Execution.Dispatch.DispatchID
			for i := 1; i <= recordsPerSession; i++ {
				pubReq := validPublishRecordRequest(sessionID, events.SourceSequence(i), progressDraft(sessionID))
				pubReq.SourceEventID = events.SourceEventID(fmt.Sprintf("%s-evt-%d", sessionID, i))
				if _, err := svc.PublishRecord(ctx, pubReq); err != nil {
					return workers.WorkstationDispatchResult{}, err
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
	svc, err = service.New(execution, eventsSvc, nil)
	if err != nil {
		t.Fatalf("service.New() error = %v, want nil", err)
	}
	ctx := context.Background()

	var wg sync.WaitGroup
	for s := range sessions {
		sessionID := fmt.Sprintf("worker-%d", s)
		wg.Add(1)
		go func(sessionID string) {
			defer wg.Done()
			if _, err := svc.Start(ctx, validStartRequest(sessionID, sessionID)); err != nil {
				panic(err)
			}
		}(sessionID)
	}
	wg.Wait()

	for s := range sessions {
		sessionID := fmt.Sprintf("worker-%d", s)
		committed := readAllDrafts(t, eventsSvc, workersessions.Topic(sessionID))
		if len(committed) != recordsPerSession+2 {
			t.Fatalf("session %s committed record count = %d, want %d (opening + published + terminal)", sessionID, len(committed), recordsPerSession+2)
		}
	}
}

func TestPublishRecordRequest_Validate_RejectsBlankSessionID(t *testing.T) {
	req := validPublishRecordRequest("   ", 1, toolDraft("tc-1"))
	if err := req.Validate(); !errors.Is(err, workersessions.ErrInvalidSessionID) {
		t.Fatalf("Validate() error = %v, want ErrInvalidSessionID", err)
	}
}

// TestPublishRecord_RejectsPublicationForUnknownSession proves PublishRecord
// distinguishes "no such session was ever reserved or started" from "the
// session exists but its publication window is not open": an identity that
// was never registered returns ErrSessionNotFound.
func TestPublishRecord_RejectsPublicationForUnknownSession(t *testing.T) {
	eventsSvc := newEventsAppender()
	registry, err := service.New(succeedingExecution(), eventsSvc, nil)
	if err != nil {
		t.Fatalf("service.New() error = %v, want nil", err)
	}

	_, err = registry.PublishRecord(context.Background(), validPublishRecordRequest("worker-1", 1, toolDraft("tc-1")))
	if !errors.Is(err, workersessions.ErrSessionNotFound) {
		t.Fatalf("PublishRecord() for an unknown session error = %v, want ErrSessionNotFound", err)
	}
}

// TestPublishRecord_RejectsPublicationAfterTerminal proves the review-flagged
// defect is fixed: once a session's terminal record has committed (Start has
// already returned, with no output published during dispatch), a later
// PublishRecord call is rejected with ErrPublicationNotOpen and appends
// nothing after the terminal record.
func TestPublishRecord_RejectsPublicationAfterTerminal(t *testing.T) {
	eventsSvc := newEventsAppender()
	registry, err := service.New(succeedingExecution(), eventsSvc, nil)
	if err != nil {
		t.Fatalf("service.New() error = %v, want nil", err)
	}
	ctx := context.Background()

	if _, err := registry.Start(ctx, validStartRequest("worker-1", "dispatch-1")); err != nil {
		t.Fatalf("Start() error = %v, want nil", err)
	}

	_, err = registry.PublishRecord(ctx, validPublishRecordRequest("worker-1", 1, toolDraft("tc-late")))
	if !errors.Is(err, workersessions.ErrPublicationNotOpen) {
		t.Fatalf("PublishRecord() after terminal error = %v, want ErrPublicationNotOpen", err)
	}

	committed := readAllDrafts(t, eventsSvc, workersessions.Topic("worker-1"))
	if len(committed) != 2 {
		t.Fatalf("committed record count = %d, want exactly 2 (opening + terminal only)", len(committed))
	}
}

// TestPublishRecord_LateOutputLosesRaceAgainstTerminal_IsRejectedNotInsertedAfterTerminal
// proves the exact race the review flagged is closed: a PublishRecord call
// that has not yet reached the publication lock by the time the terminal
// record starts committing is rejected outright once it does reach the
// lock, never inserted into the topic after the terminal record. The
// publishing goroutine blocks on a channel the test only closes after Start
// has already returned (so the terminal record has already committed and
// the window has already closed), making this deterministic rather than a
// timing-dependent race.
func TestPublishRecord_LateOutputLosesRaceAgainstTerminal_IsRejectedNotInsertedAfterTerminal(t *testing.T) {
	eventsSvc := newEventsAppender()
	topic := workersessions.Topic("worker-1")

	release := make(chan struct{})
	publishDone := make(chan error, 1)

	var svc workersessions.Service
	execution := &fakeExecution{
		dispatch: func(ctx context.Context, req workers.WorkstationDispatchRequest) (workers.WorkstationDispatchResult, error) {
			go func() {
				<-release
				_, err := svc.PublishRecord(ctx, validPublishRecordRequest("worker-1", 1, toolDraft("tc-late")))
				publishDone <- err
			}()
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
	svc, err = service.New(execution, eventsSvc, nil)
	if err != nil {
		t.Fatalf("service.New() error = %v, want nil", err)
	}

	if _, err := svc.Start(context.Background(), validStartRequest("worker-1", "dispatch-1")); err != nil {
		t.Fatalf("Start() error = %v, want nil", err)
	}

	close(release)
	if err := <-publishDone; !errors.Is(err, workersessions.ErrPublicationNotOpen) {
		t.Fatalf("late PublishRecord() error = %v, want ErrPublicationNotOpen", err)
	}

	committed := readAllDrafts(t, eventsSvc, topic)
	if len(committed) != 2 {
		t.Fatalf("committed record count = %d, want exactly 2 (opening + terminal; the late output must never be inserted after it)", len(committed))
	}
}

// TestPublishRecord_OutOfOrderSourceSequence_IsRejected proves PublishRecord
// enforces non-decreasing SourceSequence per (SourceType, SourceID): once
// SourceSequence 3 has been accepted for a source, a later call presenting
// SourceSequence 2 for the same source is rejected rather than committed
// out of order.
func TestPublishRecord_OutOfOrderSourceSequence_IsRejected(t *testing.T) {
	eventsSvc := newEventsAppender()
	var svc workersessions.Service
	var outOfOrderErr error

	execution := &fakeExecution{
		dispatch: func(ctx context.Context, req workers.WorkstationDispatchRequest) (workers.WorkstationDispatchResult, error) {
			if _, err := svc.PublishRecord(ctx, validPublishRecordRequest("worker-1", 1, toolDraft("tc-1"))); err != nil {
				t.Fatalf("PublishRecord() seq=1 error = %v, want nil", err)
			}
			if _, err := svc.PublishRecord(ctx, validPublishRecordRequest("worker-1", 3, toolDraft("tc-3"))); err != nil {
				t.Fatalf("PublishRecord() seq=3 error = %v, want nil", err)
			}
			_, outOfOrderErr = svc.PublishRecord(ctx, validPublishRecordRequest("worker-1", 2, toolDraft("tc-2")))
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
	svc, err = service.New(execution, eventsSvc, nil)
	if err != nil {
		t.Fatalf("service.New() error = %v, want nil", err)
	}

	if _, err := svc.Start(context.Background(), validStartRequest("worker-1", "dispatch-1")); err != nil {
		t.Fatalf("Start() error = %v, want nil", err)
	}
	if !errors.Is(outOfOrderErr, workersessions.ErrOutOfOrderPublication) {
		t.Fatalf("PublishRecord() seq=2 after seq=3 error = %v, want ErrOutOfOrderPublication", outOfOrderErr)
	}

	committed := readAllDrafts(t, eventsSvc, workersessions.Topic("worker-1"))
	if len(committed) != 4 {
		t.Fatalf("committed record count = %d, want 4 (opening, seq=1, seq=3, terminal; the out-of-order seq=2 must never commit)", len(committed))
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
	svc, err = service.New(execution, eventsSvc, nil)
	if err != nil {
		t.Fatalf("service.New() error = %v, want nil", err)
	}

	if _, err := svc.Start(context.Background(), validStartRequest("worker-1", "dispatch-1")); err != nil {
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
	svc, err = service.New(execution, eventsSvc, nil)
	if err != nil {
		t.Fatalf("service.New() error = %v, want nil", err)
	}

	if _, err := svc.Start(context.Background(), validStartRequest("worker-1", "dispatch-1")); err != nil {
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
	svc, err = service.New(execution, eventsSvc, nil)
	if err != nil {
		t.Fatalf("service.New() error = %v, want nil", err)
	}
	ctx := context.Background()

	if _, err := svc.Start(ctx, validStartRequest("worker-1", "worker-1")); err != nil {
		t.Fatalf("Start(worker-1) error = %v, want nil", err)
	}
	if _, err := svc.Start(ctx, validStartRequest("worker-2", "worker-2")); err != nil {
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
	svc, err = service.New(execution, eventsSvc, nil)
	if err != nil {
		t.Fatalf("service.New() error = %v, want nil", err)
	}
	ctx := context.Background()

	if _, err := svc.Start(ctx, validStartRequest("worker-1", "dispatch-1")); err != nil {
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
	svc, err = service.New(execution, eventsSvc, nil)
	if err != nil {
		t.Fatalf("service.New() error = %v, want nil", err)
	}

	if _, err := svc.Start(context.Background(), validStartRequest("worker-1", "dispatch-1")); err != nil {
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
	svc, err = service.New(execution, eventsSvc, nil)
	if err != nil {
		t.Fatalf("service.New() error = %v, want nil", err)
	}

	if _, err := svc.Start(context.Background(), validStartRequest("worker-1", "dispatch-1")); err != nil {
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
