package workflow_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/apisurface"
	"github.com/portpowered/infinite-you/pkg/cli/workflow"
	workflowpreview "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/preview"
	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
)

func TestValidate_ValidWorkflowNameHumanOutput(t *testing.T) {
	projectRoot := t.TempDir()
	writeWorkflow(t, projectRoot, "review.js", validWorkflowSource)

	var output bytes.Buffer
	err := workflow.Validate(workflow.ValidateConfig{
		SourceConfig: workflow.SourceConfig{
			Dir:         projectRoot,
			SourceKind:  string(workflowsource.KindWorkflowName),
			SourceValue: "review",
		},
		Output: &output,
	})
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	text := output.String()
	for _, want := range []string{"Workflow validation passed.", "Source hash:"} {
		if !strings.Contains(text, want) {
			t.Fatalf("output missing %q:\n%s", want, text)
		}
	}
	wantRef := workflowsource.ProjectClaudeWorkflowsDir + "/review.js"
	if !strings.Contains(text, wantRef) {
		t.Fatalf("output missing source ref %q:\n%s", wantRef, text)
	}
}

func TestValidate_JSONOutputMatchesCanonicalValidationResult(t *testing.T) {
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
	want := apisurface.FactoryWorkflowValidationResultFromPreview(
		apisurface.BuildFactoryWorkflowValidation(input),
	)

	var output bytes.Buffer
	if err := workflow.Validate(workflow.ValidateConfig{
		SourceConfig: workflow.SourceConfig{
			Dir:         projectRoot,
			SourceKind:  string(workflowsource.KindWorkflowName),
			SourceValue: "review",
		},
		JSON:   true,
		Output: &output,
	}); err != nil {
		t.Fatalf("Validate: %v", err)
	}

	wantJSON, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("marshal canonical validation result: %v", err)
	}
	gotJSON := bytes.TrimSpace(output.Bytes())
	if !bytes.Equal(gotJSON, wantJSON) {
		var got apisurface.FactoryWorkflowValidationResult
		if err := json.Unmarshal(gotJSON, &got); err != nil {
			t.Fatalf("decode validation JSON: %v", err)
		}
		t.Fatalf("validation JSON = %#v, want %#v", got, want)
	}
	if !want.Valid || len(want.BlockingDiagnostics) != 0 {
		t.Fatalf("want valid result with empty blocking diagnostics, got valid=%v diagnostics=%d",
			want.Valid, len(want.BlockingDiagnostics))
	}
}

func TestValidate_InlineWorkflowSource(t *testing.T) {
	projectRoot := t.TempDir()

	var output bytes.Buffer
	err := workflow.Validate(workflow.ValidateConfig{
		SourceConfig: workflow.SourceConfig{
			Dir:          projectRoot,
			SourceKind:   string(workflowsource.KindInlineWorkflow),
			InlineSource: `meta({ name: "inline", version: 1 }); phase("setup");`,
		},
		Output: &output,
	})
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if !strings.Contains(output.String(), "Workflow validation passed.") {
		t.Fatalf("output = %q, want validation passed summary", output.String())
	}
}
