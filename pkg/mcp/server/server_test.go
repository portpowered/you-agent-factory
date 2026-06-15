package server_test

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	mcpfactorysession "github.com/portpowered/infinite-you/pkg/mcp/factorysession"
	mcpserver "github.com/portpowered/infinite-you/pkg/mcp/server"
	mcpworkflow "github.com/portpowered/infinite-you/pkg/mcp/workflow"
	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
)

func TestNewPreviewServer_AdvertisesWorkflowPreviewCatalog(t *testing.T) {
	ctx := context.Background()
	serverTransport, clientTransport := sdkmcp.NewInMemoryTransports()

	server := mcpserver.NewPreviewServer()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("connect server: %v", err)
	}
	defer serverSession.Close()

	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "test-client", Version: "1.0.0"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("connect client: %v", err)
	}
	defer clientSession.Close()

	listResult, err := clientSession.ListTools(ctx, &sdkmcp.ListToolsParams{})
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}

	wantNames := mcpworkflow.ToolNames()
	gotNames := make([]string, len(listResult.Tools))
	for i, tool := range listResult.Tools {
		gotNames[i] = tool.Name
	}
	slices.Sort(gotNames)
	sortedWant := append([]string(nil), wantNames...)
	slices.Sort(sortedWant)
	if !slices.Equal(gotNames, sortedWant) {
		t.Fatalf("listed tools = %#v, want %#v", gotNames, sortedWant)
	}
}

func TestServeStdio_DiscoverToolsOverOwnedTransport(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	serverReader, clientWriter := io.Pipe()
	clientReader, serverWriter := io.Pipe()

	errCh := make(chan error, 1)
	go func() {
		errCh <- mcpserver.ServeStdio(ctx, serverReader, serverWriter)
	}()

	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "test-client", Version: "1.0.0"}, nil)
	transport := &sdkmcp.IOTransport{
		Reader: clientReader,
		Writer: clientWriter,
	}
	clientSession, err := client.Connect(ctx, transport, nil)
	if err != nil {
		t.Fatalf("connect client: %v", err)
	}
	defer clientSession.Close()

	listResult, err := clientSession.ListTools(ctx, &sdkmcp.ListToolsParams{})
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	if len(listResult.Tools) != 2 {
		t.Fatalf("tool count = %d, want 2", len(listResult.Tools))
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

	cancel()
	if err := <-errCh; err != nil && !strings.Contains(err.Error(), "context canceled") {
		t.Fatalf("serve stdio: %v", err)
	}
}

func TestServeStdio_ValidateSourceToolReturnsStructuredEnvelope(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	serverReader, clientWriter := io.Pipe()
	clientReader, serverWriter := io.Pipe()

	go func() {
		_ = mcpserver.ServeStdio(ctx, serverReader, serverWriter)
	}()

	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "test-client", Version: "1.0.0"}, nil)
	transport := &sdkmcp.IOTransport{
		Reader: clientReader,
		Writer: clientWriter,
	}
	clientSession, err := client.Connect(ctx, transport, nil)
	if err != nil {
		t.Fatalf("connect client: %v", err)
	}
	defer clientSession.Close()

	projectRoot := t.TempDir()
	writeWorkflow(t, projectRoot, "review.js", `
meta({ name: "review", version: 1 });
phase("setup");
log("starting");
`)

	callResult, err := clientSession.CallTool(ctx, &sdkmcp.CallToolParams{
		Name: mcpfactorysession.ToolValidateSource,
		Arguments: map[string]any{
			"sourceKind":  string(factoryapi.WORKFLOWNAME),
			"projectRoot": projectRoot,
			"sourceValue": "review",
		},
	})
	if err != nil {
		t.Fatalf("call validate tool: %v", err)
	}
	if callResult.IsError {
		t.Fatalf("call result marked error: %#v", callResult)
	}
	if len(callResult.Content) == 0 {
		t.Fatal("expected text content in tool result")
	}
	text, ok := callResult.Content[0].(*sdkmcp.TextContent)
	if !ok {
		t.Fatalf("content type = %T, want *mcp.TextContent", callResult.Content[0])
	}

	var response mcpfactorysession.ToolResponse[struct {
		Valid bool `json:"valid"`
	}]
	if err := json.Unmarshal([]byte(text.Text), &response); err != nil {
		t.Fatalf("unmarshal tool response: %v", err)
	}
	if response.Result == nil || !response.Result.Valid {
		t.Fatalf("response = %#v, want valid preview result", response)
	}
}

func writeWorkflow(t *testing.T, projectRoot, name, content string) {
	t.Helper()
	workflowDir := filepath.Join(projectRoot, workflowsource.ProjectClaudeWorkflowsDir)
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		t.Fatalf("mkdir workflows: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workflowDir, name), []byte(content), 0o600); err != nil {
		t.Fatalf("write workflow: %v", err)
	}
}
