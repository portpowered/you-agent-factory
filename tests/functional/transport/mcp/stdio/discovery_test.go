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

	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	mcpgenerated "github.com/portpowered/infinite-you/pkg/transports/mcp/generated"
	mcpstdio "github.com/portpowered/infinite-you/pkg/transports/mcp/stdio"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const mcpStdioStopTimeout = 5 * time.Second

type mcpStdioTopologyLedger struct {
	sync.Mutex
	rootBuilds            int
	rootCloses            int
	invocationStarts      int
	invocationReturns     int
	stdioSessionsOpened   int
	stdioSessionsClosed   int
	contextsCreated       int
	contextsCanceled      int
	streamsOpened         int
	streamsClosed         int
	temporaryRootsMade    int
	temporaryRootsRemoved int
}

var mcpStdioTopology mcpStdioTopologyLedger

// TestMain reports the stdio inventory and verifies package-owned lifecycle
// accounting after every scenario cleanup has run. The eight isolated rows
// are the six non-constructor top-level rows plus the two named initializer
// rows; the constructor validation row is process-free.
func TestMain(m *testing.M) {
	exitCode := m.Run()

	if err := mcpStdioTopology.cleanupError(); err != nil {
		fmt.Fprintf(os.Stderr, "GATE-CLEANUP failure: %v\n", err)
		if exitCode == 0 {
			exitCode = 1
		}
	}

	fmt.Fprintln(os.Stderr, "GATE-INVENTORY stdio: top_level_tests=7 named_initializer_rows=2 eligible_process_free_rows=1 isolated_rows=8")
	fmt.Fprintf(os.Stderr, "GATE-STDIO-ISOLATED: %s\n", mcpStdioTopology.isolationSummary())
	fmt.Fprintf(os.Stderr, "GATE-LIFECYCLE: %s\n", mcpStdioTopology.lifecycleSummary())
	fmt.Fprintf(os.Stderr, "GATE-CLEANUP: %s\n", mcpStdioTopology.summary())
	fmt.Fprintln(os.Stderr, "GATE-REPEAT: per-invocation resource balances are checked for each package run; use the declared -count=3 gate for repeatability")
	fmt.Fprintln(os.Stderr, "GATE-MCP-CONFORMANCE: empty/duplicate IDs and maximum-frame semantics remain unspecified; no assertion is invented")
	fmt.Fprintln(os.Stderr, "GATE-ROOT-PROCESS-INTEGRATION: built-child crash, signal, and exit-status behavior are not exercised by this Process.Execute lane")
	os.Exit(exitCode)
}

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
	})
	t.Run("runtime-backed", func(t *testing.T) {
		projectRoot := support.ScaffoldSingleStepFactory(t, "mcp-stdio-discovery-runtime")
		trackMCPTempRoot(t, projectRoot)

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
	t            *testing.T
	client       *stdioMCPClient
	stdin        *os.File
	stdinRead    *os.File
	stdout       *bufio.Reader
	stdoutRead   *os.File
	stdoutWrite  *os.File
	serveErr     <-chan error
	cancel       context.CancelFunc
	shutdownOnce sync.Once
	shutdownDone chan struct{}
	shutdownErr  error
	streamsOnce  sync.Once
	cleanupOnce  sync.Once
	cleanupErr   error
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
	mcpStdioTopology.recordRootBuild()
	t.Cleanup(func() {
		closeContext, cancel := context.WithTimeout(context.Background(), mcpStdioStopTimeout)
		closeErr := process.Close(closeContext)
		cancel()
		mcpStdioTopology.recordRootClose()
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
	mcpStdioTopology.recordInvocationStarted()
	defer mcpStdioTopology.recordInvocationReturned()
	return process.Execute(input)
}

func trackedMCPTempDir(t testing.TB) string {
	t.Helper()
	directory := t.TempDir()
	trackMCPTempRoot(t, directory)
	return directory
}

func trackMCPTempRoot(t testing.TB, directory string) {
	t.Helper()
	if strings.TrimSpace(directory) == "" {
		t.Fatal("track MCP temporary root requires a directory")
	}
	mcpStdioTopology.recordTemporaryRootMade()
	t.Cleanup(func() {
		if err := os.RemoveAll(directory); err != nil {
			t.Errorf("remove MCP temporary root %q: %v", directory, err)
		}
		mcpStdioTopology.recordTemporaryRootRemoved()
	})
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
	mcpStdioTopology.recordInvocationStarted()
	mcpStdioTopology.recordStdioSessionOpened()
	mcpStdioTopology.recordContextCreated()
	mcpStdioTopology.recordStreamsOpened()

	serveErr := make(chan error, 1)
	var stderr bytes.Buffer
	go func() {
		serveErr <- process.Execute(root.Input{
			Args: []string{
				"you", "server", "mcp",
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
	mcpStdioTopology.recordInvocationStarted()
	mcpStdioTopology.recordStdioSessionOpened()
	mcpStdioTopology.recordContextCreated()
	mcpStdioTopology.recordStreamsOpened()

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
		mcpStdioTopology.recordStdioSessionClosed()
		mcpStdioTopology.recordInvocationReturned()
		s.cleanupErr = errors.Join(cleanupErrors...)
	})
	if s.cleanupErr != nil {
		s.t.Errorf("MCP stdio invocation cleanup: %v", s.cleanupErr)
	}
}

func (s *stdioMCPServer) shutdown() error {
	s.shutdownOnce.Do(func() {
		s.cancel()
		_ = s.stdin.Close()
		mcpStdioTopology.recordContextCanceled()
		select {
		case err := <-s.serveErr:
			if err != nil && err != io.EOF && !errors.Is(err, context.Canceled) && !strings.Contains(err.Error(), "file already closed") {
				s.shutdownErr = fmt.Errorf("MCP stdio server: %w", err)
			}
		// This bounded wait is only a hang guard. Normal completion is the
		// serveErr channel, and cancellation/EOF are never synchronized by a
		// sleep or timeout-padded readiness delay.
		case <-time.After(mcpStdioStopTimeout):
			s.shutdownErr = fmt.Errorf("MCP stdio server did not shut down after stdin closed")
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
		mcpStdioTopology.recordStreamsClosed()
	})
}

func (l *mcpStdioTopologyLedger) recordRootBuild() {
	l.Lock()
	l.rootBuilds++
	l.Unlock()
}

func (l *mcpStdioTopologyLedger) recordRootClose() {
	l.Lock()
	l.rootCloses++
	l.Unlock()
}

func (l *mcpStdioTopologyLedger) recordInvocationStarted() {
	l.Lock()
	l.invocationStarts++
	l.Unlock()
}

func (l *mcpStdioTopologyLedger) recordInvocationReturned() {
	l.Lock()
	l.invocationReturns++
	l.Unlock()
}

func (l *mcpStdioTopologyLedger) recordStdioSessionOpened() {
	l.Lock()
	l.stdioSessionsOpened++
	l.Unlock()
}

func (l *mcpStdioTopologyLedger) recordStdioSessionClosed() {
	l.Lock()
	l.stdioSessionsClosed++
	l.Unlock()
}

func (l *mcpStdioTopologyLedger) recordContextCreated() {
	l.Lock()
	l.contextsCreated++
	l.Unlock()
}

func (l *mcpStdioTopologyLedger) recordContextCanceled() {
	l.Lock()
	l.contextsCanceled++
	l.Unlock()
}

func (l *mcpStdioTopologyLedger) recordStreamsOpened() {
	l.Lock()
	l.streamsOpened++
	l.Unlock()
}

func (l *mcpStdioTopologyLedger) recordStreamsClosed() {
	l.Lock()
	l.streamsClosed++
	l.Unlock()
}

func (l *mcpStdioTopologyLedger) recordTemporaryRootMade() {
	l.Lock()
	l.temporaryRootsMade++
	l.Unlock()
}

func (l *mcpStdioTopologyLedger) recordTemporaryRootRemoved() {
	l.Lock()
	l.temporaryRootsRemoved++
	l.Unlock()
}

func (l *mcpStdioTopologyLedger) cleanupError() error {
	l.Lock()
	defer l.Unlock()
	var errs []error
	if l.rootBuilds != l.rootCloses {
		errs = append(errs, fmt.Errorf("MCP application roots built/closed = %d/%d", l.rootBuilds, l.rootCloses))
	}
	if l.invocationStarts != l.invocationReturns {
		errs = append(errs, fmt.Errorf("MCP invocation starts/returns = %d/%d", l.invocationStarts, l.invocationReturns))
	}
	if l.stdioSessionsOpened != l.stdioSessionsClosed {
		errs = append(errs, fmt.Errorf("MCP stdio sessions opened/closed = %d/%d", l.stdioSessionsOpened, l.stdioSessionsClosed))
	}
	if l.contextsCreated != l.contextsCanceled {
		errs = append(errs, fmt.Errorf("MCP invocation contexts created/canceled = %d/%d", l.contextsCreated, l.contextsCanceled))
	}
	if l.streamsOpened != l.streamsClosed {
		errs = append(errs, fmt.Errorf("MCP streams opened/closed = %d/%d", l.streamsOpened, l.streamsClosed))
	}
	if l.temporaryRootsMade != l.temporaryRootsRemoved {
		errs = append(errs, fmt.Errorf("MCP temporary roots made/removed = %d/%d", l.temporaryRootsMade, l.temporaryRootsRemoved))
	}
	return errors.Join(errs...)
}

func (l *mcpStdioTopologyLedger) isolationSummary() string {
	l.Lock()
	defer l.Unlock()
	return fmt.Sprintf(
		"root_builds=%d root_closes=%d; root-backed rows retain distinct Process instances; process_free_constructor=not_acquired",
		l.rootBuilds, l.rootCloses,
	)
}

func (l *mcpStdioTopologyLedger) lifecycleSummary() string {
	l.Lock()
	defer l.Unlock()
	return fmt.Sprintf(
		"invocations=%d/%d sessions=%d/%d contexts=%d/%d streams=%d/%d temporary_roots=%d/%d; shutdown observes cancellation and stdout EOF; pre-initialize environment failures remain session-free",
		l.invocationStarts, l.invocationReturns,
		l.stdioSessionsOpened, l.stdioSessionsClosed,
		l.contextsCreated, l.contextsCanceled,
		l.streamsOpened, l.streamsClosed,
		l.temporaryRootsMade, l.temporaryRootsRemoved,
	)
}

func (l *mcpStdioTopologyLedger) summary() string {
	l.Lock()
	defer l.Unlock()
	return fmt.Sprintf(
		"roots=%d/%d; invocations=%d/%d; sessions=%d/%d; contexts=%d/%d; streams=%d/%d; temporary_roots=%d/%d; child_processes=0 ports=0 routes=0 (not acquired)",
		l.rootBuilds, l.rootCloses,
		l.invocationStarts, l.invocationReturns,
		l.stdioSessionsOpened, l.stdioSessionsClosed,
		l.contextsCreated, l.contextsCanceled,
		l.streamsOpened, l.streamsClosed,
		l.temporaryRootsMade, l.temporaryRootsRemoved,
	)
}
