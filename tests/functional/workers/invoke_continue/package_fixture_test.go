package acceptance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	providerswire "github.com/portpowered/infinite-you/pkg/services/providers/wire"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const invokeContinuePackageFixtureTimeout = 15 * time.Second

// invokeContinuePackageFixture is the package-owned executable spine for the
// invoke/continue scenarios. The process and its route table are immutable
// after setup; scenario state is carried by explicit Factory Session IDs and
// the provider runner's recorded calls.
type invokeContinuePackageFixture struct {
	rootDir              string
	hostDir              string
	homeDir              string
	baseURL              string
	process              support.ApplicationProcess
	command              *invokeContinuePackageCommand
	router               *invokeContinueStaticCommandRoute
	apiStopped           <-chan struct{}
	apiStarts            *atomic.Int32
	processBuilds        *atomic.Int32
	processClosed        atomic.Bool
	streamsOpened        atomic.Int32
	streamsClosed        atomic.Int32
	scenarioRuns         atomic.Uint64
	scenarios            []invokeContinueScenario
	managerRunner        *s8RemoteProviderRunner
	managerRepositoryA   s8Repository
	managerRepositoryB   s8Repository
	interruptRunner      *s8InterruptProviderRunner
	interruptRepositoryA s8Repository
	interruptRepositoryB s8Repository

	sessionsMu        sync.Mutex
	openedSessionIDs  []string
	closedSessionIDs  []string
	deletedSessionIDs []string
}

// invokeContinueScenario is a pre-registered provider route plus the
// scenario-local filesystem and Factory Session scope. The route is selected
// only by the immutable working directory supplied in the execution request;
// no request order or mutable registration is involved after BuildProcess.
type invokeContinueScenario struct {
	fixture             *invokeContinuePackageFixture
	name                string
	runNumber           uint64
	workingDirectory    string
	homeDirectory       string
	providerRunner      invokeContinueProviderCommandRunner
	streamingRunner     *wsrFT015StreamingProviderRunner
	blockingRunner      *invokeContinueBlockingProviderRunner
	unsupportedProvider providers.Service
	reset               func()
	session             *invokeContinueFactorySession
}

type invokeContinueProviderCommandRunner interface {
	platformprocess.CommandRunner
	CallCount() int
	Requests() []platformprocess.CommandRequest
}

func (scenario *invokeContinueScenario) environment() []string {
	return invokeContinueEnvironment(scenario.homeDirectory)
}

func (scenario *invokeContinueScenario) close(t testing.TB) {
	t.Helper()
	scenario.session.close(t)
	scenario.session.assertDeleted(t)
}

type invokeContinuePackageCommand struct {
	cancel context.CancelFunc
	done   chan error
	once   sync.Once
	err    error
}

// invokeContinuePackageFixtureState keeps lazy setup cheap for selectors that
// do not exercise CASE-01 while allowing TestMain to own final process cleanup.
var invokeContinuePackageFixtureState struct {
	sync.Mutex
	fixture *invokeContinuePackageFixture
}

func TestMain(m *testing.M) {
	exitCode := m.Run()
	if err := closeInvokeContinuePackageFixture(); err != nil {
		fmt.Fprintf(os.Stderr, "invoke/continue package fixture cleanup failed: %v\n", err)
		exitCode = 1
	}
	os.Exit(exitCode)
}

func ensureInvokeContinuePackageFixture(t *testing.T) *invokeContinuePackageFixture {
	t.Helper()
	return ensureInvokeContinuePackageFixtureMode(t, false)
}

func ensureInvokeContinueForcedCleanupFixture(t *testing.T) *invokeContinuePackageFixture {
	t.Helper()
	return ensureInvokeContinuePackageFixtureMode(t, true)
}

func ensureInvokeContinuePackageFixtureMode(t *testing.T, cleanupOnly bool) *invokeContinuePackageFixture {
	t.Helper()

	invokeContinuePackageFixtureState.Lock()
	fixture := invokeContinuePackageFixtureState.fixture
	invokeContinuePackageFixtureState.Unlock()
	if fixture != nil {
		return fixture
	}

	created, err := newInvokeContinuePackageFixture(t, cleanupOnly)
	if err != nil {
		t.Fatalf("set up invoke/continue package fixture: %v", err)
	}

	invokeContinuePackageFixtureState.Lock()
	if invokeContinuePackageFixtureState.fixture == nil {
		invokeContinuePackageFixtureState.fixture = created
		fixture = created
	} else {
		fixture = invokeContinuePackageFixtureState.fixture
	}
	invokeContinuePackageFixtureState.Unlock()
	return fixture
}

func newInvokeContinuePackageFixture(t *testing.T, cleanupOnly bool) (*invokeContinuePackageFixture, error) {
	t.Helper()
	rootDir, err := os.MkdirTemp("", "c11-invoke-continue-")
	if err != nil {
		return nil, fmt.Errorf("create package fixture root: %w", err)
	}
	keepRoot := false
	defer func() {
		if !keepRoot {
			_ = os.RemoveAll(rootDir)
		}
	}()
	hostDir, homeDir, err := prepareInvokeContinuePackageRoot(t, rootDir)
	if err != nil {
		return nil, err
	}
	setup, err := newInvokeContinueScenarioSetup(t, rootDir, homeDir, cleanupOnly)
	if err != nil {
		return nil, err
	}
	route := &invokeContinueStaticCommandRoute{routes: setup.routes}
	var unsupportedProvider providers.Service
	if !cleanupOnly {
		unsupportedProvider, err = providerswire.NewService(
			providerswire.WithCommandRunner(route),
			providerswire.WithCatalogCapabilityOverrides(providerswire.CatalogCapabilityOverride{
				Provider:     providers.IDCodex,
				Capabilities: []providers.Capability{providers.CapabilityPromptSubmission},
			}),
		)
		if err != nil {
			return nil, fmt.Errorf("build unsupported continuation provider: %w", err)
		}
		for index := range setup.scenarios {
			if setup.scenarios[index].name == "unsupported-provider" {
				setup.scenarios[index].unsupportedProvider = unsupportedProvider
			}
		}
	}
	started, err := startInvokeContinuePackageProcess(t, rootDir, hostDir, homeDir, route, unsupportedProvider)
	if err != nil {
		return nil, err
	}
	keepRoot = true
	return &invokeContinuePackageFixture{
		rootDir:              rootDir,
		hostDir:              hostDir,
		homeDir:              homeDir,
		baseURL:              started.baseURL,
		process:              started.process,
		command:              started.command,
		router:               route,
		apiStopped:           started.apiStopped,
		apiStarts:            started.apiStarts,
		processBuilds:        started.processBuilds,
		scenarios:            setup.scenarios,
		managerRunner:        setup.managerRunner,
		managerRepositoryA:   setup.managerRepositoryA,
		managerRepositoryB:   setup.managerRepositoryB,
		interruptRunner:      setup.interruptRunner,
		interruptRepositoryA: setup.interruptRepositoryA,
		interruptRepositoryB: setup.interruptRepositoryB,
	}, nil
}

func startInvokeContinuePackageCommand(
	process support.ApplicationProcess,
	inputs *support.CapturedInputs,
) *invokeContinuePackageCommand {
	parent := inputs.Input.Context
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	input := inputs.Input
	input.Context = ctx
	command := &invokeContinuePackageCommand{cancel: cancel, done: make(chan error, 1)}
	go func() {
		command.done <- process.Execute(input)
	}()
	return command
}

func (command *invokeContinuePackageCommand) stop() error {
	if command == nil {
		return nil
	}
	command.once.Do(func() {
		command.cancel()
		select {
		case err := <-command.done:
			if err != nil && !errors.Is(err, context.Canceled) {
				command.err = fmt.Errorf("stop package fixture command: %w", err)
			}
		case <-time.After(invokeContinuePackageFixtureTimeout):
			command.err = errors.New("timed out stopping package fixture command")
		}
	})
	return command.err
}

func closeInvokeContinuePackageFixture() error {
	invokeContinuePackageFixtureState.Lock()
	fixture := invokeContinuePackageFixtureState.fixture
	invokeContinuePackageFixtureState.Unlock()
	if fixture == nil {
		return nil
	}

	var errs []error
	if err := fixture.command.stop(); err != nil {
		errs = append(errs, err)
	}
	closeCtx, cancel := context.WithTimeout(context.Background(), invokeContinuePackageFixtureTimeout)
	if err := fixture.process.Close(closeCtx); err != nil {
		errs = append(errs, fmt.Errorf("close package fixture process: %w", err))
	} else {
		fixture.processClosed.Store(true)
	}
	cancel()
	select {
	case <-fixture.apiStopped:
	case <-time.After(invokeContinuePackageFixtureTimeout):
		errs = append(errs, errors.New("package fixture API server did not stop"))
	}

	if got := fixture.processBuilds.Load(); got != 1 {
		errs = append(errs, fmt.Errorf("package fixture process builds = %d, want exactly one", got))
	}
	if got := fixture.apiStarts.Load(); got != 1 {
		errs = append(errs, fmt.Errorf("package fixture API starts = %d, want exactly one", got))
	}
	fixture.sessionsMu.Lock()
	if len(fixture.openedSessionIDs) != len(fixture.closedSessionIDs) || len(fixture.openedSessionIDs) != len(fixture.deletedSessionIDs) {
		errs = append(errs, fmt.Errorf(
			"package fixture Factory Sessions opened = %d, closed = %d, deleted = %d",
			len(fixture.openedSessionIDs), len(fixture.closedSessionIDs), len(fixture.deletedSessionIDs),
		))
	} else {
		deleted := make(map[string]struct{}, len(fixture.deletedSessionIDs))
		for _, sessionID := range fixture.deletedSessionIDs {
			deleted[sessionID] = struct{}{}
		}
		for _, sessionID := range fixture.openedSessionIDs {
			if _, ok := deleted[sessionID]; !ok {
				errs = append(errs, fmt.Errorf("Factory Session %q was opened but not deleted", sessionID))
			}
		}
	}
	fixture.sessionsMu.Unlock()
	if got := fixture.streamsOpened.Load(); got != fixture.streamsClosed.Load() {
		errs = append(errs, fmt.Errorf(
			"package fixture streams opened = %d, closed = %d",
			got, fixture.streamsClosed.Load(),
		))
	}
	fixture.router.Close()
	if got := fixture.router.routeCount(); got != 0 {
		errs = append(errs, fmt.Errorf("package fixture routes remaining after close = %d", got))
	}
	if got := fixture.router.activeCallCount(); got != 0 {
		errs = append(errs, fmt.Errorf("package fixture active provider calls after close = %d", got))
	}
	if got := fixture.activeProviderCallCount(); got != 0 {
		errs = append(errs, fmt.Errorf("package fixture provider-runner calls after close = %d", got))
	}
	if reachable, err := invokeContinueListenerReachable(fixture.baseURL); err != nil {
		errs = append(errs, fmt.Errorf("probe package fixture listener: %w", err))
	} else if reachable {
		errs = append(errs, errors.New("package fixture listener remained reachable after process close"))
	}
	if available, err := invokeContinuePortAvailable(fixture.baseURL); err != nil {
		errs = append(errs, fmt.Errorf("probe package fixture port: %w", err))
	} else if available {
		errs = append(errs, errors.New("package fixture listener port remained available after process close"))
	}
	if err := os.RemoveAll(fixture.rootDir); err != nil {
		errs = append(errs, fmt.Errorf("remove package fixture root %q: %w", fixture.rootDir, err))
	} else if _, err := os.Stat(fixture.rootDir); !errors.Is(err, os.ErrNotExist) {
		errs = append(errs, fmt.Errorf("package fixture root %q remains after cleanup: %v", fixture.rootDir, err))
	}
	if err := writeInvokeContinueForcedCleanupReport(fixture); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func (fixture *invokeContinuePackageFixture) activeProviderCallCount() int {
	if fixture == nil {
		return 0
	}
	active := 0
	if fixture.managerRunner != nil {
		active += fixture.managerRunner.ActiveCallCount()
	}
	if fixture.interruptRunner != nil {
		active += fixture.interruptRunner.ActiveCallCount()
	}
	return active
}

func invokeContinueListenerReachable(baseURL string) (bool, error) {
	parsed, err := url.Parse(strings.TrimSuffix(baseURL, "/"))
	if err != nil {
		return false, err
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return false, fmt.Errorf("package fixture API URL %q has no scheme or host", baseURL)
	}
	client := http.Client{Timeout: time.Second}
	response, err := client.Get(parsed.String() + "/status")
	if err != nil {
		return false, nil
	}
	defer response.Body.Close()
	return true, nil
}

func invokeContinuePortAvailable(baseURL string) (bool, error) {
	parsed, err := url.Parse(strings.TrimSuffix(baseURL, "/"))
	if err != nil {
		return false, err
	}
	if parsed.Host == "" {
		return false, fmt.Errorf("package fixture API URL %q has no host", baseURL)
	}
	connection, err := net.DialTimeout("tcp", parsed.Host, time.Second)
	if err != nil {
		return false, nil
	}
	_ = connection.Close()
	return true, nil
}

func (fixture *invokeContinuePackageFixture) openSession(t *testing.T) *invokeContinueFactorySession {
	t.Helper()
	opened := support.OpenFactorySessionAt(t, fixture.baseURL, fixture.hostDir)
	sessionID := opened.Session.Id
	if sessionID == factorysessions.DefaultSessionID {
		t.Fatalf("CASE-01 Factory Session ID = %q, want explicit non-default session", sessionID)
	}
	fixture.sessionsMu.Lock()
	for _, openedSessionID := range fixture.openedSessionIDs {
		if openedSessionID == sessionID {
			fixture.sessionsMu.Unlock()
			t.Fatalf("Factory Session ID %q was reused across scenarios", sessionID)
		}
	}
	fixture.openedSessionIDs = append(fixture.openedSessionIDs, sessionID)
	fixture.sessionsMu.Unlock()
	session := &invokeContinueFactorySession{fixture: fixture, id: sessionID}
	t.Cleanup(func() {
		if session.closed {
			return
		}
		session.close(t)
	})
	return session
}

type invokeContinueFactorySession struct {
	fixture *invokeContinuePackageFixture
	id      string
	closed  bool
}

func (session *invokeContinueFactorySession) close(t testing.TB) {
	t.Helper()
	if session.closed {
		return
	}
	support.CloseFactorySessionAt(t, session.fixture.baseURL, session.id)
	session.closed = true
	session.fixture.sessionsMu.Lock()
	session.fixture.closedSessionIDs = append(session.fixture.closedSessionIDs, session.id)
	session.fixture.deletedSessionIDs = append(session.fixture.deletedSessionIDs, session.id)
	session.fixture.sessionsMu.Unlock()
}

func (session *invokeContinueFactorySession) assertDeleted(t testing.TB) {
	t.Helper()
	if !session.closed {
		t.Fatal("assertDeleted requires a closed Factory Session")
	}
	response, err := http.Get(strings.TrimSuffix(session.fixture.baseURL, "/") + "/factory-sessions/" + url.PathEscape(session.id))
	if err != nil {
		t.Fatalf("GET deleted Factory Session %q: %v", session.id, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("GET deleted Factory Session %q status = %d, want 404", session.id, response.StatusCode)
	}
}

func (fixture *invokeContinuePackageFixture) assertSpine(t *testing.T) {
	t.Helper()
	if got := fixture.processBuilds.Load(); got != 1 {
		t.Fatalf("SPINE-001 process builds = %d, want exactly one", got)
	}
	if got := fixture.apiStarts.Load(); got != 1 {
		t.Fatalf("SPINE-001 API starts = %d, want exactly one", got)
	}
}

func (fixture *invokeContinuePackageFixture) scenario(t *testing.T, name string) *invokeContinueScenario {
	t.Helper()
	for index := range fixture.scenarios {
		if fixture.scenarios[index].name != name {
			continue
		}
		scenario := &fixture.scenarios[index]
		scenario.fixture = fixture
		scenario.runNumber = fixture.scenarioRuns.Add(1)
		if scenario.reset != nil {
			scenario.reset()
		}
		scenario.session = fixture.openSession(t)
		return scenario
	}
	t.Fatalf("invoke/continue scenario %q was not pre-registered", name)
	return nil
}

type invokeContinueExecutionSpec struct {
	requestID        string
	workerSessionID  string
	dispatchID       string
	factorySessionID string
	workingDirectory string
	userMessage      string
}

func invokeContinueExecutionDocument(spec invokeContinueExecutionSpec) map[string]any {
	return map[string]any{
		"requestId":       spec.requestID,
		"workerSessionId": spec.workerSessionID,
		"execution": map[string]any{
			"workstationName":  "direct",
			"workingDirectory": spec.workingDirectory,
			"factorySessionId": spec.factorySessionID,
			"workerType":       "direct-worker",
			"runnerId":         "codex",
			"executorProvider": "codex",
			"modelProvider":    "codex",
			"model":            "functional-model",
			"userMessage":      spec.userMessage,
			"dispatch": map[string]any{
				"dispatchId":      spec.dispatchID,
				"workstationName": "direct",
				"workerType":      "direct-worker",
			},
		},
	}
}

func writeInvokeContinueJSON(t testing.TB, path string, document map[string]any) {
	t.Helper()
	raw, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		t.Fatalf("marshal invoke/continue execution: %v", err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("write invoke/continue execution: %v", err)
	}
}

func writeInvokeContinueExecutionSpec(t testing.TB, path string, spec invokeContinueExecutionSpec) {
	t.Helper()
	writeInvokeContinueJSON(t, path, invokeContinueExecutionDocument(spec))
}

type invokeContinueBlockingProviderRunner struct {
	started       chan struct{}
	canceled      chan struct{}
	startOnce     sync.Once
	cancelOnce    sync.Once
	calls         atomic.Int32
	cancellations atomic.Int32
}

func newInvokeContinueBlockingProviderRunner() *invokeContinueBlockingProviderRunner {
	return &invokeContinueBlockingProviderRunner{started: make(chan struct{}), canceled: make(chan struct{})}
}

func (runner *invokeContinueBlockingProviderRunner) reset() {
	runner.startOnce = sync.Once{}
	runner.cancelOnce = sync.Once{}
	runner.started = make(chan struct{})
	runner.canceled = make(chan struct{})
	runner.calls.Store(0)
	runner.cancellations.Store(0)
}

func (runner *invokeContinueBlockingProviderRunner) Run(
	ctx context.Context,
	_ platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	runner.calls.Add(1)
	runner.startOnce.Do(func() { close(runner.started) })
	<-ctx.Done()
	runner.cancellations.Add(1)
	runner.cancelOnce.Do(func() { close(runner.canceled) })
	return platformprocess.CommandResult{}, ctx.Err()
}

func (runner *invokeContinueBlockingProviderRunner) CallCount() int {
	return int(runner.calls.Load())
}

func (runner *invokeContinueBlockingProviderRunner) CancellationCount() int {
	return int(runner.cancellations.Load())
}

func (runner *invokeContinueBlockingProviderRunner) CancellationObserved() <-chan struct{} {
	return runner.canceled
}

var _ platformprocess.CommandRunner = (*invokeContinueBlockingProviderRunner)(nil)

func invokeContinueEnvironment(homeDir string) []string {
	return append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
}

func writeInvokeContinueExecution(
	t testing.TB,
	path string,
	sessionID string,
	workingDirectory string,
) {
	t.Helper()
	writeInvokeContinueExecutionSpec(t, path, invokeContinueExecutionSpec{
		requestID:        "local-invoke-request",
		workerSessionID:  "local-source-session",
		dispatchID:       "local-source-dispatch",
		factorySessionID: sessionID,
		workingDirectory: workingDirectory,
		userMessage:      "initial direct prompt",
	})
}
