package wire

import (
	"context"
	"testing"

	"github.com/portpowered/infinite-you/internal/packagedfactorycatalog"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorydefinitionswire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/wire"
	systeminitialization "github.com/portpowered/infinite-you/pkg/services/system_initialization"
)

func bootstrapCompositionTestPersistence(t *testing.T) factorydefinitions.Persistence {
	t.Helper()

	edges := serviceedges.Edges{}
	portableFileSystem := provideFactoryDefinitionPortableFileSystem(edges)
	applier, err := factorydefinitionswire.NewPortableBundledFilesApplier(portableFileSystem)
	if err != nil {
		t.Fatalf("NewPortableBundledFilesApplier() error = %v", err)
	}
	starterWork, err := factorydefinitionswire.NewFactoryStarterWorkApplier(portableFileSystem)
	if err != nil {
		t.Fatalf("NewFactoryStarterWorkApplier() error = %v", err)
	}
	materializer := providePortableBundledFilesMaterializer(portableFileSystem)
	loadingFileSystem := provideFactoryDefinitionLoadingFileSystem(edges)
	namedPathFileSystem := provideFactoryDefinitionNamedPathFileSystem(edges)
	namedPaths, err := provideFactoryDefinitionNamedPathResolver(namedPathFileSystem)
	if err != nil {
		t.Fatalf("provideFactoryDefinitionNamedPathResolver() error = %v", err)
	}
	authoredReaderFileSystem := provideFactoryDefinitionAuthoredReaderFileSystem(edges)
	sourceResolver, err := providePortableBundledFileSourceResolver(portableFileSystem)
	if err != nil {
		t.Fatalf("providePortableBundledFileSourceResolver() error = %v", err)
	}
	inspection := provideFactoryDefinitionPortableBundledFileInspection(edges)
	toolPathLookup := provideFactoryDefinitionRequiredToolPathLookup(edges)
	toolVersionProbe := provideFactoryDefinitionRequiredToolVersionProbe(edges)
	requiredToolChecker, err := provideFactoryDefinitionRequiredToolChecker(toolPathLookup, toolVersionProbe)
	if err != nil {
		t.Fatalf("provideFactoryDefinitionRequiredToolChecker() error = %v", err)
	}
	loader := provideFactoryDefinitionLoader(
		applier,
		starterWork,
		materializer,
		loadingFileSystem,
		namedPaths,
		authoredReaderFileSystem,
		sourceResolver,
		inspection,
		requiredToolChecker,
	)
	pruneRemovedDocs, err := factorydefinitionswire.NewPortableBundledDocsPruner(portableFileSystem)
	if err != nil {
		t.Fatalf("NewPortableBundledDocsPruner() error = %v", err)
	}
	authoredWriterFileSystem := provideFactoryDefinitionAuthoredWriterFileSystem(edges)
	inboxEnsurer := provideFactoryDefinitionInputInboxSentinelEnsurer(authoredWriterFileSystem)
	persistenceFileSystem := provideFactoryDefinitionPersistenceFileSystem(edges)
	directoryReplacementStore := provideFactoryDefinitionDirectoryReplacementStore(edges)
	persistence, err := provideFactoryDefinitionPersistence(
		factorydefinitionswire.NewValidationOperations(nil, loader.LoadSourceFromCanonicalJSON),
		loader,
		pruneRemovedDocs,
		materializer,
		providePortableBundledFileWritesValidator(portableFileSystem),
		providePortableBundledFilesCopier(portableFileSystem),
		authoredWriterFileSystem,
		inboxEnsurer,
		persistenceFileSystem,
		namedPaths,
		directoryReplacementStore,
	)
	if err != nil {
		t.Fatalf("provideFactoryDefinitionPersistence() error = %v", err)
	}
	return persistence
}

func bootstrapCompositionGoalCatalog(t *testing.T) factorydefinitions.PackagedFactoryCatalogOperations {
	t.Helper()

	catalog, err := packagedfactorycatalog.LoadPublishedDefinitionCatalog()
	if err != nil {
		t.Fatalf("LoadPublishedDefinitionCatalog() error = %v", err)
	}
	definition, ok := catalog.Lookup("@you/goal")
	if !ok {
		t.Fatal("published catalog is missing @you/goal")
	}
	packagedCatalog, err := providePackagedFactoryCatalog([]factorydefinitions.PackagedDefinition{definition})
	if err != nil {
		t.Fatalf("providePackagedFactoryCatalog() error = %v", err)
	}
	return packagedCatalog
}

func TestProvideSystemInitializationServiceComposedInitializeCreatesThenSkipsPackagedFactories(t *testing.T) {
	t.Parallel()

	edges := serviceedges.Edges{}
	files := provideOperatorSettingsFileSystem(edges)
	decoder := provideOperatorConfigDecoder()
	encoder := provideOperatorConfigEncoder()
	loadOperatorConfig := provideOperatorConfigLoader(files, decoder)
	ensureOperatorBackendScope := provideOperatorBackendScopeEnsurer(
		files,
		provideOperatorSettingsCreateTemporaryFile(edges),
		provideOperatorSettingsIDGenerator(edges),
		decoder,
		encoder,
	)

	service, err := provideSystemInitializationService(
		bootstrapCompositionTestPersistence(t),
		platformfilesystem.Local{},
		bootstrapCompositionGoalCatalog(t),
		loadOperatorConfig,
		ensureOperatorBackendScope,
		provideSystemInitializationInspectPath(edges),
		provideSystemInitializationLegacyFactoryMigrationFileSystem(edges),
	)
	if err != nil {
		t.Fatalf("provideSystemInitializationService() error = %v", err)
	}

	homeDir := t.TempDir()
	first, err := service.Initialize(context.Background(), systeminitialization.Request{HomeDir: homeDir})
	if err != nil {
		t.Fatalf("first Initialize() error = %v", err)
	}
	if first.SystemConfigOutcome != systeminitialization.SystemConfigCreated {
		t.Fatalf("first system config outcome = %q, want created", first.SystemConfigOutcome)
	}
	if len(first.PackagedFactories) != 1 ||
		first.PackagedFactories[0].Name != "@you/goal" ||
		first.PackagedFactories[0].Outcome != systeminitialization.PackagedFactoryCreated {
		t.Fatalf("first packaged factories = %#v, want one created @you/goal", first.PackagedFactories)
	}

	second, err := service.Initialize(context.Background(), systeminitialization.Request{HomeDir: homeDir})
	if err != nil {
		t.Fatalf("second Initialize() error = %v", err)
	}
	if second.SystemConfigOutcome != systeminitialization.SystemConfigSkipped {
		t.Fatalf("second system config outcome = %q, want skipped", second.SystemConfigOutcome)
	}
	if len(second.PackagedFactories) != 1 ||
		second.PackagedFactories[0].Name != "@you/goal" ||
		second.PackagedFactories[0].Outcome != systeminitialization.PackagedFactorySkipped {
		t.Fatalf("second packaged factories = %#v, want one skipped @you/goal", second.PackagedFactories)
	}
}
