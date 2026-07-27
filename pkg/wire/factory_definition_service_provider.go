package wire

import (
	"context"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryloading "github.com/portpowered/infinite-you/pkg/services/factory_definitions/loading"
	factorydefinitionsservice "github.com/portpowered/infinite-you/pkg/services/factory_definitions/service"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factorysessionwire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/wire"
	wirefactorydefinitions "github.com/portpowered/infinite-you/pkg/wire/factorydefinitions"
)

func provideFactoryDefinitionsFactory(
	persistence factorydefinitions.Persistence,
	loader *factoryloading.Loader,
	applySupportedFiles factorydefinitions.PortableBundledFilesApplier,
	applyStarterWork factorydefinitions.FactoryStarterWorkApplier,
	namedPaths factorydefinitions.NamedPathResolver,
	namedFactoryCatalogFileSystem factorydefinitions.NamedFactoryCatalogFileSystem,
	clock factorydefinitions.Clock,
	versionFileSystem factorydefinitions.VersionFileSystem,
	listEffective factorydefinitions.EffectiveFactoryCatalogOperation,
	packagedCatalog factorydefinitions.PackagedFactoryCatalogOperations,
	packagedInstaller factorydefinitions.PackagedFactoryInstallationOperations,
) factorysessionwire.FactoryDefinitionsFactory {
	return func(
		sessionHost factorysessions.DefinitionHost,
		validator factorydefinitions.Validator,
	) factorydefinitions.Service {
		definitions := factorydefinitionsservice.New(
			sessionHost,
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
			wirefactorydefinitions.PortableFactoryConfigPreparer(
				applySupportedFiles,
				applyStarterWork,
			),
			wirefactorydefinitions.FactorySnapshotCapturer(),
			persistence.ReplaceFactoryLayout,
			namedPaths,
			namedFactoryCatalogFileSystem,
			packagedCatalog,
			packagedInstaller,
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
