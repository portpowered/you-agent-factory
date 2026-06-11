package factorysessionexecution_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
)

func TestPrepareStart_ResolvesSourceAndPolicyForSimpleWorkflow(t *testing.T) {
	projectRoot := t.TempDir()
	workflowDir := filepath.Join(projectRoot, workflowsource.ProjectClaudeWorkflowsDir)
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		t.Fatalf("mkdir workflows: %v", err)
	}
	source := readRuntimeFixture(t, "simple-final.workflow.js")
	if err := os.WriteFile(filepath.Join(workflowDir, "simple-final.js"), []byte(source), 0o600); err != nil {
		t.Fatalf("write workflow: %v", err)
	}

	prepared, err := factorysessionexecution.PrepareStart(factorysessionexecution.StartRequest{
		RequestID: "req-prepare-simple",
		Source: factorysessionexecution.Source{
			Kind:         workflowsource.KindWorkflowName,
			WorkflowName: "simple-final",
		},
		Args: map[string]any{
			"subject": "workflows",
			"count":   3,
			"prefix":  "you",
		},
		RequestedPolicy: map[string]any{
			"mode": "READ_ONLY",
		},
	}, factorysessionexecution.StartPrepareContext{
		StartSourceContext: factorysessionexecution.StartSourceContext{ProjectRoot: projectRoot},
	})
	if err != nil {
		t.Fatalf("PrepareStart: %v", err)
	}
	if prepared.ResolvedSource.SourceHash == "" {
		t.Fatal("expected resolved source hash")
	}
	if prepared.ResolvedSource.SourceRef != workflowsource.ProjectClaudeWorkflowsDir+"/simple-final.js" {
		t.Fatalf("sourceRef = %q", prepared.ResolvedSource.SourceRef)
	}
	if prepared.Policy.EffectiveHash == "" {
		t.Fatal("expected effective policy hash")
	}
	if len(prepared.Policy.Effective) == 0 {
		t.Fatal("expected effective policy projection")
	}
	if prepared.SourceContent == "" {
		t.Fatal("expected executable source content")
	}
	if prepared.TupleHash == "" {
		t.Fatal("expected idempotency tuple hash")
	}
}

func TestPrepareStart_RejectsInvalidSource(t *testing.T) {
	projectRoot := t.TempDir()
	workflowDir := filepath.Join(projectRoot, workflowsource.ProjectClaudeWorkflowsDir)
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		t.Fatalf("mkdir workflows: %v", err)
	}
	source := readRuntimeFixture(t, "syntax-error.workflow.js")
	if err := os.WriteFile(filepath.Join(workflowDir, "syntax-error.js"), []byte(source), 0o600); err != nil {
		t.Fatalf("write workflow: %v", err)
	}

	_, err := factorysessionexecution.PrepareStart(factorysessionexecution.StartRequest{
		RequestID: "req-prepare-invalid-source",
		Source: factorysessionexecution.Source{
			Kind:         workflowsource.KindWorkflowName,
			WorkflowName: "syntax-error",
		},
	}, factorysessionexecution.StartPrepareContext{
		StartSourceContext: factorysessionexecution.StartSourceContext{ProjectRoot: projectRoot},
	})
	if err == nil {
		t.Fatal("error = nil, want source validation error")
	}
	var validationErr *factorysessionexecution.ValidationError
	if !errors.As(err, &validationErr) || validationErr.Field != "source" {
		t.Fatalf("error = %v, want source validation error", err)
	}
}

func TestPrepareStart_RejectsInvalidArgs(t *testing.T) {
	projectRoot := t.TempDir()
	workflowDir := filepath.Join(projectRoot, workflowsource.ProjectClaudeWorkflowsDir)
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		t.Fatalf("mkdir workflows: %v", err)
	}
	source := readRuntimeFixture(t, "simple-final.workflow.js")
	if err := os.WriteFile(filepath.Join(workflowDir, "simple-final.js"), []byte(source), 0o600); err != nil {
		t.Fatalf("write workflow: %v", err)
	}

	_, err := factorysessionexecution.PrepareStart(factorysessionexecution.StartRequest{
		RequestID: "req-prepare-invalid-args",
		Source: factorysessionexecution.Source{
			Kind:         workflowsource.KindWorkflowName,
			WorkflowName: "simple-final",
		},
		Args: map[string]any{
			"bad": make(chan int),
		},
	}, factorysessionexecution.StartPrepareContext{
		StartSourceContext: factorysessionexecution.StartSourceContext{ProjectRoot: projectRoot},
	})
	if err == nil {
		t.Fatal("error = nil, want args validation error")
	}
	var validationErr *factorysessionexecution.ValidationError
	if !errors.As(err, &validationErr) || validationErr.Field != "args" {
		t.Fatalf("error = %v, want args validation error", err)
	}
}

func TestPrepareStart_RejectsDeniedPolicy(t *testing.T) {
	projectRoot := t.TempDir()
	workflowDir := filepath.Join(projectRoot, workflowsource.ProjectClaudeWorkflowsDir)
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		t.Fatalf("mkdir workflows: %v", err)
	}
	source := readRuntimeFixture(t, "simple-final.workflow.js")
	if err := os.WriteFile(filepath.Join(workflowDir, "simple-final.js"), []byte(source), 0o600); err != nil {
		t.Fatalf("write workflow: %v", err)
	}

	_, err := factorysessionexecution.PrepareStart(factorysessionexecution.StartRequest{
		RequestID: "req-prepare-denied-policy",
		Source: factorysessionexecution.Source{
			Kind:         workflowsource.KindWorkflowName,
			WorkflowName: "simple-final",
		},
		RequestedPolicy: map[string]any{
			"allowNetwork": true,
		},
	}, factorysessionexecution.StartPrepareContext{
		StartSourceContext: factorysessionexecution.StartSourceContext{ProjectRoot: projectRoot},
	})
	if err == nil {
		t.Fatal("error = nil, want requestedPolicy validation error")
	}
	var validationErr *factorysessionexecution.ValidationError
	if !errors.As(err, &validationErr) || validationErr.Field != "requestedPolicy" {
		t.Fatalf("error = %v, want requestedPolicy validation error", err)
	}
}

func TestPrepareStart_RejectsInvalidWait(t *testing.T) {
	projectRoot := t.TempDir()
	workflowDir := filepath.Join(projectRoot, workflowsource.ProjectClaudeWorkflowsDir)
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		t.Fatalf("mkdir workflows: %v", err)
	}
	source := readRuntimeFixture(t, "simple-final.workflow.js")
	if err := os.WriteFile(filepath.Join(workflowDir, "simple-final.js"), []byte(source), 0o600); err != nil {
		t.Fatalf("write workflow: %v", err)
	}

	invalidTimeout := int64(0)
	_, err := factorysessionexecution.PrepareStart(factorysessionexecution.StartRequest{
		RequestID: "req-prepare-invalid-wait",
		Source: factorysessionexecution.Source{
			Kind:         workflowsource.KindWorkflowName,
			WorkflowName: "simple-final",
		},
		Wait: &factorysessionexecution.WaitOptions{
			TimeoutMillis: &invalidTimeout,
		},
	}, factorysessionexecution.StartPrepareContext{
		StartSourceContext: factorysessionexecution.StartSourceContext{ProjectRoot: projectRoot},
	})
	if err == nil {
		t.Fatal("error = nil, want wait validation error")
	}
	var validationErr *factorysessionexecution.ValidationError
	if !errors.As(err, &validationErr) || validationErr.Field != "wait.timeoutMillis" {
		t.Fatalf("error = %v, want wait.timeoutMillis validation error", err)
	}
}

func readRuntimeFixture(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join("..", "..", "orchestrators", "javascript", "runtime", "testdata", name)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return string(raw)
}
