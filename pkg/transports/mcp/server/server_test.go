package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	mcpfactorysession "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/mcp"
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
			want:    `{"jsonrpc":"2.0","id":3,"error":{"code":-32601,"message":"method not found: \"nope\""}}`,
		},
	} {
		response := runRawRequest(t, server, test.request)
		if response != test.want {
			t.Fatalf("response = %s, want %s", response, test.want)
		}
	}
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

func TestAdditionalToolRegistrationValidationAndCall(t *testing.T) {
	t.Parallel()

	base := Options{ToolOperation: scriptedToolOperation(nil, nil)}
	for _, tool := range []ToolRegistration{
		{Name: "missing-handler", InputSchema: json.RawMessage(`{"type":"object"}`)},
		{Name: "missing-schema", Call: scriptedToolOperation(nil, nil)},
		{Name: "invalid-json", Call: scriptedToolOperation(nil, nil), InputSchema: json.RawMessage(`{`)},
		{Name: "non-object-schema", Call: scriptedToolOperation(nil, nil), InputSchema: json.RawMessage(`{"type":"string"}`)},
	} {
		if _, err := New(Options{ToolOperation: base.ToolOperation, AdditionalTools: []ToolRegistration{tool}}); err == nil {
			t.Errorf("New(additional tool %q) error = nil", tool.Name)
		}
	}

	server, err := New(Options{
		ToolOperation: base.ToolOperation,
		AdditionalTools: []ToolRegistration{{
			Name: "you.test.extra", InputSchema: json.RawMessage(`{"type":"object"}`),
			Call: func(_ context.Context, name string, _ json.RawMessage) (json.RawMessage, error) {
				if name != "you.test.extra" {
					return nil, errors.New("unexpected tool name")
				}
				return json.RawMessage(`{"ok":true}`), nil
			},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	response := runRawRequest(t, server, `{"jsonrpc":"2.0","id":40,"method":"tools/list","params":{}}`)
	if !strings.Contains(response, "you.test.extra") {
		t.Fatalf("tools/list = %s, missing custom tool", response)
	}
	response = runRawRequest(t, server, `{"jsonrpc":"2.0","id":41,"method":"tools/call","params":{"name":"you.test.extra","arguments":{}}}`)
	if !strings.Contains(response, `\"ok\":true`) {
		t.Fatalf("tools/call = %s, missing custom result", response)
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
