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

	_, err = service.StopSource(context.Background(), automations.StopSourceRequest{
		Identity: sourceIdentity("missing"),
	})
	assertLifecycleError(t, err, automations.ErrorCodeNotFound, automations.ErrNotFound)
	if got := effects.counts(); got != (effectCounts{}) {
		t.Fatalf("invalid operations invoked effects: %+v", got)
	}
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
	return nil
}

func (f *recordingEffects) Stop(_ context.Context, effect reconciliation.StopEffect) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stops = append(f.stops, effect)
	f.events = append(f.events, "stop")
	return nil
}

func (f *recordingEffects) Wait(
	_ context.Context,
	effect reconciliation.WaitEffect,
) (automations.SourceObservation, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.waits = append(f.waits, effect)
	f.events = append(f.events, "wait")
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
