package protocol_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	jsonRPCInvalidParamsCode   = -32602
	factorySessionNotFoundCode = "factory_session.session.not_found"
	missingFactorySessionID    = "dur-sess-missing-999"
	factorySessionGetToolName  = "you.factory_session.get"
	mcpProtocolStopTimeout     = 5 * time.Second
)

type mcpProtocolPackageFixture struct {
	sync.Mutex
	process   support.ApplicationProcess
	buildErr  error
	closeOnce sync.Once
	closeErr  error
}

var sharedMCPProtocolFixture mcpProtocolPackageFixture

type mcpProtocolTopologyLedger struct {
	sync.Mutex
	sharedRootBuilds      int
	isolatedRootBuilds    int
	sharedRootCloses      int
	isolatedRootCloses    int
	invocationStarts      int
	invocationReturns     int
	contextsCanceled      int
	streamsOpened         int
	streamsClosed         int
	temporaryRootsMade    int
	temporaryRootsRemoved int
	homeRootsMade         int
	homeRootsRemoved      int
}

var mcpProtocolTopology mcpProtocolTopologyLedger

// TestMain owns the package-scoped application root used by the two eligible
// request/error rows. The shutdown row deliberately owns a separate root so
// its whole-protocol stdio boundary remains an isolated witness.
func TestMain(m *testing.M) {
	exitCode := m.Run()

	if err := closeSharedMCPProtocolFixture(); err != nil {
		fmt.Fprintf(os.Stderr, "close shared MCP protocol process: %v\n", err)
		if exitCode == 0 {
			exitCode = 1
		}
	}
	if err := mcpProtocolTopology.cleanupError(); err != nil {
		fmt.Fprintf(os.Stderr, "MCP protocol cleanup accounting: %v\n", err)
		if exitCode == 0 {
			exitCode = 1
		}
	}
	fmt.Fprintf(os.Stderr, "GATE-PROTOCOL topology: %s\n", mcpProtocolTopology.summary())
	os.Exit(exitCode)
}

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
	withSharedMCPProtocolServer(t, func(server *fixtureBackedMCPServer) {
		assertInitializeHandshake(t, server)
		response := server.exchange(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{}}`)
		assertMCPResponseID(t, response, float64(1))
		if response.Error == nil {
			t.Fatalf("tools/call with empty params returned success %#v, want invalid-params error", response)
		}
		if response.Error.Code != jsonRPCInvalidParamsCode {
			t.Fatalf("tools/call error code = %d, want %d (invalid-params)", response.Error.Code, jsonRPCInvalidParamsCode)
		}
	})
}

// TestMCPMissingFactorySessionReturnsCanonicalNotFound proves a well-formed Factory
// Session tools/call for a missing session id returns the canonical not-found
// result at the public MCP stdio/protocol boundary.
func TestMCPMissingFactorySessionReturnsCanonicalNotFound(t *testing.T) {
	withSharedMCPProtocolServer(t, func(server *fixtureBackedMCPServer) {
		assertInitializeHandshake(t, server)
		request := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"` + factorySessionGetToolName +
			`","arguments":{"sessionId":"` + missingFactorySessionID + `"}}}`
		response := server.exchange(request)
		assertMCPResponseID(t, response, float64(1))
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
	})
}

// TestMCPServerShutdownClosesStdioCleanly proves MCP server shutdown terminates
// stdio serve cleanly without hung streams or unclean protocol failures.
func TestMCPServerShutdownClosesStdioCleanly(t *testing.T) {
	// Keep this root isolated: whole-protocol cancellation and stdout EOF are
	// the lifecycle witness, so sharing the package root would blur ownership.
	server := startFixtureBackedMCPServer(t)
	defer server.cleanup()

	assertInitializeHandshake(t, server)
	assertFixtureBackedMCPServerShutdownClean(t, server)
}

type fixtureBackedMCPServer struct {
	t                *testing.T
	process          support.ApplicationProcess
	ownsProcess      bool
	stdin            *os.File
	stdinRead        *os.File
	stdout           *bufio.Reader
	stdoutRead       *os.File
	stdoutWrite      *os.File
	serveErr         <-chan error
	cancel           context.CancelFunc
	homeDirectory    string
	workingDirectory string
	shutdownOnce     sync.Once
	shutdownDone     chan struct{}
	shutdownErr      error
	streamsOnce      sync.Once
	cleanupOnce      sync.Once
	cleanupErr       error
}

func startFixtureBackedMCPServer(t *testing.T) *fixtureBackedMCPServer {
	t.Helper()

	process, err := support.BuildProcessWithContext(context.Background(), serviceedges.Edges{})
	if err != nil {
		t.Fatalf("BuildProcess: %v", err)
	}
	mcpProtocolTopology.recordIsolatedRootBuild()
	return startFixtureBackedMCPServerWithProcess(t, process, true)
}

func withSharedMCPProtocolServer(t *testing.T, run func(*fixtureBackedMCPServer)) {
	t.Helper()

	sharedMCPProtocolFixture.Lock()
	defer sharedMCPProtocolFixture.Unlock()
	if sharedMCPProtocolFixture.process == nil && sharedMCPProtocolFixture.buildErr == nil {
		sharedMCPProtocolFixture.process, sharedMCPProtocolFixture.buildErr = support.BuildProcessWithContext(
			context.Background(), serviceedges.Edges{},
		)
		if sharedMCPProtocolFixture.buildErr == nil {
			mcpProtocolTopology.recordSharedRootBuild()
		}
	}
	if sharedMCPProtocolFixture.buildErr != nil {
		t.Fatalf("BuildProcess() for shared MCP protocol rows: %v", sharedMCPProtocolFixture.buildErr)
	}

	server := startFixtureBackedMCPServerWithProcess(t, sharedMCPProtocolFixture.process, false)
	defer server.cleanup()
	run(server)
}

func closeSharedMCPProtocolFixture() error {
	sharedMCPProtocolFixture.Lock()
	process := sharedMCPProtocolFixture.process
	sharedMCPProtocolFixture.Unlock()
	if process == nil {
		return nil
	}

	sharedMCPProtocolFixture.closeOnce.Do(func() {
		closeContext, cancel := context.WithTimeout(context.Background(), mcpProtocolStopTimeout)
		defer cancel()
		sharedMCPProtocolFixture.closeErr = process.Close(closeContext)
		mcpProtocolTopology.recordSharedRootClose()
	})
	return sharedMCPProtocolFixture.closeErr
}

func startFixtureBackedMCPServerWithProcess(
	t *testing.T,
	process support.ApplicationProcess,
	ownsProcess bool,
) *fixtureBackedMCPServer {
	t.Helper()
	if process == nil {
		t.Fatal("start fixture-backed MCP server requires an application process")
	}

	stdinRead, stdinWrite, err := os.Pipe()
	if err != nil {
		closeMCPProcessAfterStartFailure(process, ownsProcess)
		t.Fatalf("stdin pipe: %v", err)
	}
	stdoutRead, stdoutWrite, err := os.Pipe()
	if err != nil {
		_ = stdinRead.Close()
		_ = stdinWrite.Close()
		closeMCPProcessAfterStartFailure(process, ownsProcess)
		t.Fatalf("stdout pipe: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	fixturePath := testutil.MustRepoPath(t, "pkg/transports/http/testdata/durable-session-contract-fixtures.json")
	homeDirectory := t.TempDir()
	workingDirectory := t.TempDir()
	mcpProtocolTopology.recordHomeRootMade()

	serveErr := make(chan error, 1)
	var stderr bytes.Buffer
	go func() {
		serveErr <- process.Execute(root.Input{
			Args:             []string{"you", "server", "mcp", "--fixture-catalog", fixturePath},
			Env:              mcpProtocolEnvironmentWithHome(homeDirectory),
			Stdin:            stdinRead,
			Stdout:           stdoutWrite,
			Stderr:           &stderr,
			Context:          ctx,
			WorkingDirectory: workingDirectory,
		})
	}()
	server := &fixtureBackedMCPServer{
		t:                t,
		process:          process,
		ownsProcess:      ownsProcess,
		stdin:            stdinWrite,
		stdinRead:        stdinRead,
		stdout:           bufio.NewReader(stdoutRead),
		stdoutRead:       stdoutRead,
		stdoutWrite:      stdoutWrite,
		serveErr:         serveErr,
		cancel:           cancel,
		homeDirectory:    homeDirectory,
		workingDirectory: workingDirectory,
		shutdownDone:     make(chan struct{}),
	}
	mcpProtocolTopology.recordInvocationStarted()
	t.Cleanup(server.cleanup)
	return server
}

func mcpProtocolEnvironmentWithHome(homeDirectory string) []string {
	environment := make([]string, 0, len(os.Environ())+2)
	for _, entry := range os.Environ() {
		if strings.HasPrefix(entry, "HOME=") || strings.HasPrefix(entry, "USERPROFILE=") {
			continue
		}
		environment = append(environment, entry)
	}
	return append(environment, "HOME="+homeDirectory, "USERPROFILE="+homeDirectory)
}

func assertInitializeHandshake(t *testing.T, server *fixtureBackedMCPServer) {
	t.Helper()

	initResponse := server.exchange(`{"jsonrpc":"2.0","id":"init","method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"protocol-errors-test","version":"test"}}}`)
	assertMCPResponseID(t, initResponse, "init")
	if initResponse.Error != nil {
		t.Fatalf("initialize error = %#v", initResponse.Error)
	}
	if _, err := server.stdin.Write([]byte(`{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n")); err != nil {
		t.Fatalf("write initialized notification: %v", err)
	}
}

func assertMCPResponseID(t *testing.T, response mcpJSONRPCResponse, want any) {
	t.Helper()
	if !reflect.DeepEqual(response.ID, want) {
		t.Fatalf("MCP response id = %#v, want %#v", response.ID, want)
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

// cleanup is the owner of one invocation's streams, context, temporary
// working root, and (for isolated scenarios) application root. It is
// idempotent because both a scenario defer and testing.T cleanup protect the
// same real resources.
func (s *fixtureBackedMCPServer) cleanup() {
	s.cleanupOnce.Do(func() {
		var cleanupErrors []error
		if err := s.shutdown(); err != nil {
			cleanupErrors = append(cleanupErrors, err)
		}
		s.closeStreams()
		if s.ownsProcess {
			closeContext, cancel := context.WithTimeout(context.Background(), mcpProtocolStopTimeout)
			closeErr := s.process.Close(closeContext)
			cancel()
			if closeErr != nil {
				cleanupErrors = append(cleanupErrors, fmt.Errorf("close isolated MCP protocol process: %w", closeErr))
			}
			mcpProtocolTopology.recordIsolatedRootClose()
		}
		if err := os.RemoveAll(s.workingDirectory); err != nil {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("remove MCP protocol working directory: %w", err))
		}
		mcpProtocolTopology.recordTemporaryRootRemoved()
		if err := os.RemoveAll(s.homeDirectory); err != nil {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("remove MCP protocol home directory: %w", err))
		}
		mcpProtocolTopology.recordHomeRootRemoved()
		mcpProtocolTopology.recordInvocationReturned()
		s.cleanupErr = errors.Join(cleanupErrors...)
	})
	if s.cleanupErr != nil {
		s.t.Errorf("MCP protocol invocation cleanup: %v", s.cleanupErr)
	}
}

func (s *fixtureBackedMCPServer) shutdown() error {
	s.shutdownOnce.Do(func() {
		s.cancel()
		_ = s.stdin.Close()
		mcpProtocolTopology.recordContextCanceled()
		select {
		case err := <-s.serveErr:
			if err != nil && err != io.EOF && !errors.Is(err, context.Canceled) && !strings.Contains(err.Error(), "file already closed") {
				s.shutdownErr = fmt.Errorf("fixture-backed MCP server: %w", err)
			}
		// This bounded wait is only a hang guard. A returned serveErr is the
		// deterministic completion signal; the timeout protects test cleanup
		// from a genuinely stuck stream without acting as synchronization.
		case <-time.After(mcpProtocolStopTimeout):
			s.shutdownErr = fmt.Errorf("fixture-backed MCP server did not shut down after stdin closed")
		}
		close(s.shutdownDone)
	})
	<-s.shutdownDone
	return s.shutdownErr
}

func (s *fixtureBackedMCPServer) closeStreams() {
	s.streamsOnce.Do(func() {
		_ = s.stdinRead.Close()
		_ = s.stdin.Close()
		_ = s.stdoutRead.Close()
		_ = s.stdoutWrite.Close()
		mcpProtocolTopology.recordStreamsClosed()
	})
}

func closeMCPProcessAfterStartFailure(process support.ApplicationProcess, ownsProcess bool) {
	if !ownsProcess || process == nil {
		return
	}
	closeContext, cancel := context.WithTimeout(context.Background(), mcpProtocolStopTimeout)
	_ = process.Close(closeContext)
	cancel()
	mcpProtocolTopology.recordIsolatedRootClose()
}

func assertFixtureBackedMCPServerShutdownClean(t *testing.T, server *fixtureBackedMCPServer) {
	t.Helper()

	if err := server.shutdown(); err != nil {
		t.Fatalf("fixture-backed MCP server shutdown: %v", err)
	}

	_ = server.stdoutWrite.Close()
	if _, err := server.stdout.ReadByte(); err != io.EOF {
		t.Fatalf("read stdout after shutdown = %v, want EOF (no hung stream)", err)
	}
}

func (l *mcpProtocolTopologyLedger) recordSharedRootBuild() {
	l.Lock()
	l.sharedRootBuilds++
	l.Unlock()
}

func (l *mcpProtocolTopologyLedger) recordIsolatedRootBuild() {
	l.Lock()
	l.isolatedRootBuilds++
	l.Unlock()
}

func (l *mcpProtocolTopologyLedger) recordSharedRootClose() {
	l.Lock()
	l.sharedRootCloses++
	l.Unlock()
}

func (l *mcpProtocolTopologyLedger) recordIsolatedRootClose() {
	l.Lock()
	l.isolatedRootCloses++
	l.Unlock()
}

func (l *mcpProtocolTopologyLedger) recordInvocationStarted() {
	l.Lock()
	l.invocationStarts++
	l.streamsOpened++
	l.temporaryRootsMade++
	l.Unlock()
}

func (l *mcpProtocolTopologyLedger) recordInvocationReturned() {
	l.Lock()
	l.invocationReturns++
	l.Unlock()
}

func (l *mcpProtocolTopologyLedger) recordContextCanceled() {
	l.Lock()
	l.contextsCanceled++
	l.Unlock()
}

func (l *mcpProtocolTopologyLedger) recordStreamsClosed() {
	l.Lock()
	l.streamsClosed++
	l.Unlock()
}

func (l *mcpProtocolTopologyLedger) recordTemporaryRootRemoved() {
	l.Lock()
	l.temporaryRootsRemoved++
	l.Unlock()
}

func (l *mcpProtocolTopologyLedger) recordHomeRootMade() {
	l.Lock()
	l.homeRootsMade++
	l.Unlock()
}

func (l *mcpProtocolTopologyLedger) recordHomeRootRemoved() {
	l.Lock()
	l.homeRootsRemoved++
	l.Unlock()
}

func (l *mcpProtocolTopologyLedger) cleanupError() error {
	l.Lock()
	defer l.Unlock()
	var errs []error
	if l.sharedRootBuilds != 0 && l.sharedRootCloses != 1 {
		errs = append(errs, fmt.Errorf("shared application roots built/closed = %d/%d, want one close", l.sharedRootBuilds, l.sharedRootCloses))
	}
	if l.isolatedRootBuilds != l.isolatedRootCloses {
		errs = append(errs, fmt.Errorf("isolated application roots built/closed = %d/%d", l.isolatedRootBuilds, l.isolatedRootCloses))
	}
	if l.invocationStarts != l.invocationReturns {
		errs = append(errs, fmt.Errorf("MCP invocation starts/returns = %d/%d", l.invocationStarts, l.invocationReturns))
	}
	if l.contextsCanceled != l.invocationStarts {
		errs = append(errs, fmt.Errorf("MCP invocation contexts canceled/started = %d/%d", l.contextsCanceled, l.invocationStarts))
	}
	if l.streamsOpened != l.streamsClosed {
		errs = append(errs, fmt.Errorf("MCP streams opened/closed = %d/%d", l.streamsOpened, l.streamsClosed))
	}
	if l.temporaryRootsMade != l.temporaryRootsRemoved {
		errs = append(errs, fmt.Errorf("MCP temporary roots made/removed = %d/%d", l.temporaryRootsMade, l.temporaryRootsRemoved))
	}
	if l.homeRootsMade != l.homeRootsRemoved {
		errs = append(errs, fmt.Errorf("MCP home roots made/removed = %d/%d", l.homeRootsMade, l.homeRootsRemoved))
	}
	return errors.Join(errs...)
}

func (l *mcpProtocolTopologyLedger) summary() string {
	l.Lock()
	defer l.Unlock()
	return fmt.Sprintf(
		"roots shared=%d/%d isolated=%d/%d; invocations=%d/%d; contexts=%d; streams=%d/%d; temporary_roots=%d/%d home_roots=%d/%d; child_processes=0 ports=0 routes=0 (not acquired)",
		l.sharedRootBuilds, l.sharedRootCloses,
		l.isolatedRootBuilds, l.isolatedRootCloses,
		l.invocationStarts, l.invocationReturns,
		l.contextsCanceled,
		l.streamsOpened, l.streamsClosed,
		l.temporaryRootsMade, l.temporaryRootsRemoved,
		l.homeRootsMade, l.homeRootsRemoved,
	)
}
