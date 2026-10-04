package host_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/work"

	"github.com/jonboulle/clockwork"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryhost "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/host"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"go.uber.org/zap"
)

func TestNewLifecycleService_RequiresClock(t *testing.T) {
	t.Parallel()

	service, err := factoryhost.NewLifecycleService(nil, platformclock.Real{})
	if service != nil || err == nil || !strings.Contains(err.Error(), "clock is required") {
		t.Fatalf("NewLifecycleService() = (%v, %v), want nil service and clock dependency error", service, err)
	}
}

func TestNewLifecycleService_RequiresScheduler(t *testing.T) {
	t.Parallel()
	service, err := factoryhost.NewLifecycleService(clockwork.NewFakeClock(), nil)
	if service != nil || err == nil || !strings.Contains(err.Error(), "scheduler is required") {
		t.Fatalf("NewLifecycleService() = (%v, %v), want scheduler dependency error", service, err)
	}
}

// readinessTimers exposes creation and firing separately so readiness can be
// observed without advancing replay facts or sleeping for a wall-clock poll.
type readinessTimers struct {
	created chan *readinessTimer
}

type readinessTimer struct {
	duration time.Duration
	fired    chan time.Time
	mu       sync.Mutex
	stopped  bool
}

func (*readinessTimers) Now() time.Time { panic("readiness must not read fact time") }
func (s *readinessTimers) NewTimer(duration time.Duration) platformclock.Timer {
	timer := &readinessTimer{duration: duration, fired: make(chan time.Time, 1)}
	s.created <- timer
	return timer
}
func (timer *readinessTimer) C() <-chan time.Time { return timer.fired }
func (timer *readinessTimer) Stop() bool {
	timer.mu.Lock()
	defer timer.mu.Unlock()
	wasActive := !timer.stopped
	timer.stopped = true
	return wasActive
}
func (timer *readinessTimer) assertStopped(t *testing.T) {
	t.Helper()
	timer.mu.Lock()
	defer timer.mu.Unlock()
	if !timer.stopped {
		t.Fatal("readiness returned with an active timer")
	}
}
func (s *readinessTimers) next(t *testing.T, duration time.Duration) *readinessTimer {
	t.Helper()
	// The suite's timeout is the deadlock ceiling; synchronization is entirely
	// through timer creation and delivery, without a competing wall-clock wait.
	timer := <-s.created
	if timer.duration != duration {
		t.Fatalf("timer duration = %s, want %s", timer.duration, duration)
	}
	return timer
}

func awaitReadiness(t *testing.T, result <-chan error, want error) {
	t.Helper()
	err := <-result
	if !errors.Is(err, want) {
		t.Fatalf("readiness = %v, want %v", err, want)
	}
}

func TestWaitForStart_UsesControlledPollAndCeiling(t *testing.T) {
	t.Parallel()
	for _, outcome := range []string{"running", "deadline", "cancel", "run failure", "incomplete drain"} {
		t.Run(outcome, func(t *testing.T) {
			t.Parallel()
			engine := &lifecycleObserverFactory{}
			engine.setEngineState(&interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{FactoryState: string(interfaces.FactoryStatePaused)})
			handle := &factoryhost.Handle{Bundle: &factoryhost.Bundle{Factory: engine}, RunDone: make(chan struct{})}
			scheduler := &readinessTimers{created: make(chan *readinessTimer, 8)}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			result := make(chan error, 1)
			go func() { result <- factoryhost.WaitForStart(ctx, handle, scheduler) }()
			deadline := scheduler.next(t, time.Second)
			poll := scheduler.next(t, 10*time.Millisecond)
			poll.fired <- time.Time{}
			nextPoll := scheduler.next(t, 10*time.Millisecond)
			poll.assertStopped(t)
			var want error
			switch outcome {
			case "running":
				engine.setEngineState(&interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{FactoryState: string(interfaces.FactoryStateRunning)})
				nextPoll.fired <- time.Time{}
			case "deadline":
				want = context.DeadlineExceeded
				deadline.fired <- time.Time{}
			case "cancel":
				want = context.Canceled
				cancel()
			case "run failure":
				want = errors.New("controlled run failed")
				handle.SetRunResult(want)
			case "incomplete drain":
				handle.SetRunResult(&factory.IncompleteDrainError{NonTerminalWorkCount: 2})
			}
			awaitReadiness(t, result, want)
			deadline.assertStopped(t)
			nextPoll.assertStopped(t)
		})
	}
}

func TestLifecycleService_ReadinessUsesSchedulerAndStopUsesReplayFactClock(t *testing.T) {
	t.Parallel()
	finishedAt := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	factClock := clockwork.NewFakeClockAt(finishedAt)
	scheduler := &readinessTimers{created: make(chan *readinessTimer, 8)}
	lifecycle, err := factoryhost.NewLifecycleService(factClock, scheduler)
	if err != nil {
		t.Fatal(err)
	}
	recording := &terminalRecording{}
	engine := &blockingLifecycleFactory{}
	engine.setEngineState(&interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{FactoryState: string(interfaces.FactoryStatePaused)})
	handle, err := lifecycle.Start(context.Background(), &factoryhost.Bundle{Factory: engine, Recording: recording, Logger: zap.NewNop()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = lifecycle.Stop(handle) })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() { result <- lifecycle.WaitForStart(ctx, handle) }()
	deadline := scheduler.next(t, time.Second)
	poll := scheduler.next(t, 10*time.Millisecond)
	factClock.Advance(24 * time.Hour)
	select {
	case err := <-result:
		t.Fatalf("replay clock advance ended readiness: %v", err)
	default:
	}
	cancel()
	awaitReadiness(t, result, context.Canceled)
	deadline.assertStopped(t)
	poll.assertStopped(t)
	if err := lifecycle.Stop(handle); err != nil {
		t.Fatal(err)
	}
	if !handle.Completed() {
		t.Fatal("Stop returned before joining the run")
	}
	if !recording.finishedAt.Equal(finishedAt.Add(24 * time.Hour)) {
		t.Fatalf("recording finished at %s, want selected replay time", recording.finishedAt)
	}
}

func TestFinalizeArtifacts_RequiresClock(t *testing.T) {
	t.Parallel()

	err := factoryhost.FinalizeArtifacts(&factoryhost.Bundle{}, nil)
	if err == nil || !strings.Contains(err.Error(), "clock is required") {
		t.Fatalf("FinalizeArtifacts() error = %v, want required clock error", err)
	}
}

type terminalRecording struct {
	finalizeCalls int
	finishedAt    time.Time
	finalizeErr   error
}

func (*terminalRecording) BindRecordingLifecycle(
	recordings.RecordingLifecycle,
	recordings.CanonicalEventScope,
) error {
	return nil
}
func (*terminalRecording) Start(context.Context)               {}
func (*terminalRecording) Stop()                               {}
func (*terminalRecording) RecordEvent(interfaces.FactoryEvent) {}
func (*terminalRecording) RecordError(error)                   {}
func (*terminalRecording) Finish(time.Time)                    {}
func (*terminalRecording) Flush() error                        { return nil }
func (*terminalRecording) Err() error                          { return nil }
func (r *terminalRecording) Finalize(finishedAt time.Time) error {
	r.finalizeCalls++
	r.finishedAt = finishedAt
	return r.finalizeErr
}

var _ recordings.RuntimeRecorder = (*terminalRecording)(nil)

// flushFailingRecording is a controllable recordings.RuntimeRecorder fake
// used to prove that a startup Flush failure joins any preserved cleanup
// (Stop) cause into the returned Start() result rather than discarding it.
type flushFailingRecording struct {
	flushErr   error
	cleanupErr error
	stopCalls  int
}

func (*flushFailingRecording) BindRecordingLifecycle(
	recordings.RecordingLifecycle,
	recordings.CanonicalEventScope,
) error {
	return nil
}
func (*flushFailingRecording) Start(context.Context)               {}
func (r *flushFailingRecording) Stop()                             { r.stopCalls++ }
func (*flushFailingRecording) RecordEvent(interfaces.FactoryEvent) {}
func (*flushFailingRecording) RecordError(error)                   {}
func (*flushFailingRecording) Finish(time.Time)                    {}
func (r *flushFailingRecording) Flush() error                      { return r.flushErr }
func (r *flushFailingRecording) Err() error                        { return r.cleanupErr }
func (*flushFailingRecording) Finalize(time.Time) error            { return nil }

var _ recordings.RuntimeRecorder = (*flushFailingRecording)(nil)

// runTrackingFactory records whether Run was ever invoked, to prove runtime
// execution never starts after a startup recording failure.
type runTrackingFactory struct {
	lifecycleObserverFactory
	ran bool
}

func (f *runTrackingFactory) Run(context.Context) error {
	f.ran = true
	return nil
}

func TestStart_JoinsCleanupCauseWhenStartupFlushFailsAndNeverRunsFactory(t *testing.T) {
	t.Parallel()

	flushErr := errors.New("startup flush failed")
	cleanupErr := errors.New("stop cleanup failed")
	recording := &flushFailingRecording{flushErr: flushErr, cleanupErr: cleanupErr}
	factoryStub := &runTrackingFactory{}

	handle := factoryhost.Start(context.Background(), &factoryhost.Bundle{
		Factory:   factoryStub,
		Logger:    zap.NewNop(),
		Recording: recording,
	})

	if !handle.Completed() {
		t.Fatal("Start() should complete the handle synchronously when the startup flush fails")
	}
	err := handle.Result()
	if !errors.Is(err, flushErr) {
		t.Fatalf("Start() result error = %v, want it to preserve the initiating flush cause", err)
	}
	if !errors.Is(err, cleanupErr) {
		t.Fatalf("Start() result error = %v, want it to preserve the cleanup (Stop) cause", err)
	}
	if recording.stopCalls != 1 {
		t.Fatalf("recording Stop calls = %d, want exactly 1", recording.stopCalls)
	}
	if factoryStub.ran {
		t.Fatal("runtime execution must not start after a startup recording flush failure")
	}
}

func TestStopDelegatesEveryRuntimeOutcomeToRecordingFinalization(t *testing.T) {
	t.Parallel()

	finishedAt := time.Date(2026, 7, 27, 19, 0, 0, 0, time.UTC)
	tests := map[string]error{
		"normal completion":   nil,
		"caller cancellation": context.Canceled,
		"runtime crash":       errors.New("runtime crashed"),
	}
	for name, runErr := range tests {
		t.Run(name, func(t *testing.T) {
			recording := &terminalRecording{}
			handle := &factoryhost.Handle{
				Bundle:  &factoryhost.Bundle{Recording: recording},
				RunDone: make(chan struct{}),
			}
			handle.SetRunResult(runErr)

			err := factoryhost.Stop(handle, clockwork.NewFakeClockAt(finishedAt))
			if runErr != nil && !errors.Is(runErr, context.Canceled) && !errors.Is(err, runErr) {
				t.Fatalf("Stop error = %v, want run error %v", err, runErr)
			}
			if (runErr == nil || errors.Is(runErr, context.Canceled)) && err != nil {
				t.Fatalf("Stop error = %v, want nil", err)
			}
			if recording.finalizeCalls != 1 || !recording.finishedAt.Equal(finishedAt) {
				t.Fatalf(
					"recording finalization = (%d, %s), want one call at %s",
					recording.finalizeCalls,
					recording.finishedAt,
					finishedAt,
				)
			}
		})
	}
}

func TestStopPreservesFailuresAfterCanceledRun(t *testing.T) {
	t.Parallel()
	flushErr := errors.New("final recording flush failed")
	runFailure := errors.New("worker drain failed")
	for name, runErr := range map[string]error{
		"cancellation":   context.Canceled,
		"joined failure": fmt.Errorf("runtime stopped: %w", errors.Join(context.Canceled, runFailure)),
	} {
		t.Run(name, func(t *testing.T) {
			recording := &terminalRecording{finalizeErr: flushErr}
			handle := &factoryhost.Handle{
				Bundle: &factoryhost.Bundle{Recording: recording}, RunDone: make(chan struct{}),
			}
			handle.SetRunResult(runErr)
			err := factoryhost.Stop(handle, clockwork.NewFakeClock())
			if !errors.Is(err, flushErr) || errors.Is(err, context.Canceled) {
				t.Fatalf("Stop = %v, want final flush failure without benign cancellation", err)
			}
			if errors.Is(runErr, runFailure) && !errors.Is(err, runFailure) {
				t.Fatalf("Stop = %v, lost worker drain failure", err)
			}
		})
	}
}

func TestStop_RequiresClockBeforeMutatingHandle(t *testing.T) {
	t.Parallel()

	handle := &factoryhost.Handle{RunDone: make(chan struct{})}
	err := factoryhost.Stop(handle, nil)
	if err == nil || !strings.Contains(err.Error(), "clock is required") {
		t.Fatalf("Stop() error = %v, want required clock error", err)
	}
	select {
	case <-handle.RunDone:
		t.Fatal("Stop() mutated handle before rejecting missing clock")
	default:
	}
}

func TestPublishFactoryChange_RequiresClock(t *testing.T) {
	t.Parallel()

	err := factoryhost.PublishFactoryChange(context.Background(), nil, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "clock is required") {
		t.Fatalf("PublishFactoryChange() error = %v, want required clock error", err)
	}
}

type lifecycleObserverFactory struct {
	mu          sync.RWMutex
	engineState *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]
}

type lifecycleMetricRecordWriter struct {
	mu      sync.Mutex
	records []factory.RuntimeMetricRecord
}

func (w *lifecycleMetricRecordWriter) WriteMetric(
	ctx context.Context,
	record factory.RuntimeMetricRecord,
) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.records = append(w.records, record)
	return nil
}

func (w *lifecycleMetricRecordWriter) Close() error { return nil }

type orderedStopFactory struct {
	lifecycleObserverFactory
	sidecarExited      <-chan struct{}
	runStoppedTooEarly chan<- struct{}
}

func (f *orderedStopFactory) Run(ctx context.Context) error {
	<-ctx.Done()
	select {
	case <-f.sidecarExited:
	default:
		close(f.runStoppedTooEarly)
	}
	return ctx.Err()
}

func (f *lifecycleObserverFactory) Run(context.Context) error { return nil }
func (f *lifecycleObserverFactory) SubmitWorkRequest(context.Context, work.WorkRequest) (work.WorkRequestSubmitResult, error) {
	return work.WorkRequestSubmitResult{}, nil
}
func (f *lifecycleObserverFactory) SubscribeFactoryEvents(context.Context, *interfaces.FactoryEventReconnectCursor, interfaces.FactoryEventReconnectScope) (*interfaces.FactoryEventStream, error) {
	return &interfaces.FactoryEventStream{Events: make(chan interfaces.FactoryEvent)}, nil
}
func (f *lifecycleObserverFactory) Pause(context.Context) error  { return nil }
func (f *lifecycleObserverFactory) Resume(context.Context) error { return nil }
func (f *lifecycleObserverFactory) Terminate(context.Context, factory.TerminateRequest) (factory.TerminateResult, error) {
	return factory.TerminateResult{Outcome: factory.ControlOutcomeAccepted}, nil
}
func (f *lifecycleObserverFactory) Observe(context.Context, factory.ObserveRequest) (factory.ObserveResult, error) {
	return factory.ObserveResult{}, nil
}
func (f *lifecycleObserverFactory) PlanDispatch(_ context.Context, req factory.PlanDispatchRequest) (factory.PlanDispatchResult, error) {
	return factory.PlanDispatchResult{
		Outcome:       factory.DispatchPlanOutcomeAccepted,
		DispatchID:    req.DispatchID,
		CorrelationID: req.CorrelationID,
	}, nil
}
func (f *lifecycleObserverFactory) AcceptDispatchResult(_ context.Context, req factory.AcceptDispatchResultRequest) (factory.AcceptDispatchResultResult, error) {
	return factory.AcceptDispatchResultResult{
		Outcome:       factory.DispatchPlanOutcomeRetired,
		DispatchID:    req.DispatchID,
		CorrelationID: req.CorrelationID,
	}, nil
}
func (f *lifecycleObserverFactory) MoveWork(context.Context, string, string, work.WorkStateChangeSource, string) (work.OperatorMoveResult, error) {
	return work.OperatorMoveResult{}, nil
}
func (f *lifecycleObserverFactory) GetEngineStateSnapshot(context.Context) (*interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.engineState, nil
}
func (f *lifecycleObserverFactory) GetFactoryEvents(context.Context) ([]interfaces.FactoryEvent, error) {
	return nil, nil
}
func (f *lifecycleObserverFactory) WaitToComplete() <-chan struct{} {
	ch := make(chan struct{})
	close(ch)
	return ch
}
func (f *lifecycleObserverFactory) setEngineState(state *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.engineState = state
}

func TestHandle_CompletionHelpers(t *testing.T) {
	if !(*factoryhost.Handle)(nil).Completed() {
		t.Fatal("nil Handle should report completed")
	}
	if err := (*factoryhost.Handle)(nil).Wait(); err != nil {
		t.Fatalf("nil Handle wait error = %v, want nil", err)
	}

	handle := &factoryhost.Handle{RunDone: make(chan struct{})}
	if handle.Completed() {
		t.Fatal("open RunDone should report incomplete")
	}
	handle.SetRunResult(fmt.Errorf("run failed"))
	if !handle.Completed() {
		t.Fatal("closed RunDone should report completed")
	}
	if err := handle.Wait(); err == nil || err.Error() != "run failed" {
		t.Fatalf("wait error = %v, want run failed", err)
	}
}

func TestRuntimeStopOutcome_PrefersTerminalResultOverForcedCancel(t *testing.T) {
	finished := &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		RuntimeStatus: interfaces.RuntimeStatusFinished,
		FactoryState:  string(interfaces.FactoryStateRunning),
	}
	active := &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		RuntimeStatus: interfaces.RuntimeStatusActive,
		FactoryState:  string(interfaces.FactoryStateRunning),
	}

	outcome, reason := factoryhost.RuntimeStopOutcome(finished, nil, true)
	if outcome != "completed" || reason != "" {
		t.Fatalf("RuntimeStopOutcome(finished, nil, forcedCancel=true) = (%q, %q), want (completed, \"\")", outcome, reason)
	}

	outcome, reason = factoryhost.RuntimeStopOutcome(active, context.Canceled, false)
	if outcome != "canceled" || reason != "" {
		t.Fatalf("RuntimeStopOutcome(active, context.Canceled, false) = (%q, %q), want (canceled, \"\")", outcome, reason)
	}

	outcome, reason = factoryhost.RuntimeStopOutcome(active, nil, true)
	if outcome != "canceled" || reason != "" {
		t.Fatalf("RuntimeStopOutcome(active, nil, forcedCancel=true) = (%q, %q), want (canceled, \"\")", outcome, reason)
	}
}

func TestStop_EmitsCompletedLifecycleMetricWithoutRootService(t *testing.T) {
	metricsWriter := &lifecycleMetricRecordWriter{}
	metricsSink, err := factory.NewRuntimeMetricsSink(
		metricsWriter,
		factory.RuntimeMetricsScope{
			SessionID: "~default", RuntimeInstanceID: "runtime-lifecycle-metrics",
		},
		time.Now,
		factory.RuntimeMetricsArtifact{
			Path: "memory://runtime-metrics", StartTimeUTC: time.Now().UTC(),
		},
	)
	if err != nil {
		t.Fatalf("NewRuntimeMetricsSink: %v", err)
	}

	factoryStub := &lifecycleObserverFactory{}
	factoryStub.setEngineState(&interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		RuntimeStatus: interfaces.RuntimeStatusFinished,
		FactoryState:  string(interfaces.FactoryStateRunning),
	})

	runCtx, runCancel := context.WithCancel(context.Background())
	t.Cleanup(runCancel)

	handle := &factoryhost.Handle{
		Bundle: &factoryhost.Bundle{
			Factory:     factoryStub,
			MetricsSink: metricsSink,
			Logger:      zap.NewNop(),
		},
		RunDone:   make(chan struct{}),
		RunCancel: runCancel,
	}

	observerCtx, cancelObserver := context.WithCancel(runCtx)
	defer cancelObserver()
	go factoryhost.ObserveRuntimeMetrics(observerCtx, handle, platformclock.Real{})

	handle.SetRunResult(nil)
	if err := factoryhost.Stop(handle, clockwork.NewFakeClock()); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	metricsWriter.mu.Lock()
	defer metricsWriter.mu.Unlock()
	found := false
	for _, record := range metricsWriter.records {
		if record.MetricName == "runtime.lifecycle.stopped" && record.Value == 1 && record.Outcome == "completed" {
			found = true
		}
	}
	if !found {
		t.Fatalf("metrics = %#v, want completed lifecycle stop", metricsWriter.records)
	}
}

func TestStop_CancelsAndJoinsSidecarsBeforeStoppingRunLoop(t *testing.T) {
	sidecarExited := make(chan struct{})
	runStoppedTooEarly := make(chan struct{})
	factoryStub := &orderedStopFactory{
		sidecarExited:      sidecarExited,
		runStoppedTooEarly: runStoppedTooEarly,
	}
	handle := factoryhost.Start(context.Background(), &factoryhost.Bundle{
		Factory: factoryStub,
		Logger:  zap.NewNop(),
	})

	sidecarCtx, cancelSidecar := context.WithCancel(context.Background())
	handle.SidecarCancel = cancelSidecar
	handle.Sidecars.Add(1)
	go func() {
		defer handle.Sidecars.Done()
		<-sidecarCtx.Done()
		close(sidecarExited)
	}()

	err := factoryhost.Stop(handle, clockwork.NewFakeClock())
	if err != nil {
		t.Fatalf("Stop error = %v, want ordinary shutdown cancellation normalized", err)
	}
	select {
	case <-sidecarExited:
	default:
		t.Fatal("Stop returned before the sidecar exited")
	}
	select {
	case <-runStoppedTooEarly:
		t.Fatal("runtime run loop stopped before its sidecars exited")
	default:
	}
}

func TestWaitForStart_ReportsRunningReadinessWithoutRootService(t *testing.T) {
	factoryStub := &lifecycleObserverFactory{}
	factoryStub.setEngineState(&interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		RuntimeStatus: interfaces.RuntimeStatusActive,
		FactoryState:  string(interfaces.FactoryStateRunning),
	})

	handle := factoryhost.Start(context.Background(), &factoryhost.Bundle{
		Factory: factoryStub,
		Logger:  zap.NewNop(),
	})
	if handle == nil {
		t.Fatal("Start returned nil handle")
	}
	if err := factoryhost.WaitForStart(context.Background(), handle, platformclock.Real{}); err != nil {
		t.Fatalf("WaitForStart: %v", err)
	}
	handle.CancelRun()
	if err := factoryhost.Stop(handle, clockwork.NewFakeClock()); err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("Stop: %v", err)
	}
}

func TestWaitForStart_AllowsIncompleteDrainToReachHostedTransport(t *testing.T) {
	handle := &factoryhost.Handle{
		Bundle:  &factoryhost.Bundle{Factory: &lifecycleObserverFactory{}},
		RunDone: make(chan struct{}),
	}
	handle.SetRunResult(&factory.IncompleteDrainError{NonTerminalWorkCount: 2})

	if err := factoryhost.WaitForStart(context.Background(), handle, platformclock.Real{}); err != nil {
		t.Fatalf("WaitForStart: %v, want transport-visible startup", err)
	}
}
