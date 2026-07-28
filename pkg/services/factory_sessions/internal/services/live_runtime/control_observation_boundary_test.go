package liveruntime_test

import (
	"context"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
	liveruntime "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/live_runtime"
	liveruntimewire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/live_runtime/wire"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

// Compile-time seal: live-runtime control/observation depends only on the
// published Factory Runtime root Service contract.
var _ factoryruntime.Service = (*rootControlObservationFake)(nil)

// rootControlObservationFake implements factoryruntime.Service using only root
// request/result vocabulary so reviewers can verify the Sessions live-runtime
// owner without importing Runtime implementation packages.
type rootControlObservationFake struct {
	factoryruntime.Service

	factoryState    string
	pauseRequests   []factoryruntime.PauseRequest
	observeRequests []factoryruntime.ObserveRequest
	pauseCalls      int
	observeCalls    int
}

func (f *rootControlObservationFake) ControlPause(
	_ context.Context,
	req factoryruntime.PauseRequest,
) (factoryruntime.PauseResult, error) {
	f.pauseRequests = append(f.pauseRequests, req)
	f.pauseCalls++
	return factoryruntime.PauseResult{Outcome: factoryruntime.ControlOutcomeAccepted}, nil
}

func (f *rootControlObservationFake) ControlResume(
	context.Context,
	factoryruntime.ResumeRequest,
) (factoryruntime.ResumeResult, error) {
	return factoryruntime.ResumeResult{Outcome: factoryruntime.ControlOutcomeAccepted}, nil
}

func (f *rootControlObservationFake) ControlTerminate(
	context.Context,
	factoryruntime.TerminateRequest,
) (factoryruntime.TerminateResult, error) {
	return factoryruntime.TerminateResult{Outcome: factoryruntime.ControlOutcomeAccepted}, nil
}

func (f *rootControlObservationFake) ControlWaitToComplete(
	factoryruntime.WaitToCompleteRequest,
) factoryruntime.WaitToCompleteResult {
	done := make(chan struct{})
	close(done)
	return factoryruntime.WaitToCompleteResult{Done: done}
}

func (f *rootControlObservationFake) ControlMoveWork(
	context.Context,
	factoryruntime.MoveWorkRequest,
) (factoryruntime.MoveWorkResult, error) {
	return factoryruntime.MoveWorkResult{}, nil
}

func (f *rootControlObservationFake) Observe(
	_ context.Context,
	req factoryruntime.ObserveRequest,
) (factoryruntime.ObserveResult, error) {
	f.observeRequests = append(f.observeRequests, req)
	f.observeCalls++
	return factoryruntime.ObserveResult{
		Observation: factoryruntime.Observation{
			Status: factoryruntime.ObservationStatusActive,
			Health: factoryruntime.ObservationHealth{
				FactoryState: f.factoryState,
			},
		},
	}, nil
}

func (f *rootControlObservationFake) PlanDispatch(
	_ context.Context,
	req factoryruntime.PlanDispatchRequest,
) (factoryruntime.PlanDispatchResult, error) {
	return factoryruntime.PlanDispatchResult{
		Outcome:       factoryruntime.DispatchPlanOutcomeAccepted,
		DispatchID:    req.DispatchID,
		CorrelationID: req.CorrelationID,
	}, nil
}

func (f *rootControlObservationFake) AcceptDispatchResult(
	_ context.Context,
	req factoryruntime.AcceptDispatchResultRequest,
) (factoryruntime.AcceptDispatchResultResult, error) {
	return factoryruntime.AcceptDispatchResultResult{
		Outcome:       factoryruntime.DispatchPlanOutcomeRetired,
		DispatchID:    req.DispatchID,
		CorrelationID: req.CorrelationID,
	}, nil
}

func (f *rootControlObservationFake) CaptureCheckpoint(
	_ context.Context,
	req factoryruntime.CaptureCheckpointRequest,
) (factoryruntime.CaptureCheckpointResult, error) {
	id := req.CheckpointID
	if id == "" {
		id = "checkpoint-stub"
	}
	return factoryruntime.CaptureCheckpointResult{
		Outcome: factoryruntime.CheckpointOutcomeCaptured,
		Checkpoint: factoryruntime.Checkpoint{
			CheckpointID:  id,
			SchemaVersion: 1,
			StrategyKind:  "runtime",
			Payload:       []byte(`{}`),
		},
	}, nil
}

func (f *rootControlObservationFake) LoadCheckpoint(
	_ context.Context,
	req factoryruntime.LoadCheckpointRequest,
) (factoryruntime.LoadCheckpointResult, error) {
	if req.CheckpointID == "" {
		return factoryruntime.LoadCheckpointResult{}, factoryruntime.ErrCheckpointNotFound
	}
	return factoryruntime.LoadCheckpointResult{}, factoryruntime.ErrCheckpointNotFound
}

func (f *rootControlObservationFake) RestoreCheckpoint(
	_ context.Context,
	req factoryruntime.RestoreCheckpointRequest,
) (factoryruntime.RestoreCheckpointResult, error) {
	return factoryruntime.RestoreCheckpointResult{
		Outcome:      factoryruntime.CheckpointOutcomeRestored,
		CheckpointID: req.Checkpoint.CheckpointID,
	}, nil
}

func (f *rootControlObservationFake) GetEngineStateSnapshot(
	context.Context,
) (*factoryruntime.StateSnapshot, error) {
	return &factoryruntime.StateSnapshot{FactoryState: f.factoryState}, nil
}

func (f *rootControlObservationFake) SubmitWorkRequest(
	context.Context,
	work.WorkRequest,
) (work.WorkRequestSubmitResult, error) {
	return work.WorkRequestSubmitResult{}, nil
}

func (f *rootControlObservationFake) SubscribeFactoryEvents(
	context.Context,
	*factorydefinitions.FactoryEventReconnectCursor,
	factorydefinitions.FactoryEventReconnectScope,
) (*factorydefinitions.FactoryEventStream, error) {
	return nil, nil
}

func TestLiveRuntimeControlPauseConstructsRootPauseRequest(t *testing.T) {
	t.Parallel()

	runtime := &rootControlObservationFake{
		factoryState: string(factorydefinitions.FactoryStateRunning),
	}
	service, err := liveruntimewire.NewService(boundaryDependencies(runtime))
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	result, err := service.ApplyControl(
		context.Background(),
		"sess-boundary-control",
		factorysessions.LifecycleControlPause,
		factorysessions.ControlRequest{},
	)
	if err != nil {
		t.Fatalf("ApplyControl: %v", err)
	}
	if result.Outcome != factorysessions.LifecycleControlOutcomeAccepted {
		t.Fatalf("outcome = %q, want ACCEPTED", result.Outcome)
	}
	if result.Status != factorysessions.LifecycleStatusPaused {
		t.Fatalf("status = %q, want PAUSED", result.Status)
	}
	if runtime.pauseCalls != 1 {
		t.Fatalf("ControlPause calls = %d, want 1", runtime.pauseCalls)
	}
	if len(runtime.pauseRequests) != 1 {
		t.Fatalf("pause requests = %d, want 1 root PauseRequest", len(runtime.pauseRequests))
	}
}

func TestLiveRuntimeObserveConstructsRootObserveRequest(t *testing.T) {
	t.Parallel()

	runtime := &rootControlObservationFake{
		factoryState: string(factorydefinitions.FactoryStateRunning),
	}
	service, err := liveruntimewire.NewService(boundaryDependencies(runtime))
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	observeRequest := factoryruntime.ObserveRequest{
		Scope: factoryruntime.ObservationScopeStatus,
	}
	result, err := service.Observe(context.Background(), "sess-boundary-observe", observeRequest)
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}
	if runtime.observeCalls != 1 {
		t.Fatalf("Observe calls = %d, want 1", runtime.observeCalls)
	}
	if len(runtime.observeRequests) != 1 || runtime.observeRequests[0].Scope != factoryruntime.ObservationScopeStatus {
		t.Fatalf("observe requests = %#v, want one STATUS-scoped ObserveRequest", runtime.observeRequests)
	}
	if result.Observation.Status != factoryruntime.ObservationStatusActive {
		t.Fatalf("observation status = %q, want ACTIVE", result.Observation.Status)
	}
	if result.Observation.Health.FactoryState != string(factorydefinitions.FactoryStateRunning) {
		t.Fatalf(
			"observation health factoryState = %q, want %q",
			result.Observation.Health.FactoryState,
			factorydefinitions.FactoryStateRunning,
		)
	}
}

func boundaryDependencies(runtime factoryruntime.Service) liveruntime.Dependencies {
	return liveruntime.Dependencies{
		OpenForTarget: func(context.Context, factorysessions.Target) (string, error) {
			return "sess-boundary", nil
		},
		ListSessionIDs: func() []string { return nil },
		GetSession:     func(string) *livesession.LiveSession { return nil },
		RequireSession: func(string) (*livesession.LiveSession, error) {
			return nil, factorysessions.ErrSessionNotFound
		},
		BuildProjectionContext: func(context.Context, *livesession.LiveSession) (factorysessions.ProjectionContext, error) {
			return factorysessions.ProjectionContext{}, nil
		},
		SessionFactory: func(string) (factoryruntime.Service, error) { return runtime, nil },
		StopSession:      func(string) error { return nil },
		ObserveControl: func(string, factorysessions.LifecycleControlKind, factorysessions.ControlRequest, factorysessions.LifecycleControlOutcome, factorysessions.LifecycleStatus, error) {
		},
	}
}
