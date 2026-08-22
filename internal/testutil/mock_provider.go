package testutil

import (
	"context"
	"sync"

	"github.com/portpowered/infinite-you/pkg/services/providers"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

// MockProvider implements the Providers root contract for testing. It returns
// predetermined InferenceResponses in sequence. When the sequence is
// exhausted, it returns a default response.
type MockProvider struct {
	NativeProvider
	responses     []providers.ExecuteResult
	errors        []error
	calls         []providers.ExecuteRequest
	legacyCalls   []workerexecution.ProviderInferenceRequest
	mu            sync.Mutex
	index         int
	defaultResult providers.ExecuteResult
}

// NewMockProvider creates a MockProvider that returns the given responses in order.
// Each response can optionally have a paired error at the same index in the errors
// slice. When the sequence is exhausted, returns a default InferenceResponse with
// StopTokenFound=true (so MODEL_WORKER with stop tokens will ACCEPT by default).
func NewMockProvider(responses ...workerexecution.InferenceResponse) *MockProvider {
	nativeResponses := make([]providers.ExecuteResult, len(responses))
	for index, response := range responses {
		nativeResponses[index] = nativeExecuteResult(response)
	}
	provider := &MockProvider{
		responses:     nativeResponses,
		defaultResult: nativeExecuteResult(workerexecution.InferenceResponse{Content: "default mock response"}),
	}
	return provider
}

// NewMockProviderWithErrors creates a MockProvider with paired responses and errors.
// The responses and errors slices must be the same length; a nil error means success.
func NewMockProviderWithErrors(responses []workerexecution.InferenceResponse, errors []error) *MockProvider {
	nativeResponses := make([]providers.ExecuteResult, len(responses))
	for index, response := range responses {
		nativeResponses[index] = nativeExecuteResult(response)
	}
	provider := &MockProvider{
		responses:     nativeResponses,
		errors:        errors,
		defaultResult: nativeExecuteResult(workerexecution.InferenceResponse{Content: "default mock response"}),
	}
	return provider
}

// Execute records the native request and returns the next predetermined result.
func (m *MockProvider) Execute(_ context.Context, request providers.ExecuteRequest) (providers.ExecuteResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.calls = append(m.calls, request.Clone())
	dispatchID := request.Correlation.DispatchID
	if dispatchID == "" {
		dispatchID = request.AttemptID
	}
	m.legacyCalls = append(m.legacyCalls, providerInferenceRequest(request, nil, dispatchID))

	if m.index < len(m.responses) {
		result := m.responses[m.index].Clone()
		var err error
		if m.index < len(m.errors) {
			err = m.errors[m.index]
		}
		m.index++
		return authoritativeNativeResult(result), err
	}

	return authoritativeNativeResult(m.defaultResult.Clone()), nil
}

func nativeExecuteResult(response workerexecution.InferenceResponse) providers.ExecuteResult {
	result := providers.ExecuteResult{
		Content: response.Content,
		Outcome: providers.ExecuteOutcome(response.Outcome),
	}
	continuation := response.Continuation
	if continuation == nil {
		continuation = response.ProviderSession.ContinuationRef()
	}
	if continuation != nil {
		if reference, err := continuation.ToSessionRef(); err == nil {
			result.SessionRef = &reference
		}
	}
	if response.Diagnostics == nil {
		return result
	}
	metadata := cloneNativeMetadata(response.Diagnostics.Metadata)
	if response.Diagnostics.Provider != nil {
		metadata = mergeNativeMetadata(metadata, response.Diagnostics.Provider.ResponseMetadata)
	}
	result.Diagnostics = &providers.ExecuteDiagnostics{Metadata: metadata}
	if response.Diagnostics.Command != nil {
		result.Diagnostics.Command = &providers.ExecuteCommandDiagnostics{
			Command:    response.Diagnostics.Command.Command,
			Args:       append([]string(nil), response.Diagnostics.Command.Args...),
			Env:        cloneNativeMetadata(response.Diagnostics.Command.Env),
			Stdin:      response.Diagnostics.Command.Stdin,
			Stdout:     response.Diagnostics.Command.Stdout,
			Stderr:     response.Diagnostics.Command.Stderr,
			ExitCode:   response.Diagnostics.Command.ExitCode,
			TimedOut:   response.Diagnostics.Command.TimedOut,
			DurationMS: response.Diagnostics.Command.Duration.Milliseconds(),
			WorkingDir: response.Diagnostics.Command.WorkingDir,
		}
	}
	if response.Diagnostics.Panic != nil {
		result.Diagnostics.Panic = &providers.ExecutePanicDiagnostics{
			Message: response.Diagnostics.Panic.Message,
			Stack:   response.Diagnostics.Panic.Stack,
		}
	}
	return result
}

func cloneNativeMetadata(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func mergeNativeMetadata(base, overlay map[string]string) map[string]string {
	if len(overlay) == 0 {
		return base
	}
	if base == nil {
		base = make(map[string]string, len(overlay))
	}
	for key, value := range overlay {
		base[key] = value
	}
	return base
}

func authoritativeTestResponse(response workerexecution.InferenceResponse) workerexecution.InferenceResponse {
	if response.Content == "" || response.Diagnostics != nil &&
		(response.Diagnostics.Metadata[workerexecution.ProviderResponseMetadataCompletionEvidence] != "" ||
			response.Diagnostics.Provider != nil && response.Diagnostics.Provider.ResponseMetadata[workerexecution.ProviderResponseMetadataCompletionEvidence] != "") {
		return response
	}
	response.Diagnostics = workerexecution.CloneWorkDiagnostics(response.Diagnostics)
	if response.Diagnostics == nil {
		response.Diagnostics = &workerexecution.WorkDiagnostics{}
	}
	if response.Diagnostics.Metadata == nil {
		response.Diagnostics.Metadata = make(map[string]string, 1)
	}
	response.Diagnostics.Metadata[workerexecution.ProviderResponseMetadataCompletionEvidence] = "provider_response"
	return response
}

// Calls returns all InferenceRequests received by this provider, in order.
func (m *MockProvider) Calls() []workerexecution.ProviderInferenceRequest {
	m.mu.Lock()
	defer m.mu.Unlock()

	out := make([]workerexecution.ProviderInferenceRequest, len(m.legacyCalls))
	copy(out, m.legacyCalls)
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

	if len(m.legacyCalls) == 0 {
		panic("MockProvider: LastCall() called with no inferences")
	}
	return m.legacyCalls[len(m.legacyCalls)-1]
}

// Compile-time check.
var _ providers.Service = (*MockProvider)(nil)
