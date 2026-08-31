package sessioncontrols_test

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
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const sharedControlsFixtureTimeout = 15 * time.Second

var (
	sharedControlsFixtureOnce  sync.Once
	sharedControlsFixtureValue *sharedControlsFixture
	sharedControlsFixtureErr   error
	sharedControlsScenarioSlot = make(chan struct{}, 1)
)

// sharedControlsProcess returns the one root-built process used by eligible
// controls scenarios. Factory definitions, provider responses, and explicit
// Factory Sessions remain scenario-owned.
func sharedControlsProcess(t testing.TB) *sharedControlsFixture {
	t.Helper()
	sharedControlsFixtureOnce.Do(func() {
		sharedControlsFixtureValue, sharedControlsFixtureErr = newSharedControlsFixture()
	})
	if sharedControlsFixtureErr != nil {
		t.Fatalf("start shared controls fixture: %v", sharedControlsFixtureErr)
	}
	if sharedControlsFixtureValue == nil {
		t.Fatal("shared controls fixture is unavailable")
	}
	return sharedControlsFixtureValue
}

func acquireSharedControlsScenarioSlot(t testing.TB) {
	t.Helper()
	sharedControlsScenarioSlot <- struct{}{}
	t.Cleanup(func() { <-sharedControlsScenarioSlot })
}

// TestMain closes the package-scoped host after every scenario has released
// its explicit Session and command route.
func TestMain(m *testing.M) {
	code := m.Run()
	if sharedControlsFixtureValue != nil {
		if err := sharedControlsFixtureValue.close(); err != nil {
			fmt.Fprintf(os.Stderr, "close shared controls fixture: %v\n", err)
			if code == 0 {
				code = 1
			}
		}
	}
	os.Exit(code)
}

type sharedControlsFixture struct {
	rootDir      string
	homeDir      string
	bootstrapDir string
	baseURL      string

	process support.ApplicationProcess
	command *sharedControlsHostedCommand
	api     *support.ProcessAPIServer
	router  *sharedControlsCommandRouter

	sessionMu        sync.Mutex
	openedSessionIDs map[string]struct{}
	closedSessionIDs map[string]struct{}
	openMu           sync.Mutex
}

type sharedControlsHostedCommand struct {
	cancel context.CancelFunc
	done   chan struct{}

	mu  sync.Mutex
	err error
}

func newSharedControlsFixture() (*sharedControlsFixture, error) {
	rootDir, err := os.MkdirTemp("", "you-functional-sessions-controls-")
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
	if err := writeSharedControlsBootstrapFactory(bootstrapDir); err != nil {
		cleanup()
		return nil, err
	}
	mockWorkersPath, err := writeSharedControlsMockWorkers(rootDir)
	if err != nil {
		cleanup()
		return nil, err
	}

	api := support.NewProcessAPIServer()
	router := newSharedControlsCommandRouter()
	process, err := support.BuildProcessWithContext(context.Background(), serviceedges.Edges{
		APIServerStarter:          api.Start,
		FactorySessionIDGenerator: uuid.NewString,
		ProviderCommandRunner:     router,
		ScriptCommandRunner:       router,
	})
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("build root process: %w", err)
	}

	fixture := &sharedControlsFixture{
		rootDir:          rootDir,
		homeDir:          homeDir,
		bootstrapDir:     bootstrapDir,
		process:          process,
		api:              api,
		router:           router,
		openedSessionIDs: make(map[string]struct{}),
		closedSessionIDs: make(map[string]struct{}),
	}
	inputs := support.FakeInputs(context.Background(), []string{
		"you", "run", "--dir", bootstrapDir,
		"--continuously", "--with-server", "--server", "http://127.0.0.1:1",
		"--quiet", "--no-record", "--with-mock-workers", mockWorkersPath,
	})
	inputs.Input.Env = []string{"HOME=" + homeDir, "USERPROFILE=" + homeDir}
	inputs.Input.WorkingDirectory = bootstrapDir
	fixture.command = startSharedControlsHostedCommand(process, inputs.Input)
	baseURL, err := api.WaitForBaseURL(sharedControlsFixtureTimeout)
	if err != nil {
		_ = fixture.close()
		cleanup()
		return nil, fmt.Errorf("wait for loopback API: %w", err)
	}
	fixture.baseURL = baseURL
	if err := waitForSharedControlsRuntime(baseURL, sharedControlsFixtureTimeout); err != nil {
		_ = fixture.close()
		cleanup()
		return nil, err
	}
	return fixture, nil
}

func writeSharedControlsBootstrapFactory(dir string) error {
	if err := os.MkdirAll(filepath.Join(dir, "workers", "mock-worker"), 0o755); err != nil {
		return fmt.Errorf("create bootstrap worker directory: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "workstations", pauseResumeProcessTaskWorkstation), 0o755); err != nil {
		return fmt.Errorf("create bootstrap workstation directory: %w", err)
	}
	cfg := pauseResumeControlsFactoryConfig()
	cfg["name"] = "sessions-controls-shared-bootstrap"
	payload, err := json.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal bootstrap Factory: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, factorydefinitions.FactoryConfigFile), payload, 0o644); err != nil {
		return fmt.Errorf("write bootstrap Factory: %w", err)
	}
	if err := os.WriteFile(
		filepath.Join(dir, "workers", "mock-worker", "AGENTS.md"),
		[]byte("---\ntype: MODEL_WORKER\n---\nProcess the input task.\n"),
		0o644,
	); err != nil {
		return fmt.Errorf("write bootstrap worker instructions: %w", err)
	}
	if err := os.WriteFile(
		filepath.Join(dir, "workstations", pauseResumeProcessTaskWorkstation, "AGENTS.md"),
		[]byte("---\ntype: MODEL_WORKSTATION\n---\nProcess the input task.\n"),
		0o644,
	); err != nil {
		return fmt.Errorf("write bootstrap workstation instructions: %w", err)
	}
	workflowDir := filepath.Join(dir, ".claude", "workflows")
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		return fmt.Errorf("create bootstrap workflow directory: %w", err)
	}
	if err := os.WriteFile(
		filepath.Join(workflowDir, pauseResumeBusyLoopWorkflowName+".js"),
		[]byte(pauseResumeBusyLoopWorkflowSource),
		0o600,
	); err != nil {
		return fmt.Errorf("write bootstrap busy-loop workflow: %w", err)
	}
	return nil
}

func writeSharedControlsMockWorkers(rootDir string) (string, error) {
	accept := workers.MockWorkerConfig{
		WorkerName:      "mock-worker",
		WorkstationName: pauseResumeProcessTaskWorkstation,
		RunType:         workers.MockWorkerRunTypeAccept,
	}
	interrupted := workers.MockWorkerConfig{
		WorkerName:      "mock-worker",
		WorkstationName: interruptedInspectReviewWorkstation,
		RunType:         workers.MockWorkerRunTypeScript,
		ScriptConfig: &workers.MockWorkerScriptConfig{
			Command: "/bin/echo",
			Args:    []string{"interrupted"},
		},
	}
	config := &workers.MockWorkersConfig{
		UnmatchedDispatchPolicy: workers.MockWorkerUnmatchedDispatchPolicyPassthrough,
		MockWorkers:             []workers.MockWorkerConfig{accept, interrupted},
	}
	payload, err := json.Marshal(config)
	if err != nil {
		return "", fmt.Errorf("marshal shared mock workers config: %w", err)
	}
	path := filepath.Join(rootDir, "mock-workers.json")
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		return "", fmt.Errorf("write shared mock workers config: %w", err)
	}
	return path, nil
}

func startSharedControlsHostedCommand(process support.ApplicationProcess, input root.Input) *sharedControlsHostedCommand {
	parent := input.Context
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	input.Context = ctx
	command := &sharedControlsHostedCommand{cancel: cancel, done: make(chan struct{})}
	go func() {
		err := process.Execute(input)
		command.mu.Lock()
		command.err = err
		command.mu.Unlock()
		close(command.done)
	}()
	return command
}

func waitForSharedControlsRuntime(baseURL string, timeout time.Duration) error {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		response, err := http.Get(strings.TrimSuffix(baseURL, "/") + "/status")
		if err == nil {
			var status factoryapi.StatusResponse
			decodeErr := json.NewDecoder(response.Body).Decode(&status)
			response.Body.Close()
			if decodeErr == nil && strings.TrimSpace(status.RuntimeStatus) != "" {
				return nil
			}
		}
		select {
		case <-deadline.C:
			if err != nil {
				return fmt.Errorf("wait for shared controls runtime: %w", err)
			}
			return fmt.Errorf("wait for shared controls runtime: status remained unavailable")
		case <-ticker.C:
		}
	}
}

func (fixture *sharedControlsFixture) close() error {
	if fixture == nil {
		return nil
	}
	var closeErr error
	if fixture.command != nil {
		closeErr = fixture.command.stop()
	}
	if fixture.process != nil {
		ctx, cancel := context.WithTimeout(context.Background(), sharedControlsFixtureTimeout)
		closeErr = errors.Join(closeErr, fixture.process.Close(ctx))
		cancel()
	}
	if got := fixture.router.routeCount(); got != 0 {
		closeErr = errors.Join(closeErr, fmt.Errorf("shared controls command routes remaining after cleanup = %d", got))
	}
	if err := fixture.sessionLifecycleError(); err != nil {
		closeErr = errors.Join(closeErr, err)
	}
	if err := os.RemoveAll(fixture.rootDir); err != nil {
		closeErr = errors.Join(closeErr, fmt.Errorf("remove shared controls fixture root: %w", err))
	}
	return closeErr
}

func (command *sharedControlsHostedCommand) stop() error {
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
	case <-time.After(sharedControlsFixtureTimeout):
		return fmt.Errorf("timed out waiting for shared controls host shutdown")
	}
}

func (fixture *sharedControlsFixture) sessionLifecycleError() error {
	fixture.sessionMu.Lock()
	defer fixture.sessionMu.Unlock()
	if len(fixture.openedSessionIDs) != len(fixture.closedSessionIDs) {
		return fmt.Errorf(
			"shared controls Factory Session lifecycle opened %d sessions but closed %d",
			len(fixture.openedSessionIDs), len(fixture.closedSessionIDs),
		)
	}
	for sessionID := range fixture.openedSessionIDs {
		if _, ok := fixture.closedSessionIDs[sessionID]; !ok {
			return fmt.Errorf("shared controls Factory Session %q was not closed", sessionID)
		}
	}
	return nil
}

func (fixture *sharedControlsFixture) recordSessionOpened(sessionID string) error {
	fixture.sessionMu.Lock()
	defer fixture.sessionMu.Unlock()
	if _, exists := fixture.openedSessionIDs[sessionID]; exists {
		return fmt.Errorf("shared controls Factory Session %q opened twice", sessionID)
	}
	fixture.openedSessionIDs[sessionID] = struct{}{}
	return nil
}

func (fixture *sharedControlsFixture) recordSessionClosed(sessionID string) {
	fixture.sessionMu.Lock()
	defer fixture.sessionMu.Unlock()
	fixture.closedSessionIDs[sessionID] = struct{}{}
}

type sharedControlsRouteConfig struct {
	provider platformprocess.CommandRunner
	script   platformprocess.CommandRunner
	match    func(platformprocess.CommandRequest) bool
}

type sharedControlsCommandRouter struct {
	mu     sync.Mutex
	routes map[string]*sharedControlsRoute
}

type sharedControlsRoute struct {
	factoryDir string
	provider   platformprocess.CommandRunner
	script     platformprocess.CommandRunner
	match      func(platformprocess.CommandRequest) bool
}

func newSharedControlsCommandRouter() *sharedControlsCommandRouter {
	return &sharedControlsCommandRouter{routes: make(map[string]*sharedControlsRoute)}
}

func (router *sharedControlsCommandRouter) register(factoryDir string, config sharedControlsRouteConfig) error {
	abs, err := filepath.Abs(filepath.Clean(factoryDir))
	if err != nil || strings.TrimSpace(abs) == "" {
		return fmt.Errorf("shared controls Factory directory is invalid: %q", factoryDir)
	}
	router.mu.Lock()
	defer router.mu.Unlock()
	if _, exists := router.routes[abs]; exists {
		return fmt.Errorf("shared controls route for %q is already registered", abs)
	}
	router.routes[abs] = &sharedControlsRoute{
		factoryDir: abs,
		provider:   config.provider,
		script:     config.script,
		match:      config.match,
	}
	return nil
}

func (router *sharedControlsCommandRouter) unregister(factoryDir string) {
	abs, _ := filepath.Abs(filepath.Clean(factoryDir))
	router.mu.Lock()
	delete(router.routes, abs)
	router.mu.Unlock()
}

func (router *sharedControlsCommandRouter) routeCount() int {
	router.mu.Lock()
	defer router.mu.Unlock()
	return len(router.routes)
}

func (router *sharedControlsCommandRouter) Run(
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
		if isSharedControlsProviderCommand(request.Command) {
			runner = route.provider
		} else {
			runner = route.script
		}
	}
	router.mu.Unlock()
	if route == nil {
		return platformprocess.CommandResult{}, fmt.Errorf(
			"no shared controls command route matched work directory %q",
			request.WorkDir,
		)
	}
	if runner == nil {
		return platformprocess.CommandResult{}, fmt.Errorf(
			"shared controls route for %q has no runner for command %q",
			route.factoryDir,
			request.Command,
		)
	}
	return runner.Run(ctx, request)
}

func (router *sharedControlsCommandRouter) routeForRequest(request platformprocess.CommandRequest) *sharedControlsRoute {
	var best *sharedControlsRoute
	for factoryDir, route := range router.routes {
		matched := route.match != nil && route.match(request)
		if !matched {
			matched = sharedControlsPathBelongsTo(factoryDir, request.WorkDir) ||
				sharedControlsPathBelongsTo(factoryDir, request.Command)
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

func sharedControlsPathBelongsTo(factoryDir, candidate string) bool {
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

func isSharedControlsProviderCommand(command string) bool {
	switch strings.ToLower(filepath.Base(strings.TrimSpace(command))) {
	case "codex", "claude", "agy":
		return true
	default:
		return false
	}
}

type sharedControlsSession struct {
	fixture    *sharedControlsFixture
	factoryDir string
	sessionID  string
	closeOnce  sync.Once
}

func openSharedControlsSession(
	t *testing.T,
	factoryDir string,
	config sharedControlsRouteConfig,
) *sharedControlsSession {
	t.Helper()
	acquireSharedControlsScenarioSlot(t)
	fixture := sharedControlsProcess(t)
	abs, err := filepath.Abs(filepath.Clean(factoryDir))
	if err != nil {
		t.Fatalf("resolve controls Factory directory: %v", err)
	}
	if err := fixture.router.register(abs, config); err != nil {
		t.Fatalf("register shared controls command route: %v", err)
	}
	t.Cleanup(func() { fixture.router.unregister(abs) })

	fixture.openMu.Lock()
	defer fixture.openMu.Unlock()
	opened := support.OpenFactorySessionAt(t, fixture.baseURL, abs)
	sessionID := opened.Session.Id
	if err := fixture.recordSessionOpened(sessionID); err != nil {
		support.CloseFactorySessionAt(t, fixture.baseURL, sessionID)
		t.Fatalf("record shared controls Factory Session %q: %v", sessionID, err)
	}
	session := &sharedControlsSession{fixture: fixture, factoryDir: abs, sessionID: sessionID}
	t.Cleanup(func() { session.close(t) })
	return session
}

func (session *sharedControlsSession) baseURL() string {
	if session == nil || session.fixture == nil {
		return ""
	}
	return session.fixture.baseURL
}

func (session *sharedControlsSession) id() string {
	if session == nil {
		return ""
	}
	return session.sessionID
}

func (session *sharedControlsSession) close(t testing.TB) {
	if session == nil {
		return
	}
	session.closeOnce.Do(func() {
		support.CloseFactorySessionAt(t, session.fixture.baseURL, session.sessionID)
		session.fixture.recordSessionClosed(session.sessionID)
	})
}

var _ platformprocess.CommandRunner = (*sharedControlsCommandRouter)(nil)
