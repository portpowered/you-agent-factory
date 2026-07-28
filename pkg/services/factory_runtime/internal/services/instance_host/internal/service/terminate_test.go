package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	instancehost "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/instance_host"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	factoryhost "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/host"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/state"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"go.uber.org/zap"
)

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

type terminateMetricRecordWriter struct {
	records []factory.RuntimeMetricRecord
}

func (w *terminateMetricRecordWriter) WriteMetric(
	_ context.Context,
	record factory.RuntimeMetricRecord,
) error {
	w.records = append(w.records, record)
	return nil
}

func (*terminateMetricRecordWriter) Close() error { return nil }

type orderedTerminateFactory struct {
	executeObserverFactory
	sidecarExited      <-chan struct{}
	runStoppedTooEarly chan<- struct{}
}

func (f *orderedTerminateFactory) Run(ctx context.Context) error {
	<-ctx.Done()
	select {
	case <-f.sidecarExited:
	default:
		close(f.runStoppedTooEarly)
	}
	return ctx.Err()
}

func TestStopActiveHostedInstanceStopsSidecarsRunLoopAndFinalizesArtifacts(t *testing.T) {
	t.Parallel()

	finishedAt := time.Date(2026, 7, 27, 22, 0, 0, 0, time.UTC)
	recording := &terminalRecording{}
	host := newTestHost(t)
	factoryStub := &executeObserverFactory{}
	factoryStub.setEngineState(&interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		RuntimeStatus: interfaces.RuntimeStatusActive,
		FactoryState:  string(interfaces.FactoryStateRunning),
	})
	bundle := testBundle(factoryStub, "runtime-terminate-active")
	bundle.Recording = recording

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	handle, err := host.Start(ctx, bundle)
	if err != nil || handle == nil {
		t.Fatalf("Start() = (%v, %v), want hosted handle", handle, err)
	}
	if err := host.WaitForStart(ctx, handle); err != nil {
		t.Fatalf("WaitForStart() error = %v", err)
	}

	concrete := handle.(*factoryhost.Handle)
	sidecarCtx, sidecarCancel := context.WithCancel(context.Background())
	concrete.SidecarCancel = sidecarCancel
	concrete.Sidecars.Add(1)
	sidecarExited := make(chan struct{})
	go func() {
		defer concrete.Sidecars.Done()
		<-sidecarCtx.Done()
		close(sidecarExited)
	}()

	clock := clockwork.NewFakeClockAt(finishedAt)
	host.clock = clock
	host.lifecycle, err = factoryhost.NewLifecycleService(clock)
	if err != nil {
		t.Fatalf("NewLifecycleService() error = %v", err)
	}

	stopErr := host.Stop(handle)
	if !errors.Is(stopErr, context.Canceled) {
		t.Fatalf("Stop() error = %v, want context canceled", stopErr)
	}
	select {
	case <-sidecarExited:
	default:
		t.Fatal("Stop returned before sidecars exited")
	}
	if recording.finalizeCalls != 1 || !recording.finishedAt.Equal(finishedAt) {
		t.Fatalf(
			"recording finalization = (%d, %s), want one call at %s",
			recording.finalizeCalls,
			recording.finishedAt,
			finishedAt,
		)
	}
	if len(host.handles) != 0 {
		t.Fatalf("handles after stop = %d, want registry cleared", len(host.handles))
	}
}

func TestStopAlreadyStoppedReturnsErrAlreadyStoppedWithoutDoubleFinalizing(t *testing.T) {
	t.Parallel()

	recording := &terminalRecording{}
	host := newTestHost(t)
	factoryStub := &executeObserverFactory{}
	factoryStub.setEngineState(&interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		RuntimeStatus: interfaces.RuntimeStatusActive,
		FactoryState:  string(interfaces.FactoryStateRunning),
	})
	bundle := testBundle(factoryStub, "runtime-terminate-already-stopped")
	bundle.Recording = recording

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	handle, err := host.Start(ctx, bundle)
	if err != nil || handle == nil {
		t.Fatalf("Start() = (%v, %v), want hosted handle", handle, err)
	}
	if err := host.WaitForStart(ctx, handle); err != nil {
		t.Fatalf("WaitForStart() error = %v", err)
	}

	firstErr := host.Stop(handle)
	if firstErr != nil && !errors.Is(firstErr, context.Canceled) {
		t.Fatalf("first Stop() error = %v", firstErr)
	}
	if recording.finalizeCalls != 1 {
		t.Fatalf("finalize calls after first stop = %d, want 1", recording.finalizeCalls)
	}

	secondErr := host.Stop(handle)
	if !errors.Is(secondErr, factory.ErrAlreadyStopped) {
		t.Fatalf("second Stop() error = %v, want ErrAlreadyStopped", secondErr)
	}
	if recording.finalizeCalls != 1 {
		t.Fatalf("finalize calls after second stop = %d, want no double finalization", recording.finalizeCalls)
	}
}

func TestStopUnknownOrUnregisteredHandleReturnsErrNotRunning(t *testing.T) {
	t.Parallel()

	host := newTestHost(t)
	factoryStub := newLifecycleControlFactory(interfaces.FactoryStateRunning)
	other := startReadyHostedHandle(t, host, factoryStub, "runtime-terminate-other")

	unregistered := &factoryhost.Handle{
		Bundle:  testBundle(factoryStub, "runtime-terminate-unregistered"),
		RunDone: make(chan struct{}),
	}
	if err := host.Stop(unregistered); !errors.Is(err, factory.ErrNotRunning) {
		t.Fatalf("Stop(unregistered) error = %v, want ErrNotRunning", err)
	}
	if len(host.handles) != 1 || host.handles["runtime-terminate-other"] != other {
		t.Fatal("Stop(unregistered) mutated unrelated registered handle")
	}
}

type invalidHostedHandle struct{}

func (invalidHostedHandle) RuntimeInstance() factory.HostedInstance { return nil }
func (invalidHostedHandle) Completed() bool                         { return false }
func (invalidHostedHandle) Result() error                           { return nil }
func (invalidHostedHandle) Wait() error                             { return nil }
func (invalidHostedHandle) CancelRun()                              {}
func (invalidHostedHandle) RunDoneCh() <-chan struct{} {
	ch := make(chan struct{})
	close(ch)
	return ch
}

func TestStopNeverStartedInvalidHandleReturnsTypedFailure(t *testing.T) {
	t.Parallel()

	host := newTestHost(t)

	if err := host.Stop(nil); !errors.Is(err, factory.ErrNotRunning) {
		t.Fatalf("Stop(nil) error = %v, want ErrNotRunning", err)
	}

	if err := host.Stop(invalidHostedHandle{}); err == nil ||
		!strings.Contains(err.Error(), "requires a runtime handle") {
		t.Fatalf("Stop(invalid type) error = %v, want runtime-handle validation error", err)
	}

	neverStarted := &factoryhost.Handle{
		Bundle:  testBundle(newLifecycleControlFactory(interfaces.FactoryStateRunning), "runtime-never-started"),
		RunDone: make(chan struct{}),
	}
	if err := host.Stop(neverStarted); !errors.Is(err, factory.ErrNotRunning) {
		t.Fatalf("Stop(never-started) error = %v, want ErrNotRunning", err)
	}
}

func TestStopEmitsLifecycleStopMetricOnce(t *testing.T) {
	t.Parallel()

	metricsWriter := &terminateMetricRecordWriter{}
	metricsSink, err := factory.NewRuntimeMetricsSink(
		metricsWriter,
		factory.RuntimeMetricsScope{
			SessionID:         "~default",
			RuntimeInstanceID: "runtime-terminate-metrics",
		},
		time.Now,
		factory.RuntimeMetricsArtifact{
			Path:         "memory://runtime-metrics",
			StartTimeUTC: time.Now().UTC(),
		},
	)
	if err != nil {
		t.Fatalf("NewRuntimeMetricsSink: %v", err)
	}

	host := newTestHost(t)
	factoryStub := &executeObserverFactory{}
	factoryStub.setEngineState(&interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		RuntimeStatus: interfaces.RuntimeStatusFinished,
		FactoryState:  string(interfaces.FactoryStateRunning),
	})

	runCtx, runCancel := context.WithCancel(context.Background())
	t.Cleanup(runCancel)

	handle := &factoryhost.Handle{
		Bundle: &factoryhost.Bundle{
			RuntimeInstanceID: "runtime-terminate-metrics",
			Factory:           factoryStub,
			MetricsSink:       metricsSink,
			Logger:            zap.NewNop(),
		},
		RunDone:   make(chan struct{}),
		RunCancel: runCancel,
	}
	host.handles["runtime-terminate-metrics"] = handle

	observerCtx, cancelObserver := context.WithCancel(runCtx)
	defer cancelObserver()
	go factoryhost.ObserveRuntimeMetrics(observerCtx, handle)

	handle.SetRunResult(nil)
	if err := host.Stop(handle); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}

	stopCount := 0
	for _, record := range metricsWriter.records {
		if record.MetricName == "runtime.lifecycle.stopped" && record.Value == 1 && record.Outcome == "completed" {
			stopCount++
		}
	}
	if stopCount != 1 {
		t.Fatalf("lifecycle stop metrics = %d, want exactly one completed stop", stopCount)
	}
}

func TestReplacementSidecarShutdownDoesNotEmitFalseTerminalStop(t *testing.T) {
	t.Parallel()

	metricsWriter := &terminateMetricRecordWriter{}
	metricsSink, err := factory.NewRuntimeMetricsSink(
		metricsWriter,
		factory.RuntimeMetricsScope{
			SessionID:         "~default",
			RuntimeInstanceID: "runtime-terminate-replace-sidecar",
		},
		time.Now,
		factory.RuntimeMetricsArtifact{
			Path:         "memory://runtime-metrics",
			StartTimeUTC: time.Now().UTC(),
		},
	)
	if err != nil {
		t.Fatalf("NewRuntimeMetricsSink: %v", err)
	}

	host := newTestHost(t)
	currentFactory := newLifecycleControlFactory(interfaces.FactoryStateRunning)
	current := startReadyHostedHandle(t, host, currentFactory, "runtime-terminate-replace-sidecar").(*factoryhost.Handle)
	current.Bundle.MetricsSink = metricsSink

	runCtx, runCancel := context.WithCancel(context.Background())
	t.Cleanup(runCancel)
	observerCtx, cancelObserver := context.WithCancel(runCtx)
	defer cancelObserver()
	go factoryhost.ObserveRuntimeMetrics(observerCtx, current)

	replacementFactory := newLifecycleControlFactory(interfaces.FactoryStateRunning)
	replacementBundle := testBundle(replacementFactory, "runtime-terminate-replace-sidecar-next")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_, err = host.Replace(instancehost.ReplaceRequest{
		ReadinessContext:            ctx,
		ServiceContext:              ctx,
		Current:                     current,
		Replacement:                 replacementBundle,
		AttachSidecarsInServiceMode: true,
		AttachSidecars: func(_ context.Context, handle factory.HostedHandle) error {
			return nil
		},
	})
	if err != nil {
		t.Fatalf("Replace() error = %v", err)
	}

	for _, record := range metricsWriter.records {
		if record.MetricName == "runtime.lifecycle.stopped" {
			t.Fatalf("metrics %#v contain lifecycle stop during replacement sidecar shutdown", metricsWriter.records)
		}
	}
}

func TestStop_CancelsAndJoinsSidecarsBeforeStoppingRunLoop(t *testing.T) {
	t.Parallel()

	sidecarExited := make(chan struct{})
	runStoppedTooEarly := make(chan struct{})
	factoryStub := &orderedTerminateFactory{
		sidecarExited:      sidecarExited,
		runStoppedTooEarly: runStoppedTooEarly,
	}
	factoryStub.setEngineState(&interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		RuntimeStatus: interfaces.RuntimeStatusActive,
		FactoryState:  string(interfaces.FactoryStateRunning),
	})

	host := newTestHost(t)
	ctx := context.Background()
	bundle := testBundle(factoryStub, "runtime-terminate-order")
	handle, err := host.Start(ctx, bundle)
	if err != nil || handle == nil {
		t.Fatalf("Start() = (%v, %v), want hosted handle", handle, err)
	}
	if err := host.WaitForStart(ctx, handle); err != nil {
		t.Fatalf("WaitForStart() error = %v", err)
	}

	concrete := handle.(*factoryhost.Handle)
	sidecarCtx, cancelSidecar := context.WithCancel(context.Background())
	concrete.SidecarCancel = cancelSidecar
	concrete.Sidecars.Add(1)
	go func() {
		defer concrete.Sidecars.Done()
		<-sidecarCtx.Done()
		close(sidecarExited)
	}()

	stopErr := host.Stop(handle)
	if !errors.Is(stopErr, context.Canceled) {
		t.Fatalf("Stop() error = %v, want context canceled", stopErr)
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
