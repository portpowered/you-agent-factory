package execution_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const sharedExecutionFixtureTimeout = 15 * time.Second

// The portable recording projection is process-scoped and finalized when a
// live session closes. Keep scenario bodies serialized so one session cannot
// read or finalize the shared projection while another session is publishing
// its terminal state.
const sharedExecutionScenarioConcurrency = 1

type sharedExecutionSessionKind uint8

const (
	sharedExecutionSessionDurable sharedExecutionSessionKind = iota
	sharedExecutionSessionLive
)

type sharedExecutionIdentity struct {
	mu                  sync.Mutex
	pendingSessionIDs   []string
	pendingSessionKinds []sharedExecutionSessionKind
}

var (
	sharedExecutionFixtureOnce  sync.Once
	sharedExecutionFixtureValue *sharedExecutionFixture
	sharedExecutionFixtureErr   error
	sharedExecutionScenarioSlot = make(chan struct{}, sharedExecutionScenarioConcurrency)
)

// sharedExecutionProcess returns the one root-built process used by the
// execution package's eligible API scenarios. Factory definitions, command
// responses, and explicit Factory Sessions remain scenario-owned.
func sharedExecutionProcess(t testing.TB) *sharedExecutionFixture {
	t.Helper()
	sharedExecutionFixtureOnce.Do(func() {
		sharedExecutionFixtureValue, sharedExecutionFixtureErr = newSharedExecutionFixture()
	})
	if sharedExecutionFixtureErr != nil {
		t.Fatalf("start shared execution fixture: %v", sharedExecutionFixtureErr)
	}
	if sharedExecutionFixtureValue == nil {
		t.Fatal("shared execution fixture is unavailable")
	}
	return sharedExecutionFixtureValue
}

func acquireSharedExecutionScenarioSlot(t testing.TB) {
	t.Helper()
	sharedExecutionScenarioSlot <- struct{}{}
	t.Cleanup(func() { <-sharedExecutionScenarioSlot })
}

// TestMain closes the package-scoped host after every scenario has released
// its explicit session and command route.
func TestMain(m *testing.M) {
	code := m.Run()
	if sharedExecutionFixtureValue != nil {
		if err := sharedExecutionFixtureValue.close(); err != nil {
			fmt.Fprintf(os.Stderr, "close shared execution fixture: %v\n", err)
			if code == 0 {
				code = 1
			}
		}
	}
	os.Exit(code)
}

type sharedExecutionFixture struct {
	rootDir      string
	homeDir      string
	bootstrapDir string
	baseURL      string

	process support.ApplicationProcess
	command *sharedExecutionHostedCommand
	api     *support.ProcessAPIServer
	router  *sharedExecutionCommandRouter

	sessionMu        sync.Mutex
	openedSessionIDs map[string]struct{}
	closedSessionIDs map[string]struct{}

	openMu   sync.Mutex
	identity *sharedExecutionIdentity
}

type sharedExecutionHostedCommand struct {
	cancel context.CancelFunc
	done   chan struct{}

	mu  sync.Mutex
	err error
}

func newSharedExecutionFixture() (*sharedExecutionFixture, error) {
	rootDir, err := os.MkdirTemp("", "you-functional-sessions-execution-")
	if err != nil {
		return nil, fmt.Errorf("create fixture root: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(rootDir) }
	homeDir := filepath.Join(rootDir, "home")
	bootstrapDir := filepath.Join(rootDir, "bootstrap")
	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		cleanup()
		return nil, fmt.Errorf("create fixture home: %w", err)
	}
	if err := writeSharedExecutionBootstrapFactory(bootstrapDir); err != nil {
		cleanup()
		return nil, err
	}

	api := support.NewProcessAPIServer()
	router := newSharedExecutionCommandRouter()
	identity := &sharedExecutionIdentity{}
	process, err := newSharedExecutionProcess(api, router, identity)
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("build root process: %w", err)
	}

	fixture := &sharedExecutionFixture{
		rootDir:          rootDir,
		homeDir:          homeDir,
		bootstrapDir:     bootstrapDir,
		process:          process,
		api:              api,
		router:           router,
		identity:         identity,
		openedSessionIDs: make(map[string]struct{}),
		closedSessionIDs: make(map[string]struct{}),
	}
	inputs := support.FakeInputs(context.Background(), []string{
		"you", "run", "--dir", bootstrapDir,
		"--continuously", "--with-server", "--server", "http://127.0.0.1:1",
		"--quiet", "--record", filepath.Join(rootDir, "execution-shared.recording.json"),
	})
	inputs.Input.Env = []string{"HOME=" + homeDir, "USERPROFILE=" + homeDir}
	inputs.Input.WorkingDirectory = bootstrapDir
	fixture.command = startSharedExecutionHostedCommand(process, inputs.Input)
	baseURL, err := api.WaitForBaseURL(sharedExecutionFixtureTimeout)
	if err != nil {
		fixture.command.mu.Lock()
		commandErr := fixture.command.err
		fixture.command.mu.Unlock()
		_ = fixture.close()
		cleanup()
		return nil, fmt.Errorf("wait for loopback API: %w (Process.Execute error: %v)", err, commandErr)
	}
	fixture.baseURL = baseURL
	if err := waitForSharedExecutionRuntime(baseURL, sharedExecutionFixtureTimeout); err != nil {
		_ = fixture.close()
		cleanup()
		return nil, err
	}
	return fixture, nil
}

func newSharedExecutionProcess(
	api *support.ProcessAPIServer,
	router *sharedExecutionCommandRouter,
	identity *sharedExecutionIdentity,
) (support.ApplicationProcess, error) {
	return support.BuildProcessWithContext(context.Background(), serviceedges.Edges{
		APIServerStarter: func(ctx context.Context, request platformhttpserver.StartRequest) error {
			return api.Start(ctx, request)
		},
		// The dispatch-list transport identifies durable projections by this
		// prefix. Live ownership still remains explicit until the scenario
		// closes the session.
		FactorySessionIDGenerator: func() string {
			identity.mu.Lock()
			// Durable synchronous execution adds its own durable prefix around
			// this raw generator result. Explicit dispatch-list sessions are
			// opened through the live-session route and request that prefix here.
			// Only an explicitly queued live-session open also pairs its public
			// identity with the following runtime-ID allocation. Other callers of
			// this shared edge allocate request IDs and must not leave a stale
			// runtime identity for a later session.
			var id string
			if len(identity.pendingSessionKinds) > 0 {
				kind := identity.pendingSessionKinds[0]
				identity.pendingSessionKinds = identity.pendingSessionKinds[1:]
				if kind == sharedExecutionSessionLive {
					id = uuid.NewString()
				} else {
					id = "dur-sess-" + strings.ReplaceAll(uuid.NewString(), "-", "")
				}
				identity.pendingSessionIDs = append(identity.pendingSessionIDs, id)
			} else {
				id = uuid.NewString()
			}
			identity.mu.Unlock()
			return id
		},
		FactoryRuntimeIDGenerator: func() string {
			identity.mu.Lock()
			if len(identity.pendingSessionIDs) > 0 {
				id := identity.pendingSessionIDs[0]
				identity.pendingSessionIDs = identity.pendingSessionIDs[1:]
				identity.mu.Unlock()
				return id
			}
			identity.mu.Unlock()
			return uuid.NewString()
		},
		ProviderCommandRunner: router,
		ScriptCommandRunner:   router,
	})
}

func writeSharedExecutionBootstrapFactory(dir string) error {
	if err := os.MkdirAll(filepath.Join(dir, "workers", "worker-a"), 0o755); err != nil {
		return fmt.Errorf("create bootstrap worker directory: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "workstations", "process"), 0o755); err != nil {
		return fmt.Errorf("create bootstrap workstation directory: %w", err)
	}
	cfg := simplePipelineConfig()
	cfg["name"] = "sessions-execution-shared-bootstrap"
	payload, err := json.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal bootstrap Factory: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, factorydefinitions.FactoryConfigFile), payload, 0o644); err != nil {
		return fmt.Errorf("write bootstrap Factory: %w", err)
	}
	if err := os.WriteFile(
		filepath.Join(dir, "workers", "worker-a", "AGENTS.md"),
		[]byte("---\ntype: MODEL_WORKER\nmodel: gpt-5-codex\nmodelProvider: codex\n---\nProcess the input task.\n"),
		0o644,
	); err != nil {
		return fmt.Errorf("write bootstrap worker instructions: %w", err)
	}
	if err := os.WriteFile(
		filepath.Join(dir, "workstations", "process", "AGENTS.md"),
		[]byte("---\ntype: MODEL_WORKSTATION\n---\nProcess the input task.\n"),
		0o644,
	); err != nil {
		return fmt.Errorf("write bootstrap workstation instructions: %w", err)
	}
	return nil
}

func startSharedExecutionHostedCommand(process support.ApplicationProcess, input root.Input) *sharedExecutionHostedCommand {
	parent := input.Context
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	input.Context = ctx
	command := &sharedExecutionHostedCommand{cancel: cancel, done: make(chan struct{})}
	go func() {
		err := process.Execute(input)
		command.mu.Lock()
		command.err = err
		command.mu.Unlock()
		close(command.done)
	}()
	return command
}

func waitForSharedExecutionRuntime(baseURL string, timeout time.Duration) error {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		response, err := httpGetSharedExecutionStatus(baseURL)
		if err == nil && strings.TrimSpace(response.RuntimeStatus) != "" {
			return nil
		}
		select {
		case <-deadline.C:
			if err != nil {
				return fmt.Errorf("wait for shared execution runtime: %w", err)
			}
			return fmt.Errorf("wait for shared execution runtime: status remained unavailable")
		case <-ticker.C:
		}
	}
}

func httpGetSharedExecutionStatus(baseURL string) (factoryapi.StatusResponse, error) {
	response, err := http.Get(strings.TrimSuffix(baseURL, "/") + "/status")
	if err != nil {
		return factoryapi.StatusResponse{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return factoryapi.StatusResponse{}, fmt.Errorf("status endpoint returned %d", response.StatusCode)
	}
	var status factoryapi.StatusResponse
	if err := json.NewDecoder(response.Body).Decode(&status); err != nil {
		return factoryapi.StatusResponse{}, err
	}
	return status, nil
}

func (fixture *sharedExecutionFixture) close() error {
	if fixture == nil {
		return nil
	}
	var closeErr error
	if fixture.command != nil {
		closeErr = fixture.command.stop()
	}
	if fixture.process != nil {
		ctx, cancel := context.WithTimeout(context.Background(), sharedExecutionFixtureTimeout)
		closeErr = errors.Join(closeErr, fixture.process.Close(ctx))
		cancel()
	}
	if got := fixture.router.routeCount(); got != 0 {
		closeErr = errors.Join(closeErr, fmt.Errorf("shared execution command routes remaining after cleanup = %d", got))
	}
	if err := fixture.sessionLifecycleError(); err != nil {
		closeErr = errors.Join(closeErr, err)
	}
	if err := os.RemoveAll(fixture.rootDir); err != nil {
		closeErr = errors.Join(closeErr, fmt.Errorf("remove shared execution fixture root: %w", err))
	}
	return closeErr
}

func (command *sharedExecutionHostedCommand) stop() error {
	if command == nil {
		return nil
	}
	command.cancel()
	select {
	case <-command.done:
		command.mu.Lock()
		err := command.err
		command.mu.Unlock()
		if err != nil && !errors.Is(err, context.Canceled) {
			return err
		}
		return nil
	case <-time.After(sharedExecutionFixtureTimeout):
		return fmt.Errorf("timed out waiting for shared execution host shutdown")
	}
}

func (fixture *sharedExecutionFixture) sessionLifecycleError() error {
	fixture.sessionMu.Lock()
	defer fixture.sessionMu.Unlock()
	if len(fixture.openedSessionIDs) != len(fixture.closedSessionIDs) {
		return fmt.Errorf(
			"shared execution Factory Session lifecycle opened %d sessions but closed %d",
			len(fixture.openedSessionIDs), len(fixture.closedSessionIDs),
		)
	}
	for sessionID := range fixture.openedSessionIDs {
		if _, ok := fixture.closedSessionIDs[sessionID]; !ok {
			return fmt.Errorf("shared execution Factory Session %q was not closed", sessionID)
		}
	}
	return nil
}

func (fixture *sharedExecutionFixture) recordSessionOpened(sessionID string) error {
	fixture.sessionMu.Lock()
	defer fixture.sessionMu.Unlock()
	if _, exists := fixture.openedSessionIDs[sessionID]; exists {
		return fmt.Errorf("shared execution Factory Session %q opened twice", sessionID)
	}
	fixture.openedSessionIDs[sessionID] = struct{}{}
	return nil
}

func (fixture *sharedExecutionFixture) recordSessionClosed(sessionID string) {
	fixture.sessionMu.Lock()
	defer fixture.sessionMu.Unlock()
	fixture.closedSessionIDs[sessionID] = struct{}{}
}

type sharedExecutionRouteConfig struct {
	provider platformprocess.CommandRunner
	script   platformprocess.CommandRunner
	// match admits deterministic routing for JavaScript child commands whose
	// runtime working directory is the process project root rather than the
	// scenario's source directory.
	match func(platformprocess.CommandRequest) bool
}

type sharedExecutionCommandRouter struct {
	mu      sync.Mutex
	routes  map[string]*sharedExecutionCommandRoute
	history map[string]*sharedExecutionCommandRoute
}

type sharedExecutionCommandRoute struct {
	factoryDir string
	provider   platformprocess.CommandRunner
	script     platformprocess.CommandRunner
	match      func(platformprocess.CommandRequest) bool
}

func newSharedExecutionCommandRouter() *sharedExecutionCommandRouter {
	return &sharedExecutionCommandRouter{
		routes:  make(map[string]*sharedExecutionCommandRoute),
		history: make(map[string]*sharedExecutionCommandRoute),
	}
}

func (router *sharedExecutionCommandRouter) register(factoryDir string, config sharedExecutionRouteConfig) error {
	factoryDir, err := filepath.Abs(filepath.Clean(factoryDir))
	if err != nil || strings.TrimSpace(factoryDir) == "" {
		return fmt.Errorf("shared execution Factory directory is invalid: %q", factoryDir)
	}
	router.mu.Lock()
	defer router.mu.Unlock()
	if _, exists := router.routes[factoryDir]; exists {
		return fmt.Errorf("shared execution route for %q is already registered", factoryDir)
	}
	router.routes[factoryDir] = &sharedExecutionCommandRoute{
		factoryDir: factoryDir,
		provider:   config.provider,
		script:     config.script,
		match:      config.match,
	}
	return nil
}

func (router *sharedExecutionCommandRouter) unregister(factoryDir string) {
	factoryDir, _ = filepath.Abs(filepath.Clean(factoryDir))
	router.mu.Lock()
	if route := router.routes[factoryDir]; route != nil {
		router.history[factoryDir] = route
	}
	delete(router.routes, factoryDir)
	router.mu.Unlock()
}

func (router *sharedExecutionCommandRouter) routeCount() int {
	router.mu.Lock()
	defer router.mu.Unlock()
	return len(router.routes)
}

func (router *sharedExecutionCommandRouter) Run(
	ctx context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	if err := ctx.Err(); err != nil {
		return platformprocess.CommandResult{}, err
	}
	router.mu.Lock()
	route := router.routeForRequest(request)
	var runner platformprocess.CommandRunner
	if route != nil {
		if isSharedExecutionProviderCommand(request.Command) {
			runner = route.provider
		} else {
			runner = route.script
		}
	}
	router.mu.Unlock()
	if route == nil {
		return platformprocess.CommandResult{}, fmt.Errorf(
			"no shared execution command route matched work directory %q",
			request.WorkDir,
		)
	}
	if runner == nil {
		return platformprocess.CommandResult{}, fmt.Errorf(
			"shared execution route for %q has no runner for command %q",
			route.factoryDir, request.Command,
		)
	}
	return runner.Run(ctx, request)
}

func (router *sharedExecutionCommandRouter) routeForRequest(request platformprocess.CommandRequest) *sharedExecutionCommandRoute {
	var best *sharedExecutionCommandRoute
	for factoryDir, route := range router.routes {
		matched := route.match != nil && route.match(request)
		if !matched {
			matched = sharedExecutionPathBelongsTo(factoryDir, request.WorkDir) ||
				sharedExecutionPathBelongsTo(factoryDir, request.Command)
		}
		if !matched {
			continue
		}
		if best == nil || len(factoryDir) > len(best.factoryDir) {
			best = route
		}
	}
	return best
}

func sharedExecutionPathBelongsTo(factoryDir, candidate string) bool {
	candidate = strings.TrimSpace(candidate)
	if candidate == "" || !filepath.IsAbs(candidate) {
		return false
	}
	relative, err := filepath.Rel(factoryDir, filepath.Clean(candidate))
	if err != nil {
		return false
	}
	return relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)))
}

func isSharedExecutionProviderCommand(command string) bool {
	switch strings.ToLower(filepath.Base(strings.TrimSpace(command))) {
	case "codex", "claude", "agy":
		return true
	default:
		return false
	}
}

type sharedExecutionSession struct {
	fixture    *sharedExecutionFixture
	factoryDir string
	sessionID  string
	closeOnce  sync.Once
}

func openSharedExecutionSession(t *testing.T, factoryDir string, config sharedExecutionRouteConfig) *sharedExecutionSession {
	return openSharedExecutionSessionWithKind(t, factoryDir, config, sharedExecutionSessionDurable)
}

func openSharedExecutionLiveSession(t *testing.T, factoryDir string, config sharedExecutionRouteConfig) *sharedExecutionSession {
	return openSharedExecutionSessionWithKind(t, factoryDir, config, sharedExecutionSessionLive)
}

func openSharedExecutionSessionWithKind(
	t *testing.T,
	factoryDir string,
	config sharedExecutionRouteConfig,
	kind sharedExecutionSessionKind,
) *sharedExecutionSession {
	t.Helper()
	fixture := sharedExecutionProcess(t)
	factoryDir, err := filepath.Abs(filepath.Clean(factoryDir))
	if err != nil {
		t.Fatalf("resolve execution Factory directory: %v", err)
	}
	registerSharedExecutionRoute(t, factoryDir, config)
	fixture.openMu.Lock()
	defer fixture.openMu.Unlock()
	fixture.queueSessionKind(kind)
	opened := support.OpenFactorySessionAt(t, fixture.baseURL, factoryDir)
	sessionID := opened.Session.Id
	if err := fixture.recordSessionOpened(sessionID); err != nil {
		support.CloseFactorySessionAt(t, fixture.baseURL, sessionID)
		t.Fatalf("record shared execution Factory Session %q: %v", sessionID, err)
	}
	session := &sharedExecutionSession{fixture: fixture, factoryDir: factoryDir, sessionID: sessionID}
	t.Cleanup(func() { session.close(t) })
	return session
}

func registerSharedExecutionRoute(t testing.TB, factoryDir string, config sharedExecutionRouteConfig) {
	t.Helper()
	fixture := sharedExecutionProcess(t)
	factoryDir, err := filepath.Abs(filepath.Clean(factoryDir))
	if err != nil {
		t.Fatalf("resolve execution Factory directory: %v", err)
	}
	if err := fixture.router.register(factoryDir, config); err != nil {
		t.Fatalf("register shared execution command route: %v", err)
	}
	t.Cleanup(func() { fixture.router.unregister(factoryDir) })
}

func (fixture *sharedExecutionFixture) queueSessionKind(kind sharedExecutionSessionKind) {
	// The process's session-id generator is invoked by the Open Factory Session
	// request. Serializing callers around that request keeps each requested
	// route identity paired with the correct generated kind.
	fixture.identity.mu.Lock()
	defer fixture.identity.mu.Unlock()
	fixture.identity.pendingSessionKinds = append(fixture.identity.pendingSessionKinds, kind)
}

func (session *sharedExecutionSession) close(t testing.TB) {
	if session == nil {
		return
	}
	session.closeOnce.Do(func() {
		support.CloseFactorySessionAt(t, session.fixture.baseURL, session.sessionID)
		session.fixture.recordSessionClosed(session.sessionID)
	})
}

func waitForSharedExecutionEvent(t testing.TB, stream *support.FactoryEventStream, want factoryapi.FactoryEventType, timeout time.Duration) factoryapi.FactoryEvent {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			t.Fatalf("timed out waiting for shared execution event %q", want)
		}
		event, ok := stream.TryNextEvent(minSharedExecutionDuration(remaining, 250*time.Millisecond))
		if !ok {
			continue
		}
		if event.Type == want {
			return event
		}
	}
}

func minSharedExecutionDuration(left, right time.Duration) time.Duration {
	if left < right {
		return left
	}
	return right
}

var _ platformprocess.CommandRunner = (*sharedExecutionCommandRouter)(nil)
