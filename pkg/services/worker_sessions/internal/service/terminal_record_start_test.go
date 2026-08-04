package service_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/events"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/worker_sessions/internal/service"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// terminalPayload decodes a committed record's payload as a workers.Draft and
// asserts its Kind/Phase, returning the decoded Draft for further payload
// inspection.
func decodeDraft(t *testing.T, record events.Record) workers.Draft {
	t.Helper()
	var draft workers.Draft
	if err := json.Unmarshal(record.Payload, &draft); err != nil {
		t.Fatalf("unmarshal record payload as workers.Draft error = %v", err)
	}
	return draft
}

// TestStart_CompletedSession_AppendsTerminalRecordAfterOpeningRecord proves
// story 004's core contract for the ordinary success path: reading the topic
// after a COMPLETED Start returns exactly the opening record followed by one
// terminal KindSession/PhaseCompleted record, in contiguous commit order.
func TestStart_CompletedSession_AppendsTerminalRecordAfterOpeningRecord(t *testing.T) {
	eventsSvc := newEventsAppender()
	topic := workersessions.Topic("worker-1")
	registry, err := service.New(succeedingExecution(), eventsSvc, nil)
	if err != nil {
		t.Fatalf("service.New() error = %v, want nil", err)
	}
	ctx := context.Background()

	result, err := registry.Start(ctx, validStartRequest("worker-1", "dispatch-1"))
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
	registry, err := service.New(execution, eventsSvc, nil)
	if err != nil {
		t.Fatalf("service.New() error = %v, want nil", err)
	}
	ctx := context.Background()

	result, err := registry.Start(ctx, validStartRequest("worker-1", "dispatch-1"))
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
	svc, err = service.New(execution, eventsSvc, nil)
	if err != nil {
		t.Fatalf("service.New() error = %v, want nil", err)
	}
	ctx := context.Background()

	if _, err := svc.Start(ctx, validStartRequest("worker-1", "dispatch-1")); err != nil {
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
	registry, err := service.New(succeedingExecution(), appender, nil)
	if err != nil {
		t.Fatalf("service.New() error = %v, want nil", err)
	}
	ctx := context.Background()

	result, err := registry.Start(ctx, validStartRequest("worker-1", "dispatch-1"))
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
	registry, err := service.New(succeedingExecution(), appender, nil)
	if err != nil {
		t.Fatalf("service.New() error = %v, want nil", err)
	}
	ctx := context.Background()

	if _, err := registry.Start(ctx, validStartRequest("worker-1", "dispatch-1")); err != nil {
		t.Fatalf("first Start() error = %v, want nil", err)
	}
	callsAfterFirst := appender.callCount()
	if callsAfterFirst != 2 {
		t.Fatalf("first Start() published %d Events records, want 2 (opening + terminal)", callsAfterFirst)
	}

	if _, err := registry.Start(ctx, validStartRequest("worker-1", "dispatch-2")); err == nil {
		t.Fatalf("second Start() on a terminal session error = nil, want ErrSessionNotStartable")
	}
	if got := appender.callCount(); got != callsAfterFirst {
		t.Fatalf("conflicting Start() published %d Events records, want unchanged %d (no second terminal record)", got, callsAfterFirst)
	}
}
