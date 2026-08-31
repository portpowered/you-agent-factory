package root_composition_test

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	rootCompositionSharedStartupTimeout  = 60 * time.Second
	rootCompositionSharedShutdownTimeout = 15 * time.Second
)

var rootCompositionSharedFixtureState struct {
	sync.Once
	fixture *rootCompositionSharedFixture
}

// TestMain owns the one package-scoped process and API listener. The fixture
// is lazy so pure contract and process-close witnesses remain independent and
// do not accidentally acquire the shared runtime.
func TestMain(m *testing.M) {
	exitCode := m.Run()
	if err := closeRootCompositionSharedFixture(); err != nil {
		fmt.Fprintf(os.Stderr, "root-composition shared fixture cleanup failed: %v\n", err)
		exitCode = 1
	}
	os.Exit(exitCode)
}

// rootCompositionSharedFixture owns immutable production wiring and one
// continuously hosted API boundary. Factory directories, command routes, and
// Factory Sessions remain test-owned so parallel cases cannot share mutable
// runtime state.
type rootCompositionSharedFixture struct {
	rootDir     string
	hostFactory string
	homeDir     string
	baseURL     string

	process support.ApplicationProcess
	hosted  *rootCompositionHostedCommand
	api     *rootCompositionAPIServer

	providerRouter *rootCompositionCommandRouter
	scriptRouter   *rootCompositionCommandRouter
	effects        *rootCompositionEffectCounters
	rootBuilds     atomic.Int32

	sessionMu        sync.Mutex
	openedSessionIDs map[string]struct{}
	closedSessionIDs map[string]struct{}
}

type rootCompositionAPIServer struct {
	server   *support.ProcessAPIServer
	starts   atomic.Int32
	stopped  chan struct{}
	stopOnce sync.Once
}

func (server *rootCompositionAPIServer) start(
	ctx context.Context,
	request platformhttpserver.StartRequest,
) error {
	server.starts.Add(1)
	err := server.server.Start(ctx, request)
	server.stopOnce.Do(func() { close(server.stopped) })
	return err
}

type rootCompositionHostedCommand struct {
	cancel context.CancelFunc
	done   chan error
}

func startRootCompositionHostedCommand(
	process support.ApplicationProcess,
	input root.Input,
) *rootCompositionHostedCommand {
	parent := input.Context
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	input.Context = ctx
	command := &rootCompositionHostedCommand{cancel: cancel, done: make(chan error, 1)}
	go func() { command.done <- process.Execute(input) }()
	return command
}

func (command *rootCompositionHostedCommand) stop() error {
	if command == nil {
		return nil
	}
	command.cancel()
	select {
	case err := <-command.done:
		if err != nil && !errors.Is(err, context.Canceled) {
			return fmt.Errorf("stop hosted root-composition process: %w", err)
		}
		return nil
	case <-time.After(rootCompositionSharedShutdownTimeout):
		return errors.New("timed out waiting for hosted root-composition process shutdown")
	}
}

func rootCompositionSharedProcess(t *testing.T) *rootCompositionSharedFixture {
	t.Helper()
	rootCompositionSharedFixtureState.Do(func() {
		rootCompositionSharedFixtureState.fixture = newRootCompositionSharedFixture(t)
	})
	if rootCompositionSharedFixtureState.fixture == nil {
		t.Fatal("root-composition shared fixture is unavailable")
	}
	return rootCompositionSharedFixtureState.fixture
}

func newRootCompositionSharedFixture(t *testing.T) *rootCompositionSharedFixture {
	t.Helper()

	rootDir, err := os.MkdirTemp("", "c15-root-composition-")
	if err != nil {
		t.Fatalf("create root-composition shared root: %v", err)
	}
	keepRoot := false
	defer func() {
		if !keepRoot {
			_ = os.RemoveAll(rootDir)
		}
	}()

	hostFactory := filepath.Join(rootDir, "host-factory")
	if err := copyRootCompositionDirectory(
		support.LegacyFixtureDir(t, "script_executor_dir"),
		hostFactory,
	); err != nil {
		t.Fatalf("copy root-composition shared host Factory: %v", err)
	}
	support.ClearSeedInputs(t, hostFactory)
	homeDir := filepath.Join(rootDir, "home")
	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		t.Fatalf("create root-composition shared home: %v", err)
	}

	api := &rootCompositionAPIServer{
		server:  support.NewProcessAPIServer(),
		stopped: make(chan struct{}),
	}
	providerRouter := newRootCompositionCommandRouter("provider")
	scriptRouter := newRootCompositionCommandRouter("script")
	effects := &rootCompositionEffectCounters{}
	process, err := support.BuildProcessWithContext(context.Background(), serviceedges.Edges{
		APIServerStarter:                         api.start,
		ProviderCommandRunner:                    providerRouter,
		ScriptCommandRunner:                      scriptRouter,
		FactorySessionIDGenerator:                effects.nextSessionID,
		FactorySessionRuntimeInstanceIDGenerator: effects.nextRuntimeID,
		FactorySessionResponseEventIDGenerator:   effects.nextResponseEventID,
		WorkRequestIDGenerator:                   effects.nextWorkRequestID,
		InvocationMetricsRecorder:                effects,
		RuntimeHostObserver:                      effects.observeRuntimeHost,
	})
	if err != nil {
		t.Fatalf("build root-composition shared process: %v", err)
	}

	inputs := support.FakeInputs(context.Background(), []string{
		"you", "run",
		"--dir", hostFactory,
		"--continuously",
		"--with-server",
		"--server", "http://127.0.0.1:1",
		"--quiet",
		"--no-record",
	})
	inputs.Input.Env = rootCompositionEnvironment(homeDir)
	inputs.Input.WorkingDirectory = hostFactory
	hosted := startRootCompositionHostedCommand(process, inputs.Input)
	baseURL, err := api.server.WaitForBaseURL(rootCompositionSharedStartupTimeout)
	if err != nil {
		stopErr := hosted.stop()
		_ = process.Close(context.Background())
		t.Fatalf("wait for root-composition shared API: %v; hosted command error: %v; stdout: %s; stderr: %s", err, stopErr, inputs.Stdout(), inputs.Stderr())
	}

	fixture := &rootCompositionSharedFixture{
		rootDir:          rootDir,
		hostFactory:      hostFactory,
		homeDir:          homeDir,
		baseURL:          baseURL,
		process:          process,
		hosted:           hosted,
		api:              api,
		providerRouter:   providerRouter,
		scriptRouter:     scriptRouter,
		effects:          effects,
		openedSessionIDs: make(map[string]struct{}),
		closedSessionIDs: make(map[string]struct{}),
	}
	fixture.rootBuilds.Add(1)
	keepRoot = true
	return fixture
}

func (fixture *rootCompositionSharedFixture) registerCommandRunners(
	t *testing.T,
	factoryDir string,
	providerRunner platformprocess.CommandRunner,
	scriptRunner platformprocess.CommandRunner,
) {
	t.Helper()
	if providerRunner != nil {
		unregister := fixture.providerRouter.register(factoryDir, providerRunner)
		t.Cleanup(unregister)
	}
	if scriptRunner != nil {
		unregister := fixture.scriptRouter.register(factoryDir, scriptRunner)
		t.Cleanup(unregister)
	}
}

func (fixture *rootCompositionSharedFixture) openSession(t *testing.T, factoryDir string) string {
	t.Helper()
	opened := support.OpenFactorySessionAt(t, fixture.baseURL, factoryDir)
	if opened.Session == nil || opened.Session.Id == "" {
		t.Fatalf("opened shared Factory Session = %#v, want canonical identity", opened)
	}
	sessionID := opened.Session.Id
	fixture.trackSession(t, sessionID)
	return sessionID
}

func (fixture *rootCompositionSharedFixture) trackSession(t *testing.T, sessionID string) {
	t.Helper()
	if sessionID == "" {
		t.Fatal("cannot track an empty shared Factory Session ID")
	}
	fixture.sessionMu.Lock()
	if _, exists := fixture.openedSessionIDs[sessionID]; exists {
		fixture.sessionMu.Unlock()
		t.Fatalf("shared Factory Session %q was opened twice", sessionID)
	}
	fixture.openedSessionIDs[sessionID] = struct{}{}
	fixture.sessionMu.Unlock()
	t.Cleanup(func() { fixture.closeSession(t, sessionID) })
}

func (fixture *rootCompositionSharedFixture) closeSession(t testing.TB, sessionID string) {
	if fixture == nil || sessionID == "" {
		return
	}
	fixture.sessionMu.Lock()
	if _, closed := fixture.closedSessionIDs[sessionID]; closed {
		fixture.sessionMu.Unlock()
		return
	}
	fixture.sessionMu.Unlock()
	support.CloseFactorySessionAt(t, fixture.baseURL, sessionID)
	fixture.sessionMu.Lock()
	fixture.closedSessionIDs[sessionID] = struct{}{}
	fixture.sessionMu.Unlock()
}

func (fixture *rootCompositionSharedFixture) sessionLifecycleError() error {
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

func closeRootCompositionSharedFixture() error {
	fixture := rootCompositionSharedFixtureState.fixture
	if fixture == nil {
		return nil
	}
	var errs []error
	if err := fixture.sessionLifecycleError(); err != nil {
		errs = append(errs, err)
	}
	if err := fixture.hosted.stop(); err != nil {
		errs = append(errs, err)
	}
	closeCtx, cancel := context.WithTimeout(context.Background(), rootCompositionSharedShutdownTimeout)
	if err := fixture.process.Close(closeCtx); err != nil {
		errs = append(errs, fmt.Errorf("close root-composition shared process: %w", err))
	}
	cancel()
	select {
	case <-fixture.api.stopped:
	case <-time.After(rootCompositionSharedShutdownTimeout):
		errs = append(errs, errors.New("root-composition shared API server did not stop"))
	}
	if got := fixture.rootBuilds.Load(); got != 1 {
		errs = append(errs, fmt.Errorf("root-composition shared root builds = %d, want exactly one", got))
	}
	if got := fixture.api.starts.Load(); got != 1 {
		errs = append(errs, fmt.Errorf("root-composition shared API starts = %d, want exactly one", got))
	}
	if got := fixture.providerRouter.routeCount(); got != 0 {
		errs = append(errs, fmt.Errorf("active shared provider routes after cleanup = %d", got))
	}
	if got := fixture.scriptRouter.routeCount(); got != 0 {
		errs = append(errs, fmt.Errorf("active shared script routes after cleanup = %d", got))
	}
	if err := os.RemoveAll(fixture.rootDir); err != nil {
		errs = append(errs, fmt.Errorf("remove root-composition shared root %q: %w", fixture.rootDir, err))
	} else if _, err := os.Stat(fixture.rootDir); !os.IsNotExist(err) {
		errs = append(errs, fmt.Errorf("root-composition shared root %q remains after cleanup: %v", fixture.rootDir, err))
	}
	return errors.Join(errs...)
}

type rootCompositionEffectCounters struct {
	sessionID   atomic.Int32
	runtimeID   atomic.Int32
	responseID  atomic.Int32
	workRequest atomic.Int32
	metrics     atomic.Int32
	runtimeHost atomic.Int32
}

func (c *rootCompositionEffectCounters) lifecycleCount() int32 {
	return c.sessionID.Load() + c.runtimeID.Load() + c.runtimeHost.Load()
}

func (c *rootCompositionEffectCounters) responseStreamCount() int32 {
	return c.responseID.Load() + c.metrics.Load()
}

func (c *rootCompositionEffectCounters) nextSessionID() string {
	c.sessionID.Add(1)
	return uuid.NewString()
}

func (c *rootCompositionEffectCounters) nextRuntimeID() string {
	c.runtimeID.Add(1)
	return uuid.NewString()
}

func (c *rootCompositionEffectCounters) nextResponseEventID() string {
	c.responseID.Add(1)
	return uuid.NewString()
}

func (c *rootCompositionEffectCounters) nextWorkRequestID() string {
	c.workRequest.Add(1)
	return uuid.NewString()
}

func (c *rootCompositionEffectCounters) RecordInvocationMetric(factorysessions.InvocationMetric) {
	c.metrics.Add(1)
}

func (c *rootCompositionEffectCounters) observeRuntimeHost(factorysessions.RuntimeHostBinding) {
	c.runtimeHost.Add(1)
}

type rootCompositionCommandRoute struct {
	basePath string
	runner   platformprocess.CommandRunner
}

type rootCompositionCommandRouter struct {
	name   string
	mu     sync.RWMutex
	routes []*rootCompositionCommandRoute
}

func newRootCompositionCommandRouter(name string) *rootCompositionCommandRouter {
	return &rootCompositionCommandRouter{name: name}
}

func (router *rootCompositionCommandRouter) register(
	basePath string,
	runner platformprocess.CommandRunner,
) func() {
	basePath, _ = filepath.Abs(filepath.Clean(basePath))
	route := &rootCompositionCommandRoute{basePath: basePath, runner: runner}
	router.mu.Lock()
	router.routes = append(router.routes, route)
	router.mu.Unlock()
	return func() {
		router.mu.Lock()
		defer router.mu.Unlock()
		for index, candidate := range router.routes {
			if candidate != route {
				continue
			}
			router.routes = append(router.routes[:index], router.routes[index+1:]...)
			return
		}
	}
}

func (router *rootCompositionCommandRouter) Run(
	ctx context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	workDir, _ := filepath.Abs(filepath.Clean(request.WorkDir))
	router.mu.RLock()
	var selected *rootCompositionCommandRoute
	for _, route := range router.routes {
		if !rootCompositionPathContains(route.basePath, workDir) {
			continue
		}
		if selected == nil || len(route.basePath) > len(selected.basePath) {
			selected = route
		}
	}
	router.mu.RUnlock()
	if selected == nil || selected.runner == nil {
		return platformprocess.CommandResult{}, fmt.Errorf(
			"no shared %s command route for work directory %q",
			router.name,
			request.WorkDir,
		)
	}
	return selected.runner.Run(ctx, request)
}

func (router *rootCompositionCommandRouter) routeCount() int {
	router.mu.RLock()
	defer router.mu.RUnlock()
	return len(router.routes)
}

func rootCompositionPathContains(basePath, candidate string) bool {
	basePath = filepath.Clean(basePath)
	candidate = filepath.Clean(candidate)
	relative, err := filepath.Rel(basePath, candidate)
	if err != nil {
		return false
	}
	return relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)))
}

func rootCompositionEnvironment(homeDir string) []string {
	environment := make([]string, 0, len(os.Environ())+2)
	for _, entry := range os.Environ() {
		name := strings.SplitN(entry, "=", 2)[0]
		if strings.EqualFold(name, "HOME") || strings.EqualFold(name, "USERPROFILE") {
			continue
		}
		environment = append(environment, entry)
	}
	return append(environment, "HOME="+homeDir, "USERPROFILE="+homeDir)
}

func copyRootCompositionDirectory(sourceDir, destinationDir string) error {
	return filepath.WalkDir(sourceDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}
		target := destinationDir
		if relative != "." {
			target = filepath.Join(destinationDir, relative)
		}
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
