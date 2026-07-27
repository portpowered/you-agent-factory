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

const jsonRPCInvalidParamsCode = -32602

type mcpJSONRPCResponse struct {
	JSONRPC string `json:"jsonrpc"`
	ID      any    `json:"id"`
	Error   *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
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

type fixtureBackedMCPServer struct {
	t            *testing.T
	stdin        *os.File
	stdout       *bufio.Reader
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
		t:        t,
		stdin:    stdinWrite,
		stdout:   bufio.NewReader(stdoutRead),
		serveErr: serveErr,
		cancel:   cancel,
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
