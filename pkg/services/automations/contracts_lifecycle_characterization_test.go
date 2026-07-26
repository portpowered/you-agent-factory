package automations_test

import (
	"context"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/automations"
)

func TestServiceSourceLifecycle_FakeConvergesRunningIdempotently(t *testing.T) {
	t.Parallel()

	identity := automations.SourceIdentity{AutomationID: "auto-a", SourceID: "source-a"}
	svc := startFakeSource(t, identity, "schedule", nil)

	status, err := svc.SourceStatus(context.Background(), automations.SourceStatusRequest{
		Identity: identity,
	})
	if err != nil {
		t.Fatalf("SourceStatus() unexpected error: %v", err)
	}
	if status.Observation.State != automations.ObservedLifecycleRunning {
		t.Fatalf("SourceStatus() state = %q, want running", status.Observation.State)
	}

	repeated, err := svc.StartSource(context.Background(), automations.StartSourceRequest{
		Identity: identity,
		Kind:     "schedule",
	})
	if err != nil {
		t.Fatalf("repeated StartSource() unexpected error: %v", err)
	}
	assertIdempotentConverged(t, "repeated StartSource", repeated.Outcome)
}

func TestServiceSourceLifecycle_FakeConvergesStoppedIdempotently(t *testing.T) {
	t.Parallel()

	identity := automations.SourceIdentity{AutomationID: "auto-stop", SourceID: "source-stop"}
	svc := startFakeSource(t, identity, "schedule", nil)
	if _, err := svc.SourceStatus(context.Background(), automations.SourceStatusRequest{
		Identity: identity,
	}); err != nil {
		t.Fatalf("SourceStatus() unexpected error: %v", err)
	}

	stopped, err := svc.StopSource(context.Background(), automations.StopSourceRequest{
		Identity: identity,
	})
	if err != nil {
		t.Fatalf("StopSource() unexpected error: %v", err)
	}
	if stopped.Outcome.Observation.State != automations.ObservedLifecycleStopping {
		t.Fatalf("StopSource() state = %q, want stopping", stopped.Outcome.Observation.State)
	}

	waited, err := svc.WaitSource(context.Background(), automations.WaitSourceRequest{
		Identity: identity,
		Desired:  automations.DesiredLifecycleStopped,
	})
	if err != nil {
		t.Fatalf("WaitSource() unexpected error: %v", err)
	}
	if waited.Outcome.Observation.State != automations.ObservedLifecycleStopped {
		t.Fatalf("WaitSource() state = %q, want stopped", waited.Outcome.Observation.State)
	}

	repeated, err := svc.StopSource(context.Background(), automations.StopSourceRequest{
		Identity: identity,
	})
	if err != nil {
		t.Fatalf("repeated StopSource() unexpected error: %v", err)
	}
	assertIdempotentConverged(t, "repeated StopSource", repeated.Outcome)
}

func TestServiceSourceLifecycle_FakeRestartResumesCommittedCursor(t *testing.T) {
	t.Parallel()

	identity := automations.SourceIdentity{AutomationID: "auto-restart", SourceID: "source-restart"}
	lastCommitted := automations.SourceObservation{
		Identity:   identity,
		InstanceID: "instance:auto-restart:source-restart",
		State:      automations.ObservedLifecycleStopped,
		Cursor:     automations.Cursor("opaque-cursor-42"),
	}
	svc := startFakeSource(t, identity, "hosted", &lastCommitted)

	running, err := svc.SourceStatus(context.Background(), automations.SourceStatusRequest{
		Identity: identity,
	})
	if err != nil {
		t.Fatalf("SourceStatus() after restart unexpected error: %v", err)
	}
	if running.Observation.InstanceID != lastCommitted.InstanceID ||
		running.Observation.Cursor != lastCommitted.Cursor ||
		running.Observation.State != automations.ObservedLifecycleRunning {
		t.Fatalf("SourceStatus() observation = %+v, want running from committed facts", running.Observation)
	}
}

func TestServiceSourceLifecycle_FakeTypedInvalidRecoveryAndMissingSource(t *testing.T) {
	t.Parallel()

	svc := rootFor(&fakeRootService{ready: true})
	identity := automations.SourceIdentity{AutomationID: "auto-b", SourceID: "source-b"}
	_, err := svc.StartSource(context.Background(), automations.StartSourceRequest{
		Identity: identity,
		Kind:     "schedule",
		Resume: &automations.SourceObservation{
			Identity: automations.SourceIdentity{AutomationID: "other", SourceID: "source-b"},
		},
	})
	assertTypedAutomationsError(
		t, "StartSource", err, automations.ErrorCodeInvalid, automations.ErrInvalidRequest,
	)

	_, err = svc.SourceStatus(context.Background(), automations.SourceStatusRequest{
		Identity: identity,
	})
	assertTypedAutomationsError(
		t, "SourceStatus", err, automations.ErrorCodeNotFound, automations.ErrNotFound,
	)
}

func TestServiceSourceLifecycle_FakeSupportsEveryOpaqueKind(t *testing.T) {
	t.Parallel()

	for _, kind := range []string{"schedule", "hosted", "event-stream"} {
		kind := kind
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			identity := automations.SourceIdentity{AutomationID: "auto-" + kind, SourceID: "source"}
			svc := startFakeSource(t, identity, kind, nil)
			running, err := svc.WaitSource(context.Background(), automations.WaitSourceRequest{
				Identity: identity,
				Desired:  automations.DesiredLifecycleRunning,
			})
			if err != nil {
				t.Fatalf("WaitSource(%q) unexpected error: %v", kind, err)
			}
			if running.Outcome.Observation.State != automations.ObservedLifecycleRunning {
				t.Fatalf("WaitSource(%q) state = %q, want running",
					kind, running.Outcome.Observation.State)
			}
		})
	}
}

func startFakeSource(
	t *testing.T,
	identity automations.SourceIdentity,
	kind string,
	resume *automations.SourceObservation,
) automations.Root {
	t.Helper()
	svc := rootFor(&fakeRootService{ready: true})
	started, err := svc.StartSource(context.Background(), automations.StartSourceRequest{
		Identity: identity,
		Kind:     kind,
		Resume:   resume,
	})
	if err != nil {
		t.Fatalf("StartSource() unexpected error: %v", err)
	}
	if started.Outcome.Observation.State != automations.ObservedLifecycleStarting ||
		started.Outcome.Convergence != automations.ConvergenceStatusProgressing {
		t.Fatalf("StartSource() outcome = %+v, want starting and progressing", started.Outcome)
	}
	return svc
}

func assertIdempotentConverged(
	t *testing.T,
	op string,
	outcome automations.LifecycleOutcome,
) {
	t.Helper()
	if !outcome.Idempotent || outcome.Convergence != automations.ConvergenceStatusConverged {
		t.Fatalf("%s outcome = %+v, want idempotent converged", op, outcome)
	}
}
