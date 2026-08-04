package runtime

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
)

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
