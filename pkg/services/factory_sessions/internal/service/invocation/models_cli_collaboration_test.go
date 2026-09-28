package invocation

import (
	"context"
	"errors"
	"strings"
	"testing"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/models"
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

func TestOpenModelsCatalogScope_RequiresModelsRoot(t *testing.T) {
	t.Parallel()

	op := &operation{}
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

type failingPresentationSessions struct {
	invocationSessionStub
	err error
}

func (s *failingPresentationSessions) Start(context.Context, factorysessions.SessionStartRequest) (factorysessions.SessionStartResult, error) {
	return factorysessions.SessionStartResult{}, s.err
}

func TestOpenModelsPresentationScopePropagatesSessionStartFailure(t *testing.T) {
	startErr := errors.New("session start failed")
	service := &failingPresentationSessions{err: startErr}
	op := &operation{
		sessions:      service,
		artifactRoots: func(string) factoryruntime.RuntimeArtifactRoots { return factoryruntime.RuntimeArtifactRoots{} },
	}
	_, err := op.OpenModelsPresentationScope(context.Background(), models.PresentationScopeRequest{FactoryDir: "factory"})
	if !errors.Is(err, startErr) {
		t.Fatalf("OpenModelsPresentationScope() error = %v, want session start failure", err)
	}
}
