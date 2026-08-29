package acceptance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
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
	factoryinterfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	providerswire "github.com/portpowered/infinite-you/pkg/services/providers/wire"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
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
	unsupportedProvider *unsupportedContinuationProvider
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

	invokeContinuePackageFixtureState.Lock()
	fixture := invokeContinuePackageFixtureState.fixture
	invokeContinuePackageFixtureState.Unlock()
	if fixture != nil {
		return fixture
	}

	created, err := newInvokeContinuePackageFixture(t)
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

func newInvokeContinuePackageFixture(t *testing.T) (*invokeContinuePackageFixture, error) {
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

	hostDir := filepath.Join(rootDir, "host-factory")
	homeDir := filepath.Join(rootDir, "home")
	if err := copyInvokeContinueDirectory(
		support.LegacyFixtureDir(t, "executor_success"),
		hostDir,
	); err != nil {
		return nil, fmt.Errorf("copy host Factory: %w", err)
	}
	// The copied fixture is used only to provide a valid Factory definition for
	// the explicit session. Removing its seed inputs prevents the hosted
	// default session from consuming the CASE-01 provider results at startup.
	if err := os.RemoveAll(filepath.Join(hostDir, factoryinterfaces.InputsDir)); err != nil {
		return nil, fmt.Errorf("clear host Factory seed inputs: %w", err)
	}
	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		return nil, fmt.Errorf("create package fixture home %q: %w", homeDir, err)
	}

	scenarios := make([]invokeContinueScenario, 0, 16)
	routes := make([]invokeContinueStaticCommandRouteEntry, 0, 16)
	addScenario := func(
		name string,
		runner platformprocess.CommandRunner,
		providerRunner invokeContinueProviderCommandRunner,
		streamingRunner *wsrFT015StreamingProviderRunner,
		blockingRunner *invokeContinueBlockingProviderRunner,
		unsupportedProvider *unsupportedContinuationProvider,
		reset func(),
	) error {
		workingDirectory := filepath.Join(rootDir, "routes", name)
		scenarioHome := filepath.Join(rootDir, "scenario-homes", name)
		for _, dir := range []string{workingDirectory, scenarioHome} {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return fmt.Errorf("create %s scenario directory %q: %w", name, dir, err)
			}
		}
		scenarios = append(scenarios, invokeContinueScenario{
			name:                name,
			workingDirectory:    workingDirectory,
			homeDirectory:       scenarioHome,
			providerRunner:      providerRunner,
			streamingRunner:     streamingRunner,
			blockingRunner:      blockingRunner,
			unsupportedProvider: unsupportedProvider,
			reset:               reset,
		})
		routes = append(routes, invokeContinueStaticCommandRouteEntry{
			workingDirectory: workingDirectory,
			runner:           runner,
		})
		return nil
	}

	localRunner := testutil.NewProviderCommandRunner(
		platformprocess.CommandResult{Stdout: directCodexSessionOutput("local-source-thread", "initial direct output COMPLETE")},
		platformprocess.CommandResult{Stdout: directCodexSessionOutput("local-source-thread", "continued direct output COMPLETE")},
	)
	if err := addScenario("local", localRunner, localRunner, nil, nil, nil, nil); err != nil {
		return nil, err
	}
	futureRunner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{
		Stdout: directCodexSessionOutput("future-file-thread", "future-file output COMPLETE"),
	})
	if err := addScenario("future-fields", futureRunner, futureRunner, nil, nil, nil, nil); err != nil {
		return nil, err
	}
	streamingRunner := newWSRFT015StreamingProviderRunner()
	if err := addScenario("recorded-provider-session", streamingRunner, nil, streamingRunner, nil, nil, nil); err != nil {
		return nil, err
	}
	unassociatedRunner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{
		Stdout: directCodexOutputWithoutSession("completed without a Provider Session"),
	})
	if err := addScenario("unassociated-source", unassociatedRunner, unassociatedRunner, nil, nil, nil, nil); err != nil {
		return nil, err
	}
	staleRunner := testutil.NewProviderCommandRunner(
		platformprocess.CommandResult{Stdout: directCodexSessionOutput("stale-source-thread", "initial output")},
		platformprocess.CommandResult{
			Stderr:   []byte("Error: thread/resume failed: no rollout found for thread id stale-source-thread"),
			ExitCode: 1,
		},
	)
	if err := addScenario("stale-provider-session", staleRunner, staleRunner, nil, nil, nil, nil); err != nil {
		return nil, err
	}
	duplicateRunner := newInvokeContinueResettableProviderCommandRunner(
		platformprocess.CommandResult{Stdout: directCodexSessionOutput("duplicate-source-thread", "duplicate initial output")},
		platformprocess.CommandResult{Stdout: directCodexSessionOutput("duplicate-source-thread", "duplicate continued output")},
	)
	if err := addScenario("duplicate-continuation", duplicateRunner, duplicateRunner, nil, nil, nil, duplicateRunner.Reset); err != nil {
		return nil, err
	}
	blockingRunner := newInvokeContinueBlockingProviderRunner()
	if err := addScenario("dependency-cancellation", blockingRunner, nil, nil, blockingRunner, nil, blockingRunner.reset); err != nil {
		return nil, err
	}
	recoveryRunner := newInvokeContinueResettableProviderCommandRunner(platformprocess.CommandResult{
		Stdout: directCodexSessionOutput("timeout-recovery-thread", "timeout recovery output COMPLETE"),
	})
	if err := addScenario("cancellation-recovery", recoveryRunner, recoveryRunner, nil, nil, nil, recoveryRunner.Reset); err != nil {
		return nil, err
	}
	unsupportedProvider := &unsupportedContinuationProvider{
		MockProvider: testutil.NewMockProvider(workerexecution.InferenceResponse{
			Content: "initial provider output",
			ProviderSession: &providers.SessionMetadata{
				Provider: string(providers.IDCodex),
				Kind:     providers.SessionIDKind,
				ID:       "unsupported-source-thread",
			},
		}),
	}
	unsupportedRunner := testutil.NewProviderCommandRunner()
	if err := addScenario("unsupported-provider", unsupportedRunner, unsupportedRunner, nil, nil, unsupportedProvider, nil); err != nil {
		return nil, err
	}
	for _, name := range []string{
		"unknown-source",
		"empty-input",
		"remote-interrupt",
		"remote-controls",
		"remote-continue-failures",
		"remote-stream-failure",
		"remote-cancellation",
	} {
		runner := testutil.NewProviderCommandRunner()
		if err := addScenario(name, runner, runner, nil, nil, nil, nil); err != nil {
			return nil, err
		}
	}

	managerRepositoryA, err := newS8RepositoryAt(
		filepath.Join(rootDir, "routes", "manager-isolation-a"), s8RepositoryAMarker,
	)
	if err != nil {
		return nil, fmt.Errorf("create manager isolation repository A: %w", err)
	}
	managerRepositoryB, err := newS8RepositoryAt(
		filepath.Join(rootDir, "routes", "manager-isolation-b"), s8RepositoryBMarker,
	)
	if err != nil {
		return nil, fmt.Errorf("create manager isolation repository B: %w", err)
	}
	stdout := readS8ProviderFixture(t, "stdout.jsonl")
	rollout := readS8ProviderFixture(t, "rollout.jsonl")
	managerRunner := newS8RemoteProviderRunner(stdout,
		s8RemoteProviderCase{
			repository: managerRepositoryA.path, marker: managerRepositoryA.marker,
			sessionID: s8ProviderSessionA, output: s8OutputA,
			release: make(chan struct{}), started: make(chan struct{}),
		},
		s8RemoteProviderCase{
			repository: managerRepositoryB.path, marker: managerRepositoryB.marker,
			sessionID: s8ProviderSessionB, output: s8OutputB,
			release: make(chan struct{}), started: make(chan struct{}),
		},
	)
	writeS8CodexRollout(t, homeDir, s8ProviderSessionA, rollout, s8OutputA)
	writeS8CodexRollout(t, homeDir, s8ProviderSessionB, rollout, s8OutputB)
	if err := addScenario("manager-isolation", managerRunner, managerRunner, nil, nil, nil, managerRunner.reset); err != nil {
		return nil, err
	}
	// Manager requests execute in the repository-specific WorkDir, while the
	// scenario session is opened against the host Factory. Keep both repository
	// routes fixed before process construction so concurrent calls cannot select
	// a runner from mutable session state or request order.
	routes = append(routes,
		invokeContinueStaticCommandRouteEntry{workingDirectory: managerRepositoryA.path, runner: managerRunner},
		invokeContinueStaticCommandRouteEntry{workingDirectory: managerRepositoryB.path, runner: managerRunner},
	)

	interruptRepositoryA, err := newS8RepositoryAt(
		filepath.Join(rootDir, "routes", "manager-interrupt-a"), s8RepositoryAMarker,
	)
	if err != nil {
		return nil, fmt.Errorf("create manager interrupt repository A: %w", err)
	}
	interruptRepositoryB, err := newS8RepositoryAt(
		filepath.Join(rootDir, "routes", "manager-interrupt-b"), s8RepositoryBMarker,
	)
	if err != nil {
		return nil, fmt.Errorf("create manager interrupt repository B: %w", err)
	}
	interruptRunner := newS8InterruptProviderRunner(stdout, interruptRepositoryA, interruptRepositoryB)
	writeS8CodexRollout(t, homeDir, s8InterruptProviderSessionA, rollout, s8ReplacementOutput)
	writeS8CodexRollout(t, homeDir, s8InterruptProviderSessionB, rollout, s8OutputB)
	if err := addScenario("manager-interrupt", interruptRunner, interruptRunner, nil, nil, nil, interruptRunner.reset); err != nil {
		return nil, err
	}
	routes = append(routes,
		invokeContinueStaticCommandRouteEntry{workingDirectory: interruptRepositoryA.path, runner: interruptRunner},
		invokeContinueStaticCommandRouteEntry{workingDirectory: interruptRepositoryB.path, runner: interruptRunner},
	)

	route := &invokeContinueStaticCommandRoute{routes: routes}
	fallbackProvider, err := providerswire.NewService(providerswire.WithCommandRunner(route))
	if err != nil {
		return nil, fmt.Errorf("build fixture provider fallback: %w", err)
	}
	unsupportedWorkingDirectory := filepath.Join(rootDir, "routes", "unsupported-provider")
	providerOverride := &invokeContinueProviderRouter{
		fallback:                    fallbackProvider,
		unsupported:                 unsupportedProvider,
		unsupportedWorkingDirectory: unsupportedWorkingDirectory,
	}
	api := support.NewProcessAPIServer()
	apiStopped := make(chan struct{})
	var apiStopOnce sync.Once
	apiStarts := &atomic.Int32{}
	processBuilds := &atomic.Int32{}

	processBuilds.Add(1)
	process, err := support.BuildProcessWithContext(context.Background(), serviceedges.Edges{
		// This route is complete before root construction and has no registration
		// or session-based fallback after the process starts.
		ProviderCommandRunner: route,
		ProviderOverride:      providerOverride,
		ProviderSessionResolveHomeDirectory: func() (string, error) {
			return homeDir, nil
		},
		APIServerStarter: func(ctx context.Context, request platformhttpserver.StartRequest) error {
			apiStarts.Add(1)
			err := api.Start(ctx, request)
			apiStopOnce.Do(func() { close(apiStopped) })
			return err
		},
	})
	if err != nil {
		return nil, fmt.Errorf("BuildProcess: %w", err)
	}

	inputs := support.FakeInputs(context.Background(), []string{
		"you", "run",
		"--dir", hostDir,
		"--continuously",
		"--with-server",
		"--server", "http://127.0.0.1:1",
		"--quiet",
		"--no-record",
	})
	inputs.Input.Env = invokeContinueEnvironment(homeDir)
	inputs.Input.WorkingDirectory = hostDir
	command := startInvokeContinuePackageCommand(process, inputs)
	baseURL, err := api.WaitForBaseURL(invokeContinuePackageFixtureTimeout)
	if err != nil {
		_ = command.stop()
		closeCtx, cancel := context.WithTimeout(context.Background(), invokeContinuePackageFixtureTimeout)
		_ = process.Close(closeCtx)
		cancel()
		return nil, fmt.Errorf("wait for package fixture API: %w", err)
	}

	keepRoot = true
	return &invokeContinuePackageFixture{
		rootDir:              rootDir,
		hostDir:              hostDir,
		homeDir:              homeDir,
		baseURL:              baseURL,
		process:              process,
		command:              command,
		router:               route,
		apiStopped:           apiStopped,
		apiStarts:            apiStarts,
		processBuilds:        processBuilds,
		scenarios:            scenarios,
		managerRunner:        managerRunner,
		managerRepositoryA:   managerRepositoryA,
		managerRepositoryB:   managerRepositoryB,
		interruptRunner:      interruptRunner,
		interruptRepositoryA: interruptRepositoryA,
		interruptRepositoryB: interruptRepositoryB,
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

type invokeContinueStaticCommandRouteEntry struct {
	workingDirectory string
	runner           platformprocess.CommandRunner
}

type invokeContinueStaticCommandRoute struct {
	mu          sync.RWMutex
	routes      []invokeContinueStaticCommandRouteEntry
	requestLog  []platformprocess.CommandRequest
	activeCalls atomic.Int32
}

func (route *invokeContinueStaticCommandRoute) Close() {
	if route == nil {
		return
	}
	route.mu.Lock()
	route.routes = nil
	route.mu.Unlock()
}

func (route *invokeContinueStaticCommandRoute) routeCount() int {
	if route == nil {
		return 0
	}
	route.mu.RLock()
	defer route.mu.RUnlock()
	return len(route.routes)
}

func (route *invokeContinueStaticCommandRoute) activeCallCount() int {
	if route == nil {
		return 0
	}
	return int(route.activeCalls.Load())
}

func (route *invokeContinueStaticCommandRoute) requests() []platformprocess.CommandRequest {
	if route == nil {
		return nil
	}
	route.mu.RLock()
	defer route.mu.RUnlock()
	requests := make([]platformprocess.CommandRequest, len(route.requestLog))
	for index, request := range route.requestLog {
		requests[index] = cloneS8CommandRequest(request)
	}
	return requests
}

func (route *invokeContinueStaticCommandRoute) recordRequest(request platformprocess.CommandRequest) {
	route.mu.Lock()
	route.requestLog = append(route.requestLog, cloneS8CommandRequest(request))
	route.mu.Unlock()
}

// invokeContinueResettableProviderCommandRunner keeps the immutable route
// stable while allowing -count repetitions to receive a fresh ordered ledger.
// The package fixture still chooses this runner only from the pre-registered
// WorkDir route; Reset is test-process reuse, not runtime route mutation.
type invokeContinueResettableProviderCommandRunner struct {
	mu      sync.RWMutex
	results []platformprocess.CommandResult
	runner  *testutil.ProviderCommandRunner
}

func newInvokeContinueResettableProviderCommandRunner(
	results ...platformprocess.CommandResult,
) *invokeContinueResettableProviderCommandRunner {
	runner := &invokeContinueResettableProviderCommandRunner{
		results: append([]platformprocess.CommandResult(nil), results...),
	}
	runner.Reset()
	return runner
}

func (runner *invokeContinueResettableProviderCommandRunner) Reset() {
	runner.mu.Lock()
	runner.runner = testutil.NewProviderCommandRunner(runner.results...)
	runner.mu.Unlock()
}

func (runner *invokeContinueResettableProviderCommandRunner) current() *testutil.ProviderCommandRunner {
	runner.mu.RLock()
	defer runner.mu.RUnlock()
	return runner.runner
}

func (runner *invokeContinueResettableProviderCommandRunner) Run(
	ctx context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	return runner.current().Run(ctx, request)
}

func (runner *invokeContinueResettableProviderCommandRunner) CallCount() int {
	return runner.current().CallCount()
}

func (runner *invokeContinueResettableProviderCommandRunner) Requests() []platformprocess.CommandRequest {
	return runner.current().Requests()
}

var _ invokeContinueProviderCommandRunner = (*invokeContinueResettableProviderCommandRunner)(nil)

var _ platformprocess.CommandRunner = (*invokeContinueResettableProviderCommandRunner)(nil)

// Run selects only a route fixed before process construction. It deliberately
// has no mutable map, request-order fallback, or Factory Session lookup.
func (route *invokeContinueStaticCommandRoute) Run(
	ctx context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	if route == nil {
		return platformprocess.CommandResult{}, errors.New("invoke/continue provider route is unavailable")
	}
	route.activeCalls.Add(1)
	defer route.activeCalls.Add(-1)
	route.recordRequest(request)
	entry, err := route.entry(request)
	if err != nil {
		return platformprocess.CommandResult{}, err
	}
	return entry.runner.Run(ctx, request)
}

// RunStreaming preserves the optional streaming capability of a scenario
// runner while retaining the same immutable WorkDir-only route selection.
// Provider adapters use this extension for live Worker Session observations;
// a non-streaming scenario falls back to one completed chunk per stream.
func (route *invokeContinueStaticCommandRoute) RunStreaming(
	ctx context.Context,
	request platformprocess.CommandRequest,
	observer platformprocess.OutputChunkObserver,
) (platformprocess.CommandResult, error) {
	if route == nil {
		return platformprocess.CommandResult{}, errors.New("invoke/continue provider route is unavailable")
	}
	route.activeCalls.Add(1)
	defer route.activeCalls.Add(-1)
	route.recordRequest(request)
	entry, err := route.entry(request)
	if err != nil {
		return platformprocess.CommandResult{}, err
	}
	if streaming, ok := entry.runner.(interface {
		RunStreaming(context.Context, platformprocess.CommandRequest, platformprocess.OutputChunkObserver) (platformprocess.CommandResult, error)
	}); ok {
		return streaming.RunStreaming(ctx, request, observer)
	}
	result, runErr := entry.runner.Run(ctx, request)
	if observer != nil {
		if len(result.Stdout) > 0 {
			observer(platformprocess.OutputStreamStdout, append([]byte(nil), result.Stdout...))
		}
		if len(result.Stderr) > 0 {
			observer(platformprocess.OutputStreamStderr, append([]byte(nil), result.Stderr...))
		}
	}
	return result, runErr
}

func (route *invokeContinueStaticCommandRoute) entry(
	request platformprocess.CommandRequest,
) (invokeContinueStaticCommandRouteEntry, error) {
	if route == nil {
		return invokeContinueStaticCommandRouteEntry{}, errors.New("invoke/continue provider route is unavailable")
	}
	route.mu.RLock()
	defer route.mu.RUnlock()
	for _, entry := range route.routes {
		if filepath.Clean(request.WorkDir) != filepath.Clean(entry.workingDirectory) {
			continue
		}
		if entry.runner == nil {
			return invokeContinueStaticCommandRouteEntry{}, fmt.Errorf("invoke/continue provider route for WorkDir %q is unavailable", request.WorkDir)
		}
		return entry, nil
	}
	return invokeContinueStaticCommandRouteEntry{}, fmt.Errorf("no invoke/continue provider route matched WorkDir %q", request.WorkDir)
}

var _ platformprocess.CommandRunner = (*invokeContinueStaticCommandRoute)(nil)

type invokeContinueProviderRouter struct {
	fallback                    providers.Service
	unsupported                 providers.Service
	unsupportedWorkingDirectory string
}

func (router *invokeContinueProviderRouter) Execute(
	ctx context.Context,
	request providers.ExecuteRequest,
) (providers.ExecuteResult, error) {
	if router.matchesUnsupported(request.WorkingDirectory) {
		return router.unsupported.Execute(ctx, request)
	}
	return router.fallback.Execute(ctx, request)
}

func (router *invokeContinueProviderRouter) Continue(
	ctx context.Context,
	request providers.ContinueRequest,
) (providers.ContinueResult, error) {
	if router.matchesUnsupported(request.Attempt.WorkingDirectory) {
		return router.unsupported.Continue(ctx, request)
	}
	return router.fallback.Continue(ctx, request)
}

func (router *invokeContinueProviderRouter) ContinueReference(
	ctx context.Context,
	request providers.ContinueReferenceRequest,
) (providers.ContinueReferenceResult, error) {
	if router.matchesUnsupported(request.Attempt.WorkingDirectory) {
		return router.unsupported.ContinueReference(ctx, request)
	}
	return router.fallback.ContinueReference(ctx, request)
}

func (router *invokeContinueProviderRouter) matchesUnsupported(workingDirectory string) bool {
	return router != nil && router.unsupported != nil && filepath.Clean(workingDirectory) == filepath.Clean(router.unsupportedWorkingDirectory)
}

func (router *invokeContinueProviderRouter) ListProviders(
	ctx context.Context,
	request providers.ListProvidersRequest,
) (providers.ListProvidersResult, error) {
	return router.fallback.ListProviders(ctx, request)
}

func (router *invokeContinueProviderRouter) GetProvider(
	ctx context.Context,
	request providers.GetProviderRequest,
) (providers.GetProviderResult, error) {
	return router.fallback.GetProvider(ctx, request)
}

func (router *invokeContinueProviderRouter) ResolveIdentity(
	ctx context.Context,
	request providers.ResolveIdentityRequest,
) (providers.ResolveIdentityResult, error) {
	return router.fallback.ResolveIdentity(ctx, request)
}

func (router *invokeContinueProviderRouter) ResolveSelection(
	ctx context.Context,
	request providers.ResolveSelectionRequest,
) (providers.ResolveSelectionResult, error) {
	return router.fallback.ResolveSelection(ctx, request)
}

func (router *invokeContinueProviderRouter) ValidatePrerequisites(
	ctx context.Context,
	request providers.ValidatePrerequisitesRequest,
) error {
	return router.fallback.ValidatePrerequisites(ctx, request)
}

func (router *invokeContinueProviderRouter) ControlAttempt(
	ctx context.Context,
	request providers.ControlAttemptRequest,
) (providers.ControlAttemptResult, error) {
	return router.fallback.ControlAttempt(ctx, request)
}

var _ providers.Service = (*invokeContinueProviderRouter)(nil)

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

func copyInvokeContinueDirectory(sourceDir, targetDir string) error {
	return filepath.WalkDir(sourceDir, func(path string, entry fs.DirEntry, walkErr error) error {
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
		info, err := entry.Info()
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			return err
		}
		return os.WriteFile(targetPath, data, info.Mode().Perm())
	})
}
