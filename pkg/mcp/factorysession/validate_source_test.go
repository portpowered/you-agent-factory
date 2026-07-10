package factorysession_test

import (
	"encoding/json"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	mcpfactorysession "github.com/portpowered/infinite-you/pkg/mcp/factorysession"
	workflowpreview "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/preview"
	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
	workflowvalidation "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/validation"
)

const simpleValidWorkflowSource = `
meta({ name: "review", version: 1 });
phase("setup");
log("starting");
`

func TestValidateSource_ValidSimpleWorkflowFixtureReturnsDeterministicSuccess(t *testing.T) {
	projectRoot := t.TempDir()
	writeWorkflow(t, projectRoot, "review.js", simpleValidWorkflowSource)

	request := workflowNamePreviewRequest(projectRoot, "review")
	response := mcpfactorysession.ValidateSource(request)

	if response.Error != nil {
		t.Fatalf("response error = %#v, want success result", response.Error)
	}
	if response.Result == nil {
		t.Fatal("expected typed success result")
	}
	if !response.Result.Valid {
		t.Fatalf("result.valid = false, want true; preview = %#v", response.Result)
	}
	if response.Result.SourceResolution.SourceHash == nil || strings.TrimSpace(*response.Result.SourceResolution.SourceHash) == "" {
		t.Fatal("expected normalized sourceHash in success response")
	}
	if strings.TrimSpace(response.Result.PolicyPreview.PolicyHash) == "" {
		t.Fatal("expected policyHash in success response")
	}

	ctx, err := workflowsource.DefaultContext(projectRoot)
	if err != nil {
		t.Fatalf("DefaultContext: %v", err)
	}
	expected := apisurface.FactoryPreviewResultFromPreview(apisurface.BuildFactoryPreview(workflowpreview.Request{
		Source: workflowsource.Request{
			Kind:  workflowsource.KindWorkflowName,
			Value: "review",
		},
		Context: ctx,
	}))
	if response.Result.Valid != expected.Valid || deref(response.Result.SourceResolution.SourceHash) != deref(expected.SourceResolution.SourceHash) {
		t.Fatalf("mcp = %#v, api surface = %#v", response.Result, expected)
	}
}

func TestValidateSource_InvalidWorkflowFixtureReturnsTypedValidationFailure(t *testing.T) {
	projectRoot := t.TempDir()
	writeWorkflow(t, projectRoot, "broken.js", "require('fs');")

	request := workflowNamePreviewRequest(projectRoot, "broken")
	response := mcpfactorysession.ValidateSource(request)

	if response.Result != nil {
		t.Fatalf("response result = %#v, want typed validation error envelope", response.Result)
	}
	if response.Error == nil {
		t.Fatal("expected typed validation error envelope")
	}
	if response.Error.Code != workflowvalidation.CodeForbiddenHostAccess {
		t.Fatalf("error code = %q, want %q", response.Error.Code, workflowvalidation.CodeForbiddenHostAccess)
	}
	if strings.TrimSpace(response.Error.Message) == "" {
		t.Fatal("expected stable validation error message")
	}
	if response.Error.Retryable {
		t.Fatal("validation failure should not be retryable")
	}
	valid, ok := response.Error.Details["valid"].(bool)
	if !ok || valid {
		t.Fatalf("error details valid = %#v, want false", response.Error.Details["valid"])
	}
	issuesRaw, ok := response.Error.Details["sourceValidationIssues"].([]factoryapi.WorkflowDiagnostic)
	if !ok || len(issuesRaw) == 0 {
		t.Fatalf("error details issues = %#v, want source validation issues", response.Error.Details["sourceValidationIssues"])
	}
	wantPath := workflowsource.ProjectClaudeWorkflowsDir + "/broken.js"
	if issuesRaw[0].Path == nil || strings.TrimSpace(*issuesRaw[0].Path) != wantPath {
		t.Fatalf("issue path = %v, want %q", issuesRaw[0].Path, wantPath)
	}
}

func TestMockClient_ValidateSourceRoundTripDoesNotSurfaceTransportFailureForExpectedValidationErrors(t *testing.T) {
	projectRoot := t.TempDir()
	writeWorkflow(t, projectRoot, "broken.js", "require('fs');")

	request := workflowNamePreviewRequest(projectRoot, "broken")
	encoded, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	client := mcpfactorysession.NewClient()
	raw, err := client.CallTool(mcpfactorysession.ToolValidateSource, encoded)
	if err != nil {
		t.Fatalf("CallTool returned transport error for expected validation failure: %v", err)
	}

	var response mcpfactorysession.ToolResponse[factoryapi.FactoryPreviewResult]
	if err := json.Unmarshal(raw, &response); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if response.Error == nil || response.Result != nil {
		t.Fatalf("response = %#v, want typed validation error envelope", response)
	}
}

func TestMockClient_WorkflowValidateCompatibilityOnlyAliasMatchesCanonicalSuccess(t *testing.T) {
	projectRoot := t.TempDir()
	writeWorkflow(t, projectRoot, "review.js", simpleValidWorkflowSource)

	request := workflowNamePreviewRequest(projectRoot, "review")
	encoded, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	client := mcpfactorysession.NewClient()
	canonicalRaw, err := client.CallTool(mcpfactorysession.ToolValidateSource, encoded)
	if err != nil {
		t.Fatalf("canonical validate: %v", err)
	}
	aliasRaw, err := client.CallTool(mcpfactorysession.ToolWorkflowValidate, encoded)
	if err != nil {
		t.Fatalf("alias validate: %v", err)
	}
	if string(canonicalRaw) != string(aliasRaw) {
		t.Fatalf("alias response = %s, want canonical %s", aliasRaw, canonicalRaw)
	}

	var response mcpfactorysession.ToolResponse[factoryapi.FactoryPreviewResult]
	if err := json.Unmarshal(aliasRaw, &response); err != nil {
		t.Fatalf("unmarshal alias response: %v", err)
	}
	if response.Error != nil || response.Result == nil || !response.Result.Valid {
		t.Fatalf("response = %#v, want valid success result", response)
	}
}

func workflowNamePreviewRequest(projectRoot, workflowName string) factoryapi.FactoryPreviewRequest {
	projectRootPtr := projectRoot
	sourceValue := workflowName
	return factoryapi.FactoryPreviewRequest{
		SourceKind:  factoryapi.WORKFLOWNAME,
		ProjectRoot: &projectRootPtr,
		SourceValue: &sourceValue,
	}
}

func deref(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
