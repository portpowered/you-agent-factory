package internal

import (
	"context"
	"fmt"

	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/providers/internal/services/catalog"
	"github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution"
)

type service struct {
	catalog   catalog.Service
	execution execution.Service
}

func New(catalogService catalog.Service, executionService execution.Service) providers.Service {
	return &service{catalog: catalogService, execution: executionService}
}

func (s *service) List(ctx context.Context, _ providers.ListRequest) (providers.ListResponse, error) {
	descriptors, err := s.catalog.List(ctx)
	if err != nil {
		return providers.ListResponse{}, err
	}
	response := providers.ListResponse{Providers: make([]providers.Provider, len(descriptors))}
	for index, descriptor := range descriptors {
		response.Providers[index] = descriptor.Provider.Clone()
	}
	return response, nil
}

func (s *service) Get(ctx context.Context, request providers.GetRequest) (providers.GetResponse, error) {
	descriptor, err := s.catalog.Get(ctx, request.ID)
	if err != nil {
		return providers.GetResponse{}, err
	}
	return providers.GetResponse{Provider: descriptor.Provider.Clone()}, nil
}

func (s *service) Execute(ctx context.Context, request providers.ExecuteRequest) (providers.ExecuteResponse, error) {
	descriptor, err := s.executableDescriptor(ctx, request)
	if err != nil {
		return providers.ExecuteResponse{}, err
	}
	return s.execution.Execute(ctx, descriptor, request)
}

func (s *service) ExecuteStream(ctx context.Context, request providers.ExecuteRequest) (*providers.ExecutionStream, error) {
	descriptor, err := s.executableDescriptor(ctx, request)
	if err != nil {
		return nil, err
	}
	return s.execution.ExecuteStream(ctx, descriptor, request)
}

func (s *service) executableDescriptor(ctx context.Context, request providers.ExecuteRequest) (catalog.Descriptor, error) {
	if err := request.ProviderID.Validate(); err != nil {
		return catalog.Descriptor{}, fmt.Errorf("%w: %v", providers.ErrInvalidRequest, err)
	}
	descriptor, err := s.catalog.Get(ctx, request.ProviderID)
	if err != nil {
		return catalog.Descriptor{}, err
	}
	if descriptor.Provider.Availability.State != providers.AvailabilityAvailable {
		return catalog.Descriptor{}, fmt.Errorf("%w: %q: %s", providers.ErrUnavailableProvider, request.ProviderID, descriptor.Provider.Availability.Detail)
	}
	return descriptor, nil
}
