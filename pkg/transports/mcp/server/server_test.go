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

	"github.com/portpowered/infinite-you/internal/testutil"
	factorysessionexecution "github.com/portpowered/infinite-you/pkg/factory/sessions/execution"
	"github.com/portpowered/infinite-you/pkg/factory/sessions/execution/fixtures"
	mcpfactorysession "github.com/portpowered/infinite-you/pkg/transports/mcp/factorysession"
	mcpfactorycatalog "github.com/portpowered/infinite-you/pkg/transports/mcp/factorysession/catalog"
)

func TestNewValidatesOptionsAndAppliesDefaults(t *testing.T) {
	t.Parallel()

	if _, err := New(Options{}); err == nil {
		t.Fatal("New() error = nil, want missing client error")
	}

	srv, err := New(Options{
		Client:        mcpfactorysession.NewClient(),
		ServerName:    "  custom-server  ",
		ServerVersion: "  1.2.3  ",
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if srv.serverName != "custom-server" {
		t.Fatalf("serverName = %q, want %q", srv.serverName, "custom-server")
	}
	if srv.serverVersion != "1.2.3" {
		t.Fatalf("serverVersion = %q, want %q", srv.serverVersion, "1.2.3")
	}

	defaulted, err := New(Options{Client: mcpfactorysession.NewClient()})
	if err != nil {
		t.Fatalf("New() with defaults error = %v", err)
	}
	if defaulted.serverName != "you-agent-factory" {
		t.Fatalf("default serverName = %q, want %q", defaulted.serverName, "you-agent-factory")
	}
	if defaulted.serverVersion != "dev" {
		t.Fatalf("default serverVersion = %q, want %q", defaulted.serverVersion, "dev")
	}
}

func TestHandleLineProtocolResponses(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)

	t.Run("parse error", func(t *testing.T) {
		response := decodeResponse(t, runHandleLine(t, srv, "{"))
		assertProtocolError(t, response, -32700, "parse error")
	})

	t.Run("invalid jsonrpc version", func(t *testing.T) {
		response := decodeResponse(t, runHandleLine(t, srv, `{"jsonrpc":"1.0","id":1,"method":"ping"}`))
		assertProtocolError(t, response, -32600, "invalid request")
		if string(response.ID) != "1" {
			t.Fatalf("response ID = %s, want 1", response.ID)
		}
	})

	t.Run("missing method", func(t *testing.T) {
		response := decodeResponse(t, runHandleLine(t, srv, `{"jsonrpc":"2.0","id":2}`))
		assertProtocolError(t, response, -32600, "invalid request")
	})

	t.Run("unknown method", func(t *testing.T) {
		response := decodeResponse(t, runHandleLine(t, srv, `{"jsonrpc":"2.0","id":3,"method":"nope"}`))
		assertProtocolError(t, response, -32601, "method not found: nope")
	})

	t.Run("notification writes nothing", func(t *testing.T) {
		output := runHandleLine(t, srv, `{"jsonrpc":"2.0","method":"notifications/initialized"}`)
		if output != "" {
			t.Fatalf("notification output = %q, want empty", output)
		}
	})
}

func TestHandleRequestInitializePingAndToolsList(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)

	result, rpcErr, err := srv.handleRequest("initialize", nil)
	if err != nil {
		t.Fatalf("handleRequest(initialize) error = %v", err)
	}
	if rpcErr != nil {
		t.Fatalf("handleRequest(initialize) rpcErr = %#v, want nil", rpcErr)
	}
	initialize, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("initialize result type = %T, want map[string]any", result)
	}
	if initialize["protocolVersion"] != protocolVersion {
		t.Fatalf("protocolVersion = %#v, want %q", initialize["protocolVersion"], protocolVersion)
	}

	result, rpcErr, err = srv.handleRequest("ping", nil)
	if err != nil {
		t.Fatalf("handleRequest(ping) error = %v", err)
	}
	if rpcErr != nil {
		t.Fatalf("handleRequest(ping) rpcErr = %#v, want nil", rpcErr)
	}
	pingResult, ok := result.(map[string]any)
	if !ok || len(pingResult) != 0 {
		t.Fatalf("ping result = %#v, want empty object", result)
	}

	result, rpcErr, err = srv.handleRequest("tools/list", nil)
	if err != nil {
		t.Fatalf("handleRequest(tools/list) error = %v", err)
	}
	if rpcErr != nil {
		t.Fatalf("handleRequest(tools/list) rpcErr = %#v, want nil", rpcErr)
	}
	listResult, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("tools/list result type = %T, want map[string]any", result)
	}
	tools, ok := listResult["tools"].([]map[string]any)
	if !ok {
		t.Fatalf("tools/list tools = %#v, want []map[string]any", listResult["tools"])
	}
	wantCount := len(mcpfactorysession.DiscoverTools()) + len(mcpfactorysession.DiscoverCompatibilityAliases())
	if len(tools) != wantCount {
		t.Fatalf("tools/list count = %d, want %d", len(tools), wantCount)
	}
	assertToolListed(t, tools, mcpfactorysession.ToolListSessions)
	assertToolListed(t, tools, mcpfactorysession.ToolWorkflowRun)
}

func TestToolsListGeneratedCanonicalDiscoveryMatchesLegacySemantics(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	result, rpcErr, err := srv.handleToolsList(nil)
	if err != nil {
		t.Fatalf("handleToolsList() error = %v", err)
	}
	if rpcErr != nil {
		t.Fatalf("handleToolsList() rpcErr = %#v, want nil", rpcErr)
	}
	listResult := result.(map[string]any)
	listed := listResult["tools"].([]map[string]any)
	listedByName := make(map[string]map[string]any, len(listed))
	for _, tool := range listed {
		listedByName[tool["name"].(string)] = tool
	}

	legacy := mcpfactorysession.DiscoverTools()
	for _, want := range legacy {
		got, ok := listedByName[want.Name]
		if !ok {
			t.Errorf("generated tools/list missing canonical tool %q", want.Name)
			continue
		}
		if got["description"] != want.Description {
			t.Errorf("tool %q description = %#v, want %q", want.Name, got["description"], want.Description)
		}
		gotNormalized, gotErr := mcpfactorycatalog.PrepareCatalogInputSchemaForParity(got["inputSchema"].(map[string]any))
		wantNormalized, wantErr := mcpfactorycatalog.PrepareCatalogInputSchemaForParity(want.InputSchema)
		if gotErr != nil || wantErr != nil {
			t.Errorf("tool %q inputSchema normalization errors: generated=%v legacy=%v", want.Name, gotErr, wantErr)
			continue
		}
		gotSchema, gotErr := json.Marshal(gotNormalized)
		wantSchema, wantErr := json.Marshal(wantNormalized)
		if gotErr != nil || wantErr != nil {
			t.Errorf("tool %q inputSchema marshal errors: generated=%v legacy=%v", want.Name, gotErr, wantErr)
			continue
		}
		if !bytes.Equal(gotSchema, wantSchema) {
			t.Errorf("tool %q inputSchema differs from legacy discovery:\ngenerated=%#v\nlegacy=%#v", want.Name, got["inputSchema"], want.InputSchema)
		}
	}
}

func TestHandleToolsCallValidationResultsAndCompatibilityOnlyAlias(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)

	result, rpcErr, err := srv.handleToolsCall(json.RawMessage(`{`))
	if err != nil {
		t.Fatalf("handleToolsCall(invalid json) error = %v", err)
	}
	if result != nil {
		t.Fatalf("handleToolsCall(invalid json) result = %#v, want nil", result)
	}
	assertProtocolError(t, jsonRPCResponse{Error: rpcErr}, -32602, "invalid params")

	result, rpcErr, err = srv.handleToolsCall(json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("handleToolsCall(missing name) error = %v", err)
	}
	if result != nil {
		t.Fatalf("handleToolsCall(missing name) result = %#v, want nil", result)
	}
	assertProtocolError(t, jsonRPCResponse{Error: rpcErr}, -32602, "tool name is required")

	result, rpcErr, err = srv.handleToolsCall(json.RawMessage(`{"name":"you.workflow.run","arguments":{}}`))
	if err != nil {
		t.Fatalf("handleToolsCall(alias) error = %v", err)
	}
	if rpcErr != nil {
		t.Fatalf("handleToolsCall(alias) rpcErr = %#v, want nil", rpcErr)
	}
	success, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("handleToolsCall(alias) result type = %T, want map[string]any", result)
	}
	if success["isError"] != false {
		t.Fatalf("handleToolsCall(alias) isError = %#v, want false", success["isError"])
	}
	assertContentTextContains(t, success, `"error"`)

	result, rpcErr, err = srv.handleToolsCall(json.RawMessage(`{"name":"unsupported.tool","arguments":{}}`))
	if err != nil {
		t.Fatalf("handleToolsCall(unsupported) error = %v", err)
	}
	if rpcErr != nil {
		t.Fatalf("handleToolsCall(unsupported) rpcErr = %#v, want nil", rpcErr)
	}
	failure, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("handleToolsCall(unsupported) result type = %T, want map[string]any", result)
	}
	if failure["isError"] != true {
		t.Fatalf("handleToolsCall(unsupported) isError = %#v, want true", failure["isError"])
	}
	assertContentTextContains(t, failure, `unsupported tool "unsupported.tool"`)
}

func TestHandleToolsCallPreservesTextOnlySuccessAndErrorEnvelopes(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	tests := []struct {
		name        string
		params      string
		wantIsError bool
		wantText    string
	}{
		{
			name:        "serialized success response",
			params:      `{"name":"you.factory_session.list","arguments":{}}`,
			wantIsError: false,
			wantText:    `"result"`,
		},
		{
			name:        "handler argument error",
			params:      `{"name":"you.factory_session.list","arguments":"invalid"}`,
			wantIsError: true,
			wantText:    "decode list sessions input",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, rpcErr, err := srv.handleToolsCall(json.RawMessage(tt.params))
			if err != nil {
				t.Fatalf("handleToolsCall() error = %v", err)
			}
			if rpcErr != nil {
				t.Fatalf("handleToolsCall() rpcErr = %#v, want nil", rpcErr)
			}
			envelope, ok := result.(map[string]any)
			if !ok {
				t.Fatalf("handleToolsCall() result type = %T, want map[string]any", result)
			}
			assertTextOnlyCallToolResult(t, envelope, tt.wantIsError, tt.wantText)
		})
	}
}

func TestServeStdioProcessesRequestsAndSkipsBlankLines(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	input := strings.NewReader("\n" +
		`{"jsonrpc":"2.0","id":1,"method":"ping"}` + "\n" +
		`{"jsonrpc":"2.0","method":"notifications/cancelled"}` + "\n" +
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"unsupported.tool","arguments":{}}}` + "\n")
	var output bytes.Buffer

	if err := srv.ServeStdio(context.Background(), input, &output); err != nil {
		t.Fatalf("ServeStdio() error = %v", err)
	}

	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("ServeStdio() response lines = %d, want 2; output=%q", len(lines), output.String())
	}

	pingResponse := decodeResponse(t, lines[0])
	if pingResponse.Error != nil {
		t.Fatalf("ping response error = %#v, want nil", pingResponse.Error)
	}
	if string(pingResponse.ID) != "1" {
		t.Fatalf("ping response ID = %s, want 1", pingResponse.ID)
	}

	toolResponse := decodeResponse(t, lines[1])
	if toolResponse.Error != nil {
		t.Fatalf("tools/call response error = %#v, want nil", toolResponse.Error)
	}
	result, ok := toolResponse.Result.(map[string]any)
	if !ok {
		t.Fatalf("tools/call result type = %T, want map[string]any", toolResponse.Result)
	}
	if result["isError"] != true {
		t.Fatalf("tools/call isError = %#v, want true", result["isError"])
	}
}

func TestServeStdioStopsOnContextCancellationAndReaderErrors(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := srv.ServeStdio(ctx, strings.NewReader(""), io.Discard); !errors.Is(err, context.Canceled) {
		t.Fatalf("ServeStdio(cancelled) error = %v, want %v", err, context.Canceled)
	}

	readErr := errors.New("boom")
	err := srv.ServeStdio(context.Background(), errReader{err: readErr}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "read mcp request: boom") {
		t.Fatalf("ServeStdio(reader error) = %v, want wrapped read error", err)
	}
}

func TestWriteResponseErrorPaths(t *testing.T) {
	t.Parallel()

	writer := bufio.NewWriter(&bytes.Buffer{})
	if err := writeResponse(writer, jsonRPCResponse{
		JSONRPC: "2.0",
		Result:  map[string]any{"bad": make(chan int)},
	}); err == nil || !strings.Contains(err.Error(), "marshal mcp response") {
		t.Fatalf("writeResponse(marshal error) = %v, want marshal error", err)
	}

	writeFail := &countingWriter{failAt: 1, err: errors.New("write failed")}
	writeBuffered := bufio.NewWriterSize(writeFail, 1)
	err := writeResponse(writeBuffered, jsonRPCResponse{JSONRPC: "2.0", Result: map[string]any{"ok": true}})
	if err == nil || !strings.Contains(err.Error(), "write mcp response: write failed") {
		t.Fatalf("writeResponse(write error) = %v, want write error", err)
	}

	encoded, err := json.Marshal(jsonRPCResponse{JSONRPC: "2.0", Result: map[string]any{"ok": true}})
	if err != nil {
		t.Fatalf("json.Marshal(response) error = %v", err)
	}
	terminateFail := &countingWriter{failAt: 1, err: errors.New("newline failed")}
	terminateBuffered := bufio.NewWriterSize(terminateFail, len(encoded))
	err = writeResponse(terminateBuffered, jsonRPCResponse{JSONRPC: "2.0", Result: map[string]any{"ok": true}})
	if err == nil || !strings.Contains(err.Error(), "terminate mcp response: newline failed") {
		t.Fatalf("writeResponse(terminate error) = %v, want terminate error", err)
	}
}

func newTestServer(t *testing.T) *Server {
	t.Helper()

	catalogPath := testutil.MustRepoPath(t, fixtures.ContractFixtureCatalogRelativePath)
	service, err := factorysessionexecution.NewFakeServiceFromContractFixtures(catalogPath)
	if err != nil {
		t.Fatalf("NewFakeServiceFromContractFixtures() error = %v", err)
	}
	srv, err := New(Options{
		Client:        mcpfactorysession.NewClientWithService(service),
		ServerName:    "test-server",
		ServerVersion: "test-version",
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return srv
}

func runHandleLine(t *testing.T, srv *Server, line string) string {
	t.Helper()

	var output bytes.Buffer
	writer := bufio.NewWriter(&output)
	if err := srv.handleLine(context.Background(), []byte(line), writer); err != nil {
		t.Fatalf("handleLine(%s) error = %v", line, err)
	}
	if err := writer.Flush(); err != nil {
		t.Fatalf("writer.Flush() error = %v", err)
	}
	return output.String()
}

func decodeResponse(t *testing.T, raw string) jsonRPCResponse {
	t.Helper()

	var response jsonRPCResponse
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &response); err != nil {
		t.Fatalf("json.Unmarshal(%q) error = %v", raw, err)
	}
	return response
}

func assertProtocolError(t *testing.T, response jsonRPCResponse, code int, message string) {
	t.Helper()

	if response.Error == nil {
		t.Fatal("response error = nil, want protocol error")
	}
	if response.Error.Code != code || response.Error.Message != message {
		t.Fatalf("response error = %#v, want code=%d message=%q", response.Error, code, message)
	}
}

func assertToolListed(t *testing.T, tools []map[string]any, name string) {
	t.Helper()

	for _, tool := range tools {
		if tool["name"] == name {
			if tool["description"] == "" {
				t.Fatalf("tool %q description empty", name)
			}
			if _, ok := tool["inputSchema"].(map[string]any); !ok {
				t.Fatalf("tool %q inputSchema = %#v, want object", name, tool["inputSchema"])
			}
			return
		}
	}
	t.Fatalf("tool %q not found", name)
}

func assertContentTextContains(t *testing.T, result map[string]any, want string) {
	t.Helper()

	content, ok := result["content"].([]map[string]any)
	if !ok || len(content) == 0 {
		t.Fatalf("result content = %#v, want non-empty []map[string]any", result["content"])
	}
	text, ok := content[0]["text"].(string)
	if !ok {
		t.Fatalf("content[0].text = %#v, want string", content[0]["text"])
	}
	if !strings.Contains(text, want) {
		t.Fatalf("content[0].text = %q, want substring %q", text, want)
	}
}

func assertTextOnlyCallToolResult(t *testing.T, result map[string]any, wantIsError bool, wantText string) {
	t.Helper()

	if len(result) != 2 {
		t.Fatalf("CallToolResult fields = %#v, want only content and isError", result)
	}
	if result["isError"] != wantIsError {
		t.Fatalf("CallToolResult isError = %#v, want %t", result["isError"], wantIsError)
	}
	content, ok := result["content"].([]map[string]any)
	if !ok || len(content) != 1 {
		t.Fatalf("CallToolResult content = %#v, want one text block", result["content"])
	}
	if len(content[0]) != 2 || content[0]["type"] != "text" {
		t.Fatalf("CallToolResult content[0] = %#v, want text-only type/text fields", content[0])
	}
	text, ok := content[0]["text"].(string)
	if !ok || !strings.Contains(text, wantText) {
		t.Fatalf("CallToolResult content[0].text = %#v, want substring %q", content[0]["text"], wantText)
	}
}

type errReader struct {
	err error
}

func (r errReader) Read(_ []byte) (int, error) {
	return 0, r.err
}

type countingWriter struct {
	writes int
	failAt int
	err    error
}

func (w *countingWriter) Write(p []byte) (int, error) {
	w.writes++
	if w.writes == w.failAt {
		return 0, w.err
	}
	return len(p), nil
}
