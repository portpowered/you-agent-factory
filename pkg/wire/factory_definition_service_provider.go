package wire

import (
	"context"
	"fmt"

	"github.com/portpowered/infinite-you/pkg/platform/portablefiles"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorydefinitionswire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/wire"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryruntimewire "github.com/portpowered/infinite-you/pkg/services/factory_runtime/wire"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factorysessionwire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/wire"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func provideFactoryDefinitionsRuntimeRouter() *factorysessions.DefinitionRuntimeRouter {
	return factorysessionwire.NewDefinitionRuntimeRouter()
}

func provideFactoryDefinitionsRoot(
	router *factorysessions.DefinitionRuntimeRouter,
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

// provideFactoryRuntimeRootFactory composes the singular process-scoped
// Runtime root once. Factory Sessions supplies the explicit activation
// operation when it constructs its opening capability; no late delegate
// binder is exposed.
func provideFactoryRuntimeRootFactory(
	newID factoryruntime.IDGenerator,
	workflows factoryruntime.JavaScriptWorkflowDefinitions,
	clock factoryruntime.Clock,
) factorysessionwire.RuntimeRootFactory {
	return func(activation factoryruntime.RuntimeActivationOperation) (factorysessionwire.FactoryRuntimeRoot, error) {
		service, err := factoryruntimewire.NewService(
			newID,
			workflows,
			nil,
			clock,
			func(context.Context, workers.WorkstationDispatchRequest) error {
				return factoryruntime.ErrNotRunning
			},
			nil,
			activation,
		)
		return service, err
	}
}
