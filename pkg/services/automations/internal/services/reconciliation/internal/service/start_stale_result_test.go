package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	automations "github.com/portpowered/infinite-you/pkg/services/automations"
	reconciliation "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/reconciliation"
	reconciliationwire "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/reconciliation/wire"
)

func TestStartSourceSuccessDoesNotReportStartingAfterSupersedingStop(t *testing.T) {
	t.Parallel()

	identity := sourceIdentity("stale-start-success")
	effects := newBlockingStartEffects(nil)
	service := reconciliationwire.NewService(effects)
	results, errs := startSourceAsync(service, identity)
	<-effects.entered

	if _, err := service.StopSource(
		context.Background(),
		automations.StopSourceRequest{Identity: identity},
	); err != nil {
		t.Fatalf("StopSource superseding start: %v", err)
	}
	stopped, err := service.WaitSource(
		context.Background(),
		automations.WaitSourceRequest{
			Identity: identity,
			Desired:  automations.DesiredLifecycleStopped,
		},
	)
	if err != nil {
		t.Fatalf("WaitSource stopped during start effect: %v", err)
	}
	assertLifecycle(
		t, stopped.Outcome, automations.DesiredLifecycleStopped,
		automations.ObservedLifecycleStopped, automations.ConvergenceStatusConverged, false,
	)
	assertStatus(t, service, identity, stopped.Outcome.Observation.InstanceID, automations.ObservedLifecycleStopped)

	close(effects.release)
	if err := <-errs; err != nil {
		t.Fatalf("stale StartSource success: %v", err)
	}
	started := <-results
	assertLifecycle(
		t, started.Outcome, automations.DesiredLifecycleRunning,
		automations.ObservedLifecycleStopped, automations.ConvergenceStatusProgressing, true,
	)
	assertStatus(t, service, identity, stopped.Outcome.Observation.InstanceID, automations.ObservedLifecycleStopped)
}

func TestStartSourceFailureDoesNotOverwriteSupersedingStop(t *testing.T) {
	t.Parallel()

	identity := sourceIdentity("superseded-start-failure")
	effects := newBlockingStartEffects(errors.New("late start failure"))
	service := reconciliationwire.NewService(effects)
	results, errs := startSourceAsync(service, identity)
	<-effects.entered

	if _, err := service.StopSource(
		context.Background(),
		automations.StopSourceRequest{Identity: identity},
	); err != nil {
		t.Fatalf("StopSource superseding start: %v", err)
	}
	stopped, err := service.WaitSource(
		context.Background(),
		automations.WaitSourceRequest{
			Identity: identity,
			Desired:  automations.DesiredLifecycleStopped,
		},
	)
	if err != nil {
		t.Fatalf("WaitSource stopped during start effect: %v", err)
	}
	assertLifecycle(
		t, stopped.Outcome, automations.DesiredLifecycleStopped,
		automations.ObservedLifecycleStopped, automations.ConvergenceStatusConverged, false,
	)

	close(effects.release)
	if err := <-errs; err != nil {
		t.Fatalf("stale StartSource failure: %v", err)
	}
	assertLifecycle(
		t, (<-results).Outcome, automations.DesiredLifecycleRunning,
		automations.ObservedLifecycleStopped, automations.ConvergenceStatusProgressing, true,
	)
}

func TestStartSourceCancellationDoesNotOverwriteNewerRunningObservation(t *testing.T) {
	t.Parallel()

	identity := sourceIdentity("stale-start-cancellation")
	effects := newBlockingStartEffects(context.Canceled)
	service := reconciliationwire.NewService(effects)
	results, errs := startSourceAsync(service, identity)
	<-effects.entered

	running, err := service.WaitSource(
		context.Background(),
		automations.WaitSourceRequest{
			Identity: identity,
			Desired:  automations.DesiredLifecycleRunning,
		},
	)
	if err != nil {
		t.Fatalf("WaitSource running during start effect: %v", err)
	}
	assertLifecycle(
		t, running.Outcome, automations.DesiredLifecycleRunning,
		automations.ObservedLifecycleRunning, automations.ConvergenceStatusConverged, false,
	)

	close(effects.release)
	if err := <-errs; err != nil {
		t.Fatalf("stale StartSource cancellation: %v", err)
	}
	assertLifecycle(
		t, (<-results).Outcome, automations.DesiredLifecycleRunning,
		automations.ObservedLifecycleRunning, automations.ConvergenceStatusConverged, true,
	)
}

func startSourceAsync(
	service reconciliation.Service,
	identity automations.SourceIdentity,
) (<-chan automations.StartSourceResult, <-chan error) {
	results := make(chan automations.StartSourceResult, 1)
	errs := make(chan error, 1)
	go func() {
		result, err := service.StartSource(
			context.Background(),
			automations.StartSourceRequest{Identity: identity, Kind: "schedule"},
		)
		results <- result
		errs <- err
	}()
	return results, errs
}

type blockingStartEffects struct {
	entered chan struct{}
	release chan struct{}
	err     error
}

func newBlockingStartEffects(err error) *blockingStartEffects {
	return &blockingStartEffects{
		entered: make(chan struct{}, 1),
		release: make(chan struct{}),
		err:     err,
	}
}

func (f *blockingStartEffects) Start(context.Context, reconciliation.StartEffect) error {
	f.entered <- struct{}{}
	<-f.release
	return f.err
}

func (*blockingStartEffects) Stop(context.Context, reconciliation.StopEffect) error { return nil }

func (*blockingStartEffects) Wait(ctx context.Context, effect reconciliation.WaitEffect) (automations.SourceObservation, error) {
	return convergedWait(ctx, effect)
}

func TestRuntimeSourceControlStaleStartDoesNotOverwriteStopOrPeer(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name    string
		lateErr error
	}{{"success", nil}, {"failure", errors.New("late failure")}} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			identity := sourceIdentity("shared-stale-start")
			entered, release := make(chan struct{}), make(chan struct{})
			service := reconciliationwire.NewService(lifecycleFixture{
				start: func(ctx context.Context, effect reconciliation.StartEffect) error {
					if effect.RuntimeID != "A" {
						return nil
					}
					close(entered)
					select {
					case <-ctx.Done():
						return ctx.Err()
					case <-release:
						return test.lateErr
					}
				},
				stop: func(_ context.Context, effect reconciliation.StopEffect) error {
					if effect.RuntimeID != "A" {
						return errors.New("stop routed to peer")
					}
					return nil
				},
				wait: convergedWait,
			})
			results := make(chan automations.StartSourceResult, 1)
			errs := make(chan error, 1)
			request := automations.StartSourceRequest{Identity: identity, Kind: "schedule"}
			go func() {
				result, err := service.StartSourceForRuntime(ctx, "A", request)
				results <- result
				errs <- err
			}()
			select {
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			case <-entered:
			}
			if _, err := service.StartSourceForRuntime(ctx, "B", request); err != nil {
				t.Fatal(err)
			}
			waitScopedSource(t, service, "B", identity, automations.DesiredLifecycleRunning, "")
			if _, err := service.StopSourceForRuntime(ctx, "A", automations.StopSourceRequest{Identity: identity}); err != nil {
				t.Fatal(err)
			}
			waitScopedSource(t, service, "A", identity, automations.DesiredLifecycleStopped, "")
			close(release)
			select {
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			case err := <-errs:
				if err != nil {
					t.Fatal(err)
				}
			}
			assertLifecycle(t, (<-results).Outcome, automations.DesiredLifecycleRunning, automations.ObservedLifecycleStopped, automations.ConvergenceStatusProgressing, true)
			peer := waitScopedSource(t, service, "B", identity, automations.DesiredLifecycleRunning, "")
			if !peer.Outcome.Idempotent {
				t.Fatal("stale A completion disturbed B")
			}
		})
	}
}

func TestRuntimeSourceControlStaleWaitDoesNotOverwriteStopOrPeer(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name string
		err  error
	}{{"success", nil}, {"failure", errors.New("late wait failure")}, {"cancelled", context.Canceled}} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			assertStaleRuntimeWait(t, test.err)
		})
	}
}

func assertStaleRuntimeWait(t *testing.T, lateErr error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	identity := sourceIdentity("shared-stale-wait")
	entered, release := make(chan struct{}), make(chan struct{})
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
	})
	service := reconciliationwire.NewService(staleWaitLifecycle(entered, release, lateErr))
	request := automations.StartSourceRequest{Identity: identity, Kind: "schedule"}
	for _, runtimeID := range []string{"A", "B"} {
		if _, err := service.StartSourceForRuntime(ctx, runtimeID, request); err != nil {
			t.Fatal(err)
		}
	}
	results := make(chan automations.WaitSourceResult, 1)
	errs := make(chan error, 1)
	go func() {
		result, err := service.WaitSourceForRuntime(ctx, "A", automations.WaitSourceRequest{Identity: identity, Desired: automations.DesiredLifecycleRunning})
		results <- result
		errs <- err
	}()
	select {
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	case <-entered:
	}
	peer := waitScopedSource(t, service, "B", identity, automations.DesiredLifecycleRunning, "")
	if _, err := service.StopSourceForRuntime(ctx, "A", automations.StopSourceRequest{Identity: identity}); err != nil {
		t.Fatal(err)
	}
	stopped := waitScopedSource(t, service, "A", identity, automations.DesiredLifecycleStopped, "")
	close(release)
	assertStaleWaitResult(t, ctx, results, errs, stopped.Outcome.Observation)
	if current := waitScopedSource(t, service, "B", identity, automations.DesiredLifecycleRunning, ""); current.Outcome.Observation != peer.Outcome.Observation || !current.Outcome.Idempotent {
		t.Fatal("late A wait disturbed B's authoritative observation")
	}
	if current := waitScopedSource(t, service, "A", identity, automations.DesiredLifecycleStopped, ""); current.Outcome.Observation != stopped.Outcome.Observation || !current.Outcome.Idempotent {
		t.Fatal("late A wait disturbed A's stopped observation")
	}
}

func staleWaitLifecycle(entered, release chan struct{}, lateErr error) lifecycleFixture {
	return lifecycleFixture{
		start: func(context.Context, reconciliation.StartEffect) error { return nil },
		stop:  func(context.Context, reconciliation.StopEffect) error { return nil },
		wait: func(ctx context.Context, effect reconciliation.WaitEffect) (automations.SourceObservation, error) {
			if effect.RuntimeID == "A" && effect.Desired == automations.DesiredLifecycleRunning {
				close(entered)
				select {
				case <-ctx.Done():
					return automations.SourceObservation{}, ctx.Err()
				case <-release:
				}
			}
			observation, err := convergedWait(ctx, effect)
			if effect.RuntimeID == "A" && effect.Desired == automations.DesiredLifecycleRunning {
				return observation, lateErr
			}
			return observation, err
		},
	}
}

func assertStaleWaitResult(t *testing.T, ctx context.Context, results <-chan automations.WaitSourceResult, errs <-chan error, stopped automations.SourceObservation) {
	t.Helper()
	select {
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	case err := <-errs:
		if err != nil {
			t.Fatalf("superseded wait error = %v, want current authoritative result", err)
		}
	}
	result := <-results
	assertLifecycle(t, result.Outcome, automations.DesiredLifecycleRunning, automations.ObservedLifecycleStopped, automations.ConvergenceStatusProgressing, true)
	if result.Outcome.Observation != stopped {
		t.Fatalf("superseded wait observation = %+v, want %+v", result.Outcome.Observation, stopped)
	}
}

func TestRuntimeSourceControlStaleStopDoesNotOverwriteRestartOrPeer(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name string
		err  error
	}{{"success", nil}, {"failure", errors.New("late stop failure")}, {"cancelled", context.Canceled}} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			assertStaleRuntimeStop(t, test.err)
		})
	}
}

func assertStaleRuntimeStop(t *testing.T, lateErr error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	identity := sourceIdentity("shared-stale-stop")
	entered, release := make(chan struct{}), make(chan struct{})
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
	})
	service := reconciliationwire.NewService(staleStopLifecycle(entered, release, lateErr))
	request := automations.StartSourceRequest{Identity: identity, Kind: "schedule"}
	for _, runtimeID := range []string{"A", "B"} {
		if _, err := service.StartSourceForRuntime(ctx, runtimeID, request); err != nil {
			t.Fatal(err)
		}
		waitScopedSource(t, service, runtimeID, identity, automations.DesiredLifecycleRunning, "")
	}
	results := make(chan automations.StopSourceResult, 1)
	errs := make(chan error, 1)
	go func() {
		result, err := service.StopSourceForRuntime(ctx, "A", automations.StopSourceRequest{Identity: identity})
		results <- result
		errs <- err
	}()
	select {
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	case <-entered:
	}
	peer := waitScopedSource(t, service, "B", identity, automations.DesiredLifecycleRunning, "")
	waitScopedSource(t, service, "A", identity, automations.DesiredLifecycleStopped, "")
	if _, err := service.StartSourceForRuntime(ctx, "A", request); err != nil {
		t.Fatal(err)
	}
	running := waitScopedSource(t, service, "A", identity, automations.DesiredLifecycleRunning, "")
	close(release)
	select {
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	case err := <-errs:
		if err != nil {
			t.Fatalf("superseded stop error = %v, want current authoritative result", err)
		}
	}
	result := <-results
	assertLifecycle(t, result.Outcome, automations.DesiredLifecycleStopped, automations.ObservedLifecycleRunning, automations.ConvergenceStatusProgressing, true)
	if result.Outcome.Observation != running.Outcome.Observation {
		t.Fatalf("late stop observation = %+v, want %+v", result.Outcome.Observation, running.Outcome.Observation)
	}
	for runtimeID, expected := range map[string]automations.SourceObservation{"A": running.Outcome.Observation, "B": peer.Outcome.Observation} {
		current := waitScopedSource(t, service, runtimeID, identity, automations.DesiredLifecycleRunning, "")
		if current.Outcome.Observation != expected || !current.Outcome.Idempotent {
			t.Fatalf("late A stop disturbed %s's authoritative observation", runtimeID)
		}
	}
}

func staleStopLifecycle(entered, release chan struct{}, lateErr error) lifecycleFixture {
	return lifecycleFixture{
		start: func(context.Context, reconciliation.StartEffect) error { return nil },
		stop: func(ctx context.Context, effect reconciliation.StopEffect) error {
			if effect.RuntimeID != "A" {
				return errors.New("stop routed to peer")
			}
			close(entered)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-release:
				return lateErr
			}
		},
		wait: convergedWait,
	}
}
