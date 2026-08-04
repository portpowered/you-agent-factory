package internal

import (
	"context"
	"fmt"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// catalogPathsService is the stateless implementation of the narrow,
// read-only Factory Definitions catalog/path capability.
type catalogPathsService struct {
	listEffective       factorydefinitions.EffectiveFactoryCatalogOperation
	resolveNamedFactory factorydefinitions.ResolveNamedFactoryOperation
	resolveCurrentDir   factorydefinitions.CurrentFactoryDirectoryResolver
}

// NewCatalogPathsService constructs the narrow, stateless, read-only Factory
// Definitions catalog/path capability from exact injected read collaborators.
// Construction performs no filesystem reads or writes, starts no lifecycle
// work, and caches no operation results.
func NewCatalogPathsService(
	listEffective factorydefinitions.EffectiveFactoryCatalogOperation,
	resolveNamedFactory factorydefinitions.ResolveNamedFactoryOperation,
	resolveCurrentDir factorydefinitions.CurrentFactoryDirectoryResolver,
) (factorydefinitions.CatalogPathsService, error) {
	if listEffective == nil {
		return nil, fmt.Errorf("effective Factory catalog operation is required")
	}
	if resolveNamedFactory == nil {
		return nil, fmt.Errorf("named Factory resolve operation is required")
	}
	if resolveCurrentDir == nil {
		return nil, fmt.Errorf("current Factory directory resolver is required")
	}
	return &catalogPathsService{
		listEffective:       listEffective,
		resolveNamedFactory: resolveNamedFactory,
		resolveCurrentDir:   resolveCurrentDir,
	}, nil
}

func (s *catalogPathsService) ListEffectiveFactories(
	ctx context.Context,
	request factorydefinitions.ListEffectiveFactoriesRequest,
) (factorydefinitions.ListEffectiveFactoriesResult, error) {
	return s.listEffective(ctx, request)
}

func (s *catalogPathsService) ResolveNamedFactory(
	ctx context.Context,
	request factorydefinitions.ResolveNamedFactoryRequest,
) (factorydefinitions.ResolveNamedFactoryResult, error) {
	return s.resolveNamedFactory(ctx, request)
}

func (s *catalogPathsService) ResolveCurrentFactoryLocation(
	ctx context.Context,
	request factorydefinitions.ResolveCurrentFactoryLocationRequest,
) (factorydefinitions.ResolveCurrentFactoryLocationResult, error) {
	if err := ctx.Err(); err != nil {
		return factorydefinitions.ResolveCurrentFactoryLocationResult{}, err
	}
	factoryDir, err := s.resolveCurrentDir(request.RootDir)
	if err != nil {
		return factorydefinitions.ResolveCurrentFactoryLocationResult{}, err
	}
	return factorydefinitions.ResolveCurrentFactoryLocationResult{FactoryDir: factoryDir}, nil
}
