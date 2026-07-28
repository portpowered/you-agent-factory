package executionopening

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimeopening"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
)

type executionOpeningFileSystemStub struct {
	workingDirectory string
	getwdError       error
	foundPath        string
	inspectedPaths   []string
}

type executionOpeningCommandRunner struct{}

func (executionOpeningCommandRunner) Run(context.Context, workers.CommandRequest) (workers.CommandResult, error) {
	return workers.CommandResult{}, nil
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

func TestExecutionOpeningDoesNotDependOnInitializerOrConcreteTransports(t *testing.T) {
	t.Parallel()

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read executionopening package: %v", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") ||
			strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		source, err := os.ReadFile(entry.Name())
		if err != nil {
			t.Fatalf("read %s: %v", entry.Name(), err)
		}
		for _, forbidden := range []string{"pkg/initializer", "pkg/transports/mcp", "pkg/transports/http", "pkg/services/edges"} {
			if strings.Contains(string(source), forbidden) {
				t.Errorf("%s imports forbidden lifecycle or transport package %q", entry.Name(), forbidden)
			}
		}
	}
}

func TestExecutionOpeningDoesNotSelectAmbientPathEffects(t *testing.T) {
	t.Parallel()

	source, err := os.ReadFile("factory.go")
	if err != nil {
		t.Fatalf("read factory.go: %v", err)
	}
	for _, forbidden := range []string{"os.Getwd(", "os.Stat("} {
		if strings.Contains(string(source), forbidden) {
			t.Errorf("factory.go selects ambient path effect %q; inject ExecutionOpeningFileSystem", forbidden)
		}
	}
}

func TestPathResolutionUsesInjectedExecutionOpeningFileSystem(t *testing.T) {
	t.Parallel()

	root := filepath.Join("workspace", "repository")
	catalog := filepath.Join(root, filepath.FromSlash(factorysessions.ContractFixtureCatalogRelativePath))
	paths := &executionOpeningFileSystemStub{
		workingDirectory: filepath.Join(root, "nested", "directory"),
		foundPath:        catalog,
	}
	factory := &Factory{paths: paths}

	projectRoot, err := factory.ResolveProjectRoot("")
	if err != nil || projectRoot != paths.workingDirectory {
		t.Fatalf("ResolveProjectRoot() = (%q, %v), want injected working directory %q", projectRoot, err, paths.workingDirectory)
	}
	fixtureCatalog, err := factory.resolveFixtureCatalog("")
	if err != nil || fixtureCatalog != catalog {
		t.Fatalf("resolveFixtureCatalog() = (%q, %v), want injected catalog %q", fixtureCatalog, err, catalog)
	}
	if len(paths.inspectedPaths) < 2 || paths.inspectedPaths[len(paths.inspectedPaths)-1] != catalog {
		t.Fatalf("inspected paths = %#v, want ancestor search ending at %q", paths.inspectedPaths, catalog)
	}
}

func TestPathResolutionPropagatesInjectedWorkingDirectoryFailure(t *testing.T) {
	t.Parallel()

	want := errors.New("working directory unavailable")
	factory := &Factory{paths: &executionOpeningFileSystemStub{getwdError: want}}
	if _, err := factory.ResolveProjectRoot(""); !errors.Is(err, want) {
		t.Fatalf("ResolveProjectRoot() error = %v, want %v", err, want)
	}
	if _, err := factory.resolveFixtureCatalog(""); !errors.Is(err, want) {
		t.Fatalf("resolveFixtureCatalog() error = %v, want %v", err, want)
	}
}

func TestNewFactoryRequiresRuntimeArtifactRootResolver(t *testing.T) {
	t.Parallel()

	_, err := NewFactory(
		&runtimeopening.Factory{},
		runtimeopening.ExternalEffects{},
		executionOpeningCommandRunner{},
		&workers.MockPTYAllocator{},
		func(factorysessions.ExecutionProvider, string, string, string, workers.InvocationExecutor, factoryruntime.Clock) (factorysessions.ExecutionService, error) {
			return nil, nil
		},
		func(workers.CommandRunner, workers.PTYAllocator) (workers.InvocationExecutor, error) {
			return nil, nil
		},
		func(factoryruntime.Clock) factoryruntime.Clock { return nil },
		nil,
		func(platformprocess.CommandRunner) workers.CommandRunner { return nil },
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
		runtimeopening.ExternalEffects{},
		executionOpeningCommandRunner{},
		&workers.MockPTYAllocator{},
		func(factorysessions.ExecutionProvider, string, string, string, workers.InvocationExecutor, factoryruntime.Clock) (factorysessions.ExecutionService, error) {
			return nil, nil
		},
		func(workers.CommandRunner, workers.PTYAllocator) (workers.InvocationExecutor, error) {
			return nil, nil
		},
		func(factoryruntime.Clock) factoryruntime.Clock { return nil },
		func(string) factoryruntime.RuntimeArtifactRoots { return factoryruntime.RuntimeArtifactRoots{} },
		func(platformprocess.CommandRunner) workers.CommandRunner { return nil },
		nil,
		zap.NewNop(),
	)
	if err == nil || !strings.Contains(err.Error(), "execution-opening filesystem is required") {
		t.Fatalf("NewFactory() error = %v, want missing execution-opening filesystem", err)
	}
}
