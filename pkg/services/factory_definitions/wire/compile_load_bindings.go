package wire

import (
	"fmt"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	internalauthoredlayout "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/authoring_layout/authoredlayout"
	compilationservice "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation"
	compilationcanonical "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation/canonical"
	compilationloadedsource "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation/loadedsource"
	compilationloading "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation/loading"
	compilationwire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation/wire"
	runtimesnapshot "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/runtime_snapshot"
	runtimesnapshotwire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/runtime_snapshot/wire"
	wirevalidation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/wire/validation"
	factorymapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
	authoredmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig/authored"
)

// Loader is the compilation-owned Factory Definitions loader selected by owner
// wire composition. Root pkg/wire binds through this alias instead of public
// transitional loading shims.
type Loader = compilationloading.Loader

// NewLoader binds Factory Definitions loading to the selected authored and
// canonical representation adapters through compilation-owned loading.
func NewLoader(
	applySupportedFiles factorydefinitions.PortableBundledFilesApplier,
	applyStarterWork factorydefinitions.FactoryStarterWorkApplier,
	materializeFiles factorydefinitions.PortableBundledFilesMaterializer,
	loadingFileSystem factorydefinitions.LoadingFileSystem,
	namedPaths factorydefinitions.NamedPathResolver,
	authoredReader *AuthoredLayoutReader,
	loadAuthoredSource factorydefinitions.AuthoredFactorySourceLoader,
	newSource factorydefinitions.LoadedFactorySourceFactory,
	sourceResolver factorydefinitions.PortableBundledFileSourceResolver,
	inspectSource factorydefinitions.PortableBundledFileInspection,
	requiredToolChecker factorydefinitions.RequiredToolChecker,
	decodeFactory factorydefinitions.FactoryConfigJSONDecoder,
	normalizeCanonical CanonicalFactoryNormalizer,
) *Loader {
	return compilationloading.New(
		loadingFileSystem,
		loadAuthoredSource,
		namedPaths.ResolveCurrentDir,
		newSource,
		factorymapping.ExpandFactoryConfigForRuntimeLoad,
		decodeFactory,
		factorymapping.MarshalCanonicalFactoryConfig,
		authoredmapping.AuthoredFactoryConfigForExpandedLayout,
		normalizeCanonical,
		func(
			factoryDir string,
			factoryConfig *factorydefinitions.FactoryConfig,
		) error {
			return wirevalidation.
				ValidatePortableResourceManifestOnPathWithSourceResolver(
					factoryDir,
					factoryConfig,
					sourceResolver,
					inspectSource,
					requiredToolChecker,
				)
		},
		func(
			factoryDir string,
			factoryConfig *factorydefinitions.FactoryConfig,
		) error {
			return wirevalidation.
				ValidatePortableBundledFilesForExpandOnPathWithSourceResolver(
					factoryDir,
					factoryConfig,
					sourceResolver,
					inspectSource,
				)
		},
		wirevalidation.ValidateBlockingFactoryLoad,
		applySupportedFiles,
		applyStarterWork,
		materializeFiles,
		authoredReader.LoadWorkerConfig,
		authoredReader.LoadWorkstationConfig,
		authoredReader.LoadWorkerBody,
		authoredReader.LoadWorkstationBody,
		authoredReader.LoadWorkstationPromptTemplate,
		authoredmapping.SafeFactoryLayoutSegment,
		authoredReader.SplitRuntimeEntityDirExists,
	)
}

// NewPathRequiredToolChecker constructs the Factory Definitions external-tool
// checker through compilation-owned loading.
func NewPathRequiredToolChecker(
	lookPath factorydefinitions.RequiredToolPathLookup,
	versionProbe factorydefinitions.RequiredToolVersionProbe,
) (factorydefinitions.RequiredToolChecker, error) {
	return compilationloading.NewPathRequiredToolChecker(lookPath, versionProbe)
}

// LoadedFactorySourceFactory binds the compilation-owned effective-source
// implementation to the Factory Definitions root constructor contract.
func LoadedFactorySourceFactory() factorydefinitions.LoadedFactorySourceFactory {
	return func(
		factoryDir string,
		factoryConfig *factorydefinitions.FactoryConfig,
		runtimeDefinitions factorydefinitions.RuntimeDefinitionLookup,
		replacements []factorydefinitions.PortableBundledFileReplacement,
	) (factorydefinitions.MutableLoadedFactorySource, error) {
		return compilationloadedsource.New(
			factoryDir,
			factoryConfig,
			runtimeDefinitions,
			replacements,
		)
	}
}

// AuthoredLayoutReader is the completed split-layout reader supplied to loading.
type AuthoredLayoutReader = internalauthoredlayout.Reader

// NewAuthoredLayoutReader constructs only the split-layout reader.
func NewAuthoredLayoutReader(fileSystem factorydefinitions.AuthoredLayoutReaderFileSystem) *AuthoredLayoutReader {
	return internalauthoredlayout.NewReader(authoredmapping.ParseWorkerConfig,
		authoredmapping.ParseWorkstationConfig, authoredmapping.ParseAgentsBody, fileSystem)
}

// CanonicalFactoryNormalizer preserves load-specific error context.
type CanonicalFactoryNormalizer func([]byte) (*factorydefinitions.FactoryConfig, error)

// CanonicalFactoryNormalizerFromMapper binds one mapper before any load operation.
func CanonicalFactoryNormalizerFromMapper() CanonicalFactoryNormalizer {
	mapper := factorymapping.NewFactoryConfigMapper()
	return func(payload []byte) (*factorydefinitions.FactoryConfig, error) {
		config, err := mapper.Expand(payload)
		if err != nil {
			return nil, fmt.Errorf("parse factory config: %w", err)
		}
		return config, nil
	}
}

// Compilation is the completed private owner supplied to the Definitions root.
type Compilation = compilationservice.Service

// FactoryConfigJSONEncoder supplies the canonical serialization port.
func FactoryConfigJSONEncoder() factorydefinitions.FactoryConfigJSONEncoder {
	return compilationcanonical.EncodeFactoryPort()
}

// NewCompilationService constructs only the inert compilation owner.
func NewCompilationService(
	loadCanonical factorydefinitions.CanonicalFactoryJSONLoader,
	loadFromFactoryDir factorydefinitions.LoadedFactoryLoader,
	encodeFactory factorydefinitions.FactoryConfigJSONEncoder,
) Compilation {
	return compilationwire.NewService(loadCanonical, loadFromFactoryDir, encodeFactory)
}

// RuntimeSnapshot is the completed source resolution owner supplied to Definitions.
type RuntimeSnapshot = runtimesnapshot.Service

// NewRuntimeSnapshot constructs an inert resolver with the exact source and query ports.
func NewRuntimeSnapshot(
	loadCanonical factorydefinitions.CanonicalFactoryJSONLoader,
	loadFactory factorydefinitions.LoadedFactoryLoader,
	workstationLoader func() factorydefinitions.WorkstationLoader,
	readFile factorydefinitions.FileReader,
) RuntimeSnapshot {
	return runtimesnapshotwire.NewService(loadCanonical, loadFactory, workstationLoader, readFile)
}
