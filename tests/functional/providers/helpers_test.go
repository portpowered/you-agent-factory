package providers

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	providercontract "github.com/portpowered/infinite-you/pkg/services/providers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const commandRunnerCompletedLogEvent = "command_runner.completed"

const sharedProviderFixtureTimeout = FixtureTimeout

const (
	sharedMockAgentAcceptWorkID   = "shared-mock-agent-accept"
	sharedMockAgentRejectWorkID   = "shared-mock-agent-reject"
	sharedMockScriptAcceptWorkID  = "shared-mock-script-accept"
	sharedMockScriptRejectWorkID  = "shared-mock-script-reject"
	sharedMockServiceModelWorkID  = "shared-mock-service-model"
	sharedMockServiceScriptWorkID = "shared-mock-service-script"
)

type sharedProviderProcessFixture = ProcessFixture
type sharedProviderScenario struct {
	*Scenario
}

func (scenario *sharedProviderScenario) stop(t *testing.T) {
	t.Helper()
	scenario.Stop(t)
}

func (scenario *sharedProviderScenario) waitForTerminal(t testing.TB, timeout time.Duration) {
	t.Helper()
	scenario.WaitForTerminal(t, timeout)
}

func (scenario *sharedProviderScenario) listWork(t testing.TB) factoryapi.ListWorkResponse {
	t.Helper()
	return scenario.ListWork(t)
}

func (scenario *sharedProviderScenario) factoryEvents(t testing.TB) []factoryapi.FactoryEvent {
	t.Helper()
	return scenario.FactoryEvents(t)
}

func TestMain(m *testing.M) {
	code := m.Run()
	if err := CloseGlobalFixture(); err != nil {
		fmt.Fprintf(os.Stderr, "close shared provider fixture: %v\n", err)
		if code == 0 {
			code = 1
		}
	}
	os.Exit(code)
}

type fakeCommandRunner struct {
	stdout   string
	stderr   string
	exitCode int
}

func (f *fakeCommandRunner) Run(_ context.Context, _ platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	return platformprocess.CommandResult{Stdout: []byte(f.stdout), Stderr: []byte(f.stderr), ExitCode: f.exitCode}, nil
}

type canceledCommandRunner struct{}

func (canceledCommandRunner) Run(_ context.Context, _ platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	return platformprocess.CommandResult{}, context.Canceled
}

type captureCommandRunner struct {
	mu       sync.Mutex
	workDirs []string
	envs     [][]string
}

func (r *captureCommandRunner) Run(_ context.Context, req platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	r.mu.Lock()
	r.workDirs = append(r.workDirs, req.WorkDir)
	copiedEnv := make([]string, len(req.Env))
	copy(copiedEnv, req.Env)
	r.envs = append(r.envs, copiedEnv)
	r.mu.Unlock()
	return platformprocess.CommandResult{Stdout: []byte("script-output-ok")}, nil
}

func (r *captureCommandRunner) LastWorkDir() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.workDirs) == 0 {
		return ""
	}
	return r.workDirs[len(r.workDirs)-1]
}

func (r *captureCommandRunner) LastEnv() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.envs) == 0 {
		return nil
	}
	copied := make([]string, len(r.envs[len(r.envs)-1]))
	copy(copied, r.envs[len(r.envs)-1])
	return copied
}

func (r *captureCommandRunner) CallCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.workDirs)
}

type timeoutThenSuccessCommandRunner struct {
	mu        sync.Mutex
	callCount int
}

func newTimeoutThenSuccessCommandRunner() *timeoutThenSuccessCommandRunner {
	return &timeoutThenSuccessCommandRunner{}
}

func (r *timeoutThenSuccessCommandRunner) Run(ctx context.Context, _ platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	r.mu.Lock()
	r.callCount++
	call := r.callCount
	r.mu.Unlock()

	if call == 1 {
		<-ctx.Done()
		return platformprocess.CommandResult{}, ctx.Err()
	}

	return platformprocess.CommandResult{Stdout: []byte("script-output-after-retry")}, nil
}

func (r *timeoutThenSuccessCommandRunner) CallCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.callCount
}

type echoArgsRunner struct{}

func (e *echoArgsRunner) Run(_ context.Context, req platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	return platformprocess.CommandResult{Stdout: []byte(strings.Join(req.Args, "\n"))}, nil
}

type templateCaptureCommandRunner struct {
	mu      sync.Mutex
	request platformprocess.CommandRequest
}

func (r *templateCaptureCommandRunner) Run(_ context.Context, req platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	r.mu.Lock()
	r.request = req
	r.mu.Unlock()

	return platformprocess.CommandResult{Stdout: []byte(strings.Join(req.Args, "\n"))}, nil
}

func (r *templateCaptureCommandRunner) LastRequest() platformprocess.CommandRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.request
}

func failureRunner(stderr string) platformprocess.CommandRunner {
	return &fakeCommandRunner{stderr: stderr, exitCode: 1}
}

func sharedProviderFixtureFor(t *testing.T) *sharedProviderProcessFixture {
	t.Helper()
	return FixtureFor(t)
}

type providerResultCommandRunner struct {
	result platformprocess.CommandResult
	err    error
}

func (runner providerResultCommandRunner) Run(_ context.Context, _ platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	return runner.result, runner.err
}

func sharedProviderRefusalRunner() platformprocess.CommandRunner {
	return providerResultCommandRunner{result: platformprocess.CommandResult{
		ExitCode: 7,
	}, err: providercontract.ExecuteFailure{
		Kind:    providercontract.ExecuteFailureKindInvalidRequest,
		Message: "provider error: permanent_bad_request: provider rejected the execution request",
	}}
}

func sharedScriptFailureRunner() platformprocess.CommandRunner {
	return providerResultCommandRunner{result: platformprocess.CommandResult{
		Stdout:   []byte("script configured stdout"),
		Stderr:   []byte("script configured stderr"),
		ExitCode: 9,
	}}
}

func configureSharedCodexWorker(t *testing.T, dir string) {
	t.Helper()
	support.WriteAgentConfig(t, dir, "worker", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "test-model"))
}

func runSharedProviderFactory(
	t *testing.T,
	dir, workDir string,
	runner platformprocess.CommandRunner,
	timeout time.Duration,
) (*sharedProviderScenario, factoryapi.ListWorkResponse) {
	t.Helper()
	scenario, listed := RunFactory(t, dir, workDir, runner, timeout)
	return &sharedProviderScenario{Scenario: scenario}, listed
}

func runSharedMockFactory(
	t *testing.T,
	dir string,
	runner platformprocess.CommandRunner,
	timeout time.Duration,
) (*sharedProviderScenario, factoryapi.ListWorkResponse) {
	t.Helper()
	return runSharedProviderFactory(t, dir, dir, runner, timeout)
}

func findSharedRuntimeLogRecord(
	t *testing.T,
	fixture *sharedProviderProcessFixture,
	workDir string,
	exitCode int,
) map[string]any {
	t.Helper()
	return FindRuntimeLogRecord(t, fixture, workDir, exitCode)
}

func updateScriptFixtureFactory(t *testing.T, dir string, mutate func(map[string]any)) {
	t.Helper()

	path := filepath.Join(dir, "factory.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read factory.json: %v", err)
	}

	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("unmarshal factory.json: %v", err)
	}

	mutate(cfg)

	updated, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal factory.json: %v", err)
	}
	if err := os.WriteFile(path, updated, 0o644); err != nil {
		t.Fatalf("write factory.json: %v", err)
	}
}

func writeWorkstationPromptTemplate(t *testing.T, dir, templateBody string) {
	t.Helper()

	writeNamedWorkstationPromptTemplate(t, dir, "run-script", templateBody)
}

func writeNamedWorkstationPromptTemplate(t *testing.T, dir, workstationName, templateBody string) {
	t.Helper()

	path := filepath.Join(dir, "workstations", workstationName, "AGENTS.md")
	content := "---\ntype: MODEL_WORKSTATION\n---\n" + templateBody + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write workstation AGENTS.md: %v", err)
	}
}

func writeFixtureFile(t *testing.T, dir string, pathParts []string, content string) {
	t.Helper()

	path := filepath.Join(append([]string{dir}, pathParts...)...)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func writeScriptWorkerArgs(t *testing.T, dir string, args []string) {
	t.Helper()

	lines := []string{"---", "type: SCRIPT_WORKER", "command: echo", "args:"}
	for _, arg := range args {
		lines = append(lines, "  - "+quoteYAMLString(arg))
	}
	lines = append(lines, "---", "Execute the script.")
	writeFixtureFile(t, dir, []string{"workers", "script-worker", "AGENTS.md"}, strings.Join(lines, "\n")+"\n")
}

func writeRuntimeMergeWorkstationConfig(t *testing.T, dir string) {
	t.Helper()

	body := strings.Join([]string{
		`runtime prompt name={{ (index .Inputs 0).Name }}`,
		`runtime prompt work={{ (index .Inputs 0).WorkID }}`,
		`runtime prompt workdir={{ .Context.WorkDir }}`,
		`runtime prompt env={{ index .Context.Env "RUNTIME_BRANCH" }}`,
	}, "\n")
	agentsMD := strings.Join([]string{
		"---",
		"type: MODEL_WORKSTATION",
		"worker: script-worker",
		"outputs:",
		"  - workType: task",
		"    state: runtime-done",
		`workingDirectory: '/runtime/{{ (index .Inputs 0).Name }}/{{ index (index .Inputs 0).Tags "branch" }}'`,
		`worktree: 'worktrees/{{ index (index .Inputs 0).Tags "branch" }}/{{ (index .Inputs 0).WorkID }}'`,
		"env:",
		`  RUNTIME_BRANCH: '{{ index (index .Inputs 0).Tags "branch" }}'`,
		`  RUNTIME_NAME: '{{ (index .Inputs 0).Name }}'`,
		"---",
		body,
	}, "\n") + "\n"

	writeFixtureFile(t, dir, []string{"workstations", "run-script", "AGENTS.md"}, agentsMD)
}

func quoteYAMLString(value string) string {
	return strconv.Quote(value)
}

func assertCommandArgs(t *testing.T, req platformprocess.CommandRequest, want []string) {
	t.Helper()

	if !reflect.DeepEqual(req.Args, want) {
		t.Fatalf("command args = %#v, want %#v", req.Args, want)
	}
}

func assertRuntimeMergeCommandRequest(t *testing.T, dir string, req platformprocess.CommandRequest) {
	t.Helper()

	if req.Command != "echo" {
		t.Fatalf("command = %q, want %q", req.Command, "echo")
	}
	if req.WorkDir != support.ResolvedRuntimePath(dir, "/runtime/runtime-template-name/feature-runtime-config") {
		t.Fatalf("work dir = %q, want resolved runtime working_directory", req.WorkDir)
	}
	for _, want := range []string{
		"INLINE_ONLY=true",
		"RUNTIME_BRANCH=feature-runtime-config",
		"RUNTIME_NAME=runtime-template-name",
	} {
		if !containsEnv(req.Env, want) {
			t.Fatalf("script runner env missing %s in %v", want, req.Env)
		}
	}
}

func findRuntimeLogRecord(t *testing.T, path, eventName string) map[string]any {
	t.Helper()

	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open runtime log %s: %v", path, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var record map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			t.Fatalf("decode runtime log record: %v", err)
		}
		if record["event_name"] == eventName {
			return record
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan runtime log %s: %v", path, err)
	}
	t.Fatalf("runtime log %s did not contain event_name %q", path, eventName)
	return nil
}

func containsEnv(env []string, expected string) bool {
	for _, entry := range env {
		if entry == expected {
			return true
		}
	}
	return false
}

func assertSessionPlaces(t *testing.T, listed factoryapi.ListWorkResponse, wants map[string]int) {
	t.Helper()
	for placeID, want := range wants {
		if got := support.CountWorkAtCustomerState(listed, placeID); got != want {
			t.Errorf("%s token count = %d, want %d", placeID, got, want)
		}
	}
}

func assertDispatchOutput(t *testing.T, events []factoryapi.FactoryEvent, want string) {
	t.Helper()
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeDispatchResponse {
			continue
		}
		payload, err := event.Payload.AsDispatchResponseEventPayload()
		if err != nil {
			t.Fatalf("decode dispatch response: %v", err)
		}
		if payload.Output == nil || *payload.Output != want {
			t.Fatalf("dispatch output = %#v, want %q", payload.Output, want)
		}
		return
	}
	t.Fatalf("Factory Event history has no dispatch response: %#v", events)
}

const FixtureTimeout = 15 * time.Second

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

type ProcessFixture struct {
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
	fixture *ProcessFixture
	initErr error
}

// FixtureFor returns the package-shared root process fixture. The caller's
// package TestMain must call CloseGlobalFixture after its tests complete.
func FixtureFor(t *testing.T) *ProcessFixture {
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

	support.WaitForStatus(t, fixture.baseURL, FixtureTimeout, func(status factoryapi.StatusResponse) bool {
		return strings.TrimSpace(status.RuntimeStatus) != ""
	})
	return fixture
}

func newSharedProviderProcessFixture(t *testing.T) (*ProcessFixture, error) {
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
		"--runtime-log-dir", runtimeLogs,
	})
	inputs.Input.Env = []string{"HOME=" + homeDir, "USERPROFILE=" + homeDir}
	inputs.Input.WorkingDirectory = bootstrap

	fixture := &ProcessFixture{
		rootDir: rootDir, bootstrap: bootstrap, homeDir: homeDir,
		runtimeLogs: runtimeLogs, process: process, cancel: cancel,
		done: make(chan struct{}), api: api, router: router,
		constructor: constructor,
	}
	go func() {
		fixture.executeErr = process.Execute(inputs.Input)
		close(fixture.done)
	}()

	baseURL, err := api.server.WaitForBaseURL(FixtureTimeout)
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

// CloseGlobalFixture shuts down the package-shared root process and verifies
// that its listener, routes, sessions, and temporary files are gone.
func CloseGlobalFixture() error {
	sharedProviderGlobal.mu.Lock()
	fixture := sharedProviderGlobal.fixture
	sharedProviderGlobal.mu.Unlock()
	if fixture == nil {
		return nil
	}
	return fixture.close()
}

func (fixture *ProcessFixture) close() error {
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
	case <-time.After(FixtureTimeout):
		// Process.Execute normally reports shutdown through done after the
		// cancellation signal. Keep this safety bound so a broken process
		// implementation cannot strand the test process during finalization.
		closeErr = fmt.Errorf("timed out waiting for Process.Execute shutdown")
	}

	closeCtx, cancel := context.WithTimeout(context.Background(), FixtureTimeout)
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

type Scenario struct {
	fixture     *ProcessFixture
	factoryDir  string
	sessionID   string
	routeSelect string
	closeOnce   sync.Once
}

func (fixture *ProcessFixture) OpenScenario(
	t *testing.T,
	factoryDir, workDir string,
	runner platformprocess.CommandRunner,
) *Scenario {
	t.Helper()

	routeSelector := ""
	var scenario *Scenario
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
	scenario = &Scenario{
		fixture: fixture, factoryDir: factoryDir, sessionID: opened.Session.Id,
		routeSelect: routeSelector,
	}
	fixture.sessionMu.Lock()
	fixture.openedSessionIDs = append(fixture.openedSessionIDs, scenario.sessionID)
	fixture.sessionMu.Unlock()
	t.Cleanup(func() { scenario.close(t) })
	return scenario
}

func (scenario *Scenario) close(t testing.TB) {
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

func (fixture *ProcessFixture) markSessionDeleted(sessionID string) {
	fixture.sessionMu.Lock()
	defer fixture.sessionMu.Unlock()
	for _, existing := range fixture.deletedSessionIDs {
		if existing == sessionID {
			return
		}
	}
	fixture.deletedSessionIDs = append(fixture.deletedSessionIDs, sessionID)
}

func (scenario *Scenario) WaitForTerminal(t testing.TB, timeout time.Duration) {
	t.Helper()
	support.WaitForSessionTerminalStatus(t, scenario.fixture.baseURL, scenario.sessionID, timeout)
}

func (scenario *Scenario) ListWork(t testing.TB) factoryapi.ListWorkResponse {
	t.Helper()
	endpoint := strings.TrimSuffix(scenario.fixture.baseURL, "/") +
		"/factory-sessions/" + url.PathEscape(scenario.sessionID) + "/work"
	return support.GetJSON[factoryapi.ListWorkResponse](t, endpoint)
}

func (scenario *Scenario) FactoryEvents(t testing.TB) []factoryapi.FactoryEvent {
	t.Helper()
	return support.GetFactoryEventsForSessionAt(t, scenario.fixture.baseURL, scenario.sessionID)
}

func (scenario *Scenario) Stop(t *testing.T) {
	t.Helper()
	scenario.close(t)
}

func RunFactory(
	t *testing.T,
	dir, workDir string,
	runner platformprocess.CommandRunner,
	timeout time.Duration,
) (*Scenario, factoryapi.ListWorkResponse) {
	t.Helper()
	fixture := FixtureFor(t)
	scenario := fixture.OpenScenario(t, dir, workDir, runner)
	scenario.WaitForTerminal(t, timeout)
	return scenario, scenario.ListWork(t)
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

func (fixture *ProcessFixture) AssertTopology(t testing.TB) {
	t.Helper()
	if got := fixture.constructor.count(); got != 1 {
		t.Fatalf("shared provider root constructions = %d, want one", got)
	}
	if got := fixture.api.startCount(); got != 1 {
		t.Fatalf("shared provider listener starts = %d, want one", got)
	}
}

func (fixture *ProcessFixture) AssertSessionTopology(t testing.TB) {
	t.Helper()
	if err := fixture.validateSessionTopology(); err != nil {
		t.Fatal(err)
	}
}

func (fixture *ProcessFixture) validateSessionTopology() error {
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

func (fixture *ProcessFixture) RuntimeLogDir() string {
	return fixture.runtimeLogs
}
