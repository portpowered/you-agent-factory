package load_test

import (
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/config/load"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/factory/validation"
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
	loadErr, ok := load.AsBlockingFactoryLoadError(err)
	if !ok {
		t.Fatalf("error = %v, want BlockingFactoryLoadError", err)
	}
	if len(loadErr.Targets) == 0 {
		t.Fatal("expected structured blocking validation targets")
	}
	for _, target := range loadErr.Targets {
		if strings.TrimSpace(target.Message) == "" {
			t.Fatalf("target = %#v, want non-empty message", target)
		}
	}
	if !containsTargetCode(loadErr.Targets, factoryvalidation.CodeDanglingPlaceReference) {
		t.Fatalf("targets = %#v, want dangling place reference code", loadErr.Targets)
	}
}

func containsTargetCode(targets []factoryvalidation.Target, code string) bool {
	for _, target := range targets {
		if target.Code == code {
			return true
		}
	}
	return false
}
