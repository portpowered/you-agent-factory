package service_test

import (
	"context"
	"errors"
	"testing"

	automations "github.com/portpowered/infinite-you/pkg/services/automations"
	reconciliation "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/reconciliation"
	reconciliationwire "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/reconciliation/wire"
)

func TestStartSourceSuccessDoesNotReportStartingAfterSupersedingStop(t *testing.T) {
	t.Parallel()

	identity := sourceIdentity("stale-start-success")
	effects := newBlockingStartEffects(nil)
	service := reconciliationwire.NewService(effects.bundle())
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
	service := reconciliationwire.NewService(effects.bundle())
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
	service := reconciliationwire.NewService(effects.bundle())
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

func (f *blockingStartEffects) bundle() reconciliation.Effects {
	return reconciliation.Effects{
		Start: func(context.Context, reconciliation.StartEffect) error {
			f.entered <- struct{}{}
			<-f.release
			return f.err
		},
		Stop: func(context.Context, reconciliation.StopEffect) error { return nil },
		Wait: convergedWait,
	}
}
