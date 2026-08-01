package wire

import (
	"github.com/portpowered/infinite-you/pkg/platform/portablefiles"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorydefinitionswire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/wire"
)

func provideFactoryDefinitionsService(
	validator factorydefinitions.Validator,
	definitionValidation factorydefinitions.DefinitionValidationOperation,
	effectiveDefinitionValidation factorydefinitions.EffectiveDefinitionValidationOperation,
	loader *factorydefinitionswire.Loader,
	applySupportedFiles factorydefinitions.PortableBundledFilesApplier,
	applyStarterWork factorydefinitions.FactoryStarterWorkApplier,
	namedPaths factorydefinitionswire.NamedPathResolver,
	namedFactoryCatalogFileSystem factorydefinitionswire.NamedFactoryCatalogFileSystem,
	listEffective factorydefinitions.EffectiveFactoryCatalogOperation,
	packagedCatalog factorydefinitions.PackagedFactoryCatalogOperations,
	packagedInstaller factorydefinitions.PackagedFactoryInstallationOperations,
	requiredToolChecker factorydefinitions.RequiredToolChecker,
	orchestratorValidator factorydefinitions.OrchestratorDefinitionValidator,
	scaffoldInitializer factorydefinitions.ScaffoldInitializer,
	portableFileSystem portablefiles.FileSystem,
	directoryReplacementStore factorydefinitionswire.DirectoryReplacementStore,
	invocationPolicyPorts factorydefinitionswire.InvocationPolicyPorts,
) (factorydefinitions.Service, error) {
	return factorydefinitionswire.NewService(
		factorydefinitionswire.Dependencies{
			Validator:                     validator,
			DefinitionValidation:          definitionValidation,
			EffectiveDefinitionValidation: effectiveDefinitionValidation,
			Loader:                        loader,
			ApplySupportedFiles:           applySupportedFiles,
			ApplyStarterWork:              applyStarterWork,
			NamedPaths:                    namedPaths,
			NamedFactoryCatalogFileSystem: namedFactoryCatalogFileSystem,
			ListEffective:                 listEffective,
			PackagedCatalog:               packagedCatalog,
			PackagedInstaller:             packagedInstaller,
			RequiredToolChecker:           requiredToolChecker,
			OrchestratorValidator:         orchestratorValidator,
			PortableFileSystem:            portableFileSystem,
			DirectoryReplacementStore:     directoryReplacementStore,
			InvocationPolicyPorts:         invocationPolicyPorts,
		},
		factorydefinitionswire.WithDistributionScaffold(
			scaffoldInitializer,
			factorydefinitionswire.LocalFactoryNameResolver(),
		),
	)
}
