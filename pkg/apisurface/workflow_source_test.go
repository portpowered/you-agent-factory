package apisurface_test

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/apisurface"
	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
)

func TestNormalizeWorkflowSourceRequest_ResolvesWorkflowNameThroughOrchestratorSource(t *testing.T) {
	projectRoot := t.TempDir()
	writeWorkflow(t, projectRoot, "review.js", validWorkflowSource)

	ctx, err := workflowsource.DefaultContext(projectRoot)
	if err != nil {
		t.Fatalf("DefaultContext: %v", err)
	}
	req := workflowsource.Request{
		Kind:  workflowsource.KindWorkflowName,
		Value: "review",
	}

	viaSurface := apisurface.NormalizeWorkflowSourceRequest(req, ctx)
	direct := workflowsource.Resolve(req, ctx)
	if viaSurface.Found != direct.Found ||
		viaSurface.SourceRef != direct.SourceRef ||
		viaSurface.SourceHash != direct.SourceHash {
		t.Fatalf("surface = %#v, orchestrator = %#v, want equivalent source resolution", viaSurface, direct)
	}
}

func TestNormalizeWorkflowSourceRequest_MissingWorkflowReportsNotFoundDiagnostic(t *testing.T) {
	projectRoot := t.TempDir()
	ctx, err := workflowsource.DefaultContext(projectRoot)
	if err != nil {
		t.Fatalf("DefaultContext: %v", err)
	}

	resolution := apisurface.NormalizeWorkflowSourceRequest(workflowsource.Request{
		Kind:  workflowsource.KindWorkflowName,
		Value: "missing",
	}, ctx)
	if resolution.Found {
		t.Fatalf("resolution = %#v, want not found", resolution)
	}
	if len(resolution.Diagnostics) == 0 {
		t.Fatal("expected source resolution diagnostics")
	}
	if resolution.Diagnostics[0].Code != workflowsource.CodeSourceNotFound {
		t.Fatalf("diagnostic code = %q, want %q", resolution.Diagnostics[0].Code, workflowsource.CodeSourceNotFound)
	}
}
