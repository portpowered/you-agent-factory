package factorydefinitions

import (
	contracts "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryauthoredlayout "github.com/portpowered/infinite-you/pkg/services/factory_definitions/authoredlayout"
	factorydefinitionswire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/wire"
)

// Loader binds Factory Definitions loading to the selected authored and
// canonical representation adapters through owner wire composition.
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
) *factorydefinitionswire.Loader {
	return factorydefinitionswire.NewLoader(
		applySupportedFiles,
		applyStarterWork,
		materializeFiles,
		loadingFileSystem,
		namedPaths,
		fileSystem,
		sourceResolver,
		inspectSource,
		requiredToolChecker,
	)
}

// AuthoredFactorySourceLoader supplies the Factory Definitions-owned authored
// source resolver to transport operations without exposing its implementation.
func AuthoredFactorySourceLoader(
	fileSystem contracts.AuthoredLayoutReaderFileSystem,
) contracts.AuthoredFactorySourceLoader {
	return factoryauthoredlayout.NewFactorySourceLoader(fileSystem)
}
