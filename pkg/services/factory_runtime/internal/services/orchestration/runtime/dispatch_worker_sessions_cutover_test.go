package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil/recordingfixtures"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
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

func TestInvokeWorkerRuntimeAttemptReservesBeforeAssociationAndMapsCanceledAdmission(t *testing.T) {
	t.Parallel()
	sessions := &associationWindowSessions{
		reserved: make(chan workersessions.ReserveRequest, 1),
		invoked:  make(chan workersessions.RuntimeAttemptRequest, 1),
	}
	ledger := newBlockingAssociationLedger()
	release := sync.OnceFunc(func() { close(ledger.release) })
	t.Cleanup(release)
	f := &factoryImpl{cfg: &runtimeConfig{workerSessions: sessions, workerAttempts: sessions, clock: testRuntimeClock{}, runtimeID: "runtime-window"}, eventHistory: ledger}
	results := make(chan struct {
		result factory.InvokeWorkerResult
		err    error
	}, 1)
	go func() {
		result, err := f.InvokeWorker(context.Background(), factory.InvokeWorkerRequest{DispatchID: "dispatch-1", Prompt: "review"})
		results <- struct {
			result factory.InvokeWorkerResult
			err    error
		}{result, err}
	}()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	select {
	case <-ledger.associated:
	case <-ctx.Done():
		t.Fatal("association was not published")
	}
	assertInvokeWorkerReservationWindow(t, sessions, ledger)
	release()
	select {
	case got := <-results:
		if got.err != nil || got.result.DispatchID != "dispatch-1" || got.result.WorkerSessionID != "dispatch-1" || got.result.Outcome != factory.InvokeWorkerOutcomeCanceled {
			t.Fatalf("canceled admission = %#v, %v", got.result, got.err)
		}
	case <-ctx.Done():
		t.Fatal("invocation did not return after association release")
	}
	select {
	case invoked := <-sessions.invoked:
		if invoked.ID != "dispatch-1" || invoked.AttemptID != "dispatch-1" || invoked.Key != (workersessions.RuntimeAttemptKey{RuntimeID: "runtime-window", DispatchID: "dispatch-1"}) {
			t.Fatalf("invocation did not retain reserved identity and key: %#v", invoked)
		}
	default:
		t.Fatal("canceled result returned without invoking keyed supervision")
	}
}

// TestFactoryImpl_PlanDispatchRecordsWorkerSessionAssociationBeforeWorkersHandoff
// proves the W4 Runtime dispatch cutover ordering guarantee: the canonical
// dispatch-to-Worker-Session association is committed to Factory Events
// before worker_sessions.Service.Start can hand the attempt to Workers.
// controlledWorkstationBoundary only receives a DispatchWorkstation call once
// the service-root test double has reserved the session and handed off -- so
// observing the association at that exact point proves the ordering.
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
// drives the injected Workers execution service underneath) instead
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
	execution *controlledWorkstationBoundary,
) *factoryImpl {
	t.Helper()
	sessions := newRuntimeWorkerSessionsService(execution)

	runtime, _, err := newTestFactoryWithScriptedLedger(
		withNet(buildSimpleNet()),
		withInlineDispatch(),
		withWorkerService(execution),
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
		nil,
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

func TestRecordedWorkerSessionObservationUsesRestoredWorldState(t *testing.T) {
	base := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	workID := "work-restored"
	events := recordedObservationTestEvents(t, base, workID)
	ledger := &recordingfixtures.ScriptedRuntimeLedger{Events: events}
	restored := interfaces.FactoryWorldState{
		WorkItemsByID: map[string]work.FactoryWorkItem{workID: {ID: workID}},
		CompletedDispatches: []interfaces.FactoryWorldDispatchCompletion{
			{
				DispatchID:  "dispatch-early",
				StartedAt:   base.Add(time.Second),
				CompletedAt: base.Add(3 * time.Second),
				WorkItemIDs: []string{workID},
				Result:      interfaces.WorkstationResult{Outcome: string(workers.OutcomeAccepted)},
			},
			{
				DispatchID:  "dispatch-late",
				StartedAt:   base.Add(5 * time.Second),
				CompletedAt: base.Add(7 * time.Second),
				WorkItemIDs: []string{workID},
				Result:      interfaces.WorkstationResult{Outcome: string(workers.OutcomeAccepted)},
			},
		},
	}
	projectorCalls := 0
	service := newRecordedWorkerSessionObservationWithRestoredState(
		nil,
		ledger,
		func([]interfaces.FactoryEvent, int) (interfaces.FactoryWorldState, error) {
			projectorCalls++
			return interfaces.FactoryWorldState{}, errors.New("full projection should not run for unchanged restored history")
		},
		platformclock.NewDeterministic(base, time.Second),
		nil,
		nil,
		"",
		nil,
		&restored,
		events,
	)

	result, err := service.ListObservations(context.Background(), workersessions.ListObservationsRequest{WorkID: workID})
	if err != nil {
		t.Fatalf("ListObservations() error = %v", err)
	}
	if projectorCalls != 0 {
		t.Fatalf("full world projection calls = %d, want 0", projectorCalls)
	}
	if len(result.Observations) != 2 || result.Observations[0].State != workersessions.StateCompleted || result.Observations[1].State != workersessions.StateCompleted {
		t.Fatalf("restored observations = %#v, want two completed attempts", result.Observations)
	}
}

func TestRecordedWorkerSessionObservation_ReplaysHistoricalTerminalStream(t *testing.T) {
	base := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	workID := "work-recorded-stream"
	dispatchID := "dispatch-recorded-stream"
	providerMetadata := &providers.SessionMetadata{Provider: string(providers.IDCodex), Kind: providers.SessionIDKind, ID: "provider-session-recorded"}
	events := []interfaces.FactoryEvent{
		{
			Context: interfaces.FactoryEventContext{Tick: 1, Sequence: 1, EventTime: base, DispatchID: stringPointerForRecordedTest(dispatchID), WorkIDs: stringSliceForRecordedTest([]string{workID})},
			Id:      "recorded-request",
			Type:    interfaces.FactoryEventTypeDispatchRequest,
		},
		{
			Context: interfaces.FactoryEventContext{Tick: 1, Sequence: 2, EventTime: base.Add(time.Second), DispatchID: stringPointerForRecordedTest(dispatchID)},
			Id:      "recorded-association",
			Type:    interfaces.FactoryEventTypeDispatchWorkerSessionAssoc,
			Payload: mustMarshalRecordedTest(t, interfaces.DispatchWorkerSessionAssociationEventPayload{WorkerSessionID: "worker-recorded-stream"}),
		},
		{
			Context: interfaces.FactoryEventContext{Tick: 2, Sequence: 3, EventTime: base.Add(2 * time.Second), DispatchID: stringPointerForRecordedTest(dispatchID)},
			Id:      "recorded-response",
			Type:    interfaces.FactoryEventTypeDispatchResponse,
		},
	}
	ledger := &recordingfixtures.ScriptedRuntimeLedger{Events: events}
	service := newRecordedWorkerSessionObservation(
		nil,
		ledger,
		func(_ []interfaces.FactoryEvent, _ int) (interfaces.FactoryWorldState, error) {
			return interfaces.FactoryWorldState{
				CompletedDispatches: []interfaces.FactoryWorldDispatchCompletion{{
					DispatchID: dispatchID, StartedAt: base, CompletedAt: base.Add(2 * time.Second), WorkItemIDs: []string{workID},
					Result: interfaces.WorkstationResult{Outcome: string(workers.OutcomeAccepted)}, ProviderSession: providerMetadata,
				}},
			}, nil
		},
		platformclock.Real{},
		nil,
	)

	subscription, err := service.StreamObservations(context.Background(), workersessions.StreamObservationsRequest{ProviderSession: providerSessionRef(*providerMetadata)})
	if err != nil {
		t.Fatalf("StreamObservations() error = %v", err)
	}
	defer subscription.Close()
	for index, want := range []workersessions.ObservationDeliveryKind{
		workersessions.ObservationDeliveryRecord,
		workersessions.ObservationDeliveryRecord,
		workersessions.ObservationDeliveryTerminalReplay,
	} {
		delivery := subscription.Next(context.Background())
		if delivery.Kind != want || delivery.Event.SourceSequence != uint64(index+1) || delivery.Event.SourceID == "" {
			t.Fatalf("delivery %d = %#v, want %s at canonical sequence %d", index, delivery, want, index+1)
		}
	}
	if closed := subscription.Next(context.Background()); closed.Kind != workersessions.ObservationDeliveryClosed {
		t.Fatalf("delivery after historical terminal = %#v, want CLOSED", closed)
	}
}

func TestRecordedWorkerSessionObservationStreamUsesAtomicSnapshotAndPreservesHistory(t *testing.T) {
	base := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	workID := "work-recorded-race"
	dispatchID := "dispatch-recorded-race"
	providerMetadata := &providers.SessionMetadata{Provider: string(providers.IDCodex), Kind: providers.SessionIDKind, ID: "provider-session-race"}
	targetEvent := func(sequence int, id string, eventType interfaces.FactoryEventType) interfaces.FactoryEvent {
		return interfaces.FactoryEvent{
			Context: interfaces.FactoryEventContext{
				Tick:       sequence,
				Sequence:   sequence,
				EventTime:  base.Add(time.Duration(sequence) * time.Second),
				DispatchID: stringPointerForRecordedTest(dispatchID),
			},
			Id:   id,
			Type: eventType,
		}
	}
	activeEvents := []interfaces.FactoryEvent{
		targetEvent(1, "race-request", interfaces.FactoryEventTypeDispatchRequest),
		targetEvent(2, "race-association", interfaces.FactoryEventTypeDispatchWorkerSessionAssoc),
	}
	activeEvents[1].Payload = mustMarshalRecordedTest(t, interfaces.DispatchWorkerSessionAssociationEventPayload{WorkerSessionID: "worker-recorded-race"})
	terminal := targetEvent(3, "race-response", interfaces.FactoryEventTypeDispatchResponse)
	other := terminal.Clone()
	other.Context.DispatchID = stringPointerForRecordedTest("other-dispatch")
	other.Id = "other-response"
	ledger := &recordingfixtures.ScriptedRuntimeLedger{
		Events: activeEvents,
		SubscribeResult: interfaces.FactoryEventStream{
			History: []interfaces.FactoryEvent{activeEvents[0], activeEvents[1], other, terminal},
			Events:  make(chan interfaces.FactoryEvent),
		},
	}
	service := newRecordedWorkerSessionObservation(
		nil,
		ledger,
		func(_ []interfaces.FactoryEvent, _ int) (interfaces.FactoryWorldState, error) {
			return interfaces.FactoryWorldState{
				ActiveDispatches: map[string]interfaces.FactoryWorldDispatch{
					dispatchID: {DispatchID: dispatchID, StartedAt: base, WorkItemIDs: []string{workID}},
				},
				ProviderSessions: []interfaces.FactoryWorldProviderSessionRecord{{
					DispatchID: dispatchID, ProviderSession: *providerMetadata, WorkItemIDs: []string{workID},
				}},
			}, nil
		},
		platformclock.NewDeterministic(base.Add(5*time.Second), time.Second),
		nil,
	)

	subscription, err := service.StreamObservations(context.Background(), workersessions.StreamObservationsRequest{
		ProviderSession: providerSessionRef(*providerMetadata),
		Limit:           2,
	})
	if err != nil {
		t.Fatalf("StreamObservations() error = %v", err)
	}
	defer subscription.Close()
	if delivery := subscription.Next(context.Background()); delivery.Kind != workersessions.ObservationDeliveryRecord || delivery.Event.SourceID != "race-request" {
		t.Fatalf("durable request delivery = %#v, want the oldest retained record", delivery)
	}
	if delivery := subscription.Next(context.Background()); delivery.Kind != workersessions.ObservationDeliveryRecord || delivery.Event.SourceID != "race-association" {
		t.Fatalf("durable association delivery = %#v, want association after the request", delivery)
	}
	if delivery := subscription.Next(context.Background()); delivery.Kind != workersessions.ObservationDeliveryTerminalReplay || delivery.Event.SourceID != "race-response" {
		t.Fatalf("atomic terminal delivery = %#v, want TERMINAL_REPLAY from the subscription snapshot", delivery)
	}
	if delivery := subscription.Next(context.Background()); delivery.Kind != workersessions.ObservationDeliveryClosed {
		t.Fatalf("delivery after terminal snapshot = %#v, want CLOSED", delivery)
	}
}

func TestRecordedWorkerSessionObservation_WorkerIDReadsNoProviderHistory(t *testing.T) {
	base := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	workID := "work-no-provider"
	dispatchID := "dispatch-no-provider"
	workerSessionID := "worker-no-provider"
	events := []interfaces.FactoryEvent{
		{
			Context: interfaces.FactoryEventContext{Tick: 1, Sequence: 1, EventTime: base, DispatchID: stringPointerForRecordedTest(dispatchID), WorkIDs: stringSliceForRecordedTest([]string{workID})},
			Id:      "no-provider-request",
			Type:    interfaces.FactoryEventTypeDispatchRequest,
			Payload: mustMarshalRecordedTest(t, interfaces.DispatchRequestEventPayload{TransitionID: "review"}),
		},
		{
			Context: interfaces.FactoryEventContext{Tick: 1, Sequence: 2, EventTime: base.Add(time.Second), DispatchID: stringPointerForRecordedTest(dispatchID)},
			Id:      "no-provider-association",
			Type:    interfaces.FactoryEventTypeDispatchWorkerSessionAssoc,
			Payload: mustMarshalRecordedTest(t, interfaces.DispatchWorkerSessionAssociationEventPayload{WorkerSessionID: workerSessionID}),
		},
		{
			Context: interfaces.FactoryEventContext{Tick: 2, Sequence: 3, EventTime: base.Add(2 * time.Second), DispatchID: stringPointerForRecordedTest(dispatchID)},
			Id:      "no-provider-response",
			Type:    interfaces.FactoryEventTypeDispatchResponse,
		},
	}
	ledger := &recordingfixtures.ScriptedRuntimeLedger{Events: events}
	service := newRecordedWorkerSessionObservation(
		nil,
		ledger,
		func(_ []interfaces.FactoryEvent, _ int) (interfaces.FactoryWorldState, error) {
			return interfaces.FactoryWorldState{CompletedDispatches: []interfaces.FactoryWorldDispatchCompletion{{
				DispatchID: dispatchID, StartedAt: base, CompletedAt: base.Add(2 * time.Second), WorkItemIDs: []string{workID},
				Result: interfaces.WorkstationResult{Outcome: string(workers.OutcomeAccepted)},
			}}}, nil
		},
		platformclock.Real{},
		nil,
	)

	show, err := service.GetObservationByWorkerSessionID(context.Background(), workersessions.GetObservationByWorkerSessionIDRequest{WorkerSessionID: workerSessionID})
	if err != nil {
		t.Fatalf("GetObservationByWorkerSessionID() error = %v", err)
	}
	if show.WorkerSessionID != workerSessionID || show.ProviderSessionAvailable || show.ProviderSession != (providers.SessionRef{}) || show.State != workersessions.StateCompleted {
		t.Fatalf("no-provider observation = %#v, want canonical provider-neutral completion", show)
	}

	if _, err := service.ReadTranscript(context.Background(), workersessions.ReadTranscriptRequest{WorkerSessionID: workerSessionID}); !errors.Is(err, workersessions.ErrObservationTranscriptUnavailable) {
		t.Fatalf("ReadTranscript(no provider) error = %v, want explicit unavailable result", err)
	}

	subscription, err := service.StreamObservationsByWorkerSessionID(context.Background(), workersessions.StreamObservationsByWorkerSessionIDRequest{WorkerSessionID: workerSessionID, ReplayOnly: true})
	if err != nil {
		t.Fatalf("StreamObservationsByWorkerSessionID(no provider) error = %v", err)
	}
	defer subscription.Close()
	if first := subscription.Next(context.Background()); first.Kind != workersessions.ObservationDeliveryRecord || first.Event.SourceID != "no-provider-request" {
		t.Fatalf("first no-provider delivery = %#v, want request record", first)
	}
	if second := subscription.Next(context.Background()); second.Kind != workersessions.ObservationDeliveryRecord || second.Event.SourceID != "no-provider-association" {
		t.Fatalf("second no-provider delivery = %#v, want association record", second)
	}
	if third := subscription.Next(context.Background()); third.Kind != workersessions.ObservationDeliveryTerminalReplay || third.Event.SourceID != "no-provider-response" {
		t.Fatalf("third no-provider delivery = %#v, want terminal response record", third)
	}
	if closed := subscription.Next(context.Background()); closed.Kind != workersessions.ObservationDeliveryClosed {
		t.Fatalf("no-provider terminal delivery = %#v, want closed stream", closed)
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
		nil,
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

func TestWorkerSessionDispatchOutcomeBranches(t *testing.T) {
	request := workers.WorkstationDispatchRequest{WorkstationName: "review", Execution: workers.WorkstationExecutionRequest{Dispatch: work.WorkDispatch{DispatchID: "dispatch-1", WorkstationName: "review"}}}
	startErr := errors.New("start failed")
	if result, err := workerSessionDispatchOutcome(request, workersessions.InvokeSessionResult{}, startErr); !errors.Is(err, startErr) || result.TerminalOutcome != workers.WorkstationDispatchTerminalOutcomeFailed || result.Result.Error != startErr.Error() {
		t.Fatalf("start error outcome = %#v, %v", result, err)
	}
	passthrough := workersessions.InvokeSessionResult{Dispatch: workers.WorkstationDispatchResult{DispatchID: "dispatch-1", TerminalOutcome: workers.WorkstationDispatchTerminalOutcomeCompleted}}
	if result, err := workerSessionDispatchOutcome(request, passthrough, nil); err != nil || result.TerminalOutcome != workers.WorkstationDispatchTerminalOutcomeCompleted {
		t.Fatalf("handed-off outcome = %#v, %v", result, err)
	}
	publicationFailure := workersessions.InvokeSessionResult{Session: workersessions.Session{Result: &workersessions.TerminalResult{Cause: &workersessions.FailureCause{Kind: workersessions.FailureCauseEventPublicationFailure, Detail: "publication failed"}}}}
	if result, err := workerSessionDispatchOutcome(request, publicationFailure, nil); err != nil || result.TerminalOutcome != workers.WorkstationDispatchTerminalOutcomeFailed || result.Result.Error != string(workersessions.FailureCauseEventPublicationFailure) {
		t.Fatalf("pre-handoff outcome = %#v, %v", result, err)
	}
	if handedOffToWorkers(publicationFailure) || !handedOffToWorkers(workersessions.InvokeSessionResult{}) {
		t.Fatal("handedOffToWorkers() classified empty/publication results incorrectly")
	}
}

func TestRecordedObservationListBranches(t *testing.T) {
	if got, err := recordedObservationListResult(nil, false, workersessions.ListObservationsResult{Observations: []workersessions.Observation{{WorkerSessionID: "live"}}}, nil); err != nil || len(got.Observations) != 1 {
		t.Fatalf("live fallback = %#v, %v", got, err)
	}
	if _, err := recordedObservationListResult(nil, false, workersessions.ListObservationsResult{}, workersessions.ErrObservationWorkNotFound); !errors.Is(err, workersessions.ErrObservationWorkNotFound) {
		t.Fatalf("missing work error = %v", err)
	}
	recorded := []workersessions.Observation{{WorkerSessionID: "b", AttemptID: "b"}, {WorkerSessionID: "a", AttemptID: "a"}}
	if got, err := recordedObservationListResult(recorded, true, workersessions.ListObservationsResult{}, workersessions.ErrObservationProjectionUnavailable); err != nil || got.Observations[0].AttemptID != "a" {
		t.Fatalf("recorded result = %#v, %v", got, err)
	}
	if got, err := recordedObservationListResult(nil, true, workersessions.ListObservationsResult{Observations: []workersessions.Observation{{WorkerSessionID: "live-known"}}}, nil); err != nil || len(got.Observations) != 1 {
		t.Fatalf("known live fallback = %#v, %v", got, err)
	}

}

func TestRecordedFailureMappingBranches(t *testing.T) {
	failureDetail := &workers.FailureDetail{Reason: workers.WorkFailureTypeAuthFailure}
	for _, reason := range []workers.WorkFailureType{workers.WorkFailureTypeAuthFailure, workers.WorkFailureTypeTimeout, workers.WorkFailureTypeThrottled, workers.WorkFailureTypeMisconfigured, workers.WorkFailureTypeUnknown, workers.WorkFailureTypeStructuredOutputSchemaViolation} {
		if failure := recordedFailure(workers.OutcomeFailed, failureDetail, &workers.WorkFailureMetadata{Family: workers.WorkFailureFamilyTerminal, Type: reason}, workersessions.StateFailed); failure == nil || failure.Detail == "" {
			t.Fatalf("recordedFailure(%q) = %#v", reason, failure)
		}
	}
	rejected := recordedFailure(workers.OutcomeRejected, nil, nil, workersessions.StateFailed)
	if rejected == nil || rejected.Kind != workersessions.FailureCauseRejected {
		t.Fatalf("recordedFailure(REJECTED) = %#v, want bounded REJECTED classification", rejected)
	}
	if recordedProviderFailureKind(&workers.FailureDetail{Reason: workers.WorkFailureTypeAuthFailure}) != providers.ExecuteFailureKindAuthentication || recordedProviderFailureKind(&workers.FailureDetail{Reason: workers.WorkFailureTypeTimeout}) != providers.ExecuteFailureKindTimeout || recordedProviderFailureKind(&workers.FailureDetail{Reason: workers.WorkFailureTypeThrottled}) != providers.ExecuteFailureKindThrottled || recordedProviderFailureKind(&workers.FailureDetail{Reason: workers.WorkFailureTypeMisconfigured}) != providers.ExecuteFailureKindMisconfigured || recordedProviderFailureKind(nil) != "" {
		t.Fatal("recordedProviderFailureKind() mapping is incorrect")
	}
}

func TestRecordedFailureKindAndTypeBranches(t *testing.T) {
	if family, ok := recordedFailureFamily(workers.WorkFailureFamilyRetryable); !ok || family == "" {
		t.Fatal("recordedFailureFamily(retryable) missing known family")
	}
	if _, ok := recordedFailureFamily("foreign"); ok {
		t.Fatal("recordedFailureFamily(foreign) = known")
	}
	if typ, ok := recordedFailureType(workers.WorkFailureTypeStructuredOutputSchemaViolation); !ok || typ == "" {
		t.Fatal("recordedFailureType(structured schema violation) missing known type")
	}
	if _, ok := recordedFailureType("foreign"); ok {
		t.Fatal("recordedFailureType(foreign) = known")
	}
}

func TestRecordedFailureDetailFallback(t *testing.T) {
	if recordedFailureDetail(workersessions.FailureCauseWorkersExecutionFailure, nil, nil) == "" || recordedFailureDetail(workersessions.FailureCauseWorkersExecutionFailure, nil, &workers.WorkFailureMetadata{Family: "foreign", Type: "foreign"}) == "" {
		t.Fatal("recordedFailureDetail() returned an empty fallback")
	}
	if got := recordedFailureDetail(workersessions.FailureCauseWorkersExecutionFailure, nil, &workers.WorkFailureMetadata{Family: "", Type: workers.WorkFailureTypeTimeout}); got != "family=unknown type=timeout" {
		t.Fatalf("recordedFailureDetail(empty family) = %q", got)
	}
}

func TestRecordedObservationTimingBranches(t *testing.T) {
	base := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	clock := platformclock.NewDeterministic(base.Add(10*time.Second), time.Second)
	fact := recordedDispatchObservation{workerSessionID: "worker-1", dispatchID: "dispatch-1", startedAt: base, state: workersessions.StateCompleted}
	ended := base.Add(2 * time.Second)
	fact.endedAt = &ended
	projected := recordedObservationFromFact(fact, clock)
	if err := projected.Validate(); err != nil || projected.DurationBasis != workersessions.DurationBasisRecordedTimestamps {
		t.Fatalf("recordedObservationFromFact(terminal) = %#v, error = %v", projected, err)
	}
	active := recordedObservationFromFact(recordedDispatchObservation{workerSessionID: "worker-active", dispatchID: "dispatch-active", startedAt: base, state: workersessions.StateRunning}, clock)
	if active.DurationBasis != workersessions.DurationBasisActiveClock || active.Duration == nil {
		t.Fatalf("recordedObservationFromFact(active) = %#v", active)
	}
	backwards := recordedDispatchObservation{workerSessionID: "worker-backwards", dispatchID: "dispatch-backwards", startedAt: base, state: workersessions.StateRunning}
	ended = base.Add(-time.Second)
	backwards.endedAt = &ended
	if got := recordedObservationFromFact(backwards, clock).Duration; got == nil || *got != 0 {
		t.Fatalf("recordedObservationFromFact(backwards) duration = %#v", got)
	}
	backwards.startedAt = base.Add(20 * time.Second)
	if got := recordedObservationFromFact(backwards, clock).Duration; got == nil || *got != 0 {
		t.Fatalf("recordedObservationFromFact(active backwards) duration = %#v", got)
	}
}

func TestMergeRecordedObservationsUsesCanonicalWorkerStartTimestamp(t *testing.T) {
	recordedStarted := time.Date(2026, 8, 10, 12, 0, 0, 100, time.UTC)
	authoritativeStarted := recordedStarted.Add(500 * time.Microsecond)

	merged := mergeRecordedObservations(
		[]workersessions.Observation{{
			WorkerSessionID: "worker-1",
			StartedAt:       &recordedStarted,
		}},
		[]workersessions.Observation{{
			WorkerSessionID: "worker-1",
			StartedAt:       &authoritativeStarted,
		}},
	)
	if len(merged) != 1 || merged[0].StartedAt == nil || !merged[0].StartedAt.Equal(authoritativeStarted) {
		t.Fatalf("merged Worker Session startedAt = %#v, want canonical opening %s", merged, authoritativeStarted.Format(time.RFC3339Nano))
	}
}

func TestMergeRecordedObservationsUsesAllLiveFacts(t *testing.T) {
	t.Parallel()
	for _, state := range []workersessions.State{workersessions.StateRunning, workersessions.StateCompleted, workersessions.StateFailed} {
		t.Run(string(state), func(t *testing.T) {
			t.Parallel()
			live := workersessions.Observation{
				WorkerSessionID: "worker-1", FactorySessionID: "selected", State: state,
				ProviderSessionAvailable: false, Transcript: workersessions.TranscriptAvailabilityUnavailable,
				WorkIDs: []string{"work-1"},
			}
			recorded := live.Clone()
			recorded.State = workersessions.StateFailed
			recorded.FactorySessionID = "~default"
			recorded.ProviderSessionAvailable = true
			recorded.Failure = &workersessions.FailureCause{Kind: workersessions.FailureCauseWorkersExecutionFailure}
			historyOnly := workersessions.Observation{WorkerSessionID: "historical", State: workersessions.StateCompleted}
			merged := mergeRecordedObservations([]workersessions.Observation{recorded, recorded, historyOnly}, []workersessions.Observation{live, live})
			if len(merged) != 2 {
				t.Fatalf("merged attempts = %#v, want two unique identities", merged)
			}
			for _, got := range merged {
				if got.WorkerSessionID == "worker-1" {
					if got.State != state || got.FactorySessionID != "selected" || got.Failure != nil || got.ProviderSessionAvailable {
						t.Fatalf("merged overlap = %#v, want live facts including absent optional facts", got)
					}
					got.WorkIDs[0] = "mutated"
				}
			}
			if live.WorkIDs[0] != "work-1" || recorded.State != workersessions.StateFailed || recorded.Failure == nil {
				t.Fatal("read mutated owner observations")
			}
		})
	}
}

type selectedObservationSource struct {
	workersessions.Service
	observation workersessions.Observation
	err         error
}

func (s *selectedObservationSource) GetObservationByWorkerSessionID(context.Context, workersessions.GetObservationByWorkerSessionIDRequest) (workersessions.Observation, error) {
	return s.observation.Clone(), s.err
}

func (s *selectedObservationSource) GetObservation(context.Context, workersessions.GetObservationRequest) (workersessions.Observation, error) {
	return s.observation.Clone(), s.err
}

func TestRecordedWorkerSessionObservationConsultsLiveBeforeHistory(t *testing.T) {
	t.Parallel()
	for _, sourceErr := range []error{nil, workersessions.ErrObservationCanceled, workersessions.ErrObservationProjectionUnavailable, context.DeadlineExceeded} {
		t.Run(fmt.Sprint(sourceErr), func(t *testing.T) {
			t.Parallel()
			service := &recordedWorkerSessionObservation{
				Service: &selectedObservationSource{observation: workersessions.Observation{WorkerSessionID: "worker-1", State: workersessions.StateRunning, FactorySessionID: "selected"}, err: sourceErr},
				ledger:  &recordingfixtures.ScriptedRuntimeLedger{},
				projector: func([]interfaces.FactoryEvent, int) (interfaces.FactoryWorldState, error) {
					t.Error("history consulted despite live presence or non-absence error")
					return interfaces.FactoryWorldState{}, nil
				},
			}
			byID, err := service.GetObservationByWorkerSessionID(context.Background(), workersessions.GetObservationByWorkerSessionIDRequest{WorkerSessionID: "worker-1"})
			if !errors.Is(err, sourceErr) || (err == nil && byID.State != workersessions.StateRunning) {
				t.Fatalf("ID read = %#v, %v; want live outcome %v", byID, err, sourceErr)
			}
			byProvider, err := service.GetObservation(context.Background(), workersessions.GetObservationRequest{ProviderSession: providers.SessionRef{Provider: "codex", Kind: "session_id", ID: "provider-1"}})
			if !errors.Is(err, sourceErr) || (err == nil && byProvider.State != workersessions.StateRunning) {
				t.Fatalf("provider read = %#v, %v; want live outcome %v", byProvider, err, sourceErr)
			}
		})
	}
}

func TestRecordedWorkerSessionObservationFallsBackOnTypedAbsence(t *testing.T) {
	t.Parallel()
	base := time.Date(2026, 10, 4, 19, 34, 0, 0, time.UTC)
	service := &recordedWorkerSessionObservation{
		Service: &selectedObservationSource{err: fmt.Errorf("absent: %w", workersessions.ErrObservationSessionNotFound)},
		ledger:  &recordingfixtures.ScriptedRuntimeLedger{Events: recordedObservationTestEvents(t, base, "work-1")},
		projector: func([]interfaces.FactoryEvent, int) (interfaces.FactoryWorldState, error) {
			return interfaces.FactoryWorldState{}, nil
		},
	}
	got, err := service.GetObservationByWorkerSessionID(context.Background(), workersessions.GetObservationByWorkerSessionIDRequest{WorkerSessionID: "worker-early"})
	if err != nil || got.WorkerSessionID != "worker-early" || got.AttemptID != "dispatch-early" || got.StartedAt == nil || !got.StartedAt.Equal(base.Add(time.Second)) {
		t.Fatalf("typed-absence fallback = %#v, %v; want unchanged recorded identity and timing", got, err)
	}
}

func TestRecordedWorkerSessionObservationLiveRecordingHealth(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name               string
		state              workersessions.State
		status             recordings.WorkerRecordingStatus
		failure            *workersessions.FailureCause
		reason, wantReason string
	}{
		{"running partial capture", workersessions.StateRunning, recordings.WorkerRecordingStatusIncomplete, nil, recordings.WorkerRecordingInterruptionProcessStopped, ""},
		{"completed before flush", workersessions.StateCompleted, recordings.WorkerRecordingStatusIncomplete, nil, recordings.WorkerRecordingInterruptionProcessStopped, ""},
		{"actual process loss", workersessions.StateFailed, recordings.WorkerRecordingStatusIncomplete, &workersessions.FailureCause{Kind: workersessions.FailureCauseProcessGone}, recordings.WorkerRecordingInterruptionProcessStopped, recordings.WorkerRecordingInterruptionProcessStopped},
		{"capture fault", workersessions.StateRunning, recordings.WorkerRecordingStatusDegraded, nil, "WRITE_FAILED", "WRITE_FAILED"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got := liveRecordingHealth(workersessions.Observation{State: test.state, RecordingHealth: test.status, RecordingHealthReason: test.reason, Failure: test.failure})
			if got.State != test.state || got.RecordingHealth != test.status || got.RecordingHealthReason != test.wantReason || got.Failure != test.failure {
				t.Fatalf("live recording health = %#v, want retained facts and reason %q", got, test.wantReason)
			}
		})
	}
}

func TestRecordedWorkerSessionObservationConfirmsMatchingLiveTerminalOutcome(t *testing.T) {
	t.Parallel()
	events := recordedObservationTestEvents(t, time.Now(), "work-1")
	events = append(events, interfaces.FactoryEvent{
		Context: interfaces.FactoryEventContext{Sequence: 3, DispatchID: stringPointerForRecordedTest("dispatch-early")},
		Type:    interfaces.FactoryEventTypeDispatchResponse,
		Payload: mustMarshalRecordedTest(t, workers.DispatchResponseEventPayload{Outcome: workers.OutcomeAccepted}),
	})
	service := &recordedWorkerSessionObservation{ledger: &recordingfixtures.ScriptedRuntimeLedger{Events: events}}
	observations := []workersessions.Observation{
		{WorkerSessionID: "worker-early", AttemptID: "dispatch-early", State: workersessions.StateCompleted},
		{WorkerSessionID: "worker-early", AttemptID: "dispatch-early", State: workersessions.StateRunning},
		{WorkerSessionID: "worker-early", AttemptID: "dispatch-early", State: workersessions.StateFailed},
		{WorkerSessionID: "worker-early", AttemptID: "other-attempt", State: workersessions.StateCompleted},
	}
	service.applyConfirmation(observations, completedFlushWatermarkSample{
		available: true, generationID: "generation",
		watermark: recordings.CanonicalEventCursor{Sequence: 3, StreamGenerationID: "generation"},
	})
	if observations[0].ConfirmationState != workersessions.ConfirmationStateConfirmed || observations[0].State != workersessions.StateCompleted {
		t.Fatalf("matching terminal confirmation = %#v", observations[0])
	}
	for _, got := range observations[1:] {
		if got.ConfirmationState != workersessions.ConfirmationStateUnconfirmed || got.StateSequenceKnown {
			t.Fatalf("unmatched live state was confirmed: %#v", got)
		}
	}
}

func TestRecordedDispatchFactsBranches(t *testing.T) {
	base, events := recordedDispatchFactTestEvents(t)
	associations, requests := recordedDispatchFacts(events)
	if associations["dispatch-1"].workerSessionID != "worker-1" || len(requests["dispatch-1"].workIDs) != 1 || requests["dispatch-1"].workIDs[0] != "work-from-context" {
		t.Fatalf("recordedDispatchFacts() associations=%#v requests=%#v", associations, requests)
	}
	_, fallbackRequests := recordedDispatchFacts([]interfaces.FactoryEvent{{Context: interfaces.FactoryEventContext{EventTime: base, DispatchID: stringPointerForRecordedTest("dispatch-input")}, Type: interfaces.FactoryEventTypeDispatchRequest, Payload: mustMarshalRecordedTest(t, interfaces.DispatchRequestEventPayload{Inputs: []interfaces.DispatchConsumedWorkRef{{WorkID: "work-from-input"}}})}})
	if len(fallbackRequests["dispatch-input"].workIDs) != 1 || fallbackRequests["dispatch-input"].workIDs[0] != "work-from-input" {
		t.Fatalf("recordedDispatchFacts(payload fallback) = %#v", fallbackRequests)
	}
	modelEvents := []interfaces.FactoryEvent{{
		Context: interfaces.FactoryEventContext{
			DispatchID: stringPointerForRecordedTest("dispatch-model"),
			EventTime:  base,
		},
		Type: interfaces.FactoryEventTypeDispatchWorkerSessionAssoc,
		Payload: mustMarshalRecordedTest(t, struct {
			WorkerSessionID string `json:"workerSessionId"`
			Model           string `json:"model"`
			ReasoningEffort string `json:"reasoningEffort"`
		}{
			WorkerSessionID: "worker-model",
			Model:           "gpt-5.6-luna",
			ReasoningEffort: "high",
		}),
	}}
	modelAssociations, _ := recordedDispatchFacts(modelEvents)
	modelAssociation := modelAssociations["dispatch-model"]
	if modelAssociation.model == nil || *modelAssociation.model != "gpt-5.6-luna" || modelAssociation.reasoningEffort == nil || *modelAssociation.reasoningEffort != "high" {
		t.Fatalf("recordedDispatchFacts(model) = %#v, want resolved execution facts", modelAssociation)
	}
	modelFact := recordedDispatchFactForTest("dispatch-model", modelAssociation, nil, nil, nil, nil, nil)
	modelObservation := recordedObservationFromFact(modelFact, nil)
	if modelObservation.Model == nil || *modelObservation.Model != "gpt-5.6-luna" || modelObservation.ReasoningEffort == nil || *modelObservation.ReasoningEffort != "high" {
		t.Fatalf("recordedObservationFromFact(model) = %#v, want resolved execution facts", modelObservation)
	}
}

func TestRecordedDispatchTimingBranches(t *testing.T) {
	base, events := recordedDispatchFactTestEvents(t)
	if got := latestFactoryEventTick(events); got != 4 {
		t.Fatalf("latestFactoryEventTick() = %d, want 4", got)
	}
	index := newRecordedDispatchEventIndex(events)
	if got := index.responseTimes["dispatch-1"]; !got.Equal(base.Add(4 * time.Second)) {
		t.Fatalf("response time = %v", got)
	}
	if got := index.responseTimes["missing"]; !got.IsZero() {
		t.Fatalf("response time(missing) = %v, want zero", got)
	}
	if got := firstRecordedTime(time.Time{}, base); !got.Equal(base) || !firstRecordedTime(base.Add(time.Second), base).Equal(base.Add(time.Second)) {
		t.Fatal("firstRecordedTime() did not select primary/fallback correctly")
	}
}

func TestRecordedDispatchIdentityBranches(t *testing.T) {
	if got := firstRecordedWorkIDs([]string{"primary"}, []string{"fallback"}); len(got) != 1 || got[0] != "primary" {
		t.Fatalf("firstRecordedWorkIDs(primary) = %v", got)
	}
	if got := firstRecordedWorkIDs(nil, []string{"fallback"}); len(got) != 1 || got[0] != "fallback" {
		t.Fatalf("firstRecordedWorkIDs(fallback) = %v", got)
	}
	if firstRecordedFailure(nil, &workersessions.FailureCause{Kind: workersessions.FailureCauseWorkersExecutionFailure, Detail: "fallback"}) == nil {
		t.Fatal("firstRecordedFailure() did not select fallback")
	}
	if !containsRecordedWorkID([]string{"work-1"}, "work-1") || containsRecordedWorkID(nil, "work-1") || len(appendUniqueRecordedString([]string{"work-1"}, "work-1")) != 1 || len(appendUniqueRecordedString(nil, "work-2")) != 1 || len(appendUniqueRecordedString(nil, "")) != 0 {
		t.Fatal("recorded work ID helpers returned incorrect values")
	}
}

func TestRecordedProviderMetadataBranches(t *testing.T) {
	value := "value"
	if stringPointerValue(&value) != value || stringPointerValue(nil) != "" || len(pointerStringSlice(&[]string{"a"})) != 1 || pointerStringSlice(nil) != nil {
		t.Fatal("recorded pointer helpers returned incorrect values")
	}
	provider := providers.SessionMetadata{Provider: "codex", Kind: providers.SessionIDKind, ID: "session-1"}
	if ref := providerSessionRef(provider); ref.Provider != providers.IDCodex || ref.ID != provider.ID || cloneProviderMetadata(&provider) == nil || cloneProviderMetadata(nil) != nil {
		t.Fatal("provider metadata helpers returned incorrect values")
	}
}

func TestRecordedWorldStateBranches(t *testing.T) {
	base, events := recordedDispatchFactTestEvents(t)
	world := interfaces.FactoryWorldState{
		WorkItemsByID:       map[string]work.FactoryWorkItem{"work-map": {ID: "work-map"}},
		ActiveWorkItemsByID: map[string]work.FactoryWorkItem{"work-active": {ID: "work-active"}},
		TerminalWorkByID:    map[string]interfaces.FactoryTerminalWork{"work-terminal": {}},
		FailedWorkItemsByID: map[string]work.FactoryWorkItem{"work-failed": {ID: "work-failed"}},
	}
	for _, workID := range []string{"work-map", "work-active", "work-terminal", "work-failed", "work-from-context"} {
		if !recordedWorkExists(world, events, workID) {
			t.Fatalf("recordedWorkExists(%q) = false, want true", workID)
		}
	}
	if recordedWorkExists(world, nil, "missing") {
		t.Fatal("recordedWorkExists(missing) = true")
	}
	completedWithResponseTimeZero := interfaces.FactoryWorldDispatchCompletion{DispatchID: "dispatch-1"}
	if ended := recordedDispatchEnd(completedWithResponseTimeZero, newRecordedDispatchEventIndex(events), "dispatch-1"); ended == nil || !ended.Equal(base.Add(4*time.Second)) {
		t.Fatalf("recordedDispatchEnd(event fallback) = %v", ended)
	}
	if recordedDispatchEnd(completedWithResponseTimeZero, newRecordedDispatchEventIndex(nil), "dispatch-1") != nil {
		t.Fatal("recordedDispatchEnd(no facts) returned a value")
	}
	stateMaps := recordedDispatchStateMaps(interfaces.FactoryWorldState{CompletedDispatches: []interfaces.FactoryWorldDispatchCompletion{{DispatchID: "completed"}}, FailedDispatches: []interfaces.FactoryWorldDispatchCompletion{{DispatchID: "failed"}}})
	if _, ok := stateMaps["completed"]; !ok {
		t.Fatal("recordedDispatchStateMaps() omitted completed dispatch")
	}
	if _, ok := stateMaps["failed"]; !ok {
		t.Fatal("recordedDispatchStateMaps() omitted failed dispatch")
	}
}

func TestRecordedLiveAdapterBranches(t *testing.T) {
	live := &recordedWorkerSessionObservation{}
	if _, err := live.GetObservation(context.Background(), workersessions.GetObservationRequest{}); !errors.Is(err, workersessions.ErrInvalidObservationIdentity) {
		t.Fatalf("nil live GetObservation() error = %v, want invalid identity", err)
	}
	if _, err := live.ReadTranscript(context.Background(), workersessions.ReadTranscriptRequest{}); !errors.Is(err, workersessions.ErrInvalidObservationIdentity) {
		t.Fatalf("nil live ReadTranscript() error = %v, want invalid identity", err)
	}
	if _, err := live.StreamObservations(context.Background(), workersessions.StreamObservationsRequest{}); !errors.Is(err, workersessions.ErrObservationProjectionUnavailable) {
		t.Fatalf("nil live StreamObservations() error = %v", err)
	}
	if _, err := live.listLive(context.Background(), workersessions.ListObservationsRequest{WorkID: "work-1"}); !errors.Is(err, workersessions.ErrObservationProjectionUnavailable) {
		t.Fatalf("nil live listLive() error = %v", err)
	}
	if _, err := (*recordedWorkerSessionObservation)(nil).ListObservations(context.Background(), workersessions.ListObservationsRequest{WorkID: "work-1"}); !errors.Is(err, workersessions.ErrObservationProjectionUnavailable) {
		t.Fatalf("nil observation ListObservations() error = %v", err)
	}
	if (*factoryImpl)(nil).WorkerSessionsObservation() != nil || (&factoryImpl{}).WorkerSessionsObservation() != nil {
		t.Fatal("WorkerSessionsObservation() on nil/unconfigured runtime returned a service")
	}
	if (&factoryImpl{cfg: &runtimeConfig{}}).WorkerSessionsObservation() == nil {
		t.Fatal("WorkerSessionsObservation() on configured runtime returned nil")
	}
	resolved := "550e8400-e29b-41d4-a716-446655440000"
	observation := (&factoryImpl{cfg: &runtimeConfig{}}).WorkerSessionsObservationForSession(resolved)
	service, ok := observation.(*recordedWorkerSessionObservation)
	if !ok {
		t.Fatalf("WorkerSessionsObservationForSession() type = %T, want recordedWorkerSessionObservation", observation)
	}
	if service.factorySessionID != resolved {
		t.Fatalf("WorkerSessionsObservationForSession() factory session id = %q, want %q", service.factorySessionID, resolved)
	}
}

func recordedDispatchFactTestEvents(t *testing.T) (time.Time, []interfaces.FactoryEvent) {
	t.Helper()
	base := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	requestPayload := mustMarshalRecordedTest(t, interfaces.DispatchRequestEventPayload{Inputs: []interfaces.DispatchConsumedWorkRef{{WorkID: "work-from-input"}}})
	return base, []interfaces.FactoryEvent{
		{Context: interfaces.FactoryEventContext{Tick: 3, Sequence: 2, EventTime: base.Add(2 * time.Second), DispatchID: stringPointerForRecordedTest("dispatch-1"), WorkIDs: stringSliceForRecordedTest([]string{"work-from-context"})}, Type: interfaces.FactoryEventTypeDispatchRequest, Payload: requestPayload},
		{Context: interfaces.FactoryEventContext{Tick: 3, Sequence: 3, EventTime: base.Add(3 * time.Second), DispatchID: stringPointerForRecordedTest("dispatch-1"), RequestID: stringPointerForRecordedTest("turn-1")}, Type: interfaces.FactoryEventTypeDispatchWorkerSessionAssoc, Payload: mustMarshalRecordedTest(t, interfaces.DispatchWorkerSessionAssociationEventPayload{WorkerSessionID: "worker-1"})},
		{Context: interfaces.FactoryEventContext{Tick: 4, Sequence: 1, EventTime: base.Add(4 * time.Second), DispatchID: stringPointerForRecordedTest("dispatch-1")}, Type: interfaces.FactoryEventTypeDispatchResponse},
		{Context: interfaces.FactoryEventContext{Tick: 0, Sequence: 0, EventTime: base}, Type: interfaces.FactoryEventTypeDispatchRequest, Payload: []byte("bad")},
		{Context: interfaces.FactoryEventContext{DispatchID: stringPointerForRecordedTest("dispatch-bad")}, Type: interfaces.FactoryEventTypeDispatchWorkerSessionAssoc, Payload: []byte("bad")},
	}
}

func intPointerForRecordedTest(value int) *int { return &value }

type historicalProviderSessions struct {
	providersessions.Service
	result providersessions.ProjectResult
	err    error
}

type processLocalWorkerSessionService struct {
	workersessions.Service
	topLevelResult        workersessions.ListWorkerSessionObservationsResult
	observationListResult workersessions.ListObservationsResult
	getByWorkerResult     workersessions.Observation
}

func (s *processLocalWorkerSessionService) ListObservations(
	context.Context,
	workersessions.ListObservationsRequest,
) (workersessions.ListObservationsResult, error) {
	return s.observationListResult, nil
}

func (s *processLocalWorkerSessionService) ListWorkerSessionObservations(
	context.Context,
	workersessions.ListWorkerSessionObservationsRequest,
) (workersessions.ListWorkerSessionObservationsResult, error) {
	return s.topLevelResult, nil
}

func (s *processLocalWorkerSessionService) GetObservationByWorkerSessionID(
	context.Context,
	workersessions.GetObservationByWorkerSessionIDRequest,
) (workersessions.Observation, error) {
	return s.getByWorkerResult, nil
}

func (s *historicalProviderSessions) Project(providersessions.ProjectRequest) (providersessions.ProjectResult, error) {
	if s == nil {
		return providersessions.ProjectResult{}, providersessions.ErrSessionStorageUnavailable
	}
	return s.result, s.err
}

// recordedDispatchFactForTest indexes the supplied events for one projection.
func recordedDispatchFactForTest(
	dispatchID string,
	association recordedDispatchAssociation,
	requests map[string]recordedDispatchRequest,
	completed map[string]interfaces.FactoryWorldDispatchCompletion,
	providerSessions []interfaces.FactoryWorldProviderSessionRecord,
	active map[string]interfaces.FactoryWorldDispatch,
	events []interfaces.FactoryEvent,
) recordedDispatchObservation {
	return recordedDispatchFact(
		dispatchID, association, requests, completed, providerSessions, active,
		newRecordedDispatchEventIndex(cloneAndSortFactoryEvents(events)),
	)
}
