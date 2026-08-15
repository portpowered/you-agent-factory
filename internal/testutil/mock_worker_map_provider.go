package testutil

import (
	"context"
	"errors"
	"sync"

	"github.com/portpowered/infinite-you/pkg/services/providers"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

// MockWorkerMapProvider implements the Providers root contract for testing.
// It returns
// predetermined InferenceResponses in sequence. When the sequence is
// exhausted, it returns a default response.
type MockWorkerMapProvider struct {
	NativeProvider
	workerCalls       map[string][]providers.ExecuteRequest
	legacyWorkerCalls map[string][]workerexecution.ProviderInferenceRequest
	mu                sync.Mutex
	workerIndex       map[string]int            // tracks call count per worker type for response sequencing
	workerResponses   map[string][]WorkResponse // optional: different response sequences per worker type
	defaultResult     providers.ExecuteResult
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
func NewMockWorkerMapProvider(responses map[string][]workerexecution.InferenceResponse) *MockWorkerMapProvider {
	return NewMockWorkerMapProviderWithDefault(mapResponses(responses))
}

func mapResponses(input map[string][]workerexecution.InferenceResponse) map[string][]WorkResponse {
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
	provider := &MockWorkerMapProvider{
		workerResponses:   responses,
		defaultResult:     nativeExecuteResult(workerexecution.InferenceResponse{Content: "default mock response"}),
		workerIndex:       make(map[string]int),
		workerCalls:       make(map[string][]providers.ExecuteRequest),
		legacyWorkerCalls: make(map[string][]workerexecution.ProviderInferenceRequest),
	}
	return provider
}

// Execute records the native request and returns the next predetermined response.
func (m *MockWorkerMapProvider) Execute(_ context.Context, request providers.ExecuteRequest) (providers.ExecuteResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	workerType := request.WorkerType
	if m.workerResponses[workerType] != nil {
		m.workerCalls[workerType] = append(m.workerCalls[workerType], request.Clone())
		dispatchID := request.Correlation.DispatchID
		if dispatchID == "" {
			dispatchID = request.AttemptID
		}
		m.legacyWorkerCalls[workerType] = append(
			m.legacyWorkerCalls[workerType],
			providerInferenceRequest(request, nil, dispatchID),
		)

		index := m.workerIndex[workerType]
		if index < len(m.workerResponses[workerType]) {
			resp := m.workerResponses[workerType][index]
			m.workerIndex[workerType]++
			if resp.Error != nil {
				return providers.ExecuteResult{}, resp.Error
			} else {
				return providers.ExecuteResult{
					Content: resp.Content,
					Diagnostics: &providers.ExecuteDiagnostics{Metadata: map[string]string{
						"completion_evidence": "provider_response",
					}},
				}, nil
			}
		}
	} else {
		return providers.ExecuteResult{}, errors.New("failed")
	}
	return authoritativeNativeResult(m.defaultResult.Clone()), nil
}

// Calls returns all InferenceRequests received by this provider, in order.
func (m *MockWorkerMapProvider) Calls(workerType string) []workerexecution.ProviderInferenceRequest {
	m.mu.Lock()
	defer m.mu.Unlock()

	out := make([]workerexecution.ProviderInferenceRequest, len(m.legacyWorkerCalls[workerType]))
	copy(out, m.legacyWorkerCalls[workerType])
	return out
}

// CallCount returns how many times Infer was called.
func (m *MockWorkerMapProvider) CallCount(workerType string) int {
	m.mu.Lock()
	defer m.mu.Unlock()

	return len(m.legacyWorkerCalls[workerType])
}

// LastCall returns the most recent InferenceRequest, or panics if none.
func (m *MockWorkerMapProvider) LastCall(workerType string) workerexecution.ProviderInferenceRequest {
	m.mu.Lock()
	defer m.mu.Unlock()

	calls := m.legacyWorkerCalls[workerType]
	if len(calls) == 0 {
		panic("MockWorkerMapProvider: LastCall() called with no inferences")
	}
	return calls[len(calls)-1]
}

// Compile-time check.
var _ providers.Service = (*MockWorkerMapProvider)(nil)
