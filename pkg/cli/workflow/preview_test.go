package workflow_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	"github.com/portpowered/infinite-you/pkg/cli/workflow"
	workflowpreview "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/preview"
	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
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
		SourceKind:  string(workflowsource.KindWorkflowName),
		SourceValue: "review",
		Output:      &output,
	})
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	text := output.String()
	for _, want := range []string{"Factory preview passed.", "Source hash:", "Policy hash:"} {
		if !strings.Contains(text, want) {
			t.Fatalf("output missing %q:\n%s", want, text)
		}
	}
}

func TestPreview_JSONOutputMatchesCanonicalFactoryPreview(t *testing.T) {
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
	want := apisurface.FactoryPreviewResultFromPreview(apisurface.BuildFactoryPreview(input))

	var output bytes.Buffer
	if err := workflow.Preview(workflow.PreviewConfig{
		Dir:         projectRoot,
		SourceKind:  string(workflowsource.KindWorkflowName),
		SourceValue: "review",
		JSON:        true,
		Output:      &output,
	}); err != nil {
		t.Fatalf("Preview: %v", err)
	}

	wantJSON, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("marshal canonical preview: %v", err)
	}
	gotJSON := bytes.TrimSpace(output.Bytes())
	if !bytes.Equal(gotJSON, wantJSON) {
		var got factoryapi.FactoryPreviewResult
		if err := json.Unmarshal(gotJSON, &got); err != nil {
			t.Fatalf("decode preview JSON: %v", err)
		}
		t.Fatalf("preview JSON = %#v, want %#v", got, want)
	}
}

func TestPreview_InvalidWorkflowReportsDiagnostics(t *testing.T) {
	projectRoot := t.TempDir()
	writeWorkflow(t, projectRoot, "broken.js", "require('fs');")

	var output bytes.Buffer
	err := workflow.Preview(workflow.PreviewConfig{
		Dir:         projectRoot,
		SourceKind:  string(workflowsource.KindWorkflowName),
		SourceValue: "broken",
		Output:      &output,
	})
	if err == nil {
		t.Fatal("expected preview failure")
	}
	if !strings.Contains(output.String(), "Factory preview failed.") {
		t.Fatalf("output = %q, want failure summary", output.String())
	}
	wantPath := workflowsource.ProjectClaudeWorkflowsDir + "/broken.js"
	if !strings.Contains(output.String(), wantPath+":") {
		t.Fatalf("output = %q, want path-aware diagnostic prefix %q", output.String(), wantPath)
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
