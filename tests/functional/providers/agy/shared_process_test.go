package agy

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
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
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const agySharedInvocationTimeout = 30 * time.Second

const agySharedRouteCount = 29

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
	rootDir            string
	process            support.ApplicationProcess
	runner             *agySharedCommandRunner
	api                *agySharedHTTPServer
	routes             map[string]*agySharedCommandRoute
	processBuilds      int
	processCloses      int
	earlyExitRelease   chan struct{}
	concurrencyRelease chan struct{}
	roleHost           *agySharedRoleHost

	closeOnce sync.Once
	closeErr  error
}

type agySharedRoleHost struct {
	command   *support.ProcessCommand
	httpRun   *agySharedHTTPRun
	baseURL   string
	homeDir   string
	factories map[string]string
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

	fixture.registerDirectRoutes(t)
	fixture.registerGoldenRoutes(t)
	fixture.registerRoleRoutes(t)
	fixture.registerRecoveryRoute(t)
	fixture.registerEarlyExitRoute(t)
	fixture.registerConcurrencyRoutes(t)
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
	fixture.processBuilds++
	return fixture
}

func (fixture *agySharedProcessFixture) registerDirectRoutes(t *testing.T) {
	t.Helper()
	fixture.addDirectRoute(t, "direct-conductor-success", agySharedCommandOutcome{
		result: platformprocess.CommandResult{Stdout: []byte(`{"event":"result","result":{"conversation_id":"agy-conductor-success","status":"SUCCESS","response":"agy functional answer COMPLETE","duration_seconds":1.0,"num_turns":1,"usage":{"input_tokens":1,"output_tokens":1,"thinking_tokens":0,"cache_read_tokens":0,"total_tokens":2}}}` + "\n")},
	})
	fixture.addDirectRoute(t, "direct-native-failure", agySharedCommandOutcome{
		result: platformprocess.CommandResult{
			Stdout:   []byte("authentication failed: token path /tmp/secret-key leaked"),
			ExitCode: 1,
		},
	})
	fixture.addDirectRoute(t, "direct-timeout", agySharedCommandOutcome{err: context.DeadlineExceeded})
	fixture.addDirectRoute(t, "direct-cancellation", agySharedCommandOutcome{err: context.Canceled})
}

func (fixture *agySharedProcessFixture) addDirectRoute(
	t *testing.T,
	selector string,
	outcome agySharedCommandOutcome,
) {
	t.Helper()
	route := fixture.newRouteDirectories(t, selector)
	copyAgyDirectory(t, support.LegacyFixtureDir(t, "executor_success"), route.workDir)
	workerConfig := support.BuildModelWorkerConfig(modelprovider.ProviderAntigravity, agyFunctionalModel)
	if selector == "direct-conductor-success" {
		workerConfig = strings.Replace(workerConfig, "stopToken: COMPLETE", "skipPermissions: true\nstopToken: COMPLETE", 1)
	}
	support.WriteAgentConfig(t, route.workDir, "worker", workerConfig)
	testutil.WriteSeedFile(t, route.workDir, "task", []byte(`{"title":"agy shared direct"}`))
	registered, err := fixture.runner.registerOutcomes(selector, route.workDir, outcome)
	if err != nil {
		t.Fatalf("register shared AGY direct route %q: %v", selector, err)
	}
	registered.rootDir = route.rootDir
	registered.homeDir = route.homeDir
	fixture.routes[selector] = registered
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
	fixture.addGoldenRoute(
		t,
		"golden-missing-file",
		agyGoldenMissingPrompt,
		"",
		"",
		"agy-trace-missing-file.stream.jsonl",
	)
}

func (fixture *agySharedProcessFixture) registerRoleRoutes(t *testing.T) {
	t.Helper()
	fixture.registerRoleColdWatchRoutes(t)
	fixture.registerRoleClipQARoutes(t)
	fixture.registerRoleClipQAInvalidRoutes(t)
	fixture.registerRoleClipQAEdgeRoutes(t)
}

func (fixture *agySharedProcessFixture) registerRecoveryRoute(t *testing.T) {
	t.Helper()
	route := fixture.newRouteDirectories(t, "role-recovery-empty-then-valid")
	copyAgySharedAsset(t, route.workDir, "clip-fixture.mp4")
	first := agySharedCommandOutcome{result: platformprocess.CommandResult{ExitCode: 0}}
	second := agySharedCommandOutcome{
		result: platformprocess.CommandResult{Stdout: agyColdWatchCompleteReportTrace(t), ExitCode: 0},
	}
	registered, err := fixture.runner.registerOutcomes(
		"role-recovery-empty-then-valid", route.workDir, first, second,
	)
	if err != nil {
		t.Fatalf("register shared AGY recovery route: %v", err)
	}
	registered.rootDir = route.rootDir
	registered.homeDir = route.homeDir
	registered.assetPath = filepath.Join(route.workDir, "clip-fixture.mp4")
	registered.factoryName = agyColdWatchFactoryName
	fixture.routes[registered.selector] = registered
}

func (fixture *agySharedProcessFixture) registerEarlyExitRoute(t *testing.T) {
	t.Helper()
	release := make(chan struct{})
	fixture.earlyExitRelease = release
	route := fixture.newRouteDirectories(t, "early-exit")
	copyAgyDirectory(t, support.LegacyFixtureDir(t, "executor_success"), route.workDir)
	workerConfig := strings.Replace(
		support.BuildModelWorkerConfig(modelprovider.ProviderAntigravity, agyFunctionalModel),
		"stopToken: COMPLETE",
		"skipPermissions: true\nstopToken: COMPLETE",
		1,
	)
	support.WriteAgentConfig(t, route.workDir, "worker", workerConfig)
	testutil.WriteSeedFile(t, route.workDir, "task", []byte(`{"title":"agy shared early exit"}`))
	registered, err := fixture.runner.registerOutcomes("early-exit", route.workDir, agySharedCommandOutcome{
		result:  platformprocess.CommandResult{Stdout: []byte(`{"event":"result","result":{"conversation_id":"early-exit","status":"SUCCESS","response":"early exit COMPLETE"}}` + "\n")},
		release: release,
	})
	if err != nil {
		t.Fatalf("register shared AGY early-exit route: %v", err)
	}
	registered.rootDir = route.rootDir
	registered.homeDir = route.homeDir
	fixture.routes[registered.selector] = registered
}

func (fixture *agySharedProcessFixture) registerConcurrencyRoutes(t *testing.T) {
	t.Helper()
	release := make(chan struct{})
	fixture.concurrencyRelease = release
	for _, test := range []struct {
		selector string
		response string
	}{
		{selector: "concurrency-a", response: "shared concurrency A COMPLETE"},
		{selector: "concurrency-b", response: "shared concurrency B COMPLETE"},
	} {
		route := fixture.newRouteDirectories(t, test.selector)
		copyAgyDirectory(t, support.LegacyFixtureDir(t, "executor_success"), route.workDir)
		workerConfig := strings.Replace(
			support.BuildModelWorkerConfig(modelprovider.ProviderAntigravity, agyFunctionalModel),
			"stopToken: COMPLETE",
			"skipPermissions: true\nstopToken: COMPLETE",
			1,
		)
		support.WriteAgentConfig(t, route.workDir, "worker", workerConfig)
		testutil.WriteSeedFile(t, route.workDir, "task", []byte(`{"title":"agy shared concurrency"}`))
		registered, err := fixture.runner.registerOutcomes(test.selector, route.workDir, agySharedCommandOutcome{
			result:  platformprocess.CommandResult{Stdout: []byte(fmt.Sprintf(`{"event":"result","result":{"conversation_id":"%s","status":"SUCCESS","response":"%s","duration_seconds":1.0,"num_turns":1,"usage":{"input_tokens":1,"output_tokens":1,"thinking_tokens":0,"cache_read_tokens":0,"total_tokens":2}}}`+"\n", test.selector, test.response))},
			release: release,
		})
		if err != nil {
			t.Fatalf("register shared AGY concurrency route %q: %v", test.selector, err)
		}
		registered.rootDir = route.rootDir
		registered.homeDir = route.homeDir
		fixture.routes[test.selector] = registered
	}
}

func agyRouteSlug(name string) string {
	return strings.ReplaceAll(strings.ToLower(strings.TrimSpace(name)), " ", "-")
}

func (fixture *agySharedProcessFixture) addGoldenRoute(
	t *testing.T,
	selector, prompt, schema, asset, trace string,
) {
	fixture.addGoldenRouteWithOutcomes(t, selector, prompt, schema, asset,
		agySharedCommandOutcome{result: platformprocess.CommandResult{Stdout: readAgyGoldenAsset(t, trace)}})
}

func (fixture *agySharedProcessFixture) addGoldenRouteWithOutcomes(
	t *testing.T,
	selector, prompt, schema, asset string,
	outcome agySharedCommandOutcome,
) {
	t.Helper()
	route := fixture.newRouteDirectories(t, selector)
	copyAgyDirectory(t, support.LegacyFixtureDir(t, "executor_success"), route.workDir)
	if asset != "" {
		copyAgySharedAsset(t, route.workDir, asset)
	}
	workerConfig := agyGoldenWorkerConfig()
	if selector == "golden-missing-file" {
		workerConfig = agyGoldenWorkerConfigWithStopToken("COMPLETE")
	}
	support.WriteAgentConfig(t, route.workDir, "worker", workerConfig)
	support.WriteWorkstationConfig(t, route.workDir, "process", agyGoldenWorkstationConfig(prompt, schema))
	testutil.WriteSeedFile(t, route.workDir, "task", []byte(`{"title":"agy shared golden"}`))
	registered, err := fixture.runner.registerOutcomes(selector, route.workDir, outcome)
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
	fixture.addRoleRouteWithOutcomes(t, selector, factoryName, asset, agySharedCommandOutcome{
		result: platformprocess.CommandResult{Stdout: stdout, ExitCode: 0},
	})
}

func (fixture *agySharedProcessFixture) addRoleRouteWithOutcomes(
	t *testing.T,
	selector, factoryName, asset string,
	outcome agySharedCommandOutcome,
) {
	t.Helper()
	route := fixture.newRouteDirectories(t, selector)
	if asset != "" && !strings.Contains(asset, "does-not-exist") {
		copyAgySharedAsset(t, route.workDir, asset)
	}
	registered, err := fixture.runner.registerOutcomes(selector, route.workDir, outcome)
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

type agySharedHostedInvocation struct {
	route          *agySharedCommandRoute
	command        *support.ProcessCommand
	httpRun        *agySharedHTTPRun
	baseURL        string
	responseStream *support.FactoryResponseEventStream
}

func (fixture *agySharedProcessFixture) startHosted(
	t *testing.T,
	selector string,
	args []string,
	captureResponseEvents bool,
) *agySharedHostedInvocation {
	t.Helper()
	route := fixture.routes[selector]
	if route == nil {
		t.Fatalf("shared AGY route %q is missing", selector)
	}
	inputs := support.FakeInputs(context.Background(), args)
	inputs.Input.Env = agySharedEnvironment(route.homeDir)
	inputs.Input.WorkingDirectory = route.workDir
	command := support.StartProcessCommand(t, fixture.process, inputs.Input)
	httpRun := fixture.api.waitForStart(t)
	baseURL := httpRun.server.WaitForURL(t)
	invocation := &agySharedHostedInvocation{
		route:   route,
		command: command,
		httpRun: httpRun,
		baseURL: baseURL,
	}
	if captureResponseEvents {
		session := support.GetDefaultSession(t, baseURL)
		invocation.responseStream = support.OpenFactoryResponseEventStreamAt(
			t,
			support.SessionResponseEventsURL(baseURL, session.Id),
		)
	}
	return invocation
}

func (fixture *agySharedProcessFixture) finishHosted(
	t *testing.T,
	invocation *agySharedHostedInvocation,
) (factoryapi.FactorySession, factoryapi.ListWorkResponse, []factoryapi.FactoryEvent, []factoryapi.FactoryResponseEvent) {
	t.Helper()
	liveSession := support.GetDefaultSession(t, invocation.baseURL)
	support.WaitForSessionTerminalStatus(t, invocation.baseURL, liveSession.Id, agySharedInvocationTimeout)
	var responseEvents []factoryapi.FactoryResponseEvent
	if invocation.responseStream != nil {
		responseEvents = collectAgyResponseEvents(t, invocation.responseStream)
	}
	session := support.GetDefaultSession(t, invocation.baseURL)
	listed := support.ListDefaultSessionWork(t, invocation.baseURL)
	events := support.GetFactoryEventsAt(t, invocation.baseURL)
	invocation.command.Stop(t)
	if invocation.responseStream != nil {
		invocation.responseStream.Close()
	}
	closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := invocation.httpRun.waitClosed(closeCtx); err != nil {
		t.Fatalf("close shared AGY HTTP server for %q: %v", invocation.route.selector, err)
	}
	return session, listed, events, responseEvents
}

func (fixture *agySharedProcessFixture) observeHostedSession(
	t *testing.T,
	baseURL, sessionID string,
	stream *support.FactoryResponseEventStream,
) (factoryapi.FactorySession, factoryapi.ListWorkResponse, []factoryapi.FactoryEvent, []factoryapi.FactoryResponseEvent) {
	t.Helper()
	support.WaitForSessionTerminalStatus(t, baseURL, sessionID, agySharedInvocationTimeout)
	var responseEvents []factoryapi.FactoryResponseEvent
	if stream != nil {
		responseEvents = collectAgyResponseEvents(t, stream)
	}
	getResponse := support.GetJSON[factoryapi.FactorySessionGetResponse](
		t,
		strings.TrimSuffix(baseURL, "/")+"/factory-sessions/"+url.PathEscape(sessionID),
	)
	session, err := getResponse.AsFactorySession()
	if err != nil {
		t.Fatalf("decode shared AGY hosted Factory Session %q: %v", sessionID, err)
	}
	listed := support.GetJSON[factoryapi.ListWorkResponse](
		t,
		strings.TrimSuffix(baseURL, "/")+"/factory-sessions/"+url.PathEscape(sessionID)+"/work",
	)
	events := support.GetFactoryEventsForSessionAt(t, baseURL, sessionID)
	return session, listed, events, responseEvents
}

func collectAgyResponseEvents(
	t *testing.T,
	stream *support.FactoryResponseEventStream,
) []factoryapi.FactoryResponseEvent {
	t.Helper()
	var events []factoryapi.FactoryResponseEvent
	for {
		frame := stream.NextFrame(agySharedInvocationTimeout)
		events = append(events, frame.Event)
		if isAgyTerminalResponseEvent(frame.Event) {
			return events
		}
	}
}

func isAgyTerminalResponseEvent(event factoryapi.FactoryResponseEvent) bool {
	if event.Kind == factoryapi.FactoryResponseEventKindError {
		return event.Phase == factoryapi.FactoryResponseEventPhaseFailed ||
			event.Phase == factoryapi.FactoryResponseEventPhaseCanceled
	}
	if event.Kind == factoryapi.FactoryResponseEventKindMessage {
		return event.Phase == factoryapi.FactoryResponseEventPhaseCompleted
	}
	return event.Kind == factoryapi.FactoryResponseEventKindRun &&
		(event.Phase == factoryapi.FactoryResponseEventPhaseFailed ||
			event.Phase == factoryapi.FactoryResponseEventPhaseCanceled)
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
	session, listed, events, _ := fixture.finishHosted(t, fixture.startHosted(t, selector, []string{
		"you", "run",
		"--dir", route.workDir,
		"--continuously",
		"--with-server",
		"--server", "http://127.0.0.1:1",
		"--quiet",
		"--no-record",
	}, false))
	return session, listed, events, route, callStart
}

func (fixture *agySharedProcessFixture) runDirect(
	t *testing.T,
	selector string,
) (factoryapi.FactorySession, factoryapi.ListWorkResponse, []factoryapi.FactoryEvent, []factoryapi.FactoryResponseEvent, *agySharedCommandRoute, int) {
	t.Helper()
	route := fixture.routes[selector]
	if route == nil {
		t.Fatalf("shared AGY direct route %q is missing", selector)
	}
	callStart := route.callCount()
	session, listed, events, responseEvents := fixture.finishHosted(t, fixture.startHosted(t, selector, []string{
		"you", "run",
		"--dir", route.workDir,
		"--continuously",
		"--with-server",
		"--server", "http://127.0.0.1:1",
		"--quiet",
		"--no-record",
	}, true))
	return session, listed, events, responseEvents, route, callStart
}

func (fixture *agySharedProcessFixture) runRole(
	t *testing.T,
	selector string,
	args []string,
) (factoryapi.InvocationResponse, []factoryapi.FactoryEvent, *agySharedCommandRoute, string, int) {
	return fixture.runRoleWithExpectation(t, selector, args, false)
}

func (fixture *agySharedProcessFixture) runRoleFailure(
	t *testing.T,
	selector string,
	args []string,
) (factoryapi.InvocationResponse, []factoryapi.FactoryEvent, *agySharedCommandRoute, string, int) {
	return fixture.runRoleWithExpectation(t, selector, args, true)
}

func (fixture *agySharedProcessFixture) runRoleWithExpectation(
	t *testing.T,
	selector string,
	args []string,
	expectFailure bool,
) (factoryapi.InvocationResponse, []factoryapi.FactoryEvent, *agySharedCommandRoute, string, int) {
	t.Helper()
	route := fixture.routes[selector]
	if route == nil {
		t.Fatalf("shared AGY role route %q is missing", selector)
	}
	callStart := route.callCount()
	host := fixture.roleHost
	if host == nil {
		t.Fatal("shared AGY role host is not running")
	}
	factoryDir := host.factories[route.factoryName]
	opened := support.OpenFactorySessionAt(t, host.baseURL, factoryDir)
	sessionID := opened.Session.Id
	if err := fixture.runner.registerScope(sessionID, route); err != nil {
		t.Fatalf("register shared AGY session route: %v", err)
	}
	t.Cleanup(func() {
		fixture.runner.unregisterScope(sessionID, route)
		support.CloseFactorySessionAt(t, host.baseURL, sessionID)
	})
	args = remoteAgyRoleArgs(args, host.baseURL, sessionID)
	args = append(args, "--output", "primary")
	inputs := support.FakeInputs(context.Background(), args)
	inputs.Input.Env = agySharedEnvironment(host.homeDir)
	inputs.Input.WorkingDirectory = route.workDir
	executionErr := fixture.process.Execute(inputs.Input)
	if expectFailure {
		if executionErr == nil {
			t.Fatalf("Process.Execute(shared AGY role %q) error = nil, want terminal failure", selector)
		}
	} else if executionErr != nil {
		t.Fatalf("Process.Execute(shared AGY role %q): %v\nstdout=%s\nstderr=%s", selector, executionErr, inputs.Stdout(), inputs.Stderr())
	}
	response := support.DecodeInvocationResponseJSON(t, inputs.Stdout())
	events := support.GetFactoryEventsForSessionAt(t, host.baseURL, sessionID)
	return response, events, route, route.assetPath, callStart
}

func remoteAgyRoleArgs(args []string, baseURL, sessionID string) []string {
	result := append([]string(nil), args...)
	for index, arg := range result {
		if arg == "run" {
			prefix := append([]string(nil), result[:index]...)
			prefix = append(prefix, "--remote", "--server", baseURL)
			result = append(prefix, result[index:]...)
			break
		}
	}
	return append(result, "--session", sessionID)
}

func (fixture *agySharedProcessFixture) startRoleHost(t *testing.T) {
	t.Helper()
	if fixture.roleHost != nil {
		t.Fatal("shared AGY role host is already running")
	}
	homeDir := filepath.Join(fixture.rootDir, "role-host-home")
	workDir := fixture.routes["direct-conductor-success"].workDir
	env := agySharedEnvironment(homeDir)
	factories := map[string]string{
		agyColdWatchFactoryName: support.InstallPackagedFactoryWithProcess(t, fixture.process, env, workDir, agyColdWatchFactoryName),
		agyClipQAFactoryName:    support.InstallPackagedFactoryWithProcess(t, fixture.process, env, workDir, agyClipQAFactoryName),
	}
	inputs := support.FakeInputs(context.Background(), []string{
		"you", "run", "--dir", workDir, "--continuously", "--with-server",
		"--server", "http://127.0.0.1:1", "--quiet", "--no-record",
	})
	inputs.Input.Env = env
	inputs.Input.WorkingDirectory = workDir
	command := support.StartProcessCommand(t, fixture.process, inputs.Input)
	httpRun := fixture.api.waitForStart(t)
	fixture.roleHost = &agySharedRoleHost{
		command: command, httpRun: httpRun, baseURL: httpRun.server.WaitForURL(t),
		homeDir: homeDir, factories: factories,
	}
}

func (fixture *agySharedProcessFixture) stopRoleHost(t *testing.T) {
	t.Helper()
	host := fixture.roleHost
	fixture.roleHost = nil
	if host == nil {
		return
	}
	host.command.Stop(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := host.httpRun.waitClosed(ctx); err != nil {
		t.Fatalf("close shared AGY role host: %v", err)
	}
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
		if fixture.processBuilds != 1 {
			closeErrors = append(closeErrors, fmt.Errorf("shared AGY process builds = %d, want 1", fixture.processBuilds))
		}
		if fixture.api != nil {
			if err := fixture.api.waitClosed(closeCtx); err != nil {
				closeErrors = append(closeErrors, err)
			}
		}
		if fixture.process != nil {
			fixture.processCloses++
			if err := fixture.process.Close(closeCtx); err != nil {
				closeErrors = append(closeErrors, err)
			}
		}
		if fixture.runner != nil {
			if got := fixture.runner.activeCallCount(); got != 0 {
				closeErrors = append(closeErrors, fmt.Errorf("shared AGY active command calls = %d, want 0", got))
			}
			if got := fixture.runner.routeCount(); got != agySharedRouteCount {
				closeErrors = append(closeErrors, fmt.Errorf("shared AGY routes before clear = %d, want %d", got, agySharedRouteCount))
			}
			for _, route := range fixture.routes {
				for _, recordingPath := range route.recordingPathsSnapshot() {
					if _, err := os.Stat(recordingPath); !errors.Is(err, os.ErrNotExist) {
						closeErrors = append(closeErrors, fmt.Errorf("shared AGY recording remains at %q: %v", recordingPath, err))
					}
				}
			}
			if err := fixture.runner.clear(); err != nil {
				closeErrors = append(closeErrors, err)
			}
			if got := fixture.runner.routeCount(); got != 0 {
				closeErrors = append(closeErrors, fmt.Errorf("shared AGY routes after clear = %d, want 0", got))
			}
		}
		if fixture.processCloses != 1 {
			closeErrors = append(closeErrors, fmt.Errorf("shared AGY process closes = %d, want 1", fixture.processCloses))
		}
		if fixture.rootDir != "" {
			if err := os.RemoveAll(fixture.rootDir); err != nil {
				closeErrors = append(closeErrors, err)
			} else if _, err := os.Stat(fixture.rootDir); !errors.Is(err, os.ErrNotExist) {
				closeErrors = append(closeErrors, fmt.Errorf("shared AGY fixture root remains: %w", err))
			}
		}
		fixture.routes = nil
		fixture.closeErr = errors.Join(closeErrors...)
	})
	return fixture.closeErr
}

var _ platformprocess.CommandRunner = (*agySharedCommandRunner)(nil)
