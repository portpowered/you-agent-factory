package apisurface_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/portpowered/infinite-you/pkg/apisurface"
	mcpworkflow "github.com/portpowered/infinite-you/pkg/mcp/workflow"
	"github.com/portpowered/infinite-you/pkg/workflowpreview"
	"github.com/portpowered/infinite-you/pkg/workflowsource"
)

const validWorkflowSource = `
meta({ name: "review", version: 1 });
phase("setup");
log("starting");
`

func TestWorkflowPreviewSurfaces_MatchForValidWorkflow(t *testing.T) {
	projectRoot := t.TempDir()
	writeWorkflow(t, projectRoot, "review.js", validWorkflowSource)

	ctx, err := workflowsource.DefaultContext(projectRoot)
	if err != nil {
		t.Fatalf("DefaultContext: %v", err)
	}
	input := workflowpreview.Request{
		Source: workflowsource.Request{
			Kind:  workflowsource.KindWorkflowName,
			Value: "review",
		},
		Context: ctx,
	}

	apiResult := apisurface.WorkflowPreviewResultFromPreview(apisurface.BuildWorkflowPreview(input))
	mcpResult := mcpworkflow.PreviewInputFromRequest(input)

	projectRootPtr := projectRoot
	sourceValue := "review"
	mcpToolResult, err := mcpworkflow.ValidateTool(mcpworkflow.ValidateToolInput{
		SourceKind:  "WORKFLOW_NAME",
		ProjectRoot: &projectRootPtr,
		SourceValue: &sourceValue,
	})
	if err != nil {
		t.Fatalf("ValidateTool: %v", err)
	}

	if apiResult.Valid != mcpResult.Valid || apiResult.Valid != mcpToolResult.Valid {
		t.Fatalf("valid mismatch: api=%v mcp=%v tool=%v", apiResult.Valid, mcpResult.Valid, mcpToolResult.Valid)
	}
	if deref(apiResult.SourceResolution.SourceHash) != deref(mcpResult.SourceResolution.SourceHash) ||
		deref(apiResult.SourceResolution.SourceHash) != deref(mcpToolResult.SourceResolution.SourceHash) {
		t.Fatalf("source hash mismatch across surfaces")
	}
	if apiResult.PolicyPreview.PolicyHash != mcpResult.PolicyPreview.PolicyHash ||
		apiResult.PolicyPreview.PolicyHash != mcpToolResult.PolicyPreview.PolicyHash {
		t.Fatalf("policy hash mismatch across surfaces")
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

func deref(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
