package mcp_resume_test

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"testing"

	mcpfactorysession "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/mcp"
)

type stdioMCPClient struct {
	stdin   io.WriteCloser
	stdout  *bufio.Reader
	nextID  atomic.Int64
	writeMu sync.Mutex
	pending sync.Map // map[int64]chan mcpCallResult
}

type mcpCallResult struct {
	response mcpJSONRPCResponse
	err      error
}

type mcpJSONRPCResponse struct {
	JSONRPC string         `json:"jsonrpc"`
	ID      int64          `json:"id"`
	Result  map[string]any `json:"result"`
	Error   *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func newStdioMCPClient(t *testing.T, stdin io.WriteCloser, stdout io.Reader) *stdioMCPClient {
	t.Helper()
	client := &stdioMCPClient{stdin: stdin, stdout: bufio.NewReader(stdout)}
	go client.readResponses()
	return client
}

func (c *stdioMCPClient) readResponses() {
	for {
		line, err := c.stdout.ReadString('\n')
		if err != nil {
			c.failPending(fmt.Errorf("read MCP response: %w", err))
			return
		}
		var response mcpJSONRPCResponse
		if err := json.Unmarshal([]byte(line), &response); err != nil {
			c.failPending(fmt.Errorf("unmarshal MCP response: %w", err))
			return
		}
		pending, ok := c.pending.LoadAndDelete(response.ID)
		if ok {
			pending.(chan mcpCallResult) <- mcpCallResult{response: response}
		}
	}
}

func (c *stdioMCPClient) failPending(err error) {
	c.pending.Range(func(key, _ any) bool {
		if pending, ok := c.pending.LoadAndDelete(key); ok {
			pending.(chan mcpCallResult) <- mcpCallResult{err: err}
		}
		return true
	})
}

func (c *stdioMCPClient) call(t *testing.T, method string, params any) mcpJSONRPCResponse {
	t.Helper()
	id := c.nextID.Add(1)
	payload, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
		"params":  params,
	})
	if err != nil {
		t.Fatalf("marshal %s request: %v", method, err)
	}
	pending := make(chan mcpCallResult, 1)
	c.pending.Store(id, pending)
	c.writeMu.Lock()
	_, err = c.stdin.Write(append(payload, '\n'))
	c.writeMu.Unlock()
	if err != nil {
		c.pending.Delete(id)
		t.Fatalf("write %s request: %v", method, err)
	}
	result := <-pending
	if result.err != nil {
		t.Fatalf("read %s response: %v", method, result.err)
	}
	if result.response.ID != id {
		t.Fatalf("%s response id = %d, want %d", method, result.response.ID, id)
	}
	return result.response
}

func (c *stdioMCPClient) callTool(t *testing.T, name string, arguments any) mcpJSONRPCResponse {
	t.Helper()
	encoded, err := json.Marshal(arguments)
	if err != nil {
		t.Fatalf("marshal tool arguments: %v", err)
	}
	return c.call(t, "tools/call", map[string]any{
		"name":      name,
		"arguments": json.RawMessage(encoded),
	})
}

func decodeToolResponse[T any](t *testing.T, response mcpJSONRPCResponse) mcpfactorysession.ToolResponse[T] {
	t.Helper()
	if response.Error != nil {
		t.Fatalf("tools/call protocol error = %#v", response.Error)
	}
	content, ok := response.Result["content"].([]any)
	if !ok || len(content) == 0 {
		t.Fatalf("tools/call result missing content: %#v", response.Result)
	}
	first, ok := content[0].(map[string]any)
	if !ok {
		t.Fatalf("tools/call content[0] = %#v, want object", content[0])
	}
	text, _ := first["text"].(string)
	var toolResponse mcpfactorysession.ToolResponse[T]
	if err := json.Unmarshal([]byte(text), &toolResponse); err != nil {
		t.Fatalf("unmarshal tool response: %v", err)
	}
	return toolResponse
}

func toolNamesFromListResult(t *testing.T, result map[string]any) []string {
	t.Helper()
	rawTools, ok := result["tools"].([]any)
	if !ok {
		t.Fatalf("tools/list result missing tools array: %#v", result)
	}
	names := make([]string, 0, len(rawTools))
	for _, raw := range rawTools {
		tool, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("tool entry = %#v, want object", raw)
		}
		name, _ := tool["name"].(string)
		names = append(names, name)
	}
	return names
}

func assertInstallSmokeInitialize(t *testing.T, client *stdioMCPClient) {
	t.Helper()
	initResult := client.call(t, "initialize", map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "install-smoke", "version": "test"},
	})
	if initResult.Error != nil {
		t.Fatalf("initialize error = %#v", initResult.Error)
	}
	protocolVersion, _ := initResult.Result["protocolVersion"].(string)
	if protocolVersion != "2024-11-05" {
		t.Fatalf("protocolVersion = %q, want 2024-11-05", protocolVersion)
	}
}
