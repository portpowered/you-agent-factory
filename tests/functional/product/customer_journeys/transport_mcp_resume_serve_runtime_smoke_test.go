package customer_journeys_test

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	mcpfactorysession "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/mcp"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	mcpgenerated "github.com/portpowered/infinite-you/pkg/transports/mcp/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestMCPResumePackage_PublicCatalogAndSessionReadUseFlattenedRuntime(t *testing.T) {
	process, err := support.BuildProcessWithContext(context.Background(), serviceedges.Edges{})
	if err != nil {
		t.Fatalf("build application process: %v", err)
	}

	workDir := t.TempDir()
	homeDir := filepath.Join(workDir, "home")
	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		t.Fatalf("create isolated home: %v", err)
	}
	stdinRead, stdinWrite, err := os.Pipe()
	if err != nil {
		t.Fatalf("create MCP stdin: %v", err)
	}
	stdoutRead, stdoutWrite, err := os.Pipe()
	if err != nil {
		_ = stdinRead.Close()
		_ = stdinWrite.Close()
		t.Fatalf("create MCP stdout: %v", err)
	}
	t.Cleanup(func() {
		_ = stdinRead.Close()
		_ = stdinWrite.Close()
		_ = stdoutRead.Close()
		_ = stdoutWrite.Close()
		closeCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := process.Close(closeCtx); err != nil {
			t.Errorf("close application process: %v", err)
		}
	})

	serveCtx, cancelServe := context.WithCancel(context.Background())
	serveErr := make(chan error, 1)
	go func() {
		serveErr <- process.Execute(root.Input{
			Args:             []string{"you", "server", "mcp"},
			Env:              append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir),
			Stdin:            stdinRead,
			Stdout:           stdoutWrite,
			Stderr:           io.Discard,
			Context:          serveCtx,
			WorkingDirectory: workDir,
		})
	}()
	client := newStdioMCPClient(t, stdinWrite, stdoutRead)
	t.Cleanup(func() {
		cancelServe()
		_ = stdinWrite.Close()
		select {
		case err := <-serveErr:
			if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, io.EOF) {
				t.Errorf("MCP serve: %v", err)
			}
		case <-time.After(15 * time.Second):
			t.Error("MCP server did not stop after cancellation")
		}
	})

	assertMCPInitialized(t, client)
	toolNames := toolNamesFromListResult(t, client.call(t, "tools/list", map[string]any{}).Result)
	wantNames := make([]string, 0, len(mcpgenerated.PrimaryDiscovery()))
	for _, definition := range mcpgenerated.PrimaryDiscovery() {
		wantNames = append(wantNames, definition.Name)
	}
	if !slices.Equal(toolNames, wantNames) {
		t.Fatalf("tools/list = %#v, want generated catalog %#v", toolNames, wantNames)
	}
	missing := decodeToolResponse[factoryapi.FactorySessionDurableReadModel](
		t,
		client.callTool(t, mcpfactorysession.ToolGetSession, map[string]any{"sessionId": "missing-session"}),
	)
	if missing.Result != nil || missing.Error == nil || missing.Error.Code != "factory_session.session.not_found" {
		t.Fatalf("missing Factory Session read = %#v, want typed not-found response", missing)
	}
}

func assertMCPInitialized(t *testing.T, client *stdioMCPClient) {
	t.Helper()
	initResult := client.call(t, "initialize", map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "mcp-public-catalog-test", "version": "test"},
	})
	if initResult.Error != nil {
		t.Fatalf("initialize error = %#v", initResult.Error)
	}
	protocolVersion, _ := initResult.Result["protocolVersion"].(string)
	if protocolVersion != "2024-11-05" {
		t.Fatalf("protocolVersion = %q, want 2024-11-05", protocolVersion)
	}
}
