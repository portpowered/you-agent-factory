package agy

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
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	agySharedScenarioTimeout = 30 * time.Second
	agySharedSuccessSelector = "agy-final-only-success"
	agySharedTimeoutSelector = "agy-timeout"
	agySharedCommand         = "agy"
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

func (fixture *agyProcessFixture) setup(t *testing.T) error {
	t.Helper()
	rootDir, err := os.MkdirTemp("", "infinite-you-agy-shared-")
	if err != nil {
		return fmt.Errorf("create package fixture root: %w", err)
	}
	fixture.rootDir = rootDir
	cleanupOnError := true
	defer func() {
		if cleanupOnError {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), agySharedScenarioTimeout)
			defer cancel()
			if fixture.daemon != nil {
				_ = fixture.daemon.stop(cleanupCtx)
			}
			if fixture.process != nil {
				_ = fixture.process.Close(cleanupCtx)
			}
			if fixture.api != nil {
				_ = fixture.api.waitClosed(cleanupCtx)
			}
			_ = os.RemoveAll(rootDir)
		}
	}()

	fixture.homeDir = filepath.Join(rootDir, "home")
	fixture.hostDir = filepath.Join(rootDir, "host")
	fixture.successDir = filepath.Join(rootDir, "success")
	fixture.timeoutDir = filepath.Join(rootDir, "timeout")
	for _, path := range []string{fixture.homeDir, fixture.hostDir, fixture.successDir, fixture.timeoutDir} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			return fmt.Errorf("create package fixture path %q: %w", path, err)
		}
	}

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
	legacyDir := support.LegacyFixtureDir(t, "executor_success")
	for _, path := range []string{fixture.hostDir, fixture.successDir, fixture.timeoutDir} {
		if err := copyAgyFactoryDirectory(legacyDir, path); err != nil {
			return fmt.Errorf("copy legacy fixture to %q: %w", path, err)
		}
		if err := os.RemoveAll(filepath.Join(path, "inputs")); err != nil {
			return fmt.Errorf("clear seed inputs in %q: %w", path, err)
		}
	}
	if err := writeAgyWorkerConfig(fixture.hostDir, successRequest.Model); err != nil {
		return err
	}
	if err := writeAgyWorkerConfig(fixture.successDir, successRequest.Model); err != nil {
		return err
	}
	if err := writeAgyWorkerConfig(fixture.timeoutDir, timeoutRequest.Model); err != nil {
		return err
	}

	successExitCode := 0
	if successLoaded.Process.ExitCode != nil {
		successExitCode = *successLoaded.Process.ExitCode
	}
	successRunner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{
		Stdout:   append([]byte(nil), successLoaded.Stdout.Raw...),
		Stderr:   []byte(successLoaded.Stderr),
		ExitCode: successExitCode,
	})
	timeoutRunner := newAgyDeadlineExceededCommandRunner(append([]byte(nil), timeoutLoaded.Stdout.Raw...))
	router := newAgySharedCommandRouter()
	if err := router.register(agySharedSuccessSelector, fixture.successDir, successRunner); err != nil {
		return fmt.Errorf("register success route: %w", err)
	}
	if err := router.register(agySharedTimeoutSelector, fixture.timeoutDir, timeoutRunner); err != nil {
		return fmt.Errorf("register timeout route: %w", err)
	}
	if err := assertAgyRoutesRejectInvalidRegistrations(router, fixture.rootDir, fixture.successDir); err != nil {
		return err
	}
	if err := router.freeze(); err != nil {
		return fmt.Errorf("freeze routes: %w", err)
	}
	fixture.router = router
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

	api := newAgySharedHTTPServer()
	process, err := support.BuildProcessWithContext(context.Background(), serviceedges.Edges{
		APIServerStarter:      api.start,
		ProviderCommandRunner: router,
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
	support.WaitForStatus(t, fixture.baseURL, agySharedScenarioTimeout, func(status factoryapi.StatusResponse) bool {
		return strings.TrimSpace(status.RuntimeStatus) != ""
	})
	cleanupOnError = false
	return nil
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
	support.WaitForSessionTerminalStatus(t, fixture.baseURL, session.Id, agySharedScenarioTimeout)

	listed := listAgySessionWork(t, fixture.baseURL, session.Id)
	factoryEvents := support.GetFactoryEventsForSessionAt(t, fixture.baseURL, session.Id)
	responseEvents := readAgyResponseEvents(t, stream, agySharedScenarioTimeout, scenario.selector)
	assertAgySessionObservations(t, scenario, session.Id, submitted, factoryEvents, responseEvents)
	fixture.assertRouteRequests(t, scenario)

	run.close(t)
	return agySharedReplay{
		Session:        session,
		Submitted:      submitted,
		Listed:         listed,
		FactoryEvents:  factoryEvents,
		ResponseEvents: responseEvents,
	}
}

type agySharedScenarioRun struct {
	fixture   *agyProcessFixture
	sessionID string
	stream    *support.FactoryResponseEventStream
	once      sync.Once
}

func (run *agySharedScenarioRun) close(t testing.TB) {
	if run == nil {
		return
	}
	run.once.Do(func() {
		if strings.TrimSpace(run.sessionID) != "" {
			support.CloseFactorySessionAt(t, run.fixture.baseURL, run.sessionID)
			assertAgyFactorySessionDeleted(t, run.fixture.baseURL, run.sessionID)
			run.fixture.recordSessionDeleted(run.sessionID)
		}
		if run.stream != nil {
			run.stream.Close()
			run.stream.WaitClosed(agySharedScenarioTimeout)
			run.fixture.recordStreamClosed()
		}
		run.fixture.forgetRun(run.sessionID)
	})
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

func (fixture *agyProcessFixture) assertRouteRequests(t testing.TB, scenario *agySharedScenario) {
	t.Helper()
	want := 1
	if scenario.selector == agySharedTimeoutSelector {
		want = 9
	}
	requests := fixture.router.requestsForSelector(scenario.selector)
	if len(requests) != want {
		t.Fatalf("AGY %q routed requests = %d, want %d", scenario.selector, len(requests), want)
	}
	for index, request := range requests {
		if request.Command != agySharedCommand || request.WorkDir != scenario.factoryDir {
			t.Fatalf("AGY %q routed request[%d] = command:%q workdir:%q, want command:%q workdir:%q", scenario.selector, index, request.Command, request.WorkDir, agySharedCommand, scenario.factoryDir)
		}
	}
}

func (fixture *agyProcessFixture) finalize() error {
	fixture.finalOnce.Do(func() {
		var errs []error
		if fixture.setupErr != nil {
			fixture.finalErr = fixture.setupErr
			return
		}
		if fixture.process == nil {
			return
		}
		closeCtx, cancel := context.WithTimeout(context.Background(), agySharedScenarioTimeout)
		defer cancel()
		if err := fixture.closeUnclosedSessions(closeCtx); err != nil {
			errs = append(errs, err)
		}
		if err := fixture.daemon.stop(closeCtx); err != nil {
			errs = append(errs, fmt.Errorf("stop daemon: %w", err))
		}
		if err := fixture.process.Close(closeCtx); err != nil {
			errs = append(errs, fmt.Errorf("close process: %w", err))
		}
		if err := fixture.api.waitClosed(closeCtx); err != nil {
			errs = append(errs, fmt.Errorf("wait for API shutdown: %w", err))
		}
		if err := assertAgyListenerClosed(fixture.baseURL); err != nil {
			errs = append(errs, err)
		}
		if got := fixture.router.activeCallCount(); got != 0 {
			errs = append(errs, fmt.Errorf("active command calls after cleanup = %d, want zero", got))
		}
		if err := fixture.router.releaseAll(); err != nil {
			errs = append(errs, fmt.Errorf("release routes: %w", err))
		}
		if got := fixture.router.routeCount(); got != 0 {
			errs = append(errs, fmt.Errorf("route count after cleanup = %d, want zero", got))
		}
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
			errs = append(errs, fmt.Errorf("cleanup census sessions opened:%d deleted:%d active:%d runs:%d streams opened:%d closed:%d", opened, deleted, activeSessions, activeRuns, streamsOpened, streamsClosed))
		}
		if err := os.RemoveAll(fixture.rootDir); err != nil {
			errs = append(errs, fmt.Errorf("remove fixture root: %w", err))
		} else if _, err := os.Stat(fixture.rootDir); !os.IsNotExist(err) {
			errs = append(errs, fmt.Errorf("fixture root remains after cleanup: %v", err))
		}
		fixture.finalErr = errors.Join(errs...)
	})
	return fixture.finalErr
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
		if run := runs[id]; run != nil && run.stream != nil {
			run.stream.Close()
			fixture.recordStreamClosed()
		}
		if err := closeAgyFactorySession(ctx, fixture.baseURL, id); err != nil {
			errs = append(errs, fmt.Errorf("close unclosed session %q: %w", id, err))
			continue
		}
		fixture.recordSessionDeleted(id)
	}
	return errors.Join(errs...)
}

func writeAgyWorkerConfig(factoryDir, model string) error {
	config := strings.Replace(
		support.BuildModelWorkerConfig(modelprovider.ProviderAntigravity, model),
		"stopToken: COMPLETE",
		"skipPermissions: true\nstopToken: COMPLETE",
		1,
	)
	path := filepath.Join(factoryDir, "workers", "worker", "AGENTS.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create AGY worker config directory: %w", err)
	}
	if err := os.WriteFile(path, []byte(config), 0o644); err != nil {
		return fmt.Errorf("write AGY worker config: %w", err)
	}
	return nil
}

func copyAgyFactoryDirectory(sourceDir, targetDir string) error {
	return filepath.WalkDir(sourceDir, func(path string, entry os.DirEntry, walkErr error) error {
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
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return os.MkdirAll(targetPath, info.Mode().Perm())
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(targetPath, data, info.Mode().Perm())
	})
}

func readAgyResponseEvents(
	t testing.TB,
	stream *support.FactoryResponseEventStream,
	timeout time.Duration,
	selector string,
) []factoryapi.FactoryResponseEvent {
	t.Helper()
	want := 2
	if selector == agySharedTimeoutSelector {
		want = 18
	}
	events := make([]factoryapi.FactoryResponseEvent, 0)
	for len(events) < want {
		frame, ok := stream.TryNextFrame(timeout)
		if !ok {
			break
		}
		events = append(events, frame.Event)
	}
	return events
}

func assertAgySessionObservations(
	t *testing.T,
	scenario *agySharedScenario,
	sessionID string,
	submitted factoryapi.SubmitWorkResponse,
	factoryEvents []factoryapi.FactoryEvent,
	responseEvents []factoryapi.FactoryResponseEvent,
) {
	t.Helper()
	workID := support.StringPointerValue(submitted.WorkId)
	if workID == "" {
		t.Fatalf("AGY %q submitted Work has no ID", scenario.selector)
	}
	support.AssertSingleWorkRequestEvent(t, factoryEvents, submitted.RequestId, workID, "task")
	assertAgyFactoryEventSequence(t, scenario.selector, factoryEvents)
	assertAgyResponseEventTopology(t, scenario.selector, responseEvents)
	for _, event := range factoryEvents {
		if event.Context.SessionId != nil && *event.Context.SessionId != sessionID {
			t.Fatalf("AGY %q Factory Event scope = %#v, want session %q", scenario.selector, event.Context, sessionID)
		}
	}
	for _, event := range responseEvents {
		if event.FactorySessionId != sessionID {
			t.Fatalf("AGY %q response event session = %q, want %q", scenario.selector, event.FactorySessionId, sessionID)
		}
		if strings.TrimSpace(event.RunId) == "" {
			t.Fatalf("AGY %q response event = %#v, want run identity", scenario.selector, event)
		}
	}
}

func assertAgyFactoryEventSequence(t testing.TB, selector string, events []factoryapi.FactoryEvent) {
	t.Helper()
	want := []factoryapi.FactoryEventType{
		factoryapi.FactoryEventTypeRunRequest,
		factoryapi.FactoryEventTypeInitialStructureRequest,
		factoryapi.FactoryEventTypeSessionStarted,
		factoryapi.FactoryEventTypeFactoryStateResponse,
		factoryapi.FactoryEventTypeWorkRequest,
	}
	attempt := []factoryapi.FactoryEventType{
		factoryapi.FactoryEventTypeDispatchRequest,
		factoryapi.FactoryEventTypeDispatchWorkerSessionAssociation,
		factoryapi.FactoryEventTypeModelRequest,
		factoryapi.FactoryEventTypeModelResponse,
		factoryapi.FactoryEventTypeAgentRunResponse,
		factoryapi.FactoryEventTypeDispatchResponse,
	}
	if selector == agySharedTimeoutSelector {
		for i := 0; i < 3; i++ {
			want = append(want, attempt...)
		}
	} else {
		want = append(want, attempt...)
	}
	if len(events) != len(want) {
		t.Fatalf("AGY %q Factory Event count = %d, want %d; types=%v", selector, len(events), len(want), agyFactoryEventTypes(events))
	}
	for index, event := range events {
		if event.Type != want[index] {
			t.Fatalf("AGY %q Factory Event[%d] = %q, want %q; types=%v", selector, index, event.Type, want[index], agyFactoryEventTypes(events))
		}
	}
}

func agyFactoryEventTypes(events []factoryapi.FactoryEvent) []factoryapi.FactoryEventType {
	types := make([]factoryapi.FactoryEventType, 0, len(events))
	for _, event := range events {
		types = append(types, event.Type)
	}
	return types
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

func listAgySessionWork(t testing.TB, baseURL, sessionID string) factoryapi.ListWorkResponse {
	t.Helper()
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID) + "/work"
	return support.GetJSON[factoryapi.ListWorkResponse](t, endpoint)
}

func assertAgyFactorySessionDeleted(t testing.TB, baseURL, sessionID string) {
	t.Helper()
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID)
	response, err := http.Get(endpoint)
	if err != nil {
		t.Fatalf("GET deleted AGY Factory Session %q: %v", sessionID, err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("GET deleted AGY Factory Session %q status = %d, want 404: %s", sessionID, response.StatusCode, strings.TrimSpace(string(body)))
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

func closeAgyFactorySession(ctx context.Context, baseURL, sessionID string) error {
	client := &http.Client{}
	defer client.CloseIdleConnections()
	cleanupCtx, cancel := context.WithTimeout(ctx, agySharedScenarioTimeout)
	defer cancel()
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID)
	terminateEndpoint := endpoint + "/terminate"
	request, err := http.NewRequestWithContext(cleanupCtx, http.MethodPost, terminateEndpoint, strings.NewReader("{}"))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	body, readErr := io.ReadAll(response.Body)
	response.Body.Close()
	if readErr != nil {
		return readErr
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		if response.StatusCode != http.StatusConflict || !strings.Contains(string(body), `"outcome":"TERMINAL_SESSION"`) {
			return fmt.Errorf("terminate status=%d body=%q", response.StatusCode, strings.TrimSpace(string(body)))
		}
	}

	poll := time.NewTicker(10 * time.Millisecond)
	defer poll.Stop()
	for {
		statusRequest, err := http.NewRequestWithContext(cleanupCtx, http.MethodGet, endpoint+"/status", nil)
		if err != nil {
			return err
		}
		statusResponse, err := client.Do(statusRequest)
		if err == nil {
			statusBody, bodyErr := io.ReadAll(statusResponse.Body)
			statusResponse.Body.Close()
			if bodyErr == nil && statusResponse.StatusCode == http.StatusOK {
				var status factoryapi.StatusResponse
				if json.Unmarshal(statusBody, &status) == nil &&
					(status.RuntimeStatus == "IDLE" || status.RuntimeStatus == "FINISHED") {
					break
				}
			}
		}
		select {
		case <-cleanupCtx.Done():
			return cleanupCtx.Err()
		case <-poll.C:
		}
	}

	deleteRequest, err := http.NewRequestWithContext(cleanupCtx, http.MethodDelete, endpoint, nil)
	if err != nil {
		return err
	}
	deleteResponse, err := client.Do(deleteRequest)
	if err != nil {
		return err
	}
	deleteBody, readErr := io.ReadAll(deleteResponse.Body)
	deleteResponse.Body.Close()
	if readErr != nil {
		return readErr
	}
	if deleteResponse.StatusCode != http.StatusNoContent && deleteResponse.StatusCode != http.StatusNotFound {
		return fmt.Errorf("delete status=%d body=%q", deleteResponse.StatusCode, strings.TrimSpace(string(deleteBody)))
	}
	return nil
}

type agySharedCommandRoute struct {
	selector string
	workDir  string
	runner   platformprocess.CommandRunner
}

// agySharedCommandRouter is immutable after freeze. WorkDir is the only
// selector used during execution, so no test-order or mutable scenario state
// can redirect a provider call to the other golden.
type agySharedCommandRouter struct {
	mu       sync.Mutex
	routes   map[string]agySharedCommandRoute
	requests []platformprocess.CommandRequest
	active   int
	frozen   bool
	released bool
}

func newAgySharedCommandRouter() *agySharedCommandRouter {
	return &agySharedCommandRouter{routes: make(map[string]agySharedCommandRoute)}
}

func (router *agySharedCommandRouter) register(selector, workDir string, runner platformprocess.CommandRunner) error {
	selector = strings.TrimSpace(selector)
	workDir = strings.TrimSpace(workDir)
	if selector == "" || workDir == "" || runner == nil {
		return fmt.Errorf("AGY route selector, WorkDir, and runner are required")
	}
	absolute, err := filepath.Abs(filepath.Clean(workDir))
	if err != nil {
		return fmt.Errorf("normalize AGY route WorkDir: %w", err)
	}
	router.mu.Lock()
	defer router.mu.Unlock()
	if router.frozen {
		return fmt.Errorf("AGY routes are already frozen")
	}
	if router.released {
		return fmt.Errorf("AGY routes have been released")
	}
	if _, exists := router.routes[absolute]; exists {
		return fmt.Errorf("AGY WorkDir route %q is already registered", absolute)
	}
	for _, route := range router.routes {
		if route.selector == selector {
			return fmt.Errorf("AGY route selector %q is already registered", selector)
		}
	}
	router.routes[absolute] = agySharedCommandRoute{selector: selector, workDir: absolute, runner: runner}
	return nil
}

func (router *agySharedCommandRouter) freeze() error {
	router.mu.Lock()
	defer router.mu.Unlock()
	if router.released {
		return fmt.Errorf("AGY routes have been released")
	}
	if len(router.routes) == 0 {
		return fmt.Errorf("AGY route table is empty")
	}
	router.frozen = true
	return nil
}

func (router *agySharedCommandRouter) releaseAll() error {
	router.mu.Lock()
	defer router.mu.Unlock()
	if router.active != 0 {
		return fmt.Errorf("cannot release AGY routes with %d active calls", router.active)
	}
	router.routes = make(map[string]agySharedCommandRoute)
	router.released = true
	return nil
}

func (router *agySharedCommandRouter) routeCount() int {
	router.mu.Lock()
	defer router.mu.Unlock()
	return len(router.routes)
}

func (router *agySharedCommandRouter) activeCallCount() int {
	router.mu.Lock()
	defer router.mu.Unlock()
	return router.active
}

func (router *agySharedCommandRouter) routeCallCount(selector string) int {
	router.mu.Lock()
	defer router.mu.Unlock()
	count := 0
	for _, request := range router.requests {
		if request.Command == agySharedCommand && request.WorkDir == router.workDirForSelectorLocked(selector) {
			count++
		}
	}
	return count
}

func (router *agySharedCommandRouter) requestsForSelector(selector string) []platformprocess.CommandRequest {
	router.mu.Lock()
	defer router.mu.Unlock()
	workDir := router.workDirForSelectorLocked(selector)
	requests := make([]platformprocess.CommandRequest, 0)
	for _, request := range router.requests {
		if request.WorkDir == workDir {
			requests = append(requests, cloneAgyCommandRequest(request))
		}
	}
	return requests
}

func (router *agySharedCommandRouter) workDirForSelectorLocked(selector string) string {
	for workDir, route := range router.routes {
		if route.selector == selector {
			return workDir
		}
	}
	// Released routes are not queried by tests; returning a sentinel keeps the
	// method side-effect-free if a diagnostic is emitted during finalization.
	return "\x00"
}

func (router *agySharedCommandRouter) Run(
	ctx context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	rawWorkDir := strings.TrimSpace(request.WorkDir)
	if rawWorkDir == "" {
		return platformprocess.CommandResult{}, fmt.Errorf("AGY command WorkDir is required")
	}
	workDir, err := filepath.Abs(filepath.Clean(rawWorkDir))
	if err != nil {
		return platformprocess.CommandResult{}, fmt.Errorf("normalize AGY command WorkDir: %w", err)
	}
	router.mu.Lock()
	if router.released {
		router.mu.Unlock()
		return platformprocess.CommandResult{}, fmt.Errorf("AGY route table is released")
	}
	route, ok := router.routes[workDir]
	if !ok {
		router.mu.Unlock()
		return platformprocess.CommandResult{}, fmt.Errorf("no AGY route matched WorkDir %q", workDir)
	}
	if request.Command != agySharedCommand {
		router.mu.Unlock()
		return platformprocess.CommandResult{}, fmt.Errorf("AGY route %q received command %q", route.selector, request.Command)
	}
	if err := ctx.Err(); err != nil {
		router.mu.Unlock()
		return platformprocess.CommandResult{}, err
	}
	router.requests = append(router.requests, cloneAgyCommandRequest(request))
	router.active++
	router.mu.Unlock()
	defer func() {
		router.mu.Lock()
		router.active--
		router.mu.Unlock()
	}()
	return route.runner.Run(ctx, request)
}

func cloneAgyCommandRequest(request platformprocess.CommandRequest) platformprocess.CommandRequest {
	request.Args = append([]string(nil), request.Args...)
	request.Stdin = append([]byte(nil), request.Stdin...)
	request.Env = append([]string(nil), request.Env...)
	return request
}

func assertAgyRoutesRejectInvalidRegistrations(router *agySharedCommandRouter, rootDir, registeredWorkDir string) error {
	if err := router.register("", filepath.Join(rootDir, "invalid"), testutil.NewProviderCommandRunner()); err == nil {
		return fmt.Errorf("empty AGY selector was accepted")
	}
	if err := router.register("duplicate-workdir", registeredWorkDir, testutil.NewProviderCommandRunner()); err == nil {
		return fmt.Errorf("duplicate AGY WorkDir was accepted")
	}
	if err := router.register(agySharedSuccessSelector, filepath.Join(rootDir, "duplicate-selector"), testutil.NewProviderCommandRunner()); err == nil {
		return fmt.Errorf("duplicate AGY selector was accepted")
	}
	if got := router.routeCount(); got != 2 {
		return fmt.Errorf("AGY route count after invalid registration = %d, want two", got)
	}
	unknown := filepath.Join(rootDir, "unknown-route")
	_, err := router.Run(context.Background(), platformprocess.CommandRequest{
		Command: agySharedCommand,
		WorkDir: unknown,
		Stdin:   []byte("sensitive AGY input"),
		Env:     []string{"AGY_SECRET=sensitive"},
	})
	if err == nil {
		return fmt.Errorf("unknown AGY WorkDir was accepted")
	}
	if strings.Contains(err.Error(), "sensitive") {
		return fmt.Errorf("unknown AGY route diagnostic leaked sensitive input")
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = router.Run(canceled, platformprocess.CommandRequest{
		Command: agySharedCommand, WorkDir: rootDir,
	})
	if err == nil {
		return fmt.Errorf("canceled AGY command was accepted")
	}
	return nil
}
