package runtime

import (
	"context"
	"testing"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	dispatchplanning "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/dispatch_planning"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

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

func (s *preHandoffFailedWorkerSessionsService) StreamObservations(
	context.Context, workersessions.StreamObservationsRequest,
) (workersessions.ObservationSubscription, error) {
	return nil, nil
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
				Cause:   &workersessions.FailureCause{Kind: workersessions.FailureCauseEventPublicationFailure},
			},
		},
	}, nil
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
