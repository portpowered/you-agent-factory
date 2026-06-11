package apisurface_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/portpowered/infinite-you/pkg/apisurface"
	workflowpreview "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/preview"
	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
)

const validWorkflowSource = `
meta({ name: "review", version: 1 });
phase("setup");
log("starting");
`

func TestBuildFactoryPreview_MatchesOrchestratorPreviewSeam(t *testing.T) {
	projectRoot := t.TempDir()
	writeWorkflow(t, projectRoot, "review.js", validWorkflowSource)

	ctx, err := workflowsource.DefaultContext(projectRoot)
	if err != nil {
		t.Fatalf("DefaultContext: %v", err)
	}
	req := workflowpreview.Request{
		Source: workflowsource.Request{
			Kind:  workflowsource.KindWorkflowName,
			Value: "review",
		},
		Context: ctx,
	}

	viaSurface := apisurface.BuildFactoryPreview(req)
	direct := workflowpreview.BuildPreview(req)
	if viaSurface.Valid != direct.Valid ||
		viaSurface.SourceResolution.SourceHash != direct.SourceResolution.SourceHash ||
		viaSurface.PolicyPreview.PolicyHash != direct.PolicyPreview.PolicyHash {
		t.Fatalf("surface = %#v, orchestrator = %#v, want equivalent preview", viaSurface, direct)
	}
}

func writeWorkflow(t *testing.T, projectRoot, name, content string) {
	t.Helper()
	workflowDir := filepath.Join(projectRoot, workflowsource.ProjectClaudeWorkflowsDir)
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		t.Fatalf("mkdir workflows: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workflowDir, name), []byte(content), 0o600); err != nil {
		t.Fatalf("write workflow: %v", err)
	}
}
