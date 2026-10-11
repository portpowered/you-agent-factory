package service

import (
	"context"
	"fmt"
	"slices"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	catalog "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/catalog"
	execution "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution"
)

type adapterBinding struct {
	provider        providers.Descriptor
	attempt         execution.Attempt
	continueAttempt execution.ContinuationAttempt
}

type service struct {
	catalog  catalog.Service
	adapters map[providers.ID]adapterBinding
}

var _ execution.ContinuationService = (*service)(nil)

// UnsupportedContinuation supplies the explicit unavailable operation for
// registrations whose adapter does not support provider-session continuation.
type UnsupportedContinuation struct{}

func (UnsupportedContinuation) Continue(context.Context, execution.ContinuationRequest) (providers.ExecuteResult, error) {
	return providers.ExecuteResult{}, providers.ExecuteFailure{
		Kind:    providers.ExecuteFailureKindDependency,
		Message: "provider continuation adapter is unavailable",
	}
}

// New constructs an inert execution service over one canonical catalog
// authority and an immutable set of private adapter attempts. The returned
// role includes exact-session continuation for direct injection into the root.
// Composition supplies the completed catalog authority.
func New(
	catalogService catalog.Service,
	registrations ...execution.Registration,
) (execution.ContinuationService, error) {
	adapters := make(map[providers.ID]adapterBinding, len(registrations))
	for _, registration := range registrations {
		if err := registration.Provider.Validate(); err != nil {
			return nil, fmt.Errorf("construct Providers Execution: %w", err)
		}
		descriptor, err := catalogService.RegistrationProvider(registration.Provider)
		if err != nil {
			return nil, fmt.Errorf(
				"construct Providers Execution: adapter provider %q: %w",
				registration.Provider,
				err,
			)
		}
		if descriptor.ID != registration.Provider {
			return nil, fmt.Errorf(
				"construct Providers Execution: adapter provider %q must use canonical id %q",
				registration.Provider,
				descriptor.ID,
			)
		}
		if descriptor.Availability != providers.AvailabilitySelectable {
			return nil, fmt.Errorf(
				"construct Providers Execution: adapter provider %q: %w",
				registration.Provider,
				providers.ErrProviderUnavailable,
			)
		}
		if _, exists := adapters[registration.Provider]; exists {
			return nil, fmt.Errorf(
				"construct Providers Execution: duplicate adapter for %q",
				registration.Provider,
			)
		}
		adapters[registration.Provider] = adapterBinding{
			provider:        descriptor,
			attempt:         registration.Attempt,
			continueAttempt: registration.Continue,
		}
	}
	return &service{catalog: catalogService, adapters: adapters}, nil
}

// Continue performs one validated exact-session adapter attempt. It shares
// ordinary execution normalization but uses the separate private continuation
// seam, so a public Execute request can never populate its resume reference.
func (s *service) Continue(
	ctx context.Context,
	request execution.ContinuationRequest,
) (result providers.ExecuteResult, executeErr error) {
	detached := request.Clone()
	redactionRequest := detached.ExecuteRequest.Clone()
	secret := continuationDiagnosticSecrets(detached.ResumeSession)
	defer func() {
		if contextErr := normalizeContextFailureWithExisting(ctx, redactionRequest, executeErr, secret...); contextErr != nil {
			executeErr = contextErr
		}
	}()
	if contextErr := normalizeContextFailure(ctx, redactionRequest, secret...); contextErr != nil {
		return providers.ExecuteResult{}, contextErr
	}
	if err := detached.Validate(); err != nil {
		return providers.ExecuteResult{}, normalizeValidationFailure(redactionRequest, secret...)
	}
	resolved, err := s.catalog.GetProvider(ctx, providers.GetProviderRequest{ID: detached.Provider})
	if err != nil {
		return providers.ExecuteResult{}, err
	}
	binding, ok := s.adapters[resolved.Provider.ID]
	if !ok {
		return providers.ExecuteResult{}, providers.ErrProviderUnavailable
	}
	if !sameRegistrationFacts(binding.provider, resolved.Provider) {
		return providers.ExecuteResult{}, providers.ExecuteFailure{
			Kind:    providers.ExecuteFailureKindDependency,
			Message: "provider registration does not match the canonical catalog",
		}
	}
	detached.Provider = resolved.Provider.ID
	redactionRequest.Provider = resolved.Provider.ID
	if detached.ResumeSession != nil {
		detached.ResumeSession.Provider = resolved.Provider.ID
	}
	detached.ExecuteRequest = safeProgressRequest(detached.ExecuteRequest, secret...)
	result, err = binding.continueAttempt(ctx, detached)
	normalizedResult, resultErr := normalizeSuccess(result, resolved.Provider.ID, redactionRequest, secret...)
	if resultErr != nil {
		return providers.ExecuteResult{}, normalizeAttemptFailure(ctx, resultErr, redactionRequest, secret...)
	}
	if err != nil {
		return normalizedResult, normalizeAttemptFailure(ctx, err, redactionRequest, secret...)
	}
	return normalizedResult, nil
}

func (s *service) Execute(
	ctx context.Context,
	request providers.ExecuteRequest,
) (result providers.ExecuteResult, executeErr error) {
	detached := request.Clone()
	redactionRequest := detached.Clone()
	defer func() {
		if contextErr := normalizeContextFailureWithExisting(ctx, redactionRequest, executeErr); contextErr != nil {
			executeErr = contextErr
		}
	}()
	if contextErr := normalizeContextFailure(ctx, redactionRequest); contextErr != nil {
		return providers.ExecuteResult{}, contextErr
	}
	if err := detached.Validate(); err != nil {
		return providers.ExecuteResult{}, normalizeValidationFailure(redactionRequest)
	}
	resolved, err := s.catalog.GetProvider(
		ctx,
		providers.GetProviderRequest{ID: detached.Provider},
	)
	if err != nil {
		return providers.ExecuteResult{}, err
	}
	binding, ok := s.adapters[resolved.Provider.ID]
	if !ok {
		return providers.ExecuteResult{}, providers.ErrProviderUnavailable
	}
	if !sameRegistrationFacts(binding.provider, resolved.Provider) {
		return providers.ExecuteResult{}, providers.ExecuteFailure{
			Kind:    providers.ExecuteFailureKindDependency,
			Message: "provider registration does not match the canonical catalog",
		}
	}
	detached.Provider = resolved.Provider.ID
	redactionRequest.Provider = resolved.Provider.ID
	detached = safeProgressRequest(detached)
	result, err = binding.attempt(ctx, detached)
	normalizedResult, resultErr := normalizeSuccess(result, resolved.Provider.ID, redactionRequest)
	if resultErr != nil {
		return providers.ExecuteResult{}, normalizeAttemptFailure(ctx, resultErr, redactionRequest)
	}
	if err != nil {
		return normalizedResult, normalizeAttemptFailure(ctx, err, redactionRequest)
	}
	return normalizedResult, nil
}

func normalizeContextFailureWithExisting(
	ctx context.Context,
	request providers.ExecuteRequest,
	existing error,
	extraSecrets ...string,
) error {
	contextFailure := normalizeContextFailure(ctx, request, extraSecrets...)
	if contextFailure == nil {
		return nil
	}
	existingFailure, existingOK := executeFailureAs(existing)
	if !existingOK {
		return contextFailure
	}
	// normalizeContextFailure returns an ExecuteFailure whenever it returns a
	// non-nil error, so its typed fields remain safe to preserve below.
	normalized, _ := executeFailureAs(contextFailure)
	normalized.SessionRef = existingFailure.SessionRef
	normalized.Diagnostics = existingFailure.Diagnostics
	return normalizeDeclaredFailure(normalized, request, extraSecrets...)
}

func continuationDiagnosticSecrets(reference *providers.SessionRef) []string {
	if reference == nil {
		return nil
	}
	return []string{reference.ID}
}

func sameRegistrationFacts(
	registered providers.Descriptor,
	resolved providers.Descriptor,
) bool {
	return registered.ID == resolved.ID &&
		registered.Availability == resolved.Availability &&
		slices.Equal(registered.Aliases, resolved.Aliases) &&
		slices.Equal(registered.Capabilities, resolved.Capabilities)
}
