package service_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	automations "github.com/portpowered/infinite-you/pkg/services/automations"
	reconciliation "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/reconciliation"
	reconciliationwire "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/reconciliation/wire"
)

func TestSourceLifecycleStartsOnceAndConvergesThroughWait(t *testing.T) {
	t.Parallel()

	effects := &recordingEffects{
		waitStates: []automations.ObservedLifecycleState{
			automations.ObservedLifecycleRunning,
		},
	}
	service := reconciliationwire.NewService(effects.bundle())
	request := automations.StartSourceRequest{
		Identity: sourceIdentity("start"),
		Kind:     "schedule",
	}

	started, err := service.StartSource(context.Background(), request)
	if err != nil {
		t.Fatalf("StartSource: %v", err)
	}
	assertLifecycle(
		t,
		started.Outcome,
		automations.DesiredLifecycleRunning,
		automations.ObservedLifecycleStarting,
		automations.ConvergenceStatusProgressing,
		false,
	)
	instanceID := started.Outcome.Observation.InstanceID
	if instanceID == "" {
		t.Fatal("StartSource returned an empty instance identity")
	}

	repeated, err := service.StartSource(context.Background(), request)
	if err != nil {
		t.Fatalf("repeated StartSource: %v", err)
	}
	assertLifecycle(
		t,
		repeated.Outcome,
		automations.DesiredLifecycleRunning,
		automations.ObservedLifecycleStarting,
		automations.ConvergenceStatusProgressing,
		true,
	)
	if got := effects.counts(); got != (effectCounts{starts: 1}) {
		t.Fatalf("effects after repeated start = %+v, want one start", got)
	}

	assertStatus(t, service, request.Identity, instanceID, automations.ObservedLifecycleStarting)
	if got := effects.counts(); got != (effectCounts{starts: 1}) {
		t.Fatalf("status invoked effects: %+v", got)
	}

	waited, err := service.WaitSource(context.Background(), automations.WaitSourceRequest{
		Identity: request.Identity,
		Desired:  automations.DesiredLifecycleRunning,
	})
	if err != nil {
		t.Fatalf("WaitSource: %v", err)
	}
	assertLifecycle(
		t,
		waited.Outcome,
		automations.DesiredLifecycleRunning,
		automations.ObservedLifecycleRunning,
		automations.ConvergenceStatusConverged,
		false,
	)
	assertStatus(t, service, request.Identity, instanceID, automations.ObservedLifecycleRunning)

	converged, err := service.StartSource(context.Background(), request)
	if err != nil {
		t.Fatalf("converged StartSource: %v", err)
	}
	assertLifecycle(
		t,
		converged.Outcome,
		automations.DesiredLifecycleRunning,
		automations.ObservedLifecycleRunning,
		automations.ConvergenceStatusConverged,
		true,
	)
	if got := effects.counts(); got != (effectCounts{starts: 1, waits: 1}) {
		t.Fatalf("effects after convergence = %+v, want one start and wait", got)
	}
}

func TestStartSourceOwnsExactlyOnceActivationAfterStartingIsAuthoritative(t *testing.T) {
	t.Parallel()

	identity := sourceIdentity("owned-activation")
	activationEntered := make(chan error, 1)
	releaseActivation := make(chan struct{})
	var service reconciliation.Service
	var activationMu sync.Mutex
	activationCalls := 0
	service = reconciliationwire.NewService(reconciliation.Effects{
		Start: func(ctx context.Context, _ reconciliation.StartEffect) error {
			status, err := service.SourceStatus(
				ctx,
				automations.SourceStatusRequest{Identity: identity},
			)
			if err != nil {
				activationEntered <- err
				return err
			}
			if status.Observation.State != automations.ObservedLifecycleStarting {
				err := errors.New("activation ran before starting became authoritative")
				activationEntered <- err
				return err
			}
			activationMu.Lock()
			activationCalls++
			activationMu.Unlock()
			activationEntered <- nil
			<-releaseActivation
			return nil
		},
	})
	request := automations.StartSourceRequest{Identity: identity, Kind: "schedule"}

	firstDone := make(chan error, 1)
	go func() {
		_, err := service.StartSource(context.Background(), request)
		firstDone <- err
	}()
	if err := <-activationEntered; err != nil {
		t.Fatal(err)
	}

	repeated, err := service.StartSource(context.Background(), request)
	if err != nil {
		t.Fatalf("repeated StartSource during activation: %v", err)
	}
	if !repeated.Outcome.Idempotent ||
		repeated.Outcome.Observation.State != automations.ObservedLifecycleStarting {
		t.Fatalf("repeated outcome = %+v, want idempotent starting", repeated.Outcome)
	}
	close(releaseActivation)
	if err := <-firstDone; err != nil {
		t.Fatalf("first StartSource activation: %v", err)
	}

	activationMu.Lock()
	defer activationMu.Unlock()
	if activationCalls != 1 {
		t.Fatalf("activation calls = %d, want 1", activationCalls)
	}
}

func TestSourceLifecycleStopsOnceAndConvergesThroughWait(t *testing.T) {
	t.Parallel()

	effects := &recordingEffects{
		waitStates: []automations.ObservedLifecycleState{
			automations.ObservedLifecycleRunning,
			automations.ObservedLifecycleStopped,
		},
	}
	service := reconciliationwire.NewService(effects.bundle())
	identity := sourceIdentity("stop")
	startAndWait(t, service, identity)

	stopped, err := service.StopSource(context.Background(), automations.StopSourceRequest{
		Identity: identity,
	})
	if err != nil {
		t.Fatalf("StopSource: %v", err)
	}
	assertLifecycle(
		t,
		stopped.Outcome,
		automations.DesiredLifecycleStopped,
		automations.ObservedLifecycleStopping,
		automations.ConvergenceStatusProgressing,
		false,
	)

	repeated, err := service.StopSource(context.Background(), automations.StopSourceRequest{
		Identity: identity,
	})
	if err != nil {
		t.Fatalf("repeated StopSource: %v", err)
	}
	assertLifecycle(
		t,
		repeated.Outcome,
		automations.DesiredLifecycleStopped,
		automations.ObservedLifecycleStopping,
		automations.ConvergenceStatusProgressing,
		true,
	)
	assertStatus(
		t,
		service,
		identity,
		stopped.Outcome.Observation.InstanceID,
		automations.ObservedLifecycleStopping,
	)

	waited, err := service.WaitSource(context.Background(), automations.WaitSourceRequest{
		Identity: identity,
		Desired:  automations.DesiredLifecycleStopped,
	})
	if err != nil {
		t.Fatalf("WaitSource stopped: %v", err)
	}
	assertLifecycle(
		t,
		waited.Outcome,
		automations.DesiredLifecycleStopped,
		automations.ObservedLifecycleStopped,
		automations.ConvergenceStatusConverged,
		false,
	)

	converged, err := service.StopSource(context.Background(), automations.StopSourceRequest{
		Identity: identity,
	})
	if err != nil {
		t.Fatalf("converged StopSource: %v", err)
	}
	if !converged.Outcome.Idempotent {
		t.Fatalf("converged StopSource outcome = %+v, want idempotent", converged.Outcome)
	}
	if got := effects.counts(); got != (effectCounts{starts: 1, stops: 1, waits: 2}) {
		t.Fatalf("effects = %+v, want one start, stop, and two waits", got)
	}
	if got := effects.eventNames(); !equalStrings(got, []string{"start", "wait", "stop", "wait"}) {
		t.Fatalf("effect order = %v, want start, wait, stop, wait", got)
	}
}

func TestStopSourceCommitsStoppingBeforeExactlyOnceEffect(t *testing.T) {
	t.Parallel()

	identity := sourceIdentity("owned-stop")
	effects := newBlockingStopEffects(identity)
	service := reconciliationwire.NewService(effects.bundle())
	effects.service = service
	startAndWait(t, service, identity)
	running, err := service.SourceStatus(
		context.Background(),
		automations.SourceStatusRequest{Identity: identity},
	)
	if err != nil {
		t.Fatalf("SourceStatus before stop: %v", err)
	}

	firstDone := make(chan error, 1)
	go func() {
		_, err := service.StopSource(
			context.Background(),
			automations.StopSourceRequest{Identity: identity},
		)
		firstDone <- err
	}()
	if err := <-effects.entered; err != nil {
		t.Fatal(err)
	}
	assertStatus(
		t,
		service,
		identity,
		running.Observation.InstanceID,
		automations.ObservedLifecycleStopping,
	)

	repeated, err := service.StopSource(
		context.Background(),
		automations.StopSourceRequest{Identity: identity},
	)
	if err != nil {
		t.Fatalf("repeated StopSource during effect: %v", err)
	}
	if !repeated.Outcome.Idempotent ||
		repeated.Outcome.Observation.State != automations.ObservedLifecycleStopping {
		t.Fatalf("repeated outcome = %+v, want idempotent stopping", repeated.Outcome)
	}

	close(effects.release)
	if err := <-firstDone; err != nil {
		t.Fatalf("first StopSource effect: %v", err)
	}
	waited, err := service.WaitSource(
		context.Background(),
		automations.WaitSourceRequest{
			Identity: identity,
			Desired:  automations.DesiredLifecycleStopped,
		},
	)
	if err != nil {
		t.Fatalf("WaitSource stopped: %v", err)
	}
	assertLifecycle(
		t,
		waited.Outcome,
		automations.DesiredLifecycleStopped,
		automations.ObservedLifecycleStopped,
		automations.ConvergenceStatusConverged,
		false,
	)
	if calls := effects.stopCount(); calls != 1 {
		t.Fatalf("stop effect calls = %d, want 1", calls)
	}
}

func TestStopSourceSuccessDoesNotReportStoppingAfterSupersedingStart(t *testing.T) {
	t.Parallel()

	identity := sourceIdentity("stale-stop-success")
	effects := newBlockingStopEffects(identity)
	service := reconciliationwire.NewService(effects.bundle())
	effects.service = service
	startAndWait(t, service, identity)

	results := make(chan automations.StopSourceResult, 1)
	errs := make(chan error, 1)
	go func() {
		result, err := service.StopSource(
			context.Background(),
			automations.StopSourceRequest{Identity: identity},
		)
		results <- result
		errs <- err
	}()
	if err := <-effects.entered; err != nil {
		t.Fatal(err)
	}

	restarted, err := service.StartSource(
		context.Background(),
		automations.StartSourceRequest{Identity: identity, Kind: "schedule"},
	)
	if err != nil {
		t.Fatalf("StartSource superseding stop: %v", err)
	}
	assertLifecycle(
		t,
		restarted.Outcome,
		automations.DesiredLifecycleRunning,
		automations.ObservedLifecycleStopping,
		automations.ConvergenceStatusProgressing,
		true,
	)

	close(effects.release)
	if err := <-errs; err != nil {
		t.Fatalf("stale StopSource success: %v", err)
	}
	stopped := <-results
	assertLifecycle(
		t,
		stopped.Outcome,
		automations.DesiredLifecycleStopped,
		automations.ObservedLifecycleStopping,
		automations.ConvergenceStatusProgressing,
		true,
	)
}

func TestStopSourceFailureDoesNotOverwriteNewerStoppedObservation(t *testing.T) {
	t.Parallel()

	identity := sourceIdentity("stale-stop-failure")
	effects := newBlockingStopEffects(identity)
	effects.stopErr = errors.New("late stop failure")
	service := reconciliationwire.NewService(effects.bundle())
	effects.service = service
	startAndWait(t, service, identity)

	results := make(chan automations.StopSourceResult, 1)
	errs := make(chan error, 1)
	go func() {
		result, err := service.StopSource(
			context.Background(),
			automations.StopSourceRequest{Identity: identity},
		)
		results <- result
		errs <- err
	}()
	if err := <-effects.entered; err != nil {
		t.Fatal(err)
	}

	waited, err := service.WaitSource(
		context.Background(),
		automations.WaitSourceRequest{
			Identity: identity,
			Desired:  automations.DesiredLifecycleStopped,
		},
	)
	if err != nil {
		t.Fatalf("WaitSource stopped during stop effect: %v", err)
	}
	assertLifecycle(
		t,
		waited.Outcome,
		automations.DesiredLifecycleStopped,
		automations.ObservedLifecycleStopped,
		automations.ConvergenceStatusConverged,
		false,
	)

	close(effects.release)
	if err := <-errs; err != nil {
		t.Fatalf("stale StopSource failure: %v", err)
	}
	stopped := <-results
	assertLifecycle(
		t,
		stopped.Outcome,
		automations.DesiredLifecycleStopped,
		automations.ObservedLifecycleStopped,
		automations.ConvergenceStatusConverged,
		true,
	)
}

func TestStopSourceFailureDoesNotOverwriteSupersedingStart(t *testing.T) {
	t.Parallel()

	identity := sourceIdentity("superseded-stop-failure")
	effects := newBlockingStopEffects(identity)
	effects.stopErr = errors.New("late stop failure")
	service := reconciliationwire.NewService(effects.bundle())
	effects.service = service
	startAndWait(t, service, identity)

	results := make(chan automations.StopSourceResult, 1)
	errs := make(chan error, 1)
	go func() {
		result, err := service.StopSource(
			context.Background(),
			automations.StopSourceRequest{Identity: identity},
		)
		results <- result
		errs <- err
	}()
	if err := <-effects.entered; err != nil {
		t.Fatal(err)
	}

	restarted, err := service.StartSource(
		context.Background(),
		automations.StartSourceRequest{Identity: identity, Kind: "schedule"},
	)
	if err != nil {
		t.Fatalf("StartSource superseding stop: %v", err)
	}
	assertLifecycle(
		t,
		restarted.Outcome,
		automations.DesiredLifecycleRunning,
		automations.ObservedLifecycleStopping,
		automations.ConvergenceStatusProgressing,
		true,
	)

	close(effects.release)
	if err := <-errs; err != nil {
		t.Fatalf("superseded StopSource failure: %v", err)
	}
	stopped := <-results
	assertLifecycle(
		t,
		stopped.Outcome,
		automations.DesiredLifecycleStopped,
		automations.ObservedLifecycleStopping,
		automations.ConvergenceStatusProgressing,
		true,
	)
}

type blockingStopEffects struct {
	identity automations.SourceIdentity
	service  reconciliation.Service
	entered  chan error
	release  chan struct{}
	stopErr  error

	mu    sync.Mutex
	calls int
}

func newBlockingStopEffects(identity automations.SourceIdentity) *blockingStopEffects {
	return &blockingStopEffects{
		identity: identity,
		entered:  make(chan error, 1),
		release:  make(chan struct{}),
	}
}

func (f *blockingStopEffects) bundle() reconciliation.Effects {
	return reconciliation.Effects{
		Start: func(context.Context, reconciliation.StartEffect) error { return nil },
		Stop:  f.stop,
		Wait:  convergedWait,
	}
}

func (f *blockingStopEffects) stop(ctx context.Context, _ reconciliation.StopEffect) error {
	status, err := f.service.SourceStatus(
		ctx,
		automations.SourceStatusRequest{Identity: f.identity},
	)
	if err == nil && status.Observation.State != automations.ObservedLifecycleStopping {
		err = errors.New("stop effect ran before stopping became authoritative")
	}
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	f.entered <- err
	<-f.release
	return f.stopErr
}

func (f *blockingStopEffects) stopCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func convergedWait(
	_ context.Context,
	effect reconciliation.WaitEffect,
) (automations.SourceObservation, error) {
	observation := effect.Observation
	if effect.Desired == automations.DesiredLifecycleRunning {
		observation.State = automations.ObservedLifecycleRunning
	} else {
		observation.State = automations.ObservedLifecycleStopped
	}
	return observation, nil
}

func TestSourceLifecycleConcurrentStartInvokesOneActivation(t *testing.T) {
	t.Parallel()

	effects := &recordingEffects{}
	service := reconciliationwire.NewService(effects.bundle())
	request := automations.StartSourceRequest{
		Identity: sourceIdentity("concurrent"),
		Kind:     "hosted",
	}
	const callers = 12
	results := make(chan automations.StartSourceResult, callers)
	errs := make(chan error, callers)
	var ready sync.WaitGroup
	ready.Add(callers)
	release := make(chan struct{})

	for range callers {
		go func() {
			ready.Done()
			<-release
			result, err := service.StartSource(context.Background(), request)
			results <- result
			errs <- err
		}()
	}
	ready.Wait()
	close(release)

	nonIdempotent := 0
	for range callers {
		if err := <-errs; err != nil {
			t.Fatalf("concurrent StartSource: %v", err)
		}
		if !(<-results).Outcome.Idempotent {
			nonIdempotent++
		}
	}
	if nonIdempotent != 1 {
		t.Fatalf("non-idempotent outcomes = %d, want 1", nonIdempotent)
	}
	if got := effects.counts().starts; got != 1 {
		t.Fatalf("start effects = %d, want 1", got)
	}
}

func TestSourceLifecycleRejectsInvalidOperationsWithoutEffects(t *testing.T) {
	t.Parallel()

	effects := &recordingEffects{}
	service := reconciliationwire.NewService(effects.bundle())
	_, err := service.StartSource(context.Background(), automations.StartSourceRequest{
		Identity: automations.SourceIdentity{AutomationID: "automation", SourceID: " "},
		Kind:     "schedule",
	})
	assertLifecycleError(t, err, automations.ErrorCodeInvalid, automations.ErrInvalidRequest)

	_, err = service.StartSource(context.Background(), automations.StartSourceRequest{
		Identity: sourceIdentity("whitespace-kind"),
		Kind:     " schedule ",
	})
	assertLifecycleError(t, err, automations.ErrorCodeInvalid, automations.ErrInvalidRequest)

	identity := sourceIdentity("malformed-resume")
	resume := automations.SourceObservation{
		Identity: identity,
		State:    automations.ObservedLifecycleRunning,
	}
	_, err = service.StartSource(context.Background(), automations.StartSourceRequest{
		Identity: identity,
		Kind:     "schedule",
		Resume:   &resume,
	})
	assertLifecycleError(t, err, automations.ErrorCodeInvalid, automations.ErrInvalidRequest)

	_, err = service.StopSource(context.Background(), automations.StopSourceRequest{})
	assertLifecycleError(t, err, automations.ErrorCodeInvalid, automations.ErrInvalidRequest)

	_, err = service.StopSource(context.Background(), automations.StopSourceRequest{
		Identity: sourceIdentity("missing"),
	})
	assertLifecycleError(t, err, automations.ErrorCodeNotFound, automations.ErrNotFound)

	_, err = service.WaitSource(context.Background(), automations.WaitSourceRequest{})
	assertLifecycleError(t, err, automations.ErrorCodeInvalid, automations.ErrInvalidRequest)

	_, err = service.WaitSource(context.Background(), automations.WaitSourceRequest{
		Identity: sourceIdentity("missing-wait"),
		Desired:  automations.DesiredLifecycleRunning,
	})
	assertLifecycleError(t, err, automations.ErrorCodeNotFound, automations.ErrNotFound)

	_, err = service.SourceStatus(context.Background(), automations.SourceStatusRequest{})
	assertLifecycleError(t, err, automations.ErrorCodeInvalid, automations.ErrInvalidRequest)

	if got := effects.counts(); got != (effectCounts{}) {
		t.Fatalf("invalid operations invoked effects: %+v", got)
	}
}

func TestSourceLifecycleReportsUnavailableEffects(t *testing.T) {
	t.Parallel()

	service := reconciliationwire.NewService()
	identity := sourceIdentity("unavailable-effects")
	_, err := service.StartSource(context.Background(), automations.StartSourceRequest{
		Identity: identity,
		Kind:     "schedule",
	})
	assertLifecycleError(t, err, automations.ErrorCodeNotReady, automations.ErrNotReady)

	_, err = service.StopSource(context.Background(), automations.StopSourceRequest{
		Identity: identity,
	})
	assertLifecycleError(t, err, automations.ErrorCodeNotReady, automations.ErrNotReady)

	_, err = service.WaitSource(context.Background(), automations.WaitSourceRequest{
		Identity: identity,
		Desired:  automations.DesiredLifecycleRunning,
	})
	assertLifecycleError(t, err, automations.ErrorCodeNotReady, automations.ErrNotReady)
}

func TestSourceLifecycleRejectsKindDriftAndForeignWaitObservation(t *testing.T) {
	t.Parallel()

	identity := sourceIdentity("effect-validation")
	resume := automations.SourceObservation{
		Identity:   identity,
		InstanceID: "persisted-effect-validation",
		State:      automations.ObservedLifecycleRunning,
	}
	service := reconciliationwire.NewService()
	if _, err := service.StartSource(context.Background(), automations.StartSourceRequest{
		Identity: identity,
		Kind:     "hosted",
		Resume:   &resume,
	}); err != nil {
		t.Fatalf("restore running source: %v", err)
	}
	_, err := service.StartSource(context.Background(), automations.StartSourceRequest{
		Identity: identity,
		Kind:     "schedule",
	})
	assertLifecycleError(t, err, automations.ErrorCodeConflict, automations.ErrConflict)

	waited, err := service.WaitSource(context.Background(), automations.WaitSourceRequest{
		Identity: identity,
		Desired:  automations.DesiredLifecycleRunning,
	})
	if err != nil {
		t.Fatalf("WaitSource for converged source: %v", err)
	}
	if !waited.Outcome.Idempotent ||
		waited.Outcome.Convergence != automations.ConvergenceStatusConverged {
		t.Fatalf("WaitSource outcome = %+v, want idempotent convergence", waited.Outcome)
	}

	foreignEffects := reconciliation.Effects{
		Start: func(context.Context, reconciliation.StartEffect) error {
			return nil
		},
		Wait: func(
			_ context.Context,
			effect reconciliation.WaitEffect,
		) (automations.SourceObservation, error) {
			observation := effect.Observation
			observation.InstanceID = "foreign-instance"
			return observation, nil
		},
	}
	foreignService := reconciliationwire.NewService(foreignEffects)
	foreignIdentity := sourceIdentity("foreign-wait")
	if _, err := foreignService.StartSource(
		context.Background(),
		automations.StartSourceRequest{Identity: foreignIdentity, Kind: "watcher"},
	); err != nil {
		t.Fatalf("StartSource before foreign wait: %v", err)
	}
	_, err = foreignService.WaitSource(context.Background(), automations.WaitSourceRequest{
		Identity: foreignIdentity,
		Desired:  automations.DesiredLifecycleRunning,
	})
	assertLifecycleError(t, err, automations.ErrorCodeInvalid, automations.ErrInvalidRequest)
}

type effectCounts struct {
	starts int
	stops  int
	waits  int
}

type recordingEffects struct {
	mu         sync.Mutex
	starts     []reconciliation.StartEffect
	stops      []reconciliation.StopEffect
	waits      []reconciliation.WaitEffect
	waitStates []automations.ObservedLifecycleState
	events     []string
	startErr   error
	stopErr    error
	waitErr    error
}

func (f *recordingEffects) bundle() reconciliation.Effects {
	return reconciliation.Effects{
		Start: f.Start,
		Stop:  f.Stop,
		Wait:  f.Wait,
	}
}

func (f *recordingEffects) Start(_ context.Context, effect reconciliation.StartEffect) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.starts = append(f.starts, effect)
	f.events = append(f.events, "start")
	return f.startErr
}

func (f *recordingEffects) Stop(_ context.Context, effect reconciliation.StopEffect) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stops = append(f.stops, effect)
	f.events = append(f.events, "stop")
	return f.stopErr
}

func (f *recordingEffects) Wait(
	_ context.Context,
	effect reconciliation.WaitEffect,
) (automations.SourceObservation, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.waits = append(f.waits, effect)
	f.events = append(f.events, "wait")
	if f.waitErr != nil {
		return automations.SourceObservation{}, f.waitErr
	}
	observation := effect.Observation
	if len(f.waitStates) > 0 {
		observation.State = f.waitStates[0]
		f.waitStates = f.waitStates[1:]
	}
	return observation, nil
}

func (f *recordingEffects) counts() effectCounts {
	f.mu.Lock()
	defer f.mu.Unlock()
	return effectCounts{
		starts: len(f.starts),
		stops:  len(f.stops),
		waits:  len(f.waits),
	}
}

func (f *recordingEffects) eventNames() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.events...)
}

func startAndWait(
	t *testing.T,
	service reconciliation.Service,
	identity automations.SourceIdentity,
) {
	t.Helper()
	if _, err := service.StartSource(context.Background(), automations.StartSourceRequest{
		Identity: identity,
		Kind:     "schedule",
	}); err != nil {
		t.Fatalf("StartSource: %v", err)
	}
	if _, err := service.WaitSource(context.Background(), automations.WaitSourceRequest{
		Identity: identity,
		Desired:  automations.DesiredLifecycleRunning,
	}); err != nil {
		t.Fatalf("WaitSource running: %v", err)
	}
}

func assertStatus(
	t *testing.T,
	service reconciliation.Service,
	identity automations.SourceIdentity,
	instanceID string,
	state automations.ObservedLifecycleState,
) {
	t.Helper()
	status, err := service.SourceStatus(context.Background(), automations.SourceStatusRequest{
		Identity: identity,
	})
	if err != nil {
		t.Fatalf("SourceStatus: %v", err)
	}
	if status.Observation.Identity != identity ||
		status.Observation.InstanceID != instanceID ||
		status.Observation.State != state {
		t.Fatalf("SourceStatus = %+v, want identity %v, instance %q, state %q",
			status.Observation, identity, instanceID, state)
	}
	status.Observation.State = automations.ObservedLifecycleFailed
	again, err := service.SourceStatus(context.Background(), automations.SourceStatusRequest{
		Identity: identity,
	})
	if err != nil {
		t.Fatalf("SourceStatus after caller mutation: %v", err)
	}
	if again.Observation.State != state {
		t.Fatalf("caller mutated authoritative status to %q, want %q",
			again.Observation.State, state)
	}
}

func assertLifecycle(
	t *testing.T,
	outcome automations.LifecycleOutcome,
	desired automations.DesiredLifecycleState,
	observed automations.ObservedLifecycleState,
	convergence automations.ConvergenceStatus,
	idempotent bool,
) {
	t.Helper()
	if outcome.Desired != desired ||
		outcome.Observation.State != observed ||
		outcome.Convergence != convergence ||
		outcome.Idempotent != idempotent {
		t.Fatalf("outcome = %+v, want desired/state/convergence/idempotent %q/%q/%q/%t",
			outcome, desired, observed, convergence, idempotent)
	}
}

func assertLifecycleError(
	t *testing.T,
	err error,
	code automations.ErrorCode,
	sentinel error,
) {
	t.Helper()
	var typed *automations.Error
	if !errors.As(err, &typed) || typed.Code != code || !errors.Is(err, sentinel) {
		t.Fatalf("error = %T %v, want code %q wrapping %v", err, err, code, sentinel)
	}
}

func sourceIdentity(suffix string) automations.SourceIdentity {
	return automations.SourceIdentity{
		AutomationID: "automation-" + suffix,
		SourceID:     "source-" + suffix,
	}
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
