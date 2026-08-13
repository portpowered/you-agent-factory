package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	"github.com/portpowered/infinite-you/pkg/services/events"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
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
	svc, err = newService(executionBoundary{execution: execution}, eventsSvc, nil)
	if err != nil {
		t.Fatalf("service.New() error = %v, want nil", err)
	}

	if _, err := svc.InvokeSession(context.Background(), validStartRequest("worker-1", "dispatch-1")); err != nil {
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

// TestPublishRecord_RejectsProviderContradictingOpening proves both provider
// publication entry points preserve the opening identity: a direct binding
// attempt and a provider-authored output naming provider B are rejected after
// the opening already established provider A, and neither creates history.
func TestPublishRecord_RejectsProviderContradictingOpening(t *testing.T) {
	eventsSvc := newEventsAppender()
	var svc workersessions.Service
	var bindingErr, publishErr error
	execution := &fakeExecution{
		dispatch: func(ctx context.Context, req workers.WorkstationDispatchRequest) (workers.WorkstationDispatchResult, error) {
			_, bindingErr = svc.EnsureProviderBinding(ctx, workersessions.ProviderBindingRequest{
				DispatchID: req.Execution.Dispatch.DispatchID,
				Provider:   "claude",
			})
			draft := messageDraft("provider B must not enter provider A history")
			draft.DispatchID = req.Execution.Dispatch.DispatchID
			draft.Provenance.Provider = "claude"
			_, publishErr = svc.PublishRecord(ctx, validPublishRecordRequest("worker-1", 1, draft))
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
	request := validStartRequest("worker-1", "dispatch-1")
	request.Execution.Execution.RunnerID = workers.RunnerIDCodex
	if _, err := svc.InvokeSession(context.Background(), request); err != nil {
		t.Fatalf("InvokeSession() error = %v, want nil", err)
	}
	if !errors.Is(bindingErr, workersessions.ErrProviderBindingConflict) {
		t.Fatalf("EnsureProviderBinding() error = %v, want ErrProviderBindingConflict", bindingErr)
	}
	if !errors.Is(publishErr, workersessions.ErrProviderBindingConflict) {
		t.Fatalf("PublishRecord() error = %v, want ErrProviderBindingConflict", publishErr)
	}

	committed := readAllDrafts(t, eventsSvc, workersessions.Topic("worker-1"))
	if len(committed) != 2 {
		t.Fatalf("committed record count = %d, want opening plus terminal only", len(committed))
	}
	if committed[0].Provenance.Provider != workers.RunnerIDCodex || committed[1].Provenance.Provider != workers.RunnerIDCodex {
		t.Fatalf("lifecycle providers = %q/%q, want codex/codex", committed[0].Provenance.Provider, committed[1].Provenance.Provider)
	}
}

// TestPublishRecord_SyntheticAgentRunProvenanceDoesNotBindProvider proves the
// Worker-authored final draft marker is not treated as a second provider
// identity. The opening may already be attributed to the selected provider,
// while the canonical Worker response still carries its internal agent-run
// provenance.
func TestPublishRecord_SyntheticAgentRunProvenanceDoesNotBindProvider(t *testing.T) {
	eventsSvc := newEventsAppender()
	var svc workersessions.Service
	var publishResult workersessions.PublishRecordResult
	var publishErr error
	execution := &fakeExecution{
		dispatch: func(ctx context.Context, req workers.WorkstationDispatchRequest) (workers.WorkstationDispatchResult, error) {
			draft := messageDraft("synthetic Worker provenance")
			draft.DispatchID = req.Execution.Dispatch.DispatchID
			draft.Provenance.Provider = "agent-run"
			publishResult, publishErr = svc.PublishRecord(ctx, validPublishRecordRequest("worker-1", 1, draft))
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
	request := validStartRequest("worker-1", "dispatch-1")
	request.Execution.Execution.RunnerID = workers.RunnerIDCodex
	if _, err := svc.InvokeSession(context.Background(), request); err != nil {
		t.Fatalf("InvokeSession() error = %v, want nil", err)
	}
	if publishErr != nil {
		t.Fatalf("PublishRecord() error = %v, want nil", publishErr)
	}
	if publishResult.Outcome != workersessions.PublishOutcomeAccepted {
		t.Fatalf("PublishRecord() outcome = %v, want Accepted", publishResult.Outcome)
	}

	committed := readAllDrafts(t, eventsSvc, workersessions.Topic("worker-1"))
	if len(committed) != 3 {
		t.Fatalf("committed record count = %d, want opening, Worker output, and terminal", len(committed))
	}
	if committed[1].Provenance.Provider != "agent-run" {
		t.Fatalf("Worker output provider = %q, want agent-run", committed[1].Provenance.Provider)
	}
}

// TestPublishRecord_InvalidDraft_ReturnsErrorAndCommitsNoRecord proves that a
// draft violating the existing Workers Kind/Phase/payload rules is rejected
// explicitly and appends nothing, before any session or publication-window
// lookup: this is validated by req.Validate() alone.
func TestPublishRecord_InvalidDraft_ReturnsErrorAndCommitsNoRecord(t *testing.T) {
	eventsSvc := newEventsAppender()
	registry, err := newService(executionBoundary{execution: succeedingExecution()}, eventsSvc, nil)
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
	registry, err := newService(executionBoundary{execution: succeedingExecution()}, eventsSvc, nil)
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
	svc, err = newService(executionBoundary{execution: execution}, appender, nil)
	if err != nil {
		t.Fatalf("service.New() error = %v, want nil", err)
	}

	if _, err := svc.InvokeSession(context.Background(), validStartRequest("worker-1", "dispatch-1")); err != nil {
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
	svc, err = newService(executionBoundary{execution: execution}, eventsSvc, nil)
	if err != nil {
		t.Fatalf("service.New() error = %v, want nil", err)
	}

	if _, err := svc.InvokeSession(context.Background(), validStartRequest("worker-1", "dispatch-1")); err != nil {
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
	svc, err = newService(executionBoundary{execution: execution}, eventsSvc, nil)
	if err != nil {
		t.Fatalf("service.New() error = %v, want nil", err)
	}

	if _, err := svc.InvokeSession(context.Background(), validStartRequest("worker-1", "dispatch-1")); err != nil {
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
	svc, err = newService(executionBoundary{execution: execution}, eventsSvc, nil)
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
			if _, err := svc.InvokeSession(ctx, validStartRequest(sessionID, sessionID)); err != nil {
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
	registry, err := newService(executionBoundary{execution: succeedingExecution()}, eventsSvc, nil)
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
	registry, err := newService(executionBoundary{execution: succeedingExecution()}, eventsSvc, nil)
	if err != nil {
		t.Fatalf("service.New() error = %v, want nil", err)
	}
	ctx := context.Background()

	if _, err := registry.InvokeSession(ctx, validStartRequest("worker-1", "dispatch-1")); err != nil {
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
	svc, err = newService(executionBoundary{execution: execution}, eventsSvc, nil)
	if err != nil {
		t.Fatalf("service.New() error = %v, want nil", err)
	}

	if _, err := svc.InvokeSession(context.Background(), validStartRequest("worker-1", "dispatch-1")); err != nil {
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
	svc, err = newService(executionBoundary{execution: execution}, eventsSvc, nil)
	if err != nil {
		t.Fatalf("service.New() error = %v, want nil", err)
	}

	if _, err := svc.InvokeSession(context.Background(), validStartRequest("worker-1", "dispatch-1")); err != nil {
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

func TestContinue_PersistsExactAttemptAndPortableContinuationLineage(t *testing.T) {
	boundary := newControlledBoundary()
	eventsSvc := newEventsAppender()
	registry, err := newService(boundary, eventsSvc, logging.NoopLogger{})
	if err != nil {
		t.Fatalf("service.New() error = %v", err)
	}
	reference := providers.SessionRef{Provider: providers.ID(" codex "), Kind: " session_id ", ID: " opaque-provider-session "}
	sourceResult := startControlledSession(t, registry, boundary, "source-exact", "dispatch-exact")
	boundary.complete(completedDispatchWithProviderSession("dispatch-exact", reference), nil)
	if result := <-sourceResult; result.Session.State != workersessions.StateCompleted {
		t.Fatalf("source session = %#v, want COMPLETED", result.Session)
	}
	request := workersessions.ContinueRequest{
		RequestID:                "continue-exact",
		SourceWorkerSessionID:    "source-exact",
		SuccessorWorkerSessionID: "successor-exact",
		FollowUpInput:            "preserve the opaque reference",
	}
	continued, err := registry.Continue(context.Background(), request)
	if err != nil {
		t.Fatalf("Continue() error = %v", err)
	}
	if continued.Session.PredecessorWorkerSessionID != request.SourceWorkerSessionID {
		t.Fatalf("successor session = %#v, want predecessor lineage", continued.Session)
	}
	sourceRecords := readWorkerSessionRecords(t, eventsSvc, request.SourceWorkerSessionID)
	assertSourceContinuationLineage(t, sourceRecords, request, reference)
	successorRecords := readWorkerSessionRecords(t, eventsSvc, request.SuccessorWorkerSessionID)
	assertSuccessorContinuationOpening(t, successorRecords, request, reference)
	successorHandoff := boundary.currentRequest()
	boundary.complete(completedDispatchWithProviderSession(successorHandoff.Execution.Dispatch.DispatchID, reference), nil)
	successorRecords = readWorkerSessionRecords(t, eventsSvc, request.SuccessorWorkerSessionID)
	if len(successorRecords) != 2 {
		t.Fatalf("successor records = %#v, want opening and terminal", successorRecords)
	}
	assertPortableContinuationLineage(t, request, sourceRecords, successorRecords)
}

func readWorkerSessionRecords(t *testing.T, eventsSvc events.Service, id string) []events.Record {
	t.Helper()
	topic := workersessions.Topic(id)
	read, err := eventsSvc.Read(context.Background(), events.ReadRequest{Topic: topic, From: events.Cursor{Topic: topic}, Limit: 10})
	if err != nil {
		t.Fatalf("Read(%q) error = %v", id, err)
	}
	return read.Records
}

func assertSourceContinuationLineage(t *testing.T, records []events.Record, request workersessions.ContinueRequest, reference providers.SessionRef) {
	t.Helper()
	if len(records) != 3 || records[2].SourceType != events.SourceType("worker_session_lineage") {
		t.Fatalf("source records = %#v, want opening/terminal/successor lineage", records)
	}
	payload := decodeLineageSessionPayload(t, records[2])
	if payload.Lineage == nil || payload.Lineage.SuccessorWorkerSessionID != request.SuccessorWorkerSessionID ||
		payload.Continuation == nil || payload.Continuation.Provider != string(reference.Provider) ||
		payload.Continuation.Kind != reference.Kind || payload.Continuation.ID != reference.ID {
		t.Fatalf("source lineage payload = %#v, want exact successor and Provider Session reference", payload)
	}
}

func assertSuccessorContinuationOpening(t *testing.T, records []events.Record, request workersessions.ContinueRequest, reference providers.SessionRef) {
	t.Helper()
	if len(records) == 0 {
		t.Fatal("successor records are empty")
	}
	payload := decodeLineageSessionPayload(t, records[0])
	if payload.AttemptReason != workers.AttemptReasonResume || payload.Lineage == nil ||
		payload.Lineage.PredecessorWorkerSessionID != request.SourceWorkerSessionID ||
		payload.Lineage.PreviousDispatchID != "dispatch-exact" || payload.Lineage.PreviousAttemptID != "dispatch-exact" ||
		payload.Continuation == nil || payload.Continuation.Provider != string(reference.Provider) ||
		payload.Continuation.Kind != reference.Kind || payload.Continuation.ID != reference.ID {
		t.Fatalf("successor opening payload = %#v, want exact RESUME lineage", payload)
	}
}

func assertPortableContinuationLineage(t *testing.T, request workersessions.ContinueRequest, sourceRecords, successorRecords []events.Record) {
	t.Helper()
	codec := recordings.WorkerRecordingCodec{}
	for _, fixture := range []struct {
		id      string
		records []events.Record
	}{
		{id: request.SourceWorkerSessionID, records: sourceRecords},
		{id: request.SuccessorWorkerSessionID, records: successorRecords},
	} {
		portable, err := codec.BuildWorkerPortableRecording(recordings.WorkerRecordingSnapshot{
			RecordingID: "recording-exact-lineage",
			Sessions: []recordings.WorkerSessionRecordingSnapshot{{
				WorkerSessionID: fixture.id,
				Topic:           workersessions.Topic(fixture.id),
				Status:          recordings.WorkerRecordingStatusComplete,
				LastPosition:    fixture.records[len(fixture.records)-1].ID.Position,
				Records:         fixture.records,
			}},
		})
		if err != nil {
			t.Fatalf("BuildWorkerPortableRecording(%q) error = %v", fixture.id, err)
		}
		replayed, err := codec.ReplayWorkerPortableRecording(portable)
		if err != nil || len(replayed.Projection.Records) != len(fixture.records) {
			t.Fatalf("portable replay(%q) = %#v, %v, want chronological lineage", fixture.id, replayed, err)
		}
		if fixture.id == request.SuccessorWorkerSessionID && (portable.Correlation.Lineage == nil || portable.Correlation.Lineage.PredecessorWorkerSessionID != request.SourceWorkerSessionID) {
			t.Fatalf("portable successor correlation = %#v, want predecessor", portable.Correlation)
		}
	}
}
