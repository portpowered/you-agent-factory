package functional_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil"
)

// barrierMockExecutor is a WorkerExecutor that blocks each Execute call until
// all N expected dispatches have been received. This prevents the factory from
// completing early when work items are submitted sequentially via HTTP and the
// mock would otherwise complete before subsequent items are dispatched.
// This remains a custom executor because MockProvider cannot replicate the
// channel-based barrier synchronization needed for concurrency testing.
type barrierMockExecutor struct {
	mu      sync.Mutex
	calls   []interfaces.WorkDispatch
	n       int           // number of expected calls before unblocking
	barrier chan struct{} // closed when n calls have been received
	once    sync.Once
}

func newBarrierMock(n int) *barrierMockExecutor {
	return &barrierMockExecutor{
		n:       n,
		barrier: make(chan struct{}),
	}
}

func (m *barrierMockExecutor) Execute(ctx context.Context, d interfaces.WorkDispatch) (interfaces.WorkResult, error) {
	m.mu.Lock()
	m.calls = append(m.calls, d)
	if len(m.calls) >= m.n {
		m.once.Do(func() { close(m.barrier) })
	}
	m.mu.Unlock()

	// Block until all N dispatches have been received, then return results together.
	// This ensures all tokens are in-flight before any result is processed.
	select {
	case <-m.barrier:
	case <-ctx.Done():
		return interfaces.WorkResult{}, ctx.Err()
	}

	return interfaces.WorkResult{
		DispatchID:   d.DispatchID,
		TransitionID: d.TransitionID,
		Outcome:      interfaces.OutcomeAccepted,
	}, nil
}

func (m *barrierMockExecutor) callCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.calls)
}

// TestE2E_FactoryDispatchesAndCompletes validates the full end-to-end dispatch
// pipeline using MockProvider to exercise prompt rendering and stop-token
// evaluation through the real service layer.
func TestE2E_FactoryDispatchesAndCompletes(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "e2e"))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "E2E test"}`))

	// MockProvider returns content containing the stop token "COMPLETE"
	// so the worker ACCEPTS via the real AgentExecutor pipeline.
	provider := testutil.NewMockProvider(
		interfaces.InferenceResponse{Content: "E2E done. COMPLETE"},
	)
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)
	h.RunUntilComplete(t, 10*time.Second)

	// Assert factory completed with exactly one terminal token.
	h.Assert().
		PlaceTokenCount("task:complete", 1).
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:failed")

	// Assert provider was called exactly once.
	if provider.CallCount() != 1 {
		t.Errorf("expected provider called 1 time, got %d", provider.CallCount())
	}

	// Assert inference request had the correct model from AGENTS.md.
	call := provider.LastCall()
	if call.Model != "test-model" {
		t.Errorf("expected model test-model, got %q", call.Model)
	}
}

// TestE2E_FactoryDispatchesMultipleWork validates that multiple work items
// are all dispatched and completed independently.
//
// Uses a barrier mock that blocks each Execute call until all 3 dispatches have
// been received. This ensures items submitted sequentially via HTTP are all
// in-flight before any result is returned, preventing premature factory completion.
func TestE2E_FactoryDispatchesMultipleWork(t *testing.T) {
	mock := newBarrierMock(3)
	dir := scaffoldFactory(t, simplePipelineConfig())
	for i := 0; i < 3; i++ {
		testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
			WorkTypeID: "task",
			TraceID:    fmt.Sprintf("trace-e2e-batch-%d", i),
			Payload:    []byte(`{"title":"batch item"}`),
		})
	}

	fs := StartFunctionalServer(t, dir, false,
		factory.WithWorkerExecutor("worker-a", mock),
	)

	// Wait for all to complete. The barrier mock ensures all 3 items are dispatched
	// before any result flows through; pendingDispatches tracking prevents the
	// termination check from firing while dispatches are outstanding.
	state := fs.WaitForCompleted(t, 15*time.Second)
	if state.TotalTokens != 3 {
		t.Errorf("expected 3 tokens, got %d", state.TotalTokens)
	}
	if state.Categories.Terminal != 3 {
		t.Errorf("expected 3 terminal tokens, got %d", state.Categories.Terminal)
	}
	if mock.callCount() != 3 {
		t.Errorf("expected mock executor called 3 times, got %d", mock.callCount())
	}
}
