package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	mcpfactorysession "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/mcp"
)

type recordedToolCall struct {
	name      string
	arguments json.RawMessage
}

func TestNewValidatesDirectDependencies(t *testing.T) {
	t.Parallel()

	if _, err := New(Options{}); err == nil || !strings.Contains(err.Error(), "tool operation") {
		t.Fatalf("New() error = %v, want missing tool operation error", err)
	}
	if _, err := New(Options{ToolOperation: scriptedToolOperation(nil, nil)}); err != nil {
		t.Fatalf("New(tool operation) error = %v", err)
	}
}

// FND-12 captured MCP success baseline: initialize/list/call succeed over
// stdio JSON-RPC with a catalog tool result. Invoked by
// `make fnd-12-mcp-behavior-baselines`. Does not refresh or re-own PR #1262
// docs/models/mcp CLI-manifest baselines.
//
// pkgmaintcheck:ignore-cyclomatic-complexity service-ownership migration preserves this decision flow; simplify branches and remove this exemption.
func TestServeStdioUsesSDKProtocolAndRegistersCatalog(t *testing.T) {
	t.Parallel()

	calls := make(chan recordedToolCall, 1)
	server, err := New(Options{
		ToolOperation: func(_ context.Context, name string, arguments json.RawMessage) (json.RawMessage, error) {
			var object map[string]any
			if err := json.Unmarshal(arguments, &object); err != nil || object == nil {
				return nil, errors.New("tool arguments must be an object")
			}
			calls <- recordedToolCall{name: name, arguments: append(json.RawMessage(nil), arguments...)}
			return json.RawMessage(`{"result":{"scope":"all","sessions":[]}}`), nil
		},
		ServerName:    "custom-server",
		ServerVersion: "1.2.3",
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	serverDone := make(chan error, 1)
	go func() { serverDone <- server.Serve(ctx, serverTransport) }()
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	defer session.Close()
	if session.InitializeResult().ServerInfo.Name != "custom-server" || session.InitializeResult().ServerInfo.Version != "1.2.3" {
		t.Fatalf("serverInfo = %#v", session.InitializeResult().ServerInfo)
	}

	listResult, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	listed := listResult.Tools
	wantCount := len(mcpfactorysession.DiscoverTools())
	if len(listed) != wantCount {
		t.Fatalf("tools/list count = %d, want %d", len(listed), wantCount)
	}
	assertToolListed(t, listed, mcpfactorysession.ToolListSessions)
	for _, tool := range listed {
		if strings.HasPrefix(tool.Name, "you.workflow.") {
			t.Fatalf("tools/list exposed removed workflow alias %q", tool.Name)
		}
	}

	called, err := session.CallTool(ctx, &mcp.CallToolParams{Name: mcpfactorysession.ToolListSessions, Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	if called.IsError {
		t.Fatalf("CallTool() isError = true: %#v", called)
	}
	content := called.Content[0].(*mcp.TextContent)
	if !strings.Contains(content.Text, `"result"`) {
		t.Fatalf("tools/call text = %q, want serialized result", content.Text)
	}
	call := <-calls
	if call.name != mcpfactorysession.ToolListSessions || string(call.arguments) != `{}` {
		t.Fatalf("tool operation call = (%q, %s), want (%q, {})", call.name, call.arguments, mcpfactorysession.ToolListSessions)
	}
	invalid, err := session.CallTool(ctx, &mcp.CallToolParams{Name: mcpfactorysession.ToolListSessions, Arguments: "invalid"})
	if err != nil {
		t.Fatalf("CallTool(invalid input) protocol error = %v", err)
	}
	if !invalid.IsError {
		t.Fatalf("CallTool(invalid input) isError = false, want tool error")
	}
}

func TestServeStdioValidatesRuntimeInputsAndCancellation(t *testing.T) {
	t.Parallel()

	server, err := New(Options{ToolOperation: scriptedToolOperation(nil, nil)})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if err := server.ServeStdio(context.Background(), nil, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "input") {
		t.Fatalf("ServeStdio(nil input) error = %v", err)
	}
	if err := server.ServeStdio(context.Background(), strings.NewReader(""), nil); err == nil || !strings.Contains(err.Error(), "output") {
		t.Fatalf("ServeStdio(nil output) error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := server.ServeStdio(ctx, strings.NewReader(""), &bytes.Buffer{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("ServeStdio(cancelled) error = %v, want context cancellation", err)
	}
}

// FND-12 captured MCP typed-failure baseline: unknown tool / unsupported
// method return protocol-visible JSON-RPC errors. Invoked by
// `make fnd-12-mcp-behavior-baselines`. Does not refresh or re-own PR #1262
// docs/models/mcp CLI-manifest baselines.
func TestSDKProtocolErrors(t *testing.T) {
	t.Parallel()
	server, err := New(Options{ToolOperation: scriptedToolOperation(nil, nil)})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	for _, test := range []struct {
		request string
		want    string
	}{
		{
			request: `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{}}`,
			want:    `{"jsonrpc":"2.0","id":1,"error":{"code":-32602,"message":"unknown tool \"\""}}`,
		},
		{
			request: `{"jsonrpc":"2.0","id":3,"method":"nope"}`,
			want:    `{"jsonrpc":"2.0","id":3,"error":{"code":0,"message":"JSON RPC not handled: \"nope\" unsupported"}}`,
		},
	} {
		response := runRawRequest(t, server, test.request)
		if response != test.want {
			t.Fatalf("response = %s, want %s", response, test.want)
		}
	}
}

// FND-12 captured MCP typed-error baseline: you.subagent and
// you.factory_session.get typed ToolResponse errors map to IsError=true
// CallToolResult payloads whose first text content carries the readable
// error.message and whose structuredContent retains the full typed envelope.
// Success results keep the plain serialized-JSON text encoding.
// Invoked by `make fnd-12-mcp-behavior-baselines`.
func TestSDKProtocolTypedToolResponseErrors(t *testing.T) {
	t.Parallel()
	server, err := New(Options{
		ToolOperation: func(_ context.Context, name string, _ json.RawMessage) (json.RawMessage, error) {
			switch name {
			case mcpfactorysession.ToolSubagent:
				return json.RawMessage(`{"error":{"code":"BAD_REQUEST","message":"prompt is required","retryable":false}}`), nil
			case mcpfactorysession.ToolGetSession:
				return json.RawMessage(`{"error":{"code":"factory_session.session.not_found","message":"factory session not found","retryable":false,"sessionId":"dur-sess-missing-999"}}`), nil
			case mcpfactorysession.ToolListSessions:
				return json.RawMessage(`{"result":{"scope":"live","sessions":[]}}`), nil
			default:
				return nil, fmt.Errorf("unexpected tool %q", name)
			}
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	for _, test := range []struct {
		name        string
		request     string
		wantText    string
		wantRaw     string
		wantSuccess bool
	}{
		{
			name:     "subagent_typed_error",
			request:  `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"you.subagent","arguments":{"prompt":""}}}`,
			wantText: "prompt is required",
			wantRaw:  `{"error":{"code":"BAD_REQUEST","message":"prompt is required","retryable":false}}`,
		},
		{
			name:     "get_session_typed_error",
			request:  `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"you.factory_session.get","arguments":{"sessionId":"dur-sess-missing-999"}}}`,
			wantText: "factory session not found",
			wantRaw:  `{"error":{"code":"factory_session.session.not_found","message":"factory session not found","retryable":false,"sessionId":"dur-sess-missing-999"}}`,
		},
		{
			name:        "list_sessions_success_unchanged",
			request:     `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"you.factory_session.list","arguments":{}}}`,
			wantText:    `{"result":{"scope":"live","sessions":[]}}`,
			wantSuccess: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			result := decodeSDKProtocolToolsCallResult(t, runRawRequest(t, server, test.request))
			if test.wantSuccess {
				if _, present := result["isError"]; present {
					t.Fatalf("isError = %#v, want omitted for success", result["isError"])
				}
				if _, present := result["structuredContent"]; present {
					t.Fatalf("structuredContent = %#v, want omitted for success", result["structuredContent"])
				}
			} else {
				if result["isError"] != true {
					t.Fatalf("isError = %#v, want true", result["isError"])
				}
				structured, ok := result["structuredContent"].(map[string]any)
				if !ok {
					t.Fatalf("structuredContent = %#v, want object", result["structuredContent"])
				}
				encoded, err := json.Marshal(structured)
				if err != nil {
					t.Fatalf("marshal structuredContent: %v", err)
				}
				if string(encoded) != test.wantRaw {
					t.Fatalf("structuredContent = %s, want %s", encoded, test.wantRaw)
				}
			}
			assertSDKProtocolTextContent(t, result, test.wantText)
		})
	}
}

// FND-12 captured MCP typed-error baseline: a typed ToolResponse error with a
// blank or whitespace error.message maps to the fixed safe nonempty fallback
// text while structuredContent preserves the raw envelope, and payloads that
// are not typed top-level ToolResponse error envelopes (missing error.code or
// carrying a result) keep the plain success encoding.
// Invoked by `make fnd-12-mcp-behavior-baselines`.
func TestSDKProtocolTypedErrorDetectionTightening(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name        string
		tool        string
		arguments   string
		payload     string
		wantText    string
		wantSuccess bool
	}{
		{
			name:      "blank_message_falls_back_to_safe_text",
			tool:      "you.subagent",
			arguments: `{"prompt":""}`,
			payload:   `{"error":{"code":"BAD_REQUEST","message":"","retryable":false}}`,
			wantText:  "tool execution failed",
		},
		{
			name:      "whitespace_message_falls_back_to_safe_text",
			tool:      "you.factory_session.get",
			arguments: `{"sessionId":"dur-sess-missing-999"}`,
			payload:   `{"error":{"code":"factory_session.session.not_found","message":"   ","retryable":false}}`,
			wantText:  "tool execution failed",
		},
		{
			name:        "error_without_code_keeps_success_encoding",
			tool:        "you.subagent",
			arguments:   `{"prompt":""}`,
			payload:     `{"error":{"message":"prompt is required"}}`,
			wantText:    `{"error":{"message":"prompt is required"}}`,
			wantSuccess: true,
		},
		{
			name:        "result_and_error_keeps_success_encoding",
			tool:        "you.factory_session.get",
			arguments:   `{"sessionId":"dur-sess-missing-999"}`,
			payload:     `{"result":{"sessionId":"dur-sess-missing-999"},"error":{"code":"X","message":"m"}}`,
			wantText:    `{"result":{"sessionId":"dur-sess-missing-999"},"error":{"code":"X","message":"m"}}`,
			wantSuccess: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			server, err := New(Options{
				ToolOperation: scriptedToolOperation(json.RawMessage(test.payload), nil),
			})
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}
			request := fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":%q,"arguments":%s}}`, test.tool, test.arguments)
			result := decodeSDKProtocolToolsCallResult(t, runRawRequest(t, server, request))
			if test.wantSuccess {
				if _, present := result["isError"]; present {
					t.Fatalf("isError = %#v, want omitted for success", result["isError"])
				}
				if _, present := result["structuredContent"]; present {
					t.Fatalf("structuredContent = %#v, want omitted for success", result["structuredContent"])
				}
			} else {
				if result["isError"] != true {
					t.Fatalf("isError = %#v, want true", result["isError"])
				}
				structured, ok := result["structuredContent"].(map[string]any)
				if !ok {
					t.Fatalf("structuredContent = %#v, want object", result["structuredContent"])
				}
				encoded, err := json.Marshal(structured)
				if err != nil {
					t.Fatalf("marshal structuredContent: %v", err)
				}
				if string(encoded) != test.payload {
					t.Fatalf("structuredContent = %s, want raw payload %s", encoded, test.payload)
				}
			}
			assertSDKProtocolTextContent(t, result, test.wantText)
		})
	}
}

func assertSDKProtocolTextContent(t *testing.T, result map[string]any, wantText string) {
	t.Helper()
	content, ok := result["content"].([]any)
	if !ok || len(content) != 1 {
		t.Fatalf("content = %#v, want one item", result["content"])
	}
	item, ok := content[0].(map[string]any)
	if !ok {
		t.Fatalf("content[0] type = %T, want object", content[0])
	}
	if item["type"] != "text" {
		t.Fatalf("content type = %#v, want text", item["type"])
	}
	if item["text"] != wantText {
		t.Fatalf("content text = %#v, want %#v", item["text"], wantText)
	}
}

func decodeSDKProtocolToolsCallResult(t *testing.T, line string) map[string]any {
	t.Helper()
	var response struct {
		Result map[string]any `json:"result"`
		Error  any            `json:"error"`
	}
	if err := json.Unmarshal([]byte(line), &response); err != nil {
		t.Fatalf("unmarshal tools/call response: %v", err)
	}
	if response.Error != nil {
		t.Fatalf("tools/call response error = %#v, want success result", response.Error)
	}
	if response.Result == nil {
		t.Fatal("tools/call response result is nil")
	}
	return response.Result
}

func scriptedToolOperation(
	result json.RawMessage,
	err error,
) ToolOperation {
	return func(context.Context, string, json.RawMessage) (json.RawMessage, error) {
		return result, err
	}
}

func runRawRequest(t *testing.T, server *Server, request string) string {
	t.Helper()
	inputReader, inputWriter := io.Pipe()
	outputReader, outputWriter := io.Pipe()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- server.ServeStdio(ctx, inputReader, outputWriter) }()
	scanner := bufio.NewScanner(outputReader)
	_, _ = io.WriteString(inputWriter, `{"jsonrpc":"2.0","id":"init","method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`+"\n")
	if !scanner.Scan() {
		t.Fatalf("read initialize response: %v", scanner.Err())
	}
	_, _ = io.WriteString(inputWriter, `{"jsonrpc":"2.0","method":"notifications/initialized"}`+"\n"+request+"\n")
	if !scanner.Scan() {
		t.Fatalf("read response: %v", scanner.Err())
	}
	response := scanner.Text()
	_ = inputWriter.Close()
	if err := <-done; err != nil {
		t.Fatalf("ServeStdio() error = %v", err)
	}
	return response
}

func assertToolListed(t *testing.T, tools []*mcp.Tool, name string) {
	t.Helper()
	for _, tool := range tools {
		if tool.Name == name {
			return
		}
	}
	t.Fatalf("tools/list missing %q", name)
}
