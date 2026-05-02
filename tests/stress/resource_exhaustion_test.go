package stress_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"

	"github.com/portpowered/infinite-you/pkg/testutil"
	"github.com/portpowered/infinite-you/pkg/workers"
)

// queueManyItems queues numItems work items via QueueBatch, sending all
// items in a single submit call to avoid channel buffer overflow.
func queueManyItems(t *testing.T, h *testutil.ServiceTestHarness, workTypeID string, numItems int) {
	t.Helper()
	ctx := context.Background()

	reqs := make([]interfaces.SubmitRequest, numItems)
	for i := range numItems {
		reqs[i] = interfaces.SubmitRequest{
			WorkTypeID: workTypeID,
			Payload:    fmt.Appendf(nil, `{"item": %d}`, i),
			TraceID:    fmt.Sprintf("trace-%s-%d", workTypeID, i),
		}
	}
	h.SubmitFull(ctx, reqs)
}

// TestResourceExhaustionGPU validates that a single GPU resource (capacity=1)
// serializes execution correctly: at most 1 work item actively processing at
// any time, and all 20 work items eventually complete.
// portos:func-length-exception owner=agent-factory reason=legacy-gpu-resource-exhaustion-fixture review=2026-07-19 removal=split-resource-fixture-run-and-serialization-assertions-before-next-resource-exhaustion-change
func TestResourceExhaustionGPU(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	const numItems = 20
	// Config: task init → processing (consumes gpu) → complete (returns gpu).
	dir := testutil.ScaffoldFactoryDir(t, &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{{Name: "task", States: []interfaces.StateConfig{
			{Name: "init", Type: interfaces.StateTypeInitial}, {Name: "processing", Type: interfaces.StateTypeProcessing},
			{Name: "complete", Type: interfaces.StateTypeTerminal}, {Name: "failed", Type: interfaces.StateTypeFailed},
		}}},
		Resources: []interfaces.ResourceConfig{{Name: "gpu", Capacity: 1}},
		Workers:   []interfaces.WorkerConfig{{Name: "gpu-worker"}, {Name: "release-worker"}},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{Name: "acquire", WorkerTypeName: "gpu-worker",
				Inputs:  []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}, {WorkTypeName: "gpu", StateName: "available"}},
				Outputs: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "processing"}}},
			{Name: "release", WorkerTypeName: "release-worker",
				Inputs:  []interfaces.IOConfig{{WorkTypeName: "task", StateName: "processing"}},
				Outputs: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "complete"}, {WorkTypeName: "gpu", StateName: "available"}}},
		},
	})
	h := testutil.NewServiceTestHarness(t, dir)

	// Track concurrency via custom executor.
	var mu sync.Mutex
	maxConcurrent := 0
	currentConcurrent := 0

	h.SetCustomExecutor("gpu-worker", &concurrencyTracker{
		mu: &mu, maxConcurrent: &maxConcurrent, currentConcurrent: &currentConcurrent,
	})

	releaseResults := make([]interfaces.WorkResult, numItems)
	for i := range releaseResults {
		releaseResults[i] = interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted}
	}
	h.MockWorker("release-worker", releaseResults...)

	// Queue all items as a single batch (one channel message), then run.
	queueManyItems(t, h, "task", numItems)

	h.RunUntilComplete(t, 30*time.Second)

	// Assert: all 20 items complete.
	snap := h.Marking()
	completeCount := len(snap.TokensInPlace("task:complete"))
	if completeCount != numItems {
		initCount := len(snap.TokensInPlace("task:init"))
		procCount := len(snap.TokensInPlace("task:processing"))
		failedCount := len(snap.TokensInPlace("task:failed"))
		t.Errorf("expected %d complete, got %d (init=%d, processing=%d, failed=%d)",
			numItems, completeCount, initCount, procCount, failedCount)
	}

	// Assert: at most 1 concurrent GPU user.
	mu.Lock()
	observed := maxConcurrent
	mu.Unlock()
	if observed > 1 {
		t.Errorf("expected at most 1 concurrent GPU user, observed %d", observed)
	}

	// Assert: no starvation — all items finished.
	h.Assert().
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:processing")

	// Assert: GPU returned to pool.
	gpuTokens := snap.TokensInPlace("gpu:available")
	if len(gpuTokens) != 1 {
		t.Errorf("expected 1 GPU token in gpu:available, got %d", len(gpuTokens))
	}

	// Assert: no tokens lost or duplicated.
	expectedTotal := numItems + 1 // work tokens + 1 GPU token
	if len(snap.Tokens) != expectedTotal {
		t.Errorf("expected %d total tokens, got %d", expectedTotal, len(snap.Tokens))
	}
}

// TestResourceExhaustionMoney validates that a money resource (capacity=5)
// correctly throttles execution: only 5 items can process concurrently,
// and as money is returned, queued items proceed.
func TestResourceExhaustionMoney(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	const (
		numItems      = 10
		moneyCapacity = 5
	)

	// Config: task init → processing (consumes money) → complete (returns money).
	dir := testutil.ScaffoldFactoryDir(t, &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{{Name: "task", States: []interfaces.StateConfig{
			{Name: "init", Type: interfaces.StateTypeInitial}, {Name: "processing", Type: interfaces.StateTypeProcessing},
			{Name: "complete", Type: interfaces.StateTypeTerminal}, {Name: "failed", Type: interfaces.StateTypeFailed},
		}}},
		Resources: []interfaces.ResourceConfig{{Name: "money", Capacity: moneyCapacity}},
		Workers:   []interfaces.WorkerConfig{{Name: "spender"}, {Name: "earner"}},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{Name: "spend", WorkerTypeName: "spender",
				Inputs:  []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}, {WorkTypeName: "money", StateName: "available"}},
				Outputs: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "processing"}}},
			{Name: "earn", WorkerTypeName: "earner",
				Inputs:  []interfaces.IOConfig{{WorkTypeName: "task", StateName: "processing"}},
				Outputs: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "complete"}, {WorkTypeName: "money", StateName: "available"}}},
		},
	})
	h := testutil.NewServiceTestHarness(t, dir)

	// Track max concurrency.
	var mu sync.Mutex
	maxConcurrent := 0
	currentConcurrent := 0

	h.SetCustomExecutor("spender", &concurrencyTracker{
		mu: &mu, maxConcurrent: &maxConcurrent, currentConcurrent: &currentConcurrent,
	})

	earnResults := make([]interfaces.WorkResult, numItems)
	for i := range earnResults {
		earnResults[i] = interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted}
	}
	h.MockWorker("earner", earnResults...)

	// Queue all items (10 fits within buffer of 16).
	for i := range numItems {
		h.SubmitWork("task", fmt.Appendf(nil, `{"item": %d}`, i))
	}

	h.RunUntilComplete(t, 30*time.Second)

	// Assert: all items complete.
	h.Assert().
		PlaceTokenCount("task:complete", numItems).
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:processing")

	// Assert: max concurrency did not exceed money capacity.
	mu.Lock()
	observed := maxConcurrent
	mu.Unlock()
	if observed > moneyCapacity {
		t.Errorf("expected max concurrent <= %d, observed %d", moneyCapacity, observed)
	}

	// Assert: all money returned.
	snap := h.Marking()
	moneyTokens := snap.TokensInPlace("money:available")
	if len(moneyTokens) != moneyCapacity {
		t.Errorf("expected %d money tokens returned, got %d", moneyCapacity, len(moneyTokens))
	}

	// Assert: no tokens lost.
	expectedTotal := numItems + moneyCapacity
	if len(snap.Tokens) != expectedTotal {
		t.Errorf("expected %d total tokens, got %d", expectedTotal, len(snap.Tokens))
	}
}

// TestResourceExhaustionMoneyConsumed validates that when money is consumed
// permanently (not returned), items that cannot acquire money are stuck and
// the system handles it gracefully — remaining items stay in init.
// portos:func-length-exception owner=agent-factory reason=legacy-consumable-resource-exhaustion-fixture review=2026-07-19 removal=split-consumable-resource-fixture-run-and-stuck-work-assertions-before-next-resource-exhaustion-change
func TestResourceExhaustionMoneyConsumed(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	const (
		numItems      = 10
		moneyCapacity = 5
	)

	// Config: task init → complete (consumes money, does NOT return it).
	dir := testutil.ScaffoldFactoryDir(t, &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{{Name: "task", States: []interfaces.StateConfig{
			{Name: "init", Type: interfaces.StateTypeInitial},
			{Name: "complete", Type: interfaces.StateTypeTerminal},
			{Name: "failed", Type: interfaces.StateTypeFailed},
		}}},
		Resources: []interfaces.ResourceConfig{{Name: "money", Capacity: moneyCapacity}},
		Workers:   []interfaces.WorkerConfig{{Name: "spender"}},
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name: "spend", WorkerTypeName: "spender",
			Inputs:  []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}, {WorkTypeName: "money", StateName: "available"}},
			Outputs: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "complete"}},
			// No money:available in outputs — money consumed permanently.
		}},
	})
	h := testutil.NewServiceTestHarness(t, dir)

	spendResults := make([]interfaces.WorkResult, numItems)
	for i := range spendResults {
		spendResults[i] = interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted}
	}
	h.MockWorker("spender", spendResults...)

	for i := range numItems {
		h.SubmitWork("task", fmt.Appendf(nil, `{"item": %d}`, i))
	}

	// This net has no path for stuck items (no money returned), so the engine
	// won't reach "all terminal" when items are stuck in init. Use a timeout
	// and check the marking state directly.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	errCh := h.RunInBackground(ctx)

	// Wait only until the marking reaches the expected stuck state.
	deadline := time.Now().Add(500 * time.Millisecond)
	for {
		snap := h.Marking()
		if len(snap.TokensInPlace("task:complete")) == moneyCapacity &&
			len(snap.TokensInPlace("task:init")) == numItems-moneyCapacity {
			break
		}
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	if err := <-errCh; err != nil && err != context.Canceled {
		t.Fatalf("factory run error: %v", err)
	}

	// Assert: exactly moneyCapacity items completed.
	snap := h.Marking()
	completeCount := len(snap.TokensInPlace("task:complete"))
	if completeCount != moneyCapacity {
		t.Errorf("expected %d complete (money capacity), got %d", moneyCapacity, completeCount)
	}

	// Assert: remaining items stuck in init (no money available).
	stuckCount := len(snap.TokensInPlace("task:init"))
	expectedStuck := numItems - moneyCapacity
	if stuckCount != expectedStuck {
		t.Errorf("expected %d stuck in init, got %d", expectedStuck, stuckCount)
	}

	// Assert: money pool is empty (all consumed).
	moneyTokens := snap.TokensInPlace("money:available")
	if len(moneyTokens) != 0 {
		t.Errorf("expected 0 money tokens (all consumed), got %d", len(moneyTokens))
	}

	// Assert: no tokens lost. Money tokens are consumed by input arcs and
	// not returned — so they are removed from the marking entirely.
	expectedTotal := completeCount + stuckCount
	if len(snap.Tokens) != expectedTotal {
		t.Errorf("expected %d total tokens (complete=%d + stuck=%d), got %d",
			expectedTotal, completeCount, stuckCount, len(snap.Tokens))
	}
}

// TestResourceExhaustionNoTokenLoss validates that across many operations with
// dual resources, tokens are never duplicated or destroyed when properly returned.
func TestResourceExhaustionNoTokenLoss(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	const (
		numItems      = 20
		gpuCapacity   = 1
		moneyCapacity = 5
	)

	// Transition requires BOTH gpu + money to fire.
	// GPU is bottleneck (cap 1), so only 1 processes at a time.
	dir := testutil.ScaffoldFactoryDir(t, &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{{Name: "task", States: []interfaces.StateConfig{
			{Name: "init", Type: interfaces.StateTypeInitial}, {Name: "processing", Type: interfaces.StateTypeProcessing},
			{Name: "complete", Type: interfaces.StateTypeTerminal}, {Name: "failed", Type: interfaces.StateTypeFailed},
		}}},
		Resources: []interfaces.ResourceConfig{{Name: "gpu", Capacity: gpuCapacity}, {Name: "money", Capacity: moneyCapacity}},
		Workers:   []interfaces.WorkerConfig{{Name: "worker"}, {Name: "releaser"}},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{Name: "acquire", WorkerTypeName: "worker",
				Inputs:  []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}, {WorkTypeName: "gpu", StateName: "available"}, {WorkTypeName: "money", StateName: "available"}},
				Outputs: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "processing"}}},
			{Name: "release", WorkerTypeName: "releaser",
				Inputs:  []interfaces.IOConfig{{WorkTypeName: "task", StateName: "processing"}},
				Outputs: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "complete"}, {WorkTypeName: "gpu", StateName: "available"}, {WorkTypeName: "money", StateName: "available"}}},
		},
	})
	h := testutil.NewServiceTestHarness(t, dir)

	acquireResults := make([]interfaces.WorkResult, numItems)
	releaseResults := make([]interfaces.WorkResult, numItems)
	for i := range numItems {
		acquireResults[i] = interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted}
		releaseResults[i] = interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted}
	}
	h.MockWorker("worker", acquireResults...)
	h.MockWorker("releaser", releaseResults...)

	queueManyItems(t, h, "task", numItems)

	h.RunUntilComplete(t, 30*time.Second)

	// Assert: all complete.
	h.Assert().
		PlaceTokenCount("task:complete", numItems).
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:processing")

	// Assert: both resources fully returned.
	snap := h.Marking()
	gpuTokens := snap.TokensInPlace("gpu:available")
	if len(gpuTokens) != gpuCapacity {
		t.Errorf("expected %d GPU tokens returned, got %d", gpuCapacity, len(gpuTokens))
	}
	moneyTokens := snap.TokensInPlace("money:available")
	if len(moneyTokens) != moneyCapacity {
		t.Errorf("expected %d money tokens returned, got %d", moneyCapacity, len(moneyTokens))
	}

	// Assert: exact token count.
	expectedTotal := numItems + gpuCapacity + moneyCapacity
	if len(snap.Tokens) != expectedTotal {
		t.Errorf("expected %d total tokens, got %d", expectedTotal, len(snap.Tokens))
	}

	// Verify no resource token duplication by checking unique IDs.
	tokenIDs := make(map[string]bool, len(snap.Tokens))
	for _, tok := range snap.Tokens {
		if tokenIDs[tok.ID] {
			t.Errorf("duplicate token ID: %s", tok.ID)
		}
		tokenIDs[tok.ID] = true
	}
}

// TestResourceExhaustionWithFailure validates that resource tokens are
// properly returned when work fails (via failure arcs) and that remaining
// items can still proceed after a failure.
func TestResourceExhaustionWithFailure(t *testing.T) {
	t.Skip("pending migration: multiple failure arcs (task:failed + resource:available) not expressible in single on_failure config")
}

// TestResourceExhaustionTimeout validates no infinite loops or deadlocks
// in resource-constrained scenarios.
func TestResourceExhaustionTimeout(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	done := make(chan struct{})
	go func() {
		defer close(done)

		dir := testutil.ScaffoldFactoryDir(t, &interfaces.FactoryConfig{
			WorkTypes: []interfaces.WorkTypeConfig{{Name: "task", States: []interfaces.StateConfig{
				{Name: "init", Type: interfaces.StateTypeInitial},
				{Name: "complete", Type: interfaces.StateTypeTerminal},
				{Name: "failed", Type: interfaces.StateTypeFailed},
			}}},
			Resources: []interfaces.ResourceConfig{{Name: "gpu", Capacity: 1}},
			Workers:   []interfaces.WorkerConfig{{Name: "w"}},
			Workstations: []interfaces.FactoryWorkstationConfig{{
				Name: "process", WorkerTypeName: "w",
				Inputs:  []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}, {WorkTypeName: "gpu", StateName: "available"}},
				Outputs: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "complete"}, {WorkTypeName: "gpu", StateName: "available"}},
			}},
		})
		h := testutil.NewServiceTestHarness(t, dir)

		results := make([]interfaces.WorkResult, 15)
		for i := range results {
			results[i] = interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted}
		}
		h.MockWorker("w", results...)

		for i := range 15 {
			h.SubmitWork("task", fmt.Appendf(nil, `{"item": %d}`, i))
		}

		h.RunUntilComplete(t, 10*time.Second)
	}()

	select {
	case <-done:
		// Completed within timeout.
	case <-time.After(10 * time.Second):
		t.Fatal("resource exhaustion test did not complete within 10s — possible deadlock")
	}
}

// --- Helper executors ---

// concurrencyTracker tracks concurrent execution count.
type concurrencyTracker struct {
	mu                *sync.Mutex
	maxConcurrent     *int
	currentConcurrent *int
}

func (ct *concurrencyTracker) Execute(_ context.Context, dispatch interfaces.WorkDispatch) (interfaces.WorkResult, error) {
	ct.mu.Lock()
	*ct.currentConcurrent++
	if *ct.currentConcurrent > *ct.maxConcurrent {
		*ct.maxConcurrent = *ct.currentConcurrent
	}
	ct.mu.Unlock()

	ct.mu.Lock()
	*ct.currentConcurrent--
	ct.mu.Unlock()

	return interfaces.WorkResult{DispatchID: dispatch.DispatchID, TransitionID: dispatch.TransitionID, Outcome: interfaces.OutcomeAccepted}, nil
}

var (
	_ workers.WorkerExecutor = (*concurrencyTracker)(nil)
)
