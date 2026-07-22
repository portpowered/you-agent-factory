package factory_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
)

const previewValidWorkflowSource = `
meta({ name: "review", version: 1 });
phase("setup");
log("starting");
workflow.log("step");
workflow.artifact({ kind: "log", label: "step" });
const result = await agent.run({ prompt: "review" });
workflow.final({ ok: true, result });
pipeline([], function () {}, function () {});
`

func TestBuildPreview_ValidWorkflowName(t *testing.T) {
	t.Parallel()
	projectRoot := t.TempDir()
	writeWorkflow(t, projectRoot, "review.js", previewValidWorkflowSource)

	ctx, err := factoryWorkflowDefinitions.DefaultSourceContext(projectRoot)
	if err != nil {
		t.Fatalf("DefaultContext: %v", err)
	}

	preview := factoryWorkflowDefinitions.BuildPreview(factory.WorkflowPreviewRequest{
		Source: factory.WorkflowSourceRequest{
			Kind:  factory.WorkflowSourceKindWorkflowName,
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
	t.Parallel()
	projectRoot := t.TempDir()
	writeWorkflow(t, projectRoot, "broken.js", "meta({ name: \"broken\" );")

	ctx, err := factoryWorkflowDefinitions.DefaultSourceContext(projectRoot)
	if err != nil {
		t.Fatalf("DefaultContext: %v", err)
	}

	preview := factoryWorkflowDefinitions.BuildPreview(factory.WorkflowPreviewRequest{
		Source: factory.WorkflowSourceRequest{
			Kind:  factory.WorkflowSourceKindWorkflowName,
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
	if preview.SourceValidationIssues[0].Code != factory.WorkflowValidationCodeSyntaxError {
		t.Fatalf("issue code = %q, want %q", preview.SourceValidationIssues[0].Code, factory.WorkflowValidationCodeSyntaxError)
	}
}

func TestBuildPreview_ForbiddenHostAccess(t *testing.T) {
	t.Parallel()
	projectRoot := t.TempDir()
	writeWorkflow(t, projectRoot, "unsafe.js", "require('fs');")

	ctx, err := factoryWorkflowDefinitions.DefaultSourceContext(projectRoot)
	if err != nil {
		t.Fatalf("DefaultContext: %v", err)
	}

	preview := factoryWorkflowDefinitions.BuildPreview(factory.WorkflowPreviewRequest{
		Source: factory.WorkflowSourceRequest{
			Kind:  factory.WorkflowSourceKindWorkflowName,
			Value: "unsafe",
		},
		Context: ctx,
	})

	if preview.Valid {
		t.Fatal("expected invalid preview for forbidden host access")
	}
	found := false
	for _, issue := range preview.SourceValidationIssues {
		if issue.Code == factory.WorkflowValidationCodeForbiddenHostAccess {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("issues = %#v, want forbidden host access", preview.SourceValidationIssues)
	}
}

func TestBuildPreview_DeniedArtifactRoot(t *testing.T) {
	t.Parallel()
	projectRoot := t.TempDir()
	writeWorkflow(t, projectRoot, "review.js", previewValidWorkflowSource)

	ctx, err := factoryWorkflowDefinitions.DefaultSourceContext(projectRoot)
	if err != nil {
		t.Fatalf("DefaultContext: %v", err)
	}

	preview := factoryWorkflowDefinitions.BuildPreview(factory.WorkflowPreviewRequest{
		Source: factory.WorkflowSourceRequest{
			Kind:         factory.WorkflowSourceKindWorkflowName,
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
	workflowDir := filepath.Join(projectRoot, factory.WorkflowSourceProjectClaudeWorkflowsDir)
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		t.Fatalf("mkdir workflows: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workflowDir, name), []byte(content), 0o600); err != nil {
		t.Fatalf("write workflow: %v", err)
	}
}
