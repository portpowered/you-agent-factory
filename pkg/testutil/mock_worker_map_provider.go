package testutil

import (
	"context"
	"errors"
	"sync"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/workers"
)

// MockProvider implements workers.Provider for testing. It returns
// predetermined InferenceResponses in sequence. When the sequence is
// exhausted, it returns a default response.
type MockWorkerMapProvider struct {
	workerCalls     map[string][]interfaces.ProviderInferenceRequest
	mu              sync.Mutex
	workerIndex     map[string]int            // tracks call count per worker type for response sequencing
	workerResponses map[string][]WorkResponse // optional: different response sequences per worker type
	defaultR        interfaces.InferenceResponse
}

// response from a provider can either be content or an error.
type WorkResponse struct {
	Content string
	Error   error
}

// MockWorkerMapProviderOption configures a MockWorkerMapProvider.
type MockWorkerMapProviderOption func(*MockWorkerMapProvider)

// NewMockProvider creates a MockProvider that returns the given responses in order.
// Each response can optionally have a paired error at the same index in the errors
// slice. When the sequence is exhausted, returns a default InferenceResponse with
// StopTokenFound=true (so MODEL_WORKER with stop tokens will ACCEPT by default).
func NewMockWorkerMapProvider(responses map[string][]interfaces.InferenceResponse) *MockWorkerMapProvider {
	return NewMockWorkerMapProviderWithDefault(mapResponses(responses))
}

func mapResponses(input map[string][]interfaces.InferenceResponse) map[string][]WorkResponse {
	mapped := make(map[string][]WorkResponse)
	for workerType, resps := range input {
		mapped[workerType] = make([]WorkResponse, len(resps))
		for i, r := range resps {
			mapped[workerType][i] = WorkResponse{Content: r.Content, Error: nil}
		}
	}
	return mapped
}

func NewMockWorkerMapProviderWithDefault(responses map[string][]WorkResponse) *MockWorkerMapProvider {
	return &MockWorkerMapProvider{
		workerResponses: responses,
		defaultR: interfaces.InferenceResponse{
			Content: "default mock response",
		},
		workerIndex: make(map[string]int),
		workerCalls: make(map[string][]interfaces.ProviderInferenceRequest),
	}
}

// Infer records the request and returns the next predetermined response.
func (m *MockWorkerMapProvider) Infer(_ context.Context, req interfaces.ProviderInferenceRequest) (interfaces.InferenceResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	workerType := req.WorkerType
	if workerType == "" {
		workerType = req.Dispatch.WorkerType
	}
	if m.workerResponses[workerType] != nil {
		m.workerCalls[workerType] = append(m.workerCalls[workerType], req)

		index := m.workerIndex[workerType]
		if index < len(m.workerResponses[workerType]) {
			resp := m.workerResponses[workerType][index]
			m.workerIndex[workerType]++
			if resp.Error != nil {
				return interfaces.InferenceResponse{}, resp.Error
			} else {
				return interfaces.InferenceResponse{
					Content: resp.Content,
				}, nil
			}
		}
	} else {
		return interfaces.InferenceResponse{}, errors.New("failed")
	}
	return m.defaultR, nil
}

// Calls returns all InferenceRequests received by this provider, in order.
func (m *MockWorkerMapProvider) Calls(workerType string) []interfaces.ProviderInferenceRequest {
	m.mu.Lock()
	defer m.mu.Unlock()

	out := make([]interfaces.ProviderInferenceRequest, len(m.workerCalls[workerType]))
	copy(out, m.workerCalls[workerType])
	return out
}

// CallCount returns how many times Infer was called.
func (m *MockWorkerMapProvider) CallCount(workerType string) int {
	m.mu.Lock()
	defer m.mu.Unlock()

	return len(m.workerCalls[workerType])
}

// LastCall returns the most recent InferenceRequest, or panics if none.
func (m *MockWorkerMapProvider) LastCall(workerType string) interfaces.ProviderInferenceRequest {
	m.mu.Lock()
	defer m.mu.Unlock()

	calls := m.workerCalls[workerType]
	if len(calls) == 0 {
		panic("MockWorkerMapProvider: LastCall() called with no inferences")
	}
	return calls[len(calls)-1]
}

// Compile-time check.
var _ workers.Provider = (*MockWorkerMapProvider)(nil)
