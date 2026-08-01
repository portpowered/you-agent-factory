// Package wire constructs the Factory Definitions compilation subservice from
// exact injected load and encode ports.
package wire

import (
	"fmt"
	factoryeffects "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/effects"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	compilationservice "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation"
	compilationcanonical "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation/internal/canonical"
	compilationloadedsource "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation/internal/loadedsource"
	compilationloading "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation/internal/loading"
	compilationruntimeconfig "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation/internal/runtimeconfig"
	compilationserviceimpl "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation/internal/service"
)

type Loader = compilationloading.Loader

func EncodeFactoryPort() factorydefinitions.FactoryConfigJSONEncoder {
	return compilationcanonical.EncodeFactoryPort()
}

// NormalizeCanonicalWorkstationRuntime exposes the compilation-owned pure
// normalization callback to sibling owner wires without exposing the private
// runtimeconfig package.
func NormalizeCanonicalWorkstationRuntime(
	workstation *factorydefinitions.FactoryWorkstationConfig,
) {
	compilationruntimeconfig.NormalizeCanonicalWorkstationRuntime(workstation)
}

// MergeRuntimeConfig exposes the compilation-owned effective-definition merge
// callback to sibling owner wires without crossing a private package boundary.
func MergeRuntimeConfig(
	factoryConfig *factorydefinitions.FactoryConfig,
	runtimeDefinitions factorydefinitions.RuntimeDefinitionLookup,
) (*factorydefinitions.FactoryConfig, error) {
	return compilationruntimeconfig.Merge(factoryConfig, runtimeDefinitions)
}

func NewLoader(
	applySupportedFiles factorydefinitions.PortableBundledFilesApplier,
	applyStarterWork factorydefinitions.FactoryStarterWorkApplier,
	materializeFiles factorydefinitions.PortableBundledFilesMaterializer,
	loadingFileSystem factoryeffects.LoadingFileSystem,
	resolveCurrentDir factorydefinitions.CurrentFactoryDirectoryResolver,
	authoredSourceLoader factorydefinitions.AuthoredFactorySourceLoader,
	newSource factorydefinitions.LoadedFactorySourceFactory,
	decodeFactory func([]byte) (*factorydefinitions.FactoryConfig, error),
	decodeAuthoredLayout func([]byte) (*factorydefinitions.FactoryConfig, error),
	encodeFactory func(*factorydefinitions.FactoryConfig) ([]byte, error),
	normalizeAuthored func(*factorydefinitions.FactoryConfig) (*factorydefinitions.FactoryConfig, error),
	normalizeCanonical func([]byte) (*factorydefinitions.FactoryConfig, error),
	validateManifest func(string, *factorydefinitions.FactoryConfig) error,
	validateCanonicalFiles func(string, *factorydefinitions.FactoryConfig) error,
	validateBlockingLoad func(*factorydefinitions.FactoryConfig) factorydefinitions.ValidationResult,
	loadWorker func(string) (*factorydefinitions.FactoryWorkerConfig, error),
	loadWorkstation func(string) (*factorydefinitions.FactoryWorkstationConfig, error),
	loadWorkerBody func(string) (string, bool, error),
	loadWorkstationBody func(string) (string, bool, error),
	loadWorkstationPrompt func(string, string) (string, error),
	safeLayoutSegment func(string, string) (string, error),
	splitRuntimeEntityExists func(string) bool,
) *Loader {
	return compilationloading.New(
		loadingFileSystem,
		authoredSourceLoader,
		resolveCurrentDir,
		newSource,
		decodeFactory,
		decodeAuthoredLayout,
		encodeFactory,
		normalizeAuthored,
		normalizeCanonical,
		validateManifest,
		validateCanonicalFiles,
		validateBlockingLoad,
		applySupportedFiles,
		applyStarterWork,
		materializeFiles,
		loadWorker,
		loadWorkstation,
		loadWorkerBody,
		loadWorkstationBody,
		loadWorkstationPrompt,
		safeLayoutSegment,
		splitRuntimeEntityExists,
	)
}

func NewPathRequiredToolChecker(
	lookPath factoryeffects.RequiredToolPathLookup,
	versionProbe factoryeffects.RequiredToolVersionProbe,
) (factorydefinitions.RequiredToolChecker, error) {
	return compilationloading.NewPathRequiredToolChecker(lookPath, versionProbe)
}

func LoadedFactorySourceFactory() factorydefinitions.LoadedFactorySourceFactory {
	return func(
		factoryDir string,
		factoryConfig *factorydefinitions.FactoryConfig,
		runtimeDefinitions factorydefinitions.RuntimeDefinitionLookup,
		replacements []factorydefinitions.PortableBundledFileReplacement,
	) (factorydefinitions.MutableLoadedFactorySource, error) {
		return compilationloadedsource.New(factoryDir, factoryConfig, runtimeDefinitions, replacements)
	}
}

// NewService constructs the private compilation subservice from exact injected
// canonical/directory load and encode ports. Callers must supply Dependencies;
// this constructor does not select Runtime/Petri implementations or take
// Wire/root construction ownership.
func NewService(deps compilationservice.Dependencies) (compilationservice.Service, error) {
	if deps.LoadCanonical == nil {
		return nil, fmt.Errorf("construct Factory Definitions compilation: canonical Factory loader is required")
	}
	if deps.LoadFromFactoryDir == nil {
		return nil, fmt.Errorf("construct Factory Definitions compilation: authored Factory directory loader is required")
	}
	if deps.EncodeFactory == nil {
		return nil, fmt.Errorf("construct Factory Definitions compilation: canonical Factory encoder is required")
	}
	service := compilationserviceimpl.New(
		deps.LoadCanonical,
		deps.LoadFromFactoryDir,
		deps.EncodeFactory,
	)
	if service == nil {
		return nil, fmt.Errorf("construct Factory Definitions compilation: implementation rejected its dependencies")
	}
	return service, nil
}
