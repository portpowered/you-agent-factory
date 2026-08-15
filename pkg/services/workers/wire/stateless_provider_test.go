package wire

import (
	"context"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/providers"
)

// statelessProviderContract supplies the native Providers operations that are
// not relevant to these Workers execution assertions. Keeping the test seam
// execute-shaped means the core Workers tests do not need the legacy
// inference-shaped adapter used by later caller-family migrations.
type statelessProviderContract struct{}

func (statelessProviderContract) ListProviders(
	context.Context,
	providers.ListProvidersRequest,
) (providers.ListProvidersResult, error) {
	return providers.ListProvidersResult{Providers: []providers.Descriptor{{ID: providers.IDCodex}}}, nil
}

func (statelessProviderContract) GetProvider(
	_ context.Context,
	request providers.GetProviderRequest,
) (providers.GetProviderResult, error) {
	if err := request.Validate(); err != nil {
		return providers.GetProviderResult{}, err
	}
	if request.ID != providers.IDCodex {
		return providers.GetProviderResult{}, providers.ErrUnknownProvider
	}
	return providers.GetProviderResult{Provider: providers.Descriptor{ID: providers.IDCodex}}, nil
}

func (statelessProviderContract) ResolveIdentity(
	_ context.Context,
	request providers.ResolveIdentityRequest,
) (providers.ResolveIdentityResult, error) {
	identity := strings.ToLower(strings.TrimSpace(request.Identity))
	if identity == "openai" {
		identity = string(providers.IDCodex)
	}
	if identity != string(providers.IDCodex) {
		return providers.ResolveIdentityResult{}, providers.ErrUnknownProvider
	}
	return providers.ResolveIdentityResult{ID: providers.IDCodex}, nil
}

func (statelessProviderContract) ResolveSelection(
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
	resolved, err := (statelessProviderContract{}).ResolveIdentity(
		ctx,
		providers.ResolveIdentityRequest{Identity: identity},
	)
	if err != nil {
		return providers.ResolveSelectionResult{}, err
	}
	return providers.ResolveSelectionResult{Provider: resolved.ID}, nil
}

func (statelessProviderContract) ControlAttempt(
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

func (statelessProviderContract) Continue(
	_ context.Context,
	request providers.ContinueRequest,
) (providers.ContinueResult, error) {
	if err := request.Validate(); err != nil {
		return providers.ContinueResult{}, err
	}
	return providers.ContinueResult{
		Reference: request.Reference,
		Outcome:   providers.ContinuationOutcomeUnsupported,
	}, nil
}

func (statelessProviderContract) ContinueReference(
	_ context.Context,
	request providers.ContinueReferenceRequest,
) (providers.ContinueReferenceResult, error) {
	reference, err := request.Reference.ToSessionRef()
	if err != nil {
		return providers.ContinueReferenceResult{}, err
	}
	return providers.ContinueReferenceResult{
		Reference: reference.ContinuationRef(),
		Outcome:   providers.ContinuationOutcomeUnsupported,
	}, nil
}
