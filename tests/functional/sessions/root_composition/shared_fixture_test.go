package root_composition_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

var (
	errRootCompositionRouteNotFound     = errors.New("root composition route not found")
	errRootCompositionRouteAmbiguous    = errors.New("root composition route is ambiguous")
	errRootCompositionRouteDuplicate    = errors.New("root composition route is already registered")
	errRootCompositionRouteOverlap      = errors.New("root composition route overlaps a registered route")
	errRootCompositionRouteCrossing     = errors.New("root composition route does not own the requested path")
	errRootCompositionRouteClosed       = errors.New("root composition route is closed")
	errRootCompositionRouteRegistration = errors.New("root composition route registration is closed")
)

// The API starter is an argument-free edge. Process.Execute preserves its
// input context through the initializer, so the server helper carries the
// route selector explicitly instead of relying on package-global active state.
type rootCompositionRouteContextKey struct{}

func withRootCompositionRouteContext(ctx context.Context, route *rootCompositionRoute) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, rootCompositionRouteContextKey{}, route)
}

func rootCompositionRouteFromContext(ctx context.Context) (*rootCompositionRoute, bool) {
	if ctx == nil {
		return nil, false
	}
	route, ok := ctx.Value(rootCompositionRouteContextKey{}).(*rootCompositionRoute)
	return route, ok && route != nil
}

type rootCompositionFixtureState struct {
	once    sync.Once
	fixture *rootCompositionFixture
	err     error
}

var sharedRootCompositionFixture rootCompositionFixtureState

// TestMain owns the one package process after all parallel scenarios have
// finished. The process is not initialized here: inert tests must be able to
// prove that package setup does not activate a Factory Session.
func TestMain(m *testing.M) {
	exitCode := m.Run()
	firstCloseErr := closeRootCompositionFixture()
	secondCloseErr := closeRootCompositionFixture()
	if err := errors.Join(firstCloseErr, secondCloseErr); err != nil {
		fmt.Fprintf(os.Stderr, "root composition fixture cleanup (including repeated close): %v\n", err)
		if exitCode == 0 {
			exitCode = 1
		}
	}
	os.Exit(exitCode)
}

type rootCompositionFixture struct {
	process   support.ApplicationProcess
	routes    *rootCompositionRouteRegistry
	effects   *rootCompositionSessionEffects
	cleanup   *rootCompositionCleanupLedger
	apiStarts atomic.Int64

	hostDir         string
	hostHome        string
	hostRoute       *rootCompositionRoute
	hostAPI         *support.ProcessAPIServer
	hostURL         string
	hostRecordPath  string
	hostLogsRoot    string
	hostMetricsRoot string
	hostMu          sync.Mutex
	hostStarting    chan struct{}
	hostStopped     bool
	hostGeneration  uint64
	hostErr         error
	hostCmd         *rootCompositionProcessCommand
}

func ensureRootCompositionFixture(t testing.TB) *rootCompositionFixture {
	t.Helper()
	sharedRootCompositionFixture.once.Do(func() {
		sharedRootCompositionFixture.fixture, sharedRootCompositionFixture.err = newRootCompositionFixture()
	})
	if sharedRootCompositionFixture.err != nil {
		t.Fatalf("construct shared root composition fixture: %v", sharedRootCompositionFixture.err)
	}
	return sharedRootCompositionFixture.fixture
}

func newRootCompositionFixture() (*rootCompositionFixture, error) {
	routes := newRootCompositionRouteRegistry()
	hostDir, hostHome, err := newRootCompositionHost()
	if err != nil {
		return nil, err
	}
	fixture := &rootCompositionFixture{
		routes:   routes,
		hostDir:  hostDir,
		hostHome: hostHome,
	}
	fixture.hostRoute, err = routes.register(rootCompositionRouteSpec{
		label:      "shared-host",
		homeDir:    hostHome,
		workingDir: hostDir,
	})
	if err != nil {
		return nil, errors.Join(err, os.RemoveAll(hostDir), os.RemoveAll(hostHome))
	}
	fixture.effects = newRootCompositionSessionEffects(routes, &fixture.apiStarts)
	process, err := support.BuildProcessWithContext(context.Background(), fixture.edges())
	if err != nil {
		return nil, errors.Join(err, os.RemoveAll(hostDir), os.RemoveAll(hostHome))
	}
	fixture.process = process

	cleanup := newRootCompositionCleanupLedger()
	if _, err := cleanup.register("shared application process", func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return process.Close(ctx)
	}); err != nil {
		return nil, errors.Join(err, process.Close(context.Background()), os.RemoveAll(hostDir), os.RemoveAll(hostHome))
	}
	fixture.cleanup = cleanup
	fixture.effects.captureConstructionSnapshot()
	return fixture, nil
}

func newRootCompositionHost() (string, string, error) {
	hostDir, err := os.MkdirTemp("", "root-composition-host-")
	if err != nil {
		return "", "", err
	}
	hostHome, err := os.MkdirTemp("", "root-composition-home-")
	if err != nil {
		return "", "", errors.Join(err, os.RemoveAll(hostDir))
	}
	config := map[string]any{
		"name": "root-composition-shared-host",
		"workTypes": []any{map[string]any{
			"name": "task",
			"states": []any{
				map[string]any{"name": "init", "type": "INITIAL"},
				map[string]any{"name": "complete", "type": "TERMINAL"},
				map[string]any{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []any{map[string]any{"name": "processor"}},
		"workstations": []any{map[string]any{
			"name":      "process",
			"worker":    "processor",
			"inputs":    []any{map[string]any{"workType": "task", "state": "init"}},
			"outputs":   []any{map[string]any{"workType": "task", "state": "complete"}},
			"onFailure": []any{map[string]any{"workType": "task", "state": "failed"}},
		}},
	}
	payload, err := json.Marshal(config)
	if err != nil {
		return "", "", errors.Join(err, os.RemoveAll(hostDir), os.RemoveAll(hostHome))
	}
	if err := os.WriteFile(filepath.Join(hostDir, "factory.json"), payload, 0o600); err != nil {
		return "", "", errors.Join(err, os.RemoveAll(hostDir), os.RemoveAll(hostHome))
	}
	return hostDir, hostHome, nil
}

func closeRootCompositionFixture() error {
	fixture := sharedRootCompositionFixture.fixture
	if fixture == nil {
		return sharedRootCompositionFixture.err
	}
	var errs []error
	fixture.hostMu.Lock()
	hostCmd := fixture.hostCmd
	fixture.hostStopped = true
	fixture.hostURL = ""
	fixture.hostMu.Unlock()
	if hostCmd != nil {
		if err := hostCmd.stop(); err != nil {
			errs = append(errs, fmt.Errorf("stop shared host Process.Execute: %w", err))
		}
	}
	if err := fixture.routes.cleanup(); err != nil {
		errs = append(errs, err)
	}
	if err := fixture.cleanup.cleanup(); err != nil {
		errs = append(errs, err)
	}
	if err := os.RemoveAll(fixture.hostDir); err != nil {
		errs = append(errs, fmt.Errorf("remove shared host directory: %w", err))
	}
	if err := os.RemoveAll(fixture.hostHome); err != nil {
		errs = append(errs, fmt.Errorf("remove shared host home: %w", err))
	}
	return errors.Join(errs...)
}

func (fixture *rootCompositionFixture) edges() serviceedges.Edges {
	return serviceedges.Edges{
		APIServerStarter:                           fixture.startAPIServer,
		ProviderOverride:                           fixture.routes.providerOverride(),
		ProviderCommandRunner:                      fixture.routes.commandRunner("provider"),
		ScriptCommandRunner:                        fixture.routes.commandRunner("script"),
		FactorySessionExecutionOpeningFileSystem:   &rootCompositionExecutionOpeningFileSystem{effects: fixture.effects},
		FactorySessionDirectoryInspection:          &rootCompositionDirectoryInspection{effects: fixture.effects},
		FactorySessionResolveHomeDirectory:         os.UserHomeDir,
		FactorySessionResolveLogicalTargetSymlinks: fixture.effects.resolveLogicalTargetSymlinks,
		FactorySessionIDGenerator:                  fixture.effects.nextSessionID,
		FactorySessionRuntimeInstanceIDGenerator:   fixture.effects.nextRuntimeInstanceID,
		FactorySessionResponseEventIDGenerator:     fixture.effects.nextResponseEventID,
		FactorySessionCursorPersistenceFileSystem: &rootCompositionCursorPersistenceFileSystem{
			effects: fixture.effects,
		},
		FactorySessionCursorCreateTemporaryFile: fixture.effects.createCursorTemporaryFile,
		FactorySessionRuntimePersistenceFileSystem: &rootCompositionRuntimePersistenceFileSystem{
			effects: fixture.effects,
		},
		FactorySessionContractFixtureReader: fixture.effects.readContractFixture,
		FactorySessionInvocationInputReader: fixture.effects.readInvocationInput,
		FactorySessionReplayRecordingReader: fixture.effects.readReplayRecording,
		FactorySessionInitialWorkReader:     fixture.effects.readInitialWork,
		RecordingReadFile:                   fixture.effects.readReplayRecording,
		InvocationMetricsRecorder:           fixture.effects,
		RuntimeHostObserver:                 fixture.effects.observeRuntimeHost,
		WorkRequestIDGenerator:              fixture.effects.nextWorkRequestID,
	}
}

func (fixture *rootCompositionFixture) startAPIServer(ctx context.Context, request platformhttpserver.StartRequest) error {
	if route, ok := rootCompositionRouteFromContext(ctx); ok {
		route.mu.Lock()
		apiStarter := route.apiStarter
		api := route.api
		route.mu.Unlock()
		if apiStarter != nil {
			fixture.apiStarts.Add(1)
			return apiStarter(ctx, request)
		}
		if api != nil {
			fixture.apiStarts.Add(1)
			return api.Start(ctx, request)
		}
		if route != fixture.hostRoute {
			fixture.routes.unmatched.Add(1)
			return fmt.Errorf("API server route %q has no server", route.label)
		}
	}
	fixture.hostMu.Lock()
	api := fixture.hostAPI
	fixture.hostMu.Unlock()
	if api == nil {
		fixture.routes.unmatched.Add(1)
		return errors.New("shared host API server is not initialized")
	}
	fixture.apiStarts.Add(1)
	return api.Start(ctx, request)
}

func (fixture *rootCompositionFixture) startSharedHost(t testing.TB) string {
	t.Helper()
	for {
		fixture.hostMu.Lock()
		if fixture.hostStarting != nil {
			ready := fixture.hostStarting
			fixture.hostMu.Unlock()
			<-ready
			continue
		}
		if fixture.hostCmd != nil && !fixture.hostStopped && fixture.hostErr == nil && fixture.hostURL != "" {
			baseURL := fixture.hostURL
			fixture.hostMu.Unlock()
			return baseURL
		}

		fixture.hostGeneration++
		generation := fixture.hostGeneration
		api := support.NewProcessAPIServer()
		recordPath := filepath.Join(
			fixture.hostDir,
			"recordings",
			fmt.Sprintf("session-%d-__factory_session_id__.replay.json", generation),
		)
		logsRoot := filepath.Join(fixture.hostDir, fmt.Sprintf("runtime-logs-%d", generation))
		metricsRoot := filepath.Join(fixture.hostDir, fmt.Sprintf("runtime-metrics-%d", generation))
		inputs := support.FakeInputs(context.Background(), []string{
			"you", "run", "--continuously", "--with-server", "--quiet", "--dir", fixture.hostDir,
			"--record", recordPath,
			"--runtime-log-dir", logsRoot,
			"--runtime-metrics-dir", metricsRoot,
		})
		inputs.Input.WorkingDirectory = fixture.hostDir
		inputs.Input.Env = append(os.Environ(), "HOME="+fixture.hostHome, "USERPROFILE="+fixture.hostHome)
		ready := make(chan struct{})
		fixture.hostAPI = api
		fixture.hostRecordPath = recordPath
		fixture.hostLogsRoot = logsRoot
		fixture.hostMetricsRoot = metricsRoot
		fixture.hostURL = ""
		fixture.hostErr = nil
		fixture.hostStopped = false
		fixture.hostStarting = ready
		command := startRootCompositionProcessCommand(fixture.process, inputs.Input)
		fixture.hostCmd = command
		fixture.hostMu.Unlock()

		baseURL, waitErr := api.WaitForBaseURL(15 * time.Second)
		fixture.hostMu.Lock()
		fixture.hostURL = baseURL
		fixture.hostErr = waitErr
		fixture.hostStarting = nil
		close(ready)
		fixture.hostMu.Unlock()
		if waitErr != nil {
			_ = command.stop()
			t.Fatalf("start shared root composition host: %v", waitErr)
		}
		return baseURL
	}
}

// stopSharedHostForDirectProcess releases the default runtime before a
// serial direct CLI witness uses the same immutable Process. The host is
// restarted lazily by the next API-backed scenario; no scenario-wide lease or
// package-wide execution lock is held while the direct witness runs.
func (fixture *rootCompositionFixture) stopSharedHostForDirectProcess(t testing.TB) {
	t.Helper()
	for {
		fixture.hostMu.Lock()
		if fixture.hostStarting != nil {
			ready := fixture.hostStarting
			fixture.hostMu.Unlock()
			<-ready
			continue
		}
		if fixture.hostCmd == nil || fixture.hostStopped {
			fixture.hostMu.Unlock()
			return
		}
		command := fixture.hostCmd
		fixture.hostStopped = true
		fixture.hostURL = ""
		fixture.hostMu.Unlock()
		if err := command.stop(); err != nil {
			t.Errorf("stop shared host before direct Process.Execute: %v", err)
		}
		return
	}
}

func (fixture *rootCompositionFixture) sharedHostAPI() *support.ProcessAPIServer {
	if fixture == nil {
		return nil
	}
	fixture.hostMu.Lock()
	defer fixture.hostMu.Unlock()
	return fixture.hostAPI
}

func (fixture *rootCompositionFixture) constructionSnapshot() rootCompositionConstructionSnapshot {
	return fixture.effects.constructionSnapshot()
}

func openRootCompositionExecutionRuntime(
	t testing.TB,
	fixture *rootCompositionFixture,
	projectRoot string,
	homeDir string,
	factorySessionID string,
	replayPath string,
) factorysessions.OpenedExecutionRuntime {
	t.Helper()
	opening := fixture.process.ExecutionRuntimeOpening()
	if opening == nil {
		t.Fatalf("shared root process execution opening is nil, want factorysessions.ExecutionRuntimeOpeningFunc")
	}
	opened, err := opening(t.Context(), factorysessions.ExecutionRuntimeOpeningRequest{
		ProjectRoot:       projectRoot,
		SystemConfigHome:  homeDir,
		FactorySessionID:  factorySessionID,
		ReplayPath:        replayPath,
		PersistencePolicy: factorysessions.PersistencePolicyEnabled,
	})
	if err != nil {
		t.Fatalf("open execution runtime for %q: %v", replayPath, err)
	}
	if opened.Execution == nil || opened.Close == nil {
		t.Fatalf("opened execution runtime = %#v, want execution and close capability", opened)
	}
	t.Cleanup(func() {
		if err := opened.Close(); err != nil {
			t.Errorf("close execution runtime %q: %v", factorySessionID, err)
		}
	})
	return opened
}

func (fixture *rootCompositionFixture) selectRouteContext(t testing.TB, inputs *support.CapturedInputs, path string) {
	t.Helper()
	if inputs == nil {
		t.Fatal("root composition inputs are nil")
	}
	route, err := fixture.routes.routeForPath(path)
	if err != nil {
		t.Fatalf("select route for %q: %v", path, err)
	}
	inputs.Input.Context = withRootCompositionRouteContext(inputs.Input.Context, route)
}

func (fixture *rootCompositionFixture) withRootCompositionRoute(
	t testing.TB,
	spec rootCompositionRouteSpec,
	run func(),
) {
	t.Helper()
	fixture.withRootCompositionRouteValue(t, spec, func(*rootCompositionRoute) {
		run()
	})
}

func (fixture *rootCompositionFixture) withRootCompositionRouteValue(
	t testing.TB,
	spec rootCompositionRouteSpec,
	run func(*rootCompositionRoute),
) {
	t.Helper()
	route, err := fixture.routes.register(spec)
	if err != nil {
		t.Fatalf("register root composition route: %v", err)
	}
	defer func() {
		if err := route.cleanup.cleanup(); err != nil {
			t.Errorf("cleanup root composition route %q: %v", route.label, err)
		}
		if err := fixture.routes.unregister(route); err != nil {
			t.Errorf("unregister root composition route %q: %v", route.label, err)
		}
	}()
	run(route)
}

func TestRootCompositionCleanupLedgerJoinsFailures(t *testing.T) {
	t.Parallel()
	ledger := newRootCompositionCleanupLedger()
	firstErr := errors.New("first cleanup failed")
	secondErr := errors.New("second cleanup failed")
	firstRan := false
	secondRan := false
	thirdRan := false
	if _, err := ledger.register("first", func() error {
		firstRan = true
		return firstErr
	}); err != nil {
		t.Fatalf("register first cleanup: %v", err)
	}
	if _, err := ledger.register("second", func() error {
		secondRan = true
		return secondErr
	}); err != nil {
		t.Fatalf("register second cleanup: %v", err)
	}
	if _, err := ledger.register("third", func() error {
		thirdRan = true
		return nil
	}); err != nil {
		t.Fatalf("register third cleanup: %v", err)
	}

	cleanupErr := ledger.cleanup()
	if !errors.Is(cleanupErr, firstErr) || !errors.Is(cleanupErr, secondErr) {
		t.Fatalf("cleanup error = %v, want both joined failures", cleanupErr)
	}
	if !firstRan || !secondRan || !thirdRan {
		t.Fatalf("cleanup execution = first:%t second:%t third:%t, want all actions", firstRan, secondRan, thirdRan)
	}
	if err := ledger.cleanup(); err != nil {
		t.Fatalf("idempotent second cleanup = %v, want nil", err)
	}
}
