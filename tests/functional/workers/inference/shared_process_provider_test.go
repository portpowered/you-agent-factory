package inference_test

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/portpowered/infinite-you/pkg/services/providers"
)

// inferenceProviderOverride routes the optional Providers root edge to one
// serialized scenario. The shared application process keeps the canonical
// Providers service for ordinary named executor paths; only a scenario that
// explicitly supplies this edge can exercise the provider-result normalization
// path with structured diagnostics.
type inferenceProviderOverride struct {
	mu       sync.RWMutex
	delegate providers.Service
}

func (router *inferenceProviderOverride) set(delegate providers.Service) {
	router.mu.Lock()
	router.delegate = delegate
	router.mu.Unlock()
}

func (router *inferenceProviderOverride) current() (providers.Service, error) {
	router.mu.RLock()
	defer router.mu.RUnlock()
	if router.delegate == nil {
		return nil, errors.New("shared inference provider override is not configured")
	}
	return router.delegate, nil
}

func (router *inferenceProviderOverride) ListProviders(
	ctx context.Context,
	request providers.ListProvidersRequest,
) (providers.ListProvidersResult, error) {
	delegate, err := router.current()
	if err != nil {
		return providers.ListProvidersResult{}, err
	}
	return delegate.ListProviders(ctx, request)
}

func (router *inferenceProviderOverride) GetProvider(
	ctx context.Context,
	request providers.GetProviderRequest,
) (providers.GetProviderResult, error) {
	delegate, err := router.current()
	if err != nil {
		return providers.GetProviderResult{}, err
	}
	return delegate.GetProvider(ctx, request)
}

func (router *inferenceProviderOverride) ResolveIdentity(
	ctx context.Context,
	request providers.ResolveIdentityRequest,
) (providers.ResolveIdentityResult, error) {
	delegate, err := router.current()
	if err != nil {
		return providers.ResolveIdentityResult{}, err
	}
	return delegate.ResolveIdentity(ctx, request)
}

func (router *inferenceProviderOverride) ResolveSelection(
	ctx context.Context,
	request providers.ResolveSelectionRequest,
) (providers.ResolveSelectionResult, error) {
	delegate, err := router.current()
	if err != nil {
		return providers.ResolveSelectionResult{}, err
	}
	return delegate.ResolveSelection(ctx, request)
}

func (router *inferenceProviderOverride) ValidatePrerequisites(
	ctx context.Context,
	request providers.ValidatePrerequisitesRequest,
) error {
	delegate, err := router.current()
	if err != nil {
		return err
	}
	return delegate.ValidatePrerequisites(ctx, request)
}

func (router *inferenceProviderOverride) Execute(
	ctx context.Context,
	request providers.ExecuteRequest,
) (providers.ExecuteResult, error) {
	delegate, err := router.current()
	if err != nil {
		return providers.ExecuteResult{}, err
	}
	return delegate.Execute(ctx, request)
}

func (router *inferenceProviderOverride) ControlAttempt(
	ctx context.Context,
	request providers.ControlAttemptRequest,
) (providers.ControlAttemptResult, error) {
	delegate, err := router.current()
	if err != nil {
		return providers.ControlAttemptResult{}, err
	}
	return delegate.ControlAttempt(ctx, request)
}

func (router *inferenceProviderOverride) Continue(
	ctx context.Context,
	request providers.ContinueRequest,
) (providers.ContinueResult, error) {
	delegate, err := router.current()
	if err != nil {
		return providers.ContinueResult{}, err
	}
	return delegate.Continue(ctx, request)
}

func (router *inferenceProviderOverride) ContinueReference(
	ctx context.Context,
	request providers.ContinueReferenceRequest,
) (providers.ContinueReferenceResult, error) {
	delegate, err := router.current()
	if err != nil {
		return providers.ContinueReferenceResult{}, err
	}
	return delegate.ContinueReference(ctx, request)
}

type inferenceIntegrationRouter struct {
	identity string
	mu       sync.RWMutex
	delegate sharedInferenceProviderIntegration
}

func (router *inferenceIntegrationRouter) set(delegate sharedInferenceProviderIntegration) {
	router.mu.Lock()
	router.delegate = delegate
	router.mu.Unlock()
}

func (router *inferenceIntegrationRouter) bind(delegate sharedInferenceProviderIntegration) error {
	router.mu.Lock()
	defer router.mu.Unlock()
	if router.delegate != nil {
		return fmt.Errorf("shared inference provider %q is already bound", router.identity)
	}
	router.delegate = delegate
	return nil
}

func (router *inferenceIntegrationRouter) unbind() {
	router.mu.Lock()
	router.delegate = nil
	router.mu.Unlock()
}

func (router *inferenceIntegrationRouter) current() sharedInferenceProviderIntegration {
	router.mu.RLock()
	defer router.mu.RUnlock()
	return router.delegate
}

func (router *inferenceIntegrationRouter) Identity() sharedInferenceProviderIdentity {
	return sharedInferenceProviderIdentity(router.identity)
}

func (router *inferenceIntegrationRouter) MaximumCapabilities() sharedInferenceProviderCapabilitySet {
	if delegate := router.current(); delegate != nil {
		return delegate.MaximumCapabilities()
	}
	return sharedInferencePromptCapabilities()
}

func (router *inferenceIntegrationRouter) Discover(ctx context.Context) (sharedInferenceProviderDiscovery, error) {
	if delegate := router.current(); delegate != nil {
		return delegate.Discover(ctx)
	}
	return sharedInferenceProviderDiscovery{}, nil
}

func (router *inferenceIntegrationRouter) Capabilities(ctx context.Context, request sharedInferenceProviderInvocationRequest) (sharedInferenceProviderCapabilitySet, error) {
	if delegate := router.current(); delegate != nil {
		return delegate.Capabilities(ctx, request)
	}
	return router.MaximumCapabilities(), nil
}

func (router *inferenceIntegrationRouter) Invoke(ctx context.Context, request sharedInferenceProviderInvocationRequest, writer sharedInferenceProviderResponseWriter) error {
	if delegate := router.current(); delegate != nil {
		return delegate.Invoke(ctx, request, writer)
	}
	return fmt.Errorf("shared inference provider %q is not configured", router.identity)
}

func (group *inferenceProcessGroup) setExternalRegistrations(registrations []sharedInferenceProviderRegistration) {
	for _, router := range group.externals {
		router.set(nil)
	}
	for _, registration := range registrations {
		if router := group.externals[registration.Manifest.ID]; router != nil {
			router.set(registration.Integration)
		}
	}
}

func (group *inferenceProcessGroup) bindExternalRegistrations(
	registrations []sharedInferenceProviderRegistration,
) (func(), error) {
	bound := make([]*inferenceIntegrationRouter, 0, len(registrations))
	for _, registration := range registrations {
		router := group.externals[registration.Manifest.ID]
		if router == nil {
			for _, candidate := range bound {
				candidate.unbind()
			}
			return nil, fmt.Errorf("shared inference provider %q is not registered", registration.Manifest.ID)
		}
		if err := router.bind(registration.Integration); err != nil {
			for _, candidate := range bound {
				candidate.unbind()
			}
			return nil, err
		}
		bound = append(bound, router)
	}
	return func() {
		for _, router := range bound {
			router.unbind()
		}
	}, nil
}

func sharedInferenceExternalManifest(id, alias string) sharedInferenceProviderManifest {
	return sharedInferenceProviderManifest{
		ID:                         id,
		Aliases:                    []string{alias},
		ImplementationAvailability: sharedInferenceImplementationExternallySupplied,
		TechnicalSupportLevel:      sharedInferenceSupportProduction,
		MaximumExecutionCapabilities: sharedInferenceProviderExecutionCapabilities{
			PromptSubmission: true,
		},
	}
}

var _ sharedInferenceProviderIntegration = (*inferenceIntegrationRouter)(nil)
var _ providers.Service = (*inferenceProviderOverride)(nil)
