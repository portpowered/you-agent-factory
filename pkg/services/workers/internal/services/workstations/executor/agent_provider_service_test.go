package executor

import (
	"context"

	"github.com/portpowered/infinite-you/pkg/services/providers"
)

func (m *agentMockProvider) ResolveIdentity(
	ctx context.Context,
	request providers.ResolveIdentityRequest,
) (providers.ResolveIdentityResult, error) {
	if request.Identity == "" {
		request.Identity = "codex"
	}
	return m.ProviderServiceAdapter.ResolveIdentity(ctx, request)
}

func (m *agentMockProvider) ValidatePrerequisites(
	ctx context.Context,
	request providers.ValidatePrerequisitesRequest,
) error {
	if request.ID == "" {
		request.ID = providers.IDCodex
	}
	return m.ProviderServiceAdapter.ValidatePrerequisites(ctx, request)
}

func (m *agentMockProvider) Execute(
	ctx context.Context,
	request providers.ExecuteRequest,
) (providers.ExecuteResult, error) {
	adapter := m.ProviderServiceAdapter
	adapter.InferFunc = m.Infer
	return adapter.Execute(ctx, request)
}

func (m *agentMockProvider) Continue(
	ctx context.Context,
	request providers.ContinueRequest,
) (providers.ContinueResult, error) {
	adapter := m.ProviderServiceAdapter
	adapter.InferFunc = m.Infer
	return adapter.Continue(ctx, request)
}

func (m *agentMockProvider) ContinueReference(
	ctx context.Context,
	request providers.ContinueReferenceRequest,
) (providers.ContinueReferenceResult, error) {
	reference, err := request.Reference.ToSessionRef()
	if err != nil {
		return providers.ContinueReferenceResult{}, providers.ContinuationFailure{
			Kind:      providers.ContinuationFailureKindInvalid,
			Message:   err.Error(),
			Reference: providers.SessionRef{},
		}
	}
	continued, err := m.Continue(ctx, providers.ContinueRequest{
		Reference: reference,
		Attempt:   request.Attempt,
	})
	if err != nil {
		return providers.ContinueReferenceResult{}, err
	}
	continuedReference := continued.Reference
	if continuedReference.Provider == "" {
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
