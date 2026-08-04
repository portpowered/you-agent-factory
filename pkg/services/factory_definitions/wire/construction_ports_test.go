package wire_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorydefinitionswire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/wire"
)

func TestPublishedConstructionPorts_ExposeRootCompositionHelpers(t *testing.T) {
	t.Parallel()

	discovery, err := factorydefinitionswire.NewEffectiveCatalogDiscovery(
		func(string) ([]factorydefinitions.NamedFactoryListEntry, error) {
			return nil, nil
		},
		func(string) ([]byte, error) { return nil, nil },
		nil,
	)
	if err != nil {
		t.Fatalf("NewEffectiveCatalogDiscovery() error = %v", err)
	}
	if discovery.ListRoot == nil || discovery.ListPackaged == nil {
		t.Fatal("NewEffectiveCatalogDiscovery() returned incomplete discovery ports")
	}

	normalize := factorydefinitionswire.EffectiveFactoryDefinitionNormalizerFromMapper()
	catalog, err := factorydefinitionswire.NewEffectiveCatalog(discovery, normalize)
	if err != nil {
		t.Fatalf("NewEffectiveCatalog() error = %v", err)
	}
	if catalog == nil {
		t.Fatal("NewEffectiveCatalog() returned nil operation")
	}

	service, err := factorydefinitionswire.NewEffectiveCatalogService(catalog)
	if err != nil {
		t.Fatalf("NewEffectiveCatalogService() error = %v", err)
	}
	if service == nil {
		t.Fatal("NewEffectiveCatalogService() returned nil service")
	}

	packagedCatalog, err := factorydefinitionswire.NewPackagedFactoryCatalog(nil)
	if err != nil {
		t.Fatalf("NewPackagedFactoryCatalog() error = %v", err)
	}
	if packagedCatalog.List == nil || packagedCatalog.Resolve == nil {
		t.Fatal("NewPackagedFactoryCatalog() returned incomplete catalog operations")
	}
}

func noopListEffective(
	context.Context,
	factorydefinitions.ListEffectiveFactoriesRequest,
) (factorydefinitions.ListEffectiveFactoriesResult, error) {
	return factorydefinitions.ListEffectiveFactoriesResult{}, nil
}

func writeFactoryJSON(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "factory.json"), []byte(`{}`), 0o644); err != nil {
		t.Fatalf("WriteFile(factory.json) in %s: %v", dir, err)
	}
}

// resolveCurrentDirFromPaths builds the same
// factorydefinitions.CurrentFactoryDirectoryResolver shape canonical Wire's
// provideCurrentFactoryDirectoryResolver constructs, from an already-injected
// path resolver, for direct use in focused Wire tests.
func resolveCurrentDirFromPaths(paths factorydefinitions.NamedPathResolver) factorydefinitions.CurrentFactoryDirectoryResolver {
	return func(rootDir string) (string, error) {
		return factorydefinitionswire.ResolveCurrent(paths, rootDir)
	}
}

func TestNewCatalogPathsServicePerformsNoIOAtConstruction(t *testing.T) {
	t.Parallel()

	paths := &recordingNamedPathResolver{}
	panicky := func(context.Context, factorydefinitions.ListEffectiveFactoriesRequest) (factorydefinitions.ListEffectiveFactoriesResult, error) {
		panic("listEffective invoked during inert construction")
	}
	panickyResolveCurrentDir := func(string) (string, error) {
		panic("resolveCurrentDir invoked during inert construction")
	}

	if _, err := factorydefinitionswire.NewCatalogPathsService(panicky, paths, panickyResolveCurrentDir, logging.NoopLogger{}); err != nil {
		t.Fatalf("NewCatalogPathsService: unexpected error: %v", err)
	}
	if paths.calls != 0 {
		t.Fatalf("named path resolver calls = %d, want 0 at construction", paths.calls)
	}
}

func TestCatalogPathsServiceResolveNamedFactoryPrefersProjectOverGlobal(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	projectRoot := filepath.Join(root, "project")
	globalRoot := filepath.Join(root, "global")
	writeFactoryJSON(t, filepath.Join(projectRoot, "shared"))
	writeFactoryJSON(t, filepath.Join(globalRoot, "shared"))

	fileSystem := platformfilesystem.Local{}
	paths, err := factorydefinitionswire.NewPathResolver(fileSystem)
	if err != nil {
		t.Fatalf("NewPathResolver: %v", err)
	}
	service, err := factorydefinitionswire.NewCatalogPathsService(noopListEffective, paths, resolveCurrentDirFromPaths(paths), logging.NoopLogger{})
	if err != nil {
		t.Fatalf("NewCatalogPathsService: %v", err)
	}

	result, err := service.ResolveNamedFactory(context.Background(), factorydefinitions.ResolveNamedFactoryRequest{
		ProjectRoot: projectRoot,
		GlobalRoot:  globalRoot,
		Name:        "shared",
	})
	if err != nil {
		t.Fatalf("ResolveNamedFactory: unexpected error: %v", err)
	}
	if result.Resolution.Source != factorydefinitions.NamedFactoryResolutionSourceProjectLocal {
		t.Fatalf("Resolution.Source = %v, want project-local", result.Resolution.Source)
	}
	if result.Resolution.PrecedenceDecision != factorydefinitions.NamedFactoryPrecedenceDecisionProjectOverGlobal {
		t.Fatalf("Resolution.PrecedenceDecision = %v, want project-over-global", result.Resolution.PrecedenceDecision)
	}
	if result.Resolution.FactoryDir != filepath.Join(projectRoot, "shared") {
		t.Fatalf("Resolution.FactoryDir = %q, want project-local location", result.Resolution.FactoryDir)
	}
}

func TestCatalogPathsServiceResolveNamedFactoryFallsBackToGlobal(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	projectRoot := filepath.Join(root, "project")
	globalRoot := filepath.Join(root, "global")
	writeFactoryJSON(t, filepath.Join(globalRoot, "only-global"))

	fileSystem := platformfilesystem.Local{}
	paths, err := factorydefinitionswire.NewPathResolver(fileSystem)
	if err != nil {
		t.Fatalf("NewPathResolver: %v", err)
	}
	service, err := factorydefinitionswire.NewCatalogPathsService(noopListEffective, paths, resolveCurrentDirFromPaths(paths), logging.NoopLogger{})
	if err != nil {
		t.Fatalf("NewCatalogPathsService: %v", err)
	}

	result, err := service.ResolveNamedFactory(context.Background(), factorydefinitions.ResolveNamedFactoryRequest{
		ProjectRoot: projectRoot,
		GlobalRoot:  globalRoot,
		Name:        "only-global",
	})
	if err != nil {
		t.Fatalf("ResolveNamedFactory: unexpected error: %v", err)
	}
	if result.Resolution.Source != factorydefinitions.NamedFactoryResolutionSourceGlobal {
		t.Fatalf("Resolution.Source = %v, want global", result.Resolution.Source)
	}
	if result.Resolution.FactoryDir != filepath.Join(globalRoot, "only-global") {
		t.Fatalf("Resolution.FactoryDir = %q, want global location", result.Resolution.FactoryDir)
	}
}

func TestCatalogPathsServiceResolveNamedFactoryRejectsInvalidName(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	fileSystem := platformfilesystem.Local{}
	paths, err := factorydefinitionswire.NewPathResolver(fileSystem)
	if err != nil {
		t.Fatalf("NewPathResolver: %v", err)
	}
	service, err := factorydefinitionswire.NewCatalogPathsService(noopListEffective, paths, resolveCurrentDirFromPaths(paths), logging.NoopLogger{})
	if err != nil {
		t.Fatalf("NewCatalogPathsService: %v", err)
	}

	_, err = service.ResolveNamedFactory(context.Background(), factorydefinitions.ResolveNamedFactoryRequest{
		ProjectRoot: filepath.Join(root, "project"),
		GlobalRoot:  filepath.Join(root, "global"),
		Name:        "../escape",
	})
	if !errors.Is(err, factorydefinitions.ErrInvalidNamedFactoryName) {
		t.Fatalf("ResolveNamedFactory error = %v, want errors.Is ErrInvalidNamedFactoryName", err)
	}
}

func TestCatalogPathsServiceResolveNamedFactoryReportsMissingDefinition(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	fileSystem := platformfilesystem.Local{}
	paths, err := factorydefinitionswire.NewPathResolver(fileSystem)
	if err != nil {
		t.Fatalf("NewPathResolver: %v", err)
	}
	service, err := factorydefinitionswire.NewCatalogPathsService(noopListEffective, paths, resolveCurrentDirFromPaths(paths), logging.NoopLogger{})
	if err != nil {
		t.Fatalf("NewCatalogPathsService: %v", err)
	}

	_, err = service.ResolveNamedFactory(context.Background(), factorydefinitions.ResolveNamedFactoryRequest{
		ProjectRoot: filepath.Join(root, "project"),
		GlobalRoot:  filepath.Join(root, "global"),
		Name:        "missing",
	})
	if !errors.Is(err, factorydefinitions.ErrNamedFactoryNotFound) {
		t.Fatalf("ResolveNamedFactory error = %v, want errors.Is ErrNamedFactoryNotFound", err)
	}
}

// TestCatalogPathsServiceResolveNamedFactoryHonorsCancelledContext proves the
// narrow capability preserves the pre-cancelled-context behavior of the ACP
// adapter it replaced: an already-cancelled context is rejected before any
// filesystem-backed named-path resolution runs, and no partial result is
// returned even though a matching project-local Factory exists on disk.
func TestCatalogPathsServiceResolveNamedFactoryHonorsCancelledContext(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	projectRoot := filepath.Join(root, "project")
	writeFactoryJSON(t, filepath.Join(projectRoot, "alpha"))

	fileSystem := platformfilesystem.Local{}
	paths, err := factorydefinitionswire.NewPathResolver(fileSystem)
	if err != nil {
		t.Fatalf("NewPathResolver: %v", err)
	}
	service, err := factorydefinitionswire.NewCatalogPathsService(noopListEffective, paths, resolveCurrentDirFromPaths(paths), logging.NoopLogger{})
	if err != nil {
		t.Fatalf("NewCatalogPathsService: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got, err := service.ResolveNamedFactory(ctx, factorydefinitions.ResolveNamedFactoryRequest{
		ProjectRoot: projectRoot,
		Name:        "alpha",
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ResolveNamedFactory error = %v, want errors.Is context.Canceled", err)
	}
	if got != (factorydefinitions.ResolveNamedFactoryResult{}) {
		t.Fatalf("ResolveNamedFactory returned a non-empty result on cancellation: %+v", got)
	}
}

func TestCatalogPathsServiceResolveCurrentFactoryLocationUsesCurrentPointer(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFactoryJSON(t, filepath.Join(root, "alpha"))

	fileSystem := platformfilesystem.Local{}
	paths, err := factorydefinitionswire.NewPathResolver(fileSystem)
	if err != nil {
		t.Fatalf("NewPathResolver: %v", err)
	}
	if err := paths.WriteCurrentPointer(root, "alpha"); err != nil {
		t.Fatalf("WriteCurrentPointer: %v", err)
	}
	service, err := factorydefinitionswire.NewCatalogPathsService(noopListEffective, paths, resolveCurrentDirFromPaths(paths), logging.NoopLogger{})
	if err != nil {
		t.Fatalf("NewCatalogPathsService: %v", err)
	}

	result, err := service.ResolveCurrentFactoryLocation(context.Background(), factorydefinitions.ResolveCurrentFactoryLocationRequest{
		RootDir: root,
	})
	if err != nil {
		t.Fatalf("ResolveCurrentFactoryLocation: unexpected error: %v", err)
	}
	if result.FactoryDir != filepath.Join(root, "alpha") {
		t.Fatalf("FactoryDir = %q, want %q", result.FactoryDir, filepath.Join(root, "alpha"))
	}
}

func TestCatalogPathsServiceResolveCurrentFactoryLocationFallsBackToDirectLayout(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFactoryJSON(t, root)

	fileSystem := platformfilesystem.Local{}
	paths, err := factorydefinitionswire.NewPathResolver(fileSystem)
	if err != nil {
		t.Fatalf("NewPathResolver: %v", err)
	}
	service, err := factorydefinitionswire.NewCatalogPathsService(noopListEffective, paths, resolveCurrentDirFromPaths(paths), logging.NoopLogger{})
	if err != nil {
		t.Fatalf("NewCatalogPathsService: %v", err)
	}

	result, err := service.ResolveCurrentFactoryLocation(context.Background(), factorydefinitions.ResolveCurrentFactoryLocationRequest{
		RootDir: root,
	})
	if err != nil {
		t.Fatalf("ResolveCurrentFactoryLocation: unexpected error: %v", err)
	}
	if result.FactoryDir != root {
		t.Fatalf("FactoryDir = %q, want the direct-layout root %q", result.FactoryDir, root)
	}
}

func TestCatalogPathsServiceResolveCurrentFactoryLocationReportsMissingLayout(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	fileSystem := platformfilesystem.Local{}
	paths, err := factorydefinitionswire.NewPathResolver(fileSystem)
	if err != nil {
		t.Fatalf("NewPathResolver: %v", err)
	}
	service, err := factorydefinitionswire.NewCatalogPathsService(noopListEffective, paths, resolveCurrentDirFromPaths(paths), logging.NoopLogger{})
	if err != nil {
		t.Fatalf("NewCatalogPathsService: %v", err)
	}

	_, err = service.ResolveCurrentFactoryLocation(context.Background(), factorydefinitions.ResolveCurrentFactoryLocationRequest{
		RootDir: root,
	})
	if !errors.Is(err, factorydefinitions.ErrFactoryLayoutNotFound) {
		t.Fatalf("ResolveCurrentFactoryLocation error = %v, want errors.Is ErrFactoryLayoutNotFound", err)
	}
}

func TestCatalogPathsServiceListEffectiveFactoriesForwardsResult(t *testing.T) {
	t.Parallel()

	want := factorydefinitions.ListEffectiveFactoriesResult{
		Entries: []factorydefinitions.EffectiveFactoryCatalogEntry{{Name: "alpha"}},
	}
	var gotRequest factorydefinitions.ListEffectiveFactoriesRequest
	listEffective := func(
		_ context.Context,
		request factorydefinitions.ListEffectiveFactoriesRequest,
	) (factorydefinitions.ListEffectiveFactoriesResult, error) {
		gotRequest = request
		return want, nil
	}

	root := t.TempDir()
	fileSystem := platformfilesystem.Local{}
	paths, err := factorydefinitionswire.NewPathResolver(fileSystem)
	if err != nil {
		t.Fatalf("NewPathResolver: %v", err)
	}
	service, err := factorydefinitionswire.NewCatalogPathsService(listEffective, paths, resolveCurrentDirFromPaths(paths), logging.NoopLogger{})
	if err != nil {
		t.Fatalf("NewCatalogPathsService: %v", err)
	}

	request := factorydefinitions.ListEffectiveFactoriesRequest{
		ProjectRoot: filepath.Join(root, "project"),
		GlobalRoot:  filepath.Join(root, "global"),
	}
	got, err := service.ListEffectiveFactories(context.Background(), request)
	if err != nil {
		t.Fatalf("ListEffectiveFactories: unexpected error: %v", err)
	}
	if gotRequest != request {
		t.Fatalf("ListEffectiveFactories forwarded request = %+v, want %+v", gotRequest, request)
	}
	if len(got.Entries) != 1 || got.Entries[0].Name != "alpha" {
		t.Fatalf("ListEffectiveFactories result = %+v, want the collaborator's result", got)
	}
}
