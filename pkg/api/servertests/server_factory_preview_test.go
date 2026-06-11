package apiserver_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
)

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
