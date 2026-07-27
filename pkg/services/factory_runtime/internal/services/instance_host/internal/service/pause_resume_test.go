package service

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	factoryhost "github.com/portpowered/infinite-you/pkg/services/factory_runtime/service/host"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/state"
)

type lifecycleControlFactory struct {
	*executeObserverFactory
	mu    sync.Mutex
	state interfaces.FactoryState
}

var _ factory.Service = (*lifecycleControlFactory)(nil)

func newLifecycleControlFactory(state interfaces.FactoryState) *lifecycleControlFactory {
	f := &lifecycleControlFactory{
		executeObserverFactory: &executeObserverFactory{},
		state:                  state,
	}
	f.syncEngineState()
	return f
}

func (f *lifecycleControlFactory) syncEngineState() {
	f.setEngineState(&interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		RuntimeStatus: interfaces.RuntimeStatusActive,
		FactoryState:  string(f.state),
	})
}

func (f *lifecycleControlFactory) ControlPause(
	_ context.Context,
	_ factory.PauseRequest,
) (factory.PauseResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch f.state {
	case interfaces.FactoryStatePaused:
		return factory.PauseResult{Outcome: factory.ControlOutcomeNoOp}, nil
	case interfaces.FactoryStateRunning, interfaces.FactoryStateIdle:
		f.state = interfaces.FactoryStatePaused
		f.syncEngineState()
		return factory.PauseResult{Outcome: factory.ControlOutcomeAccepted}, nil
	case interfaces.FactoryStateCompleted, interfaces.FactoryStateFailed:
		return factory.PauseResult{}, factory.ErrNotRunning
	default:
		return factory.PauseResult{}, factory.ErrInvalidLifecycleTransition
	}
}

func (f *lifecycleControlFactory) ControlResume(
	_ context.Context,
	_ factory.ResumeRequest,
) (factory.ResumeResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch f.state {
	case interfaces.FactoryStateRunning, interfaces.FactoryStateIdle:
		return factory.ResumeResult{Outcome: factory.ControlOutcomeNoOp}, nil
	case interfaces.FactoryStateCompleted, interfaces.FactoryStateFailed:
		return factory.ResumeResult{}, factory.ErrNotRunning
	case interfaces.FactoryStatePaused:
		f.state = interfaces.FactoryStateRunning
		f.syncEngineState()
		return factory.ResumeResult{Outcome: factory.ControlOutcomeAccepted}, nil
	default:
		return factory.ResumeResult{}, factory.ErrInvalidLifecycleTransition
	}
}

func (f *lifecycleControlFactory) ControlTerminate(
	_ context.Context,
	_ factory.TerminateRequest,
) (factory.TerminateResult, error) {
	return factory.TerminateResult{Outcome: factory.ControlOutcomeAccepted}, nil
}

func (*lifecycleControlFactory) ControlWaitToComplete(factory.WaitToCompleteRequest) factory.WaitToCompleteResult {
	done := make(chan struct{})
	close(done)
	return factory.WaitToCompleteResult{Done: done}
}

func (f *lifecycleControlFactory) ControlMoveWork(
	_ context.Context,
	req factory.MoveWorkRequest,
) (factory.MoveWorkResult, error) {
	return factory.MoveWorkResult{WorkID: req.WorkID, ToState: req.StateName}, nil
}

func (f *lifecycleControlFactory) Observe(
	_ context.Context,
	_ factory.ObserveRequest,
) (factory.ObserveResult, error) {
	return factory.ObserveResult{Observation: factory.Observation{Status: factory.ObservationStatusActive}}, nil
}

func (f *lifecycleControlFactory) PlanDispatch(
	_ context.Context,
	req factory.PlanDispatchRequest,
) (factory.PlanDispatchResult, error) {
	return factory.PlanDispatchResult{
		Outcome:    factory.DispatchPlanOutcomeAccepted,
		DispatchID: req.DispatchID,
	}, nil
}

func (f *lifecycleControlFactory) AcceptDispatchResult(
	_ context.Context,
	req factory.AcceptDispatchResultRequest,
) (factory.AcceptDispatchResultResult, error) {
	return factory.AcceptDispatchResultResult{
		Outcome:    factory.DispatchPlanOutcomeRetired,
		DispatchID: req.DispatchID,
	}, nil
}

func (f *lifecycleControlFactory) CaptureCheckpoint(
	_ context.Context,
	req factory.CaptureCheckpointRequest,
) (factory.CaptureCheckpointResult, error) {
	return factory.CaptureCheckpointResult{
		Outcome: factory.CheckpointOutcomeCaptured,
		Checkpoint: factory.Checkpoint{
			CheckpointID: req.CheckpointID, SchemaVersion: 1, Payload: []byte(`{}`),
		},
	}, nil
}

func (f *lifecycleControlFactory) LoadCheckpoint(
	_ context.Context,
	req factory.LoadCheckpointRequest,
) (factory.LoadCheckpointResult, error) {
	return factory.LoadCheckpointResult{
		Outcome:    factory.CheckpointOutcomeLoaded,
		Checkpoint: factory.Checkpoint{CheckpointID: req.CheckpointID, SchemaVersion: 1, Payload: []byte(`{}`)},
	}, nil
}

func (f *lifecycleControlFactory) RestoreCheckpoint(
	_ context.Context,
	req factory.RestoreCheckpointRequest,
) (factory.RestoreCheckpointResult, error) {
	return factory.RestoreCheckpointResult{
		Outcome:      factory.CheckpointOutcomeRestored,
		CheckpointID: req.Checkpoint.CheckpointID,
	}, nil
}

func startReadyHostedHandle(
	t *testing.T,
	host *Host,
	factoryStub factory.Factory,
	instanceID string,
) factory.HostedHandle {
	t.Helper()
	factoryStub.(*lifecycleControlFactory).setEngineState(&interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		RuntimeStatus: interfaces.RuntimeStatusActive,
		FactoryState:  string(interfaces.FactoryStateRunning),
	})
	bundle := testBundle(factoryStub, instanceID)
	ctx := context.Background()
	handle, err := host.Start(ctx, bundle)
	if err != nil || handle == nil {
		t.Fatalf("Start() = (%v, %v), want hosted handle", handle, err)
	}
	if err := host.WaitForStart(ctx, handle); err != nil {
		t.Fatalf("WaitForStart() error = %v", err)
	}
	return handle
}

func TestPauseRunningHostedInstanceReturnsAcceptedAndLeavesPaused(t *testing.T) {
	t.Parallel()

	host := newTestHost(t)
	factoryStub := newLifecycleControlFactory(interfaces.FactoryStateRunning)
	handle := startReadyHostedHandle(t, host, factoryStub, "runtime-pause-running")

	result, err := host.Pause(context.Background(), handle)
	if err != nil || result.Outcome != factory.ControlOutcomeAccepted {
		t.Fatalf("Pause() = (%#v, %v), want ACCEPTED", result, err)
	}
	if factoryStub.state != interfaces.FactoryStatePaused {
		t.Fatalf("factory state = %q, want PAUSED", factoryStub.state)
	}
	if len(host.handles) != 1 {
		t.Fatalf("handles after pause = %d, want single active handle", len(host.handles))
	}
}

func TestPauseAlreadyPausedReturnsNoOpWithoutChangingHandle(t *testing.T) {
	t.Parallel()

	host := newTestHost(t)
	factoryStub := newLifecycleControlFactory(interfaces.FactoryStateRunning)
	handle := startReadyHostedHandle(t, host, factoryStub, "runtime-pause-noop")

	if _, err := host.Pause(context.Background(), handle); err != nil {
		t.Fatalf("first Pause() error = %v", err)
	}
	second, err := host.Pause(context.Background(), handle)
	if err != nil || second.Outcome != factory.ControlOutcomeNoOp {
		t.Fatalf("second Pause() = (%#v, %v), want NO_OP", second, err)
	}
	if handle != host.handles["runtime-pause-noop"] {
		t.Fatal("pause no-op changed active handle identity")
	}
}

func TestResumePausedHostedInstanceReturnsAcceptedAndRestoresRunning(t *testing.T) {
	t.Parallel()

	host := newTestHost(t)
	factoryStub := newLifecycleControlFactory(interfaces.FactoryStateRunning)
	handle := startReadyHostedHandle(t, host, factoryStub, "runtime-resume-paused")

	if _, err := host.Pause(context.Background(), handle); err != nil {
		t.Fatalf("Pause() error = %v", err)
	}
	result, err := host.Resume(context.Background(), handle)
	if err != nil || result.Outcome != factory.ControlOutcomeAccepted {
		t.Fatalf("Resume() = (%#v, %v), want ACCEPTED", result, err)
	}
	if factoryStub.state != interfaces.FactoryStateRunning {
		t.Fatalf("factory state = %q, want RUNNING", factoryStub.state)
	}
}

func TestResumeAlreadyRunningReturnsNoOp(t *testing.T) {
	t.Parallel()

	host := newTestHost(t)
	factoryStub := newLifecycleControlFactory(interfaces.FactoryStateRunning)
	handle := startReadyHostedHandle(t, host, factoryStub, "runtime-resume-noop")

	first, err := host.Resume(context.Background(), handle)
	if err != nil || first.Outcome != factory.ControlOutcomeNoOp {
		t.Fatalf("Resume(running) = (%#v, %v), want NO_OP", first, err)
	}
	second, err := host.Resume(context.Background(), handle)
	if err != nil || second.Outcome != factory.ControlOutcomeNoOp {
		t.Fatalf("second Resume() = (%#v, %v), want NO_OP", second, err)
	}
}

func TestPauseResumeRejectStoppedFailedAndUnknownStates(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		state interfaces.FactoryState
		op    string
	}{
		{name: "pause completed", state: interfaces.FactoryStateCompleted, op: "pause"},
		{name: "resume failed", state: interfaces.FactoryStateFailed, op: "resume"},
		{name: "pause unknown", state: interfaces.FactoryState("unknown"), op: "pause"},
		{name: "resume unknown", state: interfaces.FactoryState("weird"), op: "resume"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			host := newTestHost(t)
			factoryStub := newLifecycleControlFactory(interfaces.FactoryStateRunning)
			handle := startReadyHostedHandle(t, host, factoryStub, "runtime-reject-"+tc.name)

			factoryStub.mu.Lock()
			factoryStub.state = tc.state
			factoryStub.syncEngineState()
			factoryStub.mu.Unlock()

			ctx := context.Background()
			var opErr error
			switch tc.op {
			case "pause":
				_, opErr = host.Pause(ctx, handle)
			case "resume":
				_, opErr = host.Resume(ctx, handle)
			}
			if !errors.Is(opErr, factory.ErrNotRunning) && !errors.Is(opErr, factory.ErrInvalidLifecycleTransition) {
				t.Fatalf("%s error = %v, want typed lifecycle rejection", tc.op, opErr)
			}
			if len(host.handles) != 1 {
				t.Fatalf("handles after rejection = %d, want no second handle", len(host.handles))
			}
		})
	}
}

func TestPauseResumeRejectInvalidAndUnregisteredHandles(t *testing.T) {
	t.Parallel()

	host := newTestHost(t)
	ctx := context.Background()

	_, err := host.Pause(ctx, nil)
	if err == nil || !strings.Contains(err.Error(), "requires a runtime handle") {
		t.Fatalf("Pause(nil) error = %v, want runtime-handle validation error", err)
	}
	_, err = host.Resume(ctx, nil)
	if err == nil || !strings.Contains(err.Error(), "requires a runtime handle") {
		t.Fatalf("Resume(nil) error = %v, want runtime-handle validation error", err)
	}

	factoryStub := newLifecycleControlFactory(interfaces.FactoryStateRunning)
	handle := startReadyHostedHandle(t, host, factoryStub, "runtime-unregistered")
	host.removeHandle(handle.(*factoryhost.Handle))

	_, err = host.Pause(ctx, handle)
	if !errors.Is(err, factory.ErrNotRunning) {
		t.Fatalf("Pause(unregistered) error = %v, want ErrNotRunning", err)
	}
	_, err = host.Resume(ctx, handle)
	if !errors.Is(err, factory.ErrNotRunning) {
		t.Fatalf("Resume(unregistered) error = %v, want ErrNotRunning", err)
	}
}

func TestPauseResumeDoesNotStartNewHandle(t *testing.T) {
	t.Parallel()

	host := newTestHost(t)
	factoryStub := newLifecycleControlFactory(interfaces.FactoryStateRunning)
	handle := startReadyHostedHandle(t, host, factoryStub, "runtime-single-handle")

	if _, err := host.Pause(context.Background(), handle); err != nil {
		t.Fatalf("Pause() error = %v", err)
	}
	if _, err := host.Resume(context.Background(), handle); err != nil {
		t.Fatalf("Resume() error = %v", err)
	}
	if len(host.handles) != 1 {
		t.Fatalf("handles after pause/resume = %d, want single active handle", len(host.handles))
	}
}
