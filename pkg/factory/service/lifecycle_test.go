package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/work"

	"github.com/jonboulle/clockwork"
	"github.com/portpowered/infinite-you/pkg/factory"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factoryservice "github.com/portpowered/infinite-you/pkg/factory/service"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/orchestrators/petri"
	platformmetrics "github.com/portpowered/infinite-you/pkg/platform/metrics"
	"go.uber.org/zap"
)

type lifecycleObserverFactory struct {
	mu          sync.RWMutex
	engineState *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]
}

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
	if !(*factoryservice.Handle)(nil).Completed() {
		t.Fatal("nil Handle should report completed")
	}
	if err := (*factoryservice.Handle)(nil).Wait(); err != nil {
		t.Fatalf("nil Handle wait error = %v, want nil", err)
	}

	handle := &factoryservice.Handle{RunDone: make(chan struct{})}
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

	outcome, reason := factoryservice.RuntimeStopOutcome(finished, nil, true)
	if outcome != "completed" || reason != "" {
		t.Fatalf("RuntimeStopOutcome(finished, nil, forcedCancel=true) = (%q, %q), want (completed, \"\")", outcome, reason)
	}

	outcome, reason = factoryservice.RuntimeStopOutcome(active, context.Canceled, false)
	if outcome != "canceled" || reason != "" {
		t.Fatalf("RuntimeStopOutcome(active, context.Canceled, false) = (%q, %q), want (canceled, \"\")", outcome, reason)
	}

	outcome, reason = factoryservice.RuntimeStopOutcome(active, nil, true)
	if outcome != "canceled" || reason != "" {
		t.Fatalf("RuntimeStopOutcome(active, nil, forcedCancel=true) = (%q, %q), want (canceled, \"\")", outcome, reason)
	}
}

func TestStop_EmitsCompletedLifecycleMetricWithoutRootService(t *testing.T) {
	metricsSink, err := platformmetrics.BuildRuntimeMetricsSink(
		"session-shutdown",
		"runtime-shutdown",
		"/factory",
		"/factory/current",
		t.TempDir(),
		platformmetrics.RuntimeMetricsConfig{},
	)
	if err != nil {
		t.Fatalf("BuildRuntimeMetricsSink: %v", err)
	}

	factoryStub := &lifecycleObserverFactory{}
	factoryStub.setEngineState(&interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		RuntimeStatus: interfaces.RuntimeStatusFinished,
		FactoryState:  string(interfaces.FactoryStateRunning),
	})

	runCtx, runCancel := context.WithCancel(context.Background())
	t.Cleanup(runCancel)

	handle := &factoryservice.Handle{
		Bundle: &factoryservice.Bundle{
			Factory:     factoryStub,
			MetricsSink: metricsSink,
			Logger:      zap.NewNop(),
		},
		RunDone:   make(chan struct{}),
		RunCancel: runCancel,
	}

	observerCtx, cancelObserver := context.WithCancel(runCtx)
	defer cancelObserver()
	go factoryservice.ObserveRuntimeMetrics(observerCtx, handle)

	handle.SetRunResult(nil)
	if err := factoryservice.Stop(handle, clockwork.NewFakeClock()); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	waitForLifecycleMetricsRecord(t, metricsSink.Path(), time.Second, func(record map[string]any) bool {
		return metricNameAndValue(record, "runtime.lifecycle.stopped", 1) &&
			metricRecordString(record, "outcome") == "completed"
	}, "completed stop through extracted host")
}

func TestStop_CancelsAndJoinsSidecarsBeforeStoppingRunLoop(t *testing.T) {
	sidecarExited := make(chan struct{})
	runStoppedTooEarly := make(chan struct{})
	factoryStub := &orderedStopFactory{
		sidecarExited:      sidecarExited,
		runStoppedTooEarly: runStoppedTooEarly,
	}
	handle := factoryservice.Start(context.Background(), &factoryservice.Bundle{
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

	err := factoryservice.Stop(handle, clockwork.NewFakeClock())
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

	handle := factoryservice.Start(context.Background(), &factoryservice.Bundle{
		Factory: factoryStub,
		Logger:  zap.NewNop(),
	})
	if handle == nil {
		t.Fatal("Start returned nil handle")
	}
	if err := factoryservice.WaitForStart(context.Background(), handle); err != nil {
		t.Fatalf("WaitForStart: %v", err)
	}
	handle.CancelRun()
	if err := factoryservice.Stop(handle, factory.EnsureClock(clockwork.NewFakeClock())); err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("Stop: %v", err)
	}
}

func waitForLifecycleMetricsRecord(
	t *testing.T,
	path string,
	wait time.Duration,
	predicate func(map[string]any) bool,
	label string,
) {
	t.Helper()
	deadline := time.Now().Add(wait)
	for time.Now().Before(deadline) {
		records := readLifecycleMetricsRecords(t, path)
		for _, record := range records {
			if predicate(record) {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for runtime metrics record %q at %s", label, path)
}

func readLifecycleMetricsRecords(t *testing.T, path string) []map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		t.Fatalf("read runtime metrics %q: %v", path, err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	records := make([]map[string]any, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("decode runtime metrics line %q: %v", line, err)
		}
		records = append(records, record)
	}
	return records
}

func metricNameAndValue(record map[string]any, name string, value float64) bool {
	if record == nil {
		return false
	}
	if metricRecordString(record, "metric_name") != name {
		return false
	}
	switch typed := record["value"].(type) {
	case float64:
		return typed == value
	case int:
		return float64(typed) == value
	default:
		return false
	}
}

func metricRecordString(record map[string]any, key string) string {
	if record == nil {
		return ""
	}
	value, ok := record[key].(string)
	if !ok {
		return ""
	}
	return value
}
