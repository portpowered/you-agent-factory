package testutil

import (
	"context"
	"sync"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

// MockExecutor returns predetermined WorkResults in sequence.
// When the sequence is exhausted, returns a default result.
type MockExecutor struct {
	results  []interfaces.WorkResult
	calls    []interfaces.WorkDispatch
	mu       sync.Mutex
	index    int
	defaultR interfaces.WorkResult
}

// NewMockExecutor creates a MockExecutor that returns the given results in order.
// When the sequence is exhausted, it returns a default WorkResult with OutcomeAccepted.
func NewMockExecutor(results ...interfaces.WorkResult) *MockExecutor {
	return &MockExecutor{
		results: results,
		defaultR: interfaces.WorkResult{
			Outcome: interfaces.OutcomeAccepted,
		},
	}
}

// Execute records the dispatch and returns the next predetermined result.
func (m *MockExecutor) Execute(_ context.Context, dispatch interfaces.WorkDispatch) (interfaces.WorkResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.calls = append(m.calls, dispatch)

	var result interfaces.WorkResult
	if m.index < len(m.results) {
		result = m.results[m.index]
		m.index++
	} else {
		result = m.defaultR
	}

	// Ensure TransitionID matches the dispatch.
	result.DispatchID = dispatch.DispatchID
	result.TransitionID = dispatch.TransitionID

	return result, nil
}

// Calls returns all WorkDispatches received by this executor, in order.
func (m *MockExecutor) Calls() []interfaces.WorkDispatch {
	m.mu.Lock()
	defer m.mu.Unlock()

	out := make([]interfaces.WorkDispatch, len(m.calls))
	copy(out, m.calls)
	return out
}

// CallCount returns how many times Execute was called.
func (m *MockExecutor) CallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()

	return len(m.calls)
}

// LastCall returns the most recent WorkDispatch, or panics if none.
func (m *MockExecutor) LastCall() interfaces.WorkDispatch {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.calls) == 0 {
		panic("MockExecutor: LastCall() called with no dispatches")
	}
	return m.calls[len(m.calls)-1]
}
