package stress_test

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"

	"github.com/portpowered/infinite-you/pkg/testutil"
)

// TestRecursiveWorkGeneration validates that a workflow where agents generate
// new work items terminates correctly and produces the expected token count.
//
// Workflow: work-item:init → process (spawns 2 children) → work-item:processing → finish → work-item:complete
// Recursion depth: 4 levels (1 → 2 → 4 → 8 = 15 total work items)
//
// Assertions:
//   - Exactly 15 tokens reach terminal state (complete)
//   - No tokens stuck in non-terminal places
//   - Total token count never exceeds expected maximum
func TestRecursiveWorkGeneration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	dir := testutil.ScaffoldFactoryDir(t, recursiveWorkCfg())
	h := testutil.NewServiceTestHarness(t, dir)

	const maxDepth = 3 // depths 0,1,2 spawn children; depth 3 does not → 1+2+4+8 = 15
	spawner := &recursiveSpawnerExecutor{maxDepth: maxDepth}
	h.SetCustomExecutor("spawner", spawner)

	// Finisher always accepts — one result per work item (15 total).
	finisherResults := make([]interfaces.WorkResult, 15)
	for i := range finisherResults {
		finisherResults[i] = interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted}
	}
	h.MockWorker("finisher", finisherResults...)

	// Submit root work item at depth 0.
	h.SubmitWork("work-item", []byte(`{"task": "root"}`))

	// Tick until all work tokens reach terminal state.
	// Use a generous tick budget — recursive spawning requires multiple rounds.
	h.RunUntilComplete(t, 10*time.Second)

	// Assert: exactly 15 tokens in work-item:complete.
	h.Assert().
		PlaceTokenCount("work-item:complete", 15).
		HasNoTokenInPlace("work-item:init").
		HasNoTokenInPlace("work-item:processing").
		HasNoTokenInPlace("work-item:failed")

	// Assert: total token count is exactly 15 (no phantom tokens).
	h.Assert().TokenCount(15)

	// Assert: spawner was called 15 times (once per work item).
	if spawner.callCount() != 15 {
		t.Errorf("expected spawner called 15 times, got %d", spawner.callCount())
	}

	// Verify depth distribution: 1 at depth 0, 2 at depth 1, 4 at depth 2, 8 at depth 3.
	depthCounts := spawner.depthDistribution()
	expectedDepths := map[int]int{0: 1, 1: 2, 2: 4, 3: 8}
	for depth, expected := range expectedDepths {
		if got := depthCounts[depth]; got != expected {
			t.Errorf("depth %d: expected %d calls, got %d", depth, expected, got)
		}
	}
}

// TestRecursiveWorkGenerationTimeout verifies the test completes within a
// reasonable time bound, proving no infinite loops occur.
func TestRecursiveWorkGenerationTimeout(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	done := make(chan struct{})
	go func() {
		defer close(done)

		dir := testutil.ScaffoldFactoryDir(t, recursiveWorkCfg())
		h := testutil.NewServiceTestHarness(t, dir)

		spawner := &recursiveSpawnerExecutor{maxDepth: 3}
		h.SetCustomExecutor("spawner", spawner)

		finisherResults := make([]interfaces.WorkResult, 15)
		for i := range finisherResults {
			finisherResults[i] = interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted}
		}
		h.MockWorker("finisher", finisherResults...)

		h.SubmitWork("work-item", []byte(`{"task": "root"}`))

		h.RunUntilComplete(t, 10*time.Second)
	}()

	select {
	case <-done:
		// Completed within timeout.
	case <-time.After(10 * time.Second):
		t.Fatal("recursive work generation did not complete within 10s timeout")
	}
}

// --- Config helpers ---

// recursiveWorkCfg returns a config for the recursive spawn workflow.
func recursiveWorkCfg() *interfaces.FactoryConfig {
	return &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{{
			Name: "work-item",
			States: []interfaces.StateConfig{
				{Name: "init", Type: interfaces.StateTypeInitial},
				{Name: "processing", Type: interfaces.StateTypeProcessing},
				{Name: "complete", Type: interfaces.StateTypeTerminal},
				{Name: "failed", Type: interfaces.StateTypeFailed},
			},
		}},
		Workers: []interfaces.WorkerConfig{{Name: "spawner"}, {Name: "finisher"}},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name: "process", WorkerTypeName: "spawner",
				Inputs:  []interfaces.IOConfig{{WorkTypeName: "work-item", StateName: "init"}},
				Outputs: []interfaces.IOConfig{{WorkTypeName: "work-item", StateName: "processing"}},
			},
			{
				Name: "finish", WorkerTypeName: "finisher",
				Inputs:  []interfaces.IOConfig{{WorkTypeName: "work-item", StateName: "processing"}},
				Outputs: []interfaces.IOConfig{{WorkTypeName: "work-item", StateName: "complete"}},
			},
		},
	}
}

// --- Custom executors ---

// recursiveSpawnerExecutor spawns 2 child work items for each input token
// if the current depth is less than maxDepth. Tracks depth via Tags["depth"].
type recursiveSpawnerExecutor struct {
	maxDepth int
	mu       sync.Mutex
	calls    int
	depths   []int // depth of each call for distribution tracking
}

func (e *recursiveSpawnerExecutor) callCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.calls
}

func (e *recursiveSpawnerExecutor) depthDistribution() map[int]int {
	e.mu.Lock()
	defer e.mu.Unlock()
	dist := make(map[int]int)
	for _, d := range e.depths {
		dist[d]++
	}
	return dist
}

func (e *recursiveSpawnerExecutor) Execute(_ context.Context, dispatch interfaces.WorkDispatch) (interfaces.WorkResult, error) {
	e.mu.Lock()
	e.calls++
	e.mu.Unlock()

	depth := 0
	parentWorkID := ""
	if len(dispatch.InputTokens) > 0 {
		tok := firstInputToken(dispatch.InputTokens)
		parentWorkID = tok.Color.WorkID
		if tok.Color.Tags != nil {
			if d, ok := tok.Color.Tags["depth"]; ok {
				depth, _ = strconv.Atoi(d)
			}
		}
	}

	e.mu.Lock()
	e.depths = append(e.depths, depth)
	e.mu.Unlock()

	result := interfaces.WorkResult{
		DispatchID:   dispatch.DispatchID,
		TransitionID: dispatch.TransitionID,
		Outcome:      interfaces.OutcomeAccepted,
	}

	// Spawn 2 children if not at max depth.
	if depth < e.maxDepth {
		nextDepth := strconv.Itoa(depth + 1)
		for i := range 2 {
			childID := fmt.Sprintf("%s-child-%d", parentWorkID, i)
			result.SpawnedWork = append(result.SpawnedWork, interfaces.TokenColor{
				WorkTypeID: "work-item",
				WorkID:     childID,
				ParentID:   parentWorkID,
				Tags: map[string]string{
					"depth": nextDepth,
				},
			})
		}
	}

	return result, nil
}
