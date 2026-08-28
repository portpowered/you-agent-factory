package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
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
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const sharedProviderFixtureTimeout = 15 * time.Second

const (
	sharedMockAgentAcceptWorkID   = "shared-mock-agent-accept"
	sharedMockAgentRejectWorkID   = "shared-mock-agent-reject"
	sharedMockScriptAcceptWorkID  = "shared-mock-script-accept"
	sharedMockScriptRejectWorkID  = "shared-mock-script-reject"
	sharedMockServiceModelWorkID  = "shared-mock-service-model"
	sharedMockServiceScriptWorkID = "shared-mock-service-script"
)

var sharedProviderRouteSequence atomic.Uint64

// sharedProviderHTTPServer observes the one listener owned by the shared
// root-built process. The count is local fixture evidence, not process-global
// state.
type sharedProviderHTTPServer struct {
	server *support.ProcessAPIServer

	mu       sync.Mutex
	starts   int
	done     chan struct{}
	doneOnce sync.Once
}

func newSharedProviderHTTPServer() *sharedProviderHTTPServer {
	return &sharedProviderHTTPServer{
		server: support.NewProcessAPIServer(),
		done:   make(chan struct{}),
	}
}

func (server *sharedProviderHTTPServer) start(
	ctx context.Context,
	request platformhttpserver.StartRequest,
) error {
	server.mu.Lock()
	server.starts++
	server.mu.Unlock()
	defer server.doneOnce.Do(func() { close(server.done) })
	return server.server.Start(ctx, request)
}

func (server *sharedProviderHTTPServer) startCount() int {
	server.mu.Lock()
	defer server.mu.Unlock()
	return server.starts
}

func (server *sharedProviderHTTPServer) waitClosed(ctx context.Context) error {
	select {
	case <-server.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

type sharedProviderProcessConstructor struct {
	mu     sync.Mutex
	builds int
}

func (constructor *sharedProviderProcessConstructor) build(
	edges serviceedges.Edges,
) (support.ApplicationProcess, error) {
	process, err := support.BuildProcessWithContext(context.Background(), edges)
	if err != nil {
		return nil, err
	}
	constructor.mu.Lock()
	constructor.builds++
	constructor.mu.Unlock()
	return process, nil
}

func (constructor *sharedProviderProcessConstructor) count() int {
	constructor.mu.Lock()
	defer constructor.mu.Unlock()
	return constructor.builds
}

type sharedProviderProcessFixture struct {
	rootDir     string
	bootstrap   string
	homeDir     string
	runtimeLogs string
	baseURL     string

	process     support.ApplicationProcess
	cancel      context.CancelFunc
	done        chan struct{}
	executeErr  error
	api         *sharedProviderHTTPServer
	router      *sharedProviderCommandRouter
	constructor *sharedProviderProcessConstructor

	sessionMu         sync.Mutex
	openedSessionIDs  []string
	deletedSessionIDs []string
}

var sharedProviderGlobal struct {
	mu      sync.Mutex
	fixture *sharedProviderProcessFixture
	initErr error
}

func TestMain(m *testing.M) {
	code := m.Run()

	sharedProviderGlobal.mu.Lock()
	fixture := sharedProviderGlobal.fixture
	sharedProviderGlobal.mu.Unlock()
	if fixture != nil {
		if err := fixture.close(); err != nil {
			fmt.Fprintf(os.Stderr, "close shared provider fixture: %v\n", err)
			if code == 0 {
				code = 1
			}
		}
	}
	os.Exit(code)
}

func sharedProviderFixtureFor(t *testing.T) *sharedProviderProcessFixture {
	t.Helper()

	sharedProviderGlobal.mu.Lock()
	fixture := sharedProviderGlobal.fixture
	if fixture == nil && sharedProviderGlobal.initErr == nil {
		fixture, sharedProviderGlobal.initErr = newSharedProviderProcessFixture(t)
		if fixture != nil {
			sharedProviderGlobal.fixture = fixture
		}
	}
	err := sharedProviderGlobal.initErr
	sharedProviderGlobal.mu.Unlock()
	if err != nil {
		t.Fatalf("build shared provider fixture: %v", err)
	}
	if fixture == nil {
		t.Fatal("build shared provider fixture returned nil")
	}

	support.WaitForStatus(t, fixture.baseURL, sharedProviderFixtureTimeout, func(status factoryapi.StatusResponse) bool {
		return strings.TrimSpace(status.RuntimeStatus) != ""
	})
	return fixture
}

func newSharedProviderProcessFixture(t *testing.T) (*sharedProviderProcessFixture, error) {
	rootDir, err := os.MkdirTemp("", "you-functional-providers-shared-")
	if err != nil {
		return nil, fmt.Errorf("create fixture root: %w", err)
	}
	cleanupRoot := true
	defer func() {
		if cleanupRoot {
			_ = os.RemoveAll(rootDir)
		}
	}()

	sourceDir := support.LegacyFixtureDir(t, "script_executor_dir")
	bootstrap := filepath.Join(rootDir, "bootstrap")
	if err := copySharedProviderDirectory(sourceDir, bootstrap); err != nil {
		return nil, fmt.Errorf("copy bootstrap Factory: %w", err)
	}
	if err := os.RemoveAll(filepath.Join(bootstrap, "inputs")); err != nil {
		return nil, fmt.Errorf("clear bootstrap inputs: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(bootstrap, "inputs"), 0o755); err != nil {
		return nil, fmt.Errorf("create bootstrap inputs: %w", err)
	}

	homeDir := filepath.Join(rootDir, "home")
	runtimeLogs := filepath.Join(rootDir, "runtime-logs")
	for _, path := range []string{homeDir, runtimeLogs} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			return nil, fmt.Errorf("create shared provider path %q: %w", path, err)
		}
	}
	mockConfigPath := filepath.Join(rootDir, "mock-workers.json")
	if err := writeSharedMockWorkersConfig(mockConfigPath); err != nil {
		return nil, err
	}

	api := newSharedProviderHTTPServer()
	router := newSharedProviderCommandRouter()
	constructor := &sharedProviderProcessConstructor{}
	process, err := constructor.build(serviceedges.Edges{
		APIServerStarter:      api.start,
		ProviderCommandRunner: router,
		ScriptCommandRunner:   router,
	})
	if err != nil {
		return nil, fmt.Errorf("construct root process: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	inputs := support.FakeInputs(ctx, []string{
		"you", "run",
		"--dir", bootstrap,
		"--continuously",
		"--with-server",
		"--quiet",
		"--no-record",
		"--with-mock-workers", mockConfigPath,
		"--runtime-log-dir", runtimeLogs,
	})
	inputs.Input.Env = []string{"HOME=" + homeDir, "USERPROFILE=" + homeDir}
	inputs.Input.WorkingDirectory = bootstrap

	fixture := &sharedProviderProcessFixture{
		rootDir: rootDir, bootstrap: bootstrap, homeDir: homeDir,
		runtimeLogs: runtimeLogs, process: process, cancel: cancel,
		done: make(chan struct{}), api: api, router: router,
		constructor: constructor,
	}
	go func() {
		fixture.executeErr = process.Execute(inputs.Input)
		close(fixture.done)
	}()

	baseURL, err := api.server.WaitForBaseURL(sharedProviderFixtureTimeout)
	if err != nil {
		_ = fixture.close()
		return nil, err
	}
	fixture.baseURL = baseURL
	cleanupRoot = false
	return fixture, nil
}

func copySharedProviderDirectory(source, destination string) error {
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}

func writeSharedMockWorkersConfig(path string) error {
	exitSeven := 7
	exitNine := 9
	config := workers.MockWorkersConfig{
		UnmatchedDispatchPolicy: workers.MockWorkerUnmatchedDispatchPolicyPassthrough,
		MockWorkers: []workers.MockWorkerConfig{
			sharedMockEntry("worker", "process", sharedMockAgentRejectWorkID, workers.MockWorkerRunTypeReject, &workers.MockWorkerRejectConfig{
				Stdout: "configured stdout", Stderr: "configured stderr", ExitCode: &exitSeven,
			}),
			sharedMockEntry("worker", "process", sharedMockAgentAcceptWorkID, workers.MockWorkerRunTypeAccept, nil),
			sharedMockEntry("script-worker", "run-script", sharedMockScriptRejectWorkID, workers.MockWorkerRunTypeReject, &workers.MockWorkerRejectConfig{
				Stdout: "script configured stdout", Stderr: "script configured stderr", ExitCode: &exitNine,
			}),
			sharedMockEntry("script-worker", "run-script", sharedMockScriptAcceptWorkID, workers.MockWorkerRunTypeAccept, nil),
			sharedMockEntry("worker", "process", sharedMockServiceModelWorkID, workers.MockWorkerRunTypeAccept, nil),
			sharedMockEntry("script-worker", "run-script", sharedMockServiceScriptWorkID, workers.MockWorkerRunTypeAccept, nil),
		},
	}
	payload, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("marshal shared mock-worker config: %w", err)
	}
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		return fmt.Errorf("write shared mock-worker config: %w", err)
	}
	return nil
}

func sharedMockEntry(
	workerName, workstationName, workID string,
	runType workers.MockWorkerRunType,
	reject *workers.MockWorkerRejectConfig,
) workers.MockWorkerConfig {
	return workers.MockWorkerConfig{
		WorkerName: workerName, WorkstationName: workstationName,
		WorkInputs: []workers.MockWorkInputSelector{{WorkID: workID}},
		RunType:    runType, RejectConfig: reject,
	}
}

func (fixture *sharedProviderProcessFixture) close() error {
	if fixture == nil {
		return nil
	}
	fixture.cancel()
	var closeErr error
	select {
	case <-fixture.done:
		if fixture.executeErr != nil && !strings.Contains(fixture.executeErr.Error(), "context canceled") {
			closeErr = fmt.Errorf("Process.Execute: %w", fixture.executeErr)
		}
	case <-time.After(sharedProviderFixtureTimeout):
		closeErr = fmt.Errorf("timed out waiting for Process.Execute shutdown")
	}

	closeCtx, cancel := context.WithTimeout(context.Background(), sharedProviderFixtureTimeout)
	defer cancel()
	if err := fixture.process.Close(closeCtx); err != nil && closeErr == nil {
		closeErr = fmt.Errorf("close application process: %w", err)
	}
	if err := fixture.api.waitClosed(closeCtx); err != nil && closeErr == nil {
		closeErr = fmt.Errorf("wait for API server shutdown: %w", err)
	}
	if got := fixture.constructor.count(); got != 1 && closeErr == nil {
		closeErr = fmt.Errorf("shared provider root constructions after cleanup = %d, want one", got)
	}
	if got := fixture.api.startCount(); got != 1 && closeErr == nil {
		closeErr = fmt.Errorf("shared provider listener starts after cleanup = %d, want one", got)
	}
	if got := fixture.router.routeCount(); got != 0 && closeErr == nil {
		closeErr = fmt.Errorf("shared provider routes after cleanup = %d, want zero", got)
	}
	if err := fixture.validateSessionTopology(); err != nil && closeErr == nil {
		closeErr = err
	}
	if err := os.RemoveAll(fixture.rootDir); err != nil && closeErr == nil {
		closeErr = fmt.Errorf("remove fixture root: %w", err)
	}
	if _, err := os.Stat(fixture.rootDir); !os.IsNotExist(err) && closeErr == nil {
		closeErr = fmt.Errorf("shared provider root %q remains after cleanup; stat error: %v", fixture.rootDir, err)
	}
	return closeErr
}

type sharedProviderCommandRoute struct {
	selector string
	workDir  string
	runner   platformprocess.CommandRunner
}

// sharedProviderCommandRouter routes only by the immutable resolved WorkDir
// supplied to the command-runner edge. It never selects a result by session
// order or mutable global invocation state.
type sharedProviderCommandRouter struct {
	mu       sync.Mutex
	routes   map[string]sharedProviderCommandRoute
	requests []platformprocess.CommandRequest
}

func newSharedProviderCommandRouter() *sharedProviderCommandRouter {
	return &sharedProviderCommandRouter{routes: make(map[string]sharedProviderCommandRoute)}
}

func (router *sharedProviderCommandRouter) register(
	selector, workDir string,
	runner platformprocess.CommandRunner,
) error {
	selector = strings.TrimSpace(selector)
	workDir = filepath.Clean(strings.TrimSpace(workDir))
	if selector == "" || workDir == "." || runner == nil {
		return fmt.Errorf("provider route selector, WorkDir, and runner are required")
	}
	router.mu.Lock()
	defer router.mu.Unlock()
	if _, exists := router.routes[workDir]; exists {
		return fmt.Errorf("provider WorkDir route %q is already registered", workDir)
	}
	for _, route := range router.routes {
		if route.selector == selector {
			return fmt.Errorf("provider route selector %q is already registered", selector)
		}
	}
	router.routes[workDir] = sharedProviderCommandRoute{
		selector: selector, workDir: workDir, runner: runner,
	}
	return nil
}

func (router *sharedProviderCommandRouter) unregister(selector string) error {
	selector = strings.TrimSpace(selector)
	router.mu.Lock()
	defer router.mu.Unlock()
	for workDir, route := range router.routes {
		if route.selector != selector {
			continue
		}
		delete(router.routes, workDir)
		return nil
	}
	return fmt.Errorf("provider route selector %q is not registered", selector)
}

func (router *sharedProviderCommandRouter) Run(
	ctx context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	workDir := filepath.Clean(strings.TrimSpace(request.WorkDir))
	router.mu.Lock()
	route, ok := router.routes[workDir]
	if ok {
		router.requests = append(router.requests, cloneSharedProviderCommandRequest(request))
	}
	router.mu.Unlock()
	if !ok {
		return platformprocess.CommandResult{}, fmt.Errorf("no provider route matched WorkDir %q", request.WorkDir)
	}
	return route.runner.Run(ctx, request)
}

func (router *sharedProviderCommandRouter) routeCount() int {
	router.mu.Lock()
	defer router.mu.Unlock()
	return len(router.routes)
}

func (router *sharedProviderCommandRouter) callCount() int {
	router.mu.Lock()
	defer router.mu.Unlock()
	return len(router.requests)
}

func cloneSharedProviderCommandRequest(
	request platformprocess.CommandRequest,
) platformprocess.CommandRequest {
	request.Args = append([]string(nil), request.Args...)
	request.Stdin = append([]byte(nil), request.Stdin...)
	request.Env = append([]string(nil), request.Env...)
	return request
}

type sharedProviderScenario struct {
	fixture     *sharedProviderProcessFixture
	factoryDir  string
	sessionID   string
	routeSelect string
	closeOnce   sync.Once
}

func (fixture *sharedProviderProcessFixture) openScenario(
	t *testing.T,
	factoryDir, workDir string,
	runner platformprocess.CommandRunner,
) *sharedProviderScenario {
	t.Helper()

	routeSelector := ""
	var scenario *sharedProviderScenario
	if runner != nil {
		routeSelector = fmt.Sprintf("providers-shared-route-%d", sharedProviderRouteSequence.Add(1))
		if err := fixture.router.register(routeSelector, workDir, runner); err != nil {
			t.Fatalf("register shared provider route: %v", err)
		}
		t.Cleanup(func() {
			if scenario != nil {
				return
			}
			if err := fixture.router.unregister(routeSelector); err != nil {
				t.Errorf("unregister shared provider route %q after session-open failure: %v", routeSelector, err)
			}
		})
	}
	opened := support.OpenFactorySessionAt(t, fixture.baseURL, factoryDir)
	if opened.Session == nil || strings.TrimSpace(opened.Session.Id) == "" {
		t.Fatalf("shared Factory Session for %q = %#v, want identity", factoryDir, opened)
	}
	scenario = &sharedProviderScenario{
		fixture: fixture, factoryDir: factoryDir, sessionID: opened.Session.Id,
		routeSelect: routeSelector,
	}
	fixture.sessionMu.Lock()
	fixture.openedSessionIDs = append(fixture.openedSessionIDs, scenario.sessionID)
	fixture.sessionMu.Unlock()
	t.Cleanup(func() { scenario.close(t) })
	return scenario
}

func (scenario *sharedProviderScenario) close(t testing.TB) {
	t.Helper()
	scenario.closeOnce.Do(func() {
		support.CloseFactorySessionAt(t, scenario.fixture.baseURL, scenario.sessionID)
		scenario.fixture.markSessionDeleted(scenario.sessionID)
		assertSharedFactorySessionDeleted(t, scenario.fixture.baseURL, scenario.sessionID)
		if scenario.routeSelect != "" {
			if err := scenario.fixture.router.unregister(scenario.routeSelect); err != nil {
				t.Errorf("unregister shared provider route %q: %v", scenario.routeSelect, err)
			}
		}
		if err := os.RemoveAll(scenario.factoryDir); err != nil {
			t.Errorf("remove shared scenario Factory %q: %v", scenario.factoryDir, err)
		} else if _, err := os.Stat(scenario.factoryDir); !os.IsNotExist(err) {
			t.Errorf("shared scenario Factory %q remains after cleanup; stat error: %v", scenario.factoryDir, err)
		}
	})
}

func (fixture *sharedProviderProcessFixture) markSessionDeleted(sessionID string) {
	fixture.sessionMu.Lock()
	defer fixture.sessionMu.Unlock()
	for _, existing := range fixture.deletedSessionIDs {
		if existing == sessionID {
			return
		}
	}
	fixture.deletedSessionIDs = append(fixture.deletedSessionIDs, sessionID)
}

func (scenario *sharedProviderScenario) waitForTerminal(t testing.TB, timeout time.Duration) {
	t.Helper()
	support.WaitForSessionTerminalStatus(t, scenario.fixture.baseURL, scenario.sessionID, timeout)
}

func (scenario *sharedProviderScenario) listWork(t testing.TB) factoryapi.ListWorkResponse {
	t.Helper()
	endpoint := strings.TrimSuffix(scenario.fixture.baseURL, "/") +
		"/factory-sessions/" + url.PathEscape(scenario.sessionID) + "/work"
	return support.GetJSON[factoryapi.ListWorkResponse](t, endpoint)
}

func (scenario *sharedProviderScenario) factoryEvents(t testing.TB) []factoryapi.FactoryEvent {
	t.Helper()
	return support.GetFactoryEventsForSessionAt(t, scenario.fixture.baseURL, scenario.sessionID)
}

func (scenario *sharedProviderScenario) stop(t *testing.T) {
	t.Helper()
	scenario.close(t)
}

func runSharedProviderFactory(
	t *testing.T,
	dir, workDir string,
	runner platformprocess.CommandRunner,
	timeout time.Duration,
) (*sharedProviderScenario, factoryapi.ListWorkResponse) {
	t.Helper()
	fixture := sharedProviderFixtureFor(t)
	scenario := fixture.openScenario(t, dir, workDir, runner)
	scenario.waitForTerminal(t, timeout)
	return scenario, scenario.listWork(t)
}

func runSharedMockFactory(
	t *testing.T,
	dir string,
	timeout time.Duration,
) (*sharedProviderScenario, factoryapi.ListWorkResponse) {
	t.Helper()
	fixture := sharedProviderFixtureFor(t)
	scenario := fixture.openScenario(t, dir, "", nil)
	scenario.waitForTerminal(t, timeout)
	return scenario, scenario.listWork(t)
}

func assertSharedFactorySessionDeleted(t testing.TB, baseURL, sessionID string) {
	t.Helper()
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID)
	response, err := http.Get(endpoint)
	if err != nil {
		t.Fatalf("GET deleted shared Factory Session %q: %v", sessionID, err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("GET deleted shared Factory Session %q status = %d, want 404: %s", sessionID, response.StatusCode, strings.TrimSpace(string(body)))
	}
}

func (fixture *sharedProviderProcessFixture) assertTopology(t testing.TB) {
	t.Helper()
	if got := fixture.constructor.count(); got != 1 {
		t.Fatalf("shared provider root constructions = %d, want one", got)
	}
	if got := fixture.api.startCount(); got != 1 {
		t.Fatalf("shared provider listener starts = %d, want one", got)
	}
}

func (fixture *sharedProviderProcessFixture) assertSessionTopology(t testing.TB) {
	t.Helper()
	if err := fixture.validateSessionTopology(); err != nil {
		t.Fatal(err)
	}
}

func (fixture *sharedProviderProcessFixture) validateSessionTopology() error {
	fixture.sessionMu.Lock()
	opened := append([]string(nil), fixture.openedSessionIDs...)
	deleted := append([]string(nil), fixture.deletedSessionIDs...)
	fixture.sessionMu.Unlock()
	if len(opened) != len(deleted) {
		return fmt.Errorf("shared Factory Session topology = opened:%d deleted:%d, want equal", len(opened), len(opened))
	}
	seen := make(map[string]struct{}, len(opened))
	for _, sessionID := range opened {
		if _, exists := seen[sessionID]; exists {
			return fmt.Errorf("shared Factory Session ID %q was reused", sessionID)
		}
		seen[sessionID] = struct{}{}
	}
	for _, sessionID := range deleted {
		if _, exists := seen[sessionID]; !exists {
			return fmt.Errorf("deleted shared Factory Session ID %q was not opened by this fixture", sessionID)
		}
	}
	return nil
}

func (fixture *sharedProviderProcessFixture) runtimeLogDir() string {
	return fixture.runtimeLogs
}

func findSharedRuntimeLogRecord(
	t *testing.T,
	fixture *sharedProviderProcessFixture,
	workDir string,
	exitCode int,
) map[string]any {
	t.Helper()
	var found map[string]any
	err := filepath.WalkDir(fixture.runtimeLogs, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".log" {
			return nil
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()
		decoder := json.NewDecoder(file)
		for {
			var record map[string]any
			if err := decoder.Decode(&record); err != nil {
				if err == io.EOF {
					break
				}
				return err
			}
			if record["event_name"] != commandRunnerCompletedLogEvent {
				continue
			}
			if got, ok := record["working_dir"].(string); ok && workDir != "" && got != workDir {
				continue
			}
			if got, ok := record["exit_code"].(float64); !ok || int(got) != exitCode {
				continue
			}
			found = record
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan shared runtime logs: %v", err)
	}
	if found == nil {
		t.Fatalf("shared runtime logs contain no %s record for %q exit %d", commandRunnerCompletedLogEvent, workDir, exitCode)
	}
	return found
}

func TestProvidersSharedProcessTopology(t *testing.T) {
	fixture := sharedProviderFixtureFor(t)
	fixture.assertTopology(t)

	firstDir := testutilCopySharedFixture(t, "script_executor_dir")
	secondDir := testutilCopySharedFixture(t, "script_executor_dir")
	testutil.WriteSeedFile(t, firstDir, "task", []byte("first shared payload"))
	testutil.WriteSeedFile(t, secondDir, "task", []byte("second shared payload"))
	first := fixture.openScenario(t, firstDir, firstDir, support.NewStaticSuccessCommandRunner("first-shared-output"))
	second := fixture.openScenario(t, secondDir, secondDir, support.NewStaticSuccessCommandRunner("second-shared-output"))
	if got := fixture.router.routeCount(); got != 2 {
		t.Fatalf("shared provider active routes = %d, want two", got)
	}

	var wait sync.WaitGroup
	wait.Add(2)
	go func() {
		defer wait.Done()
		first.waitForTerminal(t, sharedProviderFixtureTimeout)
	}()
	go func() {
		defer wait.Done()
		second.waitForTerminal(t, sharedProviderFixtureTimeout)
	}()
	wait.Wait()
	firstWork := first.listWork(t)
	secondWork := second.listWork(t)
	assertSessionPlaces(t, firstWork, map[string]int{"task:done": 1, "task:init": 0})
	assertSessionPlaces(t, secondWork, map[string]int{"task:done": 1, "task:init": 0})
	assertDispatchOutput(t, first.factoryEvents(t), "first-shared-output")
	assertDispatchOutput(t, second.factoryEvents(t), "second-shared-output")
	first.stop(t)
	second.stop(t)
	if got := fixture.router.routeCount(); got != 0 {
		t.Fatalf("shared provider routes after cleanup = %d, want zero", got)
	}
	fixture.assertSessionTopology(t)
}

func TestProvidersSharedProcessRoutes(t *testing.T) {
	fixture := sharedProviderFixtureFor(t)
	fixture.assertTopology(t)
	baselineCalls := fixture.router.callCount()

	workDir := filepath.Join(fixture.rootDir, "route-test-workdir")
	routeSelector := fmt.Sprintf("providers-shared-route-test-%d", sharedProviderRouteSequence.Add(1))
	runner := support.NewStaticSuccessCommandRunner("registered-route-output")
	if err := fixture.router.register(routeSelector, workDir, runner); err != nil {
		t.Fatalf("register shared provider test route: %v", err)
	}
	defer func() {
		if err := fixture.router.unregister(routeSelector); err != nil {
			t.Errorf("unregister shared provider test route: %v", err)
		}
	}()

	if err := fixture.router.register(routeSelector, workDir, support.NewStaticSuccessCommandRunner("duplicate-output")); err == nil {
		t.Fatal("duplicate shared provider route registration succeeded")
	}
	if got := fixture.router.routeCount(); got != 1 {
		t.Fatalf("shared provider route count after duplicate registration = %d, want one", got)
	}

	result, err := fixture.router.Run(context.Background(), platformprocess.CommandRequest{
		Command: "echo", WorkDir: workDir, Stdin: []byte("registered route input"),
	})
	if err != nil {
		t.Fatalf("registered shared provider route: %v", err)
	}
	if string(result.Stdout) != "registered-route-output" {
		t.Fatalf("registered shared provider route output = %q, want registered output", result.Stdout)
	}
	if got := fixture.router.callCount(); got != baselineCalls+1 {
		t.Fatalf("shared provider route calls after registered route = %d, want %d", got, baselineCalls+1)
	}

	unknownWorkDir := filepath.Join(fixture.rootDir, "unknown-route-workdir")
	_, err = fixture.router.Run(context.Background(), platformprocess.CommandRequest{
		Command: "echo", WorkDir: unknownWorkDir, Stdin: []byte("must not cross a route"),
	})
	if err == nil || !strings.Contains(err.Error(), "no provider route matched WorkDir") {
		t.Fatalf("unknown shared provider route error = %v, want explicit route failure", err)
	}
	if got := fixture.router.callCount(); got != baselineCalls+1 {
		t.Fatalf("shared provider route calls after unknown route = %d, want %d", got, baselineCalls+1)
	}
}

func TestProvidersSharedProcessAdverseRecovery(t *testing.T) {
	fixture := sharedProviderFixtureFor(t)

	t.Run("invalid_template", func(t *testing.T) {
		dir := testutilCopySharedFixture(t, "script_executor_dir")
		writeFixtureFile(t, dir, []string{"workstations", "run-script", "AGENTS.md"}, "---\ntype: MODEL_WORKSTATION\n---\n{{")
		testutil.WriteSeedFile(t, dir, "task", []byte("invalid-template-payload"))
		runner := &captureCommandRunner{}
		scenario, listed := runSharedProviderFactory(t, dir, dir, runner, 5*time.Second)
		assertSessionPlaces(t, listed, map[string]int{"task:failed": 1, "task:init": 0, "task:done": 0})
		if got := runner.CallCount(); got != 0 {
			t.Fatalf("invalid-template provider calls = %d, want zero", got)
		}
		assertDispatchErrorContains(t, scenario.factoryEvents(t), "prompt render failed")
		scenario.stop(t)
	})

	t.Run("dependency_failure", func(t *testing.T) {
		dir := testutilCopySharedFixture(t, "script_executor_dir")
		testutil.WriteSeedFile(t, dir, "task", []byte("dependency-failure-payload"))
		scenario, listed := runSharedProviderFactory(t, dir, dir, failureRunner("adverse dependency failure"), 5*time.Second)
		assertSessionPlaces(t, listed, map[string]int{"task:failed": 1, "task:init": 0, "task:done": 0})
		assertDispatchErrorContains(t, scenario.factoryEvents(t), "adverse dependency failure")
		scenario.stop(t)
	})

	t.Run("timeout", func(t *testing.T) {
		dir := testutilCopySharedFixture(t, "script_executor_dir")
		support.WriteWorkstationConfig(t, dir, "run-script", "---\ntype: MODEL_WORKSTATION\nlimits:\n  maxExecutionTime: 10ms\n---\nExecute the script.\n")
		testutil.WriteSeedFile(t, dir, "task", []byte("timeout-payload"))
		runner := newTimeoutThenSuccessCommandRunner()
		scenario, listed := runSharedProviderFactory(t, dir, dir, runner, 10*time.Second)
		assertSessionPlaces(t, listed, map[string]int{"task:done": 1, "task:init": 0, "task:failed": 0})
		if got := runner.CallCount(); got < 2 {
			t.Fatalf("timeout recovery provider calls = %d, want at least two", got)
		}
		scenario.stop(t)
	})

	t.Run("cancellation", func(t *testing.T) {
		dir := testutilCopySharedFixture(t, "script_executor_dir")
		testutil.WriteSeedFile(t, dir, "task", []byte("cancellation-payload"))
		scenario, listed := runSharedProviderFactory(t, dir, dir, canceledCommandRunner{}, 5*time.Second)
		assertSessionPlaces(t, listed, map[string]int{"task:failed": 1, "task:init": 0, "task:done": 0})
		assertDispatchErrorContains(t, scenario.factoryEvents(t), "execution cancelled: context canceled")
		scenario.stop(t)
	})

	t.Run("unknown_route", func(t *testing.T) {
		dir := testutilCopySharedFixture(t, "script_executor_dir")
		testutil.WriteSeedFile(t, dir, "task", []byte("unknown-route-payload"))
		scenario := fixture.openScenario(t, dir, "", nil)
		scenario.waitForTerminal(t, 5*time.Second)
		listed := scenario.listWork(t)
		assertSessionPlaces(t, listed, map[string]int{"task:failed": 1, "task:init": 0, "task:done": 0})
		assertDispatchErrorContains(t, scenario.factoryEvents(t), "script command execution failed")
		scenario.stop(t)
	})

	t.Run("known_good_after_adverse_cases", func(t *testing.T) {
		dir := testutilCopySharedFixture(t, "script_executor_dir")
		testutil.WriteSeedFile(t, dir, "task", []byte("known-good-payload"))
		scenario, listed := runSharedProviderFactory(t, dir, dir, support.NewStaticSuccessCommandRunner("known-good-output"), 5*time.Second)
		assertSessionPlaces(t, listed, map[string]int{"task:done": 1, "task:init": 0, "task:failed": 0})
		assertDispatchOutput(t, scenario.factoryEvents(t), "known-good-output")
		scenario.stop(t)
	})

	if got := fixture.router.routeCount(); got != 0 {
		t.Fatalf("shared provider routes after adverse recovery = %d, want zero", got)
	}
}

func testutilCopySharedFixture(t *testing.T, name string) string {
	t.Helper()
	source := support.LegacyFixtureDir(t, name)
	return testutil.CopyFixtureDir(t, source)
}
