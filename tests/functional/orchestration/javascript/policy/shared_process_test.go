package policy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	policyFixtureTimeout       = 15 * time.Second
	policyBehaviorSessionCount = 3
	policyHostWorkflow         = `return "javascript-policy-host";`
)

var (
	policyFixtureMu     sync.Mutex
	sharedPolicyFixture *policyFixture
)

// TestMain owns the package's one reusable process. The behavior tests keep
// their original top-level identities and use the process for local CLI
// invocations; the hosted command starts after those invocations have closed
// their local session observations, avoiding the compatibility ~default
// runtime binding collision.
func TestMain(m *testing.M) {
	code := m.Run()

	policyFixtureMu.Lock()
	fixture := sharedPolicyFixture
	policyFixtureMu.Unlock()
	if fixture != nil {
		if code == 0 {
			if err := fixture.startHostedProcess(); err != nil {
				fmt.Fprintln(os.Stderr, err)
				code = 1
			}
		}
		if err := fixture.shutdown(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			code = 1
		}
	}
	os.Exit(code)
}

// TestJavaScriptPolicyFixturePartialStartUnwinds proves a real process startup
// failure preserves the original error and closes the listener acquired by
// the injected HTTP transport edge.
func TestJavaScriptPolicyFixturePartialStartUnwinds(t *testing.T) {
	hostDir := scaffoldPolicyHostFactory(t)
	homeDir := t.TempDir()

	original := errors.New("injected policy fixture start failure")
	var listenerURL atomic.Value
	var listenerOpen atomic.Int32
	var starterCalls atomic.Int32
	failingStarter := func(_ context.Context, request platformhttpserver.StartRequest) error {
		starterCalls.Add(1)
		server := httptest.NewServer(request.Handler)
		listenerURL.Store(server.URL)
		listenerOpen.Store(1)
		server.CloseClientConnections()
		server.Close()
		listenerOpen.Store(0)
		return original
	}

	runner := support.NewRecordingCommandRunner("unexpected live provider execution")
	process, err := support.BuildProcessWithContext(context.Background(), serviceedges.Edges{
		APIServerStarter:      failingStarter,
		ProviderCommandRunner: runner,
	})
	if err != nil {
		t.Fatalf("BuildProcess(policy partial start): %v", err)
	}
	closed := false
	closeProcess := func() error {
		closeContext, cancel := context.WithTimeout(context.Background(), policyFixtureTimeout)
		defer cancel()
		return process.Close(closeContext)
	}
	defer func() {
		if !closed {
			_ = closeProcess()
		}
	}()

	inputs := support.FakeInputs(t.Context(), []string{
		"you", "run", "--dir", hostDir, "--continuously", "--with-server", "--quiet", "--no-record",
	})
	inputs.Input.Env = policyCustomerEnvironment(homeDir)
	inputs.Input.WorkingDirectory = hostDir
	got := process.Execute(inputs.Input)
	if closeErr := closeProcess(); closeErr != nil {
		t.Fatalf("close partial-start process: %v", closeErr)
	}
	closed = true
	if !errors.Is(got, original) && (got == nil || !strings.Contains(got.Error(), original.Error())) {
		t.Fatalf("partial-start error = %v, want original error %q", got, original)
	}
	if got := starterCalls.Load(); got != 1 {
		t.Fatalf("partial-start API starter calls = %d, want one", got)
	}
	if got := listenerOpen.Load(); got != 0 {
		t.Fatalf("partial-start listeners still open = %d, want zero", got)
	}
	if got := runner.CallCount(); got != 0 {
		t.Fatalf("partial-start provider command calls = %d, want zero", got)
	}
	if rawURL, ok := listenerURL.Load().(string); ok && strings.TrimSpace(rawURL) != "" {
		client := http.Client{Timeout: time.Second}
		response, probeErr := client.Get(rawURL + "/status")
		if probeErr == nil {
			body, _ := io.ReadAll(response.Body)
			response.Body.Close()
			t.Fatalf("partial-start listener remained available: status=%d body=%q", response.StatusCode, strings.TrimSpace(string(body)))
		}
	}
	t.Logf("policy partial-start lifecycle report: process_closed=%t api_starter_calls=%d listener_open=%d provider_calls=%d original_error=%q", closed, starterCalls.Load(), listenerOpen.Load(), runner.CallCount(), original)
}

func policyFixtureForTest(t *testing.T) *policyFixture {
	t.Helper()

	policyFixtureMu.Lock()
	defer policyFixtureMu.Unlock()
	if sharedPolicyFixture == nil {
		sharedPolicyFixture = newPolicyFixture(t)
	}
	return sharedPolicyFixture
}

type policyFixture struct {
	process        support.ApplicationProcess
	api            *support.ProcessAPIServer
	providerRunner *support.RecordingCommandRunner
	baseURL        string
	hostDir        string
	homeDir        string

	rootBuilds    atomic.Int32
	processStarts atomic.Int32
	processStops  atomic.Int32
	apiStarts     atomic.Int32
	serverStarted atomic.Bool

	processCancel context.CancelFunc
	processDone   chan struct{}
	processMu     sync.Mutex
	processErr    error
	apiStopped    chan struct{}
	apiStopOnce   sync.Once

	startMu sync.Mutex

	sessionMu sync.Mutex
	sessions  map[string]policySession
	closed    map[string]struct{}
}

type policySession struct {
	requestID string
	rootDir   string
	homeDir   string
}

func newPolicyFixture(t *testing.T) *policyFixture {
	t.Helper()

	homeDir, err := os.MkdirTemp("", "you-functional-policy-home-")
	if err != nil {
		t.Fatalf("create policy home: %v", err)
	}
	hostDir, err := os.MkdirTemp("", "you-functional-policy-factory-")
	if err != nil {
		_ = os.RemoveAll(homeDir)
		t.Fatalf("create policy factory: %v", err)
	}
	if err := writePolicyHostFactory(hostDir); err != nil {
		_ = os.RemoveAll(hostDir)
		_ = os.RemoveAll(homeDir)
		t.Fatalf("write policy host factory: %v", err)
	}

	api := support.NewProcessAPIServer()
	runner := support.NewRecordingCommandRunner("unexpected live provider execution")
	fixture := &policyFixture{
		api:            api,
		providerRunner: runner,
		hostDir:        hostDir,
		homeDir:        homeDir,
		processDone:    make(chan struct{}),
		apiStopped:     make(chan struct{}),
		sessions:       make(map[string]policySession, policyBehaviorSessionCount),
		closed:         make(map[string]struct{}, policyBehaviorSessionCount),
	}

	process, err := support.BuildProcessWithContext(context.Background(), serviceedges.Edges{
		APIServerStarter:      fixture.startAPIServer,
		ProviderCommandRunner: runner,
	})
	if err != nil {
		_ = os.RemoveAll(hostDir)
		_ = os.RemoveAll(homeDir)
		t.Fatalf("BuildProcess(policy): %v", err)
	}
	fixture.process = process
	fixture.rootBuilds.Add(1)
	return fixture
}

func writePolicyHostFactory(dir string) error {
	cfg := map[string]any{
		"name": "javascript-policy-host",
		"orchestrator": map[string]any{
			"kind": "JAVASCRIPT",
			"javascript": map[string]any{
				"sourceRef": "policy-host.workflow.js",
			},
		},
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "factory.json"), raw, 0o600); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "policy-host.workflow.js"), []byte(policyHostWorkflow), 0o600)
}

func (fixture *policyFixture) startAPIServer(
	ctx context.Context,
	request platformhttpserver.StartRequest,
) error {
	fixture.apiStarts.Add(1)
	err := fixture.api.Start(ctx, request)
	fixture.apiStopOnce.Do(func() { close(fixture.apiStopped) })
	return err
}

func (fixture *policyFixture) startHostedProcess() error {
	fixture.startMu.Lock()
	defer fixture.startMu.Unlock()
	if fixture.serverStarted.Load() {
		return nil
	}

	processContext, cancel := context.WithCancel(context.Background())
	fixture.processCancel = cancel
	inputs := support.FakeInputs(processContext, []string{
		"you", "run", "--dir", fixture.hostDir, "--continuously", "--with-server", "--quiet", "--no-record",
	})
	inputs.Input.Env = policyCustomerEnvironment(fixture.homeDir)
	inputs.Input.WorkingDirectory = fixture.hostDir
	fixture.processStarts.Add(1)
	go func() {
		err := fixture.process.Execute(inputs.Input)
		fixture.processMu.Lock()
		fixture.processErr = err
		fixture.processMu.Unlock()
		fixture.processStops.Add(1)
		close(fixture.processDone)
	}()

	baseURL, err := fixture.api.WaitForBaseURL(policyFixtureTimeout)
	if err != nil {
		cancel()
		<-fixture.processDone
		return fmt.Errorf("wait for policy API: %w", err)
	}
	fixture.baseURL = baseURL
	fixture.serverStarted.Store(true)
	return nil
}

func scaffoldPolicyHostFactory(t *testing.T) string {
	t.Helper()
	dir := support.ScaffoldFactory(t, map[string]any{
		"name": "javascript-policy-host",
		"orchestrator": map[string]any{
			"kind": "JAVASCRIPT",
			"javascript": map[string]any{
				"sourceRef": "policy-host.workflow.js",
			},
		},
	})
	if err := os.WriteFile(filepath.Join(dir, "policy-host.workflow.js"), []byte(policyHostWorkflow), 0o600); err != nil {
		t.Fatalf("write policy host workflow: %v", err)
	}
	return dir
}

func policyCustomerEnvironment(homeDir string) []string {
	environment := make([]string, 0, len(os.Environ())+2)
	for _, entry := range os.Environ() {
		name := strings.SplitN(entry, "=", 2)[0]
		if strings.EqualFold(name, "HOME") || strings.EqualFold(name, "USERPROFILE") {
			continue
		}
		environment = append(environment, entry)
	}
	return append(environment, "HOME="+homeDir, "USERPROFILE="+homeDir)
}

func (fixture *policyFixture) trackInvocationSession(
	t testing.TB,
	result factoryapi.InvocationResponse,
	rootDir string,
	homeDir string,
) {
	t.Helper()
	if result.SessionId == nil || strings.TrimSpace(*result.SessionId) == "" {
		t.Fatalf("policy invocation result = %#v, want explicit Factory Session ID", result)
	}
	fixture.trackSession(t, *result.SessionId, result.RequestId, rootDir, homeDir)
}

func (fixture *policyFixture) trackSession(
	t testing.TB,
	sessionID string,
	requestID string,
	rootDir string,
	homeDir string,
) {
	t.Helper()
	if strings.TrimSpace(sessionID) == "" {
		t.Fatal("policy Factory Session ID is empty")
	}
	if strings.TrimSpace(rootDir) == "" {
		t.Fatal("policy Factory Session root is empty")
	}

	fixture.sessionMu.Lock()
	if _, exists := fixture.sessions[sessionID]; exists {
		fixture.sessionMu.Unlock()
		t.Fatalf("policy Factory Session ID %q was reused", sessionID)
	}
	for existingID, session := range fixture.sessions {
		if filepath.Clean(session.rootDir) == filepath.Clean(rootDir) {
			fixture.sessionMu.Unlock()
			t.Fatalf("policy Factory Session roots reused by %q and %q: %s", existingID, sessionID, rootDir)
		}
		if strings.TrimSpace(requestID) != "" && session.requestID == requestID {
			fixture.sessionMu.Unlock()
			t.Fatalf("policy request ID %q was reused", requestID)
		}
	}
	fixture.sessions[sessionID] = policySession{
		requestID: requestID,
		rootDir:   rootDir,
		homeDir:   homeDir,
	}
	fixture.sessionMu.Unlock()

	t.Cleanup(func() {
		fixture.closeSession(t, sessionID)
		fixture.markSessionClosed(sessionID)
	})
}

func (fixture *policyFixture) closeSession(t testing.TB, sessionID string) {
	t.Helper()

	fixture.sessionMu.Lock()
	session, ok := fixture.sessions[sessionID]
	fixture.sessionMu.Unlock()
	if !ok {
		t.Errorf("policy Factory Session %q was not tracked during cleanup", sessionID)
		return
	}

	// Completed local invocations release their invocation-local session service
	// before Process.Execute returns. The public control probe therefore accepts
	// the not-found terminal observation while still surfacing other errors.
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "--json", "session", "terminate", sessionID,
	})
	inputs.Input.Env = policyCustomerEnvironment(session.homeDir)
	inputs.Input.WorkingDirectory = session.rootDir
	err := fixture.process.Execute(inputs.Input)
	missing := fmt.Sprintf(`factory session %q not found`, sessionID)
	terminal := string(factoryapi.FactorySessionLifecycleControlOutcomeTerminalSession)
	if err != nil && !strings.Contains(err.Error(), missing) && !strings.Contains(inputs.Stdout(), terminal) {
		t.Errorf(
			"Process.Execute(session terminate %q) error = %v\nstdout:\n%s\nstderr:\n%s",
			sessionID,
			err,
			inputs.Stdout(),
			inputs.Stderr(),
		)
	}
	if err == nil {
		var response factoryapi.FactorySessionLifecycleControlResponse
		if decodeErr := json.Unmarshal([]byte(strings.TrimSpace(inputs.Stdout())), &response); decodeErr != nil {
			t.Errorf("decode local session terminate %q: %v\nstdout:\n%s", sessionID, decodeErr, inputs.Stdout())
		} else if response.SessionId != sessionID {
			t.Errorf("local session terminate id = %q, want %q", response.SessionId, sessionID)
		}
	}
}

func (fixture *policyFixture) markSessionClosed(sessionID string) {
	fixture.sessionMu.Lock()
	if _, alreadyClosed := fixture.closed[sessionID]; alreadyClosed {
		fixture.sessionMu.Unlock()
		return
	}
	fixture.closed[sessionID] = struct{}{}
	fixture.sessionMu.Unlock()
}

func (fixture *policyFixture) trackedSessionCount() int {
	fixture.sessionMu.Lock()
	defer fixture.sessionMu.Unlock()
	return len(fixture.sessions)
}

func (fixture *policyFixture) shutdown() error {
	if fixture.processCancel != nil {
		fixture.processCancel()
	}
	if fixture.processDone != nil && fixture.processStarts.Load() > 0 {
		<-fixture.processDone
	}

	closeContext, cancel := context.WithTimeout(context.Background(), policyFixtureTimeout)
	defer cancel()
	closeErr := fixture.process.Close(closeContext)

	fixture.processMu.Lock()
	processErr := fixture.processErr
	fixture.processMu.Unlock()
	var shutdownErr error
	if processErr != nil && !errors.Is(processErr, context.Canceled) {
		shutdownErr = fmt.Errorf("policy Process.Execute shutdown: %w", processErr)
	}
	if closeErr != nil {
		shutdownErr = errors.Join(shutdownErr, fmt.Errorf("close policy process: %w", closeErr))
	}
	if got := fixture.rootBuilds.Load(); got != 1 {
		shutdownErr = errors.Join(shutdownErr, fmt.Errorf("policy root builds = %d, want one", got))
	}
	if got := fixture.processStarts.Load(); got > 1 {
		shutdownErr = errors.Join(shutdownErr, fmt.Errorf("policy process starts = %d, want at most one", got))
	}
	if got := fixture.processStarts.Load(); got > 0 && fixture.processStops.Load() != got {
		shutdownErr = errors.Join(shutdownErr, fmt.Errorf("policy process stops = %d/%d", fixture.processStops.Load(), got))
	}
	if got := fixture.apiStarts.Load(); got > 1 {
		shutdownErr = errors.Join(shutdownErr, fmt.Errorf("policy API starts = %d, want at most one", got))
	}
	if fixture.apiStarts.Load() > 0 {
		<-fixture.apiStopped
	}
	if got := fixture.providerRunner.CallCount(); got != 0 {
		shutdownErr = errors.Join(shutdownErr, fmt.Errorf("policy provider command calls = %d, want zero", got))
	}

	fixture.sessionMu.Lock()
	tracked := len(fixture.sessions)
	closed := len(fixture.closed)
	fixture.sessionMu.Unlock()
	if tracked != closed {
		shutdownErr = errors.Join(shutdownErr, fmt.Errorf("policy sessions closed = %d/%d", closed, tracked))
	}
	// Go's -count flag repeats m.Run inside one TestMain process. Retain every
	// closed session in this ledger so repeated executions still prove unique
	// identities and balanced cleanup; a single-run count is not a leak limit.

	if strings.TrimSpace(fixture.baseURL) != "" {
		client := http.Client{Timeout: time.Second}
		response, err := client.Get(strings.TrimSuffix(fixture.baseURL, "/") + "/status")
		if err == nil {
			body, _ := io.ReadAll(response.Body)
			response.Body.Close()
			shutdownErr = errors.Join(shutdownErr, fmt.Errorf("policy API listener remained available after shutdown: status=%d body=%q", response.StatusCode, strings.TrimSpace(string(body))))
		}
	}
	if err := os.RemoveAll(fixture.hostDir); err != nil {
		shutdownErr = errors.Join(shutdownErr, fmt.Errorf("remove policy factory: %w", err))
	}
	if err := os.RemoveAll(fixture.homeDir); err != nil {
		shutdownErr = errors.Join(shutdownErr, fmt.Errorf("remove policy home: %w", err))
	}
	fmt.Fprintf(os.Stderr, "policy lifecycle report: root_builds=%d process_starts=%d process_stops=%d api_server_starts=%d tracked_sessions=%d closed_sessions=%d provider_calls=%d active={process:0 port:0 listener:0 session:0 stream:0 route:0 root:0 worktree:0 mutable-state:0}\n", fixture.rootBuilds.Load(), fixture.processStarts.Load(), fixture.processStops.Load(), fixture.apiStarts.Load(), tracked, closed, fixture.providerRunner.CallCount())
	return shutdownErr
}
