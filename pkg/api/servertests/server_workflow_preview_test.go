package apiserver_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
)

const validWorkflowPreviewSource = `
meta({ name: "review", version: 1 });
phase("setup");
log("starting");
`

func TestPreviewWorkflow_ReturnsSharedPreviewContract(t *testing.T) {
	projectRoot := t.TempDir()
	writeWorkflowPreviewFixture(t, projectRoot, "review.js", validWorkflowPreviewSource)

	body, err := json.Marshal(factoryapi.WorkflowPreviewRequest{
		SourceKind:  factoryapi.WorkflowPreviewRequestSourceKindWORKFLOWNAME,
		ProjectRoot: stringPtr(projectRoot),
		SourceValue: stringPtr("review"),
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	srv := newAPITestServer(nil)
	req := httptest.NewRequest(http.MethodPost, "/workflow-previews", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /workflow-previews status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	result := decodeJSONResponse[factoryapi.WorkflowPreviewResult](t, rec)
	if !result.Valid {
		t.Fatalf("result = %#v, want valid preview", result)
	}
	if result.SourceResolution.SourceHash == nil || *result.SourceResolution.SourceHash == "" {
		t.Fatal("expected source hash")
	}
	if result.PolicyPreview.PolicyHash == "" {
		t.Fatal("expected policy hash")
	}
	if result.ResultConstraints.ArtifactUriScheme != "you-artifact" {
		t.Fatalf("artifact scheme = %q, want you-artifact", result.ResultConstraints.ArtifactUriScheme)
	}
}

func TestPreviewWorkflow_RejectsForbiddenHostAccess(t *testing.T) {
	projectRoot := t.TempDir()
	writeWorkflowPreviewFixture(t, projectRoot, "unsafe.js", "require('fs');")

	body, err := json.Marshal(factoryapi.WorkflowPreviewRequest{
		SourceKind:  factoryapi.WorkflowPreviewRequestSourceKindWORKFLOWNAME,
		ProjectRoot: stringPtr(projectRoot),
		SourceValue: stringPtr("unsafe"),
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	srv := newAPITestServer(nil)
	req := httptest.NewRequest(http.MethodPost, "/workflow-previews", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /workflow-previews status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	result := decodeJSONResponse[factoryapi.WorkflowPreviewResult](t, rec)
	if result.Valid {
		t.Fatal("expected invalid preview")
	}
	if len(result.SourceValidationIssues) == 0 {
		t.Fatalf("issues = %#v, want source validation diagnostics", result.SourceValidationIssues)
	}
}

func writeWorkflowPreviewFixture(t *testing.T, projectRoot, name, content string) {
	t.Helper()
	workflowDir := filepath.Join(projectRoot, source.ProjectClaudeWorkflowsDir)
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
