package wire_test

import (
	"context"
	"errors"
	"fmt"
	"github.com/portpowered/infinite-you/pkg/platform/inboxgitkeep"
	"github.com/portpowered/infinite-you/pkg/platform/portablefiles"
	authoringlayout "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/authoring_layout"
	compilationloading "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation/loading"
	distributionservice "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/distribution"
	snapshotsportability "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability"
	snapshotsportabilitymaterialize "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability/materialize"
	internalportableconfig "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability/portableconfig"
	snapshotsportabilitywire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability/wire"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/transports/mapping/validationentry"
	factorymapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
	authoredmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig/authored"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorydefinitionswire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/wire"
)

// Existing behavioral fixtures explicitly supply the completed lifecycle owners.
func TestNewServiceConstructsLifecycleHostThroughInternalComposition(t *testing.T) {
	t.Parallel()

	ports := validConstructionPorts(t)
	service, err := newFixtureService(
		ports.sessionHost,
		ports.activationGateway,
		ports.validator,
		ports.persistence,
		ports.loader,
		compilationForLoader(ports.loader),
		validationForLoader(ports.loader, ports.requiredToolChecker, ports.orchestratorValidator),
		runtimeSnapshotForLoader(ports.loader, ports.sessionHost),
		ports.applySupportedFiles,
		ports.applyStarterWork,
		ports.namedPaths,
		factorydefinitionswire.NewCatalogService(ports.namedPaths, ports.namedFactoryCatalogFileSystem),
		ports.clock,
		ports.versionFileSystem,
		ports.listEffective,
		ports.packagedCatalog,
		ports.packagedInstaller,
		ports.requiredToolChecker,
		ports.orchestratorValidator,
		ports.portableFileSystem,
		ports.directoryReplacementStore,
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	var root factorydefinitions.Service = service
	if root == nil {
		t.Fatal("constructed value is not assignable to factorydefinitions.Service")
	}

	_, getCurrentErr := root.GetCurrentNamedFactory(context.Background())
	if !errors.Is(getCurrentErr, factorydefinitions.ErrCurrentFactoryNotFound) {
		t.Fatalf(
			"GetCurrentNamedFactory() error = %v, want %v",
			getCurrentErr,
			factorydefinitions.ErrCurrentFactoryNotFound,
		)
	}
}

func newFixtureService(
	sessionHost factorydefinitions.SessionHost,
	activationGateway factorydefinitions.DefinitionActivationGateway,
	validator factorydefinitions.Validator,
	persistence factorydefinitions.Persistence,
	loader *compilationloading.Loader,
	compilation factorydefinitionswire.Compilation,
	validationService factorydefinitionswire.Validation,
	runtimeSnapshot factorydefinitionswire.RuntimeSnapshot,
	applySupportedFiles factorydefinitions.PortableBundledFilesApplier,
	applyStarterWork factorydefinitions.FactoryStarterWorkApplier,
	namedPaths factorydefinitions.NamedPathResolver,
	catalogService factorydefinitionswire.Catalog,
	clock factorydefinitions.Clock,
	versionFileSystem factorydefinitions.VersionFileSystem,
	listEffective factorydefinitions.EffectiveFactoryCatalogOperation,
	packagedCatalog factorydefinitions.PackagedFactoryCatalogOperations,
	packagedInstaller factorydefinitions.PackagedFactoryInstallationOperations,
	requiredToolChecker factorydefinitions.RequiredToolChecker,
	orchestratorValidator factorydefinitions.OrchestratorDefinitionValidator,
	portableFileSystem portablefiles.FileSystem,
	directoryReplacementStore factorydefinitions.DirectoryReplacementStore,
	options ...fixtureCompositionOption,
) (factorydefinitions.Service, error) {
	preparePortableFactoryConfig, captureFactorySnapshot, snapshotsPortability, authoringLayout, err := composeFactoryDefinitionSupport(
		loader,
		applySupportedFiles,
		applyStarterWork,
		validator,
		namedPaths,
		portableFileSystem,
		directoryReplacementStore,
	)
	if err != nil {
		return nil, err
	}

	host, err := factorydefinitionswire.NewLifecycleHost(sessionHost, persistence, loader, namedPaths,
		preparePortableFactoryConfig, captureFactorySnapshot)
	if err != nil {
		return nil, err
	}
	var scaffold factorydefinitions.ScaffoldInitializer
	var resolveName distributionservice.ScaffoldFactoryNameResolver
	for _, option := range options {
		scaffold, resolveName = option.scaffold, option.resolveName
	}
	distribution, err := factorydefinitionswire.NewDistribution(packagedCatalog, packagedInstaller, scaffold, resolveName)
	if err != nil {
		return nil, err
	}
	definitions, err := factorydefinitionswire.NewService(host, activationGateway, catalogService, validationService,
		authoringLayout, distribution, runtimeSnapshot, compilation, versionFileSystem, listEffective, snapshotsPortability)
	if err != nil {
		return nil, err
	}

	if definitions == nil {
		return nil, fmt.Errorf("construct Factory Definitions: implementation rejected its dependencies")
	}

	return definitions, nil
}

func composeFactoryDefinitionSupport(
	loader *compilationloading.Loader,
	applySupportedFiles factorydefinitions.PortableBundledFilesApplier,
	applyStarterWork factorydefinitions.FactoryStarterWorkApplier,
	validator factorydefinitions.Validator,
	namedPaths factorydefinitions.NamedPathResolver,
	portableFileSystem portablefiles.FileSystem,
	directoryReplacementStore factorydefinitions.DirectoryReplacementStore,
) (
	factorydefinitions.PortableFactoryConfigPreparer,
	factorydefinitions.FactorySnapshotCapturer,
	snapshotsportability.Service,
	authoringlayout.Service,
	error,
) {
	preparePortableFactoryConfig := factorydefinitionswire.PortableFactoryConfigPreparer(applySupportedFiles, applyStarterWork)
	captureFactorySnapshot := factorydefinitionswire.FactorySnapshotCapturer()
	snapshotsPortability, err := snapshotsportabilitywire.NewService(
		loader.LoadSourceFromCanonicalJSON,
		factorydefinitionswire.LoadedFactorySnapshotCapturer(),
		preparePortableFactoryConfig,
		factorydefinitionswire.FactorySnapshotJSONDecoder(),
		snapshotsportabilitymaterialize.NewMaterializer(portableFileSystem),
		snapshotsportabilitymaterialize.NewWritesValidator(portableFileSystem),
	)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	authoringFS, err := resolveAuthoringLayoutFilesystem(portableFileSystem)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	pruneRemovedDocs, err := internalportableconfig.NewPortableBundledDocsPruner(portableFileSystem)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("construct Factory Definitions authoring layout: %w", err)
	}
	mapper := factorymapping.NewFactoryConfigMapper()
	writer := factorydefinitionswire.NewAuthoredLayoutWriter(authoringFS, inboxgitkeep.NewLocal(portableFileSystem), factorydefinitionswire.AuthoredAgentsFileWriter(authoringFS))
	materializeFiles := internalportableconfig.NewMaterializer(portableFileSystem)
	validateWrites := internalportableconfig.NewWritesValidator(portableFileSystem)
	authoringLayout, err := factorydefinitionswire.NewAuthoringLayout(
		validator, validationentry.MapFactoryJSONForPersistence, mapper.Expand,
		authoredmapping.AuthoredFactoryConfigForExpandedLayout, mapper.Flatten,
		factorydefinitionswire.PreparedAuthoredLayoutWriter(writer, materializeFiles, pruneRemovedDocs),
		factorydefinitionswire.AuthoredLayoutValidator(loader, validateWrites), loader.FlattenFactoryConfig,
		factorydefinitionswire.AuthoredLayoutExpander(loader, writer, validateWrites, materializeFiles, internalportableconfig.NewFilesCopier(portableFileSystem)),
		authoringFS, namedPaths.RequireDefinitionDir, directoryReplacementStore,
	)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("construct Factory Definitions authoring layout: %w", err)
	}
	return preparePortableFactoryConfig, captureFactorySnapshot, snapshotsPortability, authoringLayout, nil
}

type fixtureCompositionOption struct {
	scaffold    factorydefinitions.ScaffoldInitializer
	resolveName distributionservice.ScaffoldFactoryNameResolver
}

func withFixtureDistributionScaffold(scaffold factorydefinitions.ScaffoldInitializer,
	resolveName distributionservice.ScaffoldFactoryNameResolver) fixtureCompositionOption {
	return fixtureCompositionOption{scaffold: scaffold, resolveName: resolveName}
}

type authoringLayoutFilesystem interface {
	portablefiles.FileSystem
	factorydefinitions.AuthoredLayoutWriterFileSystem
	factorydefinitions.PersistenceFileSystem
}

func resolveAuthoringLayoutFilesystem(files portablefiles.FileSystem) (authoringLayoutFilesystem, error) {
	fs, ok := files.(authoringLayoutFilesystem)
	if !ok {
		return nil, fmt.Errorf("construct Factory Definitions: portable filesystem must support authoring_layout persistence")
	}
	return fs, nil
}
