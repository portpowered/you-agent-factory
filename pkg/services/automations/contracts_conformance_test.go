package automations_test

import (
	"context"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/automations"
)

func TestServiceConformance_FakeConstructionIsInert(t *testing.T) {
	t.Parallel()

	fake := &fakeRootService{ready: true}
	var service automations.Service = fake

	if fake.sources != nil || fake.instances != nil || fake.admittedRequests != nil {
		t.Fatal("constructing fake Service initialized source, cursor, or admission state")
	}
	if len(fake.workRequestOutcomes) != 0 || fake.emissionCount != 0 {
		t.Fatal("constructing fake Service emitted a generated Work Request")
	}

	ready, err := service.Ready(context.Background(), automations.ReadyRequest{})
	if err != nil {
		t.Fatalf("Ready() unexpected error: %v", err)
	}
	if !ready.Ready {
		t.Fatal("Ready() = false, want true")
	}
	if fake.sources != nil || fake.instances != nil || fake.admittedRequests != nil {
		t.Fatal("readiness observation started a source or wrote cursor/admission state")
	}
	if len(fake.workRequestOutcomes) != 0 || fake.emissionCount != 0 {
		t.Fatal("readiness observation emitted a generated Work Request")
	}
}

func TestServiceConformance_EquivalentDesiredAndCursorAreStableAcrossRestart(t *testing.T) {
	t.Parallel()

	identity := automations.SourceIdentity{
		AutomationID: "auto-conformance",
		SourceID:     "source-conformance",
	}
	resume := automations.SourceObservation{
		Identity:   identity,
		InstanceID: "instance:auto-conformance:source-conformance",
		State:      automations.ObservedLifecycleRunning,
		Cursor:     "opaque-cursor-conformance",
	}
	desired := automations.DesiredSpec{
		AutomationID: identity.AutomationID,
		SourceID:     identity.SourceID,
		Kind:         "opaque-source-kind",
		State:        automations.DesiredLifecycleRunning,
	}

	beforeRestart := exerciseRestartInput(t, &fakeRootService{ready: true}, desired, resume)
	afterRestart := exerciseRestartInput(t, &fakeRootService{ready: true}, desired, resume)

	if beforeRestart != afterRestart {
		t.Fatalf(
			"equivalent restart input outcomes differ: before=%+v after=%+v",
			beforeRestart,
			afterRestart,
		)
	}
	if beforeRestart.Observation.InstanceID != resume.InstanceID ||
		beforeRestart.Observation.Cursor != resume.Cursor {
		t.Fatalf(
			"restart outcome identity/cursor = %q/%q, want %q/%q",
			beforeRestart.Observation.InstanceID,
			beforeRestart.Observation.Cursor,
			resume.InstanceID,
			resume.Cursor,
		)
	}
	if !beforeRestart.Idempotent ||
		beforeRestart.Convergence != automations.ConvergenceStatusConverged {
		t.Fatalf("restart outcome = %+v, want idempotent convergence", beforeRestart)
	}
}

func exerciseRestartInput(
	t *testing.T,
	fake *fakeRootService,
	desired automations.DesiredSpec,
	resume automations.SourceObservation,
) automations.LifecycleOutcome {
	t.Helper()

	var service automations.Service = fake
	reconciled, err := service.Reconcile(context.Background(), automations.ReconcileRequest{
		Desired: []automations.DesiredSpec{desired},
		Observed: []automations.ObservedInstance{{
			AutomationID: resume.Identity.AutomationID,
			SourceID:     resume.Identity.SourceID,
			InstanceID:   resume.InstanceID,
			State:        resume.State,
		}},
	})
	if err != nil {
		t.Fatalf("Reconcile() unexpected error: %v", err)
	}
	if len(reconciled.Outcomes) != 1 {
		t.Fatalf("Reconcile() outcomes len = %d, want 1", len(reconciled.Outcomes))
	}
	if outcome := reconciled.Outcomes[0]; outcome.InstanceID != resume.InstanceID ||
		outcome.Action != automations.ConvergenceActionUnchanged ||
		outcome.Convergence != automations.ConvergenceStatusConverged {
		t.Fatalf("Reconcile() outcome = %+v, want stable converged identity", outcome)
	}

	started, err := service.StartSource(context.Background(), automations.StartSourceRequest{
		Identity: resume.Identity,
		Kind:     desired.Kind,
		Resume:   &resume,
	})
	if err != nil {
		t.Fatalf("StartSource() unexpected error: %v", err)
	}
	return started.Outcome
}
