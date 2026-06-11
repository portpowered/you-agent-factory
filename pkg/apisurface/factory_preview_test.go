// Apisurface preview tests exercise orchestrator-owned JavaScript preview and
// source packages directly; active behavior must not depend on root pkg/workflow*
// compatibility shims.
package apisurface_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
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

func TestFactoryPreviewResultFromPreview_PreservesPathAwareDiagnostics(t *testing.T) {
	projectRoot := t.TempDir()
	writeWorkflow(t, projectRoot, "unsafe.js", "require('fs');")

	ctx, err := workflowsource.DefaultContext(projectRoot)
	if err != nil {
		t.Fatalf("DefaultContext: %v", err)
	}
	preview := apisurface.BuildFactoryPreview(workflowpreview.Request{
		Source: workflowsource.Request{
			Kind:  workflowsource.KindWorkflowName,
			Value: "unsafe",
		},
		Context: ctx,
	})
	if preview.Valid {
		t.Fatal("expected invalid preview")
	}

	result := apisurface.FactoryPreviewResultFromPreview(preview)
	if len(result.SourceValidationIssues) == 0 {
		t.Fatalf("issues = %#v, want source validation diagnostics", result.SourceValidationIssues)
	}
	wantPath := workflowsource.ProjectClaudeWorkflowsDir + "/unsafe.js"
	if result.SourceValidationIssues[0].Path == nil || strings.TrimSpace(*result.SourceValidationIssues[0].Path) != wantPath {
		t.Fatalf("issue path = %v, want %q", result.SourceValidationIssues[0].Path, wantPath)
	}
}

func TestFactoryPreviewRequestFromAPI_MapsCanonicalPreviewInput(t *testing.T) {
	projectRoot := t.TempDir()
	writeWorkflow(t, projectRoot, "review.js", validWorkflowSource)

	projectRootPtr := projectRoot
	sourceValue := "review"
	mapped, err := apisurface.FactoryPreviewRequestFromAPI(factoryapi.FactoryPreviewRequest{
		SourceKind:  factoryapi.WORKFLOWNAME,
		ProjectRoot: &projectRootPtr,
		SourceValue: &sourceValue,
	})
	if err != nil {
		t.Fatalf("FactoryPreviewRequestFromAPI: %v", err)
	}
	if mapped.Source.Kind != workflowsource.KindWorkflowName || mapped.Source.Value != "review" {
		t.Fatalf("mapped source = %#v, want workflow name review", mapped.Source)
	}
	if strings.TrimSpace(mapped.Context.ProjectRoot) != projectRoot {
		t.Fatalf("project root = %q, want %q", mapped.Context.ProjectRoot, projectRoot)
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
