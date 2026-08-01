package wire

import (
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryeffect "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/effects"
	catalogwire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/catalog/wire"
)

// NewPathResolver constructs the catalog-owned named-path resolver from the
// exact filesystem port used by Factory Definitions Wire composition.
func NewPathResolver(
	fileSystem factoryeffect.NamedPathFileSystem,
) (factoryeffect.NamedPathResolver, error) {
	return catalogwire.NewPathResolver(fileSystem)
}

// NewNamedFactoryCatalog constructs the catalog-owned named-factory catalog
// from the exact path and catalog-filesystem ports used by Wire composition.
func NewNamedFactoryCatalog(
	paths factoryeffect.NamedPathResolver,
	fileSystem factoryeffect.NamedFactoryCatalogFileSystem,
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
	fileSystem factoryeffect.PersistenceFileSystem,
	requireDefinitionDir factoryeffect.DefinitionDirectoryRequirer,
	directories factoryeffect.DirectoryReplacementStore,
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
	paths factoryeffect.NamedPathResolver,
	rootDir string,
) (string, error) {
	return catalogwire.ResolveCurrent(paths, rootDir)
}
