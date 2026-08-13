package wire

import (
	"fmt"

	"github.com/portpowered/infinite-you/pkg/platform/portablefiles"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorydefinitionswire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/wire"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
)

func provideFactoryDefinitionsRuntimeRouter() factorysessions.DefinitionRuntimeRouter {
	return factorysessions.NewDefinitionRuntimeRouter()
}

func provideFactoryDefinitionsRoot(
	router factorysessions.DefinitionRuntimeRouter,
	validator factorydefinitions.Validator,
	persistence factorydefinitions.Persistence,
	loader *factorydefinitionswire.Loader,
	applySupportedFiles factorydefinitions.PortableBundledFilesApplier,
	applyStarterWork factorydefinitions.FactoryStarterWorkApplier,
	namedPaths factorydefinitions.NamedPathResolver,
	namedFactoryCatalogFileSystem factorydefinitions.NamedFactoryCatalogFileSystem,
	clock factorydefinitions.Clock,
	versionFileSystem factorydefinitions.VersionFileSystem,
	listEffective factorydefinitions.EffectiveFactoryCatalogOperation,
	packagedCatalog factorydefinitions.PackagedFactoryCatalogOperations,
	packagedInstaller factorydefinitions.PackagedFactoryInstallationOperations,
	requiredToolChecker factorydefinitions.RequiredToolChecker,
	orchestratorValidator factorydefinitions.OrchestratorDefinitionValidator,
	portableFileSystem portablefiles.FileSystem,
	directoryReplacementStore factorydefinitions.DirectoryReplacementStore,
) (factorydefinitions.Service, error) {
	if router == nil {
		return nil, fmt.Errorf("construct Factory Definitions: runtime router is required")
	}
	return factorydefinitionswire.NewService(
		router.Host(),
		router.ActivationGateway(),
		validator,
		persistence,
		loader,
		applySupportedFiles,
		applyStarterWork,
		namedPaths,
		namedFactoryCatalogFileSystem,
		clock,
		versionFileSystem,
		listEffective,
		packagedCatalog,
		packagedInstaller,
		requiredToolChecker,
		orchestratorValidator,
		portableFileSystem,
		directoryReplacementStore,
	)
}
