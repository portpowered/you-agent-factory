package testutil

import (
	"context"
	"sync"

	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

// MockProvider implements provider.Provider for testing. It returns
// predetermined InferenceResponses in sequence. When the sequence is
// exhausted, it returns a default response.
type MockProvider struct {
	responses []workerexecution.InferenceResponse
	errors    []error
	calls     []workerexecution.ProviderInferenceRequest
	mu        sync.Mutex
	index     int
	defaultR  workerexecution.InferenceResponse
}

// NewMockProvider creates a MockProvider that returns the given responses in order.
// Each response can optionally have a paired error at the same index in the errors
// slice. When the sequence is exhausted, returns a default InferenceResponse with
// StopTokenFound=true (so MODEL_WORKER with stop tokens will ACCEPT by default).
func NewMockProvider(responses ...workerexecution.InferenceResponse) *MockProvider {
	return &MockProvider{
		responses: responses,
		defaultR: workerexecution.InferenceResponse{
			Content: "default mock response",
		},
	}
}

// NewMockProviderWithErrors creates a MockProvider with paired responses and errors.
// The responses and errors slices must be the same length; a nil error means success.
func NewMockProviderWithErrors(responses []workerexecution.InferenceResponse, errors []error) *MockProvider {
	return &MockProvider{
		responses: responses,
		errors:    errors,
		defaultR: workerexecution.InferenceResponse{
			Content: "default mock response",
		},
	}
}

// Execute records the request and returns the next predetermined response.
func (m *MockProvider) Execute(_ context.Context, req workerexecution.RunnerExecutionRequest) (workerexecution.RunnerExecutionResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.calls = append(m.calls, req)

	if m.index < len(m.responses) {
		resp := m.responses[m.index]
		var err error
		if m.index < len(m.errors) {
			err = m.errors[m.index]
		}
		m.index++
		return resp, err
	}

	return m.defaultR, nil
}

// Infer preserves the test helper's legacy convenience spelling. Production
// Workers execution uses Runner.Execute.
func (m *MockProvider) Infer(ctx context.Context, req workerexecution.ProviderInferenceRequest) (workerexecution.InferenceResponse, error) {
	return m.Execute(ctx, req)
}

// Calls returns all InferenceRequests received by this provider, in order.
func (m *MockProvider) Calls() []workerexecution.ProviderInferenceRequest {
	m.mu.Lock()
	defer m.mu.Unlock()

	out := make([]workerexecution.ProviderInferenceRequest, len(m.calls))
	copy(out, m.calls)
	return out
}

// CallCount returns how many times Infer was called.
func (m *MockProvider) CallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()

	return len(m.calls)
}

// LastCall returns the most recent InferenceRequest, or panics if none.
func (m *MockProvider) LastCall() workerexecution.ProviderInferenceRequest {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.calls) == 0 {
		panic("MockProvider: LastCall() called with no inferences")
	}
	return m.calls[len(m.calls)-1]
}

// Compile-time check.
var _ workerexecution.Runner = (*MockProvider)(nil)
