package wire

import (
	"context"
	"fmt"
	"io/fs"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorydefinitionsinternal "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal"
	catalogwire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/catalog/wire"
)

// NamedPathFileSystem is the exact filesystem effect used to resolve and
// persist Current Factory pointers and canonical named Factory paths. It is
// a pure construction-time effect port -- every production and test
// consumer reaches it only to build a factorydefinitions.NamedPathResolver
// through NewPathResolver, never as a standalone peer-service dependency --
// so it is published here rather than at the Factory Definitions service
// root, matching this package's convention of exposing focused construction
// providers rather than owner-level contracts.
type NamedPathFileSystem interface {
	ReadFile(string) ([]byte, error)
	Stat(string) (fs.FileInfo, error)
	MkdirAll(string, fs.FileMode) error
	WriteFile(string, []byte, fs.FileMode) error
}

// NewPathResolver constructs the catalog-owned named-path resolver from the
// exact filesystem port used by Factory Definitions Wire composition.
func NewPathResolver(
	fileSystem NamedPathFileSystem,
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
// effective-catalog, named-factory-catalog, and current-directory
// collaborators canonical Wire composes once for the rest of the graph.
// Named-Factory resolution calls namedFactoryCatalog.ResolveNamedFactoryAcrossRoots
// directly -- the same already-composed factorydefinitions.NamedFactoryCatalog
// singleton the deleted ACP adapter used -- instead of constructing a second
// operational path around it, and current-directory resolution reuses the
// exact factorydefinitions.CurrentFactoryDirectoryResolver value canonical
// Wire already constructs once for the rest of the graph. Construction
// performs no filesystem reads or writes. logger is the direct, required
// operation-logging abstraction; callers with no operation logging pass
// logging.NoopLogger{}.
func NewCatalogPathsService(
	listEffective factorydefinitions.EffectiveFactoryCatalogOperation,
	namedFactoryCatalog factorydefinitions.NamedFactoryCatalog,
	resolveCurrentDir factorydefinitions.CurrentFactoryDirectoryResolver,
	logger logging.Logger,
) (factorydefinitions.CatalogPathsService, error) {
	if namedFactoryCatalog == nil {
		return nil, fmt.Errorf("named Factory catalog is required")
	}
	resolveNamedFactory := func(
		ctx context.Context,
		request factorydefinitions.ResolveNamedFactoryRequest,
	) (factorydefinitions.ResolveNamedFactoryResult, error) {
		if err := ctx.Err(); err != nil {
			return factorydefinitions.ResolveNamedFactoryResult{}, err
		}
		resolution, err := namedFactoryCatalog.ResolveNamedFactoryAcrossRoots(request.ProjectRoot, request.GlobalRoot, request.Name)
		if err != nil {
			return factorydefinitions.ResolveNamedFactoryResult{}, err
		}
		if resolution == nil {
			return factorydefinitions.ResolveNamedFactoryResult{}, factorydefinitions.ErrNamedFactoryNotFound
		}
		return factorydefinitions.ResolveNamedFactoryResult{Resolution: *resolution}, nil
	}
	impl, err := factorydefinitionsinternal.NewCatalogPathsService(
		listEffective,
		resolveNamedFactory,
		resolveCurrentDir,
		logger,
	)
	if err != nil {
		return nil, err
	}
	return impl, nil
}
