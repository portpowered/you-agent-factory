package workflowsource_test

import (
	"os"
	"path/filepath"
	"testing"

	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
	cliworkflowsource "github.com/portpowered/infinite-you/pkg/transports/cli/workflowsource"
)

const validWorkflowSource = `
meta({ name: "review", version: 1 });
phase("setup");
log("starting");
`

func TestNormalizeRequest_ResolvesWorkflowName(t *testing.T) {
	projectRoot := t.TempDir()
	workflowDir := filepath.Join(projectRoot, workflowsource.ProjectClaudeWorkflowsDir)
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		t.Fatalf("mkdir workflows: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workflowDir, "review.js"), []byte(validWorkflowSource), 0o600); err != nil {
		t.Fatalf("write workflow: %v", err)
	}

	ctx, err := workflowsource.DefaultContext(projectRoot)
	if err != nil {
		t.Fatalf("DefaultContext: %v", err)
	}

	resolution := cliworkflowsource.NormalizeRequest(workflowsource.Request{
		Kind:  workflowsource.KindWorkflowName,
		Value: "review",
	}, ctx)
	if !resolution.Found {
		t.Fatalf("resolution = %#v, want found workflow", resolution)
	}
	if resolution.SourceRef != workflowsource.ProjectClaudeWorkflowsDir+"/review.js" {
		t.Fatalf("source ref = %q", resolution.SourceRef)
	}
	if resolution.SourceHash == "" {
		t.Fatal("expected stable source hash")
	}
}

func TestNormalizeRequest_MissingWorkflowReportsNotFoundDiagnostic(t *testing.T) {
	projectRoot := t.TempDir()
	ctx, err := workflowsource.DefaultContext(projectRoot)
	if err != nil {
		t.Fatalf("DefaultContext: %v", err)
	}

	resolution := cliworkflowsource.NormalizeRequest(workflowsource.Request{
		Kind:  workflowsource.KindWorkflowName,
		Value: "missing",
	}, ctx)
	if resolution.Found {
		t.Fatalf("resolution = %#v, want not found", resolution)
	}
	if len(resolution.Diagnostics) == 0 {
		t.Fatal("expected source resolution diagnostics")
	}
	if resolution.Diagnostics[0].Code != workflowsource.CodeSourceNotFound {
		t.Fatalf("diagnostic code = %q, want %q", resolution.Diagnostics[0].Code, workflowsource.CodeSourceNotFound)
	}
}
