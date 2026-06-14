package factorysession_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	mcpfactorysession "github.com/portpowered/infinite-you/pkg/mcp/factorysession"
	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
)

const validWorkflowSource = `
meta({ name: "review", version: 1 });
phase("setup");
log("starting");
`

func TestMockClient_ValidateSource_ValidFixtureReturnsSuccessPreview(t *testing.T) {
	projectRoot := writeWorkflowFixture(t, "review.js", validWorkflowSource)
	client := mcpfactorysession.NewClient()

	response, err := client.ValidateSource(factoryapi.FactoryPreviewRequest{
		SourceKind:  factoryapi.WORKFLOWNAME,
		ProjectRoot: &projectRoot,
		SourceValue: strPtr("review"),
	})
	if err != nil {
		t.Fatalf("ValidateSource: %v", err)
	}
	if response.Error != nil {
		t.Fatalf("error = %#v, want success result", response.Error)
	}
	if response.Result == nil {
		t.Fatal("result = nil, want Factory preview result")
	}
	if !response.Result.Valid {
		t.Fatal("valid = false, want true for fixture workflow")
	}
	if response.Result.SourceResolution.SourceHash == nil || strings.TrimSpace(*response.Result.SourceResolution.SourceHash) == "" {
		t.Fatal("sourceHash missing from valid preview result")
	}
	if response.Result.PolicyPreview.PolicyHash == "" {
		t.Fatal("policyPreview.policyHash missing from valid preview result")
	}
}

func TestMockClient_ValidateSource_InvalidFixtureReturnsStableValidationError(t *testing.T) {
	projectRoot := writeWorkflowFixture(t, "broken.js", "require('fs');")
	client := mcpfactorysession.NewClient()

	response, err := client.ValidateSource(factoryapi.FactoryPreviewRequest{
		SourceKind:  factoryapi.WORKFLOWNAME,
		ProjectRoot: &projectRoot,
		SourceValue: strPtr("broken"),
	})
	if err != nil {
		t.Fatalf("ValidateSource: %v", err)
	}
	if response.Result != nil {
		t.Fatalf("result = %#v, want validation error envelope", response.Result)
	}
	if response.Error == nil {
		t.Fatal("error = nil, want stable validation error envelope")
	}
	if response.Error.Code == "" || response.Error.Message == "" {
		t.Fatalf("error = %#v, want code and message", response.Error)
	}
	if response.Error.Retryable {
		t.Fatal("retryable = true, want false for validation failure")
	}
	if response.Error.Details == nil {
		t.Fatal("details = nil, want validation diagnostics")
	}
	issues, ok := response.Error.Details["sourceValidationIssues"].([]interface{})
	if !ok || len(issues) == 0 {
		t.Fatalf("details = %#v, want sourceValidationIssues diagnostics", response.Error.Details)
	}
}

func TestMockClient_ValidateSource_MissingSourceReturnsNotFoundDiagnostic(t *testing.T) {
	projectRoot := t.TempDir()
	client := mcpfactorysession.NewClient()

	response, err := client.ValidateSource(factoryapi.FactoryPreviewRequest{
		SourceKind:  factoryapi.WORKFLOWNAME,
		ProjectRoot: &projectRoot,
		SourceValue: strPtr("missing"),
	})
	if err != nil {
		t.Fatalf("ValidateSource: %v", err)
	}
	if response.Error == nil {
		t.Fatal("error = nil, want validation error envelope")
	}
	if response.Error.Code != workflowsource.CodeSourceNotFound {
		t.Fatalf("error code = %q, want %q", response.Error.Code, workflowsource.CodeSourceNotFound)
	}
	diagnostics, ok := response.Error.Details["sourceResolutionDiagnostics"].([]interface{})
	if !ok || len(diagnostics) == 0 {
		t.Fatalf("details = %#v, want sourceResolutionDiagnostics", response.Error.Details)
	}
}

func TestMockClient_ValidateSource_BadRequestReturnsStableEnvelope(t *testing.T) {
	client := mcpfactorysession.NewClient()

	response, err := client.ValidateSource(factoryapi.FactoryPreviewRequest{
		SourceKind: factoryapi.WORKFLOWNAME,
	})
	if err != nil {
		t.Fatalf("ValidateSource: %v", err)
	}
	if response.Error == nil {
		t.Fatal("error = nil, want request validation envelope")
	}
	if response.Error.Code != "BAD_REQUEST" {
		t.Fatalf("error code = %q, want BAD_REQUEST", response.Error.Code)
	}
	if response.Error.Retryable {
		t.Fatal("retryable = true, want false for request validation failure")
	}
	if !strings.Contains(response.Error.Message, "projectRoot") {
		t.Fatalf("message = %q, want projectRoot validation detail", response.Error.Message)
	}
}

func writeWorkflowFixture(t *testing.T, name, content string) string {
	t.Helper()
	projectRoot := t.TempDir()
	workflowDir := filepath.Join(projectRoot, workflowsource.ProjectClaudeWorkflowsDir)
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		t.Fatalf("mkdir workflows: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workflowDir, name), []byte(content), 0o600); err != nil {
		t.Fatalf("write workflow: %v", err)
	}
	return projectRoot
}
