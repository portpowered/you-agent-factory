package service

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryhost "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/host"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
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

func (*lifecycleControlFactory) CleanInvocationSnapshot(context.Context) (factory.CleanInvocationSnapshot, error) {
	return factory.CleanInvocationSnapshot{}, nil
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

func (f *lifecycleControlFactory) InvokeWorker(_ context.Context, _ factory.InvokeWorkerRequest) (factory.InvokeWorkerResult, error) {
	return factory.InvokeWorkerResult{}, nil
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

func startReadyHostedHandle(
	t *testing.T,
	host *Host,
	factoryStub factoryhost.Engine,
	instanceID string,
) factory.RuntimeRun {
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

// physicalHandleEngine observes only this run and holds exit until its owner
// releases it, making cancellation acknowledgement distinct from joined exit.
type physicalHandleEngine struct {
	*lifecycleControlFactory
	entered, canceled, release, exited chan struct{}
	releaseOnce                        sync.Once
	pauses, resumes, exits             atomic.Int32
}

func (f *physicalHandleEngine) Run(ctx context.Context) error {
	close(f.entered)
	<-ctx.Done()
	close(f.canceled)
	<-f.release
	f.exits.Add(1)
	close(f.exited)
	return ctx.Err()
}

func (f *physicalHandleEngine) ControlPause(ctx context.Context, req factory.PauseRequest) (factory.PauseResult, error) {
	f.pauses.Add(1)
	return f.lifecycleControlFactory.ControlPause(ctx, req)
}

func (f *physicalHandleEngine) ControlResume(ctx context.Context, req factory.ResumeRequest) (factory.ResumeResult, error) {
	f.resumes.Add(1)
	return f.lifecycleControlFactory.ControlResume(ctx, req)
}

func (f *physicalHandleEngine) releaseExit() {
	f.releaseOnce.Do(func() { close(f.release) })
}

type physicalHandleRecording struct {
	terminalRecording
	finalizations atomic.Int32
}

func (r *physicalHandleRecording) Finalize(_ time.Time) error {
	r.finalizations.Add(1)
	return nil
}

type physicalHandleFixture struct {
	host      *Host
	handle    factory.RuntimeRun
	engine    *physicalHandleEngine
	recording *physicalHandleRecording
	stopDone  chan error
}

func startPhysicalHandle(t *testing.T, host *Host, id string) *physicalHandleFixture {
	t.Helper()
	engine := &physicalHandleEngine{
		lifecycleControlFactory: newLifecycleControlFactory(interfaces.FactoryStateRunning),
		entered:                 make(chan struct{}), canceled: make(chan struct{}),
		release: make(chan struct{}), exited: make(chan struct{}),
	}
	recording := &physicalHandleRecording{}
	bundle := testBundle(engine, id)
	bundle.Recording = recording
	handle, err := host.Start(context.Background(), bundle)
	if err != nil || handle == nil {
		t.Fatalf("Start(%s) = (%v, %v), want live handle", id, handle, err)
	}
	f := &physicalHandleFixture{host: host, handle: handle, engine: engine, recording: recording}
	t.Cleanup(func() {
		engine.releaseExit()
		// A test may fail while its Stop goroutine owns the removed registration.
		// Cancel/join the run first, then join that Stop before observing cleanup.
		handle.CancelRun()
		if f.stopDone != nil {
			if err := <-f.stopDone; err != nil {
				t.Errorf("pending Stop cleanup: %v", err)
			}
		} else if err := host.Stop(handle); err != nil && !errors.Is(err, factory.ErrAlreadyStopped) {
			t.Errorf("Stop cleanup: %v", err)
		}
		if err := handle.Wait(); !errors.Is(err, context.Canceled) {
			t.Errorf("run cleanup result = %v, want context.Canceled", err)
		}
	})
	<-engine.entered
	return f
}

func (f *physicalHandleFixture) assertLive(t *testing.T, pauses, resumes int32, wantState interfaces.FactoryState) {
	t.Helper()
	snapshot, err := f.engine.GetEngineStateSnapshot(context.Background())
	if err != nil || snapshot.FactoryState != string(wantState) ||
		f.engine.pauses.Load() != pauses || f.engine.resumes.Load() != resumes {
		t.Fatalf("live engine = (%v, %v, pauses=%d, resumes=%d), want (%s, %d, %d)",
			snapshot, err, f.engine.pauses.Load(), f.engine.resumes.Load(), wantState, pauses, resumes)
	}
	for _, ch := range []<-chan struct{}{f.engine.canceled, f.engine.exited, f.handle.RunDoneCh()} {
		select {
		case <-ch:
			t.Fatal("live run cancellation/exit/completion signal closed")
		default:
		}
	}
	if f.handle.Completed() || f.handle.Result() != nil || f.engine.exits.Load() != 0 || f.recording.finalizations.Load() != 0 {
		t.Fatal("live run has terminal result, exit or finalization")
	}
}

func (f *physicalHandleFixture) assertAcceptedControls(t *testing.T) {
	t.Helper()
	result, err := f.host.Pause(context.Background(), f.handle)
	if err != nil || result.Outcome != factory.ControlOutcomeAccepted {
		t.Fatalf("Pause = (%v, %v), want ACCEPTED", result, err)
	}
	f.assertLive(t, 1, 0, interfaces.FactoryStatePaused)
	resumed, err := f.host.Resume(context.Background(), f.handle)
	if err != nil || resumed.Outcome != factory.ControlOutcomeAccepted {
		t.Fatalf("Resume = (%v, %v), want ACCEPTED", resumed, err)
	}
	f.assertLive(t, 1, 1, interfaces.FactoryStateRunning)
}

func (f *physicalHandleFixture) stopAndJoin(t *testing.T, beforeRelease func()) {
	t.Helper()
	f.stopDone = make(chan error, 1)
	go func() { f.stopDone <- f.host.Stop(f.handle) }()
	<-f.engine.canceled
	select {
	case err := <-f.stopDone:
		f.stopDone = nil
		t.Fatalf("Stop returned before exit release: %v", err)
	default:
	}
	if f.handle.Completed() || f.recording.finalizations.Load() != 0 {
		t.Fatal("run completed or finalized before exit release")
	}
	if beforeRelease != nil {
		beforeRelease()
	}
	f.engine.releaseExit()
	err := <-f.stopDone
	f.stopDone = nil
	if err != nil {
		t.Fatalf("Stop = %v, want normalized cancellation", err)
	}
	<-f.engine.exited
	<-f.handle.RunDoneCh()
	if !f.handle.Completed() || !errors.Is(f.handle.Wait(), context.Canceled) ||
		!errors.Is(f.handle.Result(), context.Canceled) || f.engine.exits.Load() != 1 || f.recording.finalizations.Load() != 1 {
		t.Fatal("Stop did not join one canceled run and finalize once")
	}
}

func TestPhysicalHandleSelectedControlsAndStopAreIsolated(t *testing.T) {
	t.Parallel()
	for _, selectedID := range []string{"A", "B"} {
		t.Run("selected_"+selectedID, func(t *testing.T) {
			t.Parallel()
			host := newTestHost(t)
			a := startPhysicalHandle(t, host, "runtime-A")
			b := startPhysicalHandle(t, host, "runtime-B")
			selected, peer := a, b
			if selectedID == "B" {
				selected, peer = b, a
			}
			result, err := host.Pause(context.Background(), selected.handle)
			if err != nil || result.Outcome != factory.ControlOutcomeAccepted {
				t.Fatalf("H01 Pause = (%v, %v), want ACCEPTED", result, err)
			}
			selected.assertLive(t, 1, 0, interfaces.FactoryStatePaused)
			peer.assertLive(t, 0, 0, interfaces.FactoryStateRunning)
			resumed, err := host.Resume(context.Background(), selected.handle)
			if err != nil || resumed.Outcome != factory.ControlOutcomeAccepted {
				t.Fatalf("H02 Resume = (%v, %v), want ACCEPTED", resumed, err)
			}
			selected.assertLive(t, 1, 1, interfaces.FactoryStateRunning)
			peer.assertLive(t, 0, 0, interfaces.FactoryStateRunning)
			selected.stopAndJoin(t, func() {
				peer.assertLive(t, 0, 0, interfaces.FactoryStateRunning)
			})
			peer.assertLive(t, 0, 0, interfaces.FactoryStateRunning)
			peer.assertAcceptedControls(t)
			peer.stopAndJoin(t, nil)
			if selected.recording.finalizations.Load() != 1 || selected.engine.exits.Load() != 1 {
				t.Fatal("peer shutdown repeated selected cleanup")
			}
		})
	}
}

func TestPhysicalForeignLiveHandleCannotControlEqualIDRun(t *testing.T) {
	t.Parallel()
	for _, direction := range []string{"A_receives_B", "B_receives_A"} {
		for _, operation := range []string{"H04_Pause", "H05_Resume", "H06_Stop"} {
			t.Run(direction+"/"+operation, func(t *testing.T) {
				t.Parallel()
				a := startPhysicalHandle(t, newTestHost(t), "equal-runtime-ID")
				b := startPhysicalHandle(t, newTestHost(t), "equal-runtime-ID")
				receiver, foreign := a, b
				if direction == "B_receives_A" {
					receiver, foreign = b, a
				}
				a.assertLive(t, 0, 0, interfaces.FactoryStateRunning)
				b.assertLive(t, 0, 0, interfaces.FactoryStateRunning)
				var err error
				switch operation {
				case "H04_Pause":
					var result factory.PauseResult
					result, err = receiver.host.Pause(context.Background(), foreign.handle)
					if !reflect.DeepEqual(result, factory.PauseResult{}) {
						t.Fatalf("foreign Pause result = %#v, want zero result", result)
					}
				case "H05_Resume":
					var result factory.ResumeResult
					result, err = receiver.host.Resume(context.Background(), foreign.handle)
					if !reflect.DeepEqual(result, factory.ResumeResult{}) {
						t.Fatalf("foreign Resume result = %#v, want zero result", result)
					}
				case "H06_Stop":
					err = receiver.host.Stop(foreign.handle)
				}
				if !errors.Is(err, factory.ErrNotRunning) {
					t.Fatalf("foreign %s error = %v, want ErrNotRunning", operation, err)
				}
				a.assertLive(t, 0, 0, interfaces.FactoryStateRunning)
				b.assertLive(t, 0, 0, interfaces.FactoryStateRunning)
				a.assertAcceptedControls(t)
				b.assertLive(t, 0, 0, interfaces.FactoryStateRunning)
				b.assertAcceptedControls(t)
				a.assertLive(t, 1, 1, interfaces.FactoryStateRunning)
				a.stopAndJoin(t, func() {
					b.assertLive(t, 1, 1, interfaces.FactoryStateRunning)
				})
				b.stopAndJoin(t, nil)
				if a.engine.exits.Load() != 1 || a.recording.finalizations.Load() != 1 {
					t.Fatal("host B shutdown repeated host A cleanup")
				}
			})
		}
	}
}

func (f *physicalHandleFixture) assertStoppedWithoutControls(t *testing.T) {
	t.Helper()
	if f.engine.pauses.Load() != 0 || f.engine.resumes.Load() != 0 ||
		f.engine.exits.Load() != 1 || f.recording.finalizations.Load() != 1 ||
		!f.handle.Completed() || !errors.Is(f.handle.Wait(), context.Canceled) ||
		!errors.Is(f.handle.Result(), context.Canceled) {
		t.Fatal("stale control or successor cleanup changed the stopped run")
	}
}

func TestPhysicalStoppedHandleCannotControlSameIDSuccessor(t *testing.T) {
	t.Parallel()
	for _, operation := range []string{"H07_Pause", "H08_Resume", "H09_Stop"} {
		t.Run(operation, func(t *testing.T) {
			t.Parallel()
			host := newTestHost(t)
			old := startPhysicalHandle(t, host, "restarted-runtime-ID")
			old.stopAndJoin(t, nil)
			current := startPhysicalHandle(t, host, "restarted-runtime-ID")
			if current.handle == old.handle {
				t.Fatal("Start reused the stopped handle")
			}
			current.assertLive(t, 0, 0, interfaces.FactoryStateRunning)
			var err error
			wantErr := factory.ErrNotRunning
			switch operation {
			case "H07_Pause":
				var result factory.PauseResult
				result, err = host.Pause(context.Background(), old.handle)
				if !reflect.DeepEqual(result, factory.PauseResult{}) {
					t.Fatalf("stale Pause result = %#v, want zero result", result)
				}
			case "H08_Resume":
				var result factory.ResumeResult
				result, err = host.Resume(context.Background(), old.handle)
				if !reflect.DeepEqual(result, factory.ResumeResult{}) {
					t.Fatalf("stale Resume result = %#v, want zero result", result)
				}
			case "H09_Stop":
				err = host.Stop(old.handle)
				wantErr = factory.ErrAlreadyStopped
			}
			if !errors.Is(err, wantErr) {
				t.Fatalf("stale %s error = %v, want %v", operation, err, wantErr)
			}
			current.assertLive(t, 0, 0, interfaces.FactoryStateRunning)
			old.assertStoppedWithoutControls(t)
			current.assertAcceptedControls(t)
			current.stopAndJoin(t, nil)
			old.assertStoppedWithoutControls(t)
		})
	}
}
