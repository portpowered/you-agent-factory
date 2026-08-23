package host

import (
	"context"
	"sync"
	"testing"
	"time"

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
