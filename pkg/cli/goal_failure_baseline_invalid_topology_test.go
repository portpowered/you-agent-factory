package cli

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Hermetic S02 failure-baseline fixtures for one-shot you run --factory when the
// selected factory.json carries invalid @you/goal-shaped topology.

const goalFailureBaselineInvalidTopologyJSON = `{
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

func TestFailureBaseline_InvalidTopology_RunFactoryCommandRejectsGoalShapedGraphReferences(t *testing.T) {
	dir := t.TempDir()
	factoryPath := filepath.Join(dir, "factory.json")
	if err := os.WriteFile(factoryPath, []byte(goalFailureBaselineInvalidTopologyJSON), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	root := NewRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{
		"run",
		"--factory", factoryPath,
		"--no-record",
		"--quiet",
		"invalid-topology-baseline",
	})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected invalid goal topology to fail before invocation")
	}
	if !strings.Contains(err.Error(), "invalid graph references") {
		t.Fatalf("error = %q, want invalid graph references guidance", err.Error())
	}
	if !strings.Contains(err.Error(), "blocking validation targets") {
		t.Fatalf("error = %q, want blocking validation target count", err.Error())
	}
}
