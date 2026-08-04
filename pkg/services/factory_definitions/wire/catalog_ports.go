package wire

import (
	"context"
	"fmt"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorydefinitionsinternal "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal"
	catalogwire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/catalog/wire"
)

// NewPathResolver constructs the catalog-owned named-path resolver from the
// exact filesystem port used by Factory Definitions Wire composition.
func NewPathResolver(
	fileSystem factorydefinitions.NamedPathFileSystem,
) (factorydefinitions.NamedPathResolver, error) {
	return catalogwire.NewPathResolver(fileSystem)
}

// NewNamedFactoryCatalog constructs the catalog-owned named-factory catalog
// from the exact path and catalog-filesystem ports used by Wire composition.
func NewNamedFactoryCatalog(
	paths factorydefinitions.NamedPathResolver,
	fileSystem factorydefinitions.NamedFactoryCatalogFileSystem,
) (factorydefinitions.NamedFactoryCatalog, error) {
	return catalogwire.NewNamedFactoryCatalog(paths, fileSystem)
}

// NewPersistence constructs the catalog-owned Factory Definitions persistence
// implementation from the exact serialization and filesystem ports used by Wire
// composition.
func NewPersistence(
	validator factorydefinitions.Validator,
	mapInput factorydefinitions.FactoryLayoutPayloadMapper,
	prepare factorydefinitions.FactoryLayoutPayloadPreparer,
	write func(string, *factorydefinitions.PreparedFactoryLayoutPayload, string) error,
	validate func(string) error,
	flatten factorydefinitions.FactoryLayoutFlattener,
	expand factorydefinitions.FactoryLayoutExpander,
	writeCurrent factorydefinitions.CurrentFactoryPointerWriter,
	fileSystem factorydefinitions.PersistenceFileSystem,
	requireDefinitionDir factorydefinitions.DefinitionDirectoryRequirer,
	directories factorydefinitions.DirectoryReplacementStore,
) (factorydefinitions.Persistence, error) {
	return catalogwire.NewPersistence(
		validator,
		mapInput,
		prepare,
		write,
		validate,
		flatten,
		expand,
		writeCurrent,
		fileSystem,
		requireDefinitionDir,
		directories,
	)
}

// ResolveCurrent resolves the active Factory definition under rootDir using
// catalog-owned named-factory helpers and an injected path resolver.
func ResolveCurrent(
	paths factorydefinitions.NamedPathResolver,
	rootDir string,
) (string, error) {
	return catalogwire.ResolveCurrent(paths, rootDir)
}

// NewCatalogPathsService constructs the narrow, read-only Factory
// Definitions catalog/path capability from the exact already-composed
// effective-catalog, named-path, and current-directory collaborators used by
// canonical Wire composition. Named-Factory resolution calls the catalog
// subservice's own named-factory helper directly (the same one
// catalog.Service.ResolveNamedFactory delegates to) instead of constructing a
// second catalog.Service instance, and current-directory resolution reuses
// the exact factorydefinitions.CurrentFactoryDirectoryResolver value
// canonical Wire already constructs once for the rest of the graph.
// Construction performs no filesystem reads or writes. logger is the direct,
// required operation-logging abstraction; callers with no operation logging
// pass logging.NoopLogger{}.
func NewCatalogPathsService(
	listEffective factorydefinitions.EffectiveFactoryCatalogOperation,
	namedPaths factorydefinitions.NamedPathResolver,
	resolveCurrentDir factorydefinitions.CurrentFactoryDirectoryResolver,
	logger logging.Logger,
) (factorydefinitions.CatalogPathsService, error) {
	if namedPaths == nil {
		return nil, fmt.Errorf("named Factory path resolver is required")
	}
	resolveNamedFactory := func(
		ctx context.Context,
		request factorydefinitions.ResolveNamedFactoryRequest,
	) (factorydefinitions.ResolveNamedFactoryResult, error) {
		if err := ctx.Err(); err != nil {
			return factorydefinitions.ResolveNamedFactoryResult{}, err
		}
		resolution, err := catalogwire.ResolveNamedFactory(namedPaths, request.ProjectRoot, request.GlobalRoot, request.Name)
		if err != nil {
			return factorydefinitions.ResolveNamedFactoryResult{}, err
		}
		if resolution == nil {
			return factorydefinitions.ResolveNamedFactoryResult{}, factorydefinitions.ErrNamedFactoryNotFound
		}
		return factorydefinitions.ResolveNamedFactoryResult{Resolution: *resolution}, nil
	}
	return factorydefinitionsinternal.NewCatalogPathsService(
		listEffective,
		resolveNamedFactory,
		resolveCurrentDir,
		logger,
	)
}
