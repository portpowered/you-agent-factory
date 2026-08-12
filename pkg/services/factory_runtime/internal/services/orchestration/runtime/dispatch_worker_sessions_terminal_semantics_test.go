package runtime

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil/recordingfixtures"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	dispatchplanning "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/dispatch_planning"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestRecordedWorkerSessionObservation_PreservesIncompleteOutputFromReplay(t *testing.T) {
	base := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	workID := "work-recorded-incomplete"
	dispatchID := "dispatch-recorded-incomplete"
	events := []interfaces.FactoryEvent{
		{
			Context: interfaces.FactoryEventContext{
				Tick: 1, Sequence: 1, EventTime: base,
				DispatchID: stringPointerForRecordedTest(dispatchID),
				WorkIDs:    stringSliceForRecordedTest([]string{workID}),
			},
			Id:      "incomplete-request",
			Type:    interfaces.FactoryEventTypeDispatchRequest,
			Payload: mustMarshalRecordedTest(t, interfaces.DispatchRequestEventPayload{}),
		},
		{
			Context: interfaces.FactoryEventContext{
				Tick: 1, Sequence: 2, EventTime: base.Add(time.Second),
				DispatchID: stringPointerForRecordedTest(dispatchID),
			},
			Id: "incomplete-association", Type: interfaces.FactoryEventTypeDispatchWorkerSessionAssoc,
			Payload: mustMarshalRecordedTest(t, interfaces.DispatchWorkerSessionAssociationEventPayload{
				WorkerSessionID: "worker-recorded-incomplete",
			}),
		},
		{
			Context: interfaces.FactoryEventContext{
				Tick: 2, Sequence: 3, EventTime: base.Add(2 * time.Second),
				DispatchID: stringPointerForRecordedTest(dispatchID),
			},
			Id: "incomplete-response", Type: interfaces.FactoryEventTypeDispatchResponse,
		},
	}
	diagnostics := &workers.SafeWorkDiagnostics{Provider: &workers.SafeProviderDiagnostic{ResponseMetadata: map[string]string{
		workers.ProviderResponseMetadataFailureOperation:      "completion_validation",
		workers.ProviderResponseMetadataFailureClassification: "missing_required_output",
	}}}
	service := newRecordedWorkerSessionObservation(
		nil,
		&recordingfixtures.ScriptedRuntimeLedger{Events: events},
		func(_ []interfaces.FactoryEvent, _ int) (interfaces.FactoryWorldState, error) {
			return interfaces.FactoryWorldState{
				WorkItemsByID: map[string]work.FactoryWorkItem{workID: {ID: workID}},
				CompletedDispatches: []interfaces.FactoryWorldDispatchCompletion{{
					DispatchID: dispatchID, StartedAt: base, CompletedAt: base.Add(2 * time.Second), WorkItemIDs: []string{workID},
					Result: interfaces.WorkstationResult{Outcome: string(workers.OutcomeFailed)}, Diagnostics: diagnostics,
				}},
			}, nil
		},
		platformclock.Real{},
		nil,
	)

	result, err := service.ListObservations(context.Background(), workersessions.ListObservationsRequest{WorkID: workID})
	if err != nil || len(result.Observations) != 1 {
		t.Fatalf("ListObservations() = %#v, %v; want one replayed observation", result, err)
	}
	observation := result.Observations[0]
	if observation.Failure == nil || observation.Failure.Kind != workersessions.FailureCauseIncompleteOutput {
		t.Fatalf("replayed observation failure = %#v, want INCOMPLETE_OUTPUT", observation.Failure)
	}
	if strings.Contains(observation.Failure.Detail, "missing_required_output") || strings.Contains(observation.Failure.Detail, "transcript") {
		t.Fatalf("replayed failure detail = %q, want bounded safe detail", observation.Failure.Detail)
	}
	if err := observation.Validate(); err != nil {
		t.Fatalf("replayed observation validation = %v", err)
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
	rejectedCompletion := completed
	rejectedCompletion.DispatchID = "dispatch-rejected"
	rejectedCompletion.Result.Outcome = string(workers.OutcomeRejected)
	rejectedFact := recordedDispatchFact(
		"dispatch-rejected",
		recordedDispatchAssociation{workerSessionID: "worker-rejected", turnID: "turn-rejected", eventTime: base},
		nil,
		map[string]interfaces.FactoryWorldDispatchCompletion{"dispatch-rejected": rejectedCompletion},
		nil,
		nil,
		nil,
	)
	rejectedObservation := recordedObservationFromFact(rejectedFact, nil)
	if rejectedObservation.State != workersessions.StateFailed || rejectedObservation.Failure == nil || rejectedObservation.Failure.Kind != workersessions.FailureCauseRejected {
		t.Fatalf("recorded rejection observation = %#v, want bounded REJECTED failure classification", rejectedObservation)
	}
	if err := rejectedObservation.Validate(); err != nil {
		t.Fatalf("recorded rejection observation validation = %v", err)
	}
	if recordedObservationState(string(workers.OutcomeAccepted)) != workersessions.StateCompleted || recordedObservationState(string(workers.OutcomeContinue)) != workersessions.StateCompleted || recordedObservationState(string(workers.OutcomeRejected)) != workersessions.StateFailed || recordedObservationState("unknown") != workersessions.StateFailed {
		t.Fatal("recordedObservationState() mapping is incorrect")
	}
	if recordedFailure(workers.OutcomeFailed, nil, nil, workersessions.StateRunning) != nil {
		t.Fatal("recordedFailure(active) returned a failure")
	}
}

func TestRecordedDispatchIncompleteOutputProjection(t *testing.T) {
	base := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	completed := interfaces.FactoryWorldDispatchCompletion{
		DispatchID: "dispatch-incomplete", StartedAt: base, CompletedAt: base.Add(2 * time.Second),
		Result: interfaces.WorkstationResult{Outcome: string(workers.OutcomeFailed)},
	}
	incompleteDiagnostics := &workers.SafeWorkDiagnostics{Provider: &workers.SafeProviderDiagnostic{ResponseMetadata: map[string]string{
		workers.ProviderResponseMetadataFailureOperation:      "completion_validation",
		workers.ProviderResponseMetadataFailureClassification: "missing_completion_evidence",
	}}}
	incomplete := completed
	incomplete.DispatchID = "dispatch-incomplete"
	incomplete.Diagnostics = incompleteDiagnostics
	incompleteFact := recordedDispatchFact(
		"dispatch-incomplete",
		recordedDispatchAssociation{workerSessionID: "worker-incomplete", eventTime: base},
		nil,
		map[string]interfaces.FactoryWorldDispatchCompletion{"dispatch-incomplete": incomplete},
		nil,
		nil,
		nil,
	)
	incompleteObservation := recordedObservationFromFact(incompleteFact, nil)
	if incompleteObservation.Failure == nil || incompleteObservation.Failure.Kind != workersessions.FailureCauseIncompleteOutput {
		t.Fatalf("recorded incomplete observation = %#v, want bounded INCOMPLETE_OUTPUT failure", incompleteObservation)
	}
}

// TestFactoryImpl_DirectAndChildDispatchPreserveIdenticalTerminalOutcomeMapping
// is the W4 story 002 cutover-preservation proof: a direct Runtime-root
// PlanDispatch and a scheduler-originated Factory child dispatch both now
// execute through worker_sessions.Service.Start (the same seam), and this
// proves that seam still classifies success, failure, and cancellation
// identically for both origins -- the cutover adds Worker Session
// observability without reclassifying a terminal outcome from callback error
// presence alone (AC: "Failed and canceled Worker Session terminal outcomes
// retain the existing Runtime failure and cancellation mapping... without
// being reclassified from callback error presence alone").
func TestFactoryImpl_DirectAndChildDispatchPreserveIdenticalTerminalOutcomeMapping(t *testing.T) {
	tests := []struct {
		name        string
		deliver     func(workers.WorkstationDispatchRequest) workers.WorkstationDispatchResult
		wantOutcome dispatchplanning.TerminalResultOutcome
	}{
		{"success", completedWorkersResult, dispatchplanning.TerminalResultOutcomeSuccess},
		{"failure", failedWorkersResult, dispatchplanning.TerminalResultOutcomeFailure},
		{"cancellation", canceledWorkersResult, dispatchplanning.TerminalResultOutcomeCancelled},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			directOutcome := directDispatchTerminalOutcome(t, tc.deliver)
			if directOutcome != tc.wantOutcome {
				t.Fatalf("direct dispatch terminal outcome = %q, want %q", directOutcome, tc.wantOutcome)
			}
			childOutcome := childDispatchTerminalOutcome(t, tc.deliver)
			if childOutcome != tc.wantOutcome {
				t.Fatalf("child dispatch terminal outcome = %q, want %q", childOutcome, tc.wantOutcome)
			}
			if directOutcome != childOutcome {
				t.Fatalf(
					"direct and child terminal outcomes diverged: direct=%q child=%q",
					directOutcome, childOutcome,
				)
			}
		})
	}
}

// directDispatchTerminalOutcome runs a Runtime-root PlanDispatch through
// worker_sessions.Service.Start with a controlled Workers boundary, delivers
// the supplied terminal Workers result, and returns the recorded canonical
// dispatch outcome.
func directDispatchTerminalOutcome(
	t *testing.T,
	deliver func(workers.WorkstationDispatchRequest) workers.WorkstationDispatchResult,
) dispatchplanning.TerminalResultOutcome {
	t.Helper()
	boundary := newControlledWorkstationBoundary()
	runtime, err := newTestFactory(
		withNet(buildSimpleNetWithFailureArc()),
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
		DispatchID:      "terminal-semantics-direct-" + t.Name(),
		CorrelationID:   "terminal-semantics-direct-corr-" + t.Name(),
		WorkIDs:         []string{"terminal-semantics-direct-work"},
		WorkstationName: "t-process",
		WorkerType:      "mock",
		ReplayKey:       "t-process/terminal-semantics-direct-trace/terminal-semantics-direct-work",
	}

	planErrCh := make(chan error, 1)
	go func() {
		_, planErr := impl.PlanDispatch(t.Context(), plan)
		planErrCh <- planErr
	}()

	request := awaitCanonicalWorkersRequest(t, boundary.requests)
	boundary.results <- deliver(request)
	requireNoRootErr(t, <-planErrCh, "PlanDispatch")

	return recordedTerminalOutcome(t, impl, plan.DispatchID)
}

// childDispatchTerminalOutcome runs a scheduler-originated Factory child
// dispatch (via SubmitWorkRequest + Run) through worker_sessions.Service.Start
// with a controlled Workers boundary, delivers the supplied terminal Workers
// result, and returns the recorded canonical dispatch outcome.
func childDispatchTerminalOutcome(
	t *testing.T,
	deliver func(workers.WorkstationDispatchRequest) workers.WorkstationDispatchResult,
) dispatchplanning.TerminalResultOutcome {
	t.Helper()
	boundary := newControlledWorkstationBoundary()
	runtime, err := newTestFactory(
		withNet(buildSimpleNetWithFailureArc()),
		withWorkerService(boundary),
		withLogger(logging.NoopLogger{}),
	)
	requireNoRootErr(t, err, "New")
	impl, ok := runtime.(*factoryImpl)
	if !ok {
		t.Fatalf("factory type = %T, want *factoryImpl", runtime)
	}
	if _, err := submitWorkRequests(t.Context(), runtime, []work.SubmitRequest{{
		WorkID: "terminal-semantics-child-" + t.Name(), WorkTypeID: "task",
		TraceID: "terminal-semantics-child-trace-" + t.Name(),
	}}); err != nil {
		t.Fatalf("SubmitWorkRequest: %v", err)
	}

	runDone := make(chan error, 1)
	go func() { runDone <- runtime.Run(t.Context()) }()

	request := awaitCanonicalWorkersRequest(t, boundary.requests)
	boundary.results <- deliver(request)

	if err := <-runDone; err != nil {
		t.Fatalf("Run: %v", err)
	}

	return recordedTerminalOutcome(t, impl, request.Execution.Dispatch.DispatchID)
}

// preHandoffFailedWorkerSessionsService is a stub worker_sessions.Service
// whose Start always terminalizes FAILED before ever reaching Workers (the
// exact shape a real FailureCauseEventPublicationFailure commit produces:
// Session.Result is set, but Dispatch/DispatchErr stay the zero value because
// no handoff occurred). It embeds the real Workers execution boundary only to
// prove that boundary is never invoked.
type preHandoffFailedWorkerSessionsService struct {
	execution workers.WorkstationExecutionService
}

func (s *preHandoffFailedWorkerSessionsService) Reserve(
	context.Context, workersessions.ReserveRequest,
) (workersessions.Session, error) {
	return workersessions.Session{}, nil
}

func (s *preHandoffFailedWorkerSessionsService) Get(
	context.Context, workersessions.GetRequest,
) (workersessions.Session, error) {
	return workersessions.Session{}, nil
}

func (s *preHandoffFailedWorkerSessionsService) List(
	context.Context, workersessions.ListRequest,
) (workersessions.ListResult, error) {
	return workersessions.ListResult{}, nil
}

func (s *preHandoffFailedWorkerSessionsService) ListObservations(
	context.Context, workersessions.ListObservationsRequest,
) (workersessions.ListObservationsResult, error) {
	return workersessions.ListObservationsResult{}, nil
}

func (s *preHandoffFailedWorkerSessionsService) GetObservation(
	context.Context, workersessions.GetObservationRequest,
) (workersessions.Observation, error) {
	return workersessions.Observation{}, nil
}

func (s *preHandoffFailedWorkerSessionsService) GetObservationByWorkerSessionID(
	context.Context, workersessions.GetObservationByWorkerSessionIDRequest,
) (workersessions.Observation, error) {
	return workersessions.Observation{}, nil
}

func (s *preHandoffFailedWorkerSessionsService) StreamObservations(
	context.Context, workersessions.StreamObservationsRequest,
) (workersessions.ObservationSubscription, error) {
	return workersessions.ObservationSubscription{}, nil
}

func (s *preHandoffFailedWorkerSessionsService) StreamObservationsByWorkerSessionID(
	context.Context, workersessions.StreamObservationsByWorkerSessionIDRequest,
) (workersessions.ObservationSubscription, error) {
	return workersessions.ObservationSubscription{}, nil
}

func (s *preHandoffFailedWorkerSessionsService) ReadTranscript(
	context.Context, workersessions.ReadTranscriptRequest,
) (workersessions.ReadTranscriptResult, error) {
	return workersessions.ReadTranscriptResult{}, nil
}

func (s *preHandoffFailedWorkerSessionsService) InvokeSession(
	_ context.Context, req workersessions.InvokeSessionRequest,
) (workersessions.InvokeSessionResult, error) {
	return workersessions.InvokeSessionResult{
		Session: workersessions.Session{
			ID:    req.ID,
			State: workersessions.StateFailed,
			Result: &workersessions.TerminalResult{
				Outcome: workersessions.TerminalOutcomeFailed,
				Cause: &workersessions.FailureCause{
					Kind:   workersessions.FailureCauseEventPublicationFailure,
					Detail: "the Worker Session opening record could not be published",
				},
			},
		},
	}, nil
}

func (s *preHandoffFailedWorkerSessionsService) Start(
	ctx context.Context, req workersessions.StartRequest,
) (workersessions.StartResult, error) {
	result, err := s.InvokeSession(ctx, workersessions.InvokeSessionRequest{
		ID:        req.ID,
		Execution: req.Execution,
		Retry:     req.Retry,
	})
	return workersessions.StartResult{Session: result.Session}, err
}

func (s *preHandoffFailedWorkerSessionsService) PublishRecord(
	context.Context, workersessions.PublishRecordRequest,
) (workersessions.PublishRecordResult, error) {
	return workersessions.PublishRecordResult{}, nil
}

func (s *preHandoffFailedWorkerSessionsService) AssociateProviderSession(
	context.Context, workersessions.ProviderSessionAssociationRequest,
) (workersessions.ProviderSessionAssociationResult, error) {
	return workersessions.ProviderSessionAssociationResult{}, nil
}

func (s *preHandoffFailedWorkerSessionsService) ObserveProviderSession(
	context.Context, workersessions.ProviderSessionObservationRequest,
) (workersessions.ProviderSessionAssociationResult, error) {
	return workersessions.ProviderSessionAssociationResult{}, nil
}

func (s *preHandoffFailedWorkerSessionsService) EnsureProviderBinding(
	context.Context, workersessions.ProviderBindingRequest,
) (workersessions.ProviderBindingResult, error) {
	return workersessions.ProviderBindingResult{}, nil
}

func (s *preHandoffFailedWorkerSessionsService) WorkerSessionIDForDispatch(
	_ context.Context, dispatchID string,
) (string, error) {
	return dispatchID, nil
}

func (s *preHandoffFailedWorkerSessionsService) Pause(context.Context, workersessions.ControlRequest) (workersessions.ControlResult, error) {
	return workersessions.ControlResult{}, nil
}

func (s *preHandoffFailedWorkerSessionsService) Resume(context.Context, workersessions.ControlRequest) (workersessions.ControlResult, error) {
	return workersessions.ControlResult{}, nil
}

func (s *preHandoffFailedWorkerSessionsService) Cancel(context.Context, workersessions.ControlRequest) (workersessions.ControlResult, error) {
	return workersessions.ControlResult{}, nil
}

func (s *preHandoffFailedWorkerSessionsService) Terminate(context.Context, workersessions.ControlRequest) (workersessions.ControlResult, error) {
	return workersessions.ControlResult{}, nil
}

// TestFactoryImpl_DirectDispatchPreHandoffWorkerSessionFailureDoesNotFabricateSuccess
// proves that when Worker Sessions terminalizes FAILED before ever reaching
// Workers (FailureCauseEventPublicationFailure -- the one FAILED cause with no
// real Dispatch payload), Runtime returns an explicit FAILURE terminal
// outcome instead of synthesizing a successful result or invoking the Workers
// executor at all.
func TestFactoryImpl_DirectDispatchPreHandoffWorkerSessionFailureDoesNotFabricateSuccess(t *testing.T) {
	executor := &recordingRootBoundaryExecutor{}
	sessions := &preHandoffFailedWorkerSessionsService{}
	runtime, err := newTestFactory(
		withNet(buildSimpleNetWithFailureArc()),
		withInlineDispatch(),
		withWorkerExecutor("mock", executor),
		withWorkerSessions(sessions),
		withLogger(logging.NoopLogger{}),
	)
	requireNoRootErr(t, err, "New")
	impl, ok := runtime.(*factoryImpl)
	if !ok {
		t.Fatalf("factory type = %T, want *factoryImpl", runtime)
	}
	impl.state = interfaces.FactoryStateRunning

	plan := factory.PlanDispatchRequest{
		DispatchID:      "pre-handoff-failure-dispatch",
		CorrelationID:   "pre-handoff-failure-corr",
		WorkIDs:         []string{"pre-handoff-failure-work"},
		WorkstationName: "t-process",
		WorkerType:      "mock",
		ReplayKey:       "t-process/pre-handoff-failure-trace/pre-handoff-failure-work",
	}
	if _, err := impl.PlanDispatch(t.Context(), plan); err != nil {
		t.Fatalf("PlanDispatch: %v", err)
	}
	if executor.calls.Load() != 0 {
		t.Fatalf(
			"Workers executor calls = %d, want 0 (pre-handoff Worker Session failure must never reach Workers)",
			executor.calls.Load(),
		)
	}
	outcome := recordedTerminalOutcome(t, impl, plan.DispatchID)
	if outcome != dispatchplanning.TerminalResultOutcomeFailure {
		t.Fatalf("terminal outcome = %q, want FAILURE (not fabricated success)", outcome)
	}
}

func recordedTerminalOutcome(
	t *testing.T,
	impl *factoryImpl,
	dispatchID string,
) dispatchplanning.TerminalResultOutcome {
	t.Helper()
	intent, ok := impl.dispatchPlan.Intent(dispatchID)
	if !ok {
		t.Fatalf("dispatch %q has no recorded intent", dispatchID)
	}
	if intent.Result == nil {
		t.Fatalf("dispatch %q has no recorded terminal result", dispatchID)
	}
	return intent.Result.Outcome
}
