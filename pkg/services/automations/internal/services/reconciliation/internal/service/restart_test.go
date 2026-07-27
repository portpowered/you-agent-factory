package service_test

import (
	"context"
	"errors"
	"testing"

	automations "github.com/portpowered/infinite-you/pkg/services/automations"
	reconciliationwire "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/reconciliation/wire"
)

func TestSourceLifecycleRestartRestoresDetachedObservations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		state       automations.ObservedLifecycleState
		convergence automations.ConvergenceStatus
		wantErr     error
		starts      int
	}{
		{"running", automations.ObservedLifecycleRunning, automations.ConvergenceStatusConverged, nil, 0},
		{"stopped", automations.ObservedLifecycleStopped, automations.ConvergenceStatusProgressing, nil, 1},
		{"starting", automations.ObservedLifecycleStarting, automations.ConvergenceStatusProgressing, nil, 0},
		{"stopping", automations.ObservedLifecycleStopping, automations.ConvergenceStatusProgressing, nil, 0},
		{"failed", automations.ObservedLifecycleFailed, automations.ConvergenceStatusFailed, automations.ErrSupervisionFailed, 0},
		{"cancelled", automations.ObservedLifecycleCancelled, automations.ConvergenceStatusCancelled, context.Canceled, 0},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			effects := &recordingEffects{}
			service := reconciliationwire.NewService(effects.bundle())
			identity := sourceIdentity("restart-" + test.name)
			resume := automations.SourceObservation{
				Identity:   identity,
				InstanceID: "persisted-instance-" + test.name,
				State:      test.state,
				Cursor:     "opaque-cursor-" + automations.Cursor(test.name),
			}

			result, err := service.StartSource(context.Background(), automations.StartSourceRequest{
				Identity: identity,
				Kind:     "hosted",
				Resume:   &resume,
			})
			if test.wantErr == nil && err != nil {
				t.Fatalf("StartSource() unexpected error: %v", err)
			}
			if test.wantErr != nil && !errors.Is(err, test.wantErr) {
				t.Fatalf("StartSource() error = %v, want errors.Is %v", err, test.wantErr)
			}
			if result.Outcome.Observation.InstanceID != resume.InstanceID ||
				result.Outcome.Observation.Cursor != resume.Cursor ||
				result.Outcome.Convergence != test.convergence {
				t.Fatalf("StartSource() outcome = %+v, want identity/cursor/convergence %q/%q/%q",
					result.Outcome, resume.InstanceID, resume.Cursor, test.convergence)
			}
			if got := effects.counts().starts; got != test.starts {
				t.Fatalf("StartSource() effects = %d, want %d", got, test.starts)
			}

			status, statusErr := service.SourceStatus(context.Background(), automations.SourceStatusRequest{
				Identity: identity,
			})
			if statusErr != nil {
				t.Fatalf("SourceStatus() unexpected error: %v", statusErr)
			}
			wantState := test.state
			if test.state == automations.ObservedLifecycleStopped {
				wantState = automations.ObservedLifecycleStarting
			}
			if status.Observation.InstanceID != resume.InstanceID ||
				status.Observation.Cursor != resume.Cursor ||
				status.Observation.State != wantState {
				t.Fatalf("SourceStatus() = %+v, want instance/cursor/state %q/%q/%q",
					status.Observation, resume.InstanceID, resume.Cursor, wantState)
			}
		})
	}
}

func TestSourceLifecycleRestartContinuesTransitionalObservation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		suffix    string
		resumed   automations.ObservedLifecycleState
		observed  automations.ObservedLifecycleState
		converged automations.ConvergenceStatus
		restarts  bool
	}{
		{"starting reaches running", "starting", automations.ObservedLifecycleStarting, automations.ObservedLifecycleRunning, automations.ConvergenceStatusConverged, false},
		{"stopping reaches stopped", "stopping", automations.ObservedLifecycleStopping, automations.ObservedLifecycleStopped, automations.ConvergenceStatusProgressing, true},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			effects := &recordingEffects{waitStates: []automations.ObservedLifecycleState{test.observed}}
			service := reconciliationwire.NewService(effects.bundle())
			identity := sourceIdentity("transition-" + test.suffix)
			resume := automations.SourceObservation{
				Identity: identity, InstanceID: "persisted-" + test.suffix,
				State: test.resumed, Cursor: "cursor-" + automations.Cursor(test.suffix),
			}
			if _, err := service.StartSource(context.Background(), automations.StartSourceRequest{
				Identity: identity, Kind: "watcher", Resume: &resume,
			}); err != nil {
				t.Fatalf("StartSource() unexpected error: %v", err)
			}

			waited, err := service.WaitSource(context.Background(), automations.WaitSourceRequest{
				Identity: identity, Desired: automations.DesiredLifecycleRunning,
			})
			if err != nil {
				t.Fatalf("WaitSource() unexpected error: %v", err)
			}
			if waited.Outcome.Observation.InstanceID != resume.InstanceID ||
				waited.Outcome.Observation.Cursor != resume.Cursor ||
				waited.Outcome.Observation.State != test.observed ||
				waited.Outcome.Convergence != test.converged {
				t.Fatalf("WaitSource() outcome = %+v", waited.Outcome)
			}
			if got := effects.counts(); got != (effectCounts{waits: 1}) {
				t.Fatalf("restart effects = %+v, want observation only", got)
			}
			if test.restarts {
				restarted, restartErr := service.StartSource(
					context.Background(),
					automations.StartSourceRequest{Identity: identity, Kind: "watcher"},
				)
				if restartErr != nil {
					t.Fatalf("StartSource() after stopped observation: %v", restartErr)
				}
				if restarted.Outcome.Observation.State != automations.ObservedLifecycleStarting ||
					restarted.Outcome.Observation.InstanceID != resume.InstanceID {
					t.Fatalf("restarted outcome = %+v, want preserved starting instance", restarted.Outcome)
				}
				if got := effects.counts(); got != (effectCounts{starts: 1, waits: 1}) {
					t.Fatalf("restart effects = %+v, want one observation then one start", got)
				}
			}
		})
	}
}

func TestSourceLifecycleRejectsStaleAndForeignResumeWithoutMutation(t *testing.T) {
	t.Parallel()

	effects := &recordingEffects{}
	service := reconciliationwire.NewService(effects.bundle())
	identity := sourceIdentity("authoritative")
	original := automations.SourceObservation{
		Identity: identity, InstanceID: "persisted-authoritative",
		State: automations.ObservedLifecycleRunning, Cursor: "cursor-current",
	}
	if _, err := service.StartSource(context.Background(), automations.StartSourceRequest{
		Identity: identity, Kind: "hosted", Resume: &original,
	}); err != nil {
		t.Fatalf("initial StartSource() unexpected error: %v", err)
	}

	stale := original
	stale.Cursor = "cursor-stale"
	_, err := service.StartSource(context.Background(), automations.StartSourceRequest{
		Identity: identity, Kind: "hosted", Resume: &stale,
	})
	assertLifecycleError(t, err, automations.ErrorCodeConflict, automations.ErrConflict)

	contradictory := original
	contradictory.State = automations.ObservedLifecycleStopped
	_, err = service.StartSource(context.Background(), automations.StartSourceRequest{
		Identity: identity, Kind: "hosted", Resume: &contradictory,
	})
	assertLifecycleError(t, err, automations.ErrorCodeConflict, automations.ErrConflict)

	foreignIdentity := sourceIdentity("foreign")
	foreign := original
	foreign.Identity = foreignIdentity
	_, err = service.StartSource(context.Background(), automations.StartSourceRequest{
		Identity: foreignIdentity, Kind: "hosted", Resume: &foreign,
	})
	assertLifecycleError(t, err, automations.ErrorCodeConflict, automations.ErrConflict)
	_, err = service.SourceStatus(context.Background(), automations.SourceStatusRequest{
		Identity: foreignIdentity,
	})
	assertLifecycleError(t, err, automations.ErrorCodeNotFound, automations.ErrNotFound)

	status, err := service.SourceStatus(context.Background(), automations.SourceStatusRequest{
		Identity: identity,
	})
	if err != nil {
		t.Fatalf("SourceStatus() unexpected error: %v", err)
	}
	if status.Observation != original {
		t.Fatalf("SourceStatus() = %+v, want preserved %+v", status.Observation, original)
	}
	if got := effects.counts(); got != (effectCounts{}) {
		t.Fatalf("invalid resume effects = %+v, want none", got)
	}
}
