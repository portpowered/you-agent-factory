package apisurface_test

import (
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/apisurface"
	workflowpreview "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/preview"
	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
)

func TestBuildFactoryWorkflowValidation_MatchesOrchestratorPreviewSeam(t *testing.T) {
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

	viaSurface := apisurface.BuildFactoryWorkflowValidation(req)
	direct := workflowpreview.BuildPreview(req)
	if viaSurface.Valid != direct.Valid ||
		viaSurface.SourceResolution.SourceHash != direct.SourceResolution.SourceHash {
		t.Fatalf("surface = %#v, orchestrator = %#v, want equivalent validation preview", viaSurface, direct)
	}
}

func TestFactoryWorkflowValidationResultFromPreview_ValidWorkflowHasEmptyBlockingDiagnostics(t *testing.T) {
	projectRoot := t.TempDir()
	writeWorkflow(t, projectRoot, "review.js", validWorkflowSource)

	ctx, err := workflowsource.DefaultContext(projectRoot)
	if err != nil {
		t.Fatalf("DefaultContext: %v", err)
	}
	preview := apisurface.BuildFactoryWorkflowValidation(workflowpreview.Request{
		Source: workflowsource.Request{
			Kind:  workflowsource.KindWorkflowName,
			Value: "review",
		},
		Context: ctx,
	})
	if !preview.Valid {
		t.Fatal("expected valid workflow validation preview")
	}

	result := apisurface.FactoryWorkflowValidationResultFromPreview(preview)
	if !result.Valid {
		t.Fatalf("valid = false, want true")
	}
	if len(result.BlockingDiagnostics) != 0 {
		t.Fatalf("blocking diagnostics = %#v, want empty", result.BlockingDiagnostics)
	}
	if result.SourceResolution.SourceHash == nil || strings.TrimSpace(*result.SourceResolution.SourceHash) == "" {
		t.Fatalf("source hash = %v, want non-empty hash", result.SourceResolution.SourceHash)
	}
	wantRef := workflowsource.ProjectClaudeWorkflowsDir + "/review.js"
	if result.SourceResolution.SourceRef == nil || strings.TrimSpace(*result.SourceResolution.SourceRef) != wantRef {
		t.Fatalf("source ref = %v, want %q", result.SourceResolution.SourceRef, wantRef)
	}
}
