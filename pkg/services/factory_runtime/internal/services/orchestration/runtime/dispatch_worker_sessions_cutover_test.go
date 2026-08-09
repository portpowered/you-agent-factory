package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil/recordingfixtures"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	eventswire "github.com/portpowered/infinite-you/pkg/services/events/wire"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	workersessionswire "github.com/portpowered/infinite-you/pkg/services/worker_sessions/wire"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// blockingAssociationLedger pauses immediately after the canonical association
// is committed, producing the exact control window that formerly exposed an
// unknown Worker Session before Start had reserved its identity.
type blockingAssociationLedger struct {
	*recordingfixtures.ScriptedRuntimeLedger

	associated     chan struct{}
	associatedOnce sync.Once
	release        chan struct{}
}

func newBlockingAssociationLedger() *blockingAssociationLedger {
	return &blockingAssociationLedger{
		ScriptedRuntimeLedger: &recordingfixtures.ScriptedRuntimeLedger{},
		associated:            make(chan struct{}),
		release:               make(chan struct{}),
	}
}

func (l *blockingAssociationLedger) RecordDispatchWorkerSessionAssociation(
	tick int,
	dispatchID string,
	workerSessionID string,
	requestID string,
	eventTime time.Time,
) {
	l.ScriptedRuntimeLedger.RecordDispatchWorkerSessionAssociation(tick, dispatchID, workerSessionID, requestID, eventTime)
	l.associatedOnce.Do(func() { close(l.associated) })
	<-l.release
}

func TestStartThroughWorkerSessions_AssociationIsControlAddressableBeforeStart(t *testing.T) {
	workerService := newControlledWorkstationBoundary()
	workersBoundary := workers.NewWorkstationPoolBoundary(workers.WorkstationPoolBoundaryConfig{
		Service:    workerService,
		RouteNames: []string{"review"},
		Async:      true,
	})
	events, err := eventswire.NewService(logging.NoopLogger{})
	if err != nil {
		t.Fatalf("New events service: %v", err)
	}
	workerSessions, err := workersessionswire.NewService(workersBoundary, events, logging.NoopLogger{}, platformclock.Real{}, unavailableProviderSessions{})
	if err != nil {
		t.Fatalf("New Worker Sessions service: %v", err)
	}
	ledger := newBlockingAssociationLedger()
	request := workers.WorkstationDispatchRequest{
		WorkstationName: "review",
		Execution: workers.WorkstationExecutionRequest{Dispatch: work.WorkDispatch{
			DispatchID: "dispatch-1", WorkstationName: "review", Execution: work.ExecutionMetadata{RequestID: "turn-1"},
		}},
	}
	cfg := &runtimeConfig{workerSessions: workerSessions, clock: testRuntimeClock{}}
	accepted := make(chan workers.WorkstationDispatchResult, 1)
	acceptedErr := make(chan error, 1)
	startErr := make(chan error, 1)
	go func() {
		startErr <- startThroughWorkerSessions(context.Background(), cfg, ledger, workersBoundary, request, func(
			_ context.Context,
			_ workers.WorkstationDispatchRequest,
			result workers.WorkstationDispatchResult,
			err error,
		) {
			accepted <- result
			acceptedErr <- err
		})
	}()
	<-ledger.associated

	reserved, err := workerSessions.Get(context.Background(), workersessions.GetRequest{ID: "dispatch-1"})
	if err != nil || reserved.State != workersessions.StateReserved {
		t.Fatalf("Worker Session at association publication = %#v, %v, want addressable RESERVED session", reserved, err)
	}
	controlled, err := workerSessions.Cancel(context.Background(), workersessions.ControlRequest{ID: "dispatch-1"})
	if err != nil || controlled.Outcome != workersessions.ControlOutcomeApplied || controlled.Session.State != workersessions.StateCanceled {
		t.Fatalf("Cancel() in association/Start window = %#v, %v, want applied CANCELED", controlled, err)
	}

	close(ledger.release)
	if err := <-startErr; err != nil {
		t.Fatalf("startThroughWorkerSessions() error = %v", err)
	}
	result := <-accepted
	if err := <-acceptedErr; !errors.Is(err, workers.ErrWorkstationDispatchCanceled) ||
		result.TerminalOutcome != workers.WorkstationDispatchTerminalOutcomeCanceled {
		t.Fatalf("accepted control-won result = %#v, %v, want canceled Workers result", result, err)
	}
	select {
	case dispatched := <-workerService.requests:
		t.Fatalf("Workers dispatch started after pre-admission control: %#v", dispatched)
	default:
	}
}

// TestFactoryImpl_PlanDispatchRecordsWorkerSessionAssociationBeforeWorkersHandoff
// proves the W4 Runtime dispatch cutover ordering guarantee: the canonical
// dispatch-to-Worker-Session association is committed to Factory Events
// before worker_sessions.Service.Start can hand the attempt to Workers.
// controlledWorkstationBoundary only receives a DispatchWorkstation call once
// Start has reserved the session, transitioned STARTING, and handed off --
// so observing the association at that exact point proves the ordering
// without needing a controlled Worker Sessions fake.
func TestFactoryImpl_PlanDispatchRecordsWorkerSessionAssociationBeforeWorkersHandoff(t *testing.T) {
	boundary := newControlledWorkstationBoundary()
	runtime, ledger, err := newTestFactoryWithScriptedLedger(
		withNet(buildSimpleNet()),
		withInlineDispatch(),
		withWorkerService(boundary),
		withLogger(logging.NoopLogger{}),
	)
	requireNoRootErr(t, err, "New")

	impl, ok := runtime.(*factoryImpl)
	if !ok {
		t.Fatalf("factory type = %T, want *factoryImpl", runtime)
	}
	impl.state = interfaces.FactoryStateRunning

	plan := factory.PlanDispatchRequest{
		DispatchID:      "assoc-dispatch-1",
		CorrelationID:   "assoc-corr-1",
		WorkIDs:         []string{"assoc-work-1"},
		WorkstationName: "t-process",
		WorkerType:      "mock",
		ReplayKey:       "t-process/assoc-trace/assoc-work-1",
	}

	plannedCh := make(chan factory.PlanDispatchResult, 1)
	planErrCh := make(chan error, 1)
	go func() {
		planned, planErr := impl.PlanDispatch(t.Context(), plan)
		plannedCh <- planned
		planErrCh <- planErr
	}()

	request := awaitCanonicalWorkersRequest(t, boundary.requests)
	associations := ledger.DispatchWorkerSessionAssociationsSnapshot()
	if len(associations) != 1 {
		t.Fatalf("associations observed before Workers handoff = %#v, want exactly one", associations)
	}
	if associations[0].DispatchID != plan.DispatchID {
		t.Fatalf("association dispatch ID = %q, want %q", associations[0].DispatchID, plan.DispatchID)
	}
	if associations[0].WorkerSessionID == "" {
		t.Fatal("association Worker Session ID = empty, want a stable non-empty identity")
	}

	boundary.results <- completedWorkersResult(request)
	requireNoRootErr(t, <-planErrCh, "PlanDispatch")
	planned := <-plannedCh
	if planned.Outcome != factory.DispatchPlanOutcomeAccepted {
		t.Fatalf("PlanDispatch outcome = %q, want ACCEPTED", planned.Outcome)
	}

	if got := len(ledger.DispatchWorkerSessionAssociationsSnapshot()); got != 1 {
		t.Fatalf("final association count = %d, want exactly one (no duplicate from the terminal path)", got)
	}
}

// TestFactoryImpl_PlanDispatchExecutesThroughWorkerSessionsStart proves every
// resolved dispatch now executes through worker_sessions.Service.Start (which
// drives the existing Workers workstation-pool boundary underneath) instead
// of Runtime invoking that boundary directly, while preserving the existing
// accepted dispatch result shape.
func TestFactoryImpl_PlanDispatchExecutesThroughWorkerSessionsStart(t *testing.T) {
	executor := &recordingRootBoundaryExecutor{}
	runtime, err := newTestFactory(
		withNet(buildSimpleNet()),
		withInlineDispatch(),
		withWorkerExecutor("mock", executor),
		withLogger(logging.NoopLogger{}),
	)
	requireNoRootErr(t, err, "New")

	impl, ok := runtime.(*factoryImpl)
	if !ok {
		t.Fatalf("factory type = %T, want *factoryImpl", runtime)
	}
	impl.state = interfaces.FactoryStateRunning

	plan := factory.PlanDispatchRequest{
		DispatchID:      "worker-sessions-cutover-dispatch",
		CorrelationID:   "worker-sessions-cutover-corr",
		WorkIDs:         []string{"worker-sessions-cutover-work"},
		WorkstationName: "t-process",
		WorkerType:      "mock",
		ReplayKey:       "t-process/worker-sessions-cutover-trace/worker-sessions-cutover-work",
	}

	planned, err := impl.PlanDispatch(t.Context(), plan)
	requireNoRootErr(t, err, "PlanDispatch")
	if planned.Outcome != factory.DispatchPlanOutcomeAccepted {
		t.Fatalf("PlanDispatch outcome = %q, want ACCEPTED", planned.Outcome)
	}
	if executor.calls.Load() != 1 {
		t.Fatalf("Workers executor calls = %d, want 1 through worker_sessions.Service.Start", executor.calls.Load())
	}
	lastDispatchID, _ := executor.lastDispatchID.Load().(string)
	if lastDispatchID != plan.DispatchID {
		t.Fatalf("executed dispatch ID = %q, want %q", lastDispatchID, plan.DispatchID)
	}

	accepted, err := impl.AcceptDispatchResult(t.Context(), factory.AcceptDispatchResultRequest{
		DispatchID:    plan.DispatchID,
		CorrelationID: plan.CorrelationID,
		WorkID:        "worker-sessions-cutover-work",
		ResultOutcome: factory.DispatchResultOutcomeSuccess,
	})
	requireNoRootErr(t, err, "AcceptDispatchResult")
	if accepted.Outcome != factory.DispatchPlanOutcomeDuplicateIdempotent {
		t.Fatalf(
			"AcceptDispatchResult outcome = %q, want DUPLICATE_IDEMPOTENT after Worker Sessions completion",
			accepted.Outcome,
		)
	}
}

// newInvokeWorkerTestFactory composes the real Worker Sessions service over the
// controlled Workers boundary for the InvokeWorker cells, because identity
// reservation is the behavior those cells test and a fake Reserve would concede
// it. It lives beside the cutover harness rather than with its own cells
// because this is the one file in the package that already assembles a real
// Worker Sessions service, and one such assembly is enough.
func newInvokeWorkerTestFactory(
	t *testing.T,
	boundary *controlledWorkstationBoundary,
) *factoryImpl {
	t.Helper()
	poolBoundary := workers.NewWorkstationPoolBoundary(workers.WorkstationPoolBoundaryConfig{
		Service:    boundary,
		RouteNames: []string{workers.ProviderInvocationRoute},
		Async:      true,
	})
	events, err := eventswire.NewService(logging.NoopLogger{})
	requireNoRootErr(t, err, "New events service")
	sessions, err := workersessionswire.NewService(poolBoundary, events, logging.NoopLogger{}, platformclock.Real{}, unavailableProviderSessions{})
	requireNoRootErr(t, err, "New Worker Sessions service")

	runtime, _, err := newTestFactoryWithScriptedLedger(
		withNet(buildSimpleNet()),
		withInlineDispatch(),
		withWorkerService(boundary),
		withWorkerSessions(sessions),
		withLogger(logging.NoopLogger{}),
	)
	requireNoRootErr(t, err, "New")
	impl, ok := runtime.(*factoryImpl)
	if !ok {
		t.Fatalf("factory type = %T, want *factoryImpl", runtime)
	}
	return impl
}
func TestRecordedWorkerSessionObservation_ListsHistoricalAttemptsInChronologicalOrder(t *testing.T) {
	base := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	workID := "work-recorded"
	events := recordedObservationTestEvents(t, base, workID)
	ledger := &recordingfixtures.ScriptedRuntimeLedger{Events: events}
	projector := func(_ []interfaces.FactoryEvent, _ int) (interfaces.FactoryWorldState, error) {
		return interfaces.FactoryWorldState{
			WorkItemsByID: map[string]work.FactoryWorkItem{
				workID: {ID: workID},
			},
			CompletedDispatches: []interfaces.FactoryWorldDispatchCompletion{
				{
					DispatchID:  "dispatch-late",
					StartedAt:   base.Add(5 * time.Second),
					CompletedAt: base.Add(7 * time.Second),
					WorkItemIDs: []string{workID},
					Result:      interfaces.WorkstationResult{Outcome: string(workers.OutcomeAccepted)},
				},
				{
					DispatchID:  "dispatch-early",
					StartedAt:   base.Add(time.Second),
					CompletedAt: base.Add(3 * time.Second),
					WorkItemIDs: []string{workID},
					Result:      interfaces.WorkstationResult{Outcome: string(workers.OutcomeAccepted)},
				},
			},
		}, nil
	}

	service := newRecordedWorkerSessionObservation(
		nil,
		ledger,
		factory.WorldStateProjector(projector),
		platformclock.NewDeterministic(base, time.Second),
	)
	result, err := service.ListObservations(context.Background(), workersessions.ListObservationsRequest{WorkID: workID})
	if err != nil {
		t.Fatalf("ListObservations() error = %v", err)
	}
	if len(result.Observations) != 2 {
		t.Fatalf("ListObservations() returned %d observations, want 2", len(result.Observations))
	}
	if got := result.Observations[0].WorkerSessionID; got != "worker-early" {
		t.Fatalf("first Worker Session = %q, want worker-early", got)
	}
	if got := result.Observations[1].WorkerSessionID; got != "worker-late" {
		t.Fatalf("second Worker Session = %q, want worker-late", got)
	}
	if result.Observations[0].AttemptID != "dispatch-early" || result.Observations[0].Duration == nil || *result.Observations[0].Duration != 2*time.Second {
		t.Fatalf("early recorded observation = %#v, want dispatch-early with 2s duration", result.Observations[0])
	}
	if result.Observations[1].AttemptID != "dispatch-late" || result.Observations[1].TurnID != "turn-late" {
		t.Fatalf("late recorded observation = %#v, want dispatch-late/turn-late", result.Observations[1])
	}
}

func TestRecordedWorkerSessionObservation_KnownWorkWithoutSessionsIsExplicitlyEmpty(t *testing.T) {
	workID := "work-without-sessions"
	event := interfaces.FactoryEvent{
		Context: interfaces.FactoryEventContext{Tick: 1, WorkIDs: stringSliceForRecordedTest([]string{workID})},
		Type:    interfaces.FactoryEventTypeWorkRequest,
	}
	ledger := &recordingfixtures.ScriptedRuntimeLedger{Events: []interfaces.FactoryEvent{event}}
	service := newRecordedWorkerSessionObservation(
		nil,
		ledger,
		func(_ []interfaces.FactoryEvent, _ int) (interfaces.FactoryWorldState, error) {
			return interfaces.FactoryWorldState{WorkItemsByID: map[string]work.FactoryWorkItem{workID: {ID: workID}}}, nil
		},
		platformclock.Real{},
	)

	result, err := service.ListObservations(context.Background(), workersessions.ListObservationsRequest{WorkID: workID})
	if err != nil {
		t.Fatalf("ListObservations() error = %v, want explicit empty success", err)
	}
	if result.Observations == nil || len(result.Observations) != 0 {
		t.Fatalf("observations = %#v, want non-nil empty result", result.Observations)
	}
}

func recordedObservationTestEvents(t *testing.T, base time.Time, workID string) []interfaces.FactoryEvent {
	t.Helper()
	requestPayload, err := json.Marshal(interfaces.DispatchRequestEventPayload{TransitionID: "review"})
	if err != nil {
		t.Fatalf("marshal dispatch request: %v", err)
	}
	events := []interfaces.FactoryEvent{
		{
			Context: interfaces.FactoryEventContext{
				Tick: 1, Sequence: 1, EventTime: base.Add(time.Second),
				DispatchID: stringPointerForRecordedTest("dispatch-early"),
				WorkIDs:    stringSliceForRecordedTest([]string{workID}),
			},
			Type:    interfaces.FactoryEventTypeDispatchRequest,
			Payload: requestPayload,
		},
		{
			Context: interfaces.FactoryEventContext{
				Tick: 1, Sequence: 2, EventTime: base.Add(time.Second),
				DispatchID: stringPointerForRecordedTest("dispatch-early"),
				RequestID:  stringPointerForRecordedTest("turn-early"),
			},
			Type:    interfaces.FactoryEventTypeDispatchWorkerSessionAssoc,
			Payload: mustMarshalRecordedTest(t, interfaces.DispatchWorkerSessionAssociationEventPayload{WorkerSessionID: "worker-early"}),
		},
		{
			Context: interfaces.FactoryEventContext{
				Tick: 5, Sequence: 1, EventTime: base.Add(5 * time.Second),
				DispatchID: stringPointerForRecordedTest("dispatch-late"),
				WorkIDs:    stringSliceForRecordedTest([]string{workID}),
			},
			Type:    interfaces.FactoryEventTypeDispatchRequest,
			Payload: requestPayload,
		},
		{
			Context: interfaces.FactoryEventContext{
				Tick: 5, Sequence: 2, EventTime: base.Add(5 * time.Second),
				DispatchID: stringPointerForRecordedTest("dispatch-late"),
				RequestID:  stringPointerForRecordedTest("turn-late"),
			},
			Type:    interfaces.FactoryEventTypeDispatchWorkerSessionAssoc,
			Payload: mustMarshalRecordedTest(t, interfaces.DispatchWorkerSessionAssociationEventPayload{WorkerSessionID: "worker-late"}),
		},
	}
	return events
}

func mustMarshalRecordedTest(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal recorded test payload: %v", err)
	}
	return data
}

func stringPointerForRecordedTest(value string) *string { return &value }

func stringSliceForRecordedTest(value []string) *[]string { return &value }

var _ recordings.RuntimeLedger = (*recordingfixtures.ScriptedRuntimeLedger)(nil)
