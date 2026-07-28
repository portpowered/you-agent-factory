package wire

import (
	"context"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorydefinitionsservice "github.com/portpowered/infinite-you/pkg/services/factory_definitions/service"
	factorydefinitionswire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/wire"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factorysessionwire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/wire"
)

func provideFactoryDefinitionsFactory(
	persistence factorydefinitions.Persistence,
	loader *factorydefinitionswire.DefinitionLoader,
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
) factorysessionwire.FactoryDefinitionsFactory {
	return func(
		sessionHost factorysessions.DefinitionHost,
		activationGateway factorydefinitions.DefinitionActivationGateway,
		validator factorydefinitions.Validator,
	) factorydefinitions.Service {
		definitions := factorydefinitionsservice.New(
			sessionHost,
			activationGateway,
			clock,
			versionFileSystem,
			validator,
			loader.LoadSourceFromCanonicalJSON,
			func(
				factoryDir string,
				workstationLoader factorydefinitions.WorkstationLoader,
			) (factorydefinitions.MutableLoadedFactorySource, error) {
				return loader.LoadRuntimeSource(factoryDir, workstationLoader)
			},
			namedPaths.ReadCurrentPointer,
			func(
				ctx context.Context,
				segment string,
				payload []byte,
				_ factorydefinitions.Validator,
			) (*factorydefinitions.PreparedFactoryLayoutPayload, error) {
				return persistence.PrepareFactoryLayout(ctx, segment, payload)
			},
			persistence.CreateNamedFactory,
			namedPaths.WriteCurrentPointer,
			factorydefinitionswire.PortableFactoryConfigPreparer(
				applySupportedFiles,
				applyStarterWork,
			),
			factorydefinitionswire.FactorySnapshotCapturer(),
			persistence.ReplaceFactoryLayout,
			namedPaths,
			namedFactoryCatalogFileSystem,
			packagedCatalog,
			packagedInstaller,
			requiredToolChecker,
			orchestratorValidator,
		)
		if definitions == nil {
			return nil
		}
		attached, err := factorydefinitionsservice.AttachEffectiveCatalog(
			definitions,
			listEffective,
		)
		if err != nil {
			return nil
		}
		return attached
	}
}
