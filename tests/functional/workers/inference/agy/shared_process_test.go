package agy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	agySharedScenarioTimeout = 30 * time.Second
	agySharedSuccessSelector = "agy-final-only-success"
	agySharedTimeoutSelector = "agy-timeout"
	agySharedCommand         = "agy"
	agySharedIdleHostFactory = `{"name":"agy-shared-host","workTypes":[],"workers":[],"workstations":[]}`
)

var agySharedProcess = &agyProcessFixture{}

// TestMain owns the package-scoped process. The process is deliberately lazy:
// a focused selector still starts exactly one production-composed process, but
// does not pay for a fixture when no AGY test is selected.
func TestMain(m *testing.M) {
	code := m.Run()
	if err := agySharedProcess.finalize(); err != nil {
		fmt.Fprintf(os.Stderr, "AGY shared process cleanup: %v\n", err)
		if code == 0 {
			code = 1
		}
	}
	os.Exit(code)
}

type agySharedHTTPServer struct {
	server *support.ProcessAPIServer

	mu       sync.Mutex
	starts   int
	done     chan struct{}
	doneOnce sync.Once
}

func newAgySharedHTTPServer() *agySharedHTTPServer {
	return &agySharedHTTPServer{
		server: support.NewProcessAPIServer(),
		done:   make(chan struct{}),
	}
}

func (server *agySharedHTTPServer) start(
	ctx context.Context,
	request platformhttpserver.StartRequest,
) error {
	server.mu.Lock()
	server.starts++
	server.mu.Unlock()
	defer server.doneOnce.Do(func() { close(server.done) })
	return server.server.Start(ctx, request)
}

func (server *agySharedHTTPServer) startCount() int {
	server.mu.Lock()
	defer server.mu.Unlock()
	return server.starts
}

func (server *agySharedHTTPServer) waitClosed(ctx context.Context) error {
	select {
	case <-server.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

type agySharedDaemon struct {
	cancel context.CancelFunc
	done   chan struct{}

	mu  sync.Mutex
	err error
}

// agyStaticCommandRunner is repeatable across -count runs. A queued test
// runner would fall back to its generic response after the first invocation,
// which would make a package-scoped process observe a different provider
// transcript on the second and later repetitions.
type agyStaticCommandRunner struct {
	result platformprocess.CommandResult
}

func newAgyStaticCommandRunner(result platformprocess.CommandResult) platformprocess.CommandRunner {
	return &agyStaticCommandRunner{result: agyCloneCommandResult(result)}
}

func (runner *agyStaticCommandRunner) Run(
	ctx context.Context,
	_ platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	if err := ctx.Err(); err != nil {
		return platformprocess.CommandResult{}, err
	}
	return agyCloneCommandResult(runner.result), nil
}

func agyCloneCommandResult(result platformprocess.CommandResult) platformprocess.CommandResult {
	result.Stdout = append([]byte(nil), result.Stdout...)
	result.Stderr = append([]byte(nil), result.Stderr...)
	return result
}

func startAgySharedDaemon(process support.ApplicationProcess, input root.Input) *agySharedDaemon {
	parent := input.Context
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	input.Context = ctx
	daemon := &agySharedDaemon{cancel: cancel, done: make(chan struct{})}
	go func() {
		err := process.Execute(input)
		daemon.mu.Lock()
		daemon.err = err
		daemon.mu.Unlock()
		close(daemon.done)
	}()
	return daemon
}

func (daemon *agySharedDaemon) stop(ctx context.Context) error {
	if daemon == nil {
		return nil
	}
	daemon.cancel()
	select {
	case <-daemon.done:
		daemon.mu.Lock()
		err := daemon.err
		daemon.mu.Unlock()
		if err != nil && !errors.Is(err, context.Canceled) {
			return err
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

type agySharedScenario struct {
	selector   string
	factoryDir string
	loaded     support.ProviderSessionCase
	request    agyGoldenRequest
}

type agySharedReplay struct {
	Session        factoryapi.FactorySessionSummary
	Submitted      factoryapi.SubmitWorkResponse
	Listed         factoryapi.ListWorkResponse
	FactoryEvents  []factoryapi.FactoryEvent
	ResponseEvents []factoryapi.FactoryResponseEvent
	RouteCalls     int
}

type agyProcessFixture struct {
	once      sync.Once
	setupErr  error
	finalOnce sync.Once
	finalErr  error

	rootDir    string
	homeDir    string
	hostDir    string
	baseURL    string
	successDir string
	timeoutDir string

	process support.ApplicationProcess
	api     *agySharedHTTPServer
	daemon  *agySharedDaemon
	router  *agySharedCommandRouter

	scenarios map[string]*agySharedScenario

	sessionMu        sync.Mutex
	openedSessionIDs []string
	deletedSessionID map[string]struct{}
	activeSessions   map[string]struct{}
	activeRuns       map[string]*agySharedScenarioRun

	streamMu      sync.Mutex
	streamsOpened int
	streamsClosed int

	processBuilds int
}

func agySharedProcessForTest(t *testing.T) *agyProcessFixture {
	t.Helper()
	agySharedProcess.once.Do(func() {
		agySharedProcess.setupErr = agySharedProcess.setup(t)
	})
	if agySharedProcess.setupErr != nil {
		t.Fatalf("setup shared AGY process: %v", agySharedProcess.setupErr)
	}
	return agySharedProcess
}

func (fixture *agyProcessFixture) setup(t *testing.T) (setupErr error) {
	t.Helper()
	rootDir, err := os.MkdirTemp("", "infinite-you-agy-shared-")
	if err != nil {
		return fmt.Errorf("create package fixture root: %w", err)
	}
	fixture.rootDir = rootDir
	defer func() {
		if setupErr == nil {
			return
		}
		cleanupErr := fixture.cleanupResources(context.Background())
		setupErr = errors.Join(setupErr, cleanupErr)
	}()

	if err := fixture.createDirectories(rootDir); err != nil {
		return err
	}
	if err := fixture.loadScenarios(t); err != nil {
		return err
	}
	if err := fixture.copyFactoryDirectories(t); err != nil {
		return err
	}
	if err := fixture.registerRoutes(); err != nil {
		return err
	}
	if err := fixture.startProcess(); err != nil {
		return err
	}
	if err := fixture.waitForReady(t); err != nil {
		return err
	}
	return nil
}

func (fixture *agyProcessFixture) createDirectories(rootDir string) error {
	fixture.homeDir = filepath.Join(rootDir, "home")
	fixture.hostDir = filepath.Join(rootDir, "host")
	fixture.successDir = filepath.Join(rootDir, "success")
	fixture.timeoutDir = filepath.Join(rootDir, "timeout")
	for _, path := range []string{fixture.homeDir, fixture.hostDir, fixture.successDir, fixture.timeoutDir} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			return fmt.Errorf("create package fixture path %q: %w", path, err)
		}
	}
	// Keep the long-lived API host's default session idle; scenario behavior
	// runs through the two immutable explicit-session Factory roots below.
	if err := os.WriteFile(filepath.Join(fixture.hostDir, "factory.json"), []byte(agySharedIdleHostFactory), 0o644); err != nil {
		return fmt.Errorf("write idle AGY host factory: %w", err)
	}
	return nil
}

func (fixture *agyProcessFixture) loadScenarios(t *testing.T) error {
	repoRoot := testutil.MustRepoRoot(t)
	successLoaded, successRequest, err := readAgyGoldenCase(
		repoRoot,
		agyFinalOnlySuccessGoldenCase,
		"agy-final-only-success",
		support.ProviderSessionFidelityFinalOnly,
	)
	if err != nil {
		return err
	}
	timeoutLoaded, timeoutRequest, err := readAgyGoldenCase(
		repoRoot,
		agyTimeoutGoldenCase,
		"agy-timeout",
		support.ProviderSessionFidelityFinalOnly,
	)
	if err != nil {
		return err
	}
	fixture.scenarios = map[string]*agySharedScenario{
		agyFinalOnlySuccessGoldenCase: {
			selector: agySharedSuccessSelector, factoryDir: fixture.successDir,
			loaded: successLoaded, request: successRequest,
		},
		agyTimeoutGoldenCase: {
			selector: agySharedTimeoutSelector, factoryDir: fixture.timeoutDir,
			loaded: timeoutLoaded, request: timeoutRequest,
		},
	}
	return nil
}

func (fixture *agyProcessFixture) copyFactoryDirectories(t *testing.T) error {
	legacyDir := support.LegacyFixtureDir(t, "executor_success")
	for _, path := range []string{fixture.successDir, fixture.timeoutDir} {
		if err := copyAgyFactoryDirectory(legacyDir, path); err != nil {
			return fmt.Errorf("copy legacy fixture to %q: %w", path, err)
		}
		if err := os.RemoveAll(filepath.Join(path, "inputs")); err != nil {
			return fmt.Errorf("clear seed inputs in %q: %w", path, err)
		}
	}
	successModel := fixture.scenarios[agyFinalOnlySuccessGoldenCase].request.Model
	timeoutModel := fixture.scenarios[agyTimeoutGoldenCase].request.Model
	if err := writeAgyWorkerConfig(fixture.successDir, successModel); err != nil {
		return err
	}
	return writeAgyWorkerConfig(fixture.timeoutDir, timeoutModel)
}

func (fixture *agyProcessFixture) registerRoutes() error {
	success := fixture.scenarios[agyFinalOnlySuccessGoldenCase]
	timeout := fixture.scenarios[agyTimeoutGoldenCase]
	successExitCode := 0
	if success.loaded.Process.ExitCode != nil {
		successExitCode = *success.loaded.Process.ExitCode
	}
	router := newAgySharedCommandRouter()
	if err := router.register(agySharedSuccessSelector, fixture.successDir, newAgyStaticCommandRunner(platformprocess.CommandResult{
		Stdout: append([]byte(nil), success.loaded.Stdout.Raw...),
		Stderr: []byte(success.loaded.Stderr), ExitCode: successExitCode,
	})); err != nil {
		return fmt.Errorf("register success route: %w", err)
	}
	if err := router.register(agySharedTimeoutSelector, fixture.timeoutDir, newAgyDeadlineExceededCommandRunner(append([]byte(nil), timeout.loaded.Stdout.Raw...))); err != nil {
		return fmt.Errorf("register timeout route: %w", err)
	}
	if err := assertAgyRoutesRejectInvalidRegistrations(router, fixture.rootDir, fixture.successDir); err != nil {
		return err
	}
	if err := router.freeze(); err != nil {
		return fmt.Errorf("freeze routes: %w", err)
	}
	fixture.router = router
	return nil
}

func (fixture *agyProcessFixture) startProcess() error {
	api := newAgySharedHTTPServer()
	process, err := support.BuildProcessWithContext(context.Background(), serviceedges.Edges{
		APIServerStarter:      api.start,
		ProviderCommandRunner: fixture.router,
		ProviderSessionResolveHomeDirectory: func() (string, error) {
			return fixture.homeDir, nil
		},
	})
	if err != nil {
		return fmt.Errorf("BuildProcess: %w", err)
	}
	fixture.processBuilds++
	fixture.process = process
	fixture.api = api
	inputs := support.FakeInputs(context.Background(), []string{
		"you", "run", "--dir", fixture.hostDir, "--continuously", "--with-server", "--server", "http://127.0.0.1:1", "--quiet", "--no-record",
	})
	inputs.Input.Env = []string{"HOME=" + fixture.homeDir, "USERPROFILE=" + fixture.homeDir}
	inputs.Input.WorkingDirectory = fixture.hostDir
	fixture.daemon = startAgySharedDaemon(process, inputs.Input)
	baseURL, err := api.server.WaitForBaseURL(agySharedScenarioTimeout)
	if err != nil {
		return fmt.Errorf("wait for shared API server: %w", err)
	}
	fixture.baseURL = baseURL
	return nil
}

func (fixture *agyProcessFixture) waitForReady(t *testing.T) error {
	// The production server reports readiness asynchronously after Process.Execute
	// starts; this bounded public status observation cannot be replaced by the
	// controlled command edge because it proves the real HTTP/session boundary.
	t.Helper()
	return waitForAgyRuntimeReady(fixture.baseURL, agySharedScenarioTimeout)
}

func (fixture *agyProcessFixture) scenario(t *testing.T, caseName string) *agySharedScenario {
	t.Helper()
	scenario, ok := fixture.scenarios[caseName]
	if !ok {
		t.Fatalf("shared AGY scenario %q is not registered", caseName)
	}
	return scenario
}

func (fixture *agyProcessFixture) runScenario(
	t *testing.T,
	scenario *agySharedScenario,
	workTitle string,
) agySharedReplay {
	t.Helper()
	opened := support.OpenFactorySessionAt(t, fixture.baseURL, scenario.factoryDir)
	if opened.Session == nil {
		t.Fatalf("AGY %q open response missing session: %#v", scenario.selector, opened)
	}
	session := *opened.Session
	if strings.TrimSpace(session.Id) == "" || session.Id == factorysessions.DefaultSessionID || session.IsDefault {
		t.Fatalf("AGY %q session = %#v, want unique non-default explicit session", scenario.selector, session)
	}
	if session.FolderPath != scenario.factoryDir || session.FactoryDir != scenario.factoryDir {
		t.Fatalf("AGY %q session paths = folder:%q factory:%q, want %q", scenario.selector, session.FolderPath, session.FactoryDir, scenario.factoryDir)
	}
	if err := fixture.recordSessionOpened(session.Id); err != nil {
		t.Fatalf("AGY %q session identity: %v", scenario.selector, err)
	}
	run := &agySharedScenarioRun{fixture: fixture, sessionID: session.Id}
	fixture.recordRun(run)
	t.Cleanup(func() { run.close(t) })

	stream := support.OpenFactoryResponseEventStreamAt(
		t,
		support.SessionResponseEventsURL(fixture.baseURL, session.Id),
	)
	run.stream = stream
	fixture.recordStreamOpened()

	routeRequestStart := fixture.router.requestCount()
	name := workTitle
	submitted := support.SubmitSessionWorkAt(t, fixture.baseURL, session.Id, factoryapi.SubmitWorkRequest{
		Name:         &name,
		WorkTypeName: "task",
		Payload:      map[string]string{"title": workTitle},
	})
	if submitted.SessionId == nil || *submitted.SessionId != session.Id {
		t.Fatalf("AGY %q submitted Work session ID = %#v, want %q", scenario.selector, submitted.SessionId, session.Id)
	}
	if strings.TrimSpace(support.StringPointerValue(submitted.WorkId)) == "" || strings.TrimSpace(submitted.RequestId) == "" {
		t.Fatalf("AGY %q submitted Work identity = %#v, want Work and request IDs", scenario.selector, submitted)
	}
	responseEvents := readAgyResponseEvents(t, run, agySharedScenarioTimeout, scenario.selector)
	// The response stream is the hot production publication path. Wait for its
	// exact characterized frame set before polling the separate public status
	// endpoint so the observer cannot compete with runtime event publication.
	// The status read remains a required session-boundary witness before the
	// post-terminal Work and Factory Event snapshots.
	if _, err := waitForAgySessionTerminalStatus(context.Background(), fixture.baseURL, session.Id, agySharedScenarioTimeout); err != nil {
		t.Fatalf("AGY %q terminal session status: %v", scenario.selector, err)
	}
	run.terminalObserved = true
	listed, factoryEvents, err := readAgySessionProjections(
		context.Background(), fixture.baseURL, session.Id, agySharedScenarioTimeout,
	)
	if err != nil {
		t.Fatalf("AGY %q session projections: %v", scenario.selector, err)
	}
	// Deletion followed by normal EOF proves no frame was hidden.
	assertAgyResponseEventStreamClosed(t, run, agySharedScenarioTimeout, scenario.selector, len(responseEvents))
	assertAgySessionObservations(t, scenario, session.Id, submitted, factoryEvents, responseEvents)
	routeCalls := fixture.assertRouteRequests(t, scenario, routeRequestStart)

	run.close(t)
	return agySharedReplay{
		Session:        session,
		Submitted:      submitted,
		Listed:         listed,
		FactoryEvents:  factoryEvents,
		ResponseEvents: responseEvents,
		RouteCalls:     routeCalls,
	}
}

type agySharedScenarioRun struct {
	fixture   *agyProcessFixture
	sessionID string
	stream    *support.FactoryResponseEventStream

	closeOnce sync.Once
	closeErr  error

	sessionOnce      sync.Once
	sessionCloseErr  error
	terminalObserved bool
	streamOnce       sync.Once
	streamState      sync.Mutex
	streamReadErr    error
	streamCloseErr   error
}

func (run *agySharedScenarioRun) close(t testing.TB) {
	t.Helper()
	if run == nil {
		return
	}
	if err := run.closeResources(); err != nil {
		t.Errorf("AGY scenario cleanup: %v", err)
	}
}

func (run *agySharedScenarioRun) closeResources() error {
	if run == nil {
		return nil
	}
	run.closeOnce.Do(func() {
		run.closeErr = run.closeResourcesOnce()
	})
	return run.closeErr
}

func (run *agySharedScenarioRun) closeResourcesOnce() error {
	cleanupCtx, cancel := context.WithTimeout(context.Background(), agySharedScenarioTimeout)
	defer cancel()
	var errs []error
	if err := run.closeSession(cleanupCtx); err != nil {
		errs = append(errs, err)
	}
	if err := run.closeStream(); err != nil {
		errs = append(errs, fmt.Errorf("close response stream for session %q: %w", run.sessionID, err))
	}
	if err := run.observedStreamError(); err != nil {
		errs = append(errs, fmt.Errorf("read response stream for session %q: %w", run.sessionID, err))
	}
	if strings.TrimSpace(run.sessionID) == "" {
		run.fixture.forgetRun(run.sessionID)
	}
	return errors.Join(errs...)
}

func (run *agySharedScenarioRun) closeSession(ctx context.Context) error {
	if run == nil || strings.TrimSpace(run.sessionID) == "" {
		return nil
	}
	run.sessionOnce.Do(func() {
		var errs []error
		if err := closeAgyFactorySession(ctx, run.fixture.baseURL, run.sessionID, run.terminalObserved); err != nil {
			errs = append(errs, fmt.Errorf("close Factory Session %q: %w", run.sessionID, err))
		}
		if err := assertAgyFactorySessionDeleted(run.fixture.baseURL, run.sessionID); err != nil {
			errs = append(errs, err)
		} else {
			run.fixture.recordSessionDeleted(run.sessionID)
			run.fixture.forgetRun(run.sessionID)
		}
		run.sessionCloseErr = errors.Join(errs...)
	})
	return run.sessionCloseErr
}

func (run *agySharedScenarioRun) closeStream() error {
	if run == nil || run.stream == nil {
		return nil
	}
	run.streamOnce.Do(func() {
		run.stream.Close()
		result := run.stream.TryNextFrameResult(time.Nanosecond)
		run.fixture.recordStreamClosed()
		switch result.Outcome {
		case support.FactoryResponseEventStreamOutcomeEOF,
			support.FactoryResponseEventStreamOutcomeCanceled:
			return
		case support.FactoryResponseEventStreamOutcomeReadError:
			run.streamCloseErr = result.Err
		default:
			run.streamCloseErr = fmt.Errorf("response stream close outcome = %q", result.Outcome)
		}
	})
	return run.streamCloseErr
}

func (run *agySharedScenarioRun) recordStreamReadResult(result support.FactoryResponseEventStreamWaitResult) {
	if run == nil || result.Err == nil {
		return
	}
	run.streamState.Lock()
	defer run.streamState.Unlock()
	if run.streamReadErr == nil {
		run.streamReadErr = result.Err
	}
}

func (run *agySharedScenarioRun) observedStreamError() error {
	if run == nil {
		return nil
	}
	run.streamState.Lock()
	defer run.streamState.Unlock()
	return run.streamReadErr
}

func (fixture *agyProcessFixture) recordRun(run *agySharedScenarioRun) {
	fixture.sessionMu.Lock()
	defer fixture.sessionMu.Unlock()
	if fixture.activeSessions == nil {
		fixture.activeSessions = make(map[string]struct{})
	}
	if fixture.activeRuns == nil {
		fixture.activeRuns = make(map[string]*agySharedScenarioRun)
	}
	fixture.activeSessions[run.sessionID] = struct{}{}
	fixture.activeRuns[run.sessionID] = run
}

func (fixture *agyProcessFixture) forgetRun(sessionID string) {
	fixture.sessionMu.Lock()
	defer fixture.sessionMu.Unlock()
	delete(fixture.activeSessions, sessionID)
	delete(fixture.activeRuns, sessionID)
}

func (fixture *agyProcessFixture) recordSessionOpened(sessionID string) error {
	fixture.sessionMu.Lock()
	defer fixture.sessionMu.Unlock()
	for _, existing := range fixture.openedSessionIDs {
		if existing == sessionID {
			return fmt.Errorf("session ID %q was reused", sessionID)
		}
	}
	fixture.openedSessionIDs = append(fixture.openedSessionIDs, sessionID)
	return nil
}

func (fixture *agyProcessFixture) recordSessionDeleted(sessionID string) {
	fixture.sessionMu.Lock()
	defer fixture.sessionMu.Unlock()
	if fixture.deletedSessionID == nil {
		fixture.deletedSessionID = make(map[string]struct{})
	}
	fixture.deletedSessionID[sessionID] = struct{}{}
	delete(fixture.activeSessions, sessionID)
	delete(fixture.activeRuns, sessionID)
}

func (fixture *agyProcessFixture) recordStreamOpened() {
	fixture.streamMu.Lock()
	fixture.streamsOpened++
	fixture.streamMu.Unlock()
}

func (fixture *agyProcessFixture) recordStreamClosed() {
	fixture.streamMu.Lock()
	fixture.streamsClosed++
	fixture.streamMu.Unlock()
}

func (fixture *agyProcessFixture) assertProcessTopology(t *testing.T) {
	t.Helper()
	if fixture.processBuilds != 1 || fixture.api.startCount() != 1 {
		t.Fatalf("AGY shared process topology = root:%d http:%d, want one each", fixture.processBuilds, fixture.api.startCount())
	}
	if got := fixture.router.routeCount(); got != 2 {
		t.Fatalf("AGY active route count = %d, want two immutable routes", got)
	}
	if got := fixture.router.activeCallCount(); got != 0 {
		t.Fatalf("AGY active command calls = %d, want zero after scenario", got)
	}
	fixture.sessionMu.Lock()
	opened := len(fixture.openedSessionIDs)
	deleted := len(fixture.deletedSessionID)
	active := len(fixture.activeSessions)
	runs := len(fixture.activeRuns)
	fixture.sessionMu.Unlock()
	fixture.streamMu.Lock()
	streamsOpened, streamsClosed := fixture.streamsOpened, fixture.streamsClosed
	fixture.streamMu.Unlock()
	if opened != deleted || active != 0 || runs != 0 || streamsOpened != streamsClosed {
		t.Fatalf("AGY scenario cleanup = sessions opened:%d deleted:%d active:%d runs:%d streams opened:%d closed:%d", opened, deleted, active, runs, streamsOpened, streamsClosed)
	}
}

func (fixture *agyProcessFixture) assertRouteRequests(t testing.TB, scenario *agySharedScenario, start int) int {
	t.Helper()
	want := 1
	if scenario.selector == agySharedTimeoutSelector {
		want = 9
	}
	requests := fixture.router.requestsSince(start)
	if len(requests) != want {
		t.Fatalf("AGY %q routed requests = %d, want %d", scenario.selector, len(requests), want)
	}
	for index, request := range requests {
		if request.Command != agySharedCommand || request.WorkDir != scenario.factoryDir {
			t.Fatalf("AGY %q routed request[%d] = command:%q workdir:%q, want command:%q workdir:%q", scenario.selector, index, request.Command, request.WorkDir, agySharedCommand, scenario.factoryDir)
		}
	}
	return len(requests)
}

func (fixture *agyProcessFixture) finalize() error {
	fixture.finalOnce.Do(func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), agySharedScenarioTimeout)
		defer cancel()
		fixture.finalErr = errors.Join(fixture.setupErr, fixture.cleanupResources(closeCtx))
	})
	return fixture.finalErr
}

type agyCleanupOperations struct {
	closeSessions func(context.Context) error
	stopDaemon    func(context.Context) error
	closeProcess  func(context.Context) error
	waitForAPI    func(context.Context) error
	checkListener func() error
	checkActivity func() error
	releaseRoutes func() error
	checkRoutes   func() error
	checkCensus   func() error
	removeRoot    func() error
}

func cleanupAgyResources(ctx context.Context, primary error, operations agyCleanupOperations) error {
	errs := []error{primary}
	cleanup := func(label string, operation func() error) {
		if operation == nil {
			return
		}
		if err := operation(); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", label, err))
		}
	}
	cleanupContext := func(label string, operation func(context.Context) error) {
		if operation == nil {
			return
		}
		cleanup(label, func() error { return operation(ctx) })
	}
	cleanupContext("close sessions", operations.closeSessions)
	cleanupContext("stop daemon", operations.stopDaemon)
	cleanupContext("close process", operations.closeProcess)
	cleanupContext("wait for API shutdown", operations.waitForAPI)
	cleanup("check listener", operations.checkListener)
	cleanup("check command activity", operations.checkActivity)
	cleanup("release routes", operations.releaseRoutes)
	cleanup("check routes", operations.checkRoutes)
	cleanup("check cleanup census", operations.checkCensus)
	cleanup("remove fixture root", operations.removeRoot)
	return errors.Join(errs...)
}

func (fixture *agyProcessFixture) cleanupResources(ctx context.Context) error {
	return cleanupAgyResources(ctx, nil, agyCleanupOperations{
		closeSessions: fixture.closeUnclosedSessions,
		stopDaemon: func(ctx context.Context) error {
			if fixture.daemon == nil {
				return nil
			}
			return fixture.daemon.stop(ctx)
		},
		closeProcess: func(ctx context.Context) error {
			if fixture.process == nil {
				return nil
			}
			return fixture.process.Close(ctx)
		},
		waitForAPI: func(ctx context.Context) error {
			if fixture.api == nil {
				return nil
			}
			return fixture.api.waitClosed(ctx)
		},
		checkListener: func() error {
			return assertAgyListenerClosed(fixture.baseURL)
		},
		checkActivity: func() error {
			if fixture.router == nil {
				return nil
			}
			if got := fixture.router.activeCallCount(); got != 0 {
				return fmt.Errorf("active command calls after cleanup = %d, want zero", got)
			}
			return nil
		},
		releaseRoutes: func() error {
			if fixture.router == nil {
				return nil
			}
			return fixture.router.releaseAll()
		},
		checkRoutes: func() error {
			if fixture.router == nil {
				return nil
			}
			if got := fixture.router.routeCount(); got != 0 {
				return fmt.Errorf("route count after cleanup = %d, want zero", got)
			}
			return nil
		},
		checkCensus: fixture.checkCleanupCensus,
		removeRoot:  func() error { return removeAgyFixtureRoot(fixture.rootDir) },
	})
}

func (fixture *agyProcessFixture) checkCleanupCensus() error {
	fixture.sessionMu.Lock()
	activeSessions := len(fixture.activeSessions)
	activeRuns := len(fixture.activeRuns)
	opened := len(fixture.openedSessionIDs)
	deleted := len(fixture.deletedSessionID)
	fixture.sessionMu.Unlock()
	fixture.streamMu.Lock()
	streamsOpened, streamsClosed := fixture.streamsOpened, fixture.streamsClosed
	fixture.streamMu.Unlock()
	if activeSessions != 0 || activeRuns != 0 || opened != deleted || streamsOpened != streamsClosed {
		return fmt.Errorf("sessions opened:%d deleted:%d active:%d runs:%d streams opened:%d closed:%d", opened, deleted, activeSessions, activeRuns, streamsOpened, streamsClosed)
	}
	return nil
}

func removeAgyFixtureRoot(rootDir string) error {
	if strings.TrimSpace(rootDir) == "" {
		return nil
	}
	if err := os.RemoveAll(rootDir); err != nil {
		return err
	}
	if _, err := os.Stat(rootDir); !os.IsNotExist(err) {
		return fmt.Errorf("fixture root remains after cleanup: %v", err)
	}
	return nil
}

func (fixture *agyProcessFixture) closeUnclosedSessions(ctx context.Context) error {
	fixture.sessionMu.Lock()
	ids := make([]string, 0, len(fixture.activeSessions))
	for id := range fixture.activeSessions {
		ids = append(ids, id)
	}
	runs := make(map[string]*agySharedScenarioRun, len(fixture.activeRuns))
	for id, run := range fixture.activeRuns {
		runs[id] = run
	}
	fixture.sessionMu.Unlock()
	if len(ids) == 0 {
		return nil
	}
	var errs []error
	for _, id := range ids {
		if run := runs[id]; run != nil {
			if err := run.closeSession(ctx); err != nil {
				errs = append(errs, fmt.Errorf("close unclosed session %q: %w", id, err))
			}
			if err := run.closeStream(); err != nil {
				errs = append(errs, fmt.Errorf("close response stream for unclosed session %q: %w", id, err))
			}
			if err := run.observedStreamError(); err != nil {
				errs = append(errs, fmt.Errorf("read response stream for unclosed session %q: %w", id, err))
			}
			continue
		}
		if err := closeAgyFactorySession(ctx, fixture.baseURL, id, false); err != nil {
			errs = append(errs, fmt.Errorf("close unclosed session %q: %w", id, err))
			continue
		}
		if err := assertAgyFactorySessionDeleted(fixture.baseURL, id); err != nil {
			errs = append(errs, err)
			continue
		}
		fixture.recordSessionDeleted(id)
		fixture.forgetRun(id)
	}
	return errors.Join(errs...)
}

func assertAgyResponseEventTopology(t testing.TB, selector string, events []factoryapi.FactoryResponseEvent) {
	t.Helper()
	if selector == agySharedSuccessSelector {
		if len(events) != 2 || events[0].Kind != factoryapi.FactoryResponseEventKindRun ||
			events[0].Phase != factoryapi.FactoryResponseEventPhaseCompleted ||
			events[1].Kind != factoryapi.FactoryResponseEventKindMessage ||
			events[1].Phase != factoryapi.FactoryResponseEventPhaseCompleted ||
			events[0].RunId != events[1].RunId {
			t.Fatalf("AGY success response events = %#v, want completed RUN then message on one run", events)
		}
		return
	}
	if len(events) != 18 {
		t.Fatalf("AGY timeout response event count = %d, want 18", len(events))
	}
	runs := make(map[string]int)
	for index := 0; index < len(events); index += 2 {
		message, failure := events[index], events[index+1]
		if message.Kind != factoryapi.FactoryResponseEventKindMessage ||
			message.Phase != factoryapi.FactoryResponseEventPhaseCompleted ||
			failure.Kind != factoryapi.FactoryResponseEventKindError ||
			failure.Phase != factoryapi.FactoryResponseEventPhaseFailed ||
			message.RunId == "" || message.RunId != failure.RunId {
			t.Fatalf("AGY timeout response pair[%d] = %#v/%#v, want completed message then failed error on one run", index/2, message, failure)
		}
		runs[message.RunId]++
	}
	if len(runs) != 3 {
		t.Fatalf("AGY timeout response run IDs = %d, want three; counts=%v", len(runs), runs)
	}
	for runID, count := range runs {
		if count != 3 {
			t.Fatalf("AGY timeout run %q response pairs = %d, want three", runID, count)
		}
	}
}

func assertAgyListenerClosed(baseURL string) error {
	if strings.TrimSpace(baseURL) == "" {
		return nil
	}
	client := &http.Client{Timeout: 250 * time.Millisecond}
	defer client.CloseIdleConnections()
	response, err := client.Get(strings.TrimSuffix(baseURL, "/") + "/status")
	if err != nil {
		return nil
	}
	defer response.Body.Close()
	body, readErr := io.ReadAll(response.Body)
	return fmt.Errorf("AGY listener remains reachable after cleanup: status=%d body=%q readError=%v", response.StatusCode, strings.TrimSpace(string(body)), readErr)
}
