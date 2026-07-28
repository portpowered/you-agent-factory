package wire

import (
	"fmt"

	contracts "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryauthoredlayout "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/authoring_layout/authoredlayout"
	factoryloading "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation/loading"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation/impl"
	factorymapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
	authoredmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig/authored"
)

// Loader binds Factory Definitions loading to the selected authored and
// canonical representation adapters.
func Loader(
	applySupportedFiles contracts.PortableBundledFilesApplier,
	applyStarterWork contracts.FactoryStarterWorkApplier,
	materializeFiles contracts.PortableBundledFilesMaterializer,
	loadingFileSystem contracts.LoadingFileSystem,
	namedPaths contracts.NamedPathResolver,
	fileSystem contracts.AuthoredLayoutReaderFileSystem,
	sourceResolver contracts.PortableBundledFileSourceResolver,
	inspectSource contracts.PortableBundledFileInspection,
	requiredToolChecker contracts.RequiredToolChecker,
) *factoryloading.Loader {
	mapper := factorymapping.NewFactoryConfigMapper()
	authoredReader := factoryauthoredlayout.NewReader(
		authoredmapping.ParseWorkerConfig,
		authoredmapping.ParseWorkstationConfig,
		authoredmapping.ParseAgentsBody,
		fileSystem,
	)
	return factoryloading.New(
		loadingFileSystem,
		factoryauthoredlayout.NewFactorySourceLoader(fileSystem),
		namedPaths.ResolveCurrentDir,
		LoadedFactorySourceFactory(),
		factorymapping.ExpandFactoryConfigForRuntimeLoad,
		mapper.Expand,
		factorymapping.MarshalCanonicalFactoryConfig,
		authoredmapping.AuthoredFactoryConfigForExpandedLayout,
		normalizeCanonicalFactory,
		func(
			factoryDir string,
			factoryConfig *contracts.FactoryConfig,
		) error {
			return factoryvalidation.
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
			factoryConfig *contracts.FactoryConfig,
		) error {
			return factoryvalidation.
				ValidatePortableBundledFilesForExpandOnPathWithSourceResolver(
					factoryDir,
					factoryConfig,
					sourceResolver,
					inspectSource,
				)
		},
		factoryvalidation.ValidateBlockingLoad,
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

// AuthoredFactorySourceLoader supplies the Factory Definitions-owned authored
// source resolver to transport operations without exposing its implementation.
func AuthoredFactorySourceLoader(
	fileSystem contracts.AuthoredLayoutReaderFileSystem,
) contracts.AuthoredFactorySourceLoader {
	return factoryauthoredlayout.NewFactorySourceLoader(fileSystem)
}

func normalizeCanonicalFactory(
	payload []byte,
) (*contracts.FactoryConfig, error) {
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
