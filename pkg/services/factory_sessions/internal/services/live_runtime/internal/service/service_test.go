package service_test

import (
	"context"
	"errors"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
	liveruntime "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/live_runtime"
	liveruntimewire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/live_runtime/wire"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

func TestNewServiceRequiresEveryRuntimeEffectAndStaysInert(t *testing.T) {
	t.Parallel()

	dependencies := testDependencies()
	dependencies.StopSession = nil
	if service, err := liveruntimewire.NewService(dependencies); err == nil || service != nil {
		t.Fatalf("NewService without stop effect = (%v, %v), want error", service, err)
	}

	called := false
	dependencies = testDependencies()
	dependencies.OpenForTarget = func(context.Context, factorysessions.Target) (string, error) {
		called = true
		return "", nil
	}
	service, err := liveruntimewire.NewService(dependencies)
	if err != nil || service == nil {
		t.Fatalf("NewService = (%v, %v)", service, err)
	}
	if called {
		t.Fatal("construction opened a runtime")
	}
}

func TestServiceOwnsOpenReadAndStop(t *testing.T) {
	t.Parallel()

	defaultSession := &livesession.LiveSession{ID: "default", IsDefault: true}
	namedSession := &livesession.LiveSession{ID: "named"}
	sessions := map[string]*livesession.LiveSession{"default": defaultSession, "named": namedSession}
	stopped := ""
	dependencies := testDependencies()
	dependencies.OpenForTarget = func(_ context.Context, target factorysessions.Target) (string, error) {
		if target.Ref.Kind != factorysessions.TargetKindNamed {
			t.Fatalf("target kind = %q, want named", target.Ref.Kind)
		}
		return "named", nil
	}
	dependencies.ListSessionIDs = func() []string { return []string{"named", "default"} }
	dependencies.GetSession = func(id string) *livesession.LiveSession { return sessions[id] }
	dependencies.RequireSession = func(id string) (*livesession.LiveSession, error) {
		if session := sessions[id]; session != nil {
			return session, nil
		}
		return nil, factorysessions.ErrSessionNotFound
	}
	dependencies.BuildProjectionContext = func(_ context.Context, session *livesession.LiveSession) (factorysessions.ProjectionContext, error) {
		return factorysessions.ProjectionContext{
			Session:          &factorysessions.ScopedLiveSessionSummary{ID: session.ID},
			FactorySessionID: session.ID,
		}, nil
	}
	dependencies.StopSession = func(id string) error { stopped = id; return nil }
	service, err := liveruntimewire.NewService(dependencies)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	opened, err := service.OpenForTarget(context.Background(), factorysessions.Target{Ref: factorysessions.TargetRef{Kind: factorysessions.TargetKindNamed}})
	if err != nil || opened != "named" {
		t.Fatalf("OpenForTarget = (%q, %v), want named", opened, err)
	}
	listed, err := service.List(context.Background())
	if err != nil || len(listed) != 2 || listed[0].Context.Session.ID != defaultSession.ID {
		t.Fatalf("List = (%#v, %v), want default-first sessions", listed, err)
	}
	read, err := service.Get(context.Background(), "named")
	if err != nil || read.Context.Session.ID != namedSession.ID {
		t.Fatalf("Get = (%#v, %v), want named session", read, err)
	}
	if err := service.Close(context.Background(), "named"); err != nil || stopped != "named" {
		t.Fatalf("Close = %v, stopped %q", err, stopped)
	}
}

func TestServiceCoordinatesLifecycleAndPreservesTypedRejection(t *testing.T) {
	t.Parallel()

	runtime := &testFactoryRuntime{state: string(factorydefinitions.FactoryStateRunning)}
	observed := 0
	dependencies := testDependencies()
	dependencies.SessionFactory = func(string) (factoryruntime.Service, error) { return runtime, nil }
	dependencies.ObserveControl = func(string, factorysessions.LifecycleControlKind, factorysessions.ControlRequest, factorysessions.LifecycleControlOutcome, factorysessions.LifecycleStatus, error) {
		observed++
	}
	service, err := liveruntimewire.NewService(dependencies)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	paused, err := service.ApplyControl(context.Background(), "session-1", factorysessions.LifecycleControlPause, factorysessions.ControlRequest{})
	if err != nil || paused.Status != factorysessions.LifecycleStatusPaused || runtime.pauseCalls != 1 {
		t.Fatalf("pause = (%#v, %v), calls %d", paused, err, runtime.pauseCalls)
	}
	runtime.state = string(factorydefinitions.FactoryStateCompleted)
	_, err = service.ApplyControl(context.Background(), "session-1", factorysessions.LifecycleControlResume, factorysessions.ControlRequest{})
	var controlErr *factorysessions.ControlError
	if !errors.As(err, &controlErr) || controlErr.Outcome != factorysessions.LifecycleControlOutcomeTerminalSession {
		t.Fatalf("terminal resume error = %v, want typed terminal rejection", err)
	}
	if observed != 2 {
		t.Fatalf("observations = %d, want 2", observed)
	}
}

func testDependencies() liveruntime.Dependencies {
	return liveruntime.Dependencies{
		OpenForTarget:  func(context.Context, factorysessions.Target) (string, error) { return "session", nil },
		ListSessionIDs: func() []string { return nil },
		GetSession:     func(string) *livesession.LiveSession { return nil },
		RequireSession: func(string) (*livesession.LiveSession, error) { return nil, factorysessions.ErrSessionNotFound },
		BuildProjectionContext: func(context.Context, *livesession.LiveSession) (factorysessions.ProjectionContext, error) {
			return factorysessions.ProjectionContext{}, nil
		},
		SessionFactory: func(string) (factoryruntime.Service, error) { return &testFactoryRuntime{}, nil },
		StopSession:    func(string) error { return nil },
		ObserveControl: func(string, factorysessions.LifecycleControlKind, factorysessions.ControlRequest, factorysessions.LifecycleControlOutcome, factorysessions.LifecycleStatus, error) {
		},
	}
}

type testFactoryRuntime struct {
	state      string
	pauseCalls int
}

func (f *testFactoryRuntime) Run(context.Context) error    { return nil }
func (f *testFactoryRuntime) Pause(context.Context) error  { f.pauseCalls++; return nil }
func (f *testFactoryRuntime) Resume(context.Context) error { return nil }
func (f *testFactoryRuntime) Terminate(context.Context, factoryruntime.TerminateRequest) (factoryruntime.TerminateResult, error) {
	return factoryruntime.TerminateResult{Outcome: factoryruntime.ControlOutcomeAccepted}, nil
}
func (f *testFactoryRuntime) Observe(context.Context, factoryruntime.ObserveRequest) (factoryruntime.ObserveResult, error) {
	return factoryruntime.ObserveResult{}, nil
}
func (f *testFactoryRuntime) PlanDispatch(_ context.Context, req factoryruntime.PlanDispatchRequest) (factoryruntime.PlanDispatchResult, error) {
	return factoryruntime.PlanDispatchResult{
		Outcome:       factoryruntime.DispatchPlanOutcomeAccepted,
		DispatchID:    req.DispatchID,
		CorrelationID: req.CorrelationID,
	}, nil
}
func (f *testFactoryRuntime) AcceptDispatchResult(_ context.Context, req factoryruntime.AcceptDispatchResultRequest) (factoryruntime.AcceptDispatchResultResult, error) {
	return factoryruntime.AcceptDispatchResultResult{
		Outcome:       factoryruntime.DispatchPlanOutcomeRetired,
		DispatchID:    req.DispatchID,
		CorrelationID: req.CorrelationID,
	}, nil
}
func (f *testFactoryRuntime) CaptureCheckpoint(_ context.Context, req factoryruntime.CaptureCheckpointRequest) (factoryruntime.CaptureCheckpointResult, error) {
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
func (f *testFactoryRuntime) LoadCheckpoint(_ context.Context, req factoryruntime.LoadCheckpointRequest) (factoryruntime.LoadCheckpointResult, error) {
	if req.CheckpointID == "" {
		return factoryruntime.LoadCheckpointResult{}, factoryruntime.ErrCheckpointNotFound
	}
	return factoryruntime.LoadCheckpointResult{}, factoryruntime.ErrCheckpointNotFound
}
func (f *testFactoryRuntime) RestoreCheckpoint(_ context.Context, req factoryruntime.RestoreCheckpointRequest) (factoryruntime.RestoreCheckpointResult, error) {
	return factoryruntime.RestoreCheckpointResult{
		Outcome:      factoryruntime.CheckpointOutcomeRestored,
		CheckpointID: req.Checkpoint.CheckpointID,
	}, nil
}
func (f *testFactoryRuntime) GetFactoryEvents(context.Context) ([]factorydefinitions.FactoryEvent, error) {
	return nil, nil
}
func (f *testFactoryRuntime) WaitToComplete() <-chan struct{} {
	done := make(chan struct{})
	close(done)
	return done
}
func (f *testFactoryRuntime) SubmitWorkRequest(context.Context, work.WorkRequest) (work.WorkRequestSubmitResult, error) {
	return work.WorkRequestSubmitResult{}, nil
}
func (f *testFactoryRuntime) SubscribeFactoryEvents(context.Context, *factorydefinitions.FactoryEventReconnectCursor, factorydefinitions.FactoryEventReconnectScope) (*factorydefinitions.FactoryEventStream, error) {
	return nil, nil
}
func (f *testFactoryRuntime) GetEngineStateSnapshot(context.Context) (*factoryruntime.StateSnapshot, error) {
	return &factoryruntime.StateSnapshot{FactoryState: f.state}, nil
}
func (f *testFactoryRuntime) MoveWork(context.Context, string, string, work.WorkStateChangeSource, string) (work.OperatorMoveResult, error) {
	return work.OperatorMoveResult{}, nil
}
