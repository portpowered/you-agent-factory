package service_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	automations "github.com/portpowered/infinite-you/pkg/services/automations"
	reconciliation "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/reconciliation"
	reconciliationwire "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/reconciliation/wire"
)

func TestReconcileDecisionTable(t *testing.T) {
	t.Parallel()

	const (
		automationID = "automation-a"
		sourceID     = "source-a"
		instanceID   = "instance-a"
	)
	tests := []struct {
		name        string
		desired     automations.DesiredLifecycleState
		observed    automations.ObservedLifecycleState
		hasObserved bool
		wantAction  automations.ConvergenceAction
		wantState   automations.ObservedLifecycleState
		wantStatus  automations.ConvergenceStatus
	}{
		{"absent starts", automations.DesiredLifecycleRunning, "", false, automations.ConvergenceActionCreated, automations.ObservedLifecyclePending, automations.ConvergenceStatusProgressing},
		{"absent stays stopped", automations.DesiredLifecycleStopped, "", false, automations.ConvergenceActionRemoved, automations.ObservedLifecycleStopped, automations.ConvergenceStatusConverged},
		{"running unchanged", automations.DesiredLifecycleRunning, automations.ObservedLifecycleRunning, true, automations.ConvergenceActionUnchanged, automations.ObservedLifecycleRunning, automations.ConvergenceStatusConverged},
		{"stopped starts", automations.DesiredLifecycleRunning, automations.ObservedLifecycleStopped, true, automations.ConvergenceActionUpdated, automations.ObservedLifecycleStopped, automations.ConvergenceStatusProgressing},
		{"starting progresses", automations.DesiredLifecycleRunning, automations.ObservedLifecycleStarting, true, automations.ConvergenceActionUnchanged, automations.ObservedLifecycleStarting, automations.ConvergenceStatusProgressing},
		{"stopping restarts", automations.DesiredLifecycleRunning, automations.ObservedLifecycleStopping, true, automations.ConvergenceActionUpdated, automations.ObservedLifecycleStopping, automations.ConvergenceStatusProgressing},
		{"stopped unchanged", automations.DesiredLifecycleStopped, automations.ObservedLifecycleStopped, true, automations.ConvergenceActionUnchanged, automations.ObservedLifecycleStopped, automations.ConvergenceStatusConverged},
		{"running stops", automations.DesiredLifecycleStopped, automations.ObservedLifecycleRunning, true, automations.ConvergenceActionRemoved, automations.ObservedLifecycleRunning, automations.ConvergenceStatusProgressing},
		{"stopping progresses", automations.DesiredLifecycleStopped, automations.ObservedLifecycleStopping, true, automations.ConvergenceActionUnchanged, automations.ObservedLifecycleStopping, automations.ConvergenceStatusProgressing},
		{"failed remains failed", automations.DesiredLifecycleRunning, automations.ObservedLifecycleFailed, true, automations.ConvergenceActionUnchanged, automations.ObservedLifecycleFailed, automations.ConvergenceStatusFailed},
		{"cancelled remains cancelled", automations.DesiredLifecycleRunning, automations.ObservedLifecycleCancelled, true, automations.ConvergenceActionUnchanged, automations.ObservedLifecycleCancelled, automations.ConvergenceStatusCancelled},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			request := automations.ReconcileRequest{
				Desired: []automations.DesiredSpec{{
					AutomationID: automationID,
					SourceID:     sourceID,
					Kind:         "opaque-kind",
					State:        test.desired,
				}},
			}
			if test.hasObserved {
				request.Observed = []automations.ObservedInstance{{
					AutomationID: automationID,
					SourceID:     sourceID,
					InstanceID:   instanceID,
					State:        test.observed,
				}}
			}

			outcome := reconcileOne(t, newService(), request)
			if outcome.Action != test.wantAction ||
				outcome.Observed != test.wantState ||
				outcome.Convergence != test.wantStatus {
				t.Fatalf("outcome = %+v, want action/state/status %q/%q/%q",
					outcome, test.wantAction, test.wantState, test.wantStatus)
			}
			if test.hasObserved && outcome.InstanceID != instanceID {
				t.Fatalf("instance ID = %q, want preserved %q", outcome.InstanceID, instanceID)
			}
			if !test.hasObserved && outcome.InstanceID == "" {
				t.Fatal("derived instance ID is empty")
			}
		})
	}
}

func TestReconcileRemovesObservedSourceMissingFromDesiredState(t *testing.T) {
	t.Parallel()

	outcome := reconcileOne(t, newService(), automations.ReconcileRequest{
		Observed: []automations.ObservedInstance{{
			AutomationID: "automation-orphan",
			SourceID:     "source-orphan",
			InstanceID:   "instance-orphan",
			State:        automations.ObservedLifecycleRunning,
		}},
	})
	if outcome.Desired != automations.DesiredLifecycleStopped ||
		outcome.Action != automations.ConvergenceActionRemoved ||
		outcome.Convergence != automations.ConvergenceStatusProgressing {
		t.Fatalf("orphan outcome = %+v, want removal toward stopped", outcome)
	}
}

func TestReconcileEquivalentDetachedInputsProduceStableCanonicalResults(t *testing.T) {
	t.Parallel()

	firstRequest := reconciliationRequestInOrder("b", "a")
	secondRequest := reconciliationRequestInOrder("a", "b")
	service := newService()

	first, err := service.Reconcile(context.Background(), firstRequest)
	if err != nil {
		t.Fatalf("first Reconcile: %v", err)
	}
	second, err := service.Reconcile(context.Background(), secondRequest)
	if err != nil {
		t.Fatalf("second Reconcile: %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("equivalent results differ:\nfirst:  %+v\nsecond: %+v", first, second)
	}

	original := append([]automations.ConvergenceOutcome(nil), first.Outcomes...)
	first.Outcomes[0].InstanceID = "caller-mutated"
	again, err := service.Reconcile(context.Background(), firstRequest)
	if err != nil {
		t.Fatalf("Reconcile after caller mutation: %v", err)
	}
	if !reflect.DeepEqual(again.Outcomes, original) {
		t.Fatalf("caller mutation changed later result: got %+v, want %+v", again.Outcomes, original)
	}
}

func TestReconcileOrdersSourcesWithinOneAutomation(t *testing.T) {
	t.Parallel()

	service := reconciliationwire.NewService()
	result, err := service.Reconcile(context.Background(), automations.ReconcileRequest{
		Desired: []automations.DesiredSpec{
			{AutomationID: "automation", SourceID: "source-b", Kind: "schedule", State: automations.DesiredLifecycleRunning},
			{AutomationID: "automation", SourceID: "source-a", Kind: "schedule", State: automations.DesiredLifecycleRunning},
		},
	})
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if got := []string{
		result.Outcomes[0].SourceID,
		result.Outcomes[1].SourceID,
	}; !equalStrings(got, []string{"source-a", "source-b"}) {
		t.Fatalf("decision source order = %v, want source-a then source-b", got)
	}
}

func TestReconcileRejectsInvalidAndContradictoryInputs(t *testing.T) {
	t.Parallel()

	validDesired := automations.DesiredSpec{
		AutomationID: "automation-a",
		SourceID:     "source-a",
		Kind:         "kind",
		State:        automations.DesiredLifecycleRunning,
	}
	validObserved := automations.ObservedInstance{
		AutomationID: "automation-a",
		SourceID:     "source-a",
		InstanceID:   "instance-a",
		State:        automations.ObservedLifecycleRunning,
	}
	tests := []struct {
		name    string
		request automations.ReconcileRequest
	}{
		{"empty request", automations.ReconcileRequest{}},
		{"blank desired identity", automations.ReconcileRequest{Desired: []automations.DesiredSpec{{Kind: "kind", State: automations.DesiredLifecycleRunning}}}},
		{"blank kind", automations.ReconcileRequest{Desired: []automations.DesiredSpec{{AutomationID: "a", SourceID: "s", State: automations.DesiredLifecycleRunning}}}},
		{"invalid desired state", automations.ReconcileRequest{Desired: []automations.DesiredSpec{{AutomationID: "a", SourceID: "s", Kind: "kind", State: "paused"}}}},
		{"duplicate desired", automations.ReconcileRequest{Desired: []automations.DesiredSpec{validDesired, validDesired}}},
		{"blank observed instance", automations.ReconcileRequest{Observed: []automations.ObservedInstance{{AutomationID: "a", SourceID: "s", State: automations.ObservedLifecycleRunning}}}},
		{"invalid observed state", automations.ReconcileRequest{Observed: []automations.ObservedInstance{{AutomationID: "a", SourceID: "s", InstanceID: "i", State: "unknown"}}}},
		{"duplicate observed", automations.ReconcileRequest{Observed: []automations.ObservedInstance{validObserved, validObserved}}},
		{"instance reused by another source", automations.ReconcileRequest{Observed: []automations.ObservedInstance{
			validObserved,
			{AutomationID: "automation-a", SourceID: "source-b", InstanceID: validObserved.InstanceID, State: automations.ObservedLifecycleRunning},
		}}},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			result, err := newService().Reconcile(context.Background(), test.request)
			if err == nil {
				t.Fatalf("Reconcile() = %+v, nil error; want typed invalid error", result)
			}
			var typed *automations.Error
			if !errors.As(err, &typed) || typed.Code != automations.ErrorCodeInvalid {
				t.Fatalf("error = %T %v, want invalid *automations.Error", err, err)
			}
			if !errors.Is(err, automations.ErrInvalidRequest) {
				t.Fatalf("error = %v, want errors.Is ErrInvalidRequest", err)
			}
			if len(result.Outcomes) != 0 {
				t.Fatalf("invalid result outcomes = %+v, want none", result.Outcomes)
			}
		})
	}
}

func reconciliationRequestInOrder(sourceIDs ...string) automations.ReconcileRequest {
	request := automations.ReconcileRequest{
		Desired:  make([]automations.DesiredSpec, 0, len(sourceIDs)),
		Observed: make([]automations.ObservedInstance, 0, len(sourceIDs)),
	}
	for _, sourceID := range sourceIDs {
		request.Desired = append(request.Desired, automations.DesiredSpec{
			AutomationID: "automation",
			SourceID:     sourceID,
			Kind:         "kind",
			State:        automations.DesiredLifecycleRunning,
		})
		request.Observed = append(request.Observed, automations.ObservedInstance{
			AutomationID: "automation",
			SourceID:     sourceID,
			InstanceID:   "instance-" + sourceID,
			State:        automations.ObservedLifecycleRunning,
		})
	}
	return request
}

func reconcileOne(
	t *testing.T,
	service reconciliation.Service,
	request automations.ReconcileRequest,
) automations.ConvergenceOutcome {
	t.Helper()
	result, err := service.Reconcile(context.Background(), request)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if len(result.Outcomes) != 1 {
		t.Fatalf("outcomes len = %d, want 1", len(result.Outcomes))
	}
	return result.Outcomes[0]
}

func newService() reconciliation.Service {
	return reconciliationwire.NewService()
}
