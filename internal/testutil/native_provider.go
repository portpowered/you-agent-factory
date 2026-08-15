package testutil

import (
	"context"
	"strings"
	"sync"

	"github.com/portpowered/infinite-you/pkg/services/providers"
)

// NativeProvider is an execute-shaped Providers root double. Its execution
// callback receives the public Providers request directly; it does not adapt
// through the legacy Workers inference vocabulary.
type NativeProvider struct {
	ExecuteFunc  func(context.Context, providers.ExecuteRequest) (providers.ExecuteResult, error)
	ContinueFunc func(context.Context, providers.ContinueRequest) (providers.ContinueResult, error)
}

func (provider NativeProvider) ListProviders(
	context.Context,
	providers.ListProvidersRequest,
) (providers.ListProvidersResult, error) {
	return providers.ListProvidersResult{Providers: []providers.Descriptor{{ID: providers.IDCodex}}}, nil
}

func (provider NativeProvider) GetProvider(
	_ context.Context,
	request providers.GetProviderRequest,
) (providers.GetProviderResult, error) {
	if err := request.Validate(); err != nil {
		return providers.GetProviderResult{}, err
	}
	return providers.GetProviderResult{Provider: providers.Descriptor{ID: request.ID}}, nil
}

func (provider NativeProvider) ResolveIdentity(
	_ context.Context,
	request providers.ResolveIdentityRequest,
) (providers.ResolveIdentityResult, error) {
	if err := request.Validate(); err != nil {
		return providers.ResolveIdentityResult{}, err
	}
	return providers.ResolveIdentityResult{ID: providers.ID(strings.TrimSpace(request.Identity))}, nil
}

func (provider NativeProvider) ResolveSelection(
	ctx context.Context,
	request providers.ResolveSelectionRequest,
) (providers.ResolveSelectionResult, error) {
	identity := request.Workstation
	if identity == "" {
		identity = request.Factory
	}
	if identity == "" {
		identity = request.ModelProvider
	}
	resolved, err := provider.ResolveIdentity(ctx, providers.ResolveIdentityRequest{Identity: identity})
	if err != nil {
		return providers.ResolveSelectionResult{}, err
	}
	return providers.ResolveSelectionResult{Provider: resolved.ID}, nil
}

func (provider NativeProvider) ValidatePrerequisites(
	_ context.Context,
	request providers.ValidatePrerequisitesRequest,
) error {
	return request.Validate()
}

func (provider NativeProvider) Execute(
	ctx context.Context,
	request providers.ExecuteRequest,
) (providers.ExecuteResult, error) {
	if provider.ExecuteFunc == nil {
		return providers.ExecuteResult{}, providers.ErrExecuteFailed
	}
	return provider.ExecuteFunc(ctx, request)
}

func (provider NativeProvider) ControlAttempt(
	_ context.Context,
	request providers.ControlAttemptRequest,
) (providers.ControlAttemptResult, error) {
	if err := request.Validate(); err != nil {
		return providers.ControlAttemptResult{}, err
	}
	return providers.ControlAttemptResult{
		Provider:  request.Provider,
		AttemptID: request.AttemptID,
		Action:    request.Action,
		Outcome:   providers.ControlOutcomeUnsupported,
	}, nil
}

func (provider NativeProvider) Continue(
	ctx context.Context,
	request providers.ContinueRequest,
) (providers.ContinueResult, error) {
	if err := request.Validate(); err != nil {
		return providers.ContinueResult{}, err
	}
	if provider.ContinueFunc != nil {
		return provider.ContinueFunc(ctx, request)
	}
	return providers.ContinueResult{
		Reference: request.Reference,
		Outcome:   providers.ContinuationOutcomeUnsupported,
	}, nil
}

func (provider NativeProvider) ContinueReference(
	ctx context.Context,
	request providers.ContinueReferenceRequest,
) (providers.ContinueReferenceResult, error) {
	if err := request.Reference.Validate(); err != nil {
		return providers.ContinueReferenceResult{}, err
	}
	if err := request.Attempt.Validate(); err != nil {
		return providers.ContinueReferenceResult{}, err
	}
	reference, err := request.Reference.ToSessionRef()
	if err != nil {
		return providers.ContinueReferenceResult{}, err
	}
	continued, err := provider.Continue(ctx, providers.ContinueRequest{
		Reference: reference,
		Attempt:   request.Attempt,
	})
	if err != nil {
		return providers.ContinueReferenceResult{}, err
	}
	continuedReference := continued.Reference
	if strings.TrimSpace(continuedReference.Provider.String()) == "" {
		continuedReference = reference
	}
	resultReference := continuedReference.ContinuationRef()
	resultReference.ExternalRef = request.Reference.Normalize().ExternalRef
	return providers.ContinueReferenceResult{
		Reference: resultReference,
		Outcome:   continued.Outcome,
		Result:    continued.Result,
	}, nil
}

// NativeMockProvider is a native execute-shaped sequence double for
// functional scenarios that need deterministic responses and request
// observations.
type NativeMockProvider struct {
	NativeProvider
	responses []providers.ExecuteResult
	errors    []error
	calls     []providers.ExecuteRequest
	mu        sync.Mutex
	index     int
}

func NewNativeMockProvider(responses ...providers.ExecuteResult) *NativeMockProvider {
	return NewNativeMockProviderWithErrors(responses, nil)
}

func NewNativeMockProviderWithErrors(
	responses []providers.ExecuteResult,
	errors []error,
) *NativeMockProvider {
	provider := &NativeMockProvider{
		responses: responses,
		errors:    errors,
	}
	provider.NativeProvider.ExecuteFunc = provider.Execute
	return provider
}

func (provider *NativeMockProvider) Execute(
	_ context.Context,
	request providers.ExecuteRequest,
) (providers.ExecuteResult, error) {
	provider.mu.Lock()
	defer provider.mu.Unlock()

	provider.calls = append(provider.calls, request.Clone())
	result := providers.ExecuteResult{Content: "default mock response"}
	if provider.index < len(provider.responses) {
		result = provider.responses[provider.index]
	}
	var err error
	if provider.index < len(provider.errors) {
		err = provider.errors[provider.index]
	}
	provider.index++
	return authoritativeNativeResult(result), err
}

func (provider *NativeMockProvider) Calls() []providers.ExecuteRequest {
	provider.mu.Lock()
	defer provider.mu.Unlock()

	calls := make([]providers.ExecuteRequest, len(provider.calls))
	for index, request := range provider.calls {
		calls[index] = request.Clone()
	}
	return calls
}

func (provider *NativeMockProvider) CallsForWorker(workerType string) []providers.ExecuteRequest {
	provider.mu.Lock()
	defer provider.mu.Unlock()

	var calls []providers.ExecuteRequest
	for _, request := range provider.calls {
		if request.WorkerType == workerType {
			calls = append(calls, request.Clone())
		}
	}
	return calls
}

func (provider *NativeMockProvider) CallCount() int {
	provider.mu.Lock()
	defer provider.mu.Unlock()
	return len(provider.calls)
}

func authoritativeNativeResult(result providers.ExecuteResult) providers.ExecuteResult {
	if result.Content == "" || result.Diagnostics != nil && result.Diagnostics.Metadata["completion_evidence"] != "" {
		return result
	}
	result = result.Clone()
	if result.Diagnostics == nil {
		result.Diagnostics = &providers.ExecuteDiagnostics{}
	}
	if result.Diagnostics.Metadata == nil {
		result.Diagnostics.Metadata = make(map[string]string, 1)
	}
	result.Diagnostics.Metadata["completion_evidence"] = "provider_response"
	return result
}

var _ providers.Service = (*NativeProvider)(nil)
var _ providers.Service = (*NativeMockProvider)(nil)
