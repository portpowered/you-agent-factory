package internal

import (
	"context"
	"fmt"

	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/providers/internal/services/catalog"
	"github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution"
)

type service struct {
	adapters map[providers.ExecutionKind]execution.Adapter
}

func New(adapters map[providers.ExecutionKind]execution.Adapter) execution.Service {
	cloned := make(map[providers.ExecutionKind]execution.Adapter, len(adapters))
	for kind, adapter := range adapters {
		cloned[kind] = adapter
	}
	return &service{adapters: cloned}
}

func (s *service) Execute(ctx context.Context, descriptor catalog.Descriptor, request providers.ExecuteRequest) (providers.ExecuteResponse, error) {
	adapter := s.adapters[descriptor.Provider.ExecutionKind]
	if adapter.Execute == nil {
		return providers.ExecuteResponse{}, fmt.Errorf("%w: %q", providers.ErrUnsupportedExecutor, descriptor.Provider.ExecutionKind)
	}
	return adapter.Execute(ctx, descriptor, request)
}

func (s *service) ExecuteStream(ctx context.Context, descriptor catalog.Descriptor, request providers.ExecuteRequest) (*providers.ExecutionStream, error) {
	adapter := s.adapters[descriptor.Provider.ExecutionKind]
	if adapter.ExecuteStream == nil {
		return nil, fmt.Errorf("%w: %q", providers.ErrUnsupportedExecutor, descriptor.Provider.ExecutionKind)
	}
	return adapter.ExecuteStream(ctx, descriptor, request)
}
