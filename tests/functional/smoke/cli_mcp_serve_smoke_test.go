package smoke

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	mcpfactorysession "github.com/portpowered/infinite-you/pkg/mcp/factorysession"
	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
	workflowvalidation "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/validation"
)

const mcpServeValidWorkflowSource = `
meta({ name: "review", version: 1 });
phase("setup");
log("starting");
`

func TestMCPServe_RealCLI_DiscoverToolsAndPreviewWorkflows(t *testing.T) {
	if testing.Short() {
		t.Skip("slow CLI MCP serve smoke")
	}

	projectRoot := t.TempDir()
	writeMCPWorkflow(t, projectRoot, "review.js", mcpServeValidWorkflowSource)
	writeMCPWorkflow(t, projectRoot, "broken.js", "require('fs');")

	binaryPath := buildYouCLIBinary(t)
	session, cleanup := connectRealMCPServeCLI(t, binaryPath, projectRoot)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	assertMCPServeDiscoversPreviewTools(t, ctx, session)
	assertMCPServeValidateSourceSucceeds(t, ctx, session, projectRoot, "review")
	assertMCPServeStartPreviewSucceeds(t, ctx, session, projectRoot, "review")
	assertMCPServeValidateSourceReturnsStructuredInvalidPreview(
		t, ctx, session, projectRoot, "broken", workflowvalidation.CodeForbiddenHostAccess,
	)
	assertMCPServeValidateSourceReturnsStructuredInvalidPreview(
		t, ctx, session, projectRoot, "missing", workflowsource.CodeSourceNotFound,
	)
}

func connectRealMCPServeCLI(t *testing.T, binaryPath, cwd string) (*sdkmcp.ClientSession, func()) {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, binaryPath, "mcp", "serve")
	cmd.Dir = cwd

	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		t.Fatalf("stdin pipe: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		t.Fatalf("stdout pipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		cancel()
		t.Fatalf("start you mcp serve: %v", err)
	}

	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "smoke-client", Version: "1.0.0"}, nil)
	transport := &sdkmcp.IOTransport{
		Reader: stdout,
		Writer: stdin,
	}
	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		cancel()
		_ = cmd.Wait()
		t.Fatalf("connect MCP client to you mcp serve: %v", err)
	}

	cleanup := func() {
		_ = session.Close()
		cancel()
		_ = stdin.Close()
		_ = cmd.Wait()
	}
	return session, cleanup
}

func assertMCPServeDiscoversPreviewTools(t *testing.T, ctx context.Context, session *sdkmcp.ClientSession) {
	t.Helper()

	listResult, err := session.ListTools(ctx, &sdkmcp.ListToolsParams{})
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	gotNames := make([]string, len(listResult.Tools))
	for i, tool := range listResult.Tools {
		gotNames[i] = tool.Name
	}
	slices.Sort(gotNames)
	wantNames := []string{mcpfactorysession.ToolValidateSource, mcpfactorysession.ToolStartPreview}
	slices.Sort(wantNames)
	if !slices.Equal(gotNames, wantNames) {
		t.Fatalf("listed tools = %#v, want %#v", gotNames, wantNames)
	}
}

func assertMCPServeValidateSourceSucceeds(
	t *testing.T,
	ctx context.Context,
	session *sdkmcp.ClientSession,
	projectRoot, workflowName string,
) {
	t.Helper()
	response := callMCPServePreviewTool(
		t, ctx, session, mcpfactorysession.ToolValidateSource, projectRoot, workflowName,
	)
	if response.Error != nil {
		t.Fatalf("validate response error = %#v, want success result", response.Error)
	}
	if response.Result == nil || !response.Result.Valid {
		t.Fatalf("validate response = %#v, want valid preview result", response)
	}
}

func assertMCPServeStartPreviewSucceeds(
	t *testing.T,
	ctx context.Context,
	session *sdkmcp.ClientSession,
	projectRoot, workflowName string,
) {
	t.Helper()
	response := callMCPServePreviewTool(
		t, ctx, session, mcpfactorysession.ToolStartPreview, projectRoot, workflowName,
	)
	if response.Error != nil {
		t.Fatalf("start preview response error = %#v, want success result", response.Error)
	}
	if response.Result == nil || !response.Result.Valid {
		t.Fatalf("start preview response = %#v, want valid preview result", response)
	}
}

func assertMCPServeValidateSourceReturnsStructuredInvalidPreview(
	t *testing.T,
	ctx context.Context,
	session *sdkmcp.ClientSession,
	projectRoot, workflowName, wantCode string,
) {
	t.Helper()
	response := callMCPServePreviewTool(
		t, ctx, session, mcpfactorysession.ToolValidateSource, projectRoot, workflowName,
	)
	if response.Result != nil {
		t.Fatalf("validate response result = %#v, want typed validation error envelope", response.Result)
	}
	if response.Error == nil {
		t.Fatal("expected typed validation error envelope")
	}
	if response.Error.Code != wantCode {
		t.Fatalf("error code = %q, want %q", response.Error.Code, wantCode)
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
}

func callMCPServePreviewTool(
	t *testing.T,
	ctx context.Context,
	session *sdkmcp.ClientSession,
	toolName, projectRoot, workflowName string,
) mcpfactorysession.ToolResponse[factoryapi.FactoryPreviewResult] {
	t.Helper()

	callResult, err := session.CallTool(ctx, &sdkmcp.CallToolParams{
		Name: toolName,
		Arguments: map[string]any{
			"sourceKind":  string(factoryapi.WORKFLOWNAME),
			"projectRoot": projectRoot,
			"sourceValue": workflowName,
		},
	})
	if err != nil {
		t.Fatalf("call %s: %v", toolName, err)
	}
	text := decodeMCPServeToolText(t, callResult)
	var response mcpfactorysession.ToolResponse[factoryapi.FactoryPreviewResult]
	if err := json.Unmarshal([]byte(text), &response); err != nil {
		t.Fatalf("unmarshal %s response: %v", toolName, err)
	}
	return response
}

func decodeMCPServeToolText(t *testing.T, callResult *sdkmcp.CallToolResult) string {
	t.Helper()
	if len(callResult.Content) == 0 {
		t.Fatal("expected text content in tool result")
	}
	text, ok := callResult.Content[0].(*sdkmcp.TextContent)
	if !ok {
		t.Fatalf("content type = %T, want *mcp.TextContent", callResult.Content[0])
	}
	return text.Text
}

func writeMCPWorkflow(t *testing.T, projectRoot, name, content string) {
	t.Helper()
	workflowDir := filepath.Join(projectRoot, workflowsource.ProjectClaudeWorkflowsDir)
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		t.Fatalf("mkdir workflows: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workflowDir, name), []byte(content), 0o600); err != nil {
		t.Fatalf("write workflow: %v", err)
	}
}
