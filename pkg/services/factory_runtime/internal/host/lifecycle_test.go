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
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	factoryhost "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/host"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/state"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"go.uber.org/zap"
)

func TestNewLifecycleService_RequiresClock(t *testing.T) {
	t.Parallel()

	service, err := factoryhost.NewLifecycleService(nil)
	if service != nil || err == nil || !strings.Contains(err.Error(), "clock is required") {
		t.Fatalf("NewLifecycleService() = (%v, %v), want nil service and clock dependency error", service, err)
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
}

func (*terminalRecording) BindRecordingService(
	recordings.Service,
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
	return nil
}

var _ recordings.RuntimeRecorder = (*terminalRecording)(nil)

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
			if runErr != nil && !errors.Is(err, runErr) {
				t.Fatalf("Stop error = %v, want run error %v", err, runErr)
			}
			if runErr == nil && err != nil {
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
func (f *lifecycleObserverFactory) CaptureCheckpoint(_ context.Context, req factory.CaptureCheckpointRequest) (factory.CaptureCheckpointResult, error) {
	id := req.CheckpointID
	if id == "" {
		id = "checkpoint-stub"
	}
	return factory.CaptureCheckpointResult{
		Outcome: factory.CheckpointOutcomeCaptured,
		Checkpoint: factory.Checkpoint{
			CheckpointID:  id,
			SchemaVersion: 1,
			StrategyKind:  "runtime",
			Payload:       []byte(`{}`),
		},
	}, nil
}
func (f *lifecycleObserverFactory) LoadCheckpoint(_ context.Context, req factory.LoadCheckpointRequest) (factory.LoadCheckpointResult, error) {
	if req.CheckpointID == "" {
		return factory.LoadCheckpointResult{}, factory.ErrCheckpointNotFound
	}
	return factory.LoadCheckpointResult{}, factory.ErrCheckpointNotFound
}
func (f *lifecycleObserverFactory) RestoreCheckpoint(_ context.Context, req factory.RestoreCheckpointRequest) (factory.RestoreCheckpointResult, error) {
	return factory.RestoreCheckpointResult{
		Outcome:      factory.CheckpointOutcomeRestored,
		CheckpointID: req.Checkpoint.CheckpointID,
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
	go factoryhost.ObserveRuntimeMetrics(observerCtx, handle)

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
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Stop error = %v, want context canceled", err)
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
	if err := factoryhost.WaitForStart(context.Background(), handle); err != nil {
		t.Fatalf("WaitForStart: %v", err)
	}
	handle.CancelRun()
	if err := factoryhost.Stop(handle, clockwork.NewFakeClock()); err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("Stop: %v", err)
	}
}
