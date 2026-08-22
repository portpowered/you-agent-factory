package runtime

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil/recordingfixtures"
	"github.com/portpowered/infinite-you/internal/testutil/runtimefixtures"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	dispatchplanning "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/dispatch_planning"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// recordingRootExecutionService records the detached request at the Workers
// root so the boundary test proves Runtime calls Service.Execute directly.
type recordingRootExecutionService struct {
	*testWorkstationBoundary
	calls       atomic.Int32
	lastRequest atomic.Value
}

func (service *recordingRootExecutionService) Execute(
	_ context.Context,
	request workers.ExecuteRequest,
) (workers.ExecuteResult, error) {
	service.calls.Add(1)
	service.lastRequest.Store(request)
	return workers.ExecuteResult{
		Correlation: request.Correlation,
		Outcome:     workers.ExecutionOutcomeAccepted,
		Output: workers.ProposedOutput{
			Primary: []work.WorkContentPart{{
				Type: work.WorkContentPartTypeText,
				Text: "workers-root-boundary",
			}},
		},
	}, nil
}

// recordingRootBoundaryExecutor remains for the pool-boundary compatibility
// test below; Runtime's migrated path does not call this legacy executor.
type recordingRootBoundaryExecutor struct {
	calls          atomic.Int32
	lastDispatchID atomic.Value
}

func (executor *recordingRootBoundaryExecutor) Execute(
	_ context.Context,
	dispatch work.WorkDispatch,
) (workers.WorkResult, error) {
	executor.calls.Add(1)
	executor.lastDispatchID.Store(dispatch.DispatchID)
	return workers.WorkResult{
		DispatchID:   dispatch.DispatchID,
		TransitionID: dispatch.TransitionID,
		Outcome:      workers.OutcomeAccepted,
		Output:       "workers-root-boundary",
	}, nil
}

// TestFactoryImpl_PlanDispatchExecutesThroughStatelessWorkers proves a
// Runtime-root PlanDispatch publication reaches the detached Workers Execute
// boundary without Runtime invoking a WorkerExecutor directly.
func TestFactoryImpl_PlanDispatchExecutesThroughWorkersRootBoundary(t *testing.T) {
	service := &recordingRootExecutionService{
		testWorkstationBoundary: &testWorkstationBoundary{},
	}
	runtime, err := newTestFactory(
		withNet(buildSimpleNet()),
		withInlineDispatch(),
		withWorkerService(service),
		withLogger(logging.NoopLogger{}),
	)
	requireNoRootErr(t, err, "New")

	impl, ok := runtime.(*factoryImpl)
	if !ok {
		t.Fatalf("factory type = %T, want *factoryImpl", runtime)
	}
	impl.state = interfaces.FactoryStateRunning
	ctx := context.Background()
	plan := factory.PlanDispatchRequest{
		DispatchID:      "boundary-dispatch-1",
		CorrelationID:   "boundary-corr-1",
		WorkIDs:         []string{"boundary-work-1"},
		WorkstationName: "t-process",
		WorkerType:      "mock",
		ReplayKey:       "t-process/boundary-trace/boundary-work-1",
	}

	planned, err := impl.PlanDispatch(ctx, plan)
	requireNoRootErr(t, err, "PlanDispatch")
	if planned.Outcome != factory.DispatchPlanOutcomeAccepted {
		t.Fatalf("PlanDispatch outcome = %q, want ACCEPTED", planned.Outcome)
	}
	if service.calls.Load() != 1 {
		t.Fatalf(
			"Workers Execute calls = %d, want 1 through Workers root boundary",
			service.calls.Load(),
		)
	}
	request, ok := service.lastRequest.Load().(workers.ExecuteRequest)
	if !ok {
		t.Fatal("Workers Execute request was not recorded")
	}
	if request.Correlation.DispatchID != plan.DispatchID {
		t.Fatalf("executed dispatch correlation = %q, want %q", request.Correlation.DispatchID, plan.DispatchID)
	}
	if request.Target.WorkerType != plan.WorkerType {
		t.Fatalf("executed worker type = %q, want %q", request.Target.WorkerType, plan.WorkerType)
	}
	if request.Target.WorkstationName != plan.WorkstationName {
		t.Fatalf("executed workstation = %q, want %q", request.Target.WorkstationName, plan.WorkstationName)
	}

	accepted, err := impl.AcceptDispatchResult(ctx, factory.AcceptDispatchResultRequest{
		DispatchID:    plan.DispatchID,
		CorrelationID: plan.CorrelationID,
		WorkID:        "boundary-work-1",
		ResultOutcome: factory.DispatchResultOutcomeSuccess,
	})
	requireNoRootErr(t, err, "AcceptDispatchResult")
	if accepted.Outcome != factory.DispatchPlanOutcomeDuplicateIdempotent {
		t.Fatalf(
			"AcceptDispatchResult outcome = %q, want DUPLICATE_IDEMPOTENT after Workers completion",
			accepted.Outcome,
		)
	}
}

// TestFactoryImpl_PlannedDispatchAcceptsWorkersResultThroughRuntimeRoot proves
// a planned dispatch can be result-accepted across the Runtime/Workers boundary
// using Runtime-root AcceptDispatchResult after Workers-root execution.
func TestFactoryImpl_PlannedDispatchAcceptsWorkersResultThroughRuntimeRoot(t *testing.T) {
	boundary := newControlledWorkstationBoundary()
	executor := &countingWorkerExecutor{}
	runtime, err := newTestFactory(
		withNet(buildSimpleNet()),
		withInlineDispatch(),
		withWorkerService(boundary),
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
		DispatchID:      "planned-result-dispatch",
		CorrelationID:   "planned-result-corr",
		WorkIDs:         []string{"planned-result-work"},
		WorkstationName: "t-process",
		WorkerType:      "mock",
		ReplayKey:       "t-process/planned-result-trace/planned-result-work",
	}

	plannedCh := make(chan factory.PlanDispatchResult, 1)
	planErrCh := make(chan error, 1)
	go func() {
		planned, err := impl.PlanDispatch(t.Context(), plan)
		plannedCh <- planned
		planErrCh <- err
	}()

	request := awaitCanonicalWorkersRequest(t, boundary.requests)
	if executor.calls.Load() != 0 {
		t.Fatalf(
			"Runtime invoked executor %d times outside Workers boundary",
			executor.calls.Load(),
		)
	}

	accepted, err := impl.AcceptDispatchResult(t.Context(), factory.AcceptDispatchResultRequest{
		DispatchID:    plan.DispatchID,
		CorrelationID: plan.CorrelationID,
		WorkID:        "planned-result-work",
		ResultOutcome: factory.DispatchResultOutcomeSuccess,
	})
	requireNoRootErr(t, err, "AcceptDispatchResult")
	if accepted.Outcome != factory.DispatchPlanOutcomeRetired {
		t.Fatalf("AcceptDispatchResult outcome = %q, want RETIRED", accepted.Outcome)
	}

	boundary.results <- completedWorkersResult(request)
	requireNoRootErr(t, <-planErrCh, "PlanDispatch")
	planned := <-plannedCh
	if planned.Outcome != factory.DispatchPlanOutcomeAccepted {
		t.Fatalf("PlanDispatch outcome = %q, want ACCEPTED", planned.Outcome)
	}
}

// TestWorkersServiceExecutesRuntimePlannedRequest proves the injected Workers
// service executes a request shaped like Runtime planning publishes without
// importing nested implementation packages or assembling a pool in Runtime.
func TestWorkersServiceExecutesRuntimePlannedRequest(t *testing.T) {
	executor := &recordingRootBoundaryExecutor{}
	service := &testWorkstationBoundary{executors: map[string]workers.WorkerExecutor{"mock": executor}}

	dispatch := work.WorkDispatch{
		DispatchID:      "workers-root-dispatch",
		TransitionID:    "t-process",
		WorkerType:      "mock",
		WorkstationName: "t-process",
		Execution: work.ExecutionMetadata{
			WorkIDs:   []string{"workers-root-work"},
			ReplayKey: "t-process/workers-root-trace/workers-root-work",
		},
	}
	request := workers.WorkstationDispatchRequest{
		WorkstationName: "Process",
		Execution: workers.WorkstationExecutionRequest{
			WorkerType:       "mock",
			Dispatch:         dispatch,
			FactorySessionID: "~default",
		},
	}

	result, err := service.Execute(t.Context(), testExecuteRequestFromDispatch(request))
	requireNoRootErr(t, err, "Execute")
	if result.Outcome != workers.ExecutionOutcomeAccepted {
		t.Fatalf("execution outcome = %q, want ACCEPTED", result.Outcome)
	}
	if executor.calls.Load() != 1 {
		t.Fatalf("executor calls = %d, want 1 through Workers execution service", executor.calls.Load())
	}
	lastDispatchID, _ := executor.lastDispatchID.Load().(string)
	if lastDispatchID != dispatch.DispatchID {
		t.Fatalf("executed dispatch ID = %q, want %q", lastDispatchID, dispatch.DispatchID)
	}
}

// TestFactoryImpl_ConcurrentDuplicateCompletionResolvesExactlyOnceForDirectAndChildDispatch
// proves duplicate terminal delivery for one in-flight dispatch resolves to
// exactly one accepted Runtime terminal result for both dispatch origins: a
// Runtime-root PlanDispatch (direct) and a scheduler-originated Factory
// dispatch (child). Every losing concurrent caller observes the deterministic
// DUPLICATE_IDEMPOTENT outcome, the dispatch records one Worker Session
// association and no duplicate canonical response once the Workers callback
// that follows is released, nothing is left in flight, and both origins map the
// delivered result to the identical canonical terminal outcome.
func TestFactoryImpl_ConcurrentDuplicateCompletionResolvesExactlyOnceForDirectAndChildDispatch(t *testing.T) {
	tests := []struct {
		name     string
		deliver  func(workers.WorkstationDispatchRequest) workers.WorkstationDispatchResult
		accepted factory.DispatchResultOutcome
		want     dispatchplanning.TerminalResultOutcome
	}{
		{"success", completedWorkersResult, factory.DispatchResultOutcomeSuccess, dispatchplanning.TerminalResultOutcomeSuccess},
		{"failure", failedWorkersResult, factory.DispatchResultOutcomeFailure, dispatchplanning.TerminalResultOutcomeFailure},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			direct := concurrentDirectDispatchCompletion(t, tc.deliver, tc.accepted)
			child := concurrentChildDispatchCompletion(t, tc.deliver, tc.accepted)
			if direct != tc.want || child != tc.want || direct != child {
				t.Fatalf(
					"terminal outcomes under duplicate completion = direct:%q child:%q, want both %q",
					direct, child, tc.want,
				)
			}
		})
	}
}

// concurrentDirectDispatchCompletion parks a Runtime-root PlanDispatch inside
// the Workers boundary, races duplicate terminal acceptance against it, then
// releases the real Workers callback as a late duplicate.
func concurrentDirectDispatchCompletion(
	t *testing.T,
	deliver func(workers.WorkstationDispatchRequest) workers.WorkstationDispatchResult,
	accepted factory.DispatchResultOutcome,
) dispatchplanning.TerminalResultOutcome {
	t.Helper()
	boundary := newControlledWorkstationBoundary()
	ledger := &recordingfixtures.ScriptedRuntimeLedger{GenerationID: "concurrent-direct-completion"}
	runtime, err := newTestFactory(
		withNet(buildSimpleNetWithFailureArc()),
		withInlineDispatch(),
		withWorkerService(boundary),
		withFactoryEventHistory(ledger),
		withLogger(logging.NoopLogger{}),
	)
	requireNoRootErr(t, err, "New")
	impl := runtime.(*factoryImpl)
	impl.state = interfaces.FactoryStateRunning

	plan := factory.PlanDispatchRequest{
		DispatchID:      "concurrent-direct-" + t.Name(),
		CorrelationID:   "concurrent-direct-corr-" + t.Name(),
		WorkIDs:         []string{"concurrent-direct-work"},
		WorkstationName: "t-process",
		WorkerType:      "mock",
		ReplayKey:       "t-process/concurrent-direct-trace/concurrent-direct-work",
	}
	planErrCh := make(chan error, 1)
	go func() {
		_, planErr := impl.PlanDispatch(t.Context(), plan)
		planErrCh <- planErr
	}()

	request := awaitCanonicalWorkersRequest(t, boundary.requests)
	raceDuplicateTerminalAcceptance(t, impl, factory.AcceptDispatchResultRequest{
		DispatchID:    request.Execution.Dispatch.DispatchID,
		CorrelationID: plan.CorrelationID,
		WorkID:        plan.WorkIDs[0],
		ResultOutcome: accepted,
	})
	boundary.results <- deliver(request)
	requireNoRootErr(t, <-planErrCh, "PlanDispatch")

	requireSingleCanonicalDispatchRecord(t, impl, ledger)
	return recordedTerminalOutcome(t, impl, plan.DispatchID)
}

// concurrentChildDispatchCompletion runs the same duplicate-completion race
// against a scheduler-originated Factory child dispatch.
func concurrentChildDispatchCompletion(
	t *testing.T,
	deliver func(workers.WorkstationDispatchRequest) workers.WorkstationDispatchResult,
	accepted factory.DispatchResultOutcome,
) dispatchplanning.TerminalResultOutcome {
	t.Helper()
	boundary := newControlledWorkstationBoundary()
	ledger := &recordingfixtures.ScriptedRuntimeLedger{GenerationID: "concurrent-child-completion"}
	runtime, err := newTestFactory(
		withNet(buildSimpleNetWithFailureArc()),
		withWorkerService(boundary),
		withFactoryEventHistory(ledger),
		withLogger(logging.NoopLogger{}),
	)
	requireNoRootErr(t, err, "New")
	impl := runtime.(*factoryImpl)
	workID := "concurrent-child-work-" + t.Name()
	if _, err := submitWorkRequests(t.Context(), runtime, []work.SubmitRequest{{
		WorkID: workID, WorkTypeID: "task", TraceID: "concurrent-child-trace-" + t.Name(),
	}}); err != nil {
		t.Fatalf("SubmitWorkRequest: %v", err)
	}

	runDone := make(chan error, 1)
	go func() { runDone <- runtime.Run(t.Context()) }()

	request := awaitCanonicalWorkersRequest(t, boundary.requests)
	raceDuplicateTerminalAcceptance(t, impl, factory.AcceptDispatchResultRequest{
		DispatchID:    request.Execution.Dispatch.DispatchID,
		CorrelationID: request.Execution.Dispatch.DispatchID,
		WorkID:        workID,
		ResultOutcome: accepted,
	})
	boundary.results <- deliver(request)
	if err := <-runDone; err != nil {
		t.Fatalf("Run: %v", err)
	}

	requireSingleCanonicalDispatchRecord(t, impl, ledger)
	return recordedTerminalOutcome(t, impl, request.Execution.Dispatch.DispatchID)
}

// raceDuplicateTerminalAcceptance releases concurrentTerminalCallers duplicate
// AcceptDispatchResult callers together and requires exactly one accepted
// retirement with every other caller deterministically DUPLICATE_IDEMPOTENT.
func raceDuplicateTerminalAcceptance(t *testing.T, impl *factoryImpl, terminal factory.AcceptDispatchResultRequest) {
	t.Helper()
	const concurrentTerminalCallers = 8
	var retired, duplicate, failed atomic.Int32
	var start, done sync.WaitGroup
	start.Add(1)
	done.Add(concurrentTerminalCallers)
	for range concurrentTerminalCallers {
		go func() {
			defer done.Done()
			start.Wait()
			result, err := impl.AcceptDispatchResult(t.Context(), terminal)
			switch {
			case err != nil:
				failed.Add(1)
			case result.Outcome == factory.DispatchPlanOutcomeRetired:
				retired.Add(1)
			case result.Outcome == factory.DispatchPlanOutcomeDuplicateIdempotent:
				duplicate.Add(1)
			}
		}()
	}
	start.Done()
	done.Wait()

	if failed.Load() != 0 || retired.Load() != 1 || duplicate.Load() != concurrentTerminalCallers-1 {
		t.Fatalf(
			"duplicate terminal acceptance outcomes = retired:%d duplicate:%d errors:%d, want 1, %d and 0",
			retired.Load(), duplicate.Load(), failed.Load(), concurrentTerminalCallers-1,
		)
	}
}

// requireSingleCanonicalDispatchRecord requires the raced dispatch to have
// recorded exactly one Worker Session association and at most one canonical
// workstation response, and to leave nothing in flight -- so a duplicate
// acceptance that leaked a second terminal fact or restarted the dispatch would
// be observable in recorded history rather than only in planner bookkeeping.
func requireSingleCanonicalDispatchRecord(
	t *testing.T,
	impl *factoryImpl,
	ledger *recordingfixtures.ScriptedRuntimeLedger,
) {
	t.Helper()
	if associations := ledger.CallCount("RecordDispatchWorkerSessionAssociation"); associations != 1 {
		t.Fatalf("dispatch/Worker Session association count = %d, want exactly 1", associations)
	}
	if responses := ledger.CallCount("RecordWorkstationResponse"); responses > 1 {
		t.Fatalf("canonical workstation response count = %d, want at most 1", responses)
	}
	observed, err := impl.Observe(t.Context(), factory.ObserveRequest{Scope: factory.ObservationScopeFull})
	requireNoRootErr(t, err, "Observe")
	if observed.Observation.Progress.InFlightDispatchCount != 0 {
		t.Fatalf(
			"in-flight dispatch count after terminal acceptance = %d, want 0",
			observed.Observation.Progress.InFlightDispatchCount,
		)
	}
}

type resolvedModelAssociationLedger struct {
	*recordingfixtures.ScriptedRuntimeLedger
}

func (l *resolvedModelAssociationLedger) RecordDispatchWorkerSessionAssociationWithExecution(
	tick int,
	dispatchID string,
	workerSessionID string,
	requestID string,
	facts recordings.DispatchWorkerSessionExecutionFacts,
	eventTime time.Time,
) {
	l.ScriptedRuntimeLedger.RecordDispatchWorkerSessionAssociation(tick, dispatchID, workerSessionID, requestID, eventTime)
	payload, err := json.Marshal(struct {
		WorkerSessionID string `json:"workerSessionId"`
		Model           string `json:"model,omitempty"`
		ReasoningEffort string `json:"reasoningEffort,omitempty"`
	}{
		WorkerSessionID: workerSessionID,
		Model:           facts.Model,
		ReasoningEffort: facts.ReasoningEffort,
	})
	if err != nil {
		panic(err)
	}
	l.ScriptedRuntimeLedger.AppendRecordedEvent(interfaces.FactoryEvent{
		Context: interfaces.FactoryEventContext{
			Tick:       tick,
			EventTime:  eventTime,
			DispatchID: stringPointerForRecordedTest(dispatchID),
			RequestID:  stringPointerForRecordedTest(requestID),
		},
		Id:            "factory-event/dispatch-worker-session-association/" + dispatchID,
		Payload:       payload,
		SchemaVersion: interfaces.FactoryEventSchemaVersionV1,
		Type:          interfaces.FactoryEventTypeDispatchWorkerSessionAssoc,
	})
}

func TestFactoryImpl_PlanDispatchCapturesResolvedWorkerDefinitionFactsInObservation(t *testing.T) {
	impl, execution := newResolvedWorkerDefinitionRuntime(t)
	// This focused factory has no durable Worker recording reader; leave the
	// optional health sidecar disabled so the decorator exercises its canonical
	// association projection directly.
	impl.cfg.recordingID = ""
	impl.state = interfaces.FactoryStateRunning
	plan := factory.PlanDispatchRequest{
		DispatchID: "resolved-model-dispatch", CorrelationID: "resolved-model-correlation",
		WorkIDs: []string{"resolved-model-work"}, WorkstationName: "t-process",
		WorkerType: "definition-worker", ReplayKey: "t-process/resolved-model-trace/resolved-model-work",
	}
	planned, err := impl.PlanDispatch(t.Context(), plan)
	requireNoRootErr(t, err, "PlanDispatch")
	if planned.Outcome != factory.DispatchPlanOutcomeAccepted {
		t.Fatalf("PlanDispatch outcome = %q, want ACCEPTED", planned.Outcome)
	}
	request, ok := execution.lastRequest.Load().(workers.ExecuteRequest)
	if !ok {
		t.Fatal("Workers Execute request was not recorded")
	}
	assertResolvedWorkerRequest(t, plan, request)
	observationService := impl.WorkerSessionsObservation()
	if observationService == nil {
		t.Fatal("WorkerSessionsObservation() returned nil")
	}
	observation, err := observationService.GetObservationByWorkerSessionID(
		t.Context(),
		workersessions.GetObservationByWorkerSessionIDRequest{WorkerSessionID: plan.DispatchID},
	)
	requireNoRootErr(t, err, "GetObservationByWorkerSessionID")
	assertResolvedWorkerObservation(t, plan, request, observation)
}

func newResolvedWorkerDefinitionRuntime(t *testing.T) (*factoryImpl, *recordingRootExecutionService) {
	t.Helper()
	execution := &recordingRootExecutionService{testWorkstationBoundary: &testWorkstationBoundary{}}
	ledger := &resolvedModelAssociationLedger{ScriptedRuntimeLedger: &recordingfixtures.ScriptedRuntimeLedger{}}
	runtime, err := newTestFactory(
		withNet(buildSimpleNet()),
		withInlineDispatch(),
		withWorkerService(execution),
		withFactoryEventHistory(ledger),
		withRuntimeConfig(runtimefixtures.RuntimeDefinitionLookupFixture{
			Workstations: map[string]*interfaces.FactoryWorkstationConfig{
				"t-process": {
					Name:           "t-process",
					Type:           interfaces.WorkstationTypeModel,
					WorkerTypeName: "definition-worker",
				},
			},
			Workers: map[string]*interfaces.FactoryWorkerConfig{
				"definition-worker": {
					Name:             "definition-worker",
					Type:             interfaces.WorkerTypeModel,
					ExecutorProvider: "codex",
					ModelProvider:    "resolved-provider",
					Model:            "gpt-5.6-luna",
					ReasoningEffort:  "high",
				},
			},
		}),
		withLogger(logging.NoopLogger{}),
	)
	requireNoRootErr(t, err, "New")
	impl, ok := runtime.(*factoryImpl)
	if !ok {
		t.Fatalf("factory type = %T, want *factoryImpl", runtime)
	}
	impl.cfg.worldStateProjector = func(_ []interfaces.FactoryEvent, _ int) (interfaces.FactoryWorldState, error) {
		// The read assertion only needs the association fact; supplying a
		// projector ensures WorkerSessionsObservation exercises its recorded
		// decorator instead of falling back to the raw live service.
		return interfaces.FactoryWorldState{}, nil
	}
	return impl, execution
}

func assertResolvedWorkerRequest(t *testing.T, plan factory.PlanDispatchRequest, request workers.ExecuteRequest) {
	t.Helper()
	if request.Correlation.DispatchID != plan.DispatchID {
		t.Fatalf("executed dispatch correlation = %q, want %q", request.Correlation.DispatchID, plan.DispatchID)
	}
	if request.Target.Model.Name != "gpt-5.6-luna" {
		t.Fatalf("downstream model = %q, want resolved model", request.Target.Model.Name)
	}
	if request.Target.Model.Provider != "resolved-provider" {
		t.Fatalf("downstream provider = %q, want resolved provider", request.Target.Model.Provider)
	}
	if request.Target.Model.ReasoningEffort != "high" {
		t.Fatalf("downstream reasoning effort = %q, want high", request.Target.Model.ReasoningEffort)
	}
}

func assertResolvedWorkerObservation(
	t *testing.T,
	plan factory.PlanDispatchRequest,
	request workers.ExecuteRequest,
	observation workersessions.Observation,
) {
	t.Helper()
	if observation.WorkerSessionID != plan.DispatchID {
		t.Fatalf("observation Worker Session ID = %q, want %q", observation.WorkerSessionID, plan.DispatchID)
	}
	if observation.AttemptID == "" {
		t.Fatal("observation attempt ID is empty")
	}
	assertOptionalExecutionFact(t, "model", observation.Model, request.Target.Model.Name)
	assertOptionalExecutionFact(t, "reasoning effort", observation.ReasoningEffort, request.Target.Model.ReasoningEffort)
}

func TestRecordedObservationMergeBranches(t *testing.T) {
	liveObservation := workersessions.Observation{
		WorkerSessionID: "worker-1", ProviderSession: providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: "live-session"}, ProviderSessionAvailable: true,
		TokenUsage: &workersessions.TokenUsage{InputTokens: intPointerForRecordedTest(7)}, Transcript: workersessions.TranscriptAvailabilityAvailable, Parse: workersessions.ParseDiagnostics{EventCount: 2},
		Failure: &workersessions.FailureCause{Kind: workersessions.FailureCauseWorkersExecutionFailure, Detail: "live failure"},
	}
	recorded := []workersessions.Observation{{WorkerSessionID: "worker-1"}}
	merged := mergeRecordedObservations(recorded, []workersessions.Observation{liveObservation})
	assertMergedLiveObservation(t, merged)
	assertRecordedObservationUnchanged(t, recorded)

	liveOnly := workersessions.Observation{WorkerSessionID: "worker-live-only", WorkIDs: []string{"work-live-only"}}
	liveOnlyResult := mergeRecordedObservations([]workersessions.Observation{{WorkerSessionID: "worker-recorded-only"}}, []workersessions.Observation{liveOnly})
	assertLiveOnlyMerge(t, liveOnlyResult, liveOnly)
	liveOnlyResult[0].WorkIDs[0] = "mutated"
	if liveOnly.WorkIDs[0] != "work-live-only" {
		t.Fatal("mergeRecordedObservations(live-only) returned a source-owned WorkIDs slice")
	}
	got := mergeRecordedObservations(nil, []workersessions.Observation{liveObservation})
	if len(got) != 1 {
		t.Fatalf("mergeRecordedObservations(empty recorded) returned %d observations, want 1", len(got))
	}
	if got[0].WorkerSessionID != liveObservation.WorkerSessionID {
		t.Fatalf("empty-recorded Worker Session ID = %q, want %q", got[0].WorkerSessionID, liveObservation.WorkerSessionID)
	}
}

func assertMergedLiveObservation(t *testing.T, merged []workersessions.Observation) {
	t.Helper()
	if len(merged) != 1 {
		t.Fatalf("mergeRecordedObservations() returned %d observations, want 1", len(merged))
	}
	observation := merged[0]
	if !observation.ProviderSessionAvailable {
		t.Fatal("merged observation did not retain provider session availability")
	}
	if observation.ProviderSession.ID != "live-session" {
		t.Fatalf("merged provider session ID = %q, want live-session", observation.ProviderSession.ID)
	}
	if observation.TokenUsage == nil {
		t.Fatal("merged observation did not retain token usage")
	}
	if observation.TokenUsage.InputTokens == nil {
		t.Fatal("merged observation did not retain input token usage")
	}
	if *observation.TokenUsage.InputTokens != 7 {
		t.Fatalf("merged input tokens = %d, want 7", *observation.TokenUsage.InputTokens)
	}
	if observation.Transcript != workersessions.TranscriptAvailabilityAvailable {
		t.Fatalf("merged transcript availability = %q, want available", observation.Transcript)
	}
	if observation.Parse.EventCount != 2 {
		t.Fatalf("merged parse event count = %d, want 2", observation.Parse.EventCount)
	}
	if observation.Failure == nil {
		t.Fatal("merged observation did not retain failure")
	}
}

func assertRecordedObservationUnchanged(t *testing.T, recorded []workersessions.Observation) {
	t.Helper()
	if recorded[0].ProviderSessionAvailable {
		t.Fatal("mergeRecordedObservations mutated recorded provider session")
	}
	if recorded[0].TokenUsage != nil {
		t.Fatal("mergeRecordedObservations mutated recorded token usage")
	}
	if recorded[0].Failure != nil {
		t.Fatal("mergeRecordedObservations mutated recorded failure")
	}
}

func assertLiveOnlyMerge(t *testing.T, merged []workersessions.Observation, live workersessions.Observation) {
	t.Helper()
	if len(merged) != 2 {
		t.Fatalf("live-only merge returned %d observations, want 2", len(merged))
	}
	if merged[0].WorkerSessionID != live.WorkerSessionID {
		t.Fatalf("live-only first Worker Session ID = %q, want %q", merged[0].WorkerSessionID, live.WorkerSessionID)
	}
	if merged[1].WorkerSessionID != "worker-recorded-only" {
		t.Fatalf("live-only second Worker Session ID = %q, want worker-recorded-only", merged[1].WorkerSessionID)
	}
}

func TestRecordedObservationMergePreservesExecutionFacts(t *testing.T) {
	recordedModel := "recorded-model"
	recordedEffort := "medium"
	liveModel := "live-model"
	liveEffort := "high"
	merged := mergeRecordedObservations(
		[]workersessions.Observation{
			{WorkerSessionID: "recorded-only", Model: &recordedModel, ReasoningEffort: &recordedEffort},
			{WorkerSessionID: "overlapping"},
		},
		[]workersessions.Observation{
			{WorkerSessionID: "live-only", Model: &liveModel, ReasoningEffort: &liveEffort},
			{WorkerSessionID: "overlapping", Model: &liveModel, ReasoningEffort: &liveEffort},
		},
	)
	if len(merged) != 3 {
		t.Fatalf("execution-fact merge returned %d observations, want 3", len(merged))
	}
	byID := make(map[string]workersessions.Observation, len(merged))
	for _, observation := range merged {
		byID[observation.WorkerSessionID] = observation
	}
	assertExecutionFacts(t, "recorded-only", byID["recorded-only"], recordedModel, recordedEffort)
	assertExecutionFacts(t, "live-only", byID["live-only"], liveModel, liveEffort)
	assertExecutionFacts(t, "overlapping", byID["overlapping"], liveModel, liveEffort)

	legacy := mergeRecordedObservations(
		[]workersessions.Observation{{WorkerSessionID: "legacy"}},
		[]workersessions.Observation{{WorkerSessionID: "legacy"}},
	)
	assertExecutionFactsAbsent(t, "legacy", legacy[0])

	emptyModel := ""
	emptyEffort := ""
	retained := mergeRecordedObservations(
		[]workersessions.Observation{{WorkerSessionID: "retained", Model: &recordedModel, ReasoningEffort: &recordedEffort}},
		[]workersessions.Observation{{WorkerSessionID: "retained", Model: &emptyModel, ReasoningEffort: &emptyEffort}},
	)
	assertExecutionFacts(t, "empty live", retained[0], recordedModel, recordedEffort)
}

func assertExecutionFacts(t *testing.T, label string, observation workersessions.Observation, model, effort string) {
	t.Helper()
	if observation.WorkerSessionID == "" {
		t.Fatalf("%s observation has no Worker Session ID", label)
	}
	assertOptionalExecutionFact(t, label+" model", observation.Model, model)
	assertOptionalExecutionFact(t, label+" reasoning effort", observation.ReasoningEffort, effort)
}

func assertExecutionFactsAbsent(t *testing.T, label string, observation workersessions.Observation) {
	t.Helper()
	if observation.Model != nil {
		t.Fatalf("%s model = %q, want absent", label, *observation.Model)
	}
	if observation.ReasoningEffort != nil {
		t.Fatalf("%s reasoning effort = %q, want absent", label, *observation.ReasoningEffort)
	}
}

func assertOptionalExecutionFact(t *testing.T, label string, value *string, want string) {
	t.Helper()
	if value == nil {
		t.Fatalf("%s is absent, want %q", label, want)
	}
	if *value != want {
		t.Fatalf("%s = %q, want %q", label, *value, want)
	}
}
