package agy

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const agySharedInvocationTimeout = 30 * time.Second

const agySharedRouteCount = 7

// agySharedCommandRoute is immutable after the package-owned command router
// freezes. Its request ledger is invocation-observable and remains separate
// from the other routes' ledgers.
type agySharedCommandRoute struct {
	selector    string
	workDir     string
	rootDir     string
	homeDir     string
	assetPath   string
	factoryName string
	result      platformprocess.CommandResult

	mu       sync.Mutex
	requests []platformprocess.CommandRequest
}

func (route *agySharedCommandRoute) record(request platformprocess.CommandRequest) platformprocess.CommandResult {
	route.mu.Lock()
	defer route.mu.Unlock()
	route.requests = append(route.requests, cloneAgyCommandRequest(request))
	return cloneAgyCommandResult(route.result)
}

func (route *agySharedCommandRoute) callCount() int {
	route.mu.Lock()
	defer route.mu.Unlock()
	return len(route.requests)
}

func (route *agySharedCommandRoute) lastRequest() platformprocess.CommandRequest {
	route.mu.Lock()
	defer route.mu.Unlock()
	if len(route.requests) == 0 {
		panic("agySharedCommandRoute: LastRequest called with no requests")
	}
	return cloneAgyCommandRequest(route.requests[len(route.requests)-1])
}

// agySharedCommandRunner selects solely from the normalized provider WorkDir.
// Registration is closed before root.BuildProcess, so an invocation cannot
// mutate routing or select a sibling through mutable session data.
type agySharedCommandRunner struct {
	mu        sync.Mutex
	routes    map[string]*agySharedCommandRoute
	selectors map[string]struct{}
	frozen    bool
	requests  []platformprocess.CommandRequest
}

func newAgySharedCommandRunner() *agySharedCommandRunner {
	return &agySharedCommandRunner{
		routes:    make(map[string]*agySharedCommandRoute),
		selectors: make(map[string]struct{}),
	}
}

func (runner *agySharedCommandRunner) register(
	selector, workDir string,
	result platformprocess.CommandResult,
) (*agySharedCommandRoute, error) {
	selector = strings.TrimSpace(selector)
	normalized, err := normalizeAgyRoutePath(workDir)
	if err != nil {
		return nil, err
	}
	if selector == "" {
		return nil, fmt.Errorf("AGY route selector is required")
	}

	runner.mu.Lock()
	defer runner.mu.Unlock()
	if runner.frozen {
		return nil, fmt.Errorf("AGY route table is frozen")
	}
	if _, exists := runner.routes[normalized]; exists {
		return nil, fmt.Errorf("AGY normalized WorkDir route is already registered")
	}
	if _, exists := runner.selectors[selector]; exists {
		return nil, fmt.Errorf("AGY route selector is already registered")
	}

	route := &agySharedCommandRoute{
		selector: selector,
		workDir:  filepath.Clean(workDir),
		result:   cloneAgyCommandResult(result),
	}
	runner.routes[normalized] = route
	runner.selectors[selector] = struct{}{}
	return route, nil
}

func (runner *agySharedCommandRunner) freeze() {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	runner.frozen = true
}

func (runner *agySharedCommandRunner) routeCount() int {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return len(runner.routes)
}

func (runner *agySharedCommandRunner) callCount() int {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return len(runner.requests)
}

func (runner *agySharedCommandRunner) clear() error {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	if !runner.frozen {
		return fmt.Errorf("AGY route table was not frozen")
	}
	runner.routes = nil
	runner.selectors = nil
	runner.requests = nil
	return nil
}

func (runner *agySharedCommandRunner) Run(
	ctx context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	normalized, err := normalizeAgyRoutePath(request.WorkDir)
	if err != nil {
		return platformprocess.CommandResult{}, fmt.Errorf("AGY invocation route rejected")
	}

	runner.mu.Lock()
	if !runner.frozen {
		runner.mu.Unlock()
		return platformprocess.CommandResult{}, fmt.Errorf("AGY route table is not frozen")
	}
	route, ok := runner.routes[normalized]
	if !ok {
		runner.mu.Unlock()
		return platformprocess.CommandResult{}, fmt.Errorf("AGY invocation has no frozen route")
	}
	if request.Command != "agy" {
		runner.mu.Unlock()
		return platformprocess.CommandResult{}, fmt.Errorf("AGY route received an unexpected command")
	}
	if err := ctx.Err(); err != nil {
		runner.mu.Unlock()
		return platformprocess.CommandResult{}, err
	}
	runner.requests = append(runner.requests, cloneAgyCommandRequest(request))
	runner.mu.Unlock()

	return route.record(request), nil
}

func normalizeAgyRoutePath(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("AGY route WorkDir is required")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("normalize AGY route WorkDir: %w", err)
	}
	clean := filepath.Clean(abs)
	if runtime.GOOS == "windows" {
		clean = strings.ToLower(clean)
	}
	return clean, nil
}

func cloneAgyCommandRequest(request platformprocess.CommandRequest) platformprocess.CommandRequest {
	request.Args = append([]string(nil), request.Args...)
	request.Stdin = append([]byte(nil), request.Stdin...)
	request.Env = append([]string(nil), request.Env...)
	return request
}

func cloneAgyCommandResult(result platformprocess.CommandResult) platformprocess.CommandResult {
	result.Stdout = append([]byte(nil), result.Stdout...)
	result.Stderr = append([]byte(nil), result.Stderr...)
	return result
}

type agySharedHTTPRun struct {
	server *support.ProcessAPIServer
	done   chan struct{}
}

func (run *agySharedHTTPRun) waitClosed(ctx context.Context) error {
	if run == nil {
		return fmt.Errorf("shared AGY HTTP run is nil")
	}
	select {
	case <-run.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// agySharedHTTPServer creates a fresh listener for each hosted Process.Execute
// invocation while keeping the HTTP edge itself in the one package-owned
// process graph.
type agySharedHTTPServer struct {
	mu      sync.Mutex
	starts  int
	runs    []*agySharedHTTPRun
	started chan *agySharedHTTPRun
}

func newAgySharedHTTPServer() *agySharedHTTPServer {
	return &agySharedHTTPServer{
		started: make(chan *agySharedHTTPRun, 16),
	}
}

func (server *agySharedHTTPServer) start(
	ctx context.Context,
	request platformhttpserver.StartRequest,
) error {
	run := &agySharedHTTPRun{
		server: support.NewProcessAPIServer(),
		done:   make(chan struct{}),
	}
	server.mu.Lock()
	server.starts++
	server.runs = append(server.runs, run)
	server.mu.Unlock()
	server.started <- run
	defer close(run.done)
	return run.server.Start(ctx, request)
}

func (server *agySharedHTTPServer) waitForStart(t *testing.T) *agySharedHTTPRun {
	t.Helper()
	timer := time.NewTimer(agySharedInvocationTimeout)
	defer timer.Stop()
	select {
	case run := <-server.started:
		return run
	case <-timer.C:
		t.Fatal("timed out waiting for shared AGY HTTP server starter")
		return nil
	}
}

func (server *agySharedHTTPServer) startCount() int {
	server.mu.Lock()
	defer server.mu.Unlock()
	return server.starts
}

func (server *agySharedHTTPServer) waitClosed(ctx context.Context) error {
	server.mu.Lock()
	runs := append([]*agySharedHTTPRun(nil), server.runs...)
	server.mu.Unlock()
	for _, run := range runs {
		select {
		case <-run.done:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

type agySharedProcessFixture struct {
	rootDir string
	process support.ApplicationProcess
	runner  *agySharedCommandRunner
	api     *agySharedHTTPServer
	routes  map[string]*agySharedCommandRoute

	closeOnce sync.Once
	closeErr  error
}

var agySharedFixtureState struct {
	sync.Mutex
	fixture *agySharedProcessFixture
}

func agySharedProcess(t *testing.T) *agySharedProcessFixture {
	t.Helper()
	agySharedFixtureState.Lock()
	defer agySharedFixtureState.Unlock()
	if agySharedFixtureState.fixture != nil {
		return agySharedFixtureState.fixture
	}
	fixture := newAgySharedProcessFixture(t)
	agySharedFixtureState.fixture = fixture
	return fixture
}

func TestMain(m *testing.M) {
	code := m.Run()

	agySharedFixtureState.Lock()
	fixture := agySharedFixtureState.fixture
	agySharedFixtureState.Unlock()
	if fixture != nil {
		if err := fixture.close(); err != nil {
			fmt.Fprintf(os.Stderr, "close shared AGY process fixture: %v\n", err)
			code = 1
		}
	}
	os.Exit(code)
}

func newAgySharedProcessFixture(t *testing.T) *agySharedProcessFixture {
	t.Helper()
	rootDir, err := os.MkdirTemp("", "you-agy-shared-")
	if err != nil {
		t.Fatalf("create shared AGY fixture root: %v", err)
	}
	fixture := &agySharedProcessFixture{
		rootDir: rootDir,
		runner:  newAgySharedCommandRunner(),
		api:     newAgySharedHTTPServer(),
		routes:  make(map[string]*agySharedCommandRoute),
	}
	defer func() {
		if fixture.process == nil {
			_ = os.RemoveAll(rootDir)
		}
	}()

	fixture.registerGoldenRoutes(t)
	fixture.registerRoleRoutes(t)
	fixture.runner.freeze()
	if got := fixture.runner.routeCount(); got != agySharedRouteCount {
		t.Fatalf("shared AGY frozen route count = %d, want %d", got, agySharedRouteCount)
	}
	assertAgySharedRouteRejections(t, fixture.runner, rootDir, fixture.routes["golden-video-watch"].workDir)

	process, err := support.BuildProcessWithContext(context.Background(), serviceedges.Edges{
		APIServerStarter:      fixture.api.start,
		ProviderCommandRunner: fixture.runner,
	})
	if err != nil {
		t.Fatalf("BuildProcess(shared AGY fixture): %v", err)
	}
	fixture.process = process
	return fixture
}

func (fixture *agySharedProcessFixture) registerGoldenRoutes(t *testing.T) {
	t.Helper()
	fixture.addGoldenRoute(
		t,
		"golden-video-watch",
		agyGoldenVideoPrompt,
		"",
		"clip-fixture.mp4",
		"agy-trace-video-watch.stream.jsonl",
	)
	fixture.addGoldenRoute(
		t,
		"golden-groundtruth-video",
		agyGoldenGroundtruthPrompt,
		"",
		"groundtruth-fixture.mp4",
		"agy-trace-groundtruth-verbose.stream.jsonl",
	)
	fixture.addGoldenRoute(
		t,
		"golden-clipqa",
		agyGoldenVideoPrompt,
		"",
		"clip-fixture.mp4",
		"agy-trace-clipqa-schema.stream.jsonl",
	)
	fixture.addGoldenRoute(
		t,
		"golden-structured",
		"Classify the statement as positive or negative and provide confidence.",
		agyGoldenStructuredSchema,
		"",
		"agy-trace-structured.json",
	)
}

func (fixture *agySharedProcessFixture) registerRoleRoutes(t *testing.T) {
	t.Helper()
	fixture.addRoleRoute(
		t,
		"role-cold-watch-complete",
		agyColdWatchFactoryName,
		"clip-fixture.mp4",
		agyColdWatchCompleteReportTrace(t),
	)
	fixture.addRoleRoute(
		t,
		"role-clipqa-pass",
		agyClipQAFactoryName,
		"clip-fixture.mp4",
		alignAgyClipQAReplaySchema(t, readAgyGoldenAsset(t, "agy-trace-clipqa-schema.stream.jsonl")),
	)
	verdict := validAgyClipQAVerdictPayload()
	verdict["action_completed"] = false
	verdict["spec_deviations"] = []string{"the specified action did not finish"}
	verdict["verdict"] = "reroll"
	verdict["confidence"] = 0.82
	fixture.addRoleRoute(
		t,
		"role-clipqa-reroll",
		agyClipQAFactoryName,
		"clip-fixture.mp4",
		alignAgyClipQAReplaySchema(t, agyClipQAVerdictTrace(t, verdict)),
	)
}

func (fixture *agySharedProcessFixture) addGoldenRoute(
	t *testing.T,
	selector, prompt, schema, asset, trace string,
) {
	t.Helper()
	route := fixture.newRouteDirectories(t, selector)
	copyAgyDirectory(t, support.LegacyFixtureDir(t, "executor_success"), route.workDir)
	if asset != "" {
		copyAgySharedAsset(t, route.workDir, asset)
	}
	support.WriteAgentConfig(t, route.workDir, "worker", agyGoldenWorkerConfig())
	support.WriteWorkstationConfig(t, route.workDir, "process", agyGoldenWorkstationConfig(prompt, schema))
	testutil.WriteSeedFile(t, route.workDir, "task", []byte(`{"title":"agy shared golden"}`))
	result := platformprocess.CommandResult{Stdout: readAgyGoldenAsset(t, trace)}
	registered, err := fixture.runner.register(selector, route.workDir, result)
	if err != nil {
		t.Fatalf("register shared AGY route %q: %v", selector, err)
	}
	registered.rootDir = route.rootDir
	registered.homeDir = route.homeDir
	registered.assetPath = filepath.Join(route.workDir, asset)
	fixture.routes[selector] = registered
}

func (fixture *agySharedProcessFixture) addRoleRoute(
	t *testing.T,
	selector, factoryName, asset string,
	stdout []byte,
) {
	t.Helper()
	route := fixture.newRouteDirectories(t, selector)
	if asset != "" {
		copyAgySharedAsset(t, route.workDir, asset)
	}
	registered, err := fixture.runner.register(selector, route.workDir, platformprocess.CommandResult{
		Stdout:   stdout,
		ExitCode: 0,
	})
	if err != nil {
		t.Fatalf("register shared AGY route %q: %v", selector, err)
	}
	registered.rootDir = route.rootDir
	registered.homeDir = route.homeDir
	registered.assetPath = filepath.Join(route.workDir, asset)
	registered.factoryName = factoryName
	fixture.routes[selector] = registered
}

func (fixture *agySharedProcessFixture) newRouteDirectories(
	t *testing.T,
	selector string,
) *agySharedCommandRoute {
	t.Helper()
	rootDir := filepath.Join(fixture.rootDir, selector)
	homeDir := filepath.Join(rootDir, "home")
	workDir := filepath.Join(rootDir, "work")
	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		t.Fatalf("create shared AGY home %q: %v", homeDir, err)
	}
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatalf("create shared AGY workspace %q: %v", workDir, err)
	}
	return &agySharedCommandRoute{rootDir: rootDir, homeDir: homeDir, workDir: workDir}
}

func assertAgySharedRouteRejections(
	t *testing.T,
	runner *agySharedCommandRunner,
	rootDir, existingWorkDir string,
) {
	t.Helper()
	validation := newAgySharedCommandRunner()
	if _, err := validation.register("first", existingWorkDir, platformprocess.CommandResult{}); err != nil {
		t.Fatalf("register AGY route for duplicate probe: %v", err)
	}
	if _, err := validation.register("duplicate", filepath.Join(existingWorkDir, "."), platformprocess.CommandResult{}); err == nil {
		t.Fatal("duplicate normalized AGY route was accepted")
	}
	validation.freeze()
	if _, err := runner.Run(context.Background(), platformprocess.CommandRequest{
		Command: "agy",
		WorkDir: filepath.Join(rootDir, "unknown-workdir"),
		Stdin:   []byte("secret work payload"),
		Env:     []string{"AGY_SECRET=secret"},
	}); err == nil {
		t.Fatal("unknown frozen AGY route was accepted")
	}
	if runner.callCount() != 0 {
		t.Fatalf("AGY calls after unknown route rejection = %d, want zero", runner.callCount())
	}
}

func (fixture *agySharedProcessFixture) runGolden(
	t *testing.T,
	selector string,
) (factoryapi.FactorySession, factoryapi.ListWorkResponse, []factoryapi.FactoryEvent, *agySharedCommandRoute, int) {
	t.Helper()
	route := fixture.routes[selector]
	if route == nil {
		t.Fatalf("shared AGY golden route %q is missing", selector)
	}
	callStart := route.callCount()
	inputs := support.FakeInputs(context.Background(), []string{
		"you", "run",
		"--dir", route.workDir,
		"--continuously",
		"--with-server",
		"--server", "http://127.0.0.1:1",
		"--quiet",
		"--no-record",
	})
	inputs.Input.Env = agySharedEnvironment(route.homeDir)
	inputs.Input.WorkingDirectory = route.workDir
	command := support.StartProcessCommand(t, fixture.process, inputs.Input)
	httpRun := fixture.api.waitForStart(t)
	baseURL := httpRun.server.WaitForURL(t)
	liveSession := support.GetDefaultSession(t, baseURL)
	support.WaitForSessionTerminalStatus(t, baseURL, liveSession.Id, agySharedInvocationTimeout)
	session := support.GetDefaultSession(t, baseURL)
	listed := support.ListDefaultSessionWork(t, baseURL)
	events := support.GetFactoryEventsAt(t, baseURL)
	command.Stop(t)
	closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := httpRun.waitClosed(closeCtx); err != nil {
		t.Fatalf("close shared AGY HTTP server for %q: %v", selector, err)
	}
	return session, listed, events, route, callStart
}

func (fixture *agySharedProcessFixture) runRole(
	t *testing.T,
	selector string,
	args []string,
) (factoryapi.InvocationResponse, []factoryapi.FactoryEvent, *agySharedCommandRoute, string, int) {
	t.Helper()
	route := fixture.routes[selector]
	if route == nil {
		t.Fatalf("shared AGY role route %q is missing", selector)
	}
	callStart := route.callCount()
	env := agySharedEnvironment(route.homeDir)
	support.InstallPackagedFactoryWithProcess(t, fixture.process, env, route.workDir, route.factoryName)
	recordingDir, err := os.MkdirTemp(route.rootDir, "recording-")
	if err != nil {
		t.Fatalf("create shared AGY recording directory: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(recordingDir) })
	recordingPath := filepath.Join(recordingDir, "events.replay.json")
	args = append(append([]string(nil), args...), "--record", recordingPath, "--output", "primary")
	inputs := support.FakeInputs(context.Background(), args)
	inputs.Input.Env = env
	inputs.Input.WorkingDirectory = route.workDir
	if err := fixture.process.Execute(inputs.Input); err != nil {
		t.Fatalf("Process.Execute(shared AGY role %q): %v\nstdout=%s\nstderr=%s", selector, err, inputs.Stdout(), inputs.Stderr())
	}
	response := support.DecodeInvocationResponseJSON(t, inputs.Stdout())
	events := readAgyRecording(t, recordingPath, "shared AGY role")
	return response, events, route, route.assetPath, callStart
}

func runAgySharedColdWatchComplete(
	t *testing.T,
) (factoryapi.InvocationResponse, []factoryapi.FactoryEvent, *agySharedCommandRoute, string, int) {
	t.Helper()
	fixture := agySharedProcess(t)
	route := fixture.routes["role-cold-watch-complete"]
	return fixture.runRole(t, route.selector, []string{
		"you", "--json", "run",
		"--named", agyColdWatchFactoryName,
		"--cut-path", route.assetPath,
	})
}

func runAgySharedClipQAPass(
	t *testing.T,
) (factoryapi.InvocationResponse, []factoryapi.FactoryEvent, *agySharedCommandRoute, string, int) {
	t.Helper()
	fixture := agySharedProcess(t)
	route := fixture.routes["role-clipqa-pass"]
	return fixture.runRole(t, route.selector, []string{
		"you", "--json", "run",
		"--named", agyClipQAFactoryName,
		"--clip-path", route.assetPath,
		"--shot-specification", agyClipQAShotSpec,
	})
}

func runAgySharedClipQAReroll(
	t *testing.T,
) (factoryapi.InvocationResponse, []factoryapi.FactoryEvent, *agySharedCommandRoute, string, int) {
	t.Helper()
	fixture := agySharedProcess(t)
	route := fixture.routes["role-clipqa-reroll"]
	return fixture.runRole(t, route.selector, []string{
		"you", "--json", "run",
		"--named", agyClipQAFactoryName,
		"--clip-path", route.assetPath,
		"--shot-specification", agyClipQAShotSpec,
	})
}

func agySharedEnvironment(homeDir string) []string {
	return append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
}

func copyAgyDirectory(t *testing.T, sourceDir, targetDir string) {
	t.Helper()
	err := filepath.WalkDir(sourceDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}
		if relative == "." {
			return os.MkdirAll(targetDir, 0o755)
		}
		targetPath := filepath.Join(targetDir, relative)
		if entry.IsDir() {
			return os.MkdirAll(targetPath, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(targetPath, data, 0o644)
	})
	if err != nil {
		t.Fatalf("copy shared AGY fixture %q to %q: %v", sourceDir, targetDir, err)
	}
}

func copyAgySharedAsset(t *testing.T, dir, name string) {
	t.Helper()
	data := readAgyGoldenAsset(t, name)
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
		t.Fatalf("write shared AGY asset %s: %v", name, err)
	}
}

func (fixture *agySharedProcessFixture) close() error {
	fixture.closeOnce.Do(func() {
		var closeErrors []error
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if fixture.api != nil {
			if err := fixture.api.waitClosed(closeCtx); err != nil {
				closeErrors = append(closeErrors, err)
			}
		}
		if fixture.process != nil {
			if err := fixture.process.Close(closeCtx); err != nil {
				closeErrors = append(closeErrors, err)
			}
		}
		if fixture.runner != nil {
			if err := fixture.runner.clear(); err != nil {
				closeErrors = append(closeErrors, err)
			}
		}
		if fixture.rootDir != "" {
			if err := os.RemoveAll(fixture.rootDir); err != nil {
				closeErrors = append(closeErrors, err)
			} else if _, err := os.Stat(fixture.rootDir); !errors.Is(err, os.ErrNotExist) {
				closeErrors = append(closeErrors, fmt.Errorf("shared AGY fixture root remains: %w", err))
			}
		}
		fixture.closeErr = errors.Join(closeErrors...)
	})
	return fixture.closeErr
}

var _ platformprocess.CommandRunner = (*agySharedCommandRunner)(nil)
