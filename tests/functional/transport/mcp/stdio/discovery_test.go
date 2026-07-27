package stdio_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestMCPStdioInitializeAndToolDiscovery proves MCP stdio initialize and
// tools/list succeed through the public you mcp serve boundary without widening
// into Factory Session lifecycle semantics.
func TestMCPStdioInitializeAndToolDiscovery(t *testing.T) {
	client, shutdown, serveErr := startFixtureBackedMCPServer(t)
	defer func() {
		shutdown()
		closeMCPServer(t, serveErr)
	}()

	initResult := client.call("initialize", map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "transport-discovery", "version": "test"},
	})
	if initResult.Error != nil {
		t.Fatalf("initialize error = %#v", initResult.Error)
	}
	protocolVersion, _ := initResult.Result["protocolVersion"].(string)
	if protocolVersion != "2024-11-05" {
		t.Fatalf("protocolVersion = %q, want 2024-11-05", protocolVersion)
	}

	toolsResult := client.call("tools/list", map[string]any{})
	if toolsResult.Error != nil {
		t.Fatalf("tools/list error = %#v", toolsResult.Error)
	}
	rawTools, ok := toolsResult.Result["tools"].([]any)
	if !ok {
		t.Fatalf("tools/list result missing tools array: %#v", toolsResult.Result)
	}
	if len(rawTools) == 0 {
		t.Fatal("tools/list returned an empty tools array")
	}
}

type stdioMCPClient struct {
	t      *testing.T
	stdin  io.WriteCloser
	stdout *bufio.Reader
	nextID int
}

type mcpJSONRPCResponse struct {
	JSONRPC string         `json:"jsonrpc"`
	ID      int            `json:"id"`
	Result  map[string]any `json:"result"`
	Error   *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func newStdioMCPClient(t *testing.T, stdin io.WriteCloser, stdout io.Reader) *stdioMCPClient {
	t.Helper()
	return &stdioMCPClient{t: t, stdin: stdin, stdout: bufio.NewReader(stdout)}
}

func (c *stdioMCPClient) call(method string, params any) mcpJSONRPCResponse {
	c.t.Helper()
	c.nextID++
	id := c.nextID
	payload, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
		"params":  params,
	})
	if err != nil {
		c.t.Fatalf("marshal %s request: %v", method, err)
	}
	if _, err := c.stdin.Write(append(payload, '\n')); err != nil {
		c.t.Fatalf("write %s request: %v", method, err)
	}
	line, err := c.stdout.ReadString('\n')
	if err != nil {
		c.t.Fatalf("read %s response: %v", method, err)
	}
	var response mcpJSONRPCResponse
	if err := json.Unmarshal([]byte(line), &response); err != nil {
		c.t.Fatalf("unmarshal %s response: %v", method, err)
	}
	if response.ID != id {
		c.t.Fatalf("%s response id = %d, want %d", method, response.ID, id)
	}
	return response
}

func startFixtureBackedMCPServer(t *testing.T) (*stdioMCPClient, func(), <-chan error) {
	t.Helper()

	fixtureCatalog := testutil.MustRepoPath(t, "pkg/transports/http/testdata/durable-session-contract-fixtures.json")
	process := support.BuildProcess(t, serviceedges.Edges{})

	stdinRead, stdinWrite, err := os.Pipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	stdoutRead, stdoutWrite, err := os.Pipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	t.Cleanup(func() {
		_ = stdinRead.Close()
		_ = stdinWrite.Close()
		_ = stdoutRead.Close()
		_ = stdoutWrite.Close()
	})

	ctx, cancel := context.WithCancel(context.Background())
	workingDirectory := t.TempDir()

	serveErr := make(chan error, 1)
	var stderr bytes.Buffer
	go func() {
		serveErr <- process.Execute(root.Input{
			Args: []string{
				"you", "mcp", "serve",
				"--fixture-catalog", fixtureCatalog,
			},
			Env:              os.Environ(),
			Stdin:            stdinRead,
			Stdout:           stdoutWrite,
			Stderr:           &stderr,
			Context:          ctx,
			WorkingDirectory: workingDirectory,
		})
	}()
	select {
	case err := <-serveErr:
		t.Fatalf("start fixture-backed MCP server: %v; stderr=%s", err, stderr.String())
	case <-time.After(100 * time.Millisecond):
	}

	var shutdownOnce sync.Once
	shutdown := func() {
		shutdownOnce.Do(func() {
			cancel()
			_ = stdinWrite.Close()
		})
	}
	t.Cleanup(shutdown)

	return newStdioMCPClient(t, stdinWrite, stdoutRead), shutdown, serveErr
}

func closeMCPServer(t *testing.T, serveErr <-chan error) {
	t.Helper()
	select {
	case err := <-serveErr:
		if err != nil && err != io.EOF && !errors.Is(err, context.Canceled) && !strings.Contains(err.Error(), "file already closed") {
			t.Fatalf("MCP serve: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("MCP serve did not shut down after stdin closed")
	}
}
