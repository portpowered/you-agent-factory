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
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	"github.com/portpowered/infinite-you/pkg/services/providers"
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
	workerSessions, err := workersessionswire.NewService(workersBoundary, events, logging.NoopLogger{}, platformclock.Real{}, unavailableProviderSessions{}, nil)
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
	sessions, err := workersessionswire.NewService(poolBoundary, events, logging.NoopLogger{}, platformclock.Real{}, unavailableProviderSessions{}, nil)
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

func TestRecordedWorkerSessionObservation_ReplaysHistoricalTerminalStream(t *testing.T) {
	base := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	workID := "work-recorded-stream"
	dispatchID := "dispatch-recorded-stream"
	providerMetadata := &workers.ProviderSessionMetadata{Provider: string(providers.IDCodex), Kind: providers.SessionIDKind, ID: "provider-session-recorded"}
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

func TestRecordedWorkerSessionObservationStreamUsesAtomicSnapshotAndLimit(t *testing.T) {
	base := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	workID := "work-recorded-race"
	dispatchID := "dispatch-recorded-race"
	providerMetadata := &workers.ProviderSessionMetadata{Provider: string(providers.IDCodex), Kind: providers.SessionIDKind, ID: "provider-session-race"}
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
	if delivery := subscription.Next(context.Background()); delivery.Kind != workersessions.ObservationDeliveryRecord || delivery.Event.SourceID != "race-association" {
		t.Fatalf("bounded retained delivery = %#v, want association after dropping the oldest retained record", delivery)
	}
	if delivery := subscription.Next(context.Background()); delivery.Kind != workersessions.ObservationDeliveryTerminalReplay || delivery.Event.SourceID != "race-response" {
		t.Fatalf("atomic terminal delivery = %#v, want TERMINAL_REPLAY from the subscription snapshot", delivery)
	}
	if delivery := subscription.Next(context.Background()); delivery.Kind != workersessions.ObservationDeliveryClosed {
		t.Fatalf("delivery after terminal snapshot = %#v, want CLOSED", delivery)
	}
}

func TestRecordedWorkerSessionObservation_UsesCanonicalFactsForExactQueries(t *testing.T) {
	fixture := newRecordedExactObservationFixture(t)
	requireRecordedExactObservationList(t, fixture)
	show := requireRecordedExactObservationShow(t, fixture)
	requireRecordedExactObservationTranscript(t, fixture, show)
	requireRecordedExactObservationStream(t, fixture)
}

type recordedExactObservationFixture struct {
	service        workersessions.Service
	ref            providers.SessionRef
	workID         string
	inputTokens    int
	transcriptText string
}

func newRecordedExactObservationFixture(t *testing.T) recordedExactObservationFixture {
	t.Helper()
	base := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	workID := "work-recorded-exact"
	dispatchID := "dispatch-recorded-exact"
	providerMetadata := &workers.ProviderSessionMetadata{Provider: string(providers.IDCodex), Kind: providers.SessionIDKind, ID: "provider-session-exact"}
	ref := providerSessionRef(*providerMetadata)
	events := []interfaces.FactoryEvent{
		{Context: interfaces.FactoryEventContext{Tick: 1, Sequence: 1, EventTime: base, DispatchID: stringPointerForRecordedTest(dispatchID), WorkIDs: stringSliceForRecordedTest([]string{workID})}, Id: "exact-request", Type: interfaces.FactoryEventTypeDispatchRequest, Payload: mustMarshalRecordedTest(t, interfaces.DispatchRequestEventPayload{TransitionID: "review"})},
		{Context: interfaces.FactoryEventContext{Tick: 1, Sequence: 2, EventTime: base.Add(time.Second), DispatchID: stringPointerForRecordedTest(dispatchID), RequestID: stringPointerForRecordedTest("turn-recorded-exact")}, Id: "exact-association", Type: interfaces.FactoryEventTypeDispatchWorkerSessionAssoc, Payload: mustMarshalRecordedTest(t, interfaces.DispatchWorkerSessionAssociationEventPayload{WorkerSessionID: "worker-recorded-exact"})},
		{Context: interfaces.FactoryEventContext{Tick: 2, Sequence: 3, EventTime: base.Add(3 * time.Second), DispatchID: stringPointerForRecordedTest(dispatchID)}, Id: "exact-response", Type: interfaces.FactoryEventTypeDispatchResponse},
	}
	inputTokens, outputTokens := 7, 5
	transcriptText := "historical transcript"
	providerProjection := &historicalProviderSessions{result: providersessions.ProjectResult{
		Session: ref,
		Detail: providersessions.Detail{
			ProviderSession: providersessions.Ref{Provider: providersessions.ProviderCodex, Kind: providers.SessionIDKind, ID: ref.ID},
			Parse:           providersessions.ParseSummary{EventCount: 3, TokenUsage: &providersessions.TokenUsage{InputTokens: &inputTokens, OutputTokens: &outputTokens}},
			Transcript:      []providersessions.TranscriptEntry{{Order: 0, Type: providersessions.TranscriptAssistantMessage, Text: &transcriptText}},
		},
	}}
	service := newRecordedWorkerSessionObservation(nil, &recordingfixtures.ScriptedRuntimeLedger{Events: events}, func(_ []interfaces.FactoryEvent, _ int) (interfaces.FactoryWorldState, error) {
		return interfaces.FactoryWorldState{CompletedDispatches: []interfaces.FactoryWorldDispatchCompletion{{
			DispatchID: dispatchID, StartedAt: base, CompletedAt: base.Add(3 * time.Second), WorkItemIDs: []string{workID},
			Result: interfaces.WorkstationResult{Outcome: string(workers.OutcomeAccepted)}, ProviderSession: providerMetadata,
		}}}, nil
	}, platformclock.NewDeterministic(base.Add(10*time.Second), time.Second), providerProjection)
	return recordedExactObservationFixture{service: service, ref: ref, workID: workID, inputTokens: inputTokens, transcriptText: transcriptText}
}

func requireRecordedExactObservationList(t *testing.T, fixture recordedExactObservationFixture) {
	t.Helper()
	listed, err := fixture.service.ListObservations(context.Background(), workersessions.ListObservationsRequest{WorkID: fixture.workID})
	if err != nil || len(listed.Observations) != 1 {
		t.Fatalf("ListObservations() = %#v, %v; want one recorded observation", listed, err)
	}
}

func requireRecordedExactObservationShow(t *testing.T, fixture recordedExactObservationFixture) workersessions.Observation {
	t.Helper()
	show, err := fixture.service.GetObservation(context.Background(), workersessions.GetObservationRequest{ProviderSession: fixture.ref})
	if err != nil {
		t.Fatalf("GetObservation() error = %v", err)
	}
	if show.WorkerSessionID != "worker-recorded-exact" || show.WorkIDs[0] != fixture.workID || show.State != workersessions.StateCompleted || show.Duration == nil || *show.Duration != 3*time.Second {
		t.Fatalf("GetObservation() = %#v, want canonical completed correlation and duration", show)
	}
	if show.TokenUsage == nil || show.TokenUsage.InputTokens == nil || *show.TokenUsage.InputTokens != fixture.inputTokens || show.Transcript != workersessions.TranscriptAvailabilityAvailable || show.Parse.EventCount != 3 {
		t.Fatalf("GetObservation() detail = %#v, want Provider Sessions enrichment", show)
	}
	return show
}

func requireRecordedExactObservationTranscript(t *testing.T, fixture recordedExactObservationFixture, show workersessions.Observation) {
	t.Helper()
	transcript, err := fixture.service.ReadTranscript(context.Background(), workersessions.ReadTranscriptRequest{ProviderSession: fixture.ref})
	if err != nil {
		t.Fatalf("ReadTranscript() error = %v", err)
	}
	if transcript.WorkerSessionID != show.WorkerSessionID || transcript.WorkIDs[0] != fixture.workID || transcript.State != workersessions.StateCompleted || len(transcript.Entries) != 1 || transcript.Entries[0].Text == nil || *transcript.Entries[0].Text != fixture.transcriptText {
		t.Fatalf("ReadTranscript() = %#v, want canonical identity and normalized entry", transcript)
	}
}

func requireRecordedExactObservationStream(t *testing.T, fixture recordedExactObservationFixture) {
	t.Helper()
	subscription, err := fixture.service.StreamObservations(context.Background(), workersessions.StreamObservationsRequest{ProviderSession: fixture.ref})
	if err != nil {
		t.Fatalf("StreamObservations() error = %v", err)
	}
	defer subscription.Close()
	for index, want := range []workersessions.ObservationDeliveryKind{workersessions.ObservationDeliveryRecord, workersessions.ObservationDeliveryRecord, workersessions.ObservationDeliveryTerminalReplay} {
		delivery := subscription.Next(context.Background())
		if delivery.Kind != want || delivery.Event.SourceID == "" || delivery.Event.SourceSequence != uint64(index+1) {
			t.Fatalf("historical delivery %d = %#v, want %s", index, delivery, want)
		}
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
	if !acceptableLiveObservationError(nil) || !acceptableLiveObservationError(workersessions.ErrObservationProjectionUnavailable) || !acceptableLiveObservationError(workersessions.ErrObservationWorkNotFound) || acceptableLiveObservationError(errors.New("other")) {
		t.Fatal("acceptableLiveObservationError() classification is incorrect")
	}
}

func TestRecordedDispatchFailureProjection(t *testing.T) {
	base := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	failureDetail := &workers.FailureDetail{Reason: workers.WorkFailureTypeAuthFailure}
	metadata := &workers.WorkFailureMetadata{Family: workers.WorkFailureFamilyRetryable, Type: workers.WorkFailureTypeTimeout}
	providerMetadata := &workers.ProviderSessionMetadata{Provider: "codex", Kind: providers.SessionIDKind, ID: "provider-session-1"}
	completed := interfaces.FactoryWorldDispatchCompletion{DispatchID: "dispatch-1", StartedAt: base, CompletedAt: base.Add(2 * time.Second), WorkItemIDs: []string{"work-1"}, Result: interfaces.WorkstationResult{Outcome: string(workers.OutcomeFailed), FailureDetail: failureDetail, FailureMetadata: metadata}, ProviderSession: providerMetadata}
	active := interfaces.FactoryWorldDispatch{DispatchID: "dispatch-1", StartedAt: base.Add(time.Second), WorkItemIDs: []string{"work-2"}}
	providerRecords := []interfaces.FactoryWorldProviderSessionRecord{{DispatchID: "dispatch-1", ProviderSession: *providerMetadata, WorkItemIDs: []string{"work-3"}, FailureDetail: failureDetail}}
	fact := recordedDispatchFact("dispatch-1", recordedDispatchAssociation{workerSessionID: "worker-1", turnID: "turn-1", eventTime: base}, map[string]recordedDispatchRequest{"dispatch-1": {workIDs: []string{"work-1"}, startedAt: base}}, map[string]interfaces.FactoryWorldDispatchCompletion{"dispatch-1": completed}, providerRecords, map[string]interfaces.FactoryWorldDispatch{"dispatch-1": active}, nil)
	if fact.state != workersessions.StateFailed || fact.provider == nil || len(fact.workIDs) != 1 || fact.failure == nil {
		t.Fatalf("recordedDispatchFact() = %#v", fact)
	}
	if recordedObservationState(string(workers.OutcomeAccepted)) != workersessions.StateCompleted || recordedObservationState(string(workers.OutcomeContinue)) != workersessions.StateCompleted || recordedObservationState(string(workers.OutcomeRejected)) != workersessions.StateFailed || recordedObservationState("unknown") != workersessions.StateFailed {
		t.Fatal("recordedObservationState() mapping is incorrect")
	}
	if recordedFailure(nil, nil, workersessions.StateRunning) != nil {
		t.Fatal("recordedFailure(active) returned a failure")
	}
}

func TestRecordedFailureMappingBranches(t *testing.T) {
	failureDetail := &workers.FailureDetail{Reason: workers.WorkFailureTypeAuthFailure}
	for _, reason := range []workers.WorkFailureType{workers.WorkFailureTypeAuthFailure, workers.WorkFailureTypeTimeout, workers.WorkFailureTypeThrottled, workers.WorkFailureTypeMisconfigured, workers.WorkFailureTypeUnknown, workers.WorkFailureTypeStructuredOutputSchemaViolation} {
		if failure := recordedFailure(failureDetail, &workers.WorkFailureMetadata{Family: workers.WorkFailureFamilyTerminal, Type: reason}, workersessions.StateFailed); failure == nil || failure.Detail == "" {
			t.Fatalf("recordedFailure(%q) = %#v", reason, failure)
		}
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
	if recordedFailureDetail(nil, nil) == "" || recordedFailureDetail(nil, &workers.WorkFailureMetadata{Family: "foreign", Type: "foreign"}) == "" {
		t.Fatal("recordedFailureDetail() returned an empty fallback")
	}
	if got := recordedFailureDetail(nil, &workers.WorkFailureMetadata{Family: "", Type: workers.WorkFailureTypeTimeout}); got != "family=unknown type=timeout" {
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
}

func TestRecordedDispatchTimingBranches(t *testing.T) {
	base, events := recordedDispatchFactTestEvents(t)
	if got := latestFactoryEventTick(events); got != 4 {
		t.Fatalf("latestFactoryEventTick() = %d, want 4", got)
	}
	if got := eventTimeForDispatch(events, "dispatch-1"); !got.Equal(base.Add(4 * time.Second)) {
		t.Fatalf("eventTimeForDispatch() = %v", got)
	}
	if got := eventTimeForDispatch(events, "missing"); !got.IsZero() {
		t.Fatalf("eventTimeForDispatch(missing) = %v, want zero", got)
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
	provider := workers.ProviderSessionMetadata{Provider: "codex", Kind: providers.SessionIDKind, ID: "session-1"}
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
	if ended := recordedDispatchEnd(completedWithResponseTimeZero, events, "dispatch-1"); ended == nil || !ended.Equal(base.Add(4*time.Second)) {
		t.Fatalf("recordedDispatchEnd(event fallback) = %v", ended)
	}
	if recordedDispatchEnd(completedWithResponseTimeZero, nil, "dispatch-1") != nil {
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

func TestRecordedObservationMergeBranches(t *testing.T) {
	liveObservation := workersessions.Observation{
		WorkerSessionID: "worker-1", ProviderSession: providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: "live-session"}, ProviderSessionAvailable: true,
		TokenUsage: &workersessions.TokenUsage{InputTokens: intPointerForRecordedTest(7)}, Transcript: workersessions.TranscriptAvailabilityAvailable, Parse: workersessions.ParseDiagnostics{EventCount: 2},
		Failure: &workersessions.FailureCause{Kind: workersessions.FailureCauseWorkersExecutionFailure, Detail: "live failure"},
	}
	merged := mergeRecordedObservations([]workersessions.Observation{{WorkerSessionID: "worker-1"}}, []workersessions.Observation{liveObservation})
	if len(merged) != 1 || !merged[0].ProviderSessionAvailable || merged[0].ProviderSession.ID != "live-session" || merged[0].TokenUsage == nil || *merged[0].TokenUsage.InputTokens != 7 || merged[0].Transcript != workersessions.TranscriptAvailabilityAvailable || merged[0].Parse.EventCount != 2 || merged[0].Failure == nil {
		t.Fatalf("mergeRecordedObservations() = %#v", merged)
	}
	if mergeRecordedObservations(nil, []workersessions.Observation{liveObservation}) != nil {
		t.Fatal("mergeRecordedObservations(empty recorded) returned non-nil")
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

func (s *historicalProviderSessions) Project(providersessions.ProjectRequest) (providersessions.ProjectResult, error) {
	if s == nil {
		return providersessions.ProjectResult{}, providersessions.ErrSessionStorageUnavailable
	}
	return s.result, s.err
}
