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
