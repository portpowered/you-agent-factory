package factorysession_test

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	mcpfactorysession "github.com/portpowered/infinite-you/pkg/transports/mcp/factorysession"
	mcpgenerated "github.com/portpowered/infinite-you/pkg/transports/mcp/generated"
)

var mcpWorkflowDefinitions = testutil.ScriptedJavaScriptWorkflowDefinitions{
	DefaultSourceContextFunc: func(root string) (factory.WorkflowSourceContext, error) {
		return factory.WorkflowSourceContext{ProjectRoot: root}, nil
	},
	BuildPreviewFunc: func(request factory.WorkflowPreviewRequest) factory.WorkflowPreview {
		sourceRef := factory.WorkflowSourceProjectClaudeWorkflowsDir + "/" + request.Source.Value + ".js"
		preview := factory.WorkflowPreview{
			Valid: true,
			SourceResolution: factory.WorkflowSourceResolution{
				RequestKind:  request.Source.Kind,
				RequestValue: request.Source.Value,
				ResolvedKind: request.Source.Kind,
				SourceRef:    sourceRef,
				SourceHash:   "sha256:mcp-workflow-source",
				Found:        true,
				ArtifactRoot: factory.WorkflowSourceArtifactRootDecision{Allowed: true},
			},
			PolicyPreview: factory.JavaScriptPolicyPreview{PolicyHash: "sha256:mcp-workflow-policy"},
		}
		if request.Source.Value == "broken" {
			preview.Valid = false
			preview.SourceValidationIssues = []factory.WorkflowPreviewSourceValidationIssue{{
				Code:    factory.WorkflowValidationCodeForbiddenHostAccess,
				Message: "host filesystem access is unavailable",
				Path:    sourceRef,
			}}
		}
		return preview
	},
}

func TestStableIDHandlerRegistryCoversGeneratedCanonicalDiscovery(t *testing.T) {
	t.Parallel()

	for _, tool := range mcpgenerated.PrimaryDiscovery() {
		binding, ok := mcpfactorysession.ResolveToolHandlerBinding(tool.Name)
		if !ok {
			t.Fatalf("generated tool %q (%s) has no handwritten handler binding", tool.Name, tool.ID)
		}
		if binding.ToolID != tool.ID {
			t.Fatalf("tool %q binding tool ID = %q, want generated %q", tool.Name, binding.ToolID, tool.ID)
		}
		wantHandlerID := strings.Replace(tool.ID, "mcp.tool.", "mcp.handler.", 1)
		if binding.HandlerID != wantHandlerID {
			t.Fatalf("tool %q handler ID = %q, want contracted %q", tool.Name, binding.HandlerID, wantHandlerID)
		}
		if !mcpfactorysession.IsCanonicalToolHandlerRegistered(tool.Name) {
			t.Fatalf("generated tool %q is not registered as canonical", tool.Name)
		}
	}
}

func TestStableIDHandlerRegistryMatchesAuthoredHandlerBindings(t *testing.T) {
	t.Parallel()

	payload, err := os.ReadFile(testutil.MustRepoPath(t, "contracts/mcp/tools.json"))
	if err != nil {
		t.Fatalf("read authored MCP catalog: %v", err)
	}
	var catalog struct {
		Tools map[string]struct {
			Name    string `json:"name"`
			Handler struct {
				ID string `json:"id"`
			} `json:"handler"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(payload, &catalog); err != nil {
		t.Fatalf("decode authored MCP catalog: %v", err)
	}

	for toolID, authored := range catalog.Tools {
		binding, ok := mcpfactorysession.ResolveToolHandlerBinding(authored.Name)
		if !ok {
			t.Fatalf("authored tool %q (%s) has no handwritten handler binding", authored.Name, toolID)
		}
		if binding.ToolID != toolID || binding.HandlerID != authored.Handler.ID {
			t.Fatalf("tool %q binding = %#v, want tool ID %q and handler ID %q", authored.Name, binding, toolID, authored.Handler.ID)
		}
	}
}

func TestStableIDHandlerRegistryPreservesSuccessAndDomainErrorOutcomes(t *testing.T) {
	t.Parallel()

	client := clientWithScript(scriptedExecutionService{
		listSessions: func(context.Context, factorysessions.ListSessionsRequest) (factorysessions.ListSessionsResult, error) {
			return persistedSessionList(), nil
		},
		getSession: func(context.Context, string) (factorysessions.SessionReadResult, error) {
			return factorysessions.SessionReadResult{}, factorysessions.ErrDurableSessionNotFound
		},
	})
	successRaw, err := client.CallTool(context.Background(), mcpfactorysession.ToolListSessions, json.RawMessage(`{"scope":"persisted"}`))
	if err != nil {
		t.Fatalf("CallTool(success) error = %v", err)
	}
	if !strings.Contains(string(successRaw), `"durableSessions"`) {
		t.Fatalf("CallTool(success) = %s, want durable session result", successRaw)
	}

	domainErrorRaw, err := client.CallTool(context.Background(), mcpfactorysession.ToolGetSession, json.RawMessage(`{"sessionId":"missing-session"}`))
	if err != nil {
		t.Fatalf("CallTool(domain error) transport error = %v", err)
	}
	if !strings.Contains(string(domainErrorRaw), `"error"`) || !strings.Contains(string(domainErrorRaw), `"retryable":false`) {
		t.Fatalf("CallTool(domain error) = %s, want typed non-retryable error envelope", domainErrorRaw)
	}
}

func TestValidateSource_ValidSimpleWorkflowFixtureReturnsDeterministicSuccess(t *testing.T) {
	projectRoot := t.TempDir()

	request := workflowNamePreviewRequest(projectRoot, "review")
	response := mcpfactorysession.ValidateSource(context.Background(), mcpWorkflowDefinitions, request)

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

	ctx, err := mcpWorkflowDefinitions.DefaultSourceContext(projectRoot)
	if err != nil {
		t.Fatalf("DefaultContext: %v", err)
	}
	expected := apisurface.FactoryPreviewResultFromPreview(mcpWorkflowDefinitions.BuildPreview(factory.WorkflowPreviewRequest{
		Source: factory.WorkflowSourceRequest{
			Kind:  factory.WorkflowSourceKindWorkflowName,
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

	request := workflowNamePreviewRequest(projectRoot, "broken")
	response := mcpfactorysession.ValidateSource(context.Background(), mcpWorkflowDefinitions, request)

	if response.Result != nil {
		t.Fatalf("response result = %#v, want typed validation error envelope", response.Result)
	}
	if response.Error == nil {
		t.Fatal("expected typed validation error envelope")
	}
	if response.Error.Code != factory.WorkflowValidationCodeForbiddenHostAccess {
		t.Fatalf("error code = %q, want %q", response.Error.Code, factory.WorkflowValidationCodeForbiddenHostAccess)
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
	wantPath := factory.WorkflowSourceProjectClaudeWorkflowsDir + "/broken.js"
	if issuesRaw[0].Path == nil || strings.TrimSpace(*issuesRaw[0].Path) != wantPath {
		t.Fatalf("issue path = %v, want %q", issuesRaw[0].Path, wantPath)
	}
}

func TestMockClient_ValidateSourceRoundTripDoesNotSurfaceTransportFailureForExpectedValidationErrors(t *testing.T) {
	projectRoot := t.TempDir()

	request := workflowNamePreviewRequest(projectRoot, "broken")
	encoded, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	client := newTestClientWithWorkflows(mcpWorkflowDefinitions)
	raw, err := client.CallTool(context.Background(), mcpfactorysession.ToolValidateSource, encoded)
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
