package invocation

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimeopening"
	"github.com/portpowered/infinite-you/pkg/services/models"
	"go.uber.org/zap"
)

func TestResolvedOperatorDefaultsFromPresentationPreservesModelDefaults(t *testing.T) {
	t.Parallel()

	defaults := resolvedOperatorDefaultsFromPresentation(models.PresentationOperatorDefaults{
		WorkerModelProvider: "codex",
		WorkerModel:         "gpt-5.6",
	})
	if defaults.WorkerModelProvider != "codex" {
		t.Fatalf("provider = %q, want codex", defaults.WorkerModelProvider)
	}
	if defaults.WorkerModel != "gpt-5.6" {
		t.Fatalf("model = %q, want gpt-5.6", defaults.WorkerModel)
	}
}

func TestOpenModelsCatalogScope_RequiresOperation(t *testing.T) {
	t.Parallel()

	var op *operation
	_, err := op.OpenModelsCatalogScope(context.Background())
	if err == nil || !strings.Contains(err.Error(), "invocation operation is required") {
		t.Fatalf("error = %v, want required operation", err)
	}
}

func TestOpenModelsCatalogScope_RequiresOpenRuntime(t *testing.T) {
	t.Parallel()

	op := &operation{}
	_, err := op.OpenModelsCatalogScope(context.Background())
	if err == nil || !strings.Contains(err.Error(), "invocation operation is required") {
		t.Fatalf("error = %v, want required open runtime", err)
	}
}

func TestOpenModelsCatalogScope_RequiresModelsRoot(t *testing.T) {
	t.Parallel()

	op := &operation{openRuntime: &runtimeopening.Factory{}}
	_, err := op.OpenModelsCatalogScope(context.Background())
	if err == nil || !strings.Contains(err.Error(), "models presentation root is unavailable") {
		t.Fatalf("error = %v, want unavailable models root", err)
	}
}

func TestOpenModelsPresentationScope_RequiresOperation(t *testing.T) {
	t.Parallel()

	var op *operation
	_, err := op.OpenModelsPresentationScope(context.Background(), models.PresentationScopeRequest{})
	if err == nil || !strings.Contains(err.Error(), "invocation operation is required") {
		t.Fatalf("error = %v, want required operation", err)
	}
}

func TestOpenModelsPresentationScope_PropagatesFactoryDirResolutionFailure(t *testing.T) {
	t.Parallel()

	op := &operation{
		workingDirectory: workingDirectoryStub{err: errors.New("cwd unavailable")},
	}
	_, err := op.OpenModelsPresentationScope(context.Background(), models.PresentationScopeRequest{})
	if err == nil || !strings.Contains(err.Error(), "cwd unavailable") {
		t.Fatalf("error = %v, want factory dir resolution failure", err)
	}
}

func TestOpenModelsPresentationScope_PropagatesRuntimeOpenFailure(t *testing.T) {
	t.Parallel()

	factoryDir := t.TempDir()
	op, err := NewOperation(
		&runtimeopening.Factory{},
		nil,
		workingDirectoryStub{dir: factoryDir},
		func(root string) (string, error) {
			return filepath.Join(root, factorydefinitions.FactoryDir, "active"), nil
		},
		artifactExporterStub{},
		factorysessions.DefaultModelInvocationTimeout,
		func(string) factoryruntime.RuntimeArtifactRoots { return factoryruntime.RuntimeArtifactRoots{} },
		func() string { return "presentation-scope-test" },
		zap.NewNop(),
		invocationPresentationOwnerStub{},
	)
	if err != nil {
		t.Fatalf("NewOperation() error = %v", err)
	}
	scopeOpener, ok := op.(interface {
		OpenModelsPresentationScope(context.Context, models.PresentationScopeRequest) (models.PresentationScope, error)
	})
	if !ok {
		t.Fatal("NewOperation() must retain the internal Models scope opener")
	}
	_, err = scopeOpener.OpenModelsPresentationScope(context.Background(), models.PresentationScopeRequest{
		FactoryDir: factoryDir,
	})
	if err == nil {
		t.Fatal("OpenModelsPresentationScope() error = nil, want runtime open failure")
	}
}
