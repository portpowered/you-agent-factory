package wire

import (
	"github.com/portpowered/infinite-you/pkg/platform/portablefiles"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorydefinitionswire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/wire"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factorysessionwire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/wire"
)

func provideFactoryDefinitionsFactory(
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
) factorysessionwire.FactoryDefinitionsFactory {
	return func(
		sessionHost factorysessions.DefinitionHost,
		activationGateway factorydefinitions.DefinitionActivationGateway,
		validator factorydefinitions.Validator,
	) factorydefinitions.Service {
		definitions, err := factorydefinitionswire.NewService(
			sessionHost,
			activationGateway,
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
		if err != nil {
			return nil
		}
		return definitions
	}
}
