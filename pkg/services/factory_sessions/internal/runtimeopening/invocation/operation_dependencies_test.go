package invocation

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimeopening"
)

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
			_, err := NewOperation(openRuntime, runtimeopening.ExternalEffects{}, test.workingDir, test.resolver, test.exporter, test.timeout, func(string) factoryruntime.RuntimeArtifactRoots { return factoryruntime.RuntimeArtifactRoots{} }, func() string { return "session-test-id" })
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("NewOperation() error = %v, want %q", err, test.want)
			}
		})
	}
}

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
