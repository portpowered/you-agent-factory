package service

import (
	"context"
	"fmt"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	catalog "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/catalog"
	execution "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution"
)

type service struct {
	catalog  catalog.Service
	attempts map[providers.ID]execution.Attempt
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
	attempts := make(map[providers.ID]execution.Attempt, len(registrations))
	for _, registration := range registrations {
		if err := registration.Provider.Validate(); err != nil {
			return nil, fmt.Errorf("construct Providers Execution: %w", err)
		}
		canonical, err := catalogService.ResolveProviderID(registration.Provider)
		if err != nil {
			return nil, fmt.Errorf(
				"construct Providers Execution: adapter provider %q: %w",
				registration.Provider,
				err,
			)
		}
		if canonical != registration.Provider {
			return nil, fmt.Errorf(
				"construct Providers Execution: adapter provider %q must use canonical id %q",
				registration.Provider,
				canonical,
			)
		}
		if registration.Attempt == nil {
			return nil, fmt.Errorf(
				"construct Providers Execution: adapter for %q is required",
				registration.Provider,
			)
		}
		if _, exists := attempts[registration.Provider]; exists {
			return nil, fmt.Errorf(
				"construct Providers Execution: duplicate adapter for %q",
				registration.Provider,
			)
		}
		attempts[registration.Provider] = registration.Attempt
	}
	return &service{catalog: catalogService, attempts: attempts}, nil
}

func (s *service) Execute(
	ctx context.Context,
	request providers.ExecuteRequest,
) (providers.ExecuteResult, error) {
	detached := request.Clone()
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
	attempt, ok := s.attempts[resolved.Provider.ID]
	if !ok {
		return providers.ExecuteResult{}, providers.ErrProviderUnavailable
	}
	detached.Provider = resolved.Provider.ID
	result, err := attempt(ctx, detached)
	if err != nil {
		return providers.ExecuteResult{}, normalizeAttemptFailure(ctx, err, detached)
	}
	return normalizeSuccess(result, resolved.Provider.ID, detached)
}
