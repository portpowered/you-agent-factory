package stress_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"

	"github.com/portpowered/infinite-you/pkg/testutil"
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

	type workflowDef struct {
		name         string
		workType     string
		resourceName string
		resourceCap  int
		workerName   string
	}

	defs := []workflowDef{
		{name: "alpha-pipeline", workType: "alpha-task", resourceName: "alpha-gpu", resourceCap: 2, workerName: "alpha-worker"},
		{name: "beta-pipeline", workType: "beta-task", resourceName: "beta-gpu", resourceCap: 3, workerName: "beta-worker"},
		{name: "gamma-pipeline", workType: "gamma-task", resourceName: "gamma-gpu", resourceCap: 1, workerName: "gamma-worker"},
	}

	// Build configs and harnesses for each workflow.
	harnesses := make([]*testutil.ServiceTestHarness, numWorkflows)
	for i, def := range defs {
		cfg := &interfaces.FactoryConfig{
			WorkTypes: []interfaces.WorkTypeConfig{{
				Name: def.workType,
				States: []interfaces.StateConfig{
					{Name: "init", Type: interfaces.StateTypeInitial},
					{Name: "processing", Type: interfaces.StateTypeProcessing},
					{Name: "complete", Type: interfaces.StateTypeTerminal},
					{Name: "failed", Type: interfaces.StateTypeFailed},
				},
			}},
			Resources: []interfaces.ResourceConfig{{Name: def.resourceName, Capacity: def.resourceCap}},
			Workers:   []interfaces.WorkerConfig{{Name: def.workerName}, {Name: def.workerName + "-finish"}},
			Workstations: []interfaces.FactoryWorkstationConfig{
				{
					Name: def.name + "-process", WorkerTypeName: def.workerName,
					Inputs: []interfaces.IOConfig{
						{WorkTypeName: def.workType, StateName: "init"},
						{WorkTypeName: def.resourceName, StateName: "available"},
					},
					Outputs: []interfaces.IOConfig{{WorkTypeName: def.workType, StateName: "processing"}},
				},
				{
					Name: def.name + "-finish", WorkerTypeName: def.workerName + "-finish",
					Inputs: []interfaces.IOConfig{{WorkTypeName: def.workType, StateName: "processing"}},
					Outputs: []interfaces.IOConfig{
						{WorkTypeName: def.workType, StateName: "complete"},
						{WorkTypeName: def.resourceName, StateName: "available"},
					},
				},
			},
		}
		dir := testutil.ScaffoldFactoryDir(t, cfg)
		h := testutil.NewServiceTestHarness(t, dir)

		// Mock workers: enough results for all items (process + finish).
		processResults := make([]interfaces.WorkResult, itemsPerWorkflow)
		finishResults := make([]interfaces.WorkResult, itemsPerWorkflow)
		for j := range itemsPerWorkflow {
			processResults[j] = interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted}
			finishResults[j] = interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted}
			_ = j
		}
		h.MockWorker(def.workerName, processResults...)
		h.MockWorker(def.workerName+"-finish", finishResults...)

		harnesses[i] = h
	}

	// Submit 5 work items to each workflow.
	for i, def := range defs {
		for j := range itemsPerWorkflow {
			err := harnesses[i].SubmitFull(context.Background(), []interfaces.SubmitRequest{{
				WorkTypeID: def.workType,
				WorkID:     fmt.Sprintf("%s-work-%d", def.name, j),
				TraceID:    fmt.Sprintf("%s-trace-%d", def.name, j),
				Payload:    fmt.Appendf(nil, `{"workflow": %q, "item": %d}`, def.name, j),
			}})
			if err != nil {
				t.Fatalf("workflow %q: submit %d failed: %v", def.name, j, err)
			}
		}
	}

	// Run all workflows concurrently.
	var wg sync.WaitGroup

	for i := range numWorkflows {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			harnesses[idx].RunUntilComplete(t, 10*time.Second)
		}(i)
	}

	// Wait with timeout.
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// All completed.
	case <-time.After(10 * time.Second):
		t.Fatal("multi-workflow execution did not complete within 10s timeout")
	}

	// Assert: all work items reach terminal state in each workflow.
	for i, def := range defs {
		snap := harnesses[i].Marking()
		completeCount := len(snap.TokensInPlace(def.workType + ":complete"))
		initCount := len(snap.TokensInPlace(def.workType + ":init"))
		processingCount := len(snap.TokensInPlace(def.workType + ":processing"))
		failedCount := len(snap.TokensInPlace(def.workType + ":failed"))

		if completeCount != itemsPerWorkflow {
			t.Errorf("workflow %q: expected %d tokens in complete, got %d (init=%d, processing=%d, failed=%d)",
				def.name, itemsPerWorkflow, completeCount, initCount, processingCount, failedCount)
		}
		if initCount != 0 {
			t.Errorf("workflow %q: expected 0 tokens in init, got %d", def.name, initCount)
		}
		if processingCount != 0 {
			t.Errorf("workflow %q: expected 0 tokens in processing, got %d", def.name, processingCount)
		}
	}

	// Assert: no cross-workflow token contamination.
	// Each workflow's tokens should only reference its own work type or resource.
	for i, def := range defs {
		snap := harnesses[i].Marking()
		// Collect all other workflow work types and resources for cross-contamination check.
		foreignTypes := make(map[string]string) // typeID → owning workflow name
		for j, other := range defs {
			if j == i {
				continue
			}
			foreignTypes[other.workType] = other.name
			foreignTypes[other.resourceName] = other.name
		}
		for _, tok := range snap.Tokens {
			if owner, isForeign := foreignTypes[tok.Color.WorkTypeID]; isForeign {
				t.Errorf("workflow %q: found token with WorkTypeID %q belonging to workflow %q — cross-contamination detected",
					def.name, tok.Color.WorkTypeID, owner)
			}
		}
	}

	// Assert: resource pools are isolated — each workflow's resource tokens
	// are back in the available place.
	for i, def := range defs {
		snap := harnesses[i].Marking()
		resourcePlace := def.resourceName + ":available"
		resourceTokens := snap.TokensInPlace(resourcePlace)
		if len(resourceTokens) != def.resourceCap {
			t.Errorf("workflow %q: expected %d resource tokens in %q, got %d",
				def.name, def.resourceCap, resourcePlace, len(resourceTokens))
		}
	}
}
