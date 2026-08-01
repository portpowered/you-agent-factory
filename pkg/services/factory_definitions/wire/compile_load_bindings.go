package wire

import (
	"fmt"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryeffect "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/effects"
	authoringlayoutwire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/authoring_layout/wire"
	compilationwire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation/wire"
	wirevalidation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/wire/validation"
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
	representation Representation,
) *Loader {
	authoredReader := authoringlayoutwire.NewReader(
		representation.ParseWorker,
		representation.ParseWorkstation,
		representation.ParseAgentsBody,
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
		representation.DecodeFactoryRuntime,
		representation.DecodeFactory,
		representation.EncodeFactory,
		representation.NormalizeAuthored,
		func(payload []byte) (*factorydefinitions.FactoryConfig, error) {
			return normalizeCanonicalFactory(representation, payload)
		},
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
		representation.SafeLayoutSegment,
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
	representation Representation,
	payload []byte,
) (*factorydefinitions.FactoryConfig, error) {
	factoryConfig, err := representation.DecodeFactory(payload)
	if err != nil {
		return nil, fmt.Errorf("parse factory config: %w", err)
	}
	authoredFactoryConfig, err := representation.NormalizeAuthored(
		factoryConfig,
	)
	if err != nil {
		return nil, fmt.Errorf("normalize authored factory config: %w", err)
	}
	canonical, err := representation.EncodeFactory(authoredFactoryConfig)
	if err != nil {
		return nil, fmt.Errorf("normalize factory config: %w", err)
	}
	if len(canonical) == 0 {
		return nil, fmt.Errorf("normalize factory config: empty canonical representation")
	}
	return factoryConfig, nil
}
