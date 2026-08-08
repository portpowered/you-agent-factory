package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/events"
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/worker_sessions/internal/service"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// decodeDraft decodes a committed record's payload as a workers.Draft for
// Kind/Phase/payload assertions.
func decodeDraft(t *testing.T, record events.Record) workers.Draft {
	t.Helper()
	var draft workers.Draft
	if err := json.Unmarshal(record.Payload, &draft); err != nil {
		t.Fatalf("unmarshal record payload as workers.Draft error = %v", err)
	}
	return draft
}

// TestStart_CommitsOpeningRecordBeforeWorkersInvocation proves the W3
// before-handoff barrier: reading worker-session/<id>/events from its zero
// cursor, from inside the controlled Workers boundary's own dispatch
// callback (i.e. as early as Workers could possibly act, including emitting
// output immediately upon invocation), already observes the committed
// KindSession/PhaseStarted opening record.
func TestStart_CommitsOpeningRecordBeforeWorkersInvocation(t *testing.T) {
	eventsSvc := newEventsAppender()
	topic := workersessions.Topic("worker-1")

	var observed events.ReadResult
	var readErr error
	execution := &fakeExecution{
		dispatch: func(ctx context.Context, req workers.WorkstationDispatchRequest) (workers.WorkstationDispatchResult, error) {
			observed, readErr = eventsSvc.Read(ctx, events.ReadRequest{
				Topic: topic,
				From:  events.Cursor{Topic: topic},
				Limit: 10,
			})
			return workers.WorkstationDispatchResult{
				DispatchID: req.Execution.Dispatch.DispatchID,
				Result: workers.WorkResult{
					DispatchID: req.Execution.Dispatch.DispatchID,
					Outcome:    workers.OutcomeAccepted,
				},
			}, nil
		},
	}

	registry, err := service.New(executionBoundary{execution: execution}, eventsSvc, nil)
	if err != nil {
		t.Fatalf("service.New() error = %v, want nil", err)
	}
	if _, err := registry.InvokeSession(context.Background(), validStartRequest("worker-1", "dispatch-1")); err != nil {
		t.Fatalf("Start() error = %v, want nil", err)
	}
	if readErr != nil {
		t.Fatalf("Read() during dispatch error = %v, want nil", readErr)
	}
	if observed.Outcome != events.ReadOutcomeProgress || len(observed.Records) != 1 {
		t.Fatalf("Read() during dispatch = %+v, want exactly one already-committed record", observed)
	}

	var draft workers.Draft
	if err := json.Unmarshal(observed.Records[0].Payload, &draft); err != nil {
		t.Fatalf("unmarshal opening record payload error = %v", err)
	}
	if draft.Kind != workers.KindSession || draft.Phase != workers.PhaseStarted {
		t.Fatalf("opening draft = %+v, want Kind=SESSION Phase=STARTED", draft)
	}
}

// TestStart_SubscriptionFromZeroCursor_ObservesOpeningRecord proves the same
// before-handoff barrier from a subscriber's perspective: a subscription
// opened before Start is ever called still delivers the opening record,
// since it was committed before Workers' immediate output could race it.
func TestStart_SubscriptionFromZeroCursor_ObservesOpeningRecord(t *testing.T) {
	eventsSvc := newEventsAppender()
	topic := workersessions.Topic("worker-1")
	ctx := context.Background()

	sub, err := eventsSvc.Subscribe(ctx, events.SubscribeRequest{
		Topic: topic,
		From:  events.Cursor{Topic: topic},
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("Subscribe() error = %v, want nil", err)
	}

	registry, err := service.New(executionBoundary{execution: succeedingExecution()}, eventsSvc, nil)
	if err != nil {
		t.Fatalf("service.New() error = %v, want nil", err)
	}
	if _, err := registry.InvokeSession(ctx, validStartRequest("worker-1", "dispatch-1")); err != nil {
		t.Fatalf("Start() error = %v, want nil", err)
	}

	delivery := sub.Next(ctx)
	if delivery.Kind != events.DeliveryRecord {
		t.Fatalf("Subscription.Next() kind = %v, want DeliveryRecord", delivery.Kind)
	}
	var draft workers.Draft
	if err := json.Unmarshal(delivery.Record.Payload, &draft); err != nil {
		t.Fatalf("unmarshal opening record payload error = %v", err)
	}
	if draft.Kind != workers.KindSession || draft.Phase != workers.PhaseStarted {
		t.Fatalf("opening draft = %+v, want Kind=SESSION Phase=STARTED", draft)
	}
}

// TestStart_OpeningRecordPublicationFailure_TerminalizesFailedWithoutCallingWorkers
// proves that a failure establishing the opening record is an explicit
// publication failure: Workers is never invoked, and the session
// terminalizes FAILED with the typed EVENT_PUBLICATION_FAILURE cause rather
// than leaving the session stuck in STARTING or fabricating a successful
// handoff.
func TestStart_OpeningRecordPublicationFailure_TerminalizesFailedWithoutCallingWorkers(t *testing.T) {
	execution := succeedingExecution()
	registry, err := service.New(executionBoundary{execution: execution}, &brokenEventsAppender{}, nil)
	if err != nil {
		t.Fatalf("service.New() error = %v, want nil", err)
	}

	result, err := registry.InvokeSession(context.Background(), validStartRequest("worker-1", "dispatch-1"))
	if err != nil {
		t.Fatalf("Start() error = %v, want nil", err)
	}
	if result.Session.State != workersessions.StateFailed {
		t.Fatalf("Start() state = %q, want FAILED", result.Session.State)
	}
	if result.Session.Result == nil || result.Session.Result.Cause == nil {
		t.Fatalf("Start() result = %+v, want a non-nil Cause", result.Session.Result)
	}
	if got := result.Session.Result.Cause.Kind; got != workersessions.FailureCauseEventPublicationFailure {
		t.Fatalf("Start() cause kind = %q, want EVENT_PUBLICATION_FAILURE", got)
	}
	if got := execution.callCount(); got != 0 {
		t.Fatalf("Start() called Workers %d times, want 0 when opening record publication fails", got)
	}
}

// TestStart_InvalidRequest_CreatesNoTopicRecord extends the existing
// pre-effect rejection coverage: an invalid Start request must not publish
// any Events record, not just skip the Workers call.
func TestStart_InvalidRequest_CreatesNoTopicRecord(t *testing.T) {
	appender := &countingEventsAppender{Service: newEventsAppender()}
	registry, err := service.New(executionBoundary{execution: succeedingExecution()}, appender, nil)
	if err != nil {
		t.Fatalf("service.New() error = %v, want nil", err)
	}

	req := validStartRequest("worker-1", "dispatch-1")
	req.ID = "   "
	if _, err := registry.InvokeSession(context.Background(), req); !errors.Is(err, workersessions.ErrInvalidSessionID) {
		t.Fatalf("Start() error = %v, want ErrInvalidSessionID", err)
	}
	if got := appender.callCount(); got != 0 {
		t.Fatalf("Start() with invalid request published %d Events records, want 0", got)
	}
}

// TestStart_NotStartableSession_CreatesNoTopicRecord proves a conflicting
// Start on an already-terminal session publishes no additional Events
// record: rejection happens before the opening record is ever attempted.
func TestStart_NotStartableSession_CreatesNoTopicRecord(t *testing.T) {
	appender := &countingEventsAppender{Service: newEventsAppender()}
	registry, err := service.New(executionBoundary{execution: succeedingExecution()}, appender, nil)
	if err != nil {
		t.Fatalf("service.New() error = %v, want nil", err)
	}
	ctx := context.Background()

	if _, err := registry.InvokeSession(ctx, validStartRequest("worker-1", "dispatch-1")); err != nil {
		t.Fatalf("first Start() error = %v, want nil", err)
	}
	callsAfterFirst := appender.callCount()
	if callsAfterFirst == 0 {
		t.Fatalf("first Start() published %d Events records, want at least 1", callsAfterFirst)
	}

	if _, err := registry.InvokeSession(ctx, validStartRequest("worker-1", "dispatch-2")); !errors.Is(err, workersessions.ErrSessionNotStartable) {
		t.Fatalf("second Start() error = %v, want ErrSessionNotStartable", err)
	}
	if got := appender.callCount(); got != callsAfterFirst {
		t.Fatalf("conflicting Start() published %d Events records, want unchanged %d", got, callsAfterFirst)
	}
}

// TestStart_CompletedSession_AppendsTerminalRecordAfterOpeningRecord proves
// story 004's core contract for the ordinary success path: reading the topic
// after a COMPLETED Start returns exactly the opening record followed by one
// terminal KindSession/PhaseCompleted record, in contiguous commit order.
func TestStart_CompletedSession_AppendsTerminalRecordAfterOpeningRecord(t *testing.T) {
	eventsSvc := newEventsAppender()
	topic := workersessions.Topic("worker-1")
	registry, err := service.New(executionBoundary{execution: succeedingExecution()}, eventsSvc, nil)
	if err != nil {
		t.Fatalf("service.New() error = %v, want nil", err)
	}
	ctx := context.Background()

	result, err := registry.InvokeSession(ctx, validStartRequest("worker-1", "dispatch-1"))
	if err != nil {
		t.Fatalf("Start() error = %v, want nil", err)
	}
	if result.Session.State != workersessions.StateCompleted {
		t.Fatalf("Start() state = %q, want COMPLETED", result.Session.State)
	}

	read, err := eventsSvc.Read(ctx, events.ReadRequest{Topic: topic, From: events.Cursor{Topic: topic}, Limit: 10})
	if err != nil {
		t.Fatalf("Read() error = %v, want nil", err)
	}
	if len(read.Records) != 2 {
		t.Fatalf("Read() returned %d records, want 2 (opening + terminal)", len(read.Records))
	}

	opening := decodeDraft(t, read.Records[0])
	if opening.Kind != workers.KindSession || opening.Phase != workers.PhaseStarted {
		t.Fatalf("first record = %+v, want Kind=SESSION Phase=STARTED", opening)
	}

	terminal := decodeDraft(t, read.Records[1])
	if terminal.Kind != workers.KindSession || terminal.Phase != workers.PhaseCompleted {
		t.Fatalf("second record = %+v, want Kind=SESSION Phase=COMPLETED", terminal)
	}
	if read.Records[1].ID.Position <= read.Records[0].ID.Position {
		t.Fatalf("terminal record position %d did not follow opening record position %d", read.Records[1].ID.Position, read.Records[0].ID.Position)
	}
}

// TestStart_FailedSession_AppendsTerminalRecordWithClassifiedFailureCause
// proves a FAILED terminal projection preserves the typed FailureCause
// classification worker_sessions itself already computed (classify.go), not
// a generic or re-derived value.
func TestStart_FailedSession_AppendsTerminalRecordWithClassifiedFailureCause(t *testing.T) {
	eventsSvc := newEventsAppender()
	topic := workersessions.Topic("worker-1")
	execution := &fakeExecution{
		dispatch: func(_ context.Context, req workers.WorkstationDispatchRequest) (workers.WorkstationDispatchResult, error) {
			return workers.WorkstationDispatchResult{
				DispatchID: req.Execution.Dispatch.DispatchID,
				Result: workers.WorkResult{
					DispatchID: req.Execution.Dispatch.DispatchID,
					Outcome:    workers.OutcomeFailed,
					Error:      "executor panic: boom",
				},
			}, nil
		},
	}
	registry, err := service.New(executionBoundary{execution: execution}, eventsSvc, nil)
	if err != nil {
		t.Fatalf("service.New() error = %v, want nil", err)
	}
	ctx := context.Background()

	result, err := registry.InvokeSession(ctx, validStartRequest("worker-1", "dispatch-1"))
	if err != nil {
		t.Fatalf("Start() error = %v, want nil", err)
	}
	if result.Session.State != workersessions.StateFailed {
		t.Fatalf("Start() state = %q, want FAILED", result.Session.State)
	}
	if got := result.Session.Result.Cause.Kind; got != workersessions.FailureCauseExecutorPanic {
		t.Fatalf("Start() cause kind = %q, want EXECUTOR_PANIC", got)
	}

	read, err := eventsSvc.Read(ctx, events.ReadRequest{Topic: topic, From: events.Cursor{Topic: topic}, Limit: 10})
	if err != nil {
		t.Fatalf("Read() error = %v, want nil", err)
	}
	if len(read.Records) != 2 {
		t.Fatalf("Read() returned %d records, want 2 (opening + terminal)", len(read.Records))
	}

	terminal := decodeDraft(t, read.Records[1])
	if terminal.Kind != workers.KindSession || terminal.Phase != workers.PhaseFailed {
		t.Fatalf("terminal record = %+v, want Kind=SESSION Phase=FAILED", terminal)
	}

	var payload struct {
		Status        string `json:"status"`
		FailureCause  string `json:"failureCause"`
		FailureDetail string `json:"failureDetail"`
	}
	if err := json.Unmarshal(terminal.Payload, &payload); err != nil {
		t.Fatalf("unmarshal terminal payload error = %v", err)
	}
	if payload.Status != string(workersessions.StateFailed) {
		t.Fatalf("terminal payload status = %q, want FAILED", payload.Status)
	}
	if payload.FailureCause != string(workersessions.FailureCauseExecutorPanic) {
		t.Fatalf("terminal payload failureCause = %q, want %q", payload.FailureCause, workersessions.FailureCauseExecutorPanic)
	}
	if payload.FailureDetail != result.Session.Result.Cause.Detail {
		t.Fatalf("terminal payload failureDetail = %q, want %q", payload.FailureDetail, result.Session.Result.Cause.Detail)
	}
}

func TestStart_ProviderSessionInspectionFailureReachesTerminalEventWithSafeCause(t *testing.T) {
	eventsSvc := newEventsAppender()
	topic := workersessions.Topic("worker-inspection-failure")
	execution := &fakeExecution{
		dispatch: func(_ context.Context, req workers.WorkstationDispatchRequest) (workers.WorkstationDispatchResult, error) {
			return workers.WorkstationDispatchResult{
				DispatchID: req.Execution.Dispatch.DispatchID,
				Result: workers.WorkResult{
					DispatchID: req.Execution.Dispatch.DispatchID,
					Outcome:    workers.OutcomeFailed,
					FailureMetadata: &workers.WorkFailureMetadata{
						Family: workers.WorkFailureFamilyTerminal,
						Type:   workers.WorkFailureTypeUnknown,
					},
					Diagnostics: &workers.WorkDiagnostics{
						Provider: &workers.ProviderDiagnostic{
							ResponseMetadata: map[string]string{
								workers.ProviderResponseMetadataFailureOperation:      "provider_session_ingestion",
								workers.ProviderResponseMetadataFailureClassification: "resource_limit",
							},
						},
					},
				},
			}, nil
		},
	}
	registry, err := service.New(executionBoundary{execution: execution}, eventsSvc, nil)
	if err != nil {
		t.Fatalf("service.New() error = %v, want nil", err)
	}
	result, err := registry.Start(context.Background(), validStartRequest("worker-inspection-failure", "dispatch-inspection-failure"))
	if err != nil {
		t.Fatalf("Start() error = %v, want nil", err)
	}
	if result.Session.State != workersessions.StateFailed || result.Session.Result == nil || result.Session.Result.Cause == nil {
		t.Fatalf("session = %#v, want FAILED with a cause", result.Session)
	}
	detail := result.Session.Result.Cause.Detail
	if !strings.Contains(detail, "provider_session_ingestion") || !strings.Contains(detail, "resource_limit") {
		t.Fatalf("session failure detail = %q, want safe inspection classification", detail)
	}
	if strings.TrimSpace(detail) == "" || len([]rune(detail)) > workersessions.MaxFailureCauseDetailRunes {
		t.Fatalf("session failure detail = %q, want trimmed bounded detail", detail)
	}

	read, err := eventsSvc.Read(context.Background(), events.ReadRequest{Topic: topic, From: events.Cursor{Topic: topic}, Limit: 10})
	if err != nil {
		t.Fatalf("Read() error = %v, want nil", err)
	}
	if len(read.Records) != 2 {
		t.Fatalf("Read() returned %d records, want opening plus terminal", len(read.Records))
	}
	terminal := decodeDraft(t, read.Records[1])
	if terminal.DispatchID != "dispatch-inspection-failure" || terminal.Phase != workers.PhaseFailed {
		t.Fatalf("terminal draft = %#v, want dispatch-correlated FAILED draft", terminal)
	}
	var payload struct {
		FailureCause  string `json:"failureCause"`
		FailureDetail string `json:"failureDetail"`
		Status        string `json:"status"`
	}
	if err := json.Unmarshal(terminal.Payload, &payload); err != nil {
		t.Fatalf("unmarshal terminal payload error = %v", err)
	}
	if payload.Status != string(workersessions.StateFailed) ||
		payload.FailureCause != string(workersessions.FailureCauseWorkersExecutionFailure) ||
		payload.FailureDetail != detail {
		t.Fatalf("terminal payload = %#v, want correlated safe failure cause", payload)
	}
}

func TestStart_ZeroExitTaskCompleteArtifactWithIngestionFailureIsNotPhantomSuccess(t *testing.T) {
	eventsSvc := newEventsAppender()
	topic := workersessions.Topic("worker-contradictory-ingestion")
	inspectionErr := &providersessions.LookupError{
		Provider:  providersessions.ProviderCodex,
		SessionID: "rollout-contradictory-ingestion",
		Err:       errors.Join(providersessions.ErrResourceLimitExceeded, errors.New("raw rollout must not escape")),
	}
	execution := &fakeExecution{
		dispatch: func(_ context.Context, req workers.WorkstationDispatchRequest) (workers.WorkstationDispatchResult, error) {
			return workers.WorkstationDispatchResult{
				DispatchID:      req.Execution.Dispatch.DispatchID,
				TerminalOutcome: workers.WorkstationDispatchTerminalOutcomeCompleted,
				Result: workers.WorkResult{
					DispatchID: req.Execution.Dispatch.DispatchID,
					Outcome:    workers.OutcomeAccepted,
					Output:     "artifact-created",
					ProviderSession: &workers.ProviderSessionMetadata{
						Provider: string(providersessions.ProviderCodex),
						Kind:     providersessions.SessionIDKind,
						ID:       "rollout-contradictory-ingestion",
					},
					Diagnostics: &workers.WorkDiagnostics{
						Command: &workers.CommandDiagnostic{ExitCode: 0},
						Provider: &workers.ProviderDiagnostic{
							ResponseMetadata: map[string]string{
								workers.ProviderResponseMetadataCompletionEvidence:    "task_complete",
								workers.ProviderResponseMetadataFailureOperation:      "provider_session_ingestion",
								workers.ProviderResponseMetadataFailureClassification: "resource_limit",
								workers.ProviderResponseMetadataFailureStage:          "final_parse",
							},
						},
					},
				},
			}, inspectionErr
		},
	}
	registry, err := service.New(executionBoundary{execution: execution}, eventsSvc, nil)
	if err != nil {
		t.Fatalf("service.New() error = %v, want nil", err)
	}
	result, err := registry.Start(context.Background(), validStartRequest("worker-contradictory-ingestion", "dispatch-contradictory-ingestion"))
	if err != nil {
		t.Fatalf("Start() error = %v, want nil", err)
	}
	if result.Session.State != workersessions.StateFailed || result.Session.Result == nil || result.Session.Result.Cause == nil {
		t.Fatalf("session = %#v, want FAILED with a cause", result.Session)
	}
	if result.Session.Result.Cause.Kind != workersessions.FailureCauseAdapterFailure {
		t.Fatalf("session failure kind = %q, want ADAPTER_FAILURE", result.Session.Result.Cause.Kind)
	}
	detail := result.Session.Result.Cause.Detail
	for _, want := range []string{"family=terminal", "type=unknown", "provider_session_ingestion", "resource_limit", "final_parse"} {
		if !strings.Contains(detail, want) {
			t.Fatalf("session failure detail = %q, want %q", detail, want)
		}
	}
	if strings.Contains(detail, "raw rollout") || strings.TrimSpace(detail) == "" || len([]rune(detail)) > workersessions.MaxFailureCauseDetailRunes {
		t.Fatalf("session failure detail = %q, want bounded safe cause", detail)
	}

	read, err := eventsSvc.Read(context.Background(), events.ReadRequest{Topic: topic, From: events.Cursor{Topic: topic}, Limit: 10})
	if err != nil {
		t.Fatalf("Read() error = %v, want nil", err)
	}
	if len(read.Records) != 2 {
		t.Fatalf("Read() returned %d records, want opening plus terminal", len(read.Records))
	}
	terminal := decodeDraft(t, read.Records[1])
	if terminal.Phase != workers.PhaseFailed || terminal.DispatchID != "dispatch-contradictory-ingestion" {
		t.Fatalf("terminal draft = %#v, want correlated FAILED draft", terminal)
	}
	var payload struct {
		FailureDetail string `json:"failureDetail"`
		Status        string `json:"status"`
	}
	if err := json.Unmarshal(terminal.Payload, &payload); err != nil {
		t.Fatalf("unmarshal terminal payload error = %v", err)
	}
	if payload.Status != string(workersessions.StateFailed) || payload.FailureDetail != detail {
		t.Fatalf("terminal payload = %#v, want non-empty safe failure detail", payload)
	}
}

// TestStart_TerminalRecordFollowsPublishedWorkerOutput proves ordering when a
// caller publishes a source-native Worker record from inside the controlled
// Workers boundary's own dispatch callback (simulating a real streaming
// producer): the topic ends up with opening, published, then terminal, in
// that exact contiguous order -- the terminal record always follows the last
// accepted output rather than racing ahead of it.
func TestStart_TerminalRecordFollowsPublishedWorkerOutput(t *testing.T) {
	eventsSvc := newEventsAppender()
	topic := workersessions.Topic("worker-1")

	var svc workersessions.Service
	execution := &fakeExecution{
		dispatch: func(ctx context.Context, req workers.WorkstationDispatchRequest) (workers.WorkstationDispatchResult, error) {
			childDraftPayload, marshalErr := json.Marshal(workers.MessagePayload{Role: "assistant", ContentBlocks: []workers.ContentBlock{{Kind: workers.ContentBlockText, Text: "hi"}}})
			if marshalErr != nil {
				t.Fatalf("marshal child payload error = %v", marshalErr)
			}
			if _, publishErr := svc.PublishRecord(ctx, workersessions.PublishRecordRequest{
				SessionID:      "worker-1",
				Draft:          workers.Draft{Kind: workers.KindMessage, Phase: workers.PhaseCompleted, Payload: childDraftPayload},
				SourceType:     "provider",
				SourceID:       "worker-1",
				SourceSequence: 1,
				SourceEventID:  "message-1",
				SchemaID:       "workers.draft.v1",
			}); publishErr != nil {
				t.Fatalf("PublishRecord() during dispatch error = %v, want nil", publishErr)
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
	svc, err = service.New(executionBoundary{execution: execution}, eventsSvc, nil)
	if err != nil {
		t.Fatalf("service.New() error = %v, want nil", err)
	}
	ctx := context.Background()

	if _, err := svc.InvokeSession(ctx, validStartRequest("worker-1", "dispatch-1")); err != nil {
		t.Fatalf("Start() error = %v, want nil", err)
	}

	read, err := eventsSvc.Read(ctx, events.ReadRequest{Topic: topic, From: events.Cursor{Topic: topic}, Limit: 10})
	if err != nil {
		t.Fatalf("Read() error = %v, want nil", err)
	}
	if len(read.Records) != 3 {
		t.Fatalf("Read() returned %d records, want 3 (opening, published, terminal)", len(read.Records))
	}

	opening := decodeDraft(t, read.Records[0])
	published := decodeDraft(t, read.Records[1])
	terminal := decodeDraft(t, read.Records[2])
	if opening.Phase != workers.PhaseStarted {
		t.Fatalf("record[0] phase = %q, want STARTED", opening.Phase)
	}
	if published.Kind != workers.KindMessage {
		t.Fatalf("record[1] kind = %q, want MESSAGE", published.Kind)
	}
	if terminal.Kind != workers.KindSession || terminal.Phase != workers.PhaseCompleted {
		t.Fatalf("record[2] = %+v, want Kind=SESSION Phase=COMPLETED", terminal)
	}
}

// TestStart_TerminalRecordPublicationFailure_DoesNotChangeCommittedSession
// proves the AC5 requirement directly: a failure appending the terminal
// record (isolated to exactly that append, via failOnNthAppendEventsAppender)
// is logged and never rewrites, hides, or fabricates the already-committed
// canonical W2 terminal Session Start returns.
func TestStart_TerminalRecordPublicationFailure_DoesNotChangeCommittedSession(t *testing.T) {
	appender := &failOnNthAppendEventsAppender{Service: newEventsAppender(), n: 2}
	registry, err := service.New(executionBoundary{execution: succeedingExecution()}, appender, nil)
	if err != nil {
		t.Fatalf("service.New() error = %v, want nil", err)
	}
	ctx := context.Background()

	result, err := registry.InvokeSession(ctx, validStartRequest("worker-1", "dispatch-1"))
	if err != nil {
		t.Fatalf("Start() error = %v, want nil", err)
	}
	if result.Session.State != workersessions.StateCompleted {
		t.Fatalf("Start() state = %q, want COMPLETED even though the terminal record append failed", result.Session.State)
	}
	if result.Session.Result == nil || result.Session.Result.Outcome != workersessions.TerminalOutcomeCompleted {
		t.Fatalf("Start() result = %+v, want a committed COMPLETED TerminalResult", result.Session.Result)
	}

	got, err := registry.Get(ctx, workersessions.GetRequest{ID: "worker-1"})
	if err != nil {
		t.Fatalf("Get() error = %v, want nil", err)
	}
	if got.State != workersessions.StateCompleted {
		t.Fatalf("Get() after terminal publish failure state = %q, want COMPLETED unchanged", got.State)
	}

	topic := workersessions.Topic("worker-1")
	read, err := appender.Read(ctx, events.ReadRequest{Topic: topic, From: events.Cursor{Topic: topic}, Limit: 10})
	if err != nil {
		t.Fatalf("Read() error = %v, want nil", err)
	}
	if len(read.Records) != 1 {
		t.Fatalf("Read() returned %d records, want exactly 1 (opening only; the failed terminal append committed nothing)", len(read.Records))
	}
	if decodeDraft(t, read.Records[0]).Phase != workers.PhaseStarted {
		t.Fatalf("record[0] phase = %q, want STARTED", decodeDraft(t, read.Records[0]).Phase)
	}
}

// TestStart_RepeatedStartOnTerminalSession_PublishesNoSecondTerminalRecord
// proves the exactly-once guarantee end-to-end: a conflicting Start on an
// already-terminal session is rejected before commitTerminal is ever reached
// a second time, so no second terminal record is ever appended.
func TestStart_RepeatedStartOnTerminalSession_PublishesNoSecondTerminalRecord(t *testing.T) {
	appender := &countingEventsAppender{Service: newEventsAppender()}
	registry, err := service.New(executionBoundary{execution: succeedingExecution()}, appender, nil)
	if err != nil {
		t.Fatalf("service.New() error = %v, want nil", err)
	}
	ctx := context.Background()

	if _, err := registry.InvokeSession(ctx, validStartRequest("worker-1", "dispatch-1")); err != nil {
		t.Fatalf("first Start() error = %v, want nil", err)
	}
	callsAfterFirst := appender.callCount()
	if callsAfterFirst != 2 {
		t.Fatalf("first Start() published %d Events records, want 2 (opening + terminal)", callsAfterFirst)
	}

	if _, err := registry.InvokeSession(ctx, validStartRequest("worker-1", "dispatch-2")); err == nil {
		t.Fatalf("second Start() on a terminal session error = nil, want ErrSessionNotStartable")
	}
	if got := appender.callCount(); got != callsAfterFirst {
		t.Fatalf("conflicting Start() published %d Events records, want unchanged %d (no second terminal record)", got, callsAfterFirst)
	}
}

type recordedLogEntry struct {
	message string
	fields  map[string]any
}

// recordingLogger implements logging.Logger for asserting the exact safe
// fields the registry emits without depending on a concrete logging backend.
type recordingLogger struct {
	mu      sync.Mutex
	entries []recordedLogEntry
}

func (l *recordingLogger) Debug(message string, keysAndValues ...any) {
	l.record(message, keysAndValues)
}

func (l *recordingLogger) Info(message string, keysAndValues ...any) {
	l.record(message, keysAndValues)
}

func (l *recordingLogger) Warn(message string, keysAndValues ...any) {
	l.record(message, keysAndValues)
}

func (l *recordingLogger) Error(message string, keysAndValues ...any) {
	l.record(message, keysAndValues)
}

func (l *recordingLogger) Verbose(message string, keysAndValues ...any) {
	l.record(message, keysAndValues)
}

func (l *recordingLogger) record(message string, keysAndValues []any) {
	fields := make(map[string]any, len(keysAndValues)/2)
	for index := 0; index+1 < len(keysAndValues); index += 2 {
		key, ok := keysAndValues[index].(string)
		if !ok {
			continue
		}
		fields[key] = keysAndValues[index+1]
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = append(l.entries, recordedLogEntry{message: message, fields: fields})
}

func (l *recordingLogger) entriesFor(message string) []recordedLogEntry {
	l.mu.Lock()
	defer l.mu.Unlock()
	var matches []recordedLogEntry
	for _, entry := range l.entries {
		if entry.message == message {
			matches = append(matches, entry)
		}
	}
	return matches
}

func assertNoPayloadOrCredentialKeys(t *testing.T, fields map[string]any) {
	t.Helper()
	for key := range fields {
		switch key {
		case "sessionID", "attemptID", "outcome", "state", "cause", "filter_state_count", "result_count":
			continue
		default:
			t.Fatalf("unexpected log field %q leaked into operation log: %#v", key, fields)
		}
	}
}

func newLoggingRegistry(t *testing.T, logger *recordingLogger) workersessions.Service {
	t.Helper()
	registry, err := service.New(executionBoundary{execution: succeedingExecution()}, newEventsAppender(), logger)
	if err != nil {
		t.Fatalf("service.New() error = %v, want nil", err)
	}
	return registry
}

func TestRegistryLogsReserveOutcomes(t *testing.T) {
	logger := &recordingLogger{}
	registry := newLoggingRegistry(t, logger)
	ctx := context.Background()

	if _, err := registry.Reserve(ctx, workersessions.ReserveRequest{ID: "   "}); err == nil {
		t.Fatalf("Reserve() with invalid ID unexpectedly succeeded")
	}
	rejected := logger.entriesFor("worker session reserve rejected")
	if len(rejected) != 1 || rejected[0].fields["outcome"] != "invalid" {
		t.Fatalf("reserve-rejected log = %#v", rejected)
	}
	assertNoPayloadOrCredentialKeys(t, rejected[0].fields)

	if _, err := registry.Reserve(ctx, workersessions.ReserveRequest{ID: "worker-1"}); err != nil {
		t.Fatalf("Reserve() error = %v, want nil", err)
	}
	accepted := logger.entriesFor("worker session reserve")
	if len(accepted) != 1 || accepted[0].fields["sessionID"] != "worker-1" || accepted[0].fields["outcome"] != "reserved" {
		t.Fatalf("reserve-accepted log = %#v", accepted)
	}
	assertNoPayloadOrCredentialKeys(t, accepted[0].fields)

	if _, err := registry.Reserve(ctx, workersessions.ReserveRequest{ID: "worker-1"}); err == nil {
		t.Fatalf("duplicate Reserve() unexpectedly succeeded")
	}
	accepted = logger.entriesFor("worker session reserve")
	if len(accepted) != 2 || accepted[1].fields["sessionID"] != "worker-1" || accepted[1].fields["outcome"] != "duplicate" {
		t.Fatalf("reserve-duplicate log = %#v", accepted)
	}
	assertNoPayloadOrCredentialKeys(t, accepted[1].fields)
}

func TestRegistryLogsListOutcomes(t *testing.T) {
	logger := &recordingLogger{}
	registry := newLoggingRegistry(t, logger)
	ctx := context.Background()

	if _, err := registry.List(ctx, workersessions.ListRequest{Filter: workersessions.Filter{States: []workersessions.State{"INTERRUPTED"}}}); err == nil {
		t.Fatalf("List() with invalid filter unexpectedly succeeded")
	}
	rejected := logger.entriesFor("worker session list rejected")
	if len(rejected) != 1 || rejected[0].fields["outcome"] != "invalid" {
		t.Fatalf("list-rejected log = %#v", rejected)
	}
	assertNoPayloadOrCredentialKeys(t, rejected[0].fields)

	if _, err := registry.Reserve(ctx, workersessions.ReserveRequest{ID: "worker-1"}); err != nil {
		t.Fatalf("Reserve() error = %v, want nil", err)
	}
	if _, err := registry.List(ctx, workersessions.ListRequest{}); err != nil {
		t.Fatalf("List() error = %v, want nil", err)
	}
	succeeded := logger.entriesFor("worker session list")
	if len(succeeded) != 1 || succeeded[0].fields["outcome"] != "success" || succeeded[0].fields["result_count"] != 1 {
		t.Fatalf("list-success log = %#v", succeeded)
	}
	assertNoPayloadOrCredentialKeys(t, succeeded[0].fields)
}

func TestRegistryLogsStartOutcomes(t *testing.T) {
	logger := &recordingLogger{}
	registry, err := service.New(executionBoundary{execution: &fakeExecution{
		dispatch: func(_ context.Context, req workers.WorkstationDispatchRequest) (workers.WorkstationDispatchResult, error) {
			return workers.WorkstationDispatchResult{
				DispatchID: req.Execution.Dispatch.DispatchID,
				Result: workers.WorkResult{
					DispatchID: req.Execution.Dispatch.DispatchID,
					Outcome:    workers.OutcomeFailed,
					Error:      "executor panic: boom",
				},
			}, nil
		},
	}}, newEventsAppender(), logger)
	if err != nil {
		t.Fatalf("service.New() error = %v, want nil", err)
	}
	ctx := context.Background()

	if _, err := registry.InvokeSession(ctx, validStartRequest("worker-1", "dispatch-1")); err != nil {
		t.Fatalf("Start() error = %v, want nil", err)
	}

	accepted := logger.entriesFor("worker session start accepted")
	if len(accepted) != 1 || accepted[0].fields["sessionID"] != "worker-1" ||
		accepted[0].fields["outcome"] != "reserved" || accepted[0].fields["state"] != "RESERVED" {
		t.Fatalf("start-accepted log = %#v", accepted)
	}
	assertNoPayloadOrCredentialKeys(t, accepted[0].fields)

	handoff := logger.entriesFor("worker session start")
	if len(handoff) != 1 || handoff[0].fields["sessionID"] != "worker-1" || handoff[0].fields["outcome"] != "handoff" {
		t.Fatalf("start-handoff log = %#v", handoff)
	}
	assertNoPayloadOrCredentialKeys(t, handoff[0].fields)

	terminal := logger.entriesFor("worker session start terminal")
	if len(terminal) != 1 || terminal[0].fields["sessionID"] != "worker-1" ||
		terminal[0].fields["outcome"] != "FAILED" || terminal[0].fields["cause"] != "EXECUTOR_PANIC" {
		t.Fatalf("start-terminal log = %#v", terminal)
	}
	assertNoPayloadOrCredentialKeys(t, terminal[0].fields)
	for _, entry := range terminal {
		for key, value := range entry.fields {
			if key == "cause" {
				continue
			}
			if text, ok := value.(string); ok && containsPanicWorkContent(text) {
				t.Fatalf("start-terminal log field %q leaked panic detail text: %#v", key, entry.fields)
			}
		}
	}
}

func containsPanicWorkContent(text string) bool {
	return text == "executor panic: boom"
}
