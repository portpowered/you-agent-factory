package wire

import (
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorydefinitionsinternal "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal"
	catalog "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/catalog"
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
// Definitions catalog/path capability from the exact effective-catalog
// operation and catalog-owned path/filesystem ports used by Wire
// composition. It reuses the same private catalog collaborator the root
// Service's ResolveNamedFactory delegates to, so results are identical, and
// performs no filesystem reads or writes at construction time. logger is the
// direct, required operation-logging abstraction; callers with no operation
// logging pass logging.NoopLogger{}.
func NewCatalogPathsService(
	listEffective factorydefinitions.EffectiveFactoryCatalogOperation,
	namedPaths factorydefinitions.NamedPathResolver,
	namedFactoryCatalogFileSystem factorydefinitions.NamedFactoryCatalogFileSystem,
	logger logging.Logger,
) (factorydefinitions.CatalogPathsService, error) {
	catalogService, err := catalogwire.NewService(catalog.Dependencies{
		Paths:      namedPaths,
		FileSystem: namedFactoryCatalogFileSystem,
	})
	if err != nil {
		return nil, err
	}
	resolveCurrentDir := func(rootDir string) (string, error) {
		return catalogwire.ResolveCurrent(namedPaths, rootDir)
	}
	return factorydefinitionsinternal.NewCatalogPathsService(
		listEffective,
		catalogService.ResolveNamedFactory,
		resolveCurrentDir,
		logger,
	)
}
