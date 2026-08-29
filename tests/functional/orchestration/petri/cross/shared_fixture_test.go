package cross

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
	"sync/atomic"
	"testing"
	"time"

	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const sharedCrossFixtureShutdownTimeout = 15 * time.Second

var (
	sharedCrossFixtureOnce sync.Once
	sharedCrossFixture     *sharedCrossProcessFixture
	sharedCrossFixtureErr  error
)

// TestMain owns the one production-composed process and loopback API for this
// package. The individual behavior cases own only their Factory directories
// and explicit Factory Sessions.
func TestMain(m *testing.M) {
	code := m.Run()
	if sharedCrossFixture != nil {
		if err := sharedCrossFixture.close(); err != nil {
			fmt.Fprintf(os.Stderr, "close shared Petri cross fixture: %v\n", err)
			if code == 0 {
				code = 1
			}
		}
	}
	os.Exit(code)
}

type sharedCrossProcessFixture struct {
	rootDir      string
	homeDir      string
	bootstrapDir string
	baseURL      string

	process    support.ApplicationProcess
	command    *sharedCrossHostedCommand
	api        *support.ProcessAPIServer
	apiStarter *sharedCrossAPIServerStarter
	router     *sharedCrossCommandRouter

	requestSequence  atomic.Uint64
	sessionMu        sync.Mutex
	openedSessionIDs map[string]struct{}
	closedSessionIDs map[string]struct{}
}

type sharedCrossAPIServerStarter struct {
	api    *support.ProcessAPIServer
	starts atomic.Int32
}

func (starter *sharedCrossAPIServerStarter) Start(
	ctx context.Context,
	request platformhttpserver.StartRequest,
) error {
	starter.starts.Add(1)
	return starter.api.Start(ctx, request)
}

type sharedCrossHostedCommand struct {
	cancel context.CancelFunc
	done   chan struct{}

	mu  sync.Mutex
	err error
}

func sharedCrossProcess(t testing.TB) *sharedCrossProcessFixture {
	t.Helper()
	sharedCrossFixtureOnce.Do(func() {
		sharedCrossFixture, sharedCrossFixtureErr = newSharedCrossProcessFixture(t)
	})
	if sharedCrossFixtureErr != nil {
		t.Fatalf("start shared Petri cross fixture: %v", sharedCrossFixtureErr)
	}
	if sharedCrossFixture == nil {
		t.Fatal("shared Petri cross fixture is unavailable")
	}
	return sharedCrossFixture
}

func newSharedCrossProcessFixture(t testing.TB) (*sharedCrossProcessFixture, error) {
	t.Helper()

	rootDir, err := os.MkdirTemp("", "you-functional-petri-cross-")
	if err != nil {
		return nil, fmt.Errorf("create fixture root: %w", err)
	}
	cleanupRoot := func() { _ = os.RemoveAll(rootDir) }
	homeDir := filepath.Join(rootDir, "home")
	bootstrapDir := filepath.Join(rootDir, "bootstrap")
	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		cleanupRoot()
		return nil, fmt.Errorf("create fixture home: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(rootDir, "scenarios"), 0o755); err != nil {
		cleanupRoot()
		return nil, fmt.Errorf("create scenario root: %w", err)
	}
	if err := writeSharedCrossFactory(bootstrapDir, "shared-cross-bootstrap"); err != nil {
		cleanupRoot()
		return nil, fmt.Errorf("write bootstrap Factory: %w", err)
	}
	if err := writeSharedCrossWorkflow(bootstrapDir); err != nil {
		cleanupRoot()
		return nil, fmt.Errorf("write bootstrap workflow: %w", err)
	}

	api := support.NewProcessAPIServer()
	apiStarter := &sharedCrossAPIServerStarter{api: api}
	router := newSharedCrossCommandRouter()
	process, err := support.BuildProcessWithContext(context.Background(), serviceedges.Edges{
		APIServerStarter:      apiStarter.Start,
		ProviderCommandRunner: router,
		ScriptCommandRunner:   router,
	})
	if err != nil {
		cleanupRoot()
		return nil, fmt.Errorf("build root process: %w", err)
	}

	fixture := &sharedCrossProcessFixture{
		rootDir:          rootDir,
		homeDir:          homeDir,
		bootstrapDir:     bootstrapDir,
		process:          process,
		api:              api,
		apiStarter:       apiStarter,
		router:           router,
		openedSessionIDs: make(map[string]struct{}),
		closedSessionIDs: make(map[string]struct{}),
	}
	inputs := support.FakeInputs(context.Background(), []string{
		"you", "run",
		"--dir", bootstrapDir,
		"--continuously",
		"--with-server",
		"--server", "http://127.0.0.1:1",
		"--quiet",
		"--no-record",
	})
	inputs.Input.Env = []string{"HOME=" + homeDir, "USERPROFILE=" + homeDir}
	inputs.Input.WorkingDirectory = bootstrapDir
	fixture.command = startSharedCrossHostedCommand(process, inputs.Input)

	baseURL, err := api.WaitForBaseURL(sharedCrossFixtureShutdownTimeout)
	if err != nil {
		_ = fixture.close()
		cleanupRoot()
		return nil, fmt.Errorf("wait for loopback API: %w", err)
	}
	fixture.baseURL = baseURL
	support.WaitForStatus(t, baseURL, sharedCrossFixtureShutdownTimeout, func(status factoryapi.StatusResponse) bool {
		return strings.TrimSpace(status.RuntimeStatus) != ""
	})
	return fixture, nil
}

func startSharedCrossHostedCommand(process support.Process, input root.Input) *sharedCrossHostedCommand {
	parent := input.Context
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	input.Context = ctx
	command := &sharedCrossHostedCommand{cancel: cancel, done: make(chan struct{})}
	go func() {
		err := process.Execute(input)
		command.mu.Lock()
		command.err = err
		command.mu.Unlock()
		close(command.done)
	}()
	return command
}

func (fixture *sharedCrossProcessFixture) close() error {
	if fixture == nil {
		return nil
	}
	var closeErr error
	if fixture.command != nil {
		closeErr = fixture.command.stop()
	}
	if fixture.process != nil {
		ctx, cancel := context.WithTimeout(context.Background(), sharedCrossFixtureShutdownTimeout)
		closeErr = errors.Join(closeErr, fixture.process.Close(ctx))
		cancel()
	}
	if fixture.baseURL != "" {
		client := http.Client{Timeout: time.Second}
		response, err := client.Get(fixture.baseURL + "/status")
		if err == nil {
			_ = response.Body.Close()
			closeErr = errors.Join(closeErr, fmt.Errorf("shared cross API port remains reachable after process close"))
		}
	}
	if fixture.apiStarter != nil && fixture.apiStarter.starts.Load() != 1 {
		closeErr = errors.Join(closeErr, fmt.Errorf(
			"cross fixture API server starts = %d, want exactly one",
			fixture.apiStarter.starts.Load(),
		))
	}
	if fixture.router != nil && fixture.router.routeCount() != 0 {
		closeErr = errors.Join(closeErr, fmt.Errorf(
			"shared cross command routes remaining after cleanup = %d",
			fixture.router.routeCount(),
		))
	}
	closeErr = errors.Join(closeErr, fixture.sessionLifecycleError())
	if fixture.rootDir != "" {
		if err := os.RemoveAll(fixture.rootDir); err != nil {
			closeErr = errors.Join(closeErr, fmt.Errorf("remove fixture root: %w", err))
		}
		if _, err := os.Stat(fixture.rootDir); err == nil {
			closeErr = errors.Join(closeErr, fmt.Errorf("fixture root still exists: %s", fixture.rootDir))
		} else if !os.IsNotExist(err) {
			closeErr = errors.Join(closeErr, fmt.Errorf("probe removed fixture root: %w", err))
		}
	}
	return closeErr
}

func (command *sharedCrossHostedCommand) stop() error {
	if command == nil {
		return nil
	}
	command.cancel()
	// Hosted shutdown has no completion channel exposed by the public process
	// contract; this bounded wait is cleanup protection for a missing exit, not
	// a workflow delay or polling-based readiness assertion.
	select {
	case <-command.done:
		command.mu.Lock()
		err := command.err
		command.mu.Unlock()
		if err != nil && !errors.Is(err, context.Canceled) {
			return err
		}
		return nil
	case <-time.After(sharedCrossFixtureShutdownTimeout):
		return fmt.Errorf("timed out waiting for shared cross host shutdown")
	}
}

func (fixture *sharedCrossProcessFixture) sessionLifecycleError() error {
	fixture.sessionMu.Lock()
	defer fixture.sessionMu.Unlock()
	if len(fixture.openedSessionIDs) != len(fixture.closedSessionIDs) {
		return fmt.Errorf(
			"shared cross Factory Session lifecycle opened %d sessions but closed %d",
			len(fixture.openedSessionIDs),
			len(fixture.closedSessionIDs),
		)
	}
	for sessionID := range fixture.openedSessionIDs {
		if _, closed := fixture.closedSessionIDs[sessionID]; !closed {
			return fmt.Errorf("shared cross Factory Session %q was not closed", sessionID)
		}
	}
	return nil
}

func (fixture *sharedCrossProcessFixture) recordSessionOpened(sessionID string) error {
	fixture.sessionMu.Lock()
	defer fixture.sessionMu.Unlock()
	if _, exists := fixture.openedSessionIDs[sessionID]; exists {
		return fmt.Errorf("Factory Session %q was opened twice", sessionID)
	}
	fixture.openedSessionIDs[sessionID] = struct{}{}
	return nil
}

func (fixture *sharedCrossProcessFixture) recordSessionClosed(sessionID string) {
	fixture.sessionMu.Lock()
	defer fixture.sessionMu.Unlock()
	fixture.closedSessionIDs[sessionID] = struct{}{}
}

func (fixture *sharedCrossProcessFixture) nextRequestID() string {
	return fmt.Sprintf("cross-session-request-%d", fixture.requestSequence.Add(1))
}

type sharedCrossCommandRouter struct {
	mu     sync.Mutex
	routes map[string]struct{}
}

func newSharedCrossCommandRouter() *sharedCrossCommandRouter {
	return &sharedCrossCommandRouter{routes: make(map[string]struct{})}
}

func (router *sharedCrossCommandRouter) register(factoryDir string) error {
	factoryDir = filepath.Clean(factoryDir)
	if factoryDir == "." || strings.TrimSpace(factoryDir) == "" {
		return errors.New("cross command route Factory directory is required")
	}
	router.mu.Lock()
	defer router.mu.Unlock()
	if _, exists := router.routes[factoryDir]; exists {
		return fmt.Errorf("cross command route for %q is already registered", factoryDir)
	}
	router.routes[factoryDir] = struct{}{}
	return nil
}

func (router *sharedCrossCommandRouter) unregister(factoryDir string) {
	router.mu.Lock()
	delete(router.routes, filepath.Clean(factoryDir))
	router.mu.Unlock()
}

func (router *sharedCrossCommandRouter) routeCount() int {
	router.mu.Lock()
	defer router.mu.Unlock()
	return len(router.routes)
}

func (router *sharedCrossCommandRouter) Run(
	ctx context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	if err := ctx.Err(); err != nil {
		return platformprocess.CommandResult{}, err
	}
	router.mu.Lock()
	matched := false
	for factoryDir := range router.routes {
		if sharedCrossPathBelongsTo(factoryDir, request.WorkDir) ||
			sharedCrossPathBelongsTo(factoryDir, request.Command) {
			matched = true
			break
		}
	}
	router.mu.Unlock()
	if !matched {
		return platformprocess.CommandResult{}, fmt.Errorf("no cross command route matched the request")
	}
	return platformprocess.CommandResult{Stdout: []byte("cross-command-ok")}, nil
}

func sharedCrossPathBelongsTo(factoryDir, candidate string) bool {
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

type sharedCrossSession struct {
	fixture    *sharedCrossProcessFixture
	factoryDir string
	sessionID  string
	durable    bool

	closeOnce sync.Once
}

func openSharedCrossLiveSession(t *testing.T) *sharedCrossSession {
	t.Helper()
	fixture := sharedCrossProcess(t)
	factoryDir := newSharedCrossScenarioFactory(t, fixture)
	if err := fixture.router.register(factoryDir); err != nil {
		t.Fatalf("register cross command route: %v", err)
	}
	opened := support.OpenFactorySessionAt(t, fixture.baseURL, factoryDir)
	sessionID := opened.Session.Id
	if err := fixture.recordSessionOpened(sessionID); err != nil {
		support.CloseFactorySessionAt(t, fixture.baseURL, sessionID)
		fixture.router.unregister(factoryDir)
		t.Fatalf("record opened cross Factory Session: %v", err)
	}
	session := &sharedCrossSession{
		fixture:    fixture,
		factoryDir: filepath.Clean(factoryDir),
		sessionID:  sessionID,
	}
	t.Cleanup(func() { session.close(t) })
	return session
}

func startSharedCrossJavaScriptSession(t *testing.T) *sharedCrossSession {
	t.Helper()
	fixture := sharedCrossProcess(t)
	workflowName := busyLoopWorkflowName
	started := postJSON[factoryapi.FactorySessionExecutionResponse](
		t,
		fixture.baseURL+"/factory-sessions/async",
		factoryapi.FactorySessionExecutionRequest{
			RequestId: fixture.nextRequestID(),
			Source: factoryapi.FactorySessionExecutionSource{
				Kind:         factoryapi.FactorySessionExecutionSourceKindWorkflowName,
				WorkflowName: &workflowName,
			},
		},
		"start shared cross JavaScript Factory Session",
	)
	if strings.TrimSpace(started.SessionId) == "" {
		t.Fatalf("shared cross JavaScript session id is empty: %#v", started)
	}
	if err := fixture.recordSessionOpened(started.SessionId); err != nil {
		t.Fatalf("record opened cross durable Factory Session: %v", err)
	}
	session := &sharedCrossSession{
		fixture:   fixture,
		sessionID: started.SessionId,
		durable:   true,
	}
	t.Cleanup(func() { session.close(t) })
	return session
}

func (session *sharedCrossSession) close(t testing.TB) {
	if session == nil {
		return
	}
	session.closeOnce.Do(func() {
		if session.durable {
			support.TerminateFactorySessionAt(t, session.fixture.baseURL, session.sessionID)
			waitForDurableSessionTerminal(t, session.fixture.baseURL, session.sessionID, sessionCompatTimeout)
		} else {
			support.CloseFactorySessionAt(t, session.fixture.baseURL, session.sessionID)
		}
		if session.factoryDir != "" {
			session.fixture.router.unregister(session.factoryDir)
			if err := os.RemoveAll(session.factoryDir); err != nil {
				t.Errorf("remove cross scenario Factory %s: %v", session.factoryDir, err)
			}
		}
		session.fixture.recordSessionClosed(session.sessionID)
	})
}

func newSharedCrossScenarioFactory(t testing.TB, fixture *sharedCrossProcessFixture) string {
	t.Helper()
	dir, err := os.MkdirTemp(filepath.Join(fixture.rootDir, "scenarios"), "factory-")
	if err != nil {
		t.Fatalf("create cross scenario Factory: %v", err)
	}
	if err := writeSharedCrossFactory(dir, "session-compatibility"); err != nil {
		_ = os.RemoveAll(dir)
		t.Fatalf("write cross scenario Factory: %v", err)
	}
	return dir
}

func writeSharedCrossFactory(dir, name string) error {
	config := map[string]any{
		"name": name,
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]string{{"name": "worker-a"}},
		"workstations": []map[string]any{{
			"name":      "process",
			"worker":    "worker-a",
			"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
			"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
			"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
		}},
	}
	raw, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("marshal Factory config: %w", err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create Factory directory: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, interfaces.FactoryConfigFile), raw, 0o644); err != nil {
		return fmt.Errorf("write Factory config: %w", err)
	}
	workstationPath := filepath.Join(dir, "workstations", "process", "AGENTS.md")
	if err := os.MkdirAll(filepath.Dir(workstationPath), 0o755); err != nil {
		return fmt.Errorf("create workstation directory: %w", err)
	}
	if err := os.WriteFile(workstationPath, []byte("---\ntype: MODEL_WORKSTATION\n---\nDo the work.\n"), 0o644); err != nil {
		return fmt.Errorf("write workstation config: %w", err)
	}
	return nil
}

func writeSharedCrossWorkflow(dir string) error {
	workflowPath := filepath.Join(dir, ".claude", "workflows", busyLoopWorkflowName+".js")
	if err := os.MkdirAll(filepath.Dir(workflowPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(workflowPath, []byte("var spin = 0;\nwhile (true) {\n  spin += 1;\n}\n"), 0o600)
}
