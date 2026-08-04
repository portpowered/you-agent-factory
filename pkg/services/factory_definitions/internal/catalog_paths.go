package internal

import (
	"context"
	"errors"
	"fmt"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// catalogPathsService is the stateless implementation of the narrow,
// read-only Factory Definitions catalog/path capability.
type catalogPathsService struct {
	listEffective       factorydefinitions.EffectiveFactoryCatalogOperation
	resolveNamedFactory factorydefinitions.ResolveNamedFactoryOperation
	resolveCurrentDir   factorydefinitions.CurrentFactoryDirectoryResolver
	logger              logging.Logger
}

// NewCatalogPathsService constructs the narrow, stateless, read-only Factory
// Definitions catalog/path capability from exact injected read collaborators.
// Construction performs no filesystem reads or writes, starts no lifecycle
// work, and caches no operation results. logger is the direct, required
// operation-logging abstraction; callers with no operation logging pass
// logging.NoopLogger{}.
func NewCatalogPathsService(
	listEffective factorydefinitions.EffectiveFactoryCatalogOperation,
	resolveNamedFactory factorydefinitions.ResolveNamedFactoryOperation,
	resolveCurrentDir factorydefinitions.CurrentFactoryDirectoryResolver,
	logger logging.Logger,
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
		logger:              logging.EnsureLogger(logger),
	}, nil
}

// classifyCatalogPathsFailure returns a stable, safe reason label for
// operation logs. It never echoes a raw path, name, or collaborator error
// message, only the classification sentinel the failure unwraps to.
func classifyCatalogPathsFailure(err error) string {
	switch {
	case errors.Is(err, context.Canceled):
		return "context_canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return "context_deadline_exceeded"
	case errors.Is(err, factorydefinitions.ErrInvalidNamedFactoryName):
		return "invalid_name"
	case errors.Is(err, factorydefinitions.ErrNamedFactoryNotFound):
		return "named_factory_not_found"
	case errors.Is(err, factorydefinitions.ErrFactoryLayoutNotFound):
		return "factory_layout_not_found"
	}
	return "operation_failed"
}

func (s *catalogPathsService) ListEffectiveFactories(
	ctx context.Context,
	request factorydefinitions.ListEffectiveFactoriesRequest,
) (factorydefinitions.ListEffectiveFactoriesResult, error) {
	s.logger.Info("factory_definitions.catalog_paths.list_effective_factories.started")
	result, err := s.listEffective(ctx, request)
	if err != nil {
		s.logger.Warn(
			"factory_definitions.catalog_paths.list_effective_factories.failed",
			"reason", classifyCatalogPathsFailure(err),
		)
		return factorydefinitions.ListEffectiveFactoriesResult{}, err
	}
	s.logger.Info(
		"factory_definitions.catalog_paths.list_effective_factories.finished",
		"entry_count", len(result.Entries),
	)
	return result, nil
}

func (s *catalogPathsService) ResolveNamedFactory(
	ctx context.Context,
	request factorydefinitions.ResolveNamedFactoryRequest,
) (factorydefinitions.ResolveNamedFactoryResult, error) {
	s.logger.Info("factory_definitions.catalog_paths.resolve_named_factory.started")
	if err := ctx.Err(); err != nil {
		s.logger.Warn(
			"factory_definitions.catalog_paths.resolve_named_factory.failed",
			"reason", classifyCatalogPathsFailure(err),
		)
		return factorydefinitions.ResolveNamedFactoryResult{}, err
	}
	result, err := s.resolveNamedFactory(ctx, request)
	if err != nil {
		s.logger.Warn(
			"factory_definitions.catalog_paths.resolve_named_factory.failed",
			"reason", classifyCatalogPathsFailure(err),
		)
		return factorydefinitions.ResolveNamedFactoryResult{}, err
	}
	s.logger.Info(
		"factory_definitions.catalog_paths.resolve_named_factory.finished",
		"source", string(result.Resolution.Source),
	)
	return result, nil
}

func (s *catalogPathsService) ResolveCurrentFactoryLocation(
	ctx context.Context,
	request factorydefinitions.ResolveCurrentFactoryLocationRequest,
) (factorydefinitions.ResolveCurrentFactoryLocationResult, error) {
	s.logger.Info("factory_definitions.catalog_paths.resolve_current_factory_location.started")
	if err := ctx.Err(); err != nil {
		s.logger.Warn(
			"factory_definitions.catalog_paths.resolve_current_factory_location.failed",
			"reason", classifyCatalogPathsFailure(err),
		)
		return factorydefinitions.ResolveCurrentFactoryLocationResult{}, err
	}
	factoryDir, err := s.resolveCurrentDir(request.RootDir)
	if err != nil {
		s.logger.Warn(
			"factory_definitions.catalog_paths.resolve_current_factory_location.failed",
			"reason", classifyCatalogPathsFailure(err),
		)
		return factorydefinitions.ResolveCurrentFactoryLocationResult{}, err
	}
	s.logger.Info("factory_definitions.catalog_paths.resolve_current_factory_location.finished")
	return factorydefinitions.ResolveCurrentFactoryLocationResult{FactoryDir: factoryDir}, nil
}
