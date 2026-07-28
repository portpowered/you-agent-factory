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
	provider providers.Descriptor
	attempt  execution.Attempt
}

type service struct {
	catalog  catalog.Service
	adapters map[providers.ID]adapterBinding
}

var _ execution.Service = (*service)(nil)

// New constructs an inert execution service over one canonical catalog
// authority and an immutable set of private adapter attempts.
func New(
	catalogService catalog.Service,
	registrations ...execution.Registration,
) (execution.Service, error) {
	if catalogService == nil {
		return nil, fmt.Errorf("construct Providers Execution: catalog is required")
	}
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
		if registration.Attempt == nil {
			return nil, fmt.Errorf(
				"construct Providers Execution: adapter for %q is required",
				registration.Provider,
			)
		}
		if _, exists := adapters[registration.Provider]; exists {
			return nil, fmt.Errorf(
				"construct Providers Execution: duplicate adapter for %q",
				registration.Provider,
			)
		}
		adapters[registration.Provider] = adapterBinding{
			provider: descriptor,
			attempt:  registration.Attempt,
		}
	}
	return &service{catalog: catalogService, adapters: adapters}, nil
}

func (s *service) Execute(
	ctx context.Context,
	request providers.ExecuteRequest,
) (result providers.ExecuteResult, executeErr error) {
	detached := request.Clone()
	defer func() {
		if contextErr := normalizeContextFailure(ctx, detached); contextErr != nil {
			result = providers.ExecuteResult{}
			executeErr = contextErr
		}
	}()
	if contextErr := normalizeContextFailure(ctx, detached); contextErr != nil {
		return providers.ExecuteResult{}, contextErr
	}
	if err := detached.Validate(); err != nil {
		return providers.ExecuteResult{}, normalizeValidationFailure(detached)
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
	result, err = binding.attempt(ctx, detached)
	if err != nil {
		return providers.ExecuteResult{}, normalizeAttemptFailure(ctx, err, detached)
	}
	return normalizeSuccess(result, resolved.Provider.ID, detached)
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
