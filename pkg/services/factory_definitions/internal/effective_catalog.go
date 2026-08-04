package internal

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

type effectiveCatalog struct {
	discovery factorydefinitions.EffectiveFactoryCatalogDiscovery
	normalize factorydefinitions.EffectiveFactoryDefinitionNormalizer
}

type effectiveCatalogService struct {
	factorydefinitions.Service
	listEffective factorydefinitions.EffectiveFactoryCatalogOperation
}

// EffectiveCatalogService is the read-only Factory Definitions owner used by
// transports that do not require a Factory Session.
type EffectiveCatalogService struct {
	listEffective factorydefinitions.EffectiveFactoryCatalogOperation
}

// NewEffectiveCatalog constructs the stateless effective Factory catalog.
func NewEffectiveCatalog(
	discovery factorydefinitions.EffectiveFactoryCatalogDiscovery,
	normalize factorydefinitions.EffectiveFactoryDefinitionNormalizer,
) (factorydefinitions.EffectiveFactoryCatalogOperation, error) {
	if discovery.ListRoot == nil || discovery.ListPackaged == nil {
		return nil, fmt.Errorf("effective Factory catalog source is required")
	}
	if normalize == nil {
		return nil, fmt.Errorf("effective Factory definition normalizer is required")
	}
	catalog := effectiveCatalog{discovery: discovery, normalize: normalize}
	return catalog.listEffectiveFactories, nil
}

// AttachEffectiveCatalog returns the Factory Definitions service with
// effective discovery delegated to listEffective while preserving every other
// root operation.
func AttachEffectiveCatalog(
	service factorydefinitions.Service,
	listEffective factorydefinitions.EffectiveFactoryCatalogOperation,
) (factorydefinitions.Service, error) {
	if service == nil {
		return nil, fmt.Errorf("Factory Definitions service is required")
	}
	if listEffective == nil {
		return nil, fmt.Errorf("effective Factory catalog is required")
	}
	return effectiveCatalogService{Service: service, listEffective: listEffective}, nil
}

// NewEffectiveCatalogService constructs the read-only Factory Definitions
// service slice used by transports that do not require a Factory Session.
func NewEffectiveCatalogService(
	listEffective factorydefinitions.EffectiveFactoryCatalogOperation,
) (*EffectiveCatalogService, error) {
	if listEffective == nil {
		return nil, fmt.Errorf("effective Factory catalog is required")
	}
	return &EffectiveCatalogService{listEffective: listEffective}, nil
}

func (s effectiveCatalogService) ListEffectiveFactories(
	ctx context.Context,
	request factorydefinitions.ListEffectiveFactoriesRequest,
) (factorydefinitions.ListEffectiveFactoriesResult, error) {
	return s.listEffective(ctx, request)
}

func (s *EffectiveCatalogService) ListEffectiveFactories(
	ctx context.Context,
	request factorydefinitions.ListEffectiveFactoriesRequest,
) (factorydefinitions.ListEffectiveFactoriesResult, error) {
	return s.listEffective(ctx, request)
}

func (c effectiveCatalog) listEffectiveFactories(
	ctx context.Context,
	request factorydefinitions.ListEffectiveFactoriesRequest,
) (factorydefinitions.ListEffectiveFactoriesResult, error) {
	if err := ctx.Err(); err != nil {
		return factorydefinitions.ListEffectiveFactoriesResult{}, err
	}
	if err := validateRequest(request); err != nil {
		return factorydefinitions.ListEffectiveFactoriesResult{}, err
	}

	project, err := c.discovery.ListRoot(ctx, request.ProjectRoot)
	if contextErr := ctx.Err(); contextErr != nil {
		return factorydefinitions.ListEffectiveFactoriesResult{}, contextErr
	}
	if err != nil {
		return factorydefinitions.ListEffectiveFactoriesResult{}, fmt.Errorf(
			"discover project-local Factories: %w",
			err,
		)
	}
	global, err := c.discovery.ListRoot(ctx, request.GlobalRoot)
	if contextErr := ctx.Err(); contextErr != nil {
		return factorydefinitions.ListEffectiveFactoriesResult{}, contextErr
	}
	if err != nil {
		return factorydefinitions.ListEffectiveFactoriesResult{}, fmt.Errorf(
			"discover global Factories: %w",
			err,
		)
	}
	packaged, err := c.discovery.ListPackaged(ctx)
	if contextErr := ctx.Err(); contextErr != nil {
		return factorydefinitions.ListEffectiveFactoriesResult{}, contextErr
	}
	if err != nil {
		return factorydefinitions.ListEffectiveFactoriesResult{}, fmt.Errorf(
			"discover packaged Factories: %w",
			err,
		)
	}

	entries, diagnostics, err := c.merge(ctx,
		catalogSource{
			kind:       factorydefinitions.EffectiveFactoryCatalogSourceProjectLocal,
			candidates: project,
		},
		catalogSource{
			kind:       factorydefinitions.EffectiveFactoryCatalogSourceGlobal,
			candidates: global,
		},
		catalogSource{
			kind:       factorydefinitions.EffectiveFactoryCatalogSourcePackaged,
			candidates: packaged,
		},
	)
	if err != nil {
		return factorydefinitions.ListEffectiveFactoriesResult{}, err
	}
	return factorydefinitions.ListEffectiveFactoriesResult{
		Entries:     entries,
		Diagnostics: diagnostics,
	}, nil
}

func validateRequest(request factorydefinitions.ListEffectiveFactoriesRequest) error {
	if strings.TrimSpace(request.ProjectRoot) == "" {
		return fmt.Errorf("project Factory root is required")
	}
	if strings.TrimSpace(request.GlobalRoot) == "" {
		return fmt.Errorf("global Factory root is required")
	}
	return nil
}

func detachedEntry(
	candidate factorydefinitions.EffectiveFactoryCatalogCandidate,
	definition *factorydefinitions.FactoryConfig,
) (factorydefinitions.EffectiveFactoryCatalogEntry, error) {
	cloned, err := factorydefinitions.CloneFactoryConfig(definition)
	if err != nil {
		return factorydefinitions.EffectiveFactoryCatalogEntry{}, err
	}
	return factorydefinitions.EffectiveFactoryCatalogEntry{
		Name:                candidate.Name,
		Location:            cloneString(candidate.Location),
		Definition:          cloned,
		InvocationSignature: cloned.InvocationSignature,
	}, nil
}

func cloneCandidate(
	candidate factorydefinitions.EffectiveFactoryCatalogCandidate,
) factorydefinitions.EffectiveFactoryCatalogCandidate {
	candidate.Location = cloneString(candidate.Location)
	candidate.Canonical = append([]byte(nil), candidate.Canonical...)
	return candidate
}

func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

// catalogPathsService is the stateless implementation of the narrow,
// read-only Factory Definitions catalog/path capability. It is intentionally
// unexported: the capability's public contract is published at the Wire
// composition boundary (pkg/services/factory_definitions/wire), not at the
// factory_definitions root, which already carries pre-existing,
// deletion-only interface-count debt this capability must not grow.
type catalogPathsService struct {
	listEffective       factorydefinitions.EffectiveFactoryCatalogOperation
	resolveNamedFactory func(context.Context, factorydefinitions.ResolveNamedFactoryRequest) (factorydefinitions.ResolveNamedFactoryResult, error)
	resolveCurrentDir   factorydefinitions.CurrentFactoryDirectoryResolver
	logger              logging.Logger
}

// NewCatalogPathsService constructs the narrow, stateless, read-only Factory
// Definitions catalog/path capability from exact injected read collaborators.
// Construction performs no filesystem reads or writes, starts no lifecycle
// work, and caches no operation results. logger is the direct, required
// operation-logging abstraction; callers with no operation logging pass
// logging.NoopLogger{}. The returned type is unexported; callers hold it via
// type inference and return it onward as whatever interface their own
// package publishes, exactly as any other unexported-type-behind-an-exported-
// constructor Go value works.
func NewCatalogPathsService(
	listEffective factorydefinitions.EffectiveFactoryCatalogOperation,
	resolveNamedFactory func(context.Context, factorydefinitions.ResolveNamedFactoryRequest) (factorydefinitions.ResolveNamedFactoryResult, error),
	resolveCurrentDir factorydefinitions.CurrentFactoryDirectoryResolver,
	logger logging.Logger,
) (*catalogPathsService, error) {
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
