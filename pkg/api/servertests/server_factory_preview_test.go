package apiserver_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
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

func TestPreviewFactory_ReturnsCanonicalFactoryPreviewContract(t *testing.T) {
	projectRoot := t.TempDir()
	writeWorkflowPreviewFixture(t, projectRoot, "review.js", validWorkflowPreviewSource)

	body, err := json.Marshal(factoryapi.FactoryPreviewRequest{
		SourceKind:  factoryapi.WORKFLOWNAME,
		ProjectRoot: stringPtr(projectRoot),
		SourceValue: stringPtr("review"),
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	srv := newAPITestServer(nil)
	req := httptest.NewRequest(http.MethodPost, "/factories/preview", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /factories/preview status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Deprecation") != "" {
		t.Fatalf("Deprecation = %q, want empty for canonical route", rec.Header().Get("Deprecation"))
	}

	result := decodeJSONResponse[factoryapi.FactoryPreviewResult](t, rec)
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

func TestPreviewFactory_RejectsForbiddenHostAccess(t *testing.T) {
	projectRoot := t.TempDir()
	writeWorkflowPreviewFixture(t, projectRoot, "unsafe.js", "require('fs');")

	body, err := json.Marshal(factoryapi.FactoryPreviewRequest{
		SourceKind:  factoryapi.WORKFLOWNAME,
		ProjectRoot: stringPtr(projectRoot),
		SourceValue: stringPtr("unsafe"),
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	srv := newAPITestServer(nil)
	req := httptest.NewRequest(http.MethodPost, "/factories/preview", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /factories/preview status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	result := decodeJSONResponse[factoryapi.FactoryPreviewResult](t, rec)
	if result.Valid {
		t.Fatal("expected invalid preview")
	}
	if len(result.SourceValidationIssues) == 0 {
		t.Fatalf("issues = %#v, want source validation diagnostics", result.SourceValidationIssues)
	}
	wantPath := workflowsource.ProjectClaudeWorkflowsDir + "/unsafe.js"
	if result.SourceValidationIssues[0].Path == nil || strings.TrimSpace(*result.SourceValidationIssues[0].Path) != wantPath {
		t.Fatalf("issue path = %v, want %q", result.SourceValidationIssues[0].Path, wantPath)
	}
}

func TestPreviewWorkflowCompatibility_RejectsForbiddenHostAccessWithDeprecationHeaders(t *testing.T) {
	projectRoot := t.TempDir()
	writeWorkflowPreviewFixture(t, projectRoot, "unsafe.js", "require('fs');")

	body, err := json.Marshal(factoryapi.WorkflowPreviewRequest{
		SourceKind:  factoryapi.WORKFLOWNAME,
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
	if rec.Header().Get("Deprecation") != "true" {
		t.Fatalf("Deprecation = %q, want true for compatibility alias", rec.Header().Get("Deprecation"))
	}
	if got := rec.Header().Get("Link"); got != `</factories/preview>; rel="successor-version"` {
		t.Fatalf("Link = %q, want successor-version link to /factories/preview", got)
	}

	result := decodeJSONResponse[factoryapi.WorkflowPreviewResult](t, rec)
	if result.Valid {
		t.Fatal("expected invalid preview")
	}
	if len(result.SourceValidationIssues) == 0 {
		t.Fatalf("issues = %#v, want source validation diagnostics", result.SourceValidationIssues)
	}
	wantPath := workflowsource.ProjectClaudeWorkflowsDir + "/unsafe.js"
	if result.SourceValidationIssues[0].Path == nil || strings.TrimSpace(*result.SourceValidationIssues[0].Path) != wantPath {
		t.Fatalf("issue path = %v, want %q", result.SourceValidationIssues[0].Path, wantPath)
	}
}

func TestPreviewWorkflow_ReturnsSamePreviewBodyAsCanonicalFactoryPreview(t *testing.T) {
	projectRoot := t.TempDir()
	writeWorkflowPreviewFixture(t, projectRoot, "review.js", validWorkflowPreviewSource)

	body, err := json.Marshal(factoryapi.FactoryPreviewRequest{
		SourceKind:  factoryapi.WORKFLOWNAME,
		ProjectRoot: stringPtr(projectRoot),
		SourceValue: stringPtr("review"),
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	srv := newAPITestServer(nil)

	canonicalReq := httptest.NewRequest(http.MethodPost, "/factories/preview", bytes.NewReader(body))
	canonicalReq.Header.Set("Content-Type", "application/json")
	canonicalRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(canonicalRec, canonicalReq)
	if canonicalRec.Code != http.StatusOK {
		t.Fatalf("POST /factories/preview status = %d, want 200: %s", canonicalRec.Code, canonicalRec.Body.String())
	}

	aliasReq := httptest.NewRequest(http.MethodPost, "/workflow-previews", bytes.NewReader(body))
	aliasReq.Header.Set("Content-Type", "application/json")
	aliasRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(aliasRec, aliasReq)
	if aliasRec.Code != http.StatusOK {
		t.Fatalf("POST /workflow-previews status = %d, want 200: %s", aliasRec.Code, aliasRec.Body.String())
	}

	if canonicalRec.Body.String() != aliasRec.Body.String() {
		t.Fatalf("compatibility alias body = %s, want identical canonical body %s", aliasRec.Body.String(), canonicalRec.Body.String())
	}
}

func TestPreviewWorkflow_ReturnsCompatibilityAliasWithDeprecationHeaders(t *testing.T) {
	projectRoot := t.TempDir()
	writeWorkflowPreviewFixture(t, projectRoot, "review.js", validWorkflowPreviewSource)

	body, err := json.Marshal(factoryapi.WorkflowPreviewRequest{
		SourceKind:  factoryapi.WORKFLOWNAME,
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
	if rec.Header().Get("Deprecation") != "true" {
		t.Fatalf("Deprecation = %q, want true", rec.Header().Get("Deprecation"))
	}
	if got := rec.Header().Get("Link"); got != `</factories/preview>; rel="successor-version"` {
		t.Fatalf("Link = %q, want successor-version link to /factories/preview", got)
	}

	result := decodeJSONResponse[factoryapi.WorkflowPreviewResult](t, rec)
	if !result.Valid {
		t.Fatalf("result = %#v, want valid preview", result)
	}
}
