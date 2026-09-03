package inference_test

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/pkg/services/providers"
)

// inferenceProviderOverride routes an optional Providers edge by the immutable
// Factory directory captured on each execution request. Selection behavior is
// stable across every scenario; only the final provider call varies. This lets
// independent customer sessions overlap without changing a process-wide
// delegate while either session is live.
type inferenceProviderOverride struct {
	catalog providers.Service
	mu      sync.RWMutex
	byDir   map[string]providers.Service
}

func newInferenceProviderOverride() *inferenceProviderOverride {
	return &inferenceProviderOverride{
		catalog: testutil.NativeProvider{},
		byDir:   make(map[string]providers.Service),
	}
}

func (router *inferenceProviderOverride) bind(dir string, delegate providers.Service) (func(), error) {
	if delegate == nil {
		return func() {}, nil
	}
	key := cleanInferencePath(dir)
	router.mu.Lock()
	defer router.mu.Unlock()
	if _, exists := router.byDir[key]; exists {
		return nil, fmt.Errorf("shared inference provider override route %q is already bound", key)
	}
	router.byDir[key] = delegate
	return func() {
		router.mu.Lock()
		delete(router.byDir, key)
		router.mu.Unlock()
	}, nil
}

func (router *inferenceProviderOverride) executionDelegate(request providers.ExecuteRequest) (providers.Service, error) {
	router.mu.RLock()
	defer router.mu.RUnlock()
	for _, candidate := range []string{request.FactoryDirectory, request.WorkingDirectory} {
		if delegate := router.byDir[cleanInferencePath(candidate)]; delegate != nil {
			return delegate, nil
		}
	}
	return nil, errors.New("shared inference provider override has no route for the invocation")
}

func (router *inferenceProviderOverride) ListProviders(
	ctx context.Context,
	request providers.ListProvidersRequest,
) (providers.ListProvidersResult, error) {
	return router.catalog.ListProviders(ctx, request)
}

func (router *inferenceProviderOverride) GetProvider(
	ctx context.Context,
	request providers.GetProviderRequest,
) (providers.GetProviderResult, error) {
	return router.catalog.GetProvider(ctx, request)
}

func (router *inferenceProviderOverride) ResolveIdentity(
	ctx context.Context,
	request providers.ResolveIdentityRequest,
) (providers.ResolveIdentityResult, error) {
	return router.catalog.ResolveIdentity(ctx, request)
}

func (router *inferenceProviderOverride) ResolveSelection(
	ctx context.Context,
	request providers.ResolveSelectionRequest,
) (providers.ResolveSelectionResult, error) {
	return router.catalog.ResolveSelection(ctx, request)
}

func (router *inferenceProviderOverride) ValidatePrerequisites(
	ctx context.Context,
	request providers.ValidatePrerequisitesRequest,
) error {
	return router.catalog.ValidatePrerequisites(ctx, request)
}

func (router *inferenceProviderOverride) Execute(
	ctx context.Context,
	request providers.ExecuteRequest,
) (providers.ExecuteResult, error) {
	delegate, err := router.executionDelegate(request)
	if err != nil {
		return providers.ExecuteResult{}, err
	}
	return delegate.Execute(ctx, request)
}

func (router *inferenceProviderOverride) ControlAttempt(
	ctx context.Context,
	request providers.ControlAttemptRequest,
) (providers.ControlAttemptResult, error) {
	return router.catalog.ControlAttempt(ctx, request)
}

func (router *inferenceProviderOverride) Continue(
	ctx context.Context,
	request providers.ContinueRequest,
) (providers.ContinueResult, error) {
	return router.catalog.Continue(ctx, request)
}

func (router *inferenceProviderOverride) ContinueReference(
	ctx context.Context,
	request providers.ContinueReferenceRequest,
) (providers.ContinueReferenceResult, error) {
	return router.catalog.ContinueReference(ctx, request)
}

type inferenceIntegrationRouter struct {
	identity string
	mu       sync.RWMutex
	delegate sharedInferenceProviderIntegration
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
