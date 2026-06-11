package factorysessionexecution_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
)

const durableStartWorkflowSource = `
meta({ name: "review", version: 1 });
phase("setup");
log("starting");
workflow.log("step");
workflow.artifact({ kind: "log", label: "step" });
const result = await agent.run({ prompt: "review" });
workflow.final({ ok: true, result });
parallel([pipeline([result])]);
`

func TestResolveStartSource_WorkflowName_UsesProjectClaudeWorkflowsFirst(t *testing.T) {
	projectRoot := t.TempDir()
	workflowDir := filepath.Join(projectRoot, workflowsource.ProjectClaudeWorkflowsDir)
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		t.Fatalf("mkdir workflows: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workflowDir, "review.js"), []byte(durableStartWorkflowSource), 0o600); err != nil {
		t.Fatalf("write workflow: %v", err)
	}

	normalized, err := factorysessionexecution.NormalizeStartRequest(factorysessionexecution.StartRequest{
		RequestID: "req-resolve-name",
		Source: factorysessionexecution.Source{
			Kind:         workflowsource.KindWorkflowName,
			WorkflowName: "review",
		},
	})
	if err != nil {
		t.Fatalf("NormalizeStartRequest: %v", err)
	}

	resolved, err := factorysessionexecution.ResolveStartSource(normalized, factorysessionexecution.StartSourceContext{
		ProjectRoot: projectRoot,
	})
	if err != nil {
		t.Fatalf("ResolveStartSource: %v", err)
	}
	if resolved.SourceRef != workflowsource.ProjectClaudeWorkflowsDir+"/review.js" {
		t.Fatalf("sourceRef = %q", resolved.SourceRef)
	}
	if resolved.SourceHash == "" {
		t.Fatal("expected stable source hash")
	}
	if len(resolved.ResolutionOrder) != 1 || resolved.ResolutionOrder[0] != "PROJECT_CLAUDE_WORKFLOWS" {
		t.Fatalf("resolutionOrder = %#v, want PROJECT_CLAUDE_WORKFLOWS", resolved.ResolutionOrder)
	}
}

func TestResolveStartSource_RejectsMissingProjectRoot(t *testing.T) {
	normalized, err := factorysessionexecution.NormalizeStartRequest(factorysessionexecution.StartRequest{
		RequestID: "req-missing-root",
		Source: factorysessionexecution.Source{
			Kind:         workflowsource.KindWorkflowName,
			WorkflowName: "review",
		},
	})
	if err != nil {
		t.Fatalf("NormalizeStartRequest: %v", err)
	}

	_, err = factorysessionexecution.ResolveStartSource(normalized, factorysessionexecution.StartSourceContext{})
	if err == nil {
		t.Fatal("error = nil, want projectRoot validation error")
	}
	var validationErr *factorysessionexecution.ValidationError
	if !errors.As(err, &validationErr) || validationErr.Field != "projectRoot" {
		t.Fatalf("error = %v, want projectRoot validation error", err)
	}
}

func TestResolveStartSource_RejectsUnresolvedWorkflowName(t *testing.T) {
	projectRoot := t.TempDir()
	normalized, err := factorysessionexecution.NormalizeStartRequest(factorysessionexecution.StartRequest{
		RequestID: "req-not-found",
		Source: factorysessionexecution.Source{
			Kind:         workflowsource.KindWorkflowName,
			WorkflowName: "missing",
		},
	})
	if err != nil {
		t.Fatalf("NormalizeStartRequest: %v", err)
	}

	_, err = factorysessionexecution.ResolveStartSource(normalized, factorysessionexecution.StartSourceContext{
		ProjectRoot: projectRoot,
	})
	if err == nil {
		t.Fatal("error = nil, want source validation error")
	}
	var validationErr *factorysessionexecution.ValidationError
	if !errors.As(err, &validationErr) || validationErr.Field != "source" {
		t.Fatalf("error = %v, want source validation error", err)
	}
}
