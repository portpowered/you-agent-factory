package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	mcpfactorysession "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/mcp"
	mcpgenerated "github.com/portpowered/infinite-you/pkg/transports/mcp/generated"
)

func TestMCPToolCallRemainsOpenForLongSubagentWait(t *testing.T) {
	t.Parallel()
	simulatedWork := 1200 * time.Millisecond
	if os.Getenv("YOU_MCP_LONG_DURATION_TEST") == "1" {
		simulatedWork = 70 * time.Second
	}
	server, err := New(Options{ToolOperation: func(ctx context.Context, name string, input json.RawMessage) (json.RawMessage, error) {
		if name != mcpfactorysession.ToolSubagent {
			return nil, errors.New("unexpected tool")
		}
		var request struct {
			TimeoutMillis int64 `json:"timeoutMillis"`
		}
		if err := json.Unmarshal(input, &request); err != nil || request.TimeoutMillis != 3_600_000 {
			return nil, errors.New("one-hour timeout was not forwarded")
		}
		select {
		case <-time.After(simulatedWork):
			return json.RawMessage(`{"result":{"sessionId":"long-session","status":"COMPLETED","text":"done"}}`), nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}})
	if err != nil {
		t.Fatal(err)
	}
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverCtx, stopServer := context.WithCancel(context.Background())
	defer stopServer()
	go func() { _ = server.Serve(serverCtx, serverTransport) }()
	client := mcp.NewClient(&mcp.Implementation{Name: "duration-test", Version: "1"}, nil)
	clientBudget := 5 * time.Second
	if simulatedWork > clientBudget {
		clientBudget = simulatedWork + 15*time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), clientBudget)
	defer cancel()
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	start := time.Now()
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: mcpfactorysession.ToolSubagent, Arguments: map[string]any{
		"prompt": "long task", "timeoutMillis": int64(3_600_000),
	}})
	if err != nil || result.IsError {
		t.Fatalf("long MCP call = %#v, %v", result, err)
	}
	if elapsed := time.Since(start); elapsed < simulatedWork {
		t.Fatalf("MCP call returned after %v, before simulated work ended", elapsed)
	}
}

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
func TestServeStdioUsesSDKProtocolAndRegistersGeneratedCatalog(t *testing.T) {
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
	wantCount := len(mcpgenerated.PrimaryDiscovery())
	if len(listed) != wantCount {
		t.Fatalf("tools/list count = %d, want %d", len(listed), wantCount)
	}
	for _, definition := range mcpgenerated.PrimaryDiscovery() {
		assertToolListed(t, listed, definition.Name)
	}

	called, err := session.CallTool(ctx, &mcp.CallToolParams{Name: mcpfactorysession.ToolSubagent, Arguments: map[string]any{"prompt": "hello"}})
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
	if call.name != mcpfactorysession.ToolSubagent || !strings.Contains(string(call.arguments), `"prompt":"hello"`) {
		t.Fatalf("tool operation call = (%q, %s), want %q with prompt", call.name, call.arguments, mcpfactorysession.ToolSubagent)
	}
	invalid, err := session.CallTool(ctx, &mcp.CallToolParams{Name: mcpfactorysession.ToolSubagent, Arguments: "invalid"})
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
			want:    `{"jsonrpc":"2.0","id":3,"error":{"code":-32601,"message":"method not found: \"nope\""}}`,
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
	_, response := runRawRequestAtVersion(t, server, "2024-11-05", request)
	return response
}

func runRawRequestAtVersion(t *testing.T, server *Server, version, request string) (string, string) {
	t.Helper()
	inputReader, inputWriter := io.Pipe()
	outputReader, outputWriter := io.Pipe()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- server.ServeStdio(ctx, inputReader, outputWriter) }()
	scanner := bufio.NewScanner(outputReader)
	initializeRequest := `{"jsonrpc":"2.0","id":"init","method":"initialize","params":{"protocolVersion":"` + version + `","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`
	_, _ = io.WriteString(inputWriter, initializeRequest+"\n")
	if !scanner.Scan() {
		t.Fatalf("read initialize response: %v", scanner.Err())
	}
	initializeResponse := scanner.Text()
	_, _ = io.WriteString(inputWriter, `{"jsonrpc":"2.0","method":"notifications/initialized"}`+"\n"+request+"\n")
	if !scanner.Scan() {
		t.Fatalf("read response: %v", scanner.Err())
	}
	response := scanner.Text()
	_ = inputWriter.Close()
	if err := <-done; err != nil {
		t.Fatalf("ServeStdio() error = %v", err)
	}
	return initializeResponse, response
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

func TestSkillsMethodsAndResources(t *testing.T) {
	t.Parallel()
	entry := SkillEntry{
		URI:         "skill://operator-settings/SKILL.md",
		Frontmatter: map[string]any{"name": "operator-settings", "description": "Configure subagents"},
		Resources:   []SkillResource{{URI: "skill://operator-settings/SKILL.md", Digest: "sha256:abc", Size: 42}},
	}
	server, err := New(Options{
		ToolOperation: scriptedToolOperation(nil, nil),
		Skills:        []SkillEntry{entry},
		Resources: []ResourceRegistration{{
			Resource: &mcp.Resource{URI: "you://operator/config", Name: "Operator config", MIMEType: "application/json"},
			Read: func(_ context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
				return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: req.Params.URI, MIMEType: "application/json", Text: `{}`}}}, nil
			},
		}},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	for _, test := range []struct{ request, contains string }{
		{`{"jsonrpc":"2.0","id":10,"method":"skills/list","params":{}}`, `"cacheScope":"public"`},
		{`{"jsonrpc":"2.0","id":11,"method":"skills/get","params":{"uri":"skill://operator-settings/SKILL.md"}}`, `"name":"operator-settings"`},
		{`{"jsonrpc":"2.0","id":12,"method":"resources/list","params":{}}`, `you://operator/config`},
		{`{"jsonrpc":"2.0","id":14,"method":"resources/read","params":{"uri":"you://operator/config"}}`, `"text":"{}"`},
	} {
		response := runRawRequest(t, server, test.request)
		if !strings.Contains(response, test.contains) {
			t.Fatalf("response = %s, want substring %q", response, test.contains)
		}
	}
	list := runRawRequest(t, server, `{"jsonrpc":"2.0","id":15,"method":"skills/list","params":{}}`)
	if !strings.Contains(list, `"ttlMs":300000`) || !strings.Contains(list, `"resultType":"complete"`) {
		t.Fatalf("skills/list result = %s, want complete result with ttlMs 300000", list)
	}
	missing := runRawRequest(t, server, `{"jsonrpc":"2.0","id":13,"method":"skills/get","params":{"uri":"skill://missing/SKILL.md"}}`)
	if !strings.Contains(missing, `"code":-32602`) {
		t.Fatalf("unknown skills/get response = %s, want invalid params", missing)
	}
}

func TestSkillsCapabilityAdvertisedFor2026Protocol(t *testing.T) {
	t.Parallel()
	server, err := New(Options{
		ToolOperation: scriptedToolOperation(nil, nil),
		Skills: []SkillEntry{{
			URI: "skill://test/SKILL.md", Frontmatter: map[string]any{"name": "test", "description": "Test skill"}, Resources: "dynamic",
		}},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	initialize, _ := runRawRequestAtVersion(t, server, "2026-07-28", `{"jsonrpc":"2.0","id":21,"method":"ping"}`)
	if !strings.Contains(initialize, skillsExtension) {
		t.Fatalf("initialize response = %s, want skills extension declaration", initialize)
	}
	discover, _ := runRawRequestAtVersion(t, server, "2026-07-28", `{"jsonrpc":"2.0","id":22,"method":"server/discover","params":{}}`)
	if !strings.Contains(discover, skillsExtension) {
		t.Fatalf("server/discover response = %s, want skills extension declaration", discover)
	}
}

func TestSkillsValidationAndPagination(t *testing.T) {
	t.Parallel()

	valid := SkillEntry{
		URI:         "skill://valid/SKILL.md",
		Frontmatter: map[string]any{"name": "valid", "description": "Valid skill"},
		Resources:   []SkillResource{{URI: "skill://valid/SKILL.md", Digest: "sha256:abc", Size: 3}},
	}
	tests := []struct {
		name    string
		entries []SkillEntry
	}{
		{name: "missing fields", entries: []SkillEntry{{}}},
		{name: "missing name", entries: []SkillEntry{{URI: valid.URI, Frontmatter: map[string]any{"description": "x"}, Resources: "dynamic"}}},
		{name: "missing description", entries: []SkillEntry{{URI: valid.URI, Frontmatter: map[string]any{"name": "x"}, Resources: "dynamic"}}},
		{name: "unsupported dynamic marker", entries: []SkillEntry{{URI: valid.URI, Frontmatter: valid.Frontmatter, Resources: "other"}}},
		{name: "empty manifest", entries: []SkillEntry{{URI: valid.URI, Frontmatter: valid.Frontmatter, Resources: []SkillResource{}}}},
		{name: "duplicate URI", entries: []SkillEntry{valid, valid}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := New(Options{ToolOperation: scriptedToolOperation(nil, nil), Skills: test.entries}); err == nil {
				t.Fatal("New() error = nil, want invalid skill registration error")
			}
		})
	}

	entries := make([]SkillEntry, skillPageSize+1)
	for i := range entries {
		entries[i] = SkillEntry{
			URI:         "skill://valid/SKILL.md",
			Frontmatter: valid.Frontmatter,
			Resources:   "dynamic",
		}
	}
	first, err := makeListSkillsResult(entries, nil)
	if err != nil || len(first.Skills) != skillPageSize || first.NextCursor != strconv.Itoa(skillPageSize) {
		t.Fatalf("first page = (%d skills, cursor %q), %v", len(first.Skills), first.NextCursor, err)
	}
	last, err := makeListSkillsResult(entries, &listSkillsParams{Cursor: first.NextCursor})
	if err != nil || len(last.Skills) != 1 || last.NextCursor != "" {
		t.Fatalf("last page = (%d skills, cursor %q), %v", len(last.Skills), last.NextCursor, err)
	}
	for _, cursor := range []string{"not-a-number", "-1", "102"} {
		if _, err := makeListSkillsResult(entries, &listSkillsParams{Cursor: cursor}); err == nil {
			t.Errorf("makeListSkillsResult(cursor %q) error = nil", cursor)
		}
	}
}

func TestServeValidatesServerAndTransport(t *testing.T) {
	t.Parallel()
	if err := (*Server)(nil).Serve(context.Background(), nil); err == nil || !strings.Contains(err.Error(), "server is required") {
		t.Fatalf("Serve(nil server) error = %v", err)
	}
	server, err := New(Options{ToolOperation: scriptedToolOperation(nil, nil)})
	if err != nil {
		t.Fatal(err)
	}
	if err := server.Serve(context.Background(), nil); err == nil || !strings.Contains(err.Error(), "transport is required") {
		t.Fatalf("Serve(nil transport) error = %v", err)
	}
}
