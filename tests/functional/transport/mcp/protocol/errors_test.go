package protocol_test

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
)

const (
	jsonRPCInvalidParamsCode   = -32602
	factorySessionNotFoundCode = "factory_session.session.not_found"
	missingFactorySessionID    = "dur-sess-missing-999"
	factorySessionGetToolName  = "you.factory_session.get"
)

type mcpJSONRPCResponse struct {
	JSONRPC string              `json:"jsonrpc"`
	ID      any                 `json:"id"`
	Result  *mcpToolsCallResult `json:"result"`
	Error   *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

type mcpToolsCallResult struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	IsError bool `json:"isError"`
}

type mcpToolErrorEnvelope struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
	SessionID string `json:"sessionId"`
}

// TestMCPMalformedParametersReturnInvalidParams proves malformed MCP parameters
// return a JSON-RPC invalid-params error at the public stdio/protocol boundary.
func TestMCPMalformedParametersReturnInvalidParams(t *testing.T) {
	server := startFixtureBackedMCPServer(t)
	t.Cleanup(server.shutdown)

	assertInitializeHandshake(t, server)
	response := server.exchange(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{}}`)
	if response.Error == nil {
		t.Fatalf("tools/call with empty params returned success %#v, want invalid-params error", response)
	}
	if response.Error.Code != jsonRPCInvalidParamsCode {
		t.Fatalf("tools/call error code = %d, want %d (invalid-params)", response.Error.Code, jsonRPCInvalidParamsCode)
	}
}

// TestMCPMissingFactorySessionReturnsCanonicalNotFound proves a well-formed Factory
// Session tools/call for a missing session id returns the canonical not-found
// result at the public MCP stdio/protocol boundary.
func TestMCPMissingFactorySessionReturnsCanonicalNotFound(t *testing.T) {
	server := startFixtureBackedMCPServer(t)
	t.Cleanup(server.shutdown)

	assertInitializeHandshake(t, server)
	request := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"` + factorySessionGetToolName +
		`","arguments":{"sessionId":"` + missingFactorySessionID + `"}}}`
	response := server.exchange(request)
	if response.Error != nil {
		t.Fatalf("tools/call for missing session returned JSON-RPC error %#v, want typed domain result", response.Error)
	}
	if response.Result == nil {
		t.Fatal("tools/call for missing session returned nil result")
	}
	if response.Result.IsError {
		t.Fatalf("tools/call isError = true, want typed domain error in success envelope %#v", response.Result)
	}
	if len(response.Result.Content) != 1 || response.Result.Content[0].Type != "text" {
		t.Fatalf("tools/call content = %#v, want one text item", response.Result.Content)
	}

	var payload struct {
		Error *mcpToolErrorEnvelope `json:"error"`
	}
	if err := json.Unmarshal([]byte(response.Result.Content[0].Text), &payload); err != nil {
		t.Fatalf("unmarshal tools/call domain error payload: %v", err)
	}
	if payload.Error == nil {
		t.Fatalf("tools/call payload = %q, want typed error envelope", response.Result.Content[0].Text)
	}
	if payload.Error.Code != factorySessionNotFoundCode {
		t.Fatalf("error code = %q, want %q", payload.Error.Code, factorySessionNotFoundCode)
	}
	if payload.Error.SessionID != missingFactorySessionID {
		t.Fatalf("error sessionId = %q, want %q", payload.Error.SessionID, missingFactorySessionID)
	}
	if payload.Error.Retryable {
		t.Fatalf("error retryable = true, want false for missing session")
	}
}

// TestMCPServerShutdownClosesStdioCleanly proves MCP server shutdown terminates
// stdio serve cleanly without hung streams or unclean protocol failures.
func TestMCPServerShutdownClosesStdioCleanly(t *testing.T) {
	server := startFixtureBackedMCPServer(t)

	assertInitializeHandshake(t, server)
	assertFixtureBackedMCPServerShutdownClean(t, server)
}

type fixtureBackedMCPServer struct {
	t            *testing.T
	stdin        *os.File
	stdout       *bufio.Reader
	stdoutWrite  *os.File
	serveErr     <-chan error
	cancel       context.CancelFunc
	shutdownOnce sync.Once
}

func startFixtureBackedMCPServer(t *testing.T) *fixtureBackedMCPServer {
	t.Helper()

	process, err := root.BuildProcess(t.Context(), serviceedges.Edges{})
	if err != nil {
		t.Fatalf("BuildProcess: %v", err)
	}

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
	fixturePath := testutil.MustRepoPath(t, "pkg/transports/http/testdata/durable-session-contract-fixtures.json")
	workingDirectory := t.TempDir()

	serveErr := make(chan error, 1)
	var stderr bytes.Buffer
	go func() {
		serveErr <- process.Execute(root.Input{
			Args:             []string{"you", "mcp", "serve", "--fixture-catalog", fixturePath},
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
		t.Fatalf("start fixture-backed MCP serve: %v; stderr=%s", err, stderr.String())
	case <-time.After(100 * time.Millisecond):
	}

	return &fixtureBackedMCPServer{
		t:           t,
		stdin:       stdinWrite,
		stdout:      bufio.NewReader(stdoutRead),
		stdoutWrite: stdoutWrite,
		serveErr:    serveErr,
		cancel:      cancel,
	}
}

func assertInitializeHandshake(t *testing.T, server *fixtureBackedMCPServer) {
	t.Helper()

	initResponse := server.exchange(`{"jsonrpc":"2.0","id":"init","method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"protocol-errors-test","version":"test"}}}`)
	if initResponse.Error != nil {
		t.Fatalf("initialize error = %#v", initResponse.Error)
	}
	if _, err := server.stdin.Write([]byte(`{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n")); err != nil {
		t.Fatalf("write initialized notification: %v", err)
	}
}

func (s *fixtureBackedMCPServer) exchange(request string) mcpJSONRPCResponse {
	s.t.Helper()

	if _, err := s.stdin.Write([]byte(request + "\n")); err != nil {
		s.t.Fatalf("write request %q: %v", request, err)
	}
	line, err := s.stdout.ReadString('\n')
	if err != nil {
		s.t.Fatalf("read response for %q: %v", request, err)
	}
	var response mcpJSONRPCResponse
	if err := json.Unmarshal([]byte(line), &response); err != nil {
		s.t.Fatalf("unmarshal response for %q: %v", request, err)
	}
	return response
}

func (s *fixtureBackedMCPServer) shutdown() {
	s.shutdownOnce.Do(func() {
		s.cancel()
		_ = s.stdin.Close()
	})
	select {
	case err := <-s.serveErr:
		if err != nil && err != io.EOF && !errors.Is(err, context.Canceled) && !strings.Contains(err.Error(), "file already closed") {
			s.t.Fatalf("fixture-backed MCP serve: %v", err)
		}
	case <-time.After(5 * time.Second):
		s.t.Fatal("fixture-backed MCP serve did not shut down after stdin closed")
	}
}

func assertFixtureBackedMCPServerShutdownClean(t *testing.T, server *fixtureBackedMCPServer) {
	t.Helper()

	server.shutdownOnce.Do(func() {
		server.cancel()
		_ = server.stdin.Close()
	})

	select {
	case serveErr := <-server.serveErr:
		if serveErr != nil && serveErr != io.EOF && !errors.Is(serveErr, context.Canceled) && !strings.Contains(serveErr.Error(), "file already closed") {
			t.Fatalf("fixture-backed MCP serve shutdown: %v", serveErr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("fixture-backed MCP serve did not shut down after cancel and stdin close")
	}

	_ = server.stdoutWrite.Close()
	if _, err := server.stdout.ReadByte(); err != io.EOF {
		t.Fatalf("read stdout after shutdown = %v, want EOF (no hung stream)", err)
	}
}
