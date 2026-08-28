package cli_test

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const workerSessionsCLISharedShutdownTimeout = 15 * time.Second

var workerSessionsCLISharedFixtureState struct {
	sync.Once
	fixture *workerSessionsCLISharedFixture
}

var workerSessionsCLIBinaryState struct {
	sync.Once
	directory string
	path      string
	err       error
	output    []byte
	builds    atomic.Int32
}

// TestMain owns package-wide lifecycle cleanup. The process and CLI artifact
// are initialized lazily so focused OS-boundary tests do not pay for a server
// they do not exercise, while the full package still observes one topology.
func TestMain(m *testing.M) {
	exitCode := m.Run()
	sharedFixtureErr := closeWorkerSessionsCLISharedFixture()
	if sharedFixtureErr != nil {
		fmt.Fprintf(os.Stderr, "provider sessions CLI shared fixture cleanup failed: %v\n", sharedFixtureErr)
		exitCode = 1
	}
	binaryErr := closeWorkerSessionsCLIBinary()
	if binaryErr != nil {
		fmt.Fprintf(os.Stderr, "provider sessions CLI binary cleanup failed: %v\n", binaryErr)
		exitCode = 1
	}
	if err := writeForcedProviderSessionsCleanupReport(sharedFixtureErr, binaryErr); err != nil {
		fmt.Fprintf(os.Stderr, "write forced Provider Sessions CLI cleanup report: %v\n", err)
		exitCode = 1
	}
	os.Exit(exitCode)
}

// workerSessionsCLISharedFixture owns the one production-composed process
// and API host used by all hosted scenarios. Factory directories, rollout
// files, Factory Sessions, and provider routes remain case-specific.
type workerSessionsCLISharedFixture struct {
	rootDir     string
	hostFactory string
	homeDir     string
	recordPath  string
	baseURL     string

	process support.ApplicationProcess
	hosted  *workerSessionsCLIHostedCommand
	api     *workerSessionsCLIAPIServer
	runner  *providerCommandRouteRunner

	rootBuilds atomic.Int32

	fleetGate *providerCommandRouteGate

	sessionMu        sync.Mutex
	openedSessionIDs map[string]struct{}
	closedSessionIDs map[string]struct{}
}

type workerSessionsCLIAPIServer struct {
	server *support.ProcessAPIServer
	starts atomic.Int32

	stopped  chan struct{}
	stopOnce sync.Once
}

func newWorkerSessionsCLIAPIServer() *workerSessionsCLIAPIServer {
	return &workerSessionsCLIAPIServer{
		server:  support.NewProcessAPIServer(),
		stopped: make(chan struct{}),
	}
}

func (server *workerSessionsCLIAPIServer) start(
	ctx context.Context,
	request platformhttpserver.StartRequest,
) error {
	server.starts.Add(1)
	err := server.server.Start(ctx, request)
	server.stopOnce.Do(func() { close(server.stopped) })
	return err
}

type workerSessionsCLIHostedCommand struct {
	cancel context.CancelFunc
	done   chan error
}

func startWorkerSessionsCLIHostedCommand(
	process support.ApplicationProcess,
	input root.Input,
) *workerSessionsCLIHostedCommand {
	parent := input.Context
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	input.Context = ctx
	command := &workerSessionsCLIHostedCommand{
		cancel: cancel,
		done:   make(chan error, 1),
	}
	go func() { command.done <- process.Execute(input) }()
	return command
}

func (command *workerSessionsCLIHostedCommand) stop() error {
	if command == nil {
		return nil
	}
	command.cancel()
	select {
	case err := <-command.done:
		if err != nil && !errors.Is(err, context.Canceled) {
			return fmt.Errorf("stop hosted Provider Sessions CLI process: %w", err)
		}
		return nil
	case <-time.After(workerSessionsCLISharedShutdownTimeout):
		return errors.New("timed out waiting for hosted Provider Sessions CLI process shutdown")
	}
}

func workerSessionsCLIProcess(t *testing.T) *workerSessionsCLISharedFixture {
	t.Helper()
	workerSessionsCLISharedFixtureState.Do(func() {
		workerSessionsCLISharedFixtureState.fixture = newWorkerSessionsCLISharedFixture(t)
	})
	if workerSessionsCLISharedFixtureState.fixture == nil {
		t.Fatal("Provider Sessions CLI shared fixture is unavailable")
	}
	return workerSessionsCLISharedFixtureState.fixture
}

func newWorkerSessionsCLISharedFixture(t *testing.T) *workerSessionsCLISharedFixture {
	t.Helper()
	rootDir, err := os.MkdirTemp("", "c06-provider-sessions-cli-")
	if err != nil {
		t.Fatalf("create Provider Sessions CLI package root: %v", err)
	}
	keepRoot := false
	defer func() {
		if !keepRoot {
			_ = os.RemoveAll(rootDir)
		}
	}()

	hostFactory := filepath.Join(rootDir, "host-factory")
	homeDir := filepath.Join(rootDir, "home")
	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		t.Fatalf("create Provider Sessions CLI home: %v", err)
	}
	if err := copyWorkerSessionsCLIDirectory(
		support.LegacyFixtureDir(t, "executor_success"), hostFactory,
	); err != nil {
		t.Fatalf("copy Provider Sessions CLI host Factory: %v", err)
	}
	support.ClearSeedInputs(t, hostFactory)
	support.WriteAgentConfig(t, hostFactory, "worker", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "fixture-model"))
	writeWorkerSessionRouteWorkstation(t, hostFactory)

	runner, fleetGate := newWorkerSessionsCLISharedRouteRunner(t, homeDir)
	api := newWorkerSessionsCLIAPIServer()
	process, err := support.BuildProcessWithContext(context.Background(), serviceedges.Edges{
		APIServerStarter:                    api.start,
		ProviderCommandRunner:               runner,
		ProviderSessionResolveHomeDirectory: func() (string, error) { return homeDir, nil },
	})
	if err != nil {
		t.Fatalf("build Provider Sessions CLI shared process: %v", err)
	}

	recordPath := filepath.Join(rootDir, "worker-session-recording.json")
	inputs := support.FakeInputs(context.Background(), []string{
		"you", "run", "--dir", hostFactory, "--continuously", "--with-server", "--quiet", "--record", recordPath,
	})
	inputs.Input.Env = functionalEnvironment(homeDir)
	inputs.Input.WorkingDirectory = hostFactory
	hosted := startWorkerSessionsCLIHostedCommand(process, inputs.Input)
	fixture := &workerSessionsCLISharedFixture{
		rootDir:          rootDir,
		hostFactory:      hostFactory,
		homeDir:          homeDir,
		recordPath:       recordPath,
		process:          process,
		hosted:           hosted,
		api:              api,
		runner:           runner,
		fleetGate:        fleetGate,
		openedSessionIDs: make(map[string]struct{}),
		closedSessionIDs: make(map[string]struct{}),
	}
	fixture.rootBuilds.Add(1)
	fixture.baseURL = api.server.WaitForURL(t)
	keepRoot = true
	return fixture
}

func newWorkerSessionsCLISharedRouteRunner(
	t *testing.T,
	homeDir string,
) (*providerCommandRouteRunner, *providerCommandRouteGate) {
	t.Helper()
	successStdout := readProviderFixture(t, "codex", "success", "stdout.jsonl")
	successRollout := readProviderFixture(t, "codex", "success", "rollout.jsonl")
	failureStdout := readProviderFixture(t, "codex", "structured-failure", "stdout.jsonl")
	failureRollout := bytesReplaceAll(successRollout, workerSessionsCodexSuccessID, workerSessionsCodexFailureID)
	failureRollout = bytesReplaceAll(failureRollout, "Codex fixture answer COMPLETE", "Codex authentication failed.")

	routes := map[string]platformprocess.CommandResult{
		"worker-session-cli-success": {Stdout: successStdout},
		"worker-session-cli-failure": {Stdout: failureStdout, ExitCode: 1},
	}
	writeCodexRollout(t, homeDir, workerSessionsCodexSuccessID, successRollout)
	writeCodexRollout(t, homeDir, workerSessionsCodexFailureID, failureRollout)

	addSuccessRoute := func(workName, providerSessionID string) {
		stdout := bytesReplaceAll(successStdout, workerSessionsCodexSuccessID, providerSessionID)
		rollout := bytesReplaceAll(successRollout, workerSessionsCodexSuccessID, providerSessionID)
		routes[workName] = platformprocess.CommandResult{Stdout: stdout}
		writeCodexRollout(t, homeDir, providerSessionID, rollout)
	}
	addSuccessRoute("worker-session-replay-only-redirect", "session_fixture_codex_replay_redirect")
	addSuccessRoute("worker-session-cli-recovery-success", "session_fixture_codex_recovery_success")
	addSuccessRoute("wsr-ft-001", "session_fixture_codex_wsr_ft_001")
	addSuccessRoute("wsr-ft-002", "session_fixture_codex_wsr_ft_002")
	addSuccessRoute("worker-session-fleet-alpha", "session_fixture_codex_fleet_alpha")
	addSuccessRoute("worker-session-fleet-beta", "session_fixture_codex_fleet_beta")
	addSuccessRoute("worker-session-fleet-gamma", "session_fixture_codex_fleet_gamma")

	fleetGate := newProviderCommandRouteGate()
	fleetRoutes := map[string]*providerCommandRouteGate{
		"worker-session-fleet-alpha": fleetGate,
		"worker-session-fleet-beta":  fleetGate,
		"worker-session-fleet-gamma": fleetGate,
	}
	return newProviderCommandRouteRunnerWithDynamicGates(routes, fleetRoutes), fleetGate
}

// bytesReplaceAll keeps shared-fixture setup independent from the byte-slice
// mutation details at each caller and makes route fixture substitutions clear.
func bytesReplaceAll(contents []byte, old, replacement string) []byte {
	return append([]byte(nil), strings.ReplaceAll(string(contents), old, replacement)...)
}

type workerSessionsCLICase struct {
	fixture    *workerSessionsCLISharedFixture
	factoryDir string

	sessionMu          sync.Mutex
	sessionIDs         []string
	routeMu            sync.Mutex
	routeRegistrations []*providerCommandRouteRegistration
	cleanupOnce        sync.Once
}

func newWorkerSessionsCLICase(t *testing.T) *workerSessionsCLICase {
	t.Helper()
	fixture := workerSessionsCLIProcess(t)
	factoryDir, err := os.MkdirTemp(fixture.rootDir, "case-factory-")
	if err != nil {
		t.Fatalf("create Provider Sessions CLI case Factory: %v", err)
	}
	if err := copyWorkerSessionsCLIDirectory(
		support.LegacyFixtureDir(t, "executor_success"), factoryDir,
	); err != nil {
		t.Fatalf("copy Provider Sessions CLI case Factory: %v", err)
	}
	support.ClearSeedInputs(t, factoryDir)
	support.WriteAgentConfig(t, factoryDir, "worker", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "fixture-model"))
	writeWorkerSessionRouteWorkstation(t, factoryDir)

	caseFixture := &workerSessionsCLICase{fixture: fixture, factoryDir: factoryDir}
	t.Cleanup(func() { caseFixture.cleanup(t) })
	return caseFixture
}

func (caseFixture *workerSessionsCLICase) registerRoutes(t *testing.T, keys ...string) {
	t.Helper()
	for _, key := range keys {
		registration, err := caseFixture.fixture.runner.registerRoute(key)
		if err != nil {
			t.Fatalf("register Provider Sessions CLI route %q: %v", key, err)
		}
		caseFixture.routeMu.Lock()
		caseFixture.routeRegistrations = append(caseFixture.routeRegistrations, registration)
		caseFixture.routeMu.Unlock()
	}
}

func (caseFixture *workerSessionsCLICase) closeRoute(t *testing.T, key string) {
	t.Helper()
	caseFixture.routeMu.Lock()
	var registration *providerCommandRouteRegistration
	for _, candidate := range caseFixture.routeRegistrations {
		if candidate.key == key {
			registration = candidate
			break
		}
	}
	caseFixture.routeMu.Unlock()
	if registration == nil {
		t.Fatalf("Provider Sessions CLI route %q has no case registration", key)
	}
	if err := registration.Close(); err != nil {
		t.Fatalf("close Provider Sessions CLI route %q: %v", key, err)
	}
	if remaining := caseFixture.fixture.runner.activeRouteKeys(); containsString(remaining, key) {
		t.Fatalf("Provider Sessions CLI route %q remains after close: %s", key, strings.Join(remaining, ", "))
	}
}

func (caseFixture *workerSessionsCLICase) openSession(t *testing.T) string {
	t.Helper()
	sessionID := openExplicitWorkerSession(t, caseFixture.fixture.baseURL, caseFixture.factoryDir)
	caseFixture.fixture.recordSessionOpened(t, sessionID)
	caseFixture.sessionMu.Lock()
	caseFixture.sessionIDs = append(caseFixture.sessionIDs, sessionID)
	caseFixture.sessionMu.Unlock()
	return sessionID
}

func (caseFixture *workerSessionsCLICase) cleanup(t *testing.T) {
	if caseFixture == nil {
		return
	}
	caseFixture.cleanupOnce.Do(func() {
		caseFixture.routeMu.Lock()
		routeRegistrations := append([]*providerCommandRouteRegistration(nil), caseFixture.routeRegistrations...)
		caseFixture.routeMu.Unlock()
		caseFixture.sessionMu.Lock()
		sessionIDs := append([]string(nil), caseFixture.sessionIDs...)
		caseFixture.sessionMu.Unlock()
		for index := len(sessionIDs) - 1; index >= 0; index-- {
			sessionID := sessionIDs[index]
			support.CloseFactorySessionAt(t, caseFixture.fixture.baseURL, sessionID)
			assertFactorySessionAbsent(t, caseFixture.fixture.baseURL, sessionID, caseFixture.factoryDir)
			caseFixture.fixture.recordSessionClosed(sessionID)
		}
		for index := len(routeRegistrations) - 1; index >= 0; index-- {
			registration := routeRegistrations[index]
			if err := registration.Close(); err != nil {
				t.Errorf("close Provider Sessions CLI route %q: %v", registration.key, err)
			}
		}
		if remaining := caseFixture.fixture.runner.activeRouteKeys(); len(remaining) != 0 {
			t.Errorf("Provider Sessions CLI routes remain after case cleanup: %s", strings.Join(remaining, ", "))
		}
		if err := os.RemoveAll(caseFixture.factoryDir); err != nil {
			t.Errorf("remove Provider Sessions CLI case Factory %q: %v", caseFixture.factoryDir, err)
		} else if _, err := os.Stat(caseFixture.factoryDir); !os.IsNotExist(err) {
			t.Errorf("Provider Sessions CLI case Factory %q remains after cleanup: %v", caseFixture.factoryDir, err)
		}
	})
}

func (fixture *workerSessionsCLISharedFixture) recordSessionOpened(t testing.TB, sessionID string) {
	t.Helper()
	fixture.sessionMu.Lock()
	defer fixture.sessionMu.Unlock()
	if _, exists := fixture.openedSessionIDs[sessionID]; exists {
		t.Fatalf("shared Factory Session %q was opened twice", sessionID)
	}
	fixture.openedSessionIDs[sessionID] = struct{}{}
}

func (fixture *workerSessionsCLISharedFixture) recordSessionClosed(sessionID string) {
	fixture.sessionMu.Lock()
	defer fixture.sessionMu.Unlock()
	fixture.closedSessionIDs[sessionID] = struct{}{}
}

func (fixture *workerSessionsCLISharedFixture) releaseFleetGate() {
	if fixture == nil || fixture.fleetGate == nil {
		return
	}
	fixture.fleetGate.release()
}

func (fixture *workerSessionsCLISharedFixture) resetFleetGate() {
	if fixture == nil || fixture.fleetGate == nil {
		return
	}
	fixture.fleetGate.reset()
}

func (fixture *workerSessionsCLISharedFixture) sessionLifecycleError() error {
	fixture.sessionMu.Lock()
	defer fixture.sessionMu.Unlock()
	if len(fixture.openedSessionIDs) != len(fixture.closedSessionIDs) {
		return fmt.Errorf(
			"shared Factory Session lifecycle opened %d sessions but closed %d",
			len(fixture.openedSessionIDs), len(fixture.closedSessionIDs),
		)
	}
	for sessionID := range fixture.openedSessionIDs {
		if _, closed := fixture.closedSessionIDs[sessionID]; !closed {
			return fmt.Errorf("shared Factory Session %q was not closed", sessionID)
		}
	}
	return nil
}

func closeWorkerSessionsCLISharedFixture() error {
	fixture := workerSessionsCLISharedFixtureState.fixture
	if fixture == nil {
		return nil
	}
	var errs []error
	fixture.releaseFleetGate()
	if err := fixture.hosted.stop(); err != nil {
		errs = append(errs, err)
	}
	closeCtx, cancel := context.WithTimeout(context.Background(), workerSessionsCLISharedShutdownTimeout)
	if err := fixture.process.Close(closeCtx); err != nil {
		errs = append(errs, fmt.Errorf("close shared Provider Sessions CLI process: %w", err))
	}
	cancel()
	select {
	case <-fixture.api.stopped:
	case <-time.After(workerSessionsCLISharedShutdownTimeout):
		errs = append(errs, errors.New("shared Provider Sessions CLI API server did not stop"))
	}
	fmt.Fprintf(
		os.Stderr,
		"C06 TASK-002 topology: root-builds=%d api-host-starts=%d cli-builds=%d\n",
		fixture.rootBuilds.Load(), fixture.api.starts.Load(), workerSessionsCLIBinaryState.builds.Load(),
	)
	if got := fixture.api.starts.Load(); got != 1 {
		errs = append(errs, fmt.Errorf("shared Provider Sessions CLI API starts = %d, want exactly one", got))
	}
	if got := fixture.rootBuilds.Load(); got != 1 {
		errs = append(errs, fmt.Errorf("shared Provider Sessions CLI root builds = %d, want exactly one", got))
	}
	if got := workerSessionsCLIBinaryState.builds.Load(); got > 1 {
		errs = append(errs, fmt.Errorf("Provider Sessions CLI binary builds = %d, want at most one", got))
	}
	if got := fixture.runner.ActiveCallCount(); got != 0 {
		errs = append(errs, fmt.Errorf("active Provider Sessions CLI command routes after cleanup = %d", got))
	}
	if err := fixture.runner.close(); err != nil {
		errs = append(errs, fmt.Errorf("close Provider Sessions CLI route registry: %w", err))
	}
	fmt.Fprintf(os.Stderr, "C06 TASK-002 cleanup: active-provider-routes=%d\n", fixture.runner.routeCount())
	if err := fixture.sessionLifecycleError(); err != nil {
		errs = append(errs, err)
	}
	if err := assertWorkerSessionsCLIListenerClosed(fixture.baseURL); err != nil {
		errs = append(errs, err)
	}
	if err := os.RemoveAll(fixture.rootDir); err != nil {
		errs = append(errs, fmt.Errorf("remove shared Provider Sessions CLI root %q: %w", fixture.rootDir, err))
	} else if _, err := os.Stat(fixture.rootDir); !os.IsNotExist(err) {
		errs = append(errs, fmt.Errorf("shared Provider Sessions CLI root %q remains after cleanup: %v", fixture.rootDir, err))
	}
	return errors.Join(errs...)
}

func assertWorkerSessionsCLIListenerClosed(baseURL string) error {
	client := &http.Client{Timeout: 250 * time.Millisecond}
	defer client.CloseIdleConnections()
	request, err := http.NewRequest(http.MethodGet, strings.TrimSuffix(baseURL, "/")+"/status", nil)
	if err != nil {
		return fmt.Errorf("build shared API shutdown probe: %w", err)
	}
	response, err := client.Do(request)
	if err != nil {
		return nil
	}
	defer response.Body.Close()
	return fmt.Errorf("shared Provider Sessions CLI listener remains reachable with status %d", response.StatusCode)
}

func copyWorkerSessionsCLIDirectory(srcDir, dstDir string) error {
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return err
	}
	return filepath.WalkDir(srcDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		if relative == "." {
			return nil
		}
		target := filepath.Join(dstDir, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, contents, 0o644)
	})
}

func closeWorkerSessionsCLIBinary() error {
	if workerSessionsCLIBinaryState.directory == "" {
		return nil
	}
	if err := os.RemoveAll(workerSessionsCLIBinaryState.directory); err != nil {
		return fmt.Errorf("remove cached Provider Sessions CLI binary directory: %w", err)
	}
	if _, err := os.Stat(workerSessionsCLIBinaryState.directory); !os.IsNotExist(err) {
		return fmt.Errorf("cached Provider Sessions CLI binary directory remains: %v", err)
	}
	return nil
}

func cachedWorkerSessionsCLIBinary(t *testing.T) string {
	t.Helper()
	workerSessionsCLIBinaryState.Do(func() {
		workerSessionsCLIBinaryState.directory, workerSessionsCLIBinaryState.err = os.MkdirTemp("", "c06-provider-sessions-cli-bin-")
		if workerSessionsCLIBinaryState.err != nil {
			return
		}
		name := "you"
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		workerSessionsCLIBinaryState.path = filepath.Join(workerSessionsCLIBinaryState.directory, name)
		build := exec.Command("go", "build", "-o", workerSessionsCLIBinaryState.path, "./cmd/factory")
		build.Dir = testutil.MustRepoRoot(t)
		workerSessionsCLIBinaryState.output, workerSessionsCLIBinaryState.err = build.CombinedOutput()
		if workerSessionsCLIBinaryState.err == nil {
			workerSessionsCLIBinaryState.builds.Add(1)
		}
	})
	if workerSessionsCLIBinaryState.err != nil {
		t.Fatalf("build Worker Sessions CLI binary: %v\n%s", workerSessionsCLIBinaryState.err, workerSessionsCLIBinaryState.output)
	}
	return workerSessionsCLIBinaryState.path
}
