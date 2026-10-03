package service_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	reconciliation "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/reconciliation"

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

func TestRuntimeSourceControlIsolatesSharedIdentityAndRetainsResume(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	identity := sourceIdentity("shared-runtime-source")
	var starts, stops, waits []string
	service := reconciliationwire.NewService(reconciliation.Effects{
		Start: func(_ context.Context, e reconciliation.StartEffect) error {
			starts = append(starts, e.RuntimeID)
			return nil
		},
		Stop: func(_ context.Context, e reconciliation.StopEffect) error {
			stops = append(stops, e.RuntimeID)
			return nil
		},
		Wait: func(_ context.Context, e reconciliation.WaitEffect) (automations.SourceObservation, error) {
			waits = append(waits, e.RuntimeID)
			observation, err := convergedWait(ctx, e)
			observation.Cursor = automations.Cursor("cursor-" + e.RuntimeID)
			return observation, err
		},
	})
	request := automations.StartSourceRequest{Identity: identity, Kind: "schedule"}
	var instanceID string
	for _, runtimeID := range []string{"A", "B", ""} {
		started, err := service.StartSourceForRuntime(ctx, runtimeID, request)
		if err != nil {
			t.Fatal(err)
		}
		if instanceID != "" && started.Outcome.Observation.InstanceID != instanceID {
			t.Fatal("runtime scope changed public instance identity")
		}
		instanceID = started.Outcome.Observation.InstanceID
		waitScopedSource(t, service, runtimeID, identity, automations.DesiredLifecycleRunning, "cursor-"+automations.Cursor(runtimeID))
	}
	if _, err := service.StopSourceForRuntime(ctx, "A", automations.StopSourceRequest{Identity: identity}); err != nil {
		t.Fatal(err)
	}
	stopped := waitScopedSource(t, service, "A", identity, automations.DesiredLifecycleStopped, "cursor-A")
	for _, runtimeID := range []string{"B", ""} {
		result := waitScopedSource(t, service, runtimeID, identity, automations.DesiredLifecycleRunning, "cursor-"+automations.Cursor(runtimeID))
		if !result.Outcome.Idempotent {
			t.Fatal("stopping A disturbed peer convergence")
		}
	}
	assertDetachedScopeCursor(t, service, identity, instanceID)
	request.Resume = &stopped.Outcome.Observation
	restarted, err := service.StartSourceForRuntime(ctx, "A", request)
	if err != nil {
		t.Fatal(err)
	}
	assertLifecycle(t, restarted.Outcome, automations.DesiredLifecycleRunning, automations.ObservedLifecycleStarting, automations.ConvergenceStatusProgressing, false)
	if restarted.Outcome.Observation.Cursor != "cursor-A" {
		t.Fatal("restart lost A resume facts")
	}
	if _, err := service.StartSourceForRuntime(ctx, "B", request); !errors.Is(err, automations.ErrConflict) {
		t.Fatalf("foreign resume error = %v, want conflict", err)
	}
	if !reflect.DeepEqual(starts, []string{"A", "B", "", "A"}) || !reflect.DeepEqual(stops, []string{"A"}) || !reflect.DeepEqual(waits, []string{"A", "B", "", "A"}) {
		t.Fatalf("effect scopes start/stop/wait = %v/%v/%v", starts, stops, waits)
	}
}

func waitScopedSource(t *testing.T, service reconciliation.Service, runtimeID string, identity automations.SourceIdentity, desired automations.DesiredLifecycleState, cursor automations.Cursor) automations.WaitSourceResult {
	t.Helper()
	result, err := service.WaitSourceForRuntime(context.Background(), runtimeID, automations.WaitSourceRequest{Identity: identity, Desired: desired})
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome.Convergence != automations.ConvergenceStatusConverged || result.Outcome.Observation.Cursor != cursor {
		t.Fatalf("runtime %q wait = %+v, want converged cursor %q", runtimeID, result, cursor)
	}
	return result
}

func assertDetachedScopeCursor(t *testing.T, service reconciliation.Service, identity automations.SourceIdentity, instanceID string) {
	t.Helper()
	status, err := service.SourceStatus(context.Background(), automations.SourceStatusRequest{Identity: identity})
	if err != nil || status.Observation.State != automations.ObservedLifecycleRunning || status.Observation.Cursor != "cursor-" {
		t.Fatalf("detached source = %+v, %v", status, err)
	}
	cursor, err := service.GetCursor(context.Background(), automations.GetCursorRequest{InstanceID: instanceID, ExpectedCursor: "cursor-"})
	if err != nil || cursor.Cursor != "cursor-" {
		t.Fatalf("detached cursor = %+v, %v", cursor, err)
	}
	instance, err := service.GetStatus(context.Background(), automations.GetStatusRequest{InstanceID: instanceID})
	if err != nil || instance.Status != automations.ObservedLifecycleRunning {
		t.Fatalf("detached instance = %+v, %v", instance, err)
	}
}

func TestRuntimeSourceControlInstanceOwnershipIsRuntimeLocal(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	service := reconciliationwire.NewService((&recordingEffects{}).bundle())
	identity := sourceIdentity("first")
	resume := automations.SourceObservation{Identity: identity, InstanceID: "same-persisted-instance", State: automations.ObservedLifecycleRunning, Cursor: "first-cursor"}
	request := automations.StartSourceRequest{Identity: identity, Kind: "hosted", Resume: &resume}
	if _, err := service.StartSourceForRuntime(ctx, "A", request); err != nil {
		t.Fatal(err)
	}
	resume.Identity = sourceIdentity("second")
	resume.Cursor = "second-cursor"
	request.Identity = resume.Identity
	if _, err := service.StartSourceForRuntime(ctx, "A", request); !errors.Is(err, automations.ErrConflict) {
		t.Fatalf("same-runtime alias error = %v, want conflict", err)
	}
	if _, err := service.StartSourceForRuntime(ctx, "B", request); err != nil {
		t.Fatalf("peer resume: %v", err)
	}
	waitScopedSource(t, service, "A", identity, automations.DesiredLifecycleRunning, "first-cursor")
	waitScopedSource(t, service, "B", request.Identity, automations.DesiredLifecycleRunning, "second-cursor")
	if _, err := service.GetCursor(ctx, automations.GetCursorRequest{InstanceID: resume.InstanceID}); !errors.Is(err, automations.ErrNotFound) {
		t.Fatalf("scoped instance leaked into detached reads: %v", err)
	}
	if _, err := service.GetStatus(ctx, automations.GetStatusRequest{InstanceID: resume.InstanceID}); !errors.Is(err, automations.ErrNotFound) {
		t.Fatalf("scoped status leaked into detached reads: %v", err)
	}
	if _, err := service.StopSource(ctx, automations.StopSourceRequest{Identity: identity}); !errors.Is(err, automations.ErrNotFound) {
		t.Fatalf("detached stop reached scoped instance: %v", err)
	}
}
