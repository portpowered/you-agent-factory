// Package testcomposition owns Factory Definitions implementation
// construction used by Factory Definitions invariant tests. The Go internal
// boundary prevents peers, transports, and functional tests from treating it
// as an alternate application injector.
package testcomposition

import (
	"context"
	"fmt"
	factoryeffects "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/effects"
	"path/filepath"

	"github.com/portpowered/infinite-you/pkg/platform/portablefiles"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	authoringlayoutwire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/authoring_layout/wire"
	catalogwire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/catalog/wire"
	compilationwire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation/wire"
	snapshotswire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability/wire"
	validationwire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation/wire"
)

type Representation struct {
	DecodeFactory     func([]byte) (*factorydefinitions.FactoryConfig, error)
	DecodeAuthored    func([]byte) (*factorydefinitions.FactoryConfig, error)
	EncodeFactory     func(*factorydefinitions.FactoryConfig) ([]byte, error)
	NormalizeAuthored func(*factorydefinitions.FactoryConfig) (*factorydefinitions.FactoryConfig, error)
	ParseWorker       func([]byte, string) (*factorydefinitions.FactoryWorkerConfig, error)
	ParseWorkstation  func([]byte, string) (*factorydefinitions.FactoryWorkstationConfig, error)
	ParseBody         func([]byte, string) (string, error)
	RenderWorker      func(factorydefinitions.FactoryWorkerConfig) ([]byte, error)
	RenderWorkstation func(factorydefinitions.FactoryWorkstationConfig) ([]byte, error)
	RenderBody        func(string) []byte
	SafeLayoutSegment func(string, string) (string, error)
	SafePromptPath    func(string, string) (string, error)
	MapPersistence    factorydefinitions.FactoryLayoutPayloadMapper
}

type Composition struct {
	representation Representation
	fileSystem     portablefiles.FileSystem
	directories    factoryeffects.DirectoryReplacementStore
	effects        Effects
	requiredTools  factorydefinitions.RequiredToolChecker
}

// Effects is the exact set of Factory Definitions-owned filesystem roles
// required by the owner-local test composition. Outer _test.go edges select
// their policy-free Platform implementations explicitly.
type Effects struct {
	Loading             factoryeffects.LoadingFileSystem
	AuthoredReader      factoryeffects.AuthoredLayoutReaderFileSystem
	AuthoredWriter      factoryeffects.AuthoredLayoutWriterFileSystem
	Persistence         factoryeffects.PersistenceFileSystem
	NamedPaths          factoryeffects.NamedPathFileSystem
	NamedFactoryCatalog factoryeffects.NamedFactoryCatalogFileSystem
	InboxSentinels      factoryeffects.InputInboxSentinelEnsurer
}

func New(
	representation Representation,
	fileSystem portablefiles.FileSystem,
	directories factoryeffects.DirectoryReplacementStore,
	effects Effects,
	requiredTools ...factorydefinitions.RequiredToolChecker,
) Composition {
	var checker factorydefinitions.RequiredToolChecker
	if len(requiredTools) > 0 {
		checker = requiredTools[0]
	}
	return Composition{
		representation: representation,
		fileSystem:     fileSystem,
		directories:    directories,
		effects:        effects,
		requiredTools:  checker,
	}
}

func loadedFactorySourceFactory() factorydefinitions.LoadedFactorySourceFactory {
	return func(
		factoryDir string,
		factoryConfig *factorydefinitions.FactoryConfig,
		runtimeDefinitions factorydefinitions.RuntimeDefinitionLookup,
		replacements []factorydefinitions.PortableBundledFileReplacement,
	) (factorydefinitions.MutableLoadedFactorySource, error) {
		return compilationwire.LoadedFactorySourceFactory()(factoryDir, factoryConfig, runtimeDefinitions, replacements)
	}
}

func mustSourceResolver(fileSystem portablefiles.FileSystem) factorydefinitions.PortableBundledFileSourceResolver {
	resolver, err := snapshotswire.NewPortableBundledFileSourceResolver(fileSystem)
	if err != nil {
		panic(err)
	}
	return resolver
}

func (c Composition) Loader() *compilationwire.Loader {
	representation := c.representation
	applySupportedFiles, applyStarterWork, _ := PortableOperations(c.fileSystem)
	authoredReader := authoringlayoutwire.NewReader(
		representation.ParseWorker,
		representation.ParseWorkstation,
		representation.ParseBody,
		c.effects.AuthoredReader,
	)
	materializeFiles := func(targetDir string, config *factorydefinitions.FactoryConfig) ([]factorydefinitions.PortableBundledFileReplacement, error) {
		return snapshotswire.NewMaterializer(c.fileSystem)(targetDir, config)
	}
	return compilationwire.NewLoader(
		applySupportedFiles,
		applyStarterWork,
		materializeFiles,
		c.effects.Loading,
		mustNamedPaths(c.effects.NamedPaths).ResolveCurrentDir,
		authoringlayoutwire.NewFactorySourceLoader(c.effects.AuthoredReader),
		loadedFactorySourceFactory(),
		representation.DecodeFactory,
		representation.DecodeAuthored,
		representation.EncodeFactory,
		representation.NormalizeAuthored,
		func(payload []byte) (*factorydefinitions.FactoryConfig, error) {
			config, err := representation.DecodeAuthored(payload)
			if err != nil {
				return nil, fmt.Errorf("parse factory config: %w", err)
			}
			authored, err := representation.NormalizeAuthored(config)
			if err != nil {
				return nil, fmt.Errorf("normalize authored factory config: %w", err)
			}
			canonical, err := representation.EncodeFactory(authored)
			if err != nil {
				return nil, fmt.Errorf("normalize factory config: %w", err)
			}
			if len(canonical) == 0 {
				return nil, fmt.Errorf("normalize factory config: empty canonical representation")
			}
			return config, nil
		},
		func(factoryDir string, config *factorydefinitions.FactoryConfig) error {
			return validationwire.ValidatePortableResourceManifestOnPathWithSourceResolver(
				factoryDir, config, mustSourceResolver(c.fileSystem),
				c.fileSystem,
				c.requiredTools,
			)
		},
		func(factoryDir string, config *factorydefinitions.FactoryConfig) error {
			return validationwire.ValidatePortableBundledFilesForExpandOnPathWithSourceResolver(
				factoryDir, config, mustSourceResolver(c.fileSystem), c.fileSystem,
			)
		},
		validationwire.ValidateBlockingFactoryLoad,
		authoredReader.LoadWorkerConfig,
		authoredReader.LoadWorkstationConfig,
		authoredReader.LoadWorkerBody,
		authoredReader.LoadWorkstationBody,
		authoredReader.LoadWorkstationPromptTemplate,
		representation.SafeLayoutSegment,
		authoredReader.SplitRuntimeEntityDirExists,
	)
}

func (c Composition) LoadDirectory(factoryDir string, loader factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
	return c.Loader().LoadSourceFromFactoryDir(factoryDir, loader)
}

func (c Composition) LoadCurrent(factoryRoot string, loader factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
	return c.Loader().LoadRuntimeSource(factoryRoot, loader)
}

func (c Composition) LoadCanonicalJSON(payload []byte, loader factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
	return c.Loader().LoadSourceFromCanonicalJSON(payload, loader)
}

func (c Composition) FactoryLayoutFlattener() factorydefinitions.FactoryLayoutFlattener {
	return c.Loader().FlattenFactoryConfig
}

func (c Composition) FlattenFactoryConfig(path string) ([]byte, error) {
	return c.FactoryLayoutFlattener()(path)
}

func (c Composition) LoadedFactoryLoader(factoryDir string, loader factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
	return c.LoadDirectory(factoryDir, loader)
}

func (c Composition) LoadedCurrentFactory(factoryRoot string, loader factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
	return c.LoadCurrent(factoryRoot, loader)
}

func (c Composition) LoadedFactoryFromCanonicalJSON(payload []byte, loader factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
	return c.LoadCanonicalJSON(payload, loader)
}

func (c Composition) Persistence(
	validator factorydefinitions.Validator,
	mapInput factorydefinitions.FactoryLayoutPayloadMapper,
) factorydefinitions.Persistence {
	loader := c.Loader()
	_, _, pruneRemovedDocs := PortableOperations(c.fileSystem)
	representation := c.representation
	writer := authoringlayoutwire.NewWriter(
		representation.RenderWorker,
		representation.RenderWorkstation,
		representation.RenderBody,
		authoringlayoutwire.NewAgentsFileWriter(c.effects.AuthoredWriter),
		representation.SafeLayoutSegment,
		representation.SafePromptPath,
		c.effects.AuthoredWriter,
		c.effects.InboxSentinels,
	)
	materializeFiles := func(targetDir string, config *factorydefinitions.FactoryConfig) ([]factorydefinitions.PortableBundledFileReplacement, error) {
		return snapshotswire.NewMaterializer(c.fileSystem)(targetDir, config)
	}
	validateWrites := func(targetDir string, config *factorydefinitions.FactoryConfig) error {
		return snapshotswire.NewWritesValidator(c.fileSystem)(targetDir, config)
	}
	copySupportedFiles := func(sourceDir, targetDir string, config *factorydefinitions.FactoryConfig) error {
		return snapshotswire.NewPortableBundledFilesCopier(c.fileSystem)(sourceDir, targetDir, config)
	}
	persistence, err := catalogwire.NewPersistence(
		validator,
		mapInput,
		func(ctx context.Context, segment string, payload []byte, validator factorydefinitions.Validator) (*factorydefinitions.PreparedFactoryLayoutPayload, error) {
			return authoringlayoutwire.PrepareFactoryLayout(
				ctx, segment, payload, validator, representation.DecodeAuthored,
				representation.NormalizeAuthored, representation.EncodeFactory,
			)
		},
		func(targetDir string, prepared *factorydefinitions.PreparedFactoryLayoutPayload, sourcePath string) error {
			return writer.WritePrepared(
				targetDir, prepared, sourcePath,
				materializeFiles, pruneRemovedDocs,
			)
		},
		func(targetDir string) error {
			return loader.ValidateFactoryDirReadOnly(targetDir, nil, validateWrites)
		},
		c.FactoryLayoutFlattener(),
		func(path string) (string, factorydefinitions.LayoutExpansionReport, error) {
			targetDir, sourceDir, sourcePath, config, canonical, err := loader.PrepareFactoryLayoutExpansion(path)
			if err != nil {
				return "", factorydefinitions.LayoutExpansionReport{}, err
			}
			report, err := writer.Expand(
				targetDir, sourceDir, sourcePath, config, canonical,
				validateWrites, materializeFiles,
				copySupportedFiles,
			)
			return targetDir, report, err
		},
		mustNamedPaths(c.effects.NamedPaths).WriteCurrentPointer,
		c.effects.Persistence,
		mustNamedPaths(c.effects.NamedPaths).RequireDefinitionDir,
		c.directories,
	)
	if err != nil {
		panic(err)
	}
	return persistence
}

func mustNamedPaths(fileSystem factoryeffects.NamedPathFileSystem) factoryeffects.NamedPathResolver {
	resolver, err := catalogwire.NewPathResolver(fileSystem)
	if err != nil {
		panic(err)
	}
	return resolver
}

func (c Composition) NamedPaths() factoryeffects.NamedPathResolver {
	return mustNamedPaths(c.effects.NamedPaths)
}

func (c Composition) NamedFactoryCatalog() factorydefinitions.NamedFactoryCatalog {
	catalog, err := catalogwire.NewNamedFactoryCatalog(
		c.NamedPaths(),
		c.effects.NamedFactoryCatalog,
	)
	if err != nil {
		panic(err)
	}
	return catalog
}

// PortableOperations selects local filesystem effects for owner-local Factory
// Definitions tests. It is not an application composition entrypoint.
func PortableOperations(
	fileSystem portablefiles.FileSystem,
) (
	factorydefinitions.PortableBundledFilesApplier,
	factorydefinitions.FactoryStarterWorkApplier,
	factorydefinitions.PortableBundledDocsPruner,
) {
	applySupportedFiles, err := snapshotswire.NewPortableBundledFilesApplier(
		fileSystem,
	)
	if err != nil {
		panic(err)
	}
	applyStarterWork, err := snapshotswire.NewFactoryStarterWorkApplier(
		fileSystem,
	)
	if err != nil {
		panic(err)
	}
	pruneRemovedDocs, err := snapshotswire.NewPortableBundledDocsPruner(
		fileSystem,
	)
	if err != nil {
		panic(err)
	}
	return applySupportedFiles, applyStarterWork, pruneRemovedDocs
}

func (c Composition) ApplySupportedFiles(
	factoryDir string,
	factoryConfig *factorydefinitions.FactoryConfig,
	includeInlineContent bool,
	discoverUnlistedDocs bool,
) error {
	applySupportedFiles, _, _ := PortableOperations(c.fileSystem)
	return applySupportedFiles(
		factoryDir,
		factoryConfig,
		includeInlineContent,
		discoverUnlistedDocs,
	)
}

func (c Composition) ApplyStarterWork(
	factoryDir string,
	factoryConfig *factorydefinitions.FactoryConfig,
) error {
	_, applyStarterWork, _ := PortableOperations(c.fileSystem)
	return applyStarterWork(factoryDir, factoryConfig)
}

func (c Composition) PruneRemovedDocs(
	factoryDir string,
	factoryConfig *factorydefinitions.FactoryConfig,
) error {
	_, _, pruneRemovedDocs := PortableOperations(c.fileSystem)
	return pruneRemovedDocs(factoryDir, factoryConfig)
}

func (c Composition) ExpandLayout(path string) (string, factorydefinitions.LayoutExpansionReport, error) {
	return c.Persistence(nil, nil).ExpandFactoryLayout(path)
}

func (c Composition) MapFactoryJSONForPersistence(payload []byte) (factorydefinitions.DefinitionValidationRequest, error) {
	if c.representation.MapPersistence == nil {
		return factorydefinitions.DefinitionValidationRequest{}, fmt.Errorf("Factory Definitions persistence representation mapper is required")
	}
	return c.representation.MapPersistence(payload)
}

func (c Composition) FactoryDefinitionPersistenceWithValidator(validator factorydefinitions.Validator) factorydefinitions.Persistence {
	return c.Persistence(validator, c.MapFactoryJSONForPersistence)
}

func (c Composition) PersistNamedFactory(rootDir, name string, payload []byte, validator factorydefinitions.Validator) (string, error) {
	persistence := c.FactoryDefinitionPersistenceWithValidator(validator)
	prepared, err := persistence.PrepareFactoryLayout(context.Background(), name, payload)
	if err != nil {
		return "", err
	}
	return persistence.CreateNamedFactory(rootDir, name, prepared)
}

func (c Composition) PersistNamedFactoryUnchecked(rootDir, name string, payload []byte, validator factorydefinitions.Validator) (string, error) {
	persistence := c.Persistence(
		validator,
		func([]byte) (factorydefinitions.DefinitionValidationRequest, error) {
			return factorydefinitions.DefinitionValidationRequest{
				Profile: factorydefinitions.ValidationProfileTopology,
				Config:  &factorydefinitions.FactoryConfig{},
			}, nil
		},
	)
	prepared, err := persistence.PrepareFactoryLayout(context.Background(), name, payload)
	if err != nil {
		return "", err
	}
	return persistence.CreateNamedFactory(rootDir, name, prepared)
}

func (c Composition) ReplaceNamedFactory(rootDir, name string, payload []byte, validator factorydefinitions.Validator) (string, error) {
	persistence := c.FactoryDefinitionPersistenceWithValidator(validator)
	prepared, err := persistence.PrepareFactoryLayout(context.Background(), name, payload)
	if err != nil {
		return "", err
	}
	return persistence.ReplaceNamedFactory(rootDir, name, prepared)
}

func (c Composition) ReplaceFactoryLayout(targetDir string, payload []byte, validator factorydefinitions.Validator) (*factorydefinitions.FactorySplitLayoutReplaceResult, error) {
	persistence := c.FactoryDefinitionPersistenceWithValidator(validator)
	prepared, err := persistence.PrepareFactoryLayout(context.Background(), filepath.Base(targetDir), payload)
	if err != nil {
		return nil, err
	}
	return persistence.ReplaceFactoryLayout(targetDir, prepared)
}
