package apiserver_test

import (
	"os"
	"path/filepath"
	"testing"

	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
)

const validWorkflowPreviewSource = `
meta({ name: "review", version: 1 });
phase("setup");
log("starting");
`

func writeWorkflowPreviewFixture(t *testing.T, projectRoot, name, content string) {
	t.Helper()
	workflowDir := filepath.Join(projectRoot, workflowsource.ProjectClaudeWorkflowsDir)
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		t.Fatalf("mkdir workflows: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workflowDir, name), []byte(content), 0o600); err != nil {
		t.Fatalf("write workflow: %v", err)
	}
}

func stringPtr(value string) *string {
	return &value
}
