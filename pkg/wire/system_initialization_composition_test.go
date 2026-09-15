package wire

import (
	"context"
	"errors"
	"io/fs"
	"testing"

	"github.com/portpowered/infinite-you/internal/packagedfactorycatalog"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	managedchild "github.com/portpowered/infinite-you/pkg/platform/process/managedchild"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorydefinitionswire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/wire"
	systeminitialization "github.com/portpowered/infinite-you/pkg/services/system_initialization"
	managedbackend "github.com/portpowered/infinite-you/pkg/wire/internal/managedbackend"
)

func TestSystemInitializationInspectPathPreservesOverrideAndSelectsProcessDefault(t *testing.T) {
	t.Parallel()

	path := t.TempDir()
	info, err := provideSystemInitializationInspectPath(serviceedges.Edges{})(path)
	if err != nil {
		t.Fatalf("default inspect path: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("default inspect path IsDir() = false for %q", path)
	}

	inspected := ""
	override := func(path string) (fs.FileInfo, error) {
		inspected = path
		return nil, fs.ErrPermission
	}
	_, err = provideSystemInitializationInspectPath(serviceedges.Edges{
		SystemInitializationInspectPath: override,
	})("customer-path")
	if !errors.Is(err, fs.ErrPermission) || inspected != "customer-path" {
		t.Fatalf("override inspect path = (%q, %v), want customer-path and permission error", inspected, err)
	}
}

func TestModelsManagedProcessRetainsCleanupErrorOnce(t *testing.T) {
	t.Parallel()
	cleanupErr := errors.New("bounded cleanup failure")
	cleanupCalls := 0
	process := &modelsManagedProcess{
		cleanup: func() error {
			cleanupCalls++
			return cleanupErr
		},
		finished: make(chan struct{}),
	}
	close(process.finished)

	process.cleanupResources()
	process.cleanupResources()
	if cleanupCalls != 1 {
		t.Fatalf("cleanup calls = %d, want once", cleanupCalls)
	}
	if err := process.Wait(); !errors.Is(err, cleanupErr) {
		t.Fatalf("process wait error = %v, want retained cleanup error", err)
	}
}

func TestModelsProcessLauncherUsesIndependentChildLifetimeContext(t *testing.T) {
	t.Parallel()
	parentContext, cancel := context.WithCancel(context.Background())
	defer cancel()
	startErr := errors.New("controlled child start failure")
	var childContext context.Context
	launcher := modelsProcessLauncher{
		resolveLaunch: func(context.Context, serviceedges.HostProcessStartSpec) (managedbackend.ManagedBackendLaunch, error) {
			return managedbackend.ManagedBackendLaunch{Command: "controlled-managed-backend"}, nil
		},
		startProcess: func(ctx context.Context, _ managedchild.Spec) (*managedchild.Process, error) {
			childContext = ctx
			return nil, startErr
		},
	}
	if _, err := launcher.Start(parentContext, serviceedges.HostProcessStartSpec{}); !errors.Is(err, startErr) {
		t.Fatalf("modelsProcessLauncher.Start() error = %v, want controlled start error", err)
	}
	cancel()
	if childContext == nil {
		t.Fatal("modelsProcessLauncher did not supply a child context")
	}
	if childContext.Done() != nil || childContext.Err() != nil {
		t.Fatalf("child lifetime context = (done=%v, err=%v), want cancellation-independent context", childContext.Done(), childContext.Err())
	}
}

func bootstrapCompositionTestPersistence(t *testing.T) factorydefinitions.PackagedFactoryPersistence {
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

func TestProvideSystemInitializationServiceComposedInitializeCreatesThenReportsCurrentPackagedFactories(t *testing.T) {
	t.Parallel()

	edges := serviceedges.Edges{}
	files := provideOperatorSettingsFileSystem(edges)
	providersRoot, err := provideProvidersService(edges)
	if err != nil {
		t.Fatalf("provideProvidersService() error = %v", err)
	}
	decoder := provideOperatorConfigDecoder()
	encoder := provideOperatorConfigEncoder()
	settings, err := provideOperatorSettingsService(
		files,
		provideOperatorSettingsCreateTemporaryFile(edges),
		provideOperatorSettingsProviderCatalog(providersRoot),
		decoder,
		provideOperatorConfigDiagnosticsDecoder(),
		encoder,
		provideOperatorSettingsIDGenerator(edges),
		providersRoot,
		logging.NoopLogger{},
	)
	if err != nil {
		t.Fatalf("provideOperatorSettingsService() error = %v", err)
	}
	loadOperatorConfig := provideOperatorConfigLoader(settings)
	ensureOperatorBackendScope := provideOperatorBackendScopeEnsurer(settings)

	service, err := provideSystemInitializationService(
		bootstrapCompositionTestPersistence(t),
		platformfilesystem.Local{},
		provideFactoryDefinitionPackagedInstallationDirectoryCreator(edges),
		bootstrapCompositionGoalCatalog(t),
		loadOperatorConfig,
		ensureOperatorBackendScope,
		provideSystemInitializationInspectPath(edges),
		logging.NoopLogger{},
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
		second.PackagedFactories[0].Outcome != systeminitialization.PackagedFactoryCurrent {
		t.Fatalf("second packaged factories = %#v, want one current @you/goal", second.PackagedFactories)
	}
	assertBatchColdStartSystemInitializationCall(t)
}

// assertBatchColdStartSystemInitializationCall records the
// current pre-runtime operation's value and call count. It is the package
// witness for the profile-selected system-initialization seam; it makes no
// production deferral decision.
func assertBatchColdStartSystemInitializationCall(t *testing.T) {
	t.Helper()
	wantErr := errors.New("controlled system initialization failure")
	service := &batchColdStartSystemInitializationService{err: wantErr}
	operation := provideSystemInitializationOperation(service)

	err := operation(context.Background(), "isolated-home")
	if !errors.Is(err, wantErr) {
		t.Fatalf("system initialization error = %v, want %v", err, wantErr)
	}
	if service.calls != 1 || service.homeDir != "isolated-home" {
		t.Fatalf("system initialization calls/home = %d/%q, want 1/isolated-home", service.calls, service.homeDir)
	}
	t.Logf("system initialization calls=%d home=%q error=%v", service.calls, service.homeDir, err)
}

type batchColdStartSystemInitializationService struct {
	systeminitialization.Service
	calls   int
	homeDir string
	err     error
}

func (service *batchColdStartSystemInitializationService) Initialize(
	_ context.Context,
	request systeminitialization.Request,
) (systeminitialization.Result, error) {
	service.calls++
	service.homeDir = request.HomeDir
	return systeminitialization.Result{HomeDir: request.HomeDir}, service.err
}
