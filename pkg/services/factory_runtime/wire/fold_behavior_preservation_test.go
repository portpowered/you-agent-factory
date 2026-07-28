package wire_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/jonboulle/clockwork"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryruntimewire "github.com/portpowered/infinite-you/pkg/services/factory_runtime/wire"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// Fold-behavior preservation tests construct Factory Runtime exclusively through
// factory_runtime/wire and exercise control, observation, and dispatch-plan
// outcomes through the published Service root after the internal relocation.

func TestWireFoldPreservesControlPauseResumeTerminateThroughPublishedRoot(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	stub := newFoldHostedRuntimeStub(interfaces.FactoryStateRunning)
	service := wireFoldServiceWithHostedRuntime(t, stub)

	paused, err := service.ControlPause(ctx, factoryruntime.PauseRequest{})
	if err != nil || paused.Outcome != factoryruntime.ControlOutcomeAccepted {
		t.Fatalf("ControlPause(running) = (%#v, %v), want ACCEPTED", paused, err)
	}
	if stub.factoryState != interfaces.FactoryStatePaused {
		t.Fatalf("factory state after pause = %q, want PAUSED", stub.factoryState)
	}

	paused, err = service.ControlPause(ctx, factoryruntime.PauseRequest{})
	if err != nil || paused.Outcome != factoryruntime.ControlOutcomeNoOp {
		t.Fatalf("ControlPause(paused) = (%#v, %v), want NO_OP", paused, err)
	}

	resumed, err := service.ControlResume(ctx, factoryruntime.ResumeRequest{})
	if err != nil || resumed.Outcome != factoryruntime.ControlOutcomeAccepted {
		t.Fatalf("ControlResume(paused) = (%#v, %v), want ACCEPTED", resumed, err)
	}
	if stub.factoryState != interfaces.FactoryStateRunning {
		t.Fatalf("factory state after resume = %q, want RUNNING", stub.factoryState)
	}

	resumed, err = service.ControlResume(ctx, factoryruntime.ResumeRequest{})
	if err != nil || resumed.Outcome != factoryruntime.ControlOutcomeNoOp {
		t.Fatalf("ControlResume(running) = (%#v, %v), want NO_OP", resumed, err)
	}

	terminated, err := service.ControlTerminate(ctx, factoryruntime.TerminateRequest{Reason: "fold-stop"})
	if err != nil || terminated.Outcome != factoryruntime.ControlOutcomeAccepted {
		t.Fatalf("ControlTerminate(running) = (%#v, %v), want ACCEPTED", terminated, err)
	}
	if stub.terminateCalls != 1 {
		t.Fatalf("ControlTerminate calls = %d, want 1 through published Service root", stub.terminateCalls)
	}
}

func TestWireFoldPreservesObservationThroughPublishedRoot(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	stub := newFoldHostedRuntimeStub(interfaces.FactoryStateRunning)
	service := wireFoldServiceWithHostedRuntime(t, stub)

	observed, err := service.Observe(ctx, factoryruntime.ObserveRequest{
		Scope: factoryruntime.ObservationScopeStatus,
	})
	if err != nil {
		t.Fatalf("Observe(STATUS) = %v", err)
	}
	if stub.observeCalls != 1 {
		t.Fatalf("Observe calls = %d, want 1 through published Service root", stub.observeCalls)
	}
	if stub.lastObserveScope != factoryruntime.ObservationScopeStatus {
		t.Fatalf("observe scope = %q, want STATUS", stub.lastObserveScope)
	}
	if observed.Observation.Status != factoryruntime.ObservationStatusActive {
		t.Fatalf("observation status = %q, want ACTIVE", observed.Observation.Status)
	}
	if observed.Observation.Health.FactoryState != string(interfaces.FactoryStateRunning) {
		t.Fatalf(
			"observation factoryState = %q, want %q",
			observed.Observation.Health.FactoryState,
			interfaces.FactoryStateRunning,
		)
	}

	_, err = service.Observe(ctx, factoryruntime.ObserveRequest{
		Scope: factoryruntime.ObservationScope("INVALID"),
	})
	if !errors.Is(err, factoryruntime.ErrInvalidObservationScope) {
		t.Fatalf("Observe(invalid scope) error = %v, want ErrInvalidObservationScope", err)
	}
}

func TestWireFoldPreservesDispatchPlanThroughPublishedRoot(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	stub := newFoldHostedRuntimeStub(interfaces.FactoryStateRunning)
	service := wireFoldServiceWithHostedRuntime(t, stub)
	plan := factoryruntime.PlanDispatchRequest{
		DispatchID:      "fold-dispatch-1",
		CorrelationID:   "fold-corr-1",
		WorkIDs:         []string{"fold-work-1"},
		WorkstationName: "t-process",
		WorkerType:      "mock",
		ReplayKey:       "t-process/fold-trace/fold-work-1",
	}

	planned, err := service.PlanDispatch(ctx, plan)
	if err != nil || planned.Outcome != factoryruntime.DispatchPlanOutcomeAccepted {
		t.Fatalf("PlanDispatch(running) = (%#v, %v), want ACCEPTED", planned, err)
	}
	if planned.DispatchID != plan.DispatchID || planned.CorrelationID != plan.CorrelationID {
		t.Fatalf("PlanDispatch identities = (%q, %q), want (%q, %q)",
			planned.DispatchID, planned.CorrelationID, plan.DispatchID, plan.CorrelationID)
	}
	if stub.planCalls != 1 {
		t.Fatalf("PlanDispatch calls = %d, want 1 through published Service root", stub.planCalls)
	}

	duplicate, err := service.PlanDispatch(ctx, plan)
	if err != nil || duplicate.Outcome != factoryruntime.DispatchPlanOutcomeDuplicateIdempotent {
		t.Fatalf("PlanDispatch(duplicate) = (%#v, %v), want DUPLICATE_IDEMPOTENT", duplicate, err)
	}

	retired, err := service.AcceptDispatchResult(ctx, factoryruntime.AcceptDispatchResultRequest{
		DispatchID:    plan.DispatchID,
		CorrelationID: plan.CorrelationID,
		WorkID:        "fold-work-1",
		ResultOutcome: factoryruntime.DispatchResultOutcomeSuccess,
	})
	if err != nil || retired.Outcome != factoryruntime.DispatchPlanOutcomeRetired {
		t.Fatalf("AcceptDispatchResult() = (%#v, %v), want RETIRED", retired, err)
	}
	if stub.acceptCalls != 1 {
		t.Fatalf("AcceptDispatchResult calls = %d, want 1 through published Service root", stub.acceptCalls)
	}
}

func TestWireFoldRejectsActiveBindingOnNonWireRoot(t *testing.T) {
	t.Parallel()

	err := factoryruntimewire.BindActiveService(foldNonWireRoot{}, newFoldHostedRuntimeStub(interfaces.FactoryStateRunning))
	if err == nil {
		t.Fatal("BindActiveService(non-wire root) error = nil, want construction error")
	}
}

type foldNonWireRoot struct{}

func (foldNonWireRoot) ControlPause(context.Context, factoryruntime.PauseRequest) (factoryruntime.PauseResult, error) {
	return factoryruntime.PauseResult{}, nil
}
func (foldNonWireRoot) ControlResume(context.Context, factoryruntime.ResumeRequest) (factoryruntime.ResumeResult, error) {
	return factoryruntime.ResumeResult{}, nil
}
func (foldNonWireRoot) ControlTerminate(context.Context, factoryruntime.TerminateRequest) (factoryruntime.TerminateResult, error) {
	return factoryruntime.TerminateResult{}, nil
}
func (foldNonWireRoot) ControlWaitToComplete(factoryruntime.WaitToCompleteRequest) factoryruntime.WaitToCompleteResult {
	done := make(chan struct{})
	close(done)
	return factoryruntime.WaitToCompleteResult{Done: done}
}
func (foldNonWireRoot) ControlMoveWork(context.Context, factoryruntime.MoveWorkRequest) (factoryruntime.MoveWorkResult, error) {
	return factoryruntime.MoveWorkResult{}, nil
}
func (foldNonWireRoot) Observe(context.Context, factoryruntime.ObserveRequest) (factoryruntime.ObserveResult, error) {
	return factoryruntime.ObserveResult{}, nil
}
func (foldNonWireRoot) PlanDispatch(context.Context, factoryruntime.PlanDispatchRequest) (factoryruntime.PlanDispatchResult, error) {
	return factoryruntime.PlanDispatchResult{}, nil
}
func (foldNonWireRoot) AcceptDispatchResult(context.Context, factoryruntime.AcceptDispatchResultRequest) (factoryruntime.AcceptDispatchResultResult, error) {
	return factoryruntime.AcceptDispatchResultResult{}, nil
}
func (foldNonWireRoot) CaptureCheckpoint(context.Context, factoryruntime.CaptureCheckpointRequest) (factoryruntime.CaptureCheckpointResult, error) {
	return factoryruntime.CaptureCheckpointResult{}, nil
}
func (foldNonWireRoot) LoadCheckpoint(context.Context, factoryruntime.LoadCheckpointRequest) (factoryruntime.LoadCheckpointResult, error) {
	return factoryruntime.LoadCheckpointResult{}, nil
}
func (foldNonWireRoot) RestoreCheckpoint(context.Context, factoryruntime.RestoreCheckpointRequest) (factoryruntime.RestoreCheckpointResult, error) {
	return factoryruntime.RestoreCheckpointResult{}, nil
}

func wireFoldServiceWithHostedRuntime(
	t *testing.T,
	active factoryruntime.Service,
) factoryruntime.Service {
	t.Helper()

	service, err := factoryruntimewire.NewService(
		func() string { return "fold-runtime-wire-id" },
		nil,
		nil,
		clockwork.NewFakeClock(),
		func(context.Context, workers.WorkstationDispatchRequest) error { return nil },
		func(
			context.Context,
			workers.WorkstationDispatchCancelRequest,
		) (workers.WorkstationDispatchCancelResult, error) {
			return workers.WorkstationDispatchCancelResult{}, nil
		},
	)
	if err != nil {
		t.Fatalf("NewService() = %v", err)
	}
	if service == nil {
		t.Fatal("NewService() returned nil service")
	}
	if err := factoryruntimewire.BindActiveService(service, active); err != nil {
		t.Fatalf("BindActiveService() = %v", err)
	}
	return service
}

type foldHostedRuntimeStub struct {
	mu sync.Mutex

	factoryState     interfaces.FactoryState
	observeCalls     int
	planCalls        int
	acceptCalls      int
	terminateCalls   int
	lastObserveScope factoryruntime.ObservationScope
	planned          map[string]factoryruntime.PlanDispatchRequest
}

var _ factoryruntime.Service = (*foldHostedRuntimeStub)(nil)

func newFoldHostedRuntimeStub(state interfaces.FactoryState) *foldHostedRuntimeStub {
	return &foldHostedRuntimeStub{
		factoryState: state,
		planned:      make(map[string]factoryruntime.PlanDispatchRequest),
	}
}

func (s *foldHostedRuntimeStub) ControlPause(
	_ context.Context,
	_ factoryruntime.PauseRequest,
) (factoryruntime.PauseResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch s.factoryState {
	case interfaces.FactoryStatePaused:
		return factoryruntime.PauseResult{Outcome: factoryruntime.ControlOutcomeNoOp}, nil
	case interfaces.FactoryStateRunning, interfaces.FactoryStateIdle:
		s.factoryState = interfaces.FactoryStatePaused
		return factoryruntime.PauseResult{Outcome: factoryruntime.ControlOutcomeAccepted}, nil
	default:
		return factoryruntime.PauseResult{}, factoryruntime.ErrNotRunning
	}
}

func (s *foldHostedRuntimeStub) ControlResume(
	_ context.Context,
	_ factoryruntime.ResumeRequest,
) (factoryruntime.ResumeResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch s.factoryState {
	case interfaces.FactoryStateRunning, interfaces.FactoryStateIdle:
		return factoryruntime.ResumeResult{Outcome: factoryruntime.ControlOutcomeNoOp}, nil
	case interfaces.FactoryStatePaused:
		s.factoryState = interfaces.FactoryStateRunning
		return factoryruntime.ResumeResult{Outcome: factoryruntime.ControlOutcomeAccepted}, nil
	default:
		return factoryruntime.ResumeResult{}, factoryruntime.ErrNotRunning
	}
}

func (s *foldHostedRuntimeStub) ControlTerminate(
	_ context.Context,
	_ factoryruntime.TerminateRequest,
) (factoryruntime.TerminateResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.terminateCalls++
	return factoryruntime.TerminateResult{Outcome: factoryruntime.ControlOutcomeAccepted}, nil
}

func (*foldHostedRuntimeStub) ControlWaitToComplete(factoryruntime.WaitToCompleteRequest) factoryruntime.WaitToCompleteResult {
	done := make(chan struct{})
	close(done)
	return factoryruntime.WaitToCompleteResult{Done: done}
}

func (*foldHostedRuntimeStub) ControlMoveWork(
	_ context.Context,
	req factoryruntime.MoveWorkRequest,
) (factoryruntime.MoveWorkResult, error) {
	return factoryruntime.MoveWorkResult{WorkID: req.WorkID, ToState: req.StateName}, nil
}

func (s *foldHostedRuntimeStub) Observe(
	_ context.Context,
	req factoryruntime.ObserveRequest,
) (factoryruntime.ObserveResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.observeCalls++
	s.lastObserveScope = req.Scope
	return factoryruntime.ObserveResult{
		Observation: factoryruntime.Observation{
			Status: factoryruntime.ObservationStatusActive,
			Health: factoryruntime.ObservationHealth{
				FactoryState: string(s.factoryState),
			},
		},
	}, nil
}

func (s *foldHostedRuntimeStub) PlanDispatch(
	_ context.Context,
	req factoryruntime.PlanDispatchRequest,
) (factoryruntime.PlanDispatchResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.planCalls++
	if _, exists := s.planned[req.DispatchID]; exists {
		return factoryruntime.PlanDispatchResult{
			Outcome:       factoryruntime.DispatchPlanOutcomeDuplicateIdempotent,
			DispatchID:    req.DispatchID,
			CorrelationID: req.CorrelationID,
		}, nil
	}
	s.planned[req.DispatchID] = req
	return factoryruntime.PlanDispatchResult{
		Outcome:       factoryruntime.DispatchPlanOutcomeAccepted,
		DispatchID:    req.DispatchID,
		CorrelationID: req.CorrelationID,
	}, nil
}

func (s *foldHostedRuntimeStub) AcceptDispatchResult(
	_ context.Context,
	req factoryruntime.AcceptDispatchResultRequest,
) (factoryruntime.AcceptDispatchResultResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.acceptCalls++
	return factoryruntime.AcceptDispatchResultResult{
		Outcome:    factoryruntime.DispatchPlanOutcomeRetired,
		DispatchID: req.DispatchID,
	}, nil
}

func (*foldHostedRuntimeStub) CaptureCheckpoint(
	_ context.Context,
	_ factoryruntime.CaptureCheckpointRequest,
) (factoryruntime.CaptureCheckpointResult, error) {
	return factoryruntime.CaptureCheckpointResult{}, factoryruntime.ErrCapabilityUnavailable
}

func (*foldHostedRuntimeStub) LoadCheckpoint(
	_ context.Context,
	_ factoryruntime.LoadCheckpointRequest,
) (factoryruntime.LoadCheckpointResult, error) {
	return factoryruntime.LoadCheckpointResult{}, factoryruntime.ErrCapabilityUnavailable
}

func (*foldHostedRuntimeStub) RestoreCheckpoint(
	_ context.Context,
	_ factoryruntime.RestoreCheckpointRequest,
) (factoryruntime.RestoreCheckpointResult, error) {
	return factoryruntime.RestoreCheckpointResult{}, factoryruntime.ErrCapabilityUnavailable
}
