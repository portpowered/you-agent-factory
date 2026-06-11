package workflow_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	mcpworkflow "github.com/portpowered/infinite-you/pkg/mcp/workflow"
	workflowpreview "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/preview"
	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
)

const validWorkflowSource = `
meta({ name: "review", version: 1 });
phase("setup");
log("starting");
`

func TestValidateTool_MatchesAPISurfacePreview(t *testing.T) {
	projectRoot := t.TempDir()
	writeWorkflow(t, projectRoot, "review.js", validWorkflowSource)

	projectRootPtr := projectRoot
	sourceValue := "review"
	result, err := mcpworkflow.ValidateTool(factoryapi.FactoryPreviewRequest{
		SourceKind:  factoryapi.WORKFLOWNAME,
		ProjectRoot: &projectRootPtr,
		SourceValue: &sourceValue,
	})
	if err != nil {
		t.Fatalf("ValidateTool: %v", err)
	}

	ctx, err := workflowsource.DefaultContext(projectRoot)
	if err != nil {
		t.Fatalf("DefaultContext: %v", err)
	}
	expected := apisurface.FactoryPreviewResultFromPreview(apisurface.BuildFactoryPreview(workflowpreview.Request{
		Source: workflowsource.Request{
			Kind:  workflowsource.KindWorkflowName,
			Value: "review",
		},
		Context: ctx,
	}))
	if result.Valid != expected.Valid || deref(result.SourceResolution.SourceHash) != deref(expected.SourceResolution.SourceHash) {
		t.Fatalf("mcp = %#v, api surface = %#v", result, expected)
	}
}

func TestStartTool_MatchesValidateTool(t *testing.T) {
	projectRoot := t.TempDir()
	writeWorkflow(t, projectRoot, "review.js", validWorkflowSource)

	projectRootPtr := projectRoot
	sourceValue := "review"
	request := factoryapi.FactoryPreviewRequest{
		SourceKind:  factoryapi.WORKFLOWNAME,
		ProjectRoot: &projectRootPtr,
		SourceValue: &sourceValue,
	}
	validateResult, err := mcpworkflow.ValidateTool(request)
	if err != nil {
		t.Fatalf("ValidateTool: %v", err)
	}
	startResult, err := mcpworkflow.StartTool(request)
	if err != nil {
		t.Fatalf("StartTool: %v", err)
	}
	if validateResult.Valid != startResult.Valid || deref(validateResult.SourceResolution.SourceHash) != deref(startResult.SourceResolution.SourceHash) {
		t.Fatalf("start = %#v, validate = %#v", startResult, validateResult)
	}
}

func TestMarshalToolError_EncodesStructuredFailure(t *testing.T) {
	projectRoot := t.TempDir()
	writeWorkflow(t, projectRoot, "broken.js", "require('fs');")

	projectRootPtr := projectRoot
	sourceValue := "broken"
	result, err := mcpworkflow.ValidateTool(factoryapi.FactoryPreviewRequest{
		SourceKind:  factoryapi.WORKFLOWNAME,
		ProjectRoot: &projectRootPtr,
		SourceValue: &sourceValue,
	})
	if err != nil {
		t.Fatalf("ValidateTool: %v", err)
	}
	toolErr := mcpworkflow.StructuredErrorFromPreview(result, "validation")
	encoded, err := mcpworkflow.MarshalToolError(toolErr)
	if err != nil {
		t.Fatalf("MarshalToolError: %v", err)
	}
	if len(encoded) == 0 || !strings.Contains(string(encoded), toolErr.Code) {
		t.Fatalf("encoded = %q, want structured tool error JSON", string(encoded))
	}
}

func TestStructuredErrorFromPreview_UsesFirstBlockingIssue(t *testing.T) {
	projectRoot := t.TempDir()
	writeWorkflow(t, projectRoot, "broken.js", "require('fs');")

	projectRootPtr := projectRoot
	sourceValue := "broken"
	result, err := mcpworkflow.ValidateTool(factoryapi.FactoryPreviewRequest{
		SourceKind:  factoryapi.WORKFLOWNAME,
		ProjectRoot: &projectRootPtr,
		SourceValue: &sourceValue,
	})
	if err != nil {
		t.Fatalf("ValidateTool: %v", err)
	}
	toolErr := mcpworkflow.StructuredErrorFromPreview(result, "validation")
	if toolErr.Code == "" || toolErr.Message == "" {
		t.Fatalf("tool error = %#v, want structured code and message", toolErr)
	}
	if toolErr.Preview.Valid {
		t.Fatal("expected invalid preview in structured error")
	}
}

func TestValidateTool_ReturnsPathAwareDiagnosticsForInvalidSource(t *testing.T) {
	projectRoot := t.TempDir()
	writeWorkflow(t, projectRoot, "broken.js", "require('fs');")

	projectRootPtr := projectRoot
	sourceValue := "broken"
	result, err := mcpworkflow.ValidateTool(factoryapi.FactoryPreviewRequest{
		SourceKind:  factoryapi.WORKFLOWNAME,
		ProjectRoot: &projectRootPtr,
		SourceValue: &sourceValue,
	})
	if err != nil {
		t.Fatalf("ValidateTool: %v", err)
	}
	if result.Valid {
		t.Fatal("expected invalid preview")
	}
	if len(result.SourceValidationIssues) == 0 {
		t.Fatalf("issues = %#v, want source validation diagnostics", result.SourceValidationIssues)
	}
	wantPath := workflowsource.ProjectClaudeWorkflowsDir + "/broken.js"
	if result.SourceValidationIssues[0].Path == nil || strings.TrimSpace(*result.SourceValidationIssues[0].Path) != wantPath {
		t.Fatalf("issue path = %v, want %q", result.SourceValidationIssues[0].Path, wantPath)
	}

	startResult, err := mcpworkflow.StartTool(factoryapi.FactoryPreviewRequest{
		SourceKind:  factoryapi.WORKFLOWNAME,
		ProjectRoot: &projectRootPtr,
		SourceValue: &sourceValue,
	})
	if err != nil {
		t.Fatalf("StartTool: %v", err)
	}
	if startResult.Valid || len(startResult.SourceValidationIssues) == 0 {
		t.Fatalf("start preview = %#v, want invalid preview with diagnostics", startResult)
	}
	if startResult.SourceValidationIssues[0].Path == nil || strings.TrimSpace(*startResult.SourceValidationIssues[0].Path) != wantPath {
		t.Fatalf("start issue path = %v, want %q", startResult.SourceValidationIssues[0].Path, wantPath)
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
