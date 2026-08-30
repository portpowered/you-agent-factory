package host

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
)

// Handle hosts one running factory runtime bundle and coordinates its run loop.
type Handle struct {
	Bundle *Bundle

	RunCancel context.CancelFunc
	RunDone   chan struct{}

	SidecarCancel context.CancelFunc
	Sidecars      sync.WaitGroup
	SidecarMu     sync.Mutex

	runErrMu             sync.RWMutex
	runErr               error
	lifecycleMetricsOnce sync.Once
}

func (h *Handle) RuntimeInstance() factory.RuntimeRecord {
	if h == nil {
		return nil
	}
	return h.Bundle
}

// Start launches the hosted runtime run loop for bundle without blocking on readiness.
func Start(ctx context.Context, bundle *Bundle) *Handle {
	if bundle == nil {
		return nil
	}
	runCtx, runCancel := context.WithCancel(ctx)
	handle := &Handle{
		Bundle:    bundle,
		RunCancel: runCancel,
		RunDone:   make(chan struct{}),
	}
	if bundle.Recording != nil {
		bundle.Recording.Start(runCtx)
		if err := bundle.Recording.Flush(); err != nil {
			runCancel()
			bundle.Recording.Stop()
			handle.setRunResult(errors.Join(err, bundle.Recording.Err()))
			return handle
		}
	}
	bundle.EmitRuntimeLifecycleStart()
	go func() {
		err := bundle.Factory.Run(runCtx)
		if err == nil && runCtx.Err() != nil {
			err = context.Canceled
		}
		handle.setRunResult(err)
	}()
	return handle
}

// Completed reports whether the hosted run loop has finished.
func (h *Handle) Completed() bool {
	if h == nil {
		return true
	}
	select {
	case <-h.RunDone:
		return true
	default:
		return false
	}
}

// Result returns the hosted run loop result after completion.
func (h *Handle) Result() error {
	if h == nil {
		return nil
	}
	h.runErrMu.RLock()
	defer h.runErrMu.RUnlock()
	return h.runErr
}

// SetRunResult records the hosted run loop result and unblocks waiters.
// It is exported for tests that simulate run-loop completion without starting Factory.Run.
func (h *Handle) SetRunResult(err error) {
	h.setRunResult(err)
}

func (h *Handle) setRunResult(err error) {
	if h == nil {
		return
	}
	h.runErrMu.Lock()
	h.runErr = err
	h.runErrMu.Unlock()
	close(h.RunDone)
}

// Wait blocks until the hosted run loop completes and returns its result.
func (h *Handle) Wait() error {
	if h == nil {
		return nil
	}
	<-h.RunDone
	return h.Result()
}

// CancelRun requests cancellation of the hosted run loop when it is still active.
func (h *Handle) CancelRun() {
	if h == nil || h.RunCancel == nil || h.Completed() {
		return
	}
	h.RunCancel()
}

// RunDoneCh exposes the run-completion channel for callers that multiplex shutdown.
func (h *Handle) RunDoneCh() <-chan struct{} {
	if h == nil {
		ch := make(chan struct{})
		close(ch)
		return ch
	}
	return h.RunDone
}

// LifecycleMetricsOnce exposes the stop-metrics guard used by lifecycle observers.
func (h *Handle) LifecycleMetricsOnce() *sync.Once {
	if h == nil {
		return nil
	}
	return &h.lifecycleMetricsOnce
}

const (
	runtimeMetricsObserverPollInterval = 5 * time.Millisecond
	runtimeMemoryObservationInterval   = 10 * time.Second
)

type runtimeMetricsObservation struct {
	runtimeStatus interfaces.RuntimeStatus
	factoryState  interfaces.FactoryState
	inFlightCount int
	initialized   bool
}

type runtimeMetricsObserver struct {
	last           runtimeMetricsObservation
	lastMemoryAt   time.Time
	memoryObserved bool
}

// WaitForStart blocks until the hosted runtime reports running readiness or fails early.
func WaitForStart(ctx context.Context, handle *Handle) error {
	if handle == nil || handle.Bundle == nil {
		return fmt.Errorf("runtime handle is required")
	}

	startCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-startCtx.Done():
			if handle.Completed() {
				return startupResult(handle.Result())
			}
			return startCtx.Err()
		case <-handle.RunDone:
			return startupResult(handle.Result())
		case <-ticker.C:
			// Startup readiness is deliberately checked through the aggregate
			// engine boundary. Unlike lifecycle telemetry, this handshake must
			// observe the hosted engine's complete readiness state before the
			// transport component is allowed to start.
			snap, err := handle.Bundle.Factory.GetEngineStateSnapshot(context.Background())
			if err != nil {
				continue
			}
			if snap.FactoryState == string(interfaces.FactoryStateRunning) {
				return nil
			}
		}
	}
}

// startupResult lets the process lifecycle expose a finite run's terminal
// result through its primary transport. An incomplete drain is a valid runtime
// startup boundary: the listener still needs to start and join before the
// CLI reports the non-successful run outcome.
func startupResult(err error) error {
	if errors.Is(err, factory.ErrIncompleteDrain) {
		return nil
	}
	return err
}

// Stop cancels and joins session sidecars before stopping the hosted run loop,
// then finalizes lifecycle metrics and closes replay, log, and metrics artifacts.
func Stop(handle *Handle, clock factory.Clock) error {
	if handle == nil {
		return nil
	}
	if clock == nil {
		return fmt.Errorf("stop Factory Runtime: clock is required")
	}
	StopSidecars(handle)
	handle.CancelRun()
	runErr := handle.Wait()
	finalizeRuntimeLifecycleMetrics(handle, runtimeMetricsObservation{})
	finalizationErr := FinalizeArtifacts(handle.Bundle, clock)
	closeRuntimeEventSubscriptions(handle.Bundle)
	return errors.Join(runErr, finalizationErr)
}

// FinalizeArtifacts finishes replay recording and closes runtime log and metrics sinks.
func FinalizeArtifacts(bundle *Bundle, clock factory.Clock) error {
	if bundle == nil {
		return nil
	}
	if clock == nil {
		return fmt.Errorf("finalize Factory Runtime artifacts: clock is required")
	}
	var errs []error
	if bundle.Recording != nil {
		if err := bundle.Recording.Finalize(clock.Now().UTC()); err != nil {
			errs = append(errs, err)
		}
	}
	if bundle.LogSink != nil {
		if err := bundle.LogSink.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if bundle.MetricsSink != nil {
		if err := bundle.closeMetricsSink(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func closeRuntimeEventSubscriptions(bundle *Bundle) {
	if bundle == nil || bundle.EventHistory == nil {
		return
	}
	if closer, ok := bundle.EventHistory.(interface{ CloseLiveSubscriptions() }); ok {
		closer.CloseLiveSubscriptions()
	}
}

func (bundle *Bundle) closeMetricsSink() error {
	if bundle == nil || bundle.MetricsSink == nil {
		return nil
	}
	bundle.metricsMu.Lock()
	defer bundle.metricsMu.Unlock()
	bundle.metricsClosed = true
	return errors.Join(bundle.metricsError(), bundle.MetricsSink.Close())
}

// CloseBundleSinks closes runtime log and metrics sinks created during bundle build.
func CloseBundleSinks(logSink factory.RuntimeLogSink, metricsSink factory.RuntimeMetricsSink) error {
	var errs []error
	if logSink != nil {
		if err := logSink.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if metricsSink != nil {
		if err := metricsSink.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// ObserveRuntimeMetrics polls engine snapshots and emits runtime state gauges until the
// hosted run loop completes or observerCtx is canceled.
func ObserveRuntimeMetrics(
	observerCtx context.Context,
	handle *Handle,
	clock platformclock.TimerSource,
) {
	observeRuntimeMetrics(observerCtx, handle, clock)
}

func observeRuntimeMetrics(
	observerCtx context.Context,
	handle *Handle,
	clock platformclock.TimerSource,
) {
	if handle == nil || handle.Bundle == nil || handle.Bundle.Factory == nil || clock == nil {
		return
	}
	observer := runtimeMetricsObserver{}
	observer.observe(handle, clock.Now())
	for {
		timer := clock.NewTimer(runtimeMetricsObserverPollInterval)
		select {
		case <-handle.RunDone:
			timer.Stop()
			finalizeRuntimeLifecycleMetrics(handle, observer.last)
			return
		case <-observerCtx.Done():
			timer.Stop()
			// Temporary sidecar shutdown (for example during session runtime replacement)
			// must not block on runDone or emit lifecycle stop metrics; Stop finalizes
			// lifecycle telemetry after the runtime actually exits.
			return
		case observedAt := <-timer.C():
			select {
			case <-handle.RunDone:
				finalizeRuntimeLifecycleMetrics(handle, observer.last)
				return
			default:
			}
			observer.observe(handle, observedAt)
		}
	}
}

func (o *runtimeMetricsObserver) observe(handle *Handle, observedAt time.Time) {
	if o == nil || handle == nil || handle.Bundle == nil || handle.Bundle.Factory == nil {
		return
	}
	snapshot, err := runtimeLifecycleSnapshot(context.Background(), handle.Bundle.Factory)
	if err == nil {
		current := metricsObservationFromSnapshot(snapshot)
		if current.changedFrom(o.last) {
			handle.Bundle.EmitRuntimeStateMetrics(snapshot)
			o.last = current
		}
	}
	o.observeMemory(handle.Bundle, observedAt)
}

func (o *runtimeMetricsObserver) observeMemory(bundle *Bundle, observedAt time.Time) {
	if o == nil || bundle == nil {
		return
	}
	if o.memoryObserved && observedAt.Before(o.lastMemoryAt.Add(runtimeMemoryObservationInterval)) {
		return
	}
	bundle.EmitRuntimeMemoryMetrics()
	o.lastMemoryAt = observedAt
	o.memoryObserved = true
}

// RuntimeStopOutcome derives lifecycle-stop labels from the terminal engine snapshot and run error.
func RuntimeStopOutcome(
	snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net],
	err error,
	forcedCancel bool,
) (string, string) {
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return "canceled", ""
		}
		return "failed", err.Error()
	}
	if snapshot != nil && snapshot.FactoryState == string(interfaces.FactoryStateFailed) {
		return "failed", ""
	}
	if snapshot != nil && snapshot.RuntimeStatus == interfaces.RuntimeStatusFinished {
		return "completed", ""
	}
	if forcedCancel {
		return "canceled", ""
	}
	return "completed", ""
}

func metricsObservationFromSnapshot(snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) runtimeMetricsObservation {
	observation := runtimeMetricsObservation{initialized: snapshot != nil}
	if snapshot == nil {
		return observation
	}
	observation.runtimeStatus = snapshot.RuntimeStatus
	observation.factoryState = interfaces.FactoryState(snapshot.FactoryState)
	observation.inFlightCount = snapshot.InFlightCount
	return observation
}

func (o runtimeMetricsObservation) changedFrom(previous runtimeMetricsObservation) bool {
	if !previous.initialized {
		return o.initialized
	}
	if !o.initialized {
		return false
	}
	return o.runtimeStatus != previous.runtimeStatus ||
		o.factoryState != previous.factoryState ||
		o.inFlightCount != previous.inFlightCount
}

// runtimeLifecycleSnapshot keeps lifecycle telemetry on the detached runtime
// boundary when the hosted implementation exposes it. The aggregate engine
// snapshot is intentionally a compatibility fallback: it reconstructs the
// canonical world and transition enablement, which makes stopping a runtime
// increasingly expensive as its event history grows.
func runtimeLifecycleSnapshot(
	ctx context.Context,
	engine Engine,
) (*interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], error) {
	if reader, ok := engine.(interface {
		GetWorkStateSnapshot(context.Context) (*interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], error)
	}); ok {
		return reader.GetWorkStateSnapshot(ctx)
	}
	return engine.GetEngineStateSnapshot(ctx)
}

func finalizeRuntimeLifecycleMetrics(handle *Handle, last runtimeMetricsObservation) {
	if handle == nil || handle.Bundle == nil || handle.Bundle.Factory == nil {
		return
	}
	handle.lifecycleMetricsOnce.Do(func() {
		snapshot, err := runtimeLifecycleSnapshot(context.Background(), handle.Bundle.Factory)
		if err == nil {
			current := metricsObservationFromSnapshot(snapshot)
			if current.changedFrom(last) {
				handle.Bundle.EmitRuntimeStateMetrics(snapshot)
			}
			outcome, reason := RuntimeStopOutcome(snapshot, handle.Result(), false)
			handle.Bundle.EmitRuntimeLifecycleStop(outcome, reason)
			return
		}
		outcome, reason := RuntimeStopOutcome(nil, handle.Result(), false)
		handle.Bundle.EmitRuntimeLifecycleStop(outcome, reason)
	})
}
