package automations

import (
	"context"
	"sync"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/automations"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	reconciliationAutomationID = "fun-automations-reconcile"
	reconciliationSourceID     = "schedule-source"
	reconciliationInstanceID   = "instance-schedule-source"
	reconciliationSourceKind   = "schedule"
)

func TestBuildProcessRemainsReconciliationInertBeforeExplicitRootInvocation(t *testing.T) {
	t.Parallel()

	var submissionsMu sync.Mutex
	var submissions []work.FactorySubmissionRecord
	recorder := func(record work.FactorySubmissionRecord) {
		submissionsMu.Lock()
		submissions = append(submissions, record)
		submissionsMu.Unlock()
	}

	_ = support.BuildProcess(t, serviceedges.Edges{
		SubmissionRecorder: recorder,
	})

	submissionsMu.Lock()
	defer submissionsMu.Unlock()
	if len(submissions) != 0 {
		t.Fatalf(
			"BuildProcess() submitted %d Work records, want zero before explicit Automations Root reconciliation",
			len(submissions),
		)
	}
}

func TestAutomationsReconciliationAdmitsThroughPublishedRootAfterComposition(t *testing.T) {
	t.Parallel()

	dir := support.ScaffoldFactory(t, reconciliationFactoryConfig())
	_ = support.BuildProcess(t, serviceedges.Edges{})

	root := support.AutomationsRootFromProcessEdges(t, serviceedges.Edges{}, dir)
	result, err := root.Reconcile(context.Background(), automations.ReconcileRequest{
		Desired: []automations.DesiredSpec{{
			AutomationID: reconciliationAutomationID,
			SourceID:     reconciliationSourceID,
			Kind:         reconciliationSourceKind,
			State:        automations.DesiredLifecycleRunning,
		}},
		Observed: []automations.ObservedInstance{{
			AutomationID: reconciliationAutomationID,
			SourceID:     reconciliationSourceID,
			InstanceID:   reconciliationInstanceID,
			State:        automations.ObservedLifecycleRunning,
		}},
	})
	if err != nil {
		t.Fatalf("Root.Reconcile() error = %v", err)
	}
	if len(result.Outcomes) != 1 {
		t.Fatalf("Root.Reconcile() outcomes = %+v, want one converged source", result.Outcomes)
	}
	outcome := result.Outcomes[0]
	if outcome.Convergence != automations.ConvergenceStatusConverged {
		t.Fatalf(
			"Root.Reconcile() convergence = %q, want %q",
			outcome.Convergence,
			automations.ConvergenceStatusConverged,
		)
	}
	if outcome.InstanceID != reconciliationInstanceID {
		t.Fatalf(
			"Root.Reconcile() instance ID = %q, want %q",
			outcome.InstanceID,
			reconciliationInstanceID,
		)
	}
}

func TestAutomationsReconcileAdmitsAbsentSourceThroughPublishedRootAfterComposition(t *testing.T) {
	t.Parallel()

	dir := support.ScaffoldFactory(t, reconciliationFactoryConfig())
	_ = support.BuildProcess(t, serviceedges.Edges{})

	root := support.AutomationsRootFromProcessEdges(t, serviceedges.Edges{}, dir)
	result, err := root.Reconcile(context.Background(), automations.ReconcileRequest{
		Desired: []automations.DesiredSpec{{
			AutomationID: reconciliationAutomationID,
			SourceID:     reconciliationSourceID,
			Kind:         reconciliationSourceKind,
			State:        automations.DesiredLifecycleRunning,
		}},
	})
	if err != nil {
		t.Fatalf("Root.Reconcile() error = %v", err)
	}
	if len(result.Outcomes) != 1 {
		t.Fatalf("Root.Reconcile() outcomes = %+v, want one admission outcome", result.Outcomes)
	}
	outcome := result.Outcomes[0]
	if outcome.Action != automations.ConvergenceActionCreated {
		t.Fatalf(
			"Root.Reconcile() action = %q, want %q",
			outcome.Action,
			automations.ConvergenceActionCreated,
		)
	}
	if outcome.Convergence != automations.ConvergenceStatusProgressing {
		t.Fatalf(
			"Root.Reconcile() convergence = %q, want %q",
			outcome.Convergence,
			automations.ConvergenceStatusProgressing,
		)
	}
	if outcome.InstanceID == "" {
		t.Fatal("Root.Reconcile() returned empty instance ID for absent source admission")
	}
}

func reconciliationFactoryConfig() map[string]any {
	return map[string]any{
		"name": "automations-reconciliation-proof",
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
			},
		}},
		"workers": []map[string]string{{"name": "worker"}},
		"workstations": []map[string]any{{
			"name":     "process",
			"worker":   "worker",
			"behavior": "STANDARD",
			"inputs":   []map[string]string{{"workType": "task", "state": "init"}},
			"outputs":  []map[string]string{{"workType": "task", "state": "complete"}},
		}},
	}
}
