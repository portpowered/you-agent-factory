// Package wire constructs the Factory Definitions catalog subservice from
// exact injected effect ports.
package wire

import (
	"fmt"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	catalog "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/catalog"
	catalogservice "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/catalog/internal/service"
	catalognamedfactories "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/catalog/namedfactories"
	catalognamedpaths "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/catalog/namedpaths"
	catalogpersistence "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/catalog/persistence"
)

// NewService constructs the private catalog subservice from exact injected
// path and catalog-filesystem ports. Callers must supply the exact ports; this
// constructor does not select host filesystem/SQL/OS adapters or take
// Wire/root construction ownership.
func NewService(
	paths catalog.PathResolver,
	fileSystem catalog.CatalogFileSystem,
) (catalog.Service, error) {
	if paths == nil {
		return nil, fmt.Errorf("construct Factory Definitions catalog: named Factory path resolver is required")
	}
	if fileSystem == nil {
		return nil, fmt.Errorf("construct Factory Definitions catalog: named Factory catalog filesystem is required")
	}
	service := catalogservice.New(paths, fileSystem)
	if service == nil {
		return nil, fmt.Errorf("construct Factory Definitions catalog: implementation rejected its dependencies")
	}
	return service, nil
}

// NewPathResolver constructs the catalog-owned named-path resolver from the
// exact filesystem port used by catalog composition.
func NewPathResolver(
	fileSystem catalog.PathFileSystem,
) (catalog.PathResolver, error) {
	return catalognamedpaths.New(fileSystem)
}

// NewNamedFactoryCatalog constructs the catalog-owned named-factory catalog
// from the exact path and catalog-filesystem ports used by catalog composition.
func NewNamedFactoryCatalog(
	paths catalog.PathResolver,
	fileSystem catalog.CatalogFileSystem,
) (factorydefinitions.NamedFactoryCatalog, error) {
	return catalognamedfactories.New(paths, fileSystem)
}

// ResolveCurrent resolves the active Factory definition under rootDir using
// the catalog-owned named-factory helper and an injected path resolver.
func ResolveCurrent(
	paths catalog.PathResolver,
	rootDir string,
) (string, error) {
	return catalognamedfactories.ResolveCurrent(paths, rootDir)
}

// NewPersistence constructs the catalog-owned Factory Definitions persistence
// implementation from the exact serialization and filesystem ports used by owner
// composition.
func NewPersistence(
	validator factorydefinitions.Validator,
	mapInput factorydefinitions.FactoryLayoutPayloadMapper,
	prepare factorydefinitions.FactoryLayoutPayloadPreparer,
	write func(string, *factorydefinitions.PreparedFactoryLayoutPayload, string) error,
	validate func(string) error,
	flatten factorydefinitions.FactoryLayoutFlattener,
	expand factorydefinitions.FactoryLayoutExpander,
	writeCurrent catalog.CurrentFactoryPointerWriter,
	fileSystem catalog.PersistenceFileSystem,
	requireDefinitionDir catalog.DefinitionDirectoryRequirer,
	directories catalog.DirectoryReplacementStore,
) (factorydefinitions.Persistence, error) {
	return catalogpersistence.New(
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
