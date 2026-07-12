package load_test

import (
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/config/load"
)

// Hermetic S02 failure-baseline fixtures for invalid @you/goal-shaped factory
// topology at the canonical JSON load boundary.

const failureBaselineInvalidGoalTopologyJSON = `{
  "name": "@you/goal",
  "workTypes": [{
    "name": "goal",
    "handlingBehavior": ["DEFAULT"],
    "states": [
      {"name": "init", "type": "INITIAL"},
      {"name": "plan", "type": "PROCESSING"},
      {"name": "complete", "type": "TERMINAL"},
      {"name": "failed", "type": "FAILED"}
    ]
  }],
  "workers": [{"name": "goal-planner", "type": "AGENT_WORKER"}],
  "workstations": [{
    "name": "plan-goal",
    "type": "AGENT_RUN",
    "worker": "goal-planner",
    "inputs": [{"workType": "goal", "state": "init"}],
    "outputs": [{"workType": "goal", "state": "missing-plan-state"}],
    "onFailure": [{"workType": "goal", "state": "failed"}]
  }]
}`

func TestFailureBaseline_InvalidTopology_GoalShapedCanonicalJSONRejectsGraphReferences(t *testing.T) {
	_, err := load.LoadFromCanonicalJSON([]byte(failureBaselineInvalidGoalTopologyJSON), load.LoadOptions{})
	if err == nil {
		t.Fatal("expected invalid goal topology to fail load")
	}
	if !load.IsInvalidNamedFactory(err) {
		t.Fatalf("error = %v, want ErrInvalidNamedFactory", err)
	}
	if !strings.Contains(err.Error(), "invalid graph references") {
		t.Fatalf("error = %q, want invalid graph references guidance", err.Error())
	}
	if !strings.Contains(err.Error(), "blocking validation targets") {
		t.Fatalf("error = %q, want blocking validation target count", err.Error())
	}
}
