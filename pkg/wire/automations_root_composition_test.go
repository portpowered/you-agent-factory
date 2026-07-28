package wire_test

import (
	"context"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/automations"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/wire"
)

func TestAutomationsRootFromEdgesComposesPublishedRoot(t *testing.T) {
	t.Parallel()

	root, err := wire.AutomationsRootFromEdges(
		serviceedges.Edges{},
		"wire-automations-root",
		t.TempDir(),
	)
	if err != nil {
		t.Fatalf("AutomationsRootFromEdges() error = %v", err)
	}
	if root.Operations == nil {
		t.Fatal("AutomationsRootFromEdges() returned root without operations")
	}

	result, err := root.Reconcile(context.Background(), automations.ReconcileRequest{
		Desired: []automations.DesiredSpec{{
			AutomationID: "wire-automations-root",
			SourceID:     "source-a",
			Kind:         "schedule",
			State:        automations.DesiredLifecycleRunning,
		}},
		Observed: []automations.ObservedInstance{{
			AutomationID: "wire-automations-root",
			SourceID:     "source-a",
			InstanceID:   "instance-a",
			State:        automations.ObservedLifecycleRunning,
		}},
	})
	if err != nil {
		t.Fatalf("Root.Reconcile() error = %v", err)
	}
	if len(result.Outcomes) != 1 {
		t.Fatalf("Root.Reconcile() outcomes = %+v, want one converged source", result.Outcomes)
	}
	if result.Outcomes[0].Convergence != automations.ConvergenceStatusConverged {
		t.Fatalf(
			"Root.Reconcile() convergence = %q, want %q",
			result.Outcomes[0].Convergence,
			automations.ConvergenceStatusConverged,
		)
	}
}
