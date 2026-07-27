package service_test

import (
	"context"
	"errors"
	"testing"

	automations "github.com/portpowered/infinite-you/pkg/services/automations"
	reconciliationwire "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/reconciliation/wire"
)

func TestSourceLifecycleCancellationBeforeStartDoesNotCreateSource(t *testing.T) {
	t.Parallel()

	effects := &recordingEffects{}
	service := reconciliationwire.NewService(effects.bundle())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	identity := sourceIdentity("cancel-before-start")

	result, err := service.StartSource(ctx, automations.StartSourceRequest{
		Identity: identity,
		Kind:     "schedule",
	})
	assertTerminalError(
		t, err, automations.ErrorCodeCancelled, context.Canceled,
	)
	assertLifecycle(
		t,
		result.Outcome,
		automations.DesiredLifecycleRunning,
		automations.ObservedLifecycleCancelled,
		automations.ConvergenceStatusCancelled,
		false,
	)
	if got := effects.counts(); got != (effectCounts{}) {
		t.Fatalf("cancelled start effects = %+v, want none", got)
	}
	_, statusErr := service.SourceStatus(
		context.Background(),
		automations.SourceStatusRequest{Identity: identity},
	)
	assertLifecycleError(
		t, statusErr, automations.ErrorCodeNotFound, automations.ErrNotFound,
	)
}

func TestSourceLifecycleStartCancellationPreservesPendingObservation(t *testing.T) {
	t.Parallel()

	effects := &recordingEffects{startErr: context.Canceled}
	service := reconciliationwire.NewService(effects.bundle())
	identity := sourceIdentity("cancel-start")

	result, err := service.StartSource(
		context.Background(),
		automations.StartSourceRequest{Identity: identity, Kind: "schedule"},
	)
	assertTerminalError(t, err, automations.ErrorCodeCancelled, context.Canceled)
	assertLifecycle(
		t,
		result.Outcome,
		automations.DesiredLifecycleRunning,
		automations.ObservedLifecycleCancelled,
		automations.ConvergenceStatusCancelled,
		false,
	)
	assertStatus(
		t,
		service,
		identity,
		result.Outcome.Observation.InstanceID,
		automations.ObservedLifecyclePending,
	)
	if got := effects.counts(); got != (effectCounts{starts: 1}) {
		t.Fatalf("effects after status = %+v, want one start", got)
	}
}

func TestSourceLifecycleStopCancellationPreservesRunningObservation(t *testing.T) {
	t.Parallel()

	effects := &recordingEffects{
		stopErr: context.Canceled,
		waitStates: []automations.ObservedLifecycleState{
			automations.ObservedLifecycleRunning,
		},
	}
	service := reconciliationwire.NewService(effects.bundle())
	identity := sourceIdentity("cancel-stop")
	startAndWait(t, service, identity)

	result, err := service.StopSource(
		context.Background(),
		automations.StopSourceRequest{Identity: identity},
	)
	assertTerminalError(t, err, automations.ErrorCodeCancelled, context.Canceled)
	assertLifecycle(
		t,
		result.Outcome,
		automations.DesiredLifecycleStopped,
		automations.ObservedLifecycleCancelled,
		automations.ConvergenceStatusCancelled,
		false,
	)
	assertStatus(
		t,
		service,
		identity,
		result.Outcome.Observation.InstanceID,
		automations.ObservedLifecycleRunning,
	)
}

func TestSourceLifecycleWaitCancellationPreservesStartingObservation(t *testing.T) {
	t.Parallel()

	effects := &recordingEffects{waitErr: context.Canceled}
	service := reconciliationwire.NewService(effects.bundle())
	identity := sourceIdentity("cancel-wait")
	started, err := service.StartSource(
		context.Background(),
		automations.StartSourceRequest{Identity: identity, Kind: "schedule"},
	)
	if err != nil {
		t.Fatalf("StartSource: %v", err)
	}

	result, err := service.WaitSource(
		context.Background(),
		automations.WaitSourceRequest{
			Identity: identity,
			Desired:  automations.DesiredLifecycleRunning,
		},
	)
	assertTerminalError(t, err, automations.ErrorCodeCancelled, context.Canceled)
	assertLifecycle(
		t,
		result.Outcome,
		automations.DesiredLifecycleRunning,
		automations.ObservedLifecycleCancelled,
		automations.ConvergenceStatusCancelled,
		false,
	)
	assertStatus(
		t,
		service,
		identity,
		started.Outcome.Observation.InstanceID,
		automations.ObservedLifecycleStarting,
	)
}

func TestSourceLifecycleCancellationBeforeStopAndWaitIsEffectFree(t *testing.T) {
	t.Parallel()

	effects := &recordingEffects{
		waitStates: []automations.ObservedLifecycleState{
			automations.ObservedLifecycleRunning,
		},
	}
	service := reconciliationwire.NewService(effects.bundle())
	identity := sourceIdentity("cancel-before-operations")
	startAndWait(t, service, identity)
	before := effects.counts()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	stopped, stopErr := service.StopSource(
		ctx,
		automations.StopSourceRequest{Identity: identity},
	)
	assertTerminalError(
		t, stopErr, automations.ErrorCodeCancelled, context.Canceled,
	)
	assertLifecycle(
		t,
		stopped.Outcome,
		automations.DesiredLifecycleStopped,
		automations.ObservedLifecycleCancelled,
		automations.ConvergenceStatusCancelled,
		false,
	)

	waited, waitErr := service.WaitSource(
		ctx,
		automations.WaitSourceRequest{
			Identity: identity,
			Desired:  automations.DesiredLifecycleStopped,
		},
	)
	assertTerminalError(
		t, waitErr, automations.ErrorCodeCancelled, context.Canceled,
	)
	assertLifecycle(
		t,
		waited.Outcome,
		automations.DesiredLifecycleStopped,
		automations.ObservedLifecycleCancelled,
		automations.ConvergenceStatusCancelled,
		false,
	)
	if got := effects.counts(); got != before {
		t.Fatalf("pre-cancelled operations effects = %+v, want %+v", got, before)
	}
	assertStatus(
		t,
		service,
		identity,
		stopped.Outcome.Observation.InstanceID,
		automations.ObservedLifecycleRunning,
	)
}

func TestSourceLifecycleEffectFailuresAreStableAndIsolated(t *testing.T) {
	t.Parallel()

	startFailure := errors.New("start unavailable")
	effects := &recordingEffects{startErr: startFailure}
	service := reconciliationwire.NewService(effects.bundle())
	failedIdentity := sourceIdentity("failed")

	failed, err := service.StartSource(
		context.Background(),
		automations.StartSourceRequest{Identity: failedIdentity, Kind: "hosted"},
	)
	assertTerminalError(t, err, automations.ErrorCodeFailed, startFailure)
	assertLifecycle(
		t,
		failed.Outcome,
		automations.DesiredLifecycleRunning,
		automations.ObservedLifecycleFailed,
		automations.ConvergenceStatusFailed,
		false,
	)
	assertStatus(
		t,
		service,
		failedIdentity,
		failed.Outcome.Observation.InstanceID,
		automations.ObservedLifecycleFailed,
	)

	repeated, repeatedErr := service.StartSource(
		context.Background(),
		automations.StartSourceRequest{Identity: failedIdentity, Kind: "hosted"},
	)
	assertTerminalError(t, repeatedErr, automations.ErrorCodeFailed, startFailure)
	assertLifecycle(
		t,
		repeated.Outcome,
		automations.DesiredLifecycleRunning,
		automations.ObservedLifecycleFailed,
		automations.ConvergenceStatusFailed,
		true,
	)
	if got := effects.counts().starts; got != 1 {
		t.Fatalf("repeated failed start effects = %d, want 1", got)
	}

	effects.startErr = nil
	otherIdentity := sourceIdentity("healthy")
	healthy, healthyErr := service.StartSource(
		context.Background(),
		automations.StartSourceRequest{Identity: otherIdentity, Kind: "hosted"},
	)
	if healthyErr != nil {
		t.Fatalf("unrelated StartSource: %v", healthyErr)
	}
	assertLifecycle(
		t,
		healthy.Outcome,
		automations.DesiredLifecycleRunning,
		automations.ObservedLifecycleStarting,
		automations.ConvergenceStatusProgressing,
		false,
	)
	assertStatus(
		t,
		service,
		failedIdentity,
		failed.Outcome.Observation.InstanceID,
		automations.ObservedLifecycleFailed,
	)
}

func TestSourceLifecycleStopFailureIsStableUntilDesiredStateChanges(t *testing.T) {
	t.Parallel()

	stopFailure := errors.New("stop unavailable")
	effects := &recordingEffects{
		stopErr: stopFailure,
		waitStates: []automations.ObservedLifecycleState{
			automations.ObservedLifecycleRunning,
		},
	}
	service := reconciliationwire.NewService(effects.bundle())
	identity := sourceIdentity("stop-failed")
	startAndWait(t, service, identity)

	failed, err := service.StopSource(
		context.Background(),
		automations.StopSourceRequest{Identity: identity},
	)
	assertTerminalError(t, err, automations.ErrorCodeFailed, stopFailure)
	assertLifecycle(
		t,
		failed.Outcome,
		automations.DesiredLifecycleStopped,
		automations.ObservedLifecycleFailed,
		automations.ConvergenceStatusFailed,
		false,
	)

	repeated, repeatedErr := service.StopSource(
		context.Background(),
		automations.StopSourceRequest{Identity: identity},
	)
	assertTerminalError(t, repeatedErr, automations.ErrorCodeFailed, stopFailure)
	if !repeated.Outcome.Idempotent {
		t.Fatalf("repeated stop outcome = %+v, want idempotent", repeated.Outcome)
	}
	if got := effects.counts().stops; got != 1 {
		t.Fatalf("repeated failed stop effects = %d, want 1", got)
	}
}

func TestSourceLifecycleObservedTerminalStateDoesNotRepeatWait(t *testing.T) {
	t.Parallel()

	effects := &recordingEffects{
		waitStates: []automations.ObservedLifecycleState{
			automations.ObservedLifecycleFailed,
		},
	}
	service := reconciliationwire.NewService(effects.bundle())
	identity := sourceIdentity("wait-failed")
	if _, err := service.StartSource(
		context.Background(),
		automations.StartSourceRequest{Identity: identity, Kind: "watcher"},
	); err != nil {
		t.Fatalf("StartSource: %v", err)
	}

	request := automations.WaitSourceRequest{
		Identity: identity,
		Desired:  automations.DesiredLifecycleRunning,
	}
	failed, err := service.WaitSource(context.Background(), request)
	assertTerminalError(t, err, automations.ErrorCodeFailed, automations.ErrSupervisionFailed)
	assertLifecycle(
		t,
		failed.Outcome,
		automations.DesiredLifecycleRunning,
		automations.ObservedLifecycleFailed,
		automations.ConvergenceStatusFailed,
		false,
	)

	repeated, repeatedErr := service.WaitSource(context.Background(), request)
	assertTerminalError(
		t, repeatedErr, automations.ErrorCodeFailed, automations.ErrSupervisionFailed,
	)
	if !repeated.Outcome.Idempotent {
		t.Fatalf("repeated wait outcome = %+v, want idempotent", repeated.Outcome)
	}
	if got := effects.counts().waits; got != 1 {
		t.Fatalf("repeated failed wait effects = %d, want 1", got)
	}
}

func TestSourceLifecycleObservedCancellationIsCommittedAndStable(t *testing.T) {
	t.Parallel()

	effects := &recordingEffects{
		waitStates: []automations.ObservedLifecycleState{
			automations.ObservedLifecycleCancelled,
		},
	}
	service := reconciliationwire.NewService(effects.bundle())
	identity := sourceIdentity("wait-cancelled")
	started, err := service.StartSource(
		context.Background(),
		automations.StartSourceRequest{Identity: identity, Kind: "watcher"},
	)
	if err != nil {
		t.Fatalf("StartSource: %v", err)
	}

	request := automations.WaitSourceRequest{
		Identity: identity,
		Desired:  automations.DesiredLifecycleRunning,
	}
	cancelled, err := service.WaitSource(context.Background(), request)
	assertTerminalError(t, err, automations.ErrorCodeCancelled, context.Canceled)
	assertLifecycle(
		t,
		cancelled.Outcome,
		automations.DesiredLifecycleRunning,
		automations.ObservedLifecycleCancelled,
		automations.ConvergenceStatusCancelled,
		false,
	)
	assertStatus(
		t,
		service,
		identity,
		started.Outcome.Observation.InstanceID,
		automations.ObservedLifecycleCancelled,
	)

	repeated, repeatedErr := service.WaitSource(context.Background(), request)
	assertTerminalError(
		t, repeatedErr, automations.ErrorCodeCancelled, context.Canceled,
	)
	if !repeated.Outcome.Idempotent {
		t.Fatalf("repeated wait outcome = %+v, want idempotent", repeated.Outcome)
	}
	if got := effects.counts().waits; got != 1 {
		t.Fatalf("repeated cancelled wait effects = %d, want 1", got)
	}
}

func assertTerminalError(
	t *testing.T,
	err error,
	code automations.ErrorCode,
	cause error,
) {
	t.Helper()
	var typed *automations.Error
	if !errors.As(err, &typed) ||
		typed.Code != code ||
		!errors.Is(err, cause) {
		t.Fatalf(
			"error = %T %v, want code %q wrapping %v",
			err,
			err,
			code,
			cause,
		)
	}
}
