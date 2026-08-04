package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/events"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/worker_sessions/internal/service"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

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

	registry, err := service.New(execution, eventsSvc, nil)
	if err != nil {
		t.Fatalf("service.New() error = %v, want nil", err)
	}
	if _, err := registry.Start(context.Background(), validStartRequest("worker-1", "dispatch-1")); err != nil {
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

	registry, err := service.New(succeedingExecution(), eventsSvc, nil)
	if err != nil {
		t.Fatalf("service.New() error = %v, want nil", err)
	}
	if _, err := registry.Start(ctx, validStartRequest("worker-1", "dispatch-1")); err != nil {
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
	registry, err := service.New(execution, &brokenEventsAppender{}, nil)
	if err != nil {
		t.Fatalf("service.New() error = %v, want nil", err)
	}

	result, err := registry.Start(context.Background(), validStartRequest("worker-1", "dispatch-1"))
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
	registry, err := service.New(succeedingExecution(), appender, nil)
	if err != nil {
		t.Fatalf("service.New() error = %v, want nil", err)
	}

	req := validStartRequest("worker-1", "dispatch-1")
	req.ID = "   "
	if _, err := registry.Start(context.Background(), req); !errors.Is(err, workersessions.ErrInvalidSessionID) {
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
	registry, err := service.New(succeedingExecution(), appender, nil)
	if err != nil {
		t.Fatalf("service.New() error = %v, want nil", err)
	}
	ctx := context.Background()

	if _, err := registry.Start(ctx, validStartRequest("worker-1", "dispatch-1")); err != nil {
		t.Fatalf("first Start() error = %v, want nil", err)
	}
	callsAfterFirst := appender.callCount()
	if callsAfterFirst == 0 {
		t.Fatalf("first Start() published %d Events records, want at least 1", callsAfterFirst)
	}

	if _, err := registry.Start(ctx, validStartRequest("worker-1", "dispatch-2")); !errors.Is(err, workersessions.ErrSessionNotStartable) {
		t.Fatalf("second Start() error = %v, want ErrSessionNotStartable", err)
	}
	if got := appender.callCount(); got != callsAfterFirst {
		t.Fatalf("conflicting Start() published %d Events records, want unchanged %d", got, callsAfterFirst)
	}
}
