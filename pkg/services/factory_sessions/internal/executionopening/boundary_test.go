package executionopening

import (
	"errors"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimeopening"
	durableexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/durable_execution"
	"go.uber.org/zap"
)

type executionOpeningFileSystemStub struct {
	workingDirectory string
	getwdError       error
	foundPath        string
	inspectedPaths   []string
}

func (stub *executionOpeningFileSystemStub) Getwd() (string, error) {
	return stub.workingDirectory, stub.getwdError
}

func (stub *executionOpeningFileSystemStub) Stat(path string) (fs.FileInfo, error) {
	stub.inspectedPaths = append(stub.inspectedPaths, path)
	if path == stub.foundPath {
		return nil, nil
	}
	return nil, fs.ErrNotExist
}

func TestDefaultFixtureCatalogResolutionDoesNotUseExecutionOpeningFileSystem(t *testing.T) {
	t.Parallel()

	paths := &executionOpeningFileSystemStub{
		workingDirectory: filepath.Join("workspace", "repository", "nested", "directory"),
	}
	factory := &Factory{paths: paths}

	projectRoot, err := factory.ResolveProjectRoot("")
	if err != nil || projectRoot != paths.workingDirectory {
		t.Fatalf("ResolveProjectRoot() = (%q, %v), want injected working directory %q", projectRoot, err, paths.workingDirectory)
	}
	fixtureCatalog, err := factory.resolveFixtureCatalog("")
	if err != nil || fixtureCatalog != "" {
		t.Fatalf("resolveFixtureCatalog() = (%q, %v), want empty embedded-catalog selector", fixtureCatalog, err)
	}
	if len(paths.inspectedPaths) != 0 {
		t.Fatalf("inspected paths = %#v, want no filesystem inspection", paths.inspectedPaths)
	}
}

func TestExplicitFixtureCatalogResolutionPreservesPath(t *testing.T) {
	t.Parallel()

	paths := &executionOpeningFileSystemStub{}
	factory := &Factory{paths: paths}
	fixtureCatalog, err := factory.resolveFixtureCatalog("custom/catalog.json")
	if err != nil || fixtureCatalog != "custom/catalog.json" {
		t.Fatalf("resolveFixtureCatalog() = (%q, %v), want explicit path", fixtureCatalog, err)
	}
	if len(paths.inspectedPaths) != 0 {
		t.Fatalf("inspected paths = %#v, want no filesystem inspection for explicit path", paths.inspectedPaths)
	}
}

func TestPathResolutionPropagatesInjectedWorkingDirectoryFailure(t *testing.T) {
	t.Parallel()

	want := errors.New("working directory unavailable")
	factory := &Factory{paths: &executionOpeningFileSystemStub{getwdError: want}}
	if _, err := factory.ResolveProjectRoot(""); !errors.Is(err, want) {
		t.Fatalf("ResolveProjectRoot() error = %v, want %v", err, want)
	}
	if fixtureCatalog, err := factory.resolveFixtureCatalog(""); err != nil || fixtureCatalog != "" {
		t.Fatalf("resolveFixtureCatalog() = (%q, %v), want embedded default without filesystem access", fixtureCatalog, err)
	}
}

func TestNewFactoryRequiresRuntimeArtifactRootResolver(t *testing.T) {
	t.Parallel()

	_, err := NewFactory(
		&runtimeopening.Factory{},
		workersRootExecutionProbe{},
		func(factorysessions.ExecutionProvider, string, string, string, WorkerExecution, factoryruntime.Clock) (durableexecution.Service, error) {
			return nil, nil
		},
		func(factoryruntime.Clock) factoryruntime.Clock { return nil },
		nil,
		platformfilesystem.Local{},
		zap.NewNop(),
	)
	if err == nil || !strings.Contains(err.Error(), "artifact root resolver is required") {
		t.Fatalf("NewFactory() error = %v, want missing artifact root resolver", err)
	}
}

func TestNewFactoryRequiresExecutionOpeningFileSystem(t *testing.T) {
	t.Parallel()

	_, err := NewFactory(
		&runtimeopening.Factory{},
		workersRootExecutionProbe{},
		func(factorysessions.ExecutionProvider, string, string, string, WorkerExecution, factoryruntime.Clock) (durableexecution.Service, error) {
			return nil, nil
		},
		func(factoryruntime.Clock) factoryruntime.Clock { return nil },
		func(string) factoryruntime.RuntimeArtifactRoots { return factoryruntime.RuntimeArtifactRoots{} },
		nil,
		zap.NewNop(),
	)
	if err == nil || !strings.Contains(err.Error(), "execution-opening filesystem is required") {
		t.Fatalf("NewFactory() error = %v, want missing execution-opening filesystem", err)
	}
}
