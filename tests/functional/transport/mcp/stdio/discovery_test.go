package stdio_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	mcpgenerated "github.com/portpowered/infinite-you/pkg/transports/mcp/generated"
	mcpstdio "github.com/portpowered/infinite-you/pkg/transports/mcp/stdio"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const mcpStdioStopTimeout = 5 * time.Second

// TestMCPStdioInitializeAndToolDiscovery proves MCP stdio initialize and
// tools/list succeed through the public you server mcp boundary without widening
// into Factory Session lifecycle semantics.
func TestMCPStdioInitializeAndToolDiscovery(t *testing.T) {
	// Keep this fixture isolated: initialize and discovery are separate
	// scenario-owned protocol observations and must not share session state.
	server := startFixtureBackedMCPServer(t)
	defer server.cleanup()

	initResult := server.client.call("initialize", map[string]any{
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

	toolsResult := server.client.call("tools/list", map[string]any{})
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

// TestMCPUnknownToolReturnsProtocolError proves tools/call for a tool name that
// is not in the discovered catalog returns a protocol-visible JSON-RPC error
// rather than a success result or typed Factory Session domain envelope.
func TestMCPUnknownToolReturnsProtocolError(t *testing.T) {
	// Keep this fixture isolated: unknown-tool handling is a protocol error
	// witness and must retain its own root and stdio session.
	server := startFixtureBackedMCPServer(t)
	defer server.cleanup()

	initializeMCPClient(t, server.client)

	const unknownTool = "you.factory_session.definitely_not_a_real_tool"
	callResult := server.client.call("tools/call", map[string]any{
		"name":      unknownTool,
		"arguments": map[string]any{},
	})
	if callResult.Error == nil {
		t.Fatalf("tools/call for unknown tool %q succeeded with result %#v, want JSON-RPC error", unknownTool, callResult.Result)
	}
	if callResult.Result != nil {
		t.Fatalf("tools/call for unknown tool %q returned result %#v alongside error %#v", unknownTool, callResult.Result, callResult.Error)
	}
	if callResult.Error.Code != -32602 {
		t.Fatalf("tools/call error code = %d, want -32602 (invalid params); error = %#v", callResult.Error.Code, callResult.Error)
	}
	if !strings.Contains(callResult.Error.Message, "unknown tool") {
		t.Fatalf("tools/call error message = %q, want protocol-visible unknown-tool message; error = %#v", callResult.Error.Message, callResult.Error)
	}
}

// TestMCPDiscoveryContainsCanonicalFactorySessionTools proves tools/list exposes
// the canonical Factory Session tool names published for MCP hosts without
// asserting Session lifecycle or tool execution semantics.
func TestMCPDiscoveryContainsCanonicalFactorySessionTools(t *testing.T) {
	// Keep this fixture isolated: generated discovery membership is an
	// independent catalog witness, not reusable live session state.
	server := startFixtureBackedMCPServer(t)
	defer server.cleanup()

	initializeMCPClient(t, server.client)

	toolsResult := server.client.call("tools/list", map[string]any{})
	if toolsResult.Error != nil {
		t.Fatalf("tools/list error = %#v", toolsResult.Error)
	}
	toolNames := toolNamesFromListResult(t, toolsResult.Result)

	for _, tool := range mcpgenerated.PrimaryDiscovery() {
		if !slices.Contains(toolNames, tool.Name) {
			t.Fatalf("tools/list missing canonical Factory Session tool %q; got %#v", tool.Name, toolNames)
		}
	}
}

// TestMCPStdioRuntimeRejectsMissingHomeEnvironment proves runtime-backed you
// server mcp fails with a customer-visible home diagnostic before stdio initialize when
// HOME and USERPROFILE are absent from the process environment.
func TestMCPStdioRuntimeRejectsMissingHomeEnvironment(t *testing.T) {
	// Keep this root isolated: the environment witness must return before MCP
	// initialization and cannot share process inputs with a valid invocation.
	process := buildMCPProcess(t)
	projectRoot := trackedMCPTempDir(t)
	workingDirectory := trackedMCPTempDir(t)
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "server", "mcp", "--runtime", "--project-root", projectRoot,
	})
	inputs.Env = []string{"PATH="}
	inputs.WorkingDirectory = workingDirectory
	err := executeMCPProcess(t, process, inputs.Input)
	if err == nil || !strings.Contains(err.Error(), "home directory is not defined in the supplied environment") {
		t.Fatalf("Process.Execute(you server mcp --runtime) error = %v, want missing-home diagnostic", err)
	}
}

// TestMCPStdioRuntimeRejectsInvalidRuntimeProjectRoot proves runtime-backed you
// server mcp rejects a project root that cannot resolve a factory layout before
// stdio initialize succeeds.
func TestMCPStdioRuntimeRejectsInvalidRuntimeProjectRoot(t *testing.T) {
	// Keep this root isolated: invalid Factory layout exercises initializer
	// failure with its own project and home environment.
	process := buildMCPProcess(t)
	projectRoot := trackedMCPTempDir(t)
	homeDir := trackedMCPTempDir(t)
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "server", "mcp", "--runtime", "--project-root", projectRoot,
	})
	inputs.Env = append([]string{"PATH=", "HOME=" + homeDir, "USERPROFILE=" + homeDir}, os.Environ()...)
	inputs.WorkingDirectory = projectRoot
	err := executeMCPProcess(t, process, inputs.Input)
	if err == nil || !strings.Contains(err.Error(), "factory layout not found") {
		t.Fatalf("Process.Execute(you server mcp --runtime) error = %v, want factory layout diagnostic", err)
	}
}

// TestMCPStdioFixtureAndRuntimePathsReachInitializer proves fixture-backed and
// runtime-backed you server mcp both reach a successful stdio initialize through
// the public process boundary with injected transport dependencies.
func TestMCPStdioFixtureAndRuntimePathsReachInitializer(t *testing.T) {
	// The parent row owns two named initializer witnesses. They remain
	// isolated because fixture and runtime initialization have different roots,
	// environments, and lifecycle inputs.
	t.Run("fixture-backed", func(t *testing.T) {
		// Keep the fixture-backed initializer row isolated from the runtime
		// row: each owns a distinct root, stream, and initialization path.
		server := startFixtureBackedMCPServer(t)
		defer server.cleanup()
		initializeMCPClient(t, server.client)
		assertIncompleteMCPFrameTerminates(t, server)
	})
	t.Run("runtime-backed", func(t *testing.T) {
		projectRoot := support.ScaffoldSingleStepFactory(t, "mcp-stdio-discovery-runtime")

		// Keep the runtime-backed initializer row isolated: its project root
		// and environment are distinct from the fixture-backed row.
		server := startRuntimeBackedMCPServer(t, projectRoot)
		defer server.cleanup()
		initializeMCPClient(t, server.client)
	})
}

// TestMCPStdioOpenRejectsUncomposedServerAndStreams proves the stdio transport
// refuses to open an invocation when Wire has not supplied a composed protocol
// server or the invocation streams are absent. A misconfigured composition must
// fail at open time with a diagnostic rather than hand back an inert session
// that would silently accept and drop client traffic.
func TestMCPStdioOpenRejectsUncomposedServerAndStreams(t *testing.T) {
	// This is the one eligible process-free row: Open validates the composed
	// server and invocation streams before any root or session is acquired.
	t.Parallel()

	if _, err := mcpstdio.Open(nil, strings.NewReader(""), &bytes.Buffer{}); err == nil ||
		!strings.Contains(err.Error(), "server") {
		t.Fatalf("Open(nil server) error = %v, want composed-server diagnostic", err)
	}
	if _, err := mcpstdio.Open(nil, nil, &bytes.Buffer{}); err == nil ||
		!strings.Contains(err.Error(), "streams") {
		t.Fatalf("Open(nil input) error = %v, want invocation-stream diagnostic", err)
	}
	if _, err := mcpstdio.Open(nil, strings.NewReader(""), nil); err == nil ||
		!strings.Contains(err.Error(), "streams") {
		t.Fatalf("Open(nil output) error = %v, want invocation-stream diagnostic", err)
	}
}

type stdioMCPClient struct {
	t      *testing.T
	stdin  io.WriteCloser
	stdout *bufio.Reader
	nextID int
}

type stdioMCPServer struct {
	t                   *testing.T
	client              *stdioMCPClient
	stdin               *os.File
	stdinRead           *os.File
	stdout              *bufio.Reader
	stdoutRead          *os.File
	stdoutWrite         *os.File
	serveErr            <-chan error
	cancel              context.CancelFunc
	serveOnce           sync.Once
	serveDone           chan struct{}
	serveResult         error
	serveResultAccepted bool
	shutdownOnce        sync.Once
	shutdownDone        chan struct{}
	shutdownErr         error
	streamsOnce         sync.Once
	cleanupOnce         sync.Once
	cleanupErr          error
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

func initializeMCPClient(t *testing.T, client *stdioMCPClient) {
	t.Helper()
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

func buildMCPProcess(t testing.TB) support.ApplicationProcess {
	t.Helper()
	process, err := support.BuildProcessWithContext(context.Background(), serviceedges.Edges{})
	if err != nil {
		t.Fatalf("BuildProcess: %v", err)
	}
	t.Cleanup(func() {
		closeContext, cancel := context.WithTimeout(context.Background(), mcpStdioStopTimeout)
		closeErr := process.Close(closeContext)
		cancel()
		if closeErr != nil {
			t.Errorf("close MCP application process: %v", closeErr)
		}
	})
	return process
}

func executeMCPProcess(t testing.TB, process support.ApplicationProcess, input root.Input) error {
	t.Helper()
	if process == nil {
		t.Fatal("execute MCP process requires an application process")
	}
	return process.Execute(input)
}

func trackedMCPTempDir(t testing.TB) string {
	t.Helper()
	return t.TempDir()
}

func startFixtureBackedMCPServer(t *testing.T) *stdioMCPServer {
	t.Helper()

	fixtureCatalog := testutil.MustRepoPath(t, "pkg/transports/http/testdata/durable-session-contract-fixtures.json")
	process := buildMCPProcess(t)

	stdinRead, stdinWrite, err := os.Pipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	stdoutRead, stdoutWrite, err := os.Pipe()
	if err != nil {
		_ = stdinRead.Close()
		_ = stdinWrite.Close()
		t.Fatalf("stdout pipe: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	workingDirectory := trackedMCPTempDir(t)

	serveErr := make(chan error, 1)
	var stderr bytes.Buffer
	go func() {
		serveErr <- process.Execute(root.Input{
			Args: []string{
				"you", "server", "mcp",
				"--fixture-catalog", fixtureCatalog,
			},
			Env:              builtcliacceptance.ProcessEnvForIsolatedHome(t.TempDir()),
			Stdin:            stdinRead,
			Stdout:           stdoutWrite,
			Stderr:           &stderr,
			Context:          ctx,
			WorkingDirectory: workingDirectory,
		})
	}()

	server := &stdioMCPServer{
		t:            t,
		client:       newStdioMCPClient(t, stdinWrite, stdoutRead),
		stdin:        stdinWrite,
		stdinRead:    stdinRead,
		stdout:       bufio.NewReader(stdoutRead),
		stdoutRead:   stdoutRead,
		stdoutWrite:  stdoutWrite,
		serveErr:     serveErr,
		cancel:       cancel,
		serveDone:    make(chan struct{}),
		shutdownDone: make(chan struct{}),
	}
	t.Cleanup(server.cleanup)
	return server
}

func startRuntimeBackedMCPServer(t *testing.T, projectRoot string) *stdioMCPServer {
	t.Helper()

	process := buildMCPProcess(t)

	stdinRead, stdinWrite, err := os.Pipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	stdoutRead, stdoutWrite, err := os.Pipe()
	if err != nil {
		_ = stdinRead.Close()
		_ = stdinWrite.Close()
		t.Fatalf("stdout pipe: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	homeDir := trackedMCPTempDir(t)
	env := append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)

	serveErr := make(chan error, 1)
	var stderr bytes.Buffer
	go func() {
		serveErr <- process.Execute(root.Input{
			Args: []string{
				"you", "server", "mcp",
				"--runtime", "--project-root", projectRoot,
			},
			Env:              env,
			Stdin:            stdinRead,
			Stdout:           stdoutWrite,
			Stderr:           &stderr,
			Context:          ctx,
			WorkingDirectory: projectRoot,
		})
	}()

	server := &stdioMCPServer{
		t:            t,
		client:       newStdioMCPClient(t, stdinWrite, stdoutRead),
		stdin:        stdinWrite,
		stdinRead:    stdinRead,
		stdout:       bufio.NewReader(stdoutRead),
		stdoutRead:   stdoutRead,
		stdoutWrite:  stdoutWrite,
		serveErr:     serveErr,
		cancel:       cancel,
		serveDone:    make(chan struct{}),
		shutdownDone: make(chan struct{}),
	}
	t.Cleanup(server.cleanup)
	return server
}

func (s *stdioMCPServer) cleanup() {
	s.cleanupOnce.Do(func() {
		var cleanupErrors []error
		if err := s.shutdown(); err != nil {
			cleanupErrors = append(cleanupErrors, err)
		}
		s.closeStreams()
		s.cleanupErr = errors.Join(cleanupErrors...)
	})
	if s.cleanupErr != nil {
		s.t.Errorf("MCP stdio invocation cleanup: %v", s.cleanupErr)
	}
}

func (s *stdioMCPServer) awaitServe() error {
	s.serveOnce.Do(func() {
		// This bounded wait is only a hang guard. A returned serveErr is the
		// deterministic completion signal; the timeout protects test cleanup
		// from a genuine stuck stream without acting as synchronization.
		select {
		case err := <-s.serveErr:
			s.serveResult = err
		case <-time.After(mcpStdioStopTimeout):
			s.serveResult = fmt.Errorf("MCP stdio server did not shut down after stdin closed")
		}
		close(s.serveDone)
	})
	<-s.serveDone
	return s.serveResult
}

func (s *stdioMCPServer) closeInputAndAwait() error {
	_ = s.stdin.Close()
	return s.awaitServe()
}

func (s *stdioMCPServer) acceptServeResult() {
	s.serveResultAccepted = true
}

func (s *stdioMCPServer) shutdown() error {
	s.shutdownOnce.Do(func() {
		s.cancel()
		_ = s.stdin.Close()
		if err := s.awaitServe(); err != nil &&
			!s.serveResultAccepted &&
			!errors.Is(err, io.EOF) &&
			!errors.Is(err, context.Canceled) &&
			!strings.Contains(err.Error(), "file already closed") {
			s.shutdownErr = fmt.Errorf("MCP stdio server: %w", err)
		}
		close(s.shutdownDone)
	})
	<-s.shutdownDone
	return s.shutdownErr
}

func (s *stdioMCPServer) closeStreams() {
	s.streamsOnce.Do(func() {
		_ = s.stdinRead.Close()
		_ = s.stdin.Close()
		_ = s.stdoutRead.Close()
		_ = s.stdoutWrite.Close()
	})
}

func assertIncompleteMCPFrameTerminates(t *testing.T, server *stdioMCPServer) {
	t.Helper()
	if _, err := server.stdin.Write([]byte(`{"jsonrpc":"2.0","id":2,"method":"tools/list"`)); err != nil {
		t.Fatalf("write incomplete MCP frame: %v", err)
	}
	serveErr := server.closeInputAndAwait()
	if serveErr == nil {
		t.Fatal("incomplete MCP frame returned nil, want invocation error")
	}
	server.acceptServeResult()

	if err := server.stdoutWrite.Close(); err != nil {
		t.Fatalf("close stdout after incomplete MCP frame: %v", err)
	}
	if _, err := server.stdout.ReadByte(); err != io.EOF {
		t.Fatalf("read stdout after incomplete MCP frame = %v, want EOF without a success response", err)
	}
}
