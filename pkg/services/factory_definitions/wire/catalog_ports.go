package wire

import (
	"context"

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

// CatalogPathsService is Factory Definitions' narrow, stateless, read-only
// capability for available Factory target metadata and named/current Factory
// location resolution. It intentionally excludes authoring, compilation,
// validation, snapshot, distribution, runtime, and session operations; peers
// that need only catalog and path reads depend on this interface instead of
// the full factorydefinitions.Service root.
//
// This capability is published here, at the Wire composition boundary,
// rather than at the crowded factory_definitions root: the root package
// already carries pre-existing, deletion-only service-root-interface-count
// debt (docs/internal/baselines/package-structure-baseline.json), and that
// baseline is a strict ratchet -- new entries and count increases are
// rejected outright, so a new root-level bundling interface is not an option
// here. Peers depend on this type through their own locally declared narrow
// interface (see pkg/services/chat_sessions/internal/service's
// FactoryDefinitionsCatalogPaths), which this concrete implementation
// satisfies structurally without either side needing to import the other's
// wire subpackage.
type CatalogPathsService interface {
	ListEffectiveFactories(context.Context, factorydefinitions.ListEffectiveFactoriesRequest) (factorydefinitions.ListEffectiveFactoriesResult, error)
	ResolveNamedFactory(context.Context, factorydefinitions.ResolveNamedFactoryRequest) (factorydefinitions.ResolveNamedFactoryResult, error)
	ResolveCurrentFactoryLocation(context.Context, factorydefinitions.ResolveCurrentFactoryLocationRequest) (factorydefinitions.ResolveCurrentFactoryLocationResult, error)
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
) (CatalogPathsService, error) {
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
