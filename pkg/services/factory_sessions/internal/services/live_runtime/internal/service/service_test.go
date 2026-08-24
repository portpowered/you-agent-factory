package service_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/legacysnapshot"
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

func TestServiceCancelAndTerminateUseDistinctRuntimeStopActionsAndKeepSessionInspectable(t *testing.T) {
	t.Parallel()

	const sessionID = "session-live-stop"
	runtime, session, service, stopCalls := newLiveStopControlService(t, sessionID)

	control := factorysessions.ControlRequest{
		RequestID: "control-cancel",
		TurnID:    "turn-live-stop",
		Reason:    "operator cancel",
	}
	assertAcceptedLiveStopControl(t, service, runtime, session, stopCalls, sessionID, factorysessions.LifecycleControlCancel, control, factoryruntime.WorkerSessionControlActionCancel, "cancel")

	// The runtime stop primitive is synchronous. Reset the test observation to
	// model another active session control without evicting the first session.
	runtime.state = string(factorydefinitions.FactoryStateRunning)
	terminateControl := factorysessions.ControlRequest{
		RequestID: "control-terminate",
		TurnID:    control.TurnID,
		Reason:    "operator terminate",
	}
	assertAcceptedLiveStopControl(t, service, runtime, session, stopCalls, sessionID, factorysessions.LifecycleControlTerminate, terminateControl, factoryruntime.WorkerSessionControlActionTerminate, "terminate")

	runtime.state = string(factorydefinitions.FactoryStateCompleted)
	assertRepeatedTerminalLiveStopControl(t, service, sessionID, control)
}

func newLiveStopControlService(
	t *testing.T,
	sessionID string,
) (*testFactoryRuntime, *livesession.LiveSession, liveruntime.Service, int) {
	t.Helper()
	runtime := &testFactoryRuntime{state: string(factorydefinitions.FactoryStateRunning)}
	session := &livesession.LiveSession{ID: sessionID}
	stopCalls := 0
	dependencies := testDependencies()
	dependencies.SessionFactory = func(string) (factoryruntime.Service, error) { return runtime, nil }
	dependencies.GetSession = func(id string) *livesession.LiveSession {
		if id == sessionID {
			return session
		}
		return nil
	}
	dependencies.StopSession = func(string) error {
		stopCalls++
		return nil
	}
	service, err := liveruntimewire.NewService(dependencies)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return runtime, session, service, stopCalls
}

func assertAcceptedLiveStopControl(
	t *testing.T,
	service liveruntime.Service,
	runtime *testFactoryRuntime,
	session *livesession.LiveSession,
	stopCalls int,
	sessionID string,
	kind factorysessions.LifecycleControlKind,
	control factorysessions.ControlRequest,
	action factoryruntime.WorkerSessionControlAction,
	label string,
) {
	t.Helper()
	result, err := service.ApplyControl(context.Background(), sessionID, kind, control)
	if err != nil {
		t.Fatalf("%s: %v", label, err)
	}
	if result.SessionID != sessionID || result.Outcome != factorysessions.LifecycleControlOutcomeAccepted || result.Status != factorysessions.LifecycleStatusSucceeded {
		t.Fatalf("%s result = %#v, want accepted terminal result", label, result)
	}
	requestIndex := len(runtime.terminateRequests) - 1
	if requestIndex < 0 || runtime.terminateRequests[requestIndex].WorkerSessionAction != action ||
		runtime.terminateRequests[requestIndex].TurnID != control.TurnID ||
		runtime.terminateRequests[requestIndex].ControlID != control.RequestID ||
		runtime.terminateRequests[requestIndex].Reason != control.Reason {
		t.Fatalf("%s runtime request = %#v, want %s with captured identity", label, runtime.terminateRequests, action)
	}
	if stopCalls != 0 || service.Resolve(sessionID) != session {
		t.Fatalf("%s cleanup = stop calls %d, resolved session %#v; want retained session and no registry stop", label, stopCalls, service.Resolve(sessionID))
	}
}

func assertRepeatedTerminalLiveStopControl(
	t *testing.T,
	service liveruntime.Service,
	sessionID string,
	control factorysessions.ControlRequest,
) {
	t.Helper()
	_, firstErr := service.ApplyControl(context.Background(), sessionID, factorysessions.LifecycleControlCancel, control)
	_, secondErr := service.ApplyControl(context.Background(), sessionID, factorysessions.LifecycleControlCancel, control)
	var firstControlErr, secondControlErr *factorysessions.ControlError
	if !errors.As(firstErr, &firstControlErr) || !errors.As(secondErr, &secondControlErr) {
		t.Fatalf("repeated cancel errors = %v / %v, want typed terminal errors", firstErr, secondErr)
	}
	if firstControlErr.Outcome != factorysessions.LifecycleControlOutcomeTerminalSession ||
		secondControlErr.Outcome != firstControlErr.Outcome || secondControlErr.Status != firstControlErr.Status {
		t.Fatalf("repeated cancel outcomes = %#v / %#v, want deterministic terminal rejection", firstControlErr, secondControlErr)
	}
}

func TestServiceDeleteRejectsDefaultWithoutStoppingOrRemovingIt(t *testing.T) {
	t.Parallel()

	session := &livesession.LiveSession{ID: factorysessions.DefaultSessionID, IsDefault: true}
	stopCalls := 0
	dependencies := testDependencies()
	dependencies.RequireSession = func(string) (*livesession.LiveSession, error) { return session, nil }
	dependencies.StopSession = func(string) error {
		stopCalls++
		return nil
	}
	service, err := liveruntimewire.NewService(dependencies)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	err = service.(liveruntime.DeletionService).Delete(context.Background(), factorysessions.DefaultSessionID)
	var deletionErr *factorysessions.SessionDeletionError
	if !errors.As(err, &deletionErr) || deletionErr.Reason != factorysessions.SessionDeletionReasonDefault {
		t.Fatalf("delete error = %v, want typed default-session conflict", err)
	}
	if !errors.Is(err, factorysessions.ErrSessionDeletionConflict) {
		t.Fatalf("delete error = %v, want ErrSessionDeletionConflict", err)
	}
	if stopCalls != 0 {
		t.Fatalf("stop calls = %d, want zero for default deletion refusal", stopCalls)
	}
}

func TestServiceDeleteRejectsActiveRuntimeWithoutChangingState(t *testing.T) {
	t.Parallel()

	for _, state := range []string{
		string(factorydefinitions.FactoryStateRunning),
		string(factorydefinitions.FactoryStatePaused),
		string(factorydefinitions.FactoryStateIdle),
	} {
		state := state
		t.Run(state, func(t *testing.T) {
			t.Parallel()

			runtime := &testFactoryRuntime{state: state}
			session := &livesession.LiveSession{
				ID:      "session-delete-active",
				Runtime: &factorysessions.LiveRuntime{Factory: runtime},
			}
			stopCalls := 0
			dependencies := testDependencies()
			dependencies.RequireSession = func(string) (*livesession.LiveSession, error) { return session, nil }
			dependencies.StopSession = func(string) error {
				stopCalls++
				return nil
			}
			service, err := liveruntimewire.NewService(dependencies)
			if err != nil {
				t.Fatalf("NewService: %v", err)
			}

			err = service.(liveruntime.DeletionService).Delete(context.Background(), session.ID)
			var deletionErr *factorysessions.SessionDeletionError
			if !errors.As(err, &deletionErr) || deletionErr.Reason != factorysessions.SessionDeletionReasonRuntimeActive {
				t.Fatalf("delete error = %v, want typed active-runtime conflict", err)
			}
			if deletionErr.Status != factorysessions.LifecycleStatus(state) {
				t.Fatalf("deletion status = %q, want %q", deletionErr.Status, state)
			}
			if stopCalls != 0 || runtime.state != state {
				t.Fatalf("delete changed active runtime: stop calls %d, state %q", stopCalls, runtime.state)
			}
		})
	}
}

func TestServiceDeleteRejectsInFlightLifecycleControlState(t *testing.T) {
	t.Parallel()

	for _, controlState := range []string{"PAUSING", "CANCELING", "TERMINATING"} {
		controlState := controlState
		t.Run(controlState, func(t *testing.T) {
			t.Parallel()

			runtime := &testFactoryRuntime{
				state:        string(factorydefinitions.FactoryStateCompleted),
				controlState: controlState,
			}
			session := &livesession.LiveSession{
				ID:      "session-delete-control-state",
				Runtime: &factorysessions.LiveRuntime{Factory: runtime},
			}
			stopCalls := 0
			dependencies := testDependencies()
			dependencies.RequireSession = func(string) (*livesession.LiveSession, error) { return session, nil }
			dependencies.StopSession = func(string) error {
				stopCalls++
				return nil
			}
			service, err := liveruntimewire.NewService(dependencies)
			if err != nil {
				t.Fatalf("NewService: %v", err)
			}

			err = service.(liveruntime.DeletionService).Delete(context.Background(), session.ID)
			var deletionErr *factorysessions.SessionDeletionError
			if !errors.As(err, &deletionErr) || deletionErr.Status != factorysessions.LifecycleStatus(controlState) {
				t.Fatalf("delete error = %v, want active %s conflict", err, controlState)
			}
			if stopCalls != 0 {
				t.Fatalf("stop calls = %d, want zero during %s", stopCalls, controlState)
			}
		})
	}
}

func TestServiceDeleteAfterCancelRemovesStoppedNonDefaultSession(t *testing.T) {
	t.Parallel()

	const sessionID = "session-delete-after-cancel"
	runtime := &testFactoryRuntime{state: string(factorydefinitions.FactoryStateRunning)}
	session := &livesession.LiveSession{
		ID:      sessionID,
		Runtime: &factorysessions.LiveRuntime{Factory: runtime},
	}
	stopCalls := 0
	dependencies := testDependencies()
	dependencies.RequireSession = func(string) (*livesession.LiveSession, error) { return session, nil }
	dependencies.SessionFactory = func(string) (factoryruntime.Service, error) { return runtime, nil }
	dependencies.StopSession = func(id string) error {
		if id != sessionID {
			t.Fatalf("stopped session = %q, want %q", id, sessionID)
		}
		stopCalls++
		return nil
	}
	service, err := liveruntimewire.NewService(dependencies)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	if _, err := service.ApplyControl(context.Background(), sessionID, factorysessions.LifecycleControlCancel, factorysessions.ControlRequest{RequestID: "cancel-delete"}); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if len(runtime.terminateRequests) != 1 || runtime.terminateRequests[0].WorkerSessionAction != factoryruntime.WorkerSessionControlActionCancel {
		t.Fatalf("cancel requests = %#v, want one CANCEL request", runtime.terminateRequests)
	}
	runtime.state = string(factorydefinitions.FactoryStateCompleted)

	if err := service.(liveruntime.DeletionService).Delete(context.Background(), sessionID); err != nil {
		t.Fatalf("delete after cancel: %v", err)
	}
	if stopCalls != 1 {
		t.Fatalf("stop calls = %d, want one after runtime stopped", stopCalls)
	}
}

func TestServiceLiveStopControlMissingSessionReturnsNotFound(t *testing.T) {
	t.Parallel()

	dependencies := testDependencies()
	dependencies.SessionFactory = func(string) (factoryruntime.Service, error) {
		return nil, factorysessions.ErrSessionNotFound
	}
	service, err := liveruntimewire.NewService(dependencies)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	_, err = service.ApplyControl(context.Background(), "missing-live-session", factorysessions.LifecycleControlCancel, factorysessions.ControlRequest{})
	if !errors.Is(err, factorysessions.ErrSessionNotFound) {
		t.Fatalf("missing live cancel error = %v, want ErrSessionNotFound", err)
	}
}

func TestServiceLiveStopControlMapsRuntimeFailureToTypedConflict(t *testing.T) {
	t.Parallel()

	runtimeFailure := errors.New("runtime stop rejected")
	runtime := &testFactoryRuntime{
		state:        string(factorydefinitions.FactoryStateRunning),
		terminateErr: runtimeFailure,
	}
	dependencies := testDependencies()
	dependencies.SessionFactory = func(string) (factoryruntime.Service, error) { return runtime, nil }
	service, err := liveruntimewire.NewService(dependencies)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	_, err = service.ApplyControl(context.Background(), "session-live-stop-error", factorysessions.LifecycleControlTerminate, factorysessions.ControlRequest{RequestID: "control-error"})
	var controlErr *factorysessions.ControlError
	if !errors.As(err, &controlErr) {
		t.Fatalf("terminate error = %v, want typed control error", err)
	}
	if controlErr.Outcome != factorysessions.LifecycleControlOutcomeConflict || controlErr.Status != factorysessions.LifecycleStatusRunning {
		t.Fatalf("terminate control error = %#v, want RUNNING conflict", controlErr)
	}
	if controlErr.Message == "" {
		t.Fatalf("terminate control error = %#v, want actionable message", controlErr)
	}
}

func TestServiceLiveStopControlPreservesRuntimeNoOp(t *testing.T) {
	t.Parallel()

	runtime := &testFactoryRuntime{
		state:            string(factorydefinitions.FactoryStateRunning),
		terminateOutcome: factoryruntime.ControlOutcomeNoOp,
	}
	dependencies := testDependencies()
	dependencies.SessionFactory = func(string) (factoryruntime.Service, error) { return runtime, nil }
	service, err := liveruntimewire.NewService(dependencies)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	result, err := service.ApplyControl(context.Background(), "session-live-noop", factorysessions.LifecycleControlTerminate, factorysessions.ControlRequest{RequestID: "control-noop"})
	if err != nil {
		t.Fatalf("terminate: %v", err)
	}
	if result.Outcome != factorysessions.LifecycleControlOutcomeNoOp || result.Status != factorysessions.LifecycleStatusSucceeded {
		t.Fatalf("terminate result = %#v, want runtime NO_OP with terminal status", result)
	}
}

func TestServiceRejectsRootOnlyRuntimeForLegacySnapshotPaths(t *testing.T) {
	t.Parallel()

	dependencies := testDependencies()
	dependencies.SessionFactory = func(string) (factoryruntime.Service, error) {
		return &rootOnlyRuntime{}, nil
	}
	service, err := liveruntimewire.NewService(dependencies)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	_, snapshotErr := service.Snapshot(context.Background(), "session-1")
	if snapshotErr == nil || !strings.Contains(snapshotErr.Error(), "legacy Factory Runtime observation is unavailable") {
		t.Fatalf("Snapshot error = %v, want unavailable legacy observation", snapshotErr)
	}

	_, controlErr := service.ApplyControl(
		context.Background(),
		"session-1",
		factorysessions.LifecycleControlPause,
		factorysessions.ControlRequest{},
	)
	if controlErr == nil || !strings.Contains(controlErr.Error(), "Factory Runtime observation is unavailable") {
		t.Fatalf("ApplyControl error = %v, want unavailable observation", controlErr)
	}
}

func TestServiceSnapshotUsesBoundedWorkSnapshotWhenAvailable(t *testing.T) {
	t.Parallel()

	runtime := &boundedSnapshotRuntime{}
	dependencies := testDependencies()
	dependencies.SessionFactory = func(string) (factoryruntime.Service, error) { return runtime, nil }
	service, err := liveruntimewire.NewService(dependencies)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	snapshot, err := service.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if snapshot == nil || snapshot.FactoryState != "bounded" {
		t.Fatalf("Snapshot = %#v, want bounded snapshot", snapshot)
	}
	if runtime.workSnapshotCalls != 1 || runtime.engineSnapshotCalls != 0 {
		t.Fatalf("snapshot calls = work %d, engine %d; want bounded work only", runtime.workSnapshotCalls, runtime.engineSnapshotCalls)
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

type rootOnlyRuntime struct{ factoryruntime.Service }

func (rootOnlyRuntime) Observe(context.Context, factoryruntime.ObserveRequest) (factoryruntime.ObserveResult, error) {
	return factoryruntime.ObserveResult{}, fmt.Errorf("Factory Runtime observation is unavailable")
}

type testFactoryRuntime struct {
	factoryruntime.Service
	state             string
	pauseCalls        int
	resumeCalls       int
	terminateCalls    int
	pauseRequests     []factoryruntime.PauseRequest
	resumeRequests    []factoryruntime.ResumeRequest
	terminateRequests []factoryruntime.TerminateRequest
	terminateErr      error
	terminateOutcome  factoryruntime.ControlOutcome
	controlState      string
}

type boundedSnapshotRuntime struct {
	testFactoryRuntime
	workSnapshotCalls   int
	engineSnapshotCalls int
}

func (f *boundedSnapshotRuntime) GetWorkStateSnapshot(context.Context) (*legacysnapshot.Snapshot, error) {
	f.workSnapshotCalls++
	return &legacysnapshot.Snapshot{FactoryState: "bounded"}, nil
}

func (f *boundedSnapshotRuntime) GetEngineStateSnapshot(context.Context) (*legacysnapshot.Snapshot, error) {
	f.engineSnapshotCalls++
	return &legacysnapshot.Snapshot{FactoryState: "aggregate"}, nil
}

func (f *testFactoryRuntime) Run(context.Context) error    { return nil }
func (f *testFactoryRuntime) Pause(context.Context) error  { f.pauseCalls++; return nil }
func (f *testFactoryRuntime) Resume(context.Context) error { f.resumeCalls++; return nil }
func (f *testFactoryRuntime) ControlPause(ctx context.Context, request factoryruntime.PauseRequest) (factoryruntime.PauseResult, error) {
	f.pauseRequests = append(f.pauseRequests, request)
	err := f.Pause(ctx)
	return factoryruntime.PauseResult{Outcome: factoryruntime.ControlOutcomeAccepted}, err
}
func (f *testFactoryRuntime) ControlResume(ctx context.Context, request factoryruntime.ResumeRequest) (factoryruntime.ResumeResult, error) {
	f.resumeRequests = append(f.resumeRequests, request)
	err := f.Resume(ctx)
	return factoryruntime.ResumeResult{Outcome: factoryruntime.ControlOutcomeAccepted}, err
}
func (f *testFactoryRuntime) ControlTerminate(ctx context.Context, req factoryruntime.TerminateRequest) (factoryruntime.TerminateResult, error) {
	f.terminateRequests = append(f.terminateRequests, req)
	return f.Terminate(ctx, req)
}
func (f *testFactoryRuntime) ControlWaitToComplete(factoryruntime.WaitToCompleteRequest) factoryruntime.WaitToCompleteResult {
	return factoryruntime.WaitToCompleteResult{Done: f.WaitToComplete()}
}
func (f *testFactoryRuntime) ControlMoveWork(context.Context, factoryruntime.MoveWorkRequest) (factoryruntime.MoveWorkResult, error) {
	return factoryruntime.MoveWorkResult{}, nil
}
func (f *testFactoryRuntime) Terminate(context.Context, factoryruntime.TerminateRequest) (factoryruntime.TerminateResult, error) {
	f.terminateCalls++
	outcome := f.terminateOutcome
	if outcome == "" {
		outcome = factoryruntime.ControlOutcomeAccepted
	}
	return factoryruntime.TerminateResult{Outcome: outcome}, f.terminateErr
}
func (f *testFactoryRuntime) Observe(context.Context, factoryruntime.ObserveRequest) (factoryruntime.ObserveResult, error) {
	return factoryruntime.ObserveResult{
		Observation: factoryruntime.Observation{
			Health: factoryruntime.ObservationHealth{FactoryState: f.state, LifecycleControlStatus: f.controlState},
		},
	}, nil
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
func (f *testFactoryRuntime) GetEngineStateSnapshot(context.Context) (*legacysnapshot.Snapshot, error) {
	return &legacysnapshot.Snapshot{FactoryState: f.state}, nil
}
func (f *testFactoryRuntime) MoveWork(context.Context, string, string, work.WorkStateChangeSource, string) (work.OperatorMoveResult, error) {
	return work.OperatorMoveResult{}, nil
}
