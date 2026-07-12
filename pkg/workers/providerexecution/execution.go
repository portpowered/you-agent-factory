package providerexecution

import (
	"context"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	workerprovider "github.com/portpowered/infinite-you/pkg/workers/provider"
)

// ExecutionInput is one canonical, single-attempt provider invocation.
// Retry policy remains with the caller so every call to Execute maps to exactly
// one provider call.
type ExecutionInput struct {
	Request interfaces.ProviderInferenceRequest
	Attempt int
}

// ExecutionResult is the canonical provider-boundary result shared by worker
// execution callers. FailureDetail and Diagnostics are safe to persist or
// expose through Factory Session projections.
type ExecutionResult struct {
	Response        interfaces.InferenceResponse
	Attempt         int
	ProviderSession *interfaces.ProviderSessionMetadata
	FailureMetadata *interfaces.WorkFailureMetadata
	FailureDecision *interfaces.WorkFailureDecision
	FailureDetail   *interfaces.FailureDetail
	Diagnostics     *interfaces.SafeWorkDiagnostics
}

// Executor performs one provider execution attempt.
type Executor interface {
	Execute(context.Context, ExecutionInput) (ExecutionResult, error)
}

// ProviderExecutor adapts a Provider onto the shared execution contract.
type ProviderExecutor struct {
	provider workerprovider.Provider
}

// NewProviderExecutor constructs the worker-owned provider execution boundary.
func NewProviderExecutor(provider workerprovider.Provider) *ProviderExecutor {
	return &ProviderExecutor{provider: provider}
}

// NewExecutor constructs the shared boundary when a provider is available.
// A nil provider remains an absent dependency so composition can reject live
// execution before starting a Factory Session.
func NewExecutor(provider workerprovider.Provider) Executor {
	if provider == nil {
		return nil
	}
	return NewProviderExecutor(provider)
}

// Execute invokes the provider exactly once and canonicalizes its result.
func (e *ProviderExecutor) Execute(ctx context.Context, input ExecutionInput) (ExecutionResult, error) {
	attempt := input.Attempt
	if attempt < 1 {
		attempt = 1
	}
	if err := ctx.Err(); err != nil {
		return failedExecutionResult(attempt, err), err
	}
	if e == nil || e.provider == nil {
		err := workerprovider.NewProviderError(interfaces.WorkFailureTypeMisconfigured, "provider execution requires a provider", nil)
		return failedExecutionResult(attempt, err), err
	}

	response, err := e.provider.Infer(ctx, input.Request)
	if err != nil {
		return failedExecutionResult(attempt, err), err
	}
	response.ProviderSession = canonicalProviderSession(response.ProviderSession)
	diagnostics := interfaces.SafeWorkDiagnosticsFromWorkDiagnostics(response.Diagnostics)
	return ExecutionResult{
		Response:        response,
		Attempt:         attempt,
		ProviderSession: interfaces.CloneProviderSessionMetadata(response.ProviderSession),
		Diagnostics:     diagnostics,
	}, nil
}

func failedExecutionResult(attempt int, err error) ExecutionResult {
	providerErr := workerprovider.NormalizeProviderExecutionError(err)
	if providerErr == nil {
		providerErr = workerprovider.NewProviderError(interfaces.WorkFailureTypeUnknown, "Provider execution failed.", err)
	}
	providerErr.ProviderSession = canonicalProviderSession(providerErr.ProviderSession)
	failureDetail := workerprovider.SafeProviderFailureDetail(providerErr)
	return ExecutionResult{
		Attempt:         attempt,
		ProviderSession: interfaces.CloneProviderSessionMetadata(providerErr.ProviderSession),
		FailureMetadata: workerprovider.WorkFailureMetadataFromError(providerErr),
		FailureDecision: func() *interfaces.WorkFailureDecision {
			decision := workerprovider.WorkFailureDecisionFromProviderError(providerErr)
			return &decision
		}(),
		FailureDetail: failureDetail,
		Diagnostics:   interfaces.SafeWorkDiagnosticsFromWorkDiagnostics(providerErr.Diagnostics),
	}
}

func canonicalProviderSession(session *interfaces.ProviderSessionMetadata) *interfaces.ProviderSessionMetadata {
	clone := interfaces.CloneProviderSessionMetadata(session)
	if clone != nil {
		clone.Provider = interfaces.CanonicalProviderSessionProvider(clone.Provider)
	}
	return clone
}
