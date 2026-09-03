package invocation

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimeopening"
	"go.uber.org/zap"
)

type invocationPresentationOwnerStub struct{}

func (invocationPresentationOwnerStub) RegisterDirectJavaScript(factorysessions.DirectJavaScriptRunScope) (factorysessions.OpeningScopeID, error) {
	return "", nil
}

func (invocationPresentationOwnerStub) DirectJavaScript(factorysessions.OpeningScopeID) (factorysessions.DirectJavaScriptRunScope, bool) {
	return factorysessions.DirectJavaScriptRunScope{}, false
}

func (invocationPresentationOwnerStub) RegisterStdio(factorysessions.StdioOpeningScope) (factorysessions.OpeningScopeID, error) {
	return "", nil
}

func (invocationPresentationOwnerStub) Stdio(factorysessions.OpeningScopeID) (factorysessions.StdioOpeningScope, bool) {
	return factorysessions.StdioOpeningScope{}, false
}

func (invocationPresentationOwnerStub) RegisterInvocationEvents(factorysessions.InvocationEventScope) (factorysessions.OpeningScopeID, error) {
	return "", nil
}

func (invocationPresentationOwnerStub) InvocationEvents(factorysessions.OpeningScopeID) (factorysessions.FactoryEventConsumer, bool) {
	return nil, false
}

func (invocationPresentationOwnerStub) StartFactoryEventBridge(context.Context, roles.FactoryEventReader, factorysessions.OpeningScopeID) (interface {
	Finish(context.Context, roles.FactoryEventReader, factorysessions.FactoryInvocationOutcome) error
}, error) {
	return nil, nil
}

func (invocationPresentationOwnerStub) Close(factorysessions.OpeningScopeID) {}

type workingDirectoryStub struct {
	dir string
	err error
}

func (s workingDirectoryStub) Getwd() (string, error) { return s.dir, s.err }

type artifactExporterStub struct{}

func (artifactExporterStub) ExportInvocationArtifact(string, string) error { return nil }

func TestResolveModelsInvokeFactoryDir_DelegatesDefaultLayoutToInjectedResolver(t *testing.T) {
	t.Parallel()

	var gotRoot string
	operation := &operation{
		workingDirectory: workingDirectoryStub{dir: filepath.Join("workspace", "project")},
		resolveCurrentDir: func(rootDir string) (string, error) {
			gotRoot = rootDir
			return filepath.Join(rootDir, "active"), nil
		},
	}
	resolved, err := operation.ResolveModelInvocationFactoryDir("")
	if err != nil {
		t.Fatalf("ResolveModelInvocationFactoryDir: %v", err)
	}
	wantRoot := filepath.Join("workspace", "project", factorydefinitions.FactoryDir)
	if gotRoot != wantRoot || resolved != filepath.Join(wantRoot, "active") {
		t.Fatalf("resolution = (%q, %q), want (%q, %q)", gotRoot, resolved, wantRoot, filepath.Join(wantRoot, "active"))
	}
}

func TestResolveModelsInvokeFactoryDir_PreservesExplicitDirectory(t *testing.T) {
	t.Parallel()

	resolverCalled := false
	operation := &operation{
		workingDirectory: workingDirectoryStub{dir: filepath.Join("workspace", "project")},
		resolveCurrentDir: func(string) (string, error) {
			resolverCalled = true
			return "unexpected", nil
		},
	}

	explicit := filepath.Join("workspace", "another-project", "factory")
	resolved, err := operation.ResolveModelInvocationFactoryDir(explicit)
	if err != nil {
		t.Fatalf("ResolveModelInvocationFactoryDir: %v", err)
	}
	if resolved != explicit {
		t.Fatalf("resolved directory = %q, want explicit %q", resolved, explicit)
	}
	if resolverCalled {
		t.Fatal("current-directory resolver was called for an explicit factory directory")
	}
}

func TestResolveModelsInvokeFactoryDir_FallsBackToLegacyRootLayout(t *testing.T) {
	t.Parallel()

	workingDirectory := filepath.Join("workspace", "project")
	legacyDirectory := filepath.Join(workingDirectory, "legacy-factory")
	var searched []string
	operation := &operation{
		workingDirectory: workingDirectoryStub{dir: workingDirectory},
		resolveCurrentDir: func(root string) (string, error) {
			searched = append(searched, root)
			if root == filepath.Join(workingDirectory, factorydefinitions.FactoryDir) {
				return "", factorydefinitions.ErrFactoryLayoutNotFound
			}
			if root == workingDirectory {
				return legacyDirectory, nil
			}
			return "", factorydefinitions.ErrFactoryLayoutNotFound
		},
	}

	resolved, err := operation.ResolveModelInvocationFactoryDir("")
	if err != nil {
		t.Fatalf("ResolveModelInvocationFactoryDir: %v", err)
	}
	if resolved != legacyDirectory {
		t.Fatalf("resolved directory = %q, want legacy %q", resolved, legacyDirectory)
	}
	wantSearched := []string{filepath.Join(workingDirectory, factorydefinitions.FactoryDir), workingDirectory}
	if !reflect.DeepEqual(searched, wantSearched) {
		t.Fatalf("searched roots = %#v, want %#v", searched, wantSearched)
	}
}

func TestResolveModelsInvokeFactoryDir_PreservesSearchedRootOnMissingLayout(t *testing.T) {
	t.Parallel()

	workingDirectory := filepath.Join("workspace", "project")
	operation := &operation{
		workingDirectory: workingDirectoryStub{dir: workingDirectory},
		resolveCurrentDir: func(string) (string, error) {
			return "", factorydefinitions.ErrFactoryLayoutNotFound
		},
	}

	_, err := operation.ResolveModelInvocationFactoryDir("")
	wantRoot := filepath.Join(workingDirectory, factorydefinitions.FactoryDir)
	if err == nil || !strings.Contains(err.Error(), wantRoot) {
		t.Fatalf("error = %v, want searched Factory root %q", err, wantRoot)
	}
	if !errors.Is(err, factorydefinitions.ErrFactoryLayoutNotFound) {
		t.Fatalf("error = %v, want ErrFactoryLayoutNotFound", err)
	}
}

func TestResolveModelsInvokeFactoryDir_ReportsWorkingDirectoryFailure(t *testing.T) {
	t.Parallel()

	operation := &operation{workingDirectory: workingDirectoryStub{err: errors.New("cwd unavailable")}}
	_, err := operation.ResolveModelInvocationFactoryDir("")
	if err == nil || !strings.Contains(err.Error(), "resolve models invoke factory root: cwd unavailable") {
		t.Fatalf("error = %v, want classified working-directory failure", err)
	}
}

func TestNewOperation_RequiresModelInvocationBoundaryDependencies(t *testing.T) {
	t.Parallel()

	openRuntime := &runtimeopening.Factory{}
	resolver := factorydefinitions.CurrentFactoryDirectoryResolver(func(root string) (string, error) { return root, nil })
	tests := []struct {
		name       string
		workingDir platformfilesystem.WorkingDirectory
		resolver   factorydefinitions.CurrentFactoryDirectoryResolver
		exporter   interface{ ExportInvocationArtifact(string, string) error }
		timeout    factorysessions.ModelInvocationTimeout
		want       string
	}{
		{name: "working directory", resolver: resolver, exporter: artifactExporterStub{}, timeout: factorysessions.DefaultModelInvocationTimeout, want: "working directory is required"},
		{name: "resolver", workingDir: workingDirectoryStub{}, exporter: artifactExporterStub{}, timeout: factorysessions.DefaultModelInvocationTimeout, want: "current Factory directory resolver is required"},
		{name: "artifact exporter", workingDir: workingDirectoryStub{}, resolver: resolver, timeout: factorysessions.DefaultModelInvocationTimeout, want: "artifact exporter is required"},
		{name: "timeout", workingDir: workingDirectoryStub{}, resolver: resolver, exporter: artifactExporterStub{}, want: "model invocation timeout is required"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewOperation(openRuntime, nil, test.workingDir, test.resolver, test.exporter, test.timeout, func(string) factoryruntime.RuntimeArtifactRoots { return factoryruntime.RuntimeArtifactRoots{} }, func() string { return "session-test-id" }, zap.NewNop(), nil)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("NewOperation() error = %v, want %q", err, test.want)
			}
		})
	}
}

// TestInvocationOperationOpensItsNarrowRuntimeView proves the invocation
// operation uses the grouped Factory Sessions opening capability supplied by
// Wire without requiring the concrete grouped construction type.
func TestInvocationOperationOpensItsNarrowRuntimeView(t *testing.T) {
	t.Parallel()

	opening := &invocationRuntimeOpeningStub{
		opened: roles.OpenedInvocationRuntime{Lifecycle: invocationLifecycleStub{}},
	}
	invocation, err := NewOperation(
		opening,
		nil,
		workingDirectoryStub{},
		factorydefinitions.CurrentFactoryDirectoryResolver(func(root string) (string, error) { return root, nil }),
		artifactExporterStub{},
		factorysessions.DefaultModelInvocationTimeout,
		func(string) factoryruntime.RuntimeArtifactRoots {
			return factoryruntime.RuntimeArtifactRoots{Logs: "logs", Metrics: "metrics"}
		},
		func() string { return "session-id" },
		zap.NewNop(),
		invocationPresentationOwnerStub{},
	)
	if err != nil {
		t.Fatalf("NewOperation: %v", err)
	}

	concrete, ok := invocation.(*operation)
	if !ok {
		t.Fatalf("NewOperation returned %T, want *operation", invocation)
	}
	opened, active, err := concrete.open(t.Context(), roles.InvocationTarget{
		FactorySessionID: "session-explicit",
		FactoryDir:       "factory", HomeDir: "home", RunnerID: "runner",
		CanonicalSessionID: "7d9d3fb4-6bc9-4df5-a67f-0f504f8ea3ba",
	})
	if err != nil {
		t.Fatalf("open invocation runtime: %v", err)
	}
	if active == nil || opened.Lifecycle == nil {
		t.Fatalf("opened invocation runtime = %#v, lifecycle = %#v", opened, active)
	}
	active.cancel()
	if opening.calls != 1 {
		t.Fatalf("invocation runtime openings = %d, want 1", opening.calls)
	}
	if opening.request == nil || opening.request.FactoryDefinition.Directory != "factory" ||
		opening.request.Workers.RunnerID != "runner" ||
		opening.request.FactorySession.FactorySessionID != "session-explicit" ||
		opening.request.FactorySession.CanonicalSessionID != "7d9d3fb4-6bc9-4df5-a67f-0f504f8ea3ba" ||
		opening.request.FactoryRuntime.LogDirectory != "logs" ||
		opening.request.FactoryRuntime.MetricsDirectory != "metrics" {
		t.Fatalf("invocation runtime request = %#v", opening.request)
	}
	if active.sessionID != "session-explicit" {
		t.Fatalf("lifecycle session = %q, want explicit selector", active.sessionID)
	}
}

type invocationRuntimeOpeningStub struct {
	calls   int
	request *factorysessions.RuntimeOpeningRequest
	opened  roles.OpenedInvocationRuntime
}

func (stub *invocationRuntimeOpeningStub) OpenInvocationRuntime(
	_ context.Context,
	request *factorysessions.RuntimeOpeningRequest,
) (roles.OpenedInvocationRuntime, error) {
	stub.calls++
	stub.request = request
	return stub.opened, nil
}

type invocationLifecycleStub struct{}

func (invocationLifecycleStub) StartLifecycle(context.Context, context.Context) error { return nil }

func (invocationLifecycleStub) StartWorkerLifecycle(context.Context) (factorysessions.RuntimeStop, error) {
	return nil, nil
}

func (invocationLifecycleStub) CompleteStartup(context.Context) error { return nil }

func (invocationLifecycleStub) WaitForRuntime(context.Context) error { return nil }

func (invocationLifecycleStub) StopLifecycle(context.Context) error { return nil }

func (invocationLifecycleStub) FailStartup(error) error { return nil }

func (invocationLifecycleStub) CurrentRuntimeBundle() factoryruntime.RuntimeRecord { return nil }

func TestModelInvocationContextAppliesOwnerTimeout(t *testing.T) {
	t.Parallel()

	before := time.Now()
	ctx, cancel := modelInvocationContext(t.Context(), factorysessions.ModelInvocationTimeout(2*time.Second))
	defer cancel()
	deadline, ok := ctx.Deadline()
	if !ok {
		t.Fatal("model invocation context has no deadline")
	}
	if deadline.Before(before.Add(1900*time.Millisecond)) || deadline.After(before.Add(2100*time.Millisecond)) {
		t.Fatalf("deadline = %s, want owner timeout near two seconds", deadline)
	}
}
