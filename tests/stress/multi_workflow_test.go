package stress_test

import (
	"testing"
	"time"
)

// TestMultiWorkflowConcurrentExecution validates that multiple workflows can run
// simultaneously without interfering with each other's token state, resources,
// or execution.
//
// Setup: 3 different workflow definitions, each with its own work-types,
// transitions, and resource pools. 5 work items submitted to each (15 total).
//
// Assertions:
//   - All 15 work items reach terminal state
//   - No cross-workflow token contamination
//   - Resource pools are isolated per workflow
//   - No data races (run with -race flag)
//   - Test passes within 30s timeout
func TestMultiWorkflowConcurrentExecution(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	const (
		numWorkflows     = 3
		itemsPerWorkflow = 5
	)

	defs := []multiWorkflowDef{
		{name: "alpha-pipeline", workType: "alpha-task", resourceName: "alpha-gpu", resourceCap: 2, workerName: "alpha-worker"},
		{name: "beta-pipeline", workType: "beta-task", resourceName: "beta-gpu", resourceCap: 3, workerName: "beta-worker"},
		{name: "gamma-pipeline", workType: "gamma-task", resourceName: "gamma-gpu", resourceCap: 1, workerName: "gamma-worker"},
	}

	harnesses := setupMultiWorkflowHarnesses(t, defs, itemsPerWorkflow)
	submitWorkflowWorkItems(t, harnesses, defs, itemsPerWorkflow)
	runHarnessesConcurrently(t, harnesses, 10*time.Second)
	assertMultiWorkflowStates(t, harnesses, defs, itemsPerWorkflow)
	assertNoCrossWorkflowContamination(t, harnesses, defs)
	assertWorkflowResourceIsolation(t, harnesses, defs)
}
