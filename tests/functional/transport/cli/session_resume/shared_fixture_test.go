package session_resume_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const cliResumePackageTimeout = 15 * time.Second

var cliResumePackageState struct {
	sync.Once
	fixture *cliResumePackageFixture
	err     error
}

// TestMain owns the package-scoped root and loopback server. Individual tests
// retain their top-level identities while their public Factory Sessions and
// provider state remain invocation-local.
func TestMain(m *testing.M) {
	exitCode := m.Run()
	if cliResumePackageState.fixture != nil {
		if err := cliResumePackageState.fixture.close(); err != nil {
			fmt.Fprintf(os.Stderr, "close shared CLI resume fixture: %v\n", err)
			if exitCode == 0 {
				exitCode = 1
			}
		}
	}
	os.Exit(exitCode)
}

type cliResumePackageFixture struct {
	rootDir     string
	projectRoot string
	homeDir     string
	serverURL   string

	process  support.ApplicationProcess
	command  *cliResumeHostedCommand
	api      *support.ProcessAPIServer
	provider *cliResumeProviderRouter

	// Process.Execute creates fresh command trees, but the shared process is
	// deliberately driven one CLI invocation at a time. HTTP session work can
	// still overlap because its provider state is keyed by session identity.
	executeMu sync.Mutex

	rootBuilds   atomic.Int32
	serverStarts atomic.Int32
	nextRequest  atomic.Uint64

	sessionMu        sync.Mutex
	openedSessions   map[string]struct{}
	terminalSessions map[string]struct{}
}

type cliResumeSmokeHarness struct {
	serverURL   string
	projectRoot string
	process     cliResumeProcess
	provider    *cliResumeProviderRouter
	fixture     *cliResumePackageFixture
}

type cliResumeProcess interface {
	Execute(root.Input) error
}

func cliResumePackageFixtureForTest(t testing.TB) *cliResumePackageFixture {
	t.Helper()
	cliResumePackageState.Do(func() {
		cliResumePackageState.fixture, cliResumePackageState.err = newCLIResumePackageFixture()
	})
	if cliResumePackageState.err != nil {
		t.Fatalf("start shared CLI resume fixture: %v", cliResumePackageState.err)
	}
	if cliResumePackageState.fixture == nil {
		t.Fatal("shared CLI resume fixture is unavailable")
	}
	return cliResumePackageState.fixture
}

func newCLIResumeSmokeHarness(t *testing.T) *cliResumeSmokeHarness {
	t.Helper()
	return newSharedCLIResumeHarness(t)
}

func newCLIResumeSmokeSucceededHarness(t *testing.T) *cliResumeSmokeHarness {
	t.Helper()
	return newSharedCLIResumeHarness(t)
}

func newCLIResumeSmokeRunningHarness(t *testing.T) *cliResumeSmokeHarness {
	t.Helper()
	return newSharedCLIResumeHarness(t)
}

func newSharedCLIResumeHarness(t *testing.T) *cliResumeSmokeHarness {
	t.Helper()
	fixture := cliResumePackageFixtureForTest(t)
	return &cliResumeSmokeHarness{
		serverURL:   fixture.serverURL,
		projectRoot: fixture.projectRoot,
		process:     fixture.process,
		provider:    fixture.provider,
		fixture:     fixture,
	}
}

func newCLIResumePackageFixture() (*cliResumePackageFixture, error) {
	rootDir, err := os.MkdirTemp("", "you-functional-cli-resume-")
	if err != nil {
		return nil, fmt.Errorf("create fixture root: %w", err)
	}
	removeRoot := func() { _ = os.RemoveAll(rootDir) }

	fixture := &cliResumePackageFixture{
		rootDir:          rootDir,
		projectRoot:      filepath.Join(rootDir, "factory"),
		homeDir:          filepath.Join(rootDir, "home"),
		api:              support.NewProcessAPIServer(),
		provider:         newCLIResumeProviderRouter(),
		openedSessions:   make(map[string]struct{}),
		terminalSessions: make(map[string]struct{}),
	}
	if err := prepareCLIResumeFactory(fixture.projectRoot); err != nil {
		removeRoot()
		return nil, err
	}
	if err := os.MkdirAll(fixture.homeDir, 0o755); err != nil {
		removeRoot()
		return nil, fmt.Errorf("create fixture home: %w", err)
	}

	starter := func(ctx context.Context, request platformhttpserver.StartRequest) error {
		fixture.serverStarts.Add(1)
		return fixture.api.Start(ctx, request)
	}
	fixture.rootBuilds.Add(1)
	process, err := support.BuildProcessWithContext(context.Background(), serviceedges.Edges{
		APIServerStarter: starter,
		ProviderOverride: fixture.provider,
	})
	if err != nil {
		removeRoot()
		return nil, fmt.Errorf("build shared root process: %w", err)
	}
	fixture.process = process

	stdinIsTTY := true
	inputs := root.Input{
		Args: []string{
			"you", "run", "--dir", fixture.projectRoot, "--continuously", "--with-server", "--quiet", "--no-record",
		},
		Env:              []string{"HOME=" + fixture.homeDir, "USERPROFILE=" + fixture.homeDir},
		Context:          context.Background(),
		WorkingDirectory: fixture.projectRoot,
		StdinIsTTY:       &stdinIsTTY,
	}
	fixture.command = startCLIResumeHostedCommand(process, inputs)
	fixture.serverURL, err = fixture.api.WaitForBaseURL(cliResumePackageTimeout)
	if err != nil {
		cleanupErr := fixture.close()
		return nil, errors.Join(fmt.Errorf("wait for shared CLI resume API: %w", err), cleanupErr)
	}
	return fixture, nil
}

func prepareCLIResumeFactory(projectRoot string) error {
	if err := os.MkdirAll(filepath.Join(projectRoot, ".claude", "workflows"), 0o755); err != nil {
		return fmt.Errorf("create CLI resume workflow directory: %w", err)
	}
	factoryConfig := map[string]any{
		"name": "cli-resume-smoke",
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
		return fmt.Errorf("marshal CLI resume factory: %w", err)
	}
	if err := os.WriteFile(filepath.Join(projectRoot, "factory.json"), raw, 0o644); err != nil {
		return fmt.Errorf("write CLI resume factory: %w", err)
	}
	workstationDir := filepath.Join(projectRoot, "workstations", "process")
	if err := os.MkdirAll(workstationDir, 0o755); err != nil {
		return fmt.Errorf("create CLI resume workstation: %w", err)
	}
	if err := os.WriteFile(
		filepath.Join(workstationDir, "AGENTS.md"),
		[]byte("---\ntype: MODEL_WORKSTATION\n---\nDo the work.\n"),
		0o644,
	); err != nil {
		return fmt.Errorf("write CLI resume workstation: %w", err)
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
			return fmt.Errorf("read CLI resume workflow fixture %s: %w", workflow.fixture, err)
		}
		if err := os.WriteFile(filepath.Join(projectRoot, ".claude", "workflows", workflow.name+".js"), content, 0o600); err != nil {
			return fmt.Errorf("write CLI resume workflow %s: %w", workflow.name, err)
		}
	}
	return nil
}

type cliResumeHostedCommand struct {
	cancel context.CancelFunc
	done   chan struct{}

	mu  sync.Mutex
	err error
}

func startCLIResumeHostedCommand(process cliResumeProcess, input root.Input) *cliResumeHostedCommand {
	ctx, cancel := context.WithCancel(input.Context)
	input.Context = ctx
	command := &cliResumeHostedCommand{cancel: cancel, done: make(chan struct{})}
	go func() {
		err := process.Execute(input)
		command.mu.Lock()
		command.err = err
		command.mu.Unlock()
		close(command.done)
	}()
	return command
}

func (command *cliResumeHostedCommand) stop() error {
	if command == nil {
		return nil
	}
	command.cancel()
	timer := time.NewTimer(cliResumePackageTimeout)
	defer timer.Stop()
	select {
	case <-command.done:
		command.mu.Lock()
		err := command.err
		command.mu.Unlock()
		if err != nil && !errors.Is(err, context.Canceled) {
			return err
		}
		return nil
	case <-timer.C:
		return fmt.Errorf("timed out waiting for shared CLI resume host shutdown")
	}
}

func (fixture *cliResumePackageFixture) close() error {
	if fixture == nil {
		return nil
	}
	var closeErr error
	if fixture.command != nil {
		closeErr = errors.Join(closeErr, fixture.command.stop())
	}
	if fixture.process != nil {
		ctx, cancel := context.WithTimeout(context.Background(), cliResumePackageTimeout)
		closeErr = errors.Join(closeErr, fixture.process.Close(ctx))
		cancel()
	}
	if got := fixture.rootBuilds.Load(); got != 1 {
		closeErr = errors.Join(closeErr, fmt.Errorf("shared CLI resume root builds = %d, want 1", got))
	}
	if got := fixture.serverStarts.Load(); got != 1 {
		closeErr = errors.Join(closeErr, fmt.Errorf("shared CLI resume server starts = %d, want 1", got))
	}
	if got := fixture.provider.routeCount(); got != 0 {
		closeErr = errors.Join(closeErr, fmt.Errorf("shared CLI resume provider routes after cleanup = %d", got))
	}
	closeErr = errors.Join(closeErr, fixture.sessionLifecycleError())
	if err := os.RemoveAll(fixture.rootDir); err != nil {
		closeErr = errors.Join(closeErr, fmt.Errorf("remove shared CLI resume fixture root: %w", err))
	} else if _, err := os.Stat(fixture.rootDir); !errors.Is(err, os.ErrNotExist) {
		closeErr = errors.Join(closeErr, fmt.Errorf("shared CLI resume fixture root remains after cleanup: %v", err))
	}
	return closeErr
}

func (fixture *cliResumePackageFixture) sessionLifecycleError() error {
	fixture.sessionMu.Lock()
	defer fixture.sessionMu.Unlock()
	if len(fixture.openedSessions) != len(fixture.terminalSessions) {
		return fmt.Errorf(
			"shared CLI resume session lifecycle opened %d sessions but terminal cleanup observed %d",
			len(fixture.openedSessions), len(fixture.terminalSessions),
		)
	}
	for sessionID := range fixture.openedSessions {
		if _, ok := fixture.terminalSessions[sessionID]; !ok {
			return fmt.Errorf("shared CLI resume session %q did not reach terminal cleanup", sessionID)
		}
	}
	return nil
}

func (fixture *cliResumePackageFixture) nextRequestID(purpose string) string {
	number := fixture.nextRequest.Add(1)
	return fmt.Sprintf("req-cli-resume-%s-%d", purpose, number)
}

func (harness *cliResumeSmokeHarness) startDurableSession(
	t *testing.T,
	request factoryapi.FactorySessionExecutionRequest,
) factoryapi.FactorySessionExecutionResponse {
	t.Helper()
	started := startDurableSessionViaHTTP(t, harness.serverURL, request)
	if strings.TrimSpace(started.SessionId) == "" {
		t.Fatal("shared CLI resume session id unexpectedly empty")
	}
	harness.trackSession(t, started.SessionId)
	return started
}

func (harness *cliResumeSmokeHarness) trackSession(t *testing.T, sessionID string) {
	t.Helper()
	harness.fixture.sessionMu.Lock()
	if _, exists := harness.fixture.openedSessions[sessionID]; exists {
		harness.fixture.sessionMu.Unlock()
		t.Fatalf("shared CLI resume session %q was opened twice", sessionID)
	}
	harness.fixture.openedSessions[sessionID] = struct{}{}
	harness.fixture.sessionMu.Unlock()
	t.Cleanup(func() {
		harness.cleanupSession(t, sessionID)
	})
}

func (harness *cliResumeSmokeHarness) cleanupSession(t *testing.T, sessionID string) {
	t.Helper()
	defer func() {
		if err := harness.provider.unregister(sessionID); err != nil {
			t.Errorf("unregister shared CLI resume provider route %q: %v", sessionID, err)
		}
	}()
	// Durable Factory Sessions do not expose the live-runtime /status or DELETE
	// routes. The public terminate control synchronously persists the terminal
	// durable state; the package-owned temporary root is removed after the
	// shared process is closed.
	support.TerminateFactorySessionAt(t, harness.serverURL, sessionID)
	if err := assertCLIResumeDurableSessionTerminal(harness.serverURL, sessionID); err != nil {
		t.Errorf("shared CLI resume session %q terminal cleanup probe: %v", sessionID, err)
	}
	harness.fixture.sessionMu.Lock()
	harness.fixture.terminalSessions[sessionID] = struct{}{}
	harness.fixture.sessionMu.Unlock()
}

func assertCLIResumeDurableSessionTerminal(serverURL, sessionID string) error {
	endpoint := strings.TrimSuffix(serverURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID)
	client := &http.Client{Timeout: 2 * time.Second}
	defer client.CloseIdleConnections()
	response, err := client.Get(endpoint)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		return fmt.Errorf("GET returned HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	var envelope factoryapi.FactorySessionGetResponse
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		return fmt.Errorf("decode durable session: %w", err)
	}
	durable, err := envelope.AsFactorySessionDurableReadModel()
	if err != nil {
		return fmt.Errorf("decode durable session variant: %w", err)
	}
	switch durable.Status {
	case factoryapi.FactorySessionDurableLifecycleStatusCanceled,
		factoryapi.FactorySessionDurableLifecycleStatusFailed,
		factoryapi.FactorySessionDurableLifecycleStatusInterrupted,
		factoryapi.FactorySessionDurableLifecycleStatusSucceeded,
		factoryapi.FactorySessionDurableLifecycleStatusTerminated,
		factoryapi.FactorySessionDurableLifecycleStatusTimedOut:
		return nil
	default:
		return fmt.Errorf("durable status = %q, want terminal", durable.Status)
	}
}

type cliResumeProviderRouter struct {
	testutil.NativeProvider
	mu       sync.RWMutex
	sessions map[string]*cliResumeProviderSession
}

type cliResumeProviderSession struct {
	sessionID string

	mu              sync.Mutex
	calls           int
	blockedOnce     bool
	contextCanceled bool
	executeBlocked  chan struct{}
	canceled        chan struct{}
}

func newCLIResumeProviderRouter() *cliResumeProviderRouter {
	return &cliResumeProviderRouter{
		NativeProvider: testutil.NativeProvider{},
		sessions:       make(map[string]*cliResumeProviderSession),
	}
}

func (router *cliResumeProviderRouter) Execute(
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
		return providers.ExecuteResult{}, fmt.Errorf("CLI resume provider request has no session identity")
	}
	session := router.session(sessionID)
	return session.execute(ctx)
}

func (router *cliResumeProviderRouter) session(sessionID string) *cliResumeProviderSession {
	router.mu.Lock()
	defer router.mu.Unlock()
	if session := router.sessions[sessionID]; session != nil {
		return session
	}
	session := &cliResumeProviderSession{
		sessionID:      sessionID,
		executeBlocked: make(chan struct{}),
		canceled:       make(chan struct{}),
	}
	router.sessions[sessionID] = session
	return session
}

func (router *cliResumeProviderRouter) unregister(sessionID string) error {
	router.mu.Lock()
	defer router.mu.Unlock()
	delete(router.sessions, sessionID)
	return nil
}

func (router *cliResumeProviderRouter) routeCount() int {
	router.mu.RLock()
	defer router.mu.RUnlock()
	return len(router.sessions)
}

func (router *cliResumeProviderRouter) callCount(sessionID string) int {
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

func (router *cliResumeProviderRouter) waitForExecuteBlocked(t testing.TB, sessionID string, timeout time.Duration) {
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

func (router *cliResumeProviderRouter) waitForCanceledExecute(t testing.TB, sessionID string, timeout time.Duration) {
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

func (session *cliResumeProviderSession) execute(ctx context.Context) (providers.ExecuteResult, error) {
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
		return cliResumeProviderResult(session.sessionID, "step-one", "live-provider-session-1"), nil
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
	return cliResumeProviderResult(session.sessionID, "step-two", "live-provider-session-2"), nil
}

func cliResumeProviderResult(sessionID, step, providerSessionID string) providers.ExecuteResult {
	return providers.ExecuteResult{
		Content: fmt.Sprintf(`{"text":"live:resumable-two-step-fake-children:%s:%s:workflows","label":"%s"}`, step, step, step),
		SessionRef: &providers.SessionRef{
			Provider: "mock",
			Kind:     providers.SessionIDKind,
			ID:       fmt.Sprintf("%s-%s-1", providerSessionID, sessionID),
		},
	}
}

var _ providers.Service = (*cliResumeProviderRouter)(nil)
