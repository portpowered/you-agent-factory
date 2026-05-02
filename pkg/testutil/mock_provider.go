package testutil

import (
	"context"
	"sync"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/workers"
)

// MockProvider implements workers.Provider for testing. It returns
// predetermined InferenceResponses in sequence. When the sequence is
// exhausted, it returns a default response.
type MockProvider struct {
	responses []interfaces.InferenceResponse
	errors    []error
	calls     []interfaces.ProviderInferenceRequest
	mu        sync.Mutex
	index     int
	defaultR  interfaces.InferenceResponse
}

// NewMockProvider creates a MockProvider that returns the given responses in order.
// Each response can optionally have a paired error at the same index in the errors
// slice. When the sequence is exhausted, returns a default InferenceResponse with
// StopTokenFound=true (so MODEL_WORKER with stop tokens will ACCEPT by default).
func NewMockProvider(responses ...interfaces.InferenceResponse) *MockProvider {
	return &MockProvider{
		responses: responses,
		defaultR: interfaces.InferenceResponse{
			Content: "default mock response",
		},
	}
}

// NewMockProviderWithErrors creates a MockProvider with paired responses and errors.
// The responses and errors slices must be the same length; a nil error means success.
func NewMockProviderWithErrors(responses []interfaces.InferenceResponse, errors []error) *MockProvider {
	return &MockProvider{
		responses: responses,
		errors:    errors,
		defaultR: interfaces.InferenceResponse{
			Content: "default mock response",
		},
	}
}

// Infer records the request and returns the next predetermined response.
func (m *MockProvider) Infer(_ context.Context, req interfaces.ProviderInferenceRequest) (interfaces.InferenceResponse, error) {
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

// Calls returns all InferenceRequests received by this provider, in order.
func (m *MockProvider) Calls() []interfaces.ProviderInferenceRequest {
	m.mu.Lock()
	defer m.mu.Unlock()

	out := make([]interfaces.ProviderInferenceRequest, len(m.calls))
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
func (m *MockProvider) LastCall() interfaces.ProviderInferenceRequest {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.calls) == 0 {
		panic("MockProvider: LastCall() called with no inferences")
	}
	return m.calls[len(m.calls)-1]
}

// Compile-time check.
var _ workers.Provider = (*MockProvider)(nil)
