package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
)

const runtimeMetricsObserverPollInterval = 5 * time.Millisecond

type runtimeMetricsObservation struct {
	runtimeStatus interfaces.RuntimeStatus
	factoryState  interfaces.FactoryState
	inFlightCount int
	initialized   bool
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
				return handle.Result()
			}
			return startCtx.Err()
		case <-handle.RunDone:
			return handle.Result()
		case <-ticker.C:
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

// Stop cancels the hosted run loop when needed, waits for completion, finalizes lifecycle
// metrics, and closes replay, log, and metrics artifacts.
func Stop(handle *Handle, clock factory.Clock) error {
	if handle == nil {
		return nil
	}
	handle.CancelRun()
	runErr := handle.Wait()
	finalizeRuntimeLifecycleMetrics(handle, runtimeMetricsObservation{})
	return errors.Join(runErr, FinalizeArtifacts(handle.Bundle, clock))
}

// FinalizeArtifacts finishes replay recording and closes runtime log and metrics sinks.
func FinalizeArtifacts(bundle *Bundle, clock factory.Clock) error {
	if bundle == nil {
		return nil
	}
	var errs []error
	if bundle.Recording != nil {
		bundle.Recording.Finish(factory.EnsureClock(clock).Now().UTC())
		if err := bundle.Recording.Flush(); err != nil {
			errs = append(errs, err)
		}
		if err := bundle.Recording.Err(); err != nil {
			errs = append(errs, err)
		}
	}
	if bundle.LogSink != nil {
		if err := bundle.LogSink.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if bundle.MetricsSink != nil {
		if err := bundle.MetricsSink.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// ObserveRuntimeMetrics polls engine snapshots and emits runtime state gauges until the
// hosted run loop completes or observerCtx is canceled.
func ObserveRuntimeMetrics(observerCtx context.Context, handle *Handle) {
	if handle == nil || handle.Bundle == nil || handle.Bundle.Factory == nil {
		return
	}
	ticker := time.NewTicker(runtimeMetricsObserverPollInterval)
	defer ticker.Stop()
	var last runtimeMetricsObservation
	for {
		snapshot, err := handle.Bundle.Factory.GetEngineStateSnapshot(context.Background())
		if err == nil {
			current := metricsObservationFromSnapshot(snapshot)
			if current.changedFrom(last) {
				handle.Bundle.EmitRuntimeStateMetrics(snapshot)
				last = current
			}
		}
		select {
		case <-handle.RunDone:
			finalizeRuntimeLifecycleMetrics(handle, last)
			return
		case <-observerCtx.Done():
			// Temporary sidecar shutdown (for example during session runtime replacement)
			// must not block on runDone or emit lifecycle stop metrics; Stop finalizes
			// lifecycle telemetry after the runtime actually exits.
			return
		case <-ticker.C:
		}
	}
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

func finalizeRuntimeLifecycleMetrics(handle *Handle, last runtimeMetricsObservation) {
	if handle == nil || handle.Bundle == nil || handle.Bundle.Factory == nil {
		return
	}
	handle.lifecycleMetricsOnce.Do(func() {
		snapshot, err := handle.Bundle.Factory.GetEngineStateSnapshot(context.Background())
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
