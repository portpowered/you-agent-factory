package host

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

func TestRuntimeMetricsObserverSamplesMemoryAtExistingCadence(t *testing.T) {
	t.Parallel()

	sink := &runtimeMemoryTestSink{}
	observer := runtimeMetricsObserver{}
	bundle := &Bundle{MetricsSink: sink}
	start := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)

	observer.observeMemory(bundle, start)
	if got := len(sink.samples); got != 7 {
		t.Fatalf("initial memory samples = %d, want 7", got)
	}
	observer.observeMemory(bundle, start.Add(runtimeMemoryObservationInterval-time.Nanosecond))
	if got := len(sink.samples); got != 7 {
		t.Fatalf("memory samples before cadence = %d, want 7", got)
	}
	observer.observeMemory(bundle, start.Add(runtimeMemoryObservationInterval))
	if got := len(sink.samples); got != 14 {
		t.Fatalf("memory samples at cadence = %d, want 14", got)
	}
}

func TestObserveRuntimeMetricsStopsMemorySamplingWithRunLifecycle(t *testing.T) {
	t.Parallel()

	sink := &runtimeMemoryTestSink{samplesReady: make(chan struct{})}
	engine := &runtimeMetricsTestEngine{
		snapshot: &interfaces.EngineStateSnapshot[factoryruntime.PetriMarkingSnapshot, *factoryruntime.RuntimeNet]{
			RuntimeStatus: interfaces.RuntimeStatusActive,
			FactoryState:  string(interfaces.FactoryStateRunning),
		},
	}
	handle := &Handle{
		Bundle:  &Bundle{Factory: engine, MetricsSink: sink},
		RunDone: make(chan struct{}),
	}
	clock := platformclock.NewDeterministic(time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC), time.Millisecond)
	observerDone := make(chan struct{})
	go func() {
		defer close(observerDone)
		observeRuntimeMetrics(context.Background(), handle, clock)
	}()

	select {
	case <-sink.samplesReady:
	case <-time.After(time.Second):
		t.Fatal("observer did not emit its initial memory samples")
	}
	close(handle.RunDone)
	select {
	case <-observerDone:
	case <-time.After(time.Second):
		t.Fatal("observer did not stop with the runtime lifecycle")
	}

	sampleCount := len(sink.samples)
	if got := len(sink.samples); got != sampleCount {
		t.Fatalf("memory samples after observer stop = %d, want %d", got, sampleCount)
	}
}

func TestFinalizeArtifactsWaitsForCompleteMemorySnapshot(t *testing.T) {
	sink := &runtimeMemorySnapshotBarrierSink{
		seventhStarted: make(chan struct{}),
		releaseSeventh: make(chan struct{}),
	}
	bundle := &Bundle{MetricsSink: sink}
	emissionDone := make(chan error, 1)
	go func() { emissionDone <- bundle.EmitRuntimeMemoryMetrics() }()
	<-sink.seventhStarted

	if bundle.metricsMu.TryLock() {
		bundle.metricsMu.Unlock()
		close(sink.releaseSeventh)
		t.Fatal("memory snapshot did not hold the metrics gate")
	}

	finalizeDone := make(chan error, 1)
	go func() { finalizeDone <- FinalizeArtifacts(bundle, clockwork.NewFakeClock()) }()
	close(sink.releaseSeventh)

	if err := <-emissionDone; err != nil {
		t.Fatalf("EmitRuntimeMemoryMetrics: %v", err)
	}
	if err := <-finalizeDone; err != nil {
		t.Fatalf("FinalizeArtifacts: %v", err)
	}

	sink.mu.Lock()
	defer sink.mu.Unlock()
	if len(sink.names) != 7 {
		t.Fatalf("memory snapshot records = %d, want 7; names = %#v", len(sink.names), sink.names)
	}
	if !sink.closed {
		t.Fatal("FinalizeArtifacts did not close the metrics sink")
	}
}

func TestLateMetricEmissionDoesNotReachClosedSink(t *testing.T) {
	sink := &lateMetricEmissionSink{
		closeStarted: make(chan struct{}),
		releaseClose: make(chan struct{}),
	}
	bundle := &Bundle{MetricsSink: sink}

	finalizeDone := make(chan error, 1)
	go func() { finalizeDone <- FinalizeArtifacts(bundle, clockwork.NewFakeClock()) }()
	<-sink.closeStarted

	emissionDone := make(chan error, 1)
	go func() {
		emissionDone <- bundle.MetricsEmitter().Counter(
			context.Background(), "late.metric", 1, factoryruntime.Fields{},
		)
	}()
	close(sink.releaseClose)

	if err := <-emissionDone; err != nil {
		t.Fatalf("late metric emission: %v", err)
	}
	if err := <-finalizeDone; err != nil {
		t.Fatalf("FinalizeArtifacts: %v", err)
	}

	sink.mu.Lock()
	defer sink.mu.Unlock()
	if sink.counterCalls != 0 {
		t.Fatalf("late metric counter calls = %d, want 0", sink.counterCalls)
	}
	if !sink.closed {
		t.Fatal("metrics sink was not closed")
	}
}

type runtimeMemoryTestSink struct {
	mu           sync.Mutex
	samples      []string
	samplesReady chan struct{}
	readyOnce    sync.Once
}

func (*runtimeMemoryTestSink) Counter(context.Context, string, float64, factoryruntime.Fields) error {
	return nil
}

func (*runtimeMemoryTestSink) Gauge(context.Context, string, float64, factoryruntime.Fields) error {
	return nil
}

func (sink *runtimeMemoryTestSink) Sample(
	_ context.Context,
	name string,
	_ float64,
	_ string,
	_ factoryruntime.Fields,
) error {
	sink.mu.Lock()
	sink.samples = append(sink.samples, name)
	ready := len(sink.samples) == 7
	sink.mu.Unlock()
	if ready && sink.samplesReady != nil {
		sink.readyOnce.Do(func() { close(sink.samplesReady) })
	}
	return nil
}

func (*runtimeMemoryTestSink) Close() error { return nil }
func (*runtimeMemoryTestSink) Path() string { return "memory://runtime-metrics" }
func (*runtimeMemoryTestSink) Artifact() factoryruntime.RuntimeMetricsArtifact {
	return factoryruntime.RuntimeMetricsArtifact{Path: "memory://runtime-metrics"}
}

type runtimeMetricsTestEngine struct {
	snapshot *interfaces.EngineStateSnapshot[factoryruntime.PetriMarkingSnapshot, *factoryruntime.RuntimeNet]
}

func (*runtimeMetricsTestEngine) SubmitWorkRequest(
	context.Context,
	work.WorkRequest,
) (work.WorkRequestSubmitResult, error) {
	return work.WorkRequestSubmitResult{}, nil
}

func (*runtimeMetricsTestEngine) SubscribeFactoryEvents(
	context.Context,
	*interfaces.FactoryEventReconnectCursor,
	interfaces.FactoryEventReconnectScope,
) (*interfaces.FactoryEventStream, error) {
	return &interfaces.FactoryEventStream{Events: make(chan interfaces.FactoryEvent)}, nil
}

func (engine *runtimeMetricsTestEngine) GetEngineStateSnapshot(
	context.Context,
) (*interfaces.EngineStateSnapshot[factoryruntime.PetriMarkingSnapshot, *factoryruntime.RuntimeNet], error) {
	return engine.snapshot, nil
}

func (*runtimeMetricsTestEngine) MoveWork(
	context.Context,
	string,
	string,
	work.WorkStateChangeSource,
	string,
) (work.OperatorMoveResult, error) {
	return work.OperatorMoveResult{}, nil
}

func (*runtimeMetricsTestEngine) Pause(context.Context) error  { return nil }
func (*runtimeMetricsTestEngine) Resume(context.Context) error { return nil }
func (*runtimeMetricsTestEngine) GetFactoryEvents(context.Context) ([]interfaces.FactoryEvent, error) {
	return nil, nil
}
func (*runtimeMetricsTestEngine) WaitToComplete() <-chan struct{} {
	completed := make(chan struct{})
	close(completed)
	return completed
}
func (*runtimeMetricsTestEngine) Run(context.Context) error { return nil }

var _ Engine = (*runtimeMetricsTestEngine)(nil)
var _ factoryruntime.RuntimeMetricsSink = (*runtimeMemoryTestSink)(nil)

type runtimeMemorySnapshotBarrierSink struct {
	mu             sync.Mutex
	names          []string
	closed         bool
	seventhStarted chan struct{}
	releaseSeventh chan struct{}
}

func (*runtimeMemorySnapshotBarrierSink) Counter(context.Context, string, float64, factoryruntime.Fields) error {
	return nil
}

func (*runtimeMemorySnapshotBarrierSink) Gauge(context.Context, string, float64, factoryruntime.Fields) error {
	return nil
}

func (sink *runtimeMemorySnapshotBarrierSink) Sample(
	_ context.Context,
	name string,
	_ float64,
	_ string,
	_ factoryruntime.Fields,
) error {
	if name == factoryruntime.RuntimeMemoryProcessCommitAvailable {
		close(sink.seventhStarted)
		<-sink.releaseSeventh
	}
	sink.mu.Lock()
	defer sink.mu.Unlock()
	if sink.closed {
		return errors.New("runtime metrics sink closed before snapshot completed")
	}
	sink.names = append(sink.names, name)
	return nil
}

func (sink *runtimeMemorySnapshotBarrierSink) Close() error {
	sink.mu.Lock()
	defer sink.mu.Unlock()
	sink.closed = true
	return nil
}

func (*runtimeMemorySnapshotBarrierSink) Path() string { return "memory://runtime-metrics" }
func (*runtimeMemorySnapshotBarrierSink) Artifact() factoryruntime.RuntimeMetricsArtifact {
	return factoryruntime.RuntimeMetricsArtifact{Path: "memory://runtime-metrics"}
}

var _ factoryruntime.RuntimeMetricsSink = (*runtimeMemorySnapshotBarrierSink)(nil)

type lateMetricEmissionSink struct {
	mu           sync.Mutex
	closed       bool
	counterCalls int
	closeStarted chan struct{}
	releaseClose chan struct{}
}

func (sink *lateMetricEmissionSink) Counter(context.Context, string, float64, factoryruntime.Fields) error {
	sink.mu.Lock()
	defer sink.mu.Unlock()
	sink.counterCalls++
	if sink.closed {
		return errors.New("late metric reached closed sink")
	}
	return nil
}

func (*lateMetricEmissionSink) Gauge(context.Context, string, float64, factoryruntime.Fields) error {
	return nil
}

func (*lateMetricEmissionSink) Sample(context.Context, string, float64, string, factoryruntime.Fields) error {
	return nil
}

func (sink *lateMetricEmissionSink) Close() error {
	close(sink.closeStarted)
	<-sink.releaseClose
	sink.mu.Lock()
	sink.closed = true
	sink.mu.Unlock()
	return nil
}

func (*lateMetricEmissionSink) Path() string { return "memory://late-metrics" }
func (*lateMetricEmissionSink) Artifact() factoryruntime.RuntimeMetricsArtifact {
	return factoryruntime.RuntimeMetricsArtifact{Path: "memory://late-metrics"}
}

var _ factoryruntime.RuntimeMetricsSink = (*lateMetricEmissionSink)(nil)
