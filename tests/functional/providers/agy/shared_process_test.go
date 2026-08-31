package agy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
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
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const agySharedInvocationTimeout = 30 * time.Second

const agySharedRouteCount = 29

// agySharedHTTPServer owns the one listener started by the package-scoped
// process. Explicit Factory Sessions opened against this listener carry the
// scenario-specific folder and identity; the default session is reserved for
// the long-lived host and receives no scenario Work.
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

// agySharedProcessCommand owns the long-lived host invocation without tying
// its cleanup to the first test that happens to construct the package fixture.
// The package fixture is closed from TestMain after all per-test session
// cleanups have run.
type agySharedProcessCommand struct {
	cancel context.CancelFunc
	done   chan struct{}

	mu  sync.Mutex
	err error

	stopOnce sync.Once
}

func startAgySharedProcessCommand(
	process support.Process,
	inputs *support.CapturedInputs,
) *agySharedProcessCommand {
	parent := inputs.Input.Context
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	inputs.Input.Context = ctx
	command := &agySharedProcessCommand{cancel: cancel, done: make(chan struct{})}
	go func() {
		err := process.Execute(inputs.Input)
		command.mu.Lock()
		command.err = err
		command.mu.Unlock()
		close(command.done)
	}()
	return command
}

func (command *agySharedProcessCommand) stop(ctx context.Context) error {
	if command == nil {
		return nil
	}
	command.stopOnce.Do(command.cancel)
	return command.wait(ctx)
}

func (command *agySharedProcessCommand) wait(ctx context.Context) error {
	if command == nil {
		return nil
	}
	select {
	case <-command.done:
		command.mu.Lock()
		err := command.err
		command.mu.Unlock()
		if errors.Is(err, context.Canceled) {
			return nil
		}
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

type agySharedProcessFixture struct {
	rootDir            string
	homeDir            string
	hostWorkDir        string
	baseURL            string
	process            support.ApplicationProcess
	command            *agySharedProcessCommand
	recordingProcess   support.ApplicationProcess
	runner             *agySharedCommandRunner
	api                *agySharedHTTPServer
	routes             map[string]*agySharedCommandRoute
	processBuilds      int
	processCloses      int
	recordingBuilds    int
	recordingCloses    int
	earlyExitRelease   *agySharedRelease
	concurrencyRelease *agySharedRelease

	sessionMu        sync.Mutex
	openedSessionIDs map[string]struct{}
	activeSessionIDs map[string]struct{}
	hostMu           sync.Mutex

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
	homeDir := filepath.Join(rootDir, "home")
	hostWorkDir := filepath.Join(rootDir, "host")
	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		t.Fatalf("create shared AGY home %q: %v", homeDir, err)
	}
	if err := os.MkdirAll(hostWorkDir, 0o755); err != nil {
		t.Fatalf("create shared AGY host workspace %q: %v", hostWorkDir, err)
	}
	fixture := &agySharedProcessFixture{
		rootDir:          rootDir,
		homeDir:          homeDir,
		hostWorkDir:      hostWorkDir,
		runner:           newAgySharedCommandRunner(),
		api:              newAgySharedHTTPServer(),
		routes:           make(map[string]*agySharedCommandRoute),
		openedSessionIDs: make(map[string]struct{}),
		activeSessionIDs: make(map[string]struct{}),
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
	for _, route := range fixture.routes {
		// The old per-route hosted command consumed these startup seeds. The
		// shared host must start empty so each explicit session receives only
		// the Work submitted by its own witness.
		support.ClearSeedInputs(t, route.workDir)
	}
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
	fixture.materializeRoleFactories(t)
	return fixture
}

// ensureHosted activates the one shared API listener on demand. Keeping the
// host lazy avoids booting an unused listener for recording-only focused runs;
// the recording-sensitive recovery witness uses its own root process because
// Process.Execute cannot overlap with the long-lived hosted execution.
func (fixture *agySharedProcessFixture) ensureHosted(t *testing.T) {
	t.Helper()
	fixture.hostMu.Lock()
	defer fixture.hostMu.Unlock()
	if fixture.command != nil {
		return
	}
	copyAgyDirectory(t, support.LegacyFixtureDir(t, "executor_success"), fixture.hostWorkDir)
	support.ClearSeedInputs(t, fixture.hostWorkDir)
	hostWorkerConfig := strings.Replace(
		support.BuildModelWorkerConfig(modelprovider.ProviderAntigravity, agyFunctionalModel),
		"stopToken: COMPLETE",
		"skipPermissions: true\nstopToken: COMPLETE",
		1,
	)
	support.WriteAgentConfig(t, fixture.hostWorkDir, "worker", hostWorkerConfig)
	inputs := support.FakeInputs(context.Background(), []string{
		"you", "run", "--dir", fixture.hostWorkDir,
		"--continuously", "--with-server", "--server", "http://127.0.0.1:1",
		"--quiet", "--no-record",
	})
	inputs.Input.Env = agySharedEnvironment(fixture.homeDir)
	inputs.Input.WorkingDirectory = fixture.hostWorkDir
	fixture.command = startAgySharedProcessCommand(fixture.process, inputs)
	fixture.baseURL = fixture.api.server.WaitForURL(t)
	support.WaitForStatus(t, fixture.baseURL, agySharedInvocationTimeout, func(status factoryapi.StatusResponse) bool {
		return strings.TrimSpace(status.RuntimeStatus) != ""
	})
}

// materializeRoleFactories uses the public initializer once per packaged
// Factory, then copies the resulting immutable package payload into each
// route workspace. Sessions can therefore select the route's local Factory
// while the package process and listener remain shared.
func (fixture *agySharedProcessFixture) materializeRoleFactories(t *testing.T) {
	t.Helper()
	sources := make(map[string]string)
	for _, route := range fixture.routes {
		if strings.TrimSpace(route.factoryName) == "" {
			continue
		}
		if _, ok := sources[route.factoryName]; ok {
			continue
		}
		sources[route.factoryName] = support.InstallPackagedFactoryWithProcess(
			t,
			fixture.process,
			agySharedEnvironment(fixture.homeDir),
			fixture.hostWorkDir,
			route.factoryName,
		)
	}
	for _, route := range fixture.routes {
		source, ok := sources[route.factoryName]
		if !ok {
			continue
		}
		copyAgyDirectory(t, source, route.workDir)
	}
}

func (fixture *agySharedProcessFixture) ensureRecordingProcess(t *testing.T) {
	t.Helper()
	if fixture.recordingProcess != nil {
		return
	}
	process, err := support.BuildProcessWithContext(context.Background(), serviceedges.Edges{
		ProviderCommandRunner: fixture.runner,
	})
	if err != nil {
		t.Fatalf("BuildProcess(isolated AGY recording fixture): %v", err)
	}
	fixture.recordingProcess = process
	fixture.recordingBuilds++
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
	release := newAgySharedRelease()
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
	release := newAgySharedRelease()
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

type agySharedSession struct {
	route  *agySharedCommandRoute
	id     string
	closed bool
}

func (fixture *agySharedProcessFixture) openSession(
	t *testing.T,
	route *agySharedCommandRoute,
) *agySharedSession {
	t.Helper()
	fixture.ensureHosted(t)
	if route == nil {
		t.Fatal("open shared AGY session: route is nil")
	}
	opened := support.OpenFactorySessionAt(t, fixture.baseURL, route.workDir)
	if opened.Session == nil || strings.TrimSpace(opened.Session.Id) == "" {
		t.Fatalf("shared AGY Factory Session for %q = %#v, want identity", route.selector, opened)
	}
	sessionID := strings.TrimSpace(opened.Session.Id)
	if sessionID == "~default" {
		t.Fatalf("shared AGY Factory Session for %q = %q, want explicit session", route.selector, sessionID)
	}
	fixture.sessionMu.Lock()
	if _, exists := fixture.openedSessionIDs[sessionID]; exists {
		fixture.sessionMu.Unlock()
		t.Fatalf("duplicate shared AGY Factory Session identity %q", sessionID)
	}
	fixture.openedSessionIDs[sessionID] = struct{}{}
	fixture.activeSessionIDs[sessionID] = struct{}{}
	fixture.sessionMu.Unlock()
	session := &agySharedSession{route: route, id: sessionID}
	t.Cleanup(func() { fixture.closeSession(t, session) })
	return session
}

func (fixture *agySharedProcessFixture) closeSession(t testing.TB, session *agySharedSession) {
	t.Helper()
	if session == nil || session.closed {
		return
	}
	support.CloseFactorySessionAt(t, fixture.baseURL, session.id)
	session.closed = true
	fixture.sessionMu.Lock()
	delete(fixture.activeSessionIDs, session.id)
	fixture.sessionMu.Unlock()
}

func (fixture *agySharedProcessFixture) observeSession(
	t testing.TB,
	session *agySharedSession,
	stream *support.FactoryResponseEventStream,
) (factoryapi.FactorySession, factoryapi.ListWorkResponse, []factoryapi.FactoryEvent, []factoryapi.FactoryResponseEvent) {
	t.Helper()
	if session == nil {
		t.Fatal("observe shared AGY session: session is nil")
	}
	support.WaitForSessionTerminalStatus(t, fixture.baseURL, session.id, agySharedInvocationTimeout)
	var responseEvents []factoryapi.FactoryResponseEvent
	if stream != nil {
		responseEvents = collectAgyResponseEvents(t, stream)
	}
	getResponse := support.GetJSON[factoryapi.FactorySessionGetResponse](
		t,
		strings.TrimSuffix(fixture.baseURL, "/")+"/factory-sessions/"+url.PathEscape(session.id),
	)
	liveSession, err := getResponse.AsFactorySession()
	if err != nil {
		t.Fatalf("decode shared AGY Factory Session %q: %v", session.id, err)
	}
	listed := support.GetJSON[factoryapi.ListWorkResponse](
		t,
		strings.TrimSuffix(fixture.baseURL, "/")+"/factory-sessions/"+url.PathEscape(session.id)+"/work",
	)
	events := support.GetFactoryEventsForSessionAt(t, fixture.baseURL, session.id)
	return liveSession, listed, events, responseEvents
}

func collectAgyResponseEvents(
	t testing.TB,
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
	sessionHandle := fixture.openSession(t, route)
	fixture.submitWork(t, sessionHandle, "agy shared golden")
	session, listed, events, _ := fixture.observeSession(t, sessionHandle, nil)
	fixture.closeSession(t, sessionHandle)
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
	sessionHandle := fixture.openSession(t, route)
	responseStream := support.OpenFactoryResponseEventStreamAt(
		t,
		support.SessionResponseEventsURL(fixture.baseURL, sessionHandle.id),
	)
	fixture.submitWork(t, sessionHandle, "agy shared direct")
	session, listed, events, responseEvents := fixture.observeSession(t, sessionHandle, responseStream)
	responseStream.Close()
	fixture.closeSession(t, sessionHandle)
	return session, listed, events, responseEvents, route, callStart
}

func (fixture *agySharedProcessFixture) submitWork(t testing.TB, session *agySharedSession, title string) {
	t.Helper()
	if session == nil {
		t.Fatal("submit shared AGY Work: session is nil")
	}
	name := session.route.selector
	submitted := support.SubmitSessionWorkAt(t, fixture.baseURL, session.id, factoryapi.SubmitWorkRequest{
		Name:         &name,
		WorkTypeName: "task",
		Payload:      map[string]string{"title": title},
	})
	if submitted.SessionId == nil || *submitted.SessionId != session.id {
		t.Fatalf("shared AGY submitted Work session ID = %#v, want %q", submitted.SessionId, session.id)
	}
	if strings.TrimSpace(submitted.RequestId) == "" || support.StringPointerValue(submitted.WorkId) == "" {
		t.Fatalf("shared AGY submitted Work identity = request:%q work:%q, want both identities", submitted.RequestId, support.StringPointerValue(submitted.WorkId))
	}
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

// runRoleWithRecordingExpectation is retained for the recovery witness only.
// The public live-session API intentionally does not accept a recording path;
// this narrow one-shot path preserves the production CLI recording contract
// while every other role case uses a fresh explicit session on the shared
// listener. The packaged Factory, route and command edge remain shared, while
// the secondary root process and recording path are explicit isolation
// properties of this witness.
func (fixture *agySharedProcessFixture) runRoleWithRecordingExpectation(
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
	fixture.ensureRecordingProcess(t)
	callStart := route.callCount()
	recordingDir, err := os.MkdirTemp(route.rootDir, "recording-")
	if err != nil {
		t.Fatalf("create shared AGY recording directory: %v", err)
	}
	recordingPath := filepath.Join(recordingDir, "events.replay.json")
	route.recordRecordingPath(recordingPath)
	t.Cleanup(func() { _ = os.RemoveAll(recordingDir) })
	args = append(append([]string(nil), args...), "--record", recordingPath, "--output", "primary")
	inputs := support.FakeInputs(context.Background(), args)
	inputs.Input.Env = agySharedEnvironment(fixture.homeDir)
	inputs.Input.WorkingDirectory = route.workDir
	command := startAgySharedProcessCommand(fixture.recordingProcess, inputs)
	waitCtx, cancelWait := context.WithTimeout(context.Background(), agySharedInvocationTimeout)
	executionErr := command.wait(waitCtx)
	cancelWait()
	if errors.Is(executionErr, context.DeadlineExceeded) {
		stopCtx, cancelStop := context.WithTimeout(context.Background(), agySharedInvocationTimeout)
		stopErr := command.stop(stopCtx)
		cancelStop()
		t.Fatalf("Process.Execute(shared AGY recording role %q) timed out: stop error: %v", selector, stopErr)
	}
	if expectFailure {
		if executionErr == nil {
			t.Fatalf("Process.Execute(shared AGY recording role %q) error = nil, want terminal failure", selector)
		}
	} else if executionErr != nil {
		t.Fatalf("Process.Execute(shared AGY recording role %q): %v\nstdout=%s\nstderr=%s", selector, executionErr, inputs.Stdout(), inputs.Stderr())
	}
	if strings.TrimSpace(inputs.Stdout()) == "" {
		t.Fatalf("Process.Execute(shared AGY recording role %q) returned no invocation JSON: err=%v\nstderr=%s", selector, executionErr, inputs.Stderr())
	}
	response := support.DecodeInvocationResponseJSON(t, inputs.Stdout())
	events := readAgyRecording(t, recordingPath, "shared AGY recovery role")
	if err := os.RemoveAll(recordingDir); err != nil {
		t.Fatalf("remove shared AGY recording directory: %v", err)
	}
	if _, err := os.Stat(recordingDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("shared AGY recording directory %q remains after invocation: %v", recordingDir, err)
	}
	return response, events, route, route.assetPath, callStart
}

func (fixture *agySharedProcessFixture) runRoleRecording(
	t *testing.T,
	selector string,
	args []string,
) (factoryapi.InvocationResponse, []factoryapi.FactoryEvent, *agySharedCommandRoute, string, int) {
	return fixture.runRoleWithRecordingExpectation(t, selector, args, false)
}

func (fixture *agySharedProcessFixture) runRoleRecordingFailure(
	t *testing.T,
	selector string,
	args []string,
) (factoryapi.InvocationResponse, []factoryapi.FactoryEvent, *agySharedCommandRoute, string, int) {
	return fixture.runRoleWithRecordingExpectation(t, selector, args, true)
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
	session := fixture.openSession(t, route)
	response := fixture.invokeRole(t, session, args)
	if expectFailure && response.Status != factoryapi.InvocationTerminalStatusFailed {
		t.Fatalf("shared AGY role %q status = %q, want FAILED", selector, response.Status)
	}
	if !expectFailure && response.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("shared AGY role %q status = %q, want COMPLETED", selector, response.Status)
	}
	events := support.GetFactoryEventsForSessionAt(t, fixture.baseURL, session.id)
	fixture.closeSession(t, session)
	return response, events, route, route.assetPath, callStart
}

func (fixture *agySharedProcessFixture) invokeRole(
	t testing.TB,
	session *agySharedSession,
	args []string,
) factoryapi.InvocationResponse {
	t.Helper()
	request := agyInvocationRequestFromArgs(t, args)
	response, err := fixture.invokeSession(context.Background(), session.id, request)
	if err != nil {
		t.Fatalf("invoke shared AGY Factory Session %q: %v", session.id, err)
	}
	return response
}

func agyInvocationRequestFromArgs(t testing.TB, args []string) factoryapi.InvocationRequest {
	t.Helper()
	values := make(map[string]any)
	for index := 0; index < len(args); index++ {
		if index+1 >= len(args) {
			continue
		}
		key := ""
		switch args[index] {
		case "--cut-path":
			key = "cutPath"
		case "--clip-path":
			key = "clipPath"
		case "--shot-specification":
			key = "shotSpecification"
		}
		if key != "" {
			values[key] = args[index+1]
			index++
		}
	}
	if len(values) == 0 {
		t.Fatalf("shared AGY role arguments = %#v, want invocation arguments", args)
	}
	return factoryapi.InvocationRequest{Args: &values}
}

func (fixture *agySharedProcessFixture) invokeSession(
	ctx context.Context,
	sessionID string,
	request factoryapi.InvocationRequest,
) (factoryapi.InvocationResponse, error) {
	payload, err := json.Marshal(request)
	if err != nil {
		return factoryapi.InvocationResponse{}, fmt.Errorf("marshal invocation request: %w", err)
	}
	endpoint := strings.TrimSuffix(fixture.baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID) + "/invocations"
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return factoryapi.InvocationResponse{}, fmt.Errorf("build invocation request: %w", err)
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(httpRequest)
	if err != nil {
		return factoryapi.InvocationResponse{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		return factoryapi.InvocationResponse{}, fmt.Errorf("POST %s status = %d: %s", endpoint, response.StatusCode, strings.TrimSpace(string(body)))
	}
	var result factoryapi.InvocationResponse
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		return factoryapi.InvocationResponse{}, fmt.Errorf("decode invocation response: %w", err)
	}
	return result, nil
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
		if fixture.recordingBuilds > 1 {
			closeErrors = append(closeErrors, fmt.Errorf("isolated AGY recording process builds = %d, want at most 1", fixture.recordingBuilds))
		}
		fixture.hostMu.Lock()
		command := fixture.command
		fixture.hostMu.Unlock()
		if command != nil {
			if err := command.stop(closeCtx); err != nil {
				closeErrors = append(closeErrors, fmt.Errorf("stop shared AGY host command: %w", err))
			}
		}
		if fixture.api != nil {
			if command != nil {
				if err := fixture.api.waitClosed(closeCtx); err != nil {
					closeErrors = append(closeErrors, err)
				}
			}
			wantStarts := 0
			if command != nil {
				wantStarts = 1
			}
			if fixture.api.startCount() != wantStarts {
				closeErrors = append(closeErrors, fmt.Errorf("shared AGY listener starts = %d, want %d", fixture.api.startCount(), wantStarts))
			}
		}
		if fixture.process != nil {
			fixture.processCloses++
			if err := fixture.process.Close(closeCtx); err != nil {
				closeErrors = append(closeErrors, err)
			}
		}
		if fixture.recordingProcess != nil {
			fixture.recordingCloses++
			if err := fixture.recordingProcess.Close(closeCtx); err != nil {
				closeErrors = append(closeErrors, fmt.Errorf("close isolated AGY recording process: %w", err))
			}
		}
		if fixture.runner != nil {
			if got := fixture.runner.activeCallCount(); got != 0 {
				closeErrors = append(closeErrors, fmt.Errorf("shared AGY active command calls = %d, want 0", got))
			}
			fixture.sessionMu.Lock()
			activeSessions := len(fixture.activeSessionIDs)
			fixture.sessionMu.Unlock()
			if activeSessions != 0 {
				closeErrors = append(closeErrors, fmt.Errorf("shared AGY active Factory Sessions = %d, want 0", activeSessions))
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
		if fixture.recordingCloses != fixture.recordingBuilds {
			closeErrors = append(closeErrors, fmt.Errorf("isolated AGY recording process closes = %d, want %d", fixture.recordingCloses, fixture.recordingBuilds))
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
