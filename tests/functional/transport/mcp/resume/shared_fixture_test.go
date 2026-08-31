package mcp_resume_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	mcpfactorysession "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/mcp"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const mcpResumePackageTimeout = 15 * time.Second

var mcpResumePackageState struct {
	sync.Once
	fixture *mcpResumePackageFixture
	err     error
}

// TestMain owns the one root-built MCP stdio server for this package. The
// client and server are deliberately serialized because the current stdio
// client owns one request/response stream and does not correlate concurrent
// responses.
func TestMain(m *testing.M) {
	exitCode := m.Run()
	if mcpResumePackageState.fixture != nil {
		if err := mcpResumePackageState.fixture.close(); err != nil {
			fmt.Fprintf(os.Stderr, "close shared MCP resume fixture: %v\n", err)
			if exitCode == 0 {
				exitCode = 1
			}
		}
	}
	os.Exit(exitCode)
}

type mcpResumePackageFixture struct {
	rootDir     string
	projectRoot string
	homeDir     string

	process  support.ApplicationProcess
	provider *mcpRuntimeResumeSmokeProviderRouter
	client   *stdioMCPClient

	stdinRead   *os.File
	stdinWrite  *os.File
	stdoutRead  *os.File
	stdoutWrite *os.File
	serveErr    chan error
	cancel      context.CancelFunc
	stderr      bytes.Buffer

	rootBuilds   atomic.Int32
	serverStarts atomic.Int32
	nextRequest  atomic.Uint64

	initializeOnce sync.Once
	closeOnce      sync.Once
	closeErr       error

	sessionMu        sync.Mutex
	openedSessions   map[string]struct{}
	terminalSessions map[string]struct{}
}

func mcpResumePackageFixtureForTest(t *testing.T) *mcpResumePackageFixture {
	t.Helper()
	mcpResumePackageState.Do(func() {
		mcpResumePackageState.fixture, mcpResumePackageState.err = newMCPResumePackageFixture(t)
	})
	if mcpResumePackageState.err != nil {
		t.Fatalf("start shared MCP resume fixture: %v", mcpResumePackageState.err)
	}
	if mcpResumePackageState.fixture == nil {
		t.Fatal("shared MCP resume fixture is unavailable")
	}
	fixture := mcpResumePackageState.fixture
	fixture.clientForTest(t)
	fixture.initialize(t)
	return fixture
}

func newMCPResumePackageFixture(t *testing.T) (*mcpResumePackageFixture, error) {
	t.Helper()

	rootDir, err := os.MkdirTemp("", "you-functional-mcp-resume-")
	if err != nil {
		return nil, fmt.Errorf("create fixture root: %w", err)
	}
	fixture := &mcpResumePackageFixture{
		rootDir:          rootDir,
		projectRoot:      filepath.Join(rootDir, "factory"),
		homeDir:          filepath.Join(rootDir, "home"),
		provider:         newMCPRuntimeResumeSmokeProviderRouter(),
		serveErr:         make(chan error, 1),
		openedSessions:   make(map[string]struct{}),
		terminalSessions: make(map[string]struct{}),
	}
	cleanup := func() {
		if fixture.cancel != nil {
			fixture.cancel()
		}
		closeMCPResumeStreams(fixture)
		if fixture.process != nil {
			ctx, cancel := context.WithTimeout(context.Background(), mcpResumePackageTimeout)
			_ = fixture.process.Close(ctx)
			cancel()
		}
		_ = os.RemoveAll(rootDir)
	}
	if err := prepareMCPResumeFactory(fixture.projectRoot); err != nil {
		cleanup()
		return nil, err
	}
	if err := os.MkdirAll(fixture.homeDir, 0o755); err != nil {
		cleanup()
		return nil, fmt.Errorf("create fixture home: %w", err)
	}

	process, err := support.BuildProcessWithContext(context.Background(), serviceedges.Edges{
		ProviderOverride: fixture.provider,
	})
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("build shared root process: %w", err)
	}
	fixture.process = process
	fixture.rootBuilds.Add(1)
	if err := openMCPResumeStreams(fixture); err != nil {
		cleanup()
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	fixture.cancel = cancel
	env := append(os.Environ(), "HOME="+fixture.homeDir, "USERPROFILE="+fixture.homeDir)
	fixture.serverStarts.Add(1)
	go func() {
		fixture.serveErr <- fixture.process.Execute(root.Input{
			Args:             []string{"you", "server", "mcp", "--runtime", "--project-root", fixture.projectRoot},
			Env:              env,
			Stdin:            fixture.stdinRead,
			Stdout:           fixture.stdoutWrite,
			Stderr:           &fixture.stderr,
			Context:          ctx,
			WorkingDirectory: fixture.projectRoot,
		})
	}()
	fixture.client = newStdioMCPClient(t, fixture.stdinWrite, fixture.stdoutRead)
	return fixture, nil
}

func openMCPResumeStreams(fixture *mcpResumePackageFixture) error {
	stdinRead, stdinWrite, err := os.Pipe()
	if err != nil {
		return fmt.Errorf("open MCP stdin pipe: %w", err)
	}
	stdoutRead, stdoutWrite, err := os.Pipe()
	if err != nil {
		_ = stdinRead.Close()
		_ = stdinWrite.Close()
		return fmt.Errorf("open MCP stdout pipe: %w", err)
	}
	fixture.stdinRead = stdinRead
	fixture.stdinWrite = stdinWrite
	fixture.stdoutRead = stdoutRead
	fixture.stdoutWrite = stdoutWrite
	return nil
}

func closeMCPResumeStreams(fixture *mcpResumePackageFixture) {
	for _, stream := range []*os.File{
		fixture.stdinRead,
		fixture.stdinWrite,
		fixture.stdoutRead,
		fixture.stdoutWrite,
	} {
		if stream != nil {
			_ = stream.Close()
		}
	}
}

func (fixture *mcpResumePackageFixture) initialize(t *testing.T) {
	t.Helper()
	fixture.initializeOnce.Do(func() {
		assertInstallSmokeInitialize(t, fixture.client)
	})
}

func (fixture *mcpResumePackageFixture) clientForTest(t *testing.T) *stdioMCPClient {
	t.Helper()
	fixture.client.t = t
	return fixture.client
}

func (fixture *mcpResumePackageFixture) nextRequestID(purpose string) string {
	number := fixture.nextRequest.Add(1)
	return fmt.Sprintf("req-mcp-resume-%s-%d", purpose, number)
}

func (fixture *mcpResumePackageFixture) trackSession(
	t *testing.T,
	client *stdioMCPClient,
	sessionID string,
) {
	t.Helper()
	fixture.sessionMu.Lock()
	if _, exists := fixture.openedSessions[sessionID]; exists {
		fixture.sessionMu.Unlock()
		t.Fatalf("shared MCP resume session %q was opened twice", sessionID)
	}
	fixture.openedSessions[sessionID] = struct{}{}
	fixture.sessionMu.Unlock()
	t.Cleanup(func() {
		fixture.cleanupSession(t, client, sessionID)
	})
}

func (fixture *mcpResumePackageFixture) cleanupSession(
	t *testing.T,
	client *stdioMCPClient,
	sessionID string,
) {
	t.Helper()
	response := decodeToolResponse[factoryapi.FactorySessionLifecycleControlResponse](
		t,
		client.callTool(mcpfactorysession.ToolControl, map[string]any{
			"sessionId": sessionID,
			"operation": factoryapi.FactorySessionLifecycleControlKindTerminate,
			"reason":    "shared MCP resume fixture cleanup",
		}),
	)
	if response.Error != nil || response.Result == nil {
		t.Errorf("terminate session %q = %#v, want success", sessionID, response)
		return
	}
	if response.Result.SessionId != sessionID {
		t.Errorf("terminate sessionId = %q, want %q", response.Result.SessionId, sessionID)
		return
	}
	if !waitForMCPResumeTerminal(t, client, sessionID, mcpResumePackageTimeout) {
		return
	}
	if err := fixture.provider.unregister(sessionID); err != nil {
		t.Errorf("unregister shared MCP resume provider route %q: %v", sessionID, err)
		return
	}
	fixture.sessionMu.Lock()
	fixture.terminalSessions[sessionID] = struct{}{}
	fixture.sessionMu.Unlock()
}

func waitForMCPResumeTerminal(
	t *testing.T,
	client *stdioMCPClient,
	sessionID string,
	timeout time.Duration,
) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		session := readMCPSessionDurableReadModel(t, client, sessionID)
		if isMCPResumeTerminalStatus(session.Status) {
			return true
		}
		time.Sleep(15 * time.Millisecond)
	}
	session := readMCPSessionDurableReadModel(t, client, sessionID)
	t.Errorf("session %s status = %q, want terminal within %s", sessionID, session.Status, timeout)
	return false
}

func isMCPResumeTerminalStatus(status factoryapi.FactorySessionDurableLifecycleStatus) bool {
	switch status {
	case factoryapi.FactorySessionDurableLifecycleStatusCanceled,
		factoryapi.FactorySessionDurableLifecycleStatusFailed,
		factoryapi.FactorySessionDurableLifecycleStatusInterrupted,
		factoryapi.FactorySessionDurableLifecycleStatusSucceeded,
		factoryapi.FactorySessionDurableLifecycleStatusTerminated,
		factoryapi.FactorySessionDurableLifecycleStatusTimedOut:
		return true
	default:
		return false
	}
}

func (fixture *mcpResumePackageFixture) close() error {
	fixture.closeOnce.Do(func() {
		fixture.closeErr = fixture.shutdown()
	})
	return fixture.closeErr
}

func (fixture *mcpResumePackageFixture) shutdown() error {
	var closeErr error
	if fixture.cancel != nil {
		fixture.cancel()
	}
	if fixture.stdinWrite != nil {
		_ = fixture.stdinWrite.Close()
	}
	if err := waitForMCPResumeServer(fixture.serveErr, &fixture.stderr); err != nil {
		closeErr = errors.Join(closeErr, err)
	}
	closeMCPResumeStreams(fixture)
	if fixture.process != nil {
		ctx, cancel := context.WithTimeout(context.Background(), mcpResumePackageTimeout)
		closeErr = errors.Join(closeErr, fixture.process.Close(ctx))
		cancel()
	}
	if got := fixture.rootBuilds.Load(); got != 1 {
		closeErr = errors.Join(closeErr, fmt.Errorf("shared MCP resume root builds = %d, want 1", got))
	}
	if got := fixture.serverStarts.Load(); got != 1 {
		closeErr = errors.Join(closeErr, fmt.Errorf("shared MCP resume server starts = %d, want 1", got))
	}
	if got := fixture.provider.routeCount(); got != 0 {
		closeErr = errors.Join(closeErr, fmt.Errorf("shared MCP resume provider routes after cleanup = %d", got))
	}
	closeErr = errors.Join(closeErr, fixture.sessionLifecycleError())
	if err := os.RemoveAll(fixture.rootDir); err != nil {
		closeErr = errors.Join(closeErr, fmt.Errorf("remove shared MCP resume fixture root: %w", err))
	} else if _, err := os.Stat(fixture.rootDir); !errors.Is(err, os.ErrNotExist) {
		closeErr = errors.Join(closeErr, fmt.Errorf("shared MCP resume fixture root remains: %v", err))
	}
	return closeErr
}

func waitForMCPResumeServer(serveErr <-chan error, stderr *bytes.Buffer) error {
	timer := time.NewTimer(mcpResumePackageTimeout)
	defer timer.Stop()
	select {
	case err := <-serveErr:
		if err == nil || errors.Is(err, io.EOF) || errors.Is(err, context.Canceled) {
			return nil
		}
		if strings.Contains(err.Error(), "file already closed") {
			return nil
		}
		return fmt.Errorf("MCP server: %w; stderr=%q", err, strings.TrimSpace(stderr.String()))
	case <-timer.C:
		return fmt.Errorf("MCP server did not shut down after stdin close; stderr=%q", strings.TrimSpace(stderr.String()))
	}
}

func (fixture *mcpResumePackageFixture) sessionLifecycleError() error {
	fixture.sessionMu.Lock()
	defer fixture.sessionMu.Unlock()
	if len(fixture.openedSessions) != len(fixture.terminalSessions) {
		return fmt.Errorf(
			"shared MCP resume session lifecycle opened %d sessions but terminal cleanup observed %d",
			len(fixture.openedSessions),
			len(fixture.terminalSessions),
		)
	}
	for sessionID := range fixture.openedSessions {
		if _, ok := fixture.terminalSessions[sessionID]; !ok {
			return fmt.Errorf("shared MCP resume session %q did not reach terminal cleanup", sessionID)
		}
	}
	return nil
}

func prepareMCPResumeFactory(projectRoot string) error {
	workflowDir := filepath.Join(projectRoot, ".claude", "workflows")
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		return fmt.Errorf("create MCP resume workflow directory: %w", err)
	}
	factoryConfig := map[string]any{
		"name": "mcp-resume-smoke",
		"workTypes": []any{map[string]any{
			"name": "task",
			"states": []any{
				map[string]any{"name": "init", "type": "INITIAL"},
				map[string]any{"name": "complete", "type": "TERMINAL"},
				map[string]any{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []any{map[string]any{"name": "processor"}},
		"workstations": []map[string]any{{
			"name":    "process",
			"worker":  "processor",
			"inputs":  []any{map[string]any{"workType": "task", "state": "init"}},
			"outputs": []any{map[string]any{"workType": "task", "state": "complete"}},
			"onFailure": []any{map[string]any{
				"workType": "task",
				"state":    "failed",
			}},
		}},
	}
	raw, err := json.Marshal(factoryConfig)
	if err != nil {
		return fmt.Errorf("marshal MCP resume factory: %w", err)
	}
	if err := os.WriteFile(filepath.Join(projectRoot, "factory.json"), raw, 0o644); err != nil {
		return fmt.Errorf("write MCP resume factory: %w", err)
	}
	workstationDir := filepath.Join(projectRoot, "workstations", "process")
	if err := os.MkdirAll(workstationDir, 0o755); err != nil {
		return fmt.Errorf("create MCP resume workstation: %w", err)
	}
	if err := os.WriteFile(
		filepath.Join(workstationDir, "AGENTS.md"),
		[]byte("---\ntype: MODEL_WORKSTATION\n---\nDo the work.\n"),
		0o644,
	); err != nil {
		return fmt.Errorf("write MCP resume workstation: %w", err)
	}
	for _, workflow := range []struct {
		fixture string
		name    string
	}{
		{fixture: "resumable-two-step-fake-children.workflow.js", name: "resumable-two-step-fake-children"},
		{fixture: "simple-final.workflow.js", name: "simple-final"},
		{fixture: "busy-loop.workflow.js", name: "busy-loop"},
	} {
		source := filepath.Join("..", "..", "..", "..", "fixtures", "javascript_runtime", workflow.fixture)
		content, err := os.ReadFile(source)
		if err != nil {
			return fmt.Errorf("read MCP resume workflow fixture %s: %w", workflow.fixture, err)
		}
		if err := os.WriteFile(filepath.Join(workflowDir, workflow.name+".js"), content, 0o600); err != nil {
			return fmt.Errorf("write MCP resume workflow %s: %w", workflow.name, err)
		}
	}
	return nil
}

type mcpRuntimeResumeSmokeProviderRouter struct {
	testutil.NativeProvider
	mu       sync.RWMutex
	sessions map[string]*mcpRuntimeResumeSmokeProviderSession
}

type mcpRuntimeResumeSmokeProviderSession struct {
	sessionID string

	mu              sync.Mutex
	calls           int
	blockedOnce     bool
	contextCanceled bool
	executeBlocked  chan struct{}
	canceled        chan struct{}
}

func newMCPRuntimeResumeSmokeProviderRouter() *mcpRuntimeResumeSmokeProviderRouter {
	provider := &mcpRuntimeResumeSmokeProviderRouter{
		NativeProvider: testutil.NativeProvider{},
		sessions:       make(map[string]*mcpRuntimeResumeSmokeProviderSession),
	}
	provider.NativeProvider.ExecuteFunc = provider.Execute
	return provider
}

func (router *mcpRuntimeResumeSmokeProviderRouter) Execute(
	ctx context.Context,
	request providers.ExecuteRequest,
) (providers.ExecuteResult, error) {
	sessionID := strings.TrimSpace(request.Correlation.FactorySessionID)
	if sessionID == "" {
		sessionID = strings.TrimSpace(request.SessionID)
	}
	if sessionID == "" {
		sessionID = strings.TrimSpace(request.Correlation.RequestID)
	}
	if sessionID == "" {
		return providers.ExecuteResult{}, fmt.Errorf("MCP resume provider request has no session identity")
	}
	return router.session(sessionID).execute(ctx)
}

func (router *mcpRuntimeResumeSmokeProviderRouter) session(sessionID string) *mcpRuntimeResumeSmokeProviderSession {
	router.mu.Lock()
	defer router.mu.Unlock()
	if session := router.sessions[sessionID]; session != nil {
		return session
	}
	session := &mcpRuntimeResumeSmokeProviderSession{
		sessionID:      sessionID,
		executeBlocked: make(chan struct{}),
		canceled:       make(chan struct{}),
	}
	router.sessions[sessionID] = session
	return session
}

func (router *mcpRuntimeResumeSmokeProviderRouter) unregister(sessionID string) error {
	router.mu.Lock()
	defer router.mu.Unlock()
	delete(router.sessions, sessionID)
	return nil
}

func (router *mcpRuntimeResumeSmokeProviderRouter) routeCount() int {
	router.mu.RLock()
	defer router.mu.RUnlock()
	return len(router.sessions)
}

func (router *mcpRuntimeResumeSmokeProviderRouter) callCount(sessionID string) int {
	router.mu.RLock()
	session := router.sessions[sessionID]
	router.mu.RUnlock()
	if session == nil {
		return 0
	}
	session.mu.Lock()
	defer session.mu.Unlock()
	return session.calls
}

func (router *mcpRuntimeResumeSmokeProviderRouter) waitForExecuteBlocked(
	t testing.TB,
	sessionID string,
	timeout time.Duration,
) {
	t.Helper()
	session := router.session(sessionID)
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-session.executeBlocked:
	case <-timer.C:
		t.Fatalf("provider Execute did not enter its cancellable wait for session %q", sessionID)
	}
}

func (router *mcpRuntimeResumeSmokeProviderRouter) waitForCanceledExecute(
	t testing.TB,
	sessionID string,
	timeout time.Duration,
) {
	t.Helper()
	session := router.session(sessionID)
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-session.canceled:
	case <-timer.C:
		t.Fatalf("provider Execute did not observe canceled workflow context for session %q", sessionID)
	}
}

func (session *mcpRuntimeResumeSmokeProviderSession) execute(
	ctx context.Context,
) (providers.ExecuteResult, error) {
	session.mu.Lock()
	session.calls++
	call := session.calls
	alreadyBlocked := session.blockedOnce
	if call == 2 && !alreadyBlocked {
		session.blockedOnce = true
		close(session.executeBlocked)
	}
	session.mu.Unlock()

	if call == 1 {
		return mcpRuntimeResumeSmokeProviderResult(session.sessionID, "step-one", "live-provider-session-1"), nil
	}
	if call == 2 && !alreadyBlocked {
		<-ctx.Done()
		session.mu.Lock()
		if !session.contextCanceled {
			session.contextCanceled = true
			close(session.canceled)
		}
		session.mu.Unlock()
		return providers.ExecuteResult{}, ctx.Err()
	}
	return mcpRuntimeResumeSmokeProviderResult(session.sessionID, "step-two", "live-provider-session-2"), nil
}

func mcpRuntimeResumeSmokeProviderResult(
	sessionID string,
	step string,
	providerSessionID string,
) providers.ExecuteResult {
	return providers.ExecuteResult{
		Content: fmt.Sprintf(`{"text":"live:resumable-two-step-fake-children:%s:%s:workflows","label":"%s"}`, step, step, step),
		SessionRef: &providers.SessionRef{
			Provider: "mock",
			Kind:     providers.SessionIDKind,
			ID:       fmt.Sprintf("%s-%s-1", providerSessionID, sessionID),
		},
	}
}

var _ providers.Service = (*mcpRuntimeResumeSmokeProviderRouter)(nil)
