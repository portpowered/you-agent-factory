package wire

import (
	"fmt"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryeffect "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/effects"
	authoringlayoutwire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/authoring_layout/wire"
	compilationwire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation/wire"
	wirevalidation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/wire/validation"
	factorymapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
	authoredmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig/authored"
)

// Loader is the compilation-owned Factory Definitions loader selected by owner
// wire composition. Root pkg/wire binds through this alias instead of public
// transitional loading shims.
type Loader = compilationwire.Loader

// NewLoader binds Factory Definitions loading to the selected authored and
// canonical representation adapters through compilation-owned loading.
func NewLoader(
	applySupportedFiles factorydefinitions.PortableBundledFilesApplier,
	applyStarterWork factorydefinitions.FactoryStarterWorkApplier,
	materializeFiles factorydefinitions.PortableBundledFilesMaterializer,
	loadingFileSystem factoryeffect.LoadingFileSystem,
	namedPaths factoryeffect.NamedPathResolver,
	fileSystem factoryeffect.AuthoredLayoutReaderFileSystem,
	sourceResolver factorydefinitions.PortableBundledFileSourceResolver,
	inspectSource factoryeffect.PortableBundledFileInspection,
	requiredToolChecker factorydefinitions.RequiredToolChecker,
) *Loader {
	mapper := factorymapping.NewFactoryConfigMapper()
	authoredReader := authoringlayoutwire.NewReader(
		authoredmapping.ParseWorkerConfig,
		authoredmapping.ParseWorkstationConfig,
		authoredmapping.ParseAgentsBody,
		fileSystem,
	)
	return compilationwire.NewLoader(
		applySupportedFiles,
		applyStarterWork,
		materializeFiles,
		loadingFileSystem,
		namedPaths.ResolveCurrentDir,
		authoringlayoutwire.NewFactorySourceLoader(fileSystem),
		LoadedFactorySourceFactory(),
		factorymapping.ExpandFactoryConfigForRuntimeLoad,
		mapper.Expand,
		factorymapping.MarshalCanonicalFactoryConfig,
		authoredmapping.AuthoredFactoryConfigForExpandedLayout,
		normalizeCanonicalFactory,
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
	lookPath factoryeffect.RequiredToolPathLookup,
	versionProbe factoryeffect.RequiredToolVersionProbe,
) (factorydefinitions.RequiredToolChecker, error) {
	return compilationwire.NewPathRequiredToolChecker(lookPath, versionProbe)
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
		return compilationwire.LoadedFactorySourceFactory()(factoryDir, factoryConfig, runtimeDefinitions, replacements)
	}
}

func normalizeCanonicalFactory(
	payload []byte,
) (*factorydefinitions.FactoryConfig, error) {
	mapper := factorymapping.NewFactoryConfigMapper()
	factoryConfig, err := mapper.Expand(payload)
	if err != nil {
		return nil, fmt.Errorf("parse factory config: %w", err)
	}
	authoredFactoryConfig, err := authoredmapping.AuthoredFactoryConfigForExpandedLayout(
		factoryConfig,
	)
	if err != nil {
		return nil, fmt.Errorf("normalize authored factory config: %w", err)
	}
	canonical, err := mapper.Flatten(authoredFactoryConfig)
	if err != nil {
		return nil, fmt.Errorf("normalize factory config: %w", err)
	}
	if len(canonical) == 0 {
		return nil, fmt.Errorf("normalize factory config: empty canonical representation")
	}
	return factoryConfig, nil
}
