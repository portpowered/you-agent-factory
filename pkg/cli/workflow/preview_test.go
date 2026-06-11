package workflow_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/cli/workflow"
	"github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
)

const validWorkflowSource = `
meta({ name: "review", version: 1 });
phase("setup");
log("starting");
`

func TestPreview_ValidWorkflowNameHumanOutput(t *testing.T) {
	projectRoot := t.TempDir()
	writeWorkflow(t, projectRoot, "review.js", validWorkflowSource)

	var output bytes.Buffer
	err := workflow.Preview(workflow.PreviewConfig{
		Dir:         projectRoot,
		SourceKind:  string(source.KindWorkflowName),
		SourceValue: "review",
		Output:      &output,
	})
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	text := output.String()
	for _, want := range []string{"Workflow preview passed.", "Source hash:", "Policy hash:"} {
		if !strings.Contains(text, want) {
			t.Fatalf("output missing %q:\n%s", want, text)
		}
	}
}

func TestPreview_InvalidWorkflowReportsDiagnostics(t *testing.T) {
	projectRoot := t.TempDir()
	writeWorkflow(t, projectRoot, "broken.js", "require('fs');")

	var output bytes.Buffer
	err := workflow.Preview(workflow.PreviewConfig{
		Dir:         projectRoot,
		SourceKind:  string(source.KindWorkflowName),
		SourceValue: "broken",
		Output:      &output,
	})
	if err == nil {
		t.Fatal("expected preview failure")
	}
	if !strings.Contains(output.String(), "Workflow preview failed.") {
		t.Fatalf("output = %q, want failure summary", output.String())
	}
}

func writeWorkflow(t *testing.T, projectRoot, name, content string) {
	t.Helper()
	workflowDir := filepath.Join(projectRoot, source.ProjectClaudeWorkflowsDir)
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		t.Fatalf("mkdir workflows: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workflowDir, name), []byte(content), 0o600); err != nil {
		t.Fatalf("write workflow: %v", err)
	}
}
