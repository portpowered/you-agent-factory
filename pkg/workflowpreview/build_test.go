package workflowpreview_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/portpowered/infinite-you/pkg/workflowpreview"
	"github.com/portpowered/infinite-you/pkg/workflowsource"
	"github.com/portpowered/infinite-you/pkg/workflowvalidation"
)

const validWorkflowSource = `
meta({ name: "review", version: 1 });
phase("setup");
log("starting");
workflow.log("step");
workflow.artifact({ kind: "log", label: "step" });
const result = await agent.run({ prompt: "review" });
workflow.final({ ok: true, result });
parallel([pipeline([result])]);
`

func TestBuildPreview_ValidWorkflowName(t *testing.T) {
	projectRoot := t.TempDir()
	writeWorkflow(t, projectRoot, "review.js", validWorkflowSource)

	ctx, err := workflowsource.DefaultContext(projectRoot)
	if err != nil {
		t.Fatalf("DefaultContext: %v", err)
	}

	preview := workflowpreview.BuildPreview(workflowpreview.Request{
		Source: workflowsource.Request{
			Kind:  workflowsource.KindWorkflowName,
			Value: "review",
		},
		Context: ctx,
	})
	if !preview.Valid {
		t.Fatalf("preview = %#v, want valid", preview)
	}
	if preview.SourceResolution.SourceHash == "" {
		t.Fatal("expected source hash")
	}
	if preview.PolicyPreview.PolicyHash == "" {
		t.Fatal("expected policy hash")
	}
	if preview.ResultConstraints.ArtifactURIScheme != "you-artifact" {
		t.Fatalf("artifact scheme = %q, want you-artifact", preview.ResultConstraints.ArtifactURIScheme)
	}
}

func TestBuildPreview_SyntaxError(t *testing.T) {
	projectRoot := t.TempDir()
	writeWorkflow(t, projectRoot, "broken.js", "meta({ name: \"broken\" );")

	ctx, err := workflowsource.DefaultContext(projectRoot)
	if err != nil {
		t.Fatalf("DefaultContext: %v", err)
	}

	preview := workflowpreview.BuildPreview(workflowpreview.Request{
		Source: workflowsource.Request{
			Kind:  workflowsource.KindWorkflowName,
			Value: "broken",
		},
		Context: ctx,
	})
	if preview.Valid {
		t.Fatal("expected invalid preview for syntax error")
	}
	if len(preview.SourceValidationIssues) == 0 {
		t.Fatal("expected source validation issues")
	}
	if preview.SourceValidationIssues[0].Code != workflowvalidation.CodeSyntaxError {
		t.Fatalf("issue code = %q, want %q", preview.SourceValidationIssues[0].Code, workflowvalidation.CodeSyntaxError)
	}
}

func TestBuildPreview_ForbiddenHostAccess(t *testing.T) {
	projectRoot := t.TempDir()
	writeWorkflow(t, projectRoot, "unsafe.js", "require('fs');")

	ctx, err := workflowsource.DefaultContext(projectRoot)
	if err != nil {
		t.Fatalf("DefaultContext: %v", err)
	}

	preview := workflowpreview.BuildPreview(workflowpreview.Request{
		Source: workflowsource.Request{
			Kind:  workflowsource.KindWorkflowName,
			Value: "unsafe",
		},
		Context: ctx,
	})
	if preview.Valid {
		t.Fatal("expected invalid preview for forbidden host access")
	}
	found := false
	for _, issue := range preview.SourceValidationIssues {
		if issue.Code == workflowvalidation.CodeForbiddenHostAccess {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("issues = %#v, want forbidden host access", preview.SourceValidationIssues)
	}
}

func TestBuildPreview_DeniedArtifactRoot(t *testing.T) {
	projectRoot := t.TempDir()
	writeWorkflow(t, projectRoot, "review.js", validWorkflowSource)

	ctx, err := workflowsource.DefaultContext(projectRoot)
	if err != nil {
		t.Fatalf("DefaultContext: %v", err)
	}

	preview := workflowpreview.BuildPreview(workflowpreview.Request{
		Source: workflowsource.Request{
			Kind:         workflowsource.KindWorkflowName,
			Value:        "review",
			ArtifactRoot: projectRoot,
		},
		Context: ctx,
	})
	if preview.Valid {
		t.Fatal("expected invalid preview for artifact root inside repository")
	}
	if preview.SourceResolution.ArtifactRoot.Allowed {
		t.Fatal("expected artifact root denial")
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
