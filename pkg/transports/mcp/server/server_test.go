package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	mcpfactorysession "github.com/portpowered/infinite-you/pkg/transports/mcp/factorysession"
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

func scriptedToolOperation(
	result json.RawMessage,
	err error,
) mcpfactorysession.ToolOperation {
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
