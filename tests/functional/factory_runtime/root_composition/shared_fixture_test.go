package root_composition_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/root"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const sharedRootCompositionFixtureShutdownTimeout = 15 * time.Second

var (
	sharedRootCompositionFixtureOnce  sync.Once
	sharedRootCompositionFixture      *sharedRootCompositionProcessFixture
	sharedRootCompositionFixtureErr   error
	sharedRootCompositionScenarioSlot = make(chan struct{}, 1)
)

// TestMain starts and owns the one eligible root-composition process and API
// listener before parallel test execution begins.
// RC01, RC02, and RC05 intentionally do not use this fixture because their
// construction, packaged-shape, and CLI/replay boundaries are process-scoped.
func TestMain(m *testing.M) {
	sharedRootCompositionFixtureOnce.Do(func() {
		sharedRootCompositionFixture, sharedRootCompositionFixtureErr = newSharedRootCompositionProcessFixture()
	})
	code := m.Run()
	if sharedRootCompositionFixtureErr != nil {
		fmt.Fprintf(os.Stderr, "start shared root-composition fixture: %v\n", sharedRootCompositionFixtureErr)
		if code == 0 {
			code = 1
		}
	}
	if sharedRootCompositionFixture != nil {
		if err := sharedRootCompositionFixture.close(); err != nil {
			fmt.Fprintf(os.Stderr, "close shared root-composition fixture: %v\n", err)
			if code == 0 {
				code = 1
			}
		}
	}
	os.Exit(code)
}

// enterSharedRootCompositionScenario serializes eligible root-composition
// scenarios while retaining their top-level t.Parallel declarations. This
// keeps shared recorder deltas and provider-route call counts attributable to
// one fresh Factory Session at a time.
func enterSharedRootCompositionScenario(t *testing.T) {
	t.Helper()
	sharedRootCompositionScenarioSlot <- struct{}{}
	t.Cleanup(func() { <-sharedRootCompositionScenarioSlot })
}

func sharedRootCompositionFixtureForTest(t *testing.T) *sharedRootCompositionProcessFixture {
	t.Helper()
	sharedRootCompositionFixtureOnce.Do(func() {
		sharedRootCompositionFixture, sharedRootCompositionFixtureErr = newSharedRootCompositionProcessFixture()
	})
	if sharedRootCompositionFixtureErr != nil {
		t.Fatalf("start shared root-composition fixture: %v", sharedRootCompositionFixtureErr)
	}
	if sharedRootCompositionFixture == nil {
		t.Fatal("shared root-composition fixture is unavailable")
	}
	support.WaitForRuntimeIdle(t, sharedRootCompositionFixture.baseURL, sharedRootCompositionFixtureShutdownTimeout)
	return sharedRootCompositionFixture
}

type sharedRootCompositionProcessFixture struct {
	rootDir      string
	homeDir      string
	bootstrapDir string
	jsFactoryDir string
	baseURL      string

	process   support.ApplicationProcess
	command   *sharedRootCompositionHostedCommand
	api       *support.ProcessAPIServer
	apiStops  chan struct{}
	apiOnce   sync.Once
	apiStarts atomic.Int32
	stream    sharedRootCompositionStreamLifecycle
	recorder  *factoryRuntimeDelegatingRecorder
	router    *sharedRootCompositionCommandRouter

	processStarts atomic.Int32
	processStops  atomic.Int32
	processMu     sync.Mutex
	processErr    error

	sessionMu        sync.Mutex
	openedSessionIDs map[string]struct{}
	closedSessionIDs map[string]struct{}
}

type sharedRootCompositionHostedCommand struct {
	cancel context.CancelFunc
	done   chan struct{}

	mu  sync.Mutex
	err error
}

func newSharedRootCompositionProcessFixture() (*sharedRootCompositionProcessFixture, error) {
	rootDir, err := os.MkdirTemp("", "you-functional-root-composition-")
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
	if err := writeSharedRootCompositionBootstrapFactory(bootstrapDir); err != nil {
		cleanupRoot()
		return nil, fmt.Errorf("write bootstrap Factory: %w", err)
	}
	jsFactoryDir, err := writeSharedRootCompositionJavaScriptFactory(homeDir)
	if err != nil {
		cleanupRoot()
		return nil, fmt.Errorf("write named JavaScript Factory: %w", err)
	}

	api := support.NewProcessAPIServer()
	recorder := &factoryRuntimeDelegatingRecorder{workflowHome: homeDir}
	router := newSharedRootCompositionCommandRouter()
	fixture := &sharedRootCompositionProcessFixture{
		rootDir:          rootDir,
		homeDir:          homeDir,
		bootstrapDir:     bootstrapDir,
		jsFactoryDir:     jsFactoryDir,
		api:              api,
		apiStops:         make(chan struct{}),
		recorder:         recorder,
		router:           router,
		openedSessionIDs: make(map[string]struct{}),
		closedSessionIDs: make(map[string]struct{}),
	}
	edges := recorder.edges()
	edges.APIServerStarter = fixture.startAPIServer
	edges.ProviderCommandRunner = router
	process, err := support.BuildProcessWithContext(context.Background(), edges)
	if err != nil {
		cleanupRoot()
		return nil, fmt.Errorf("build root process: %w", err)
	}
	fixture.process = process

	// The shared process owns the API listener and an inert bootstrap session;
	// eligible cases open their own Factory Session from their independent
	// fixture directory after this invocation has reached readiness.
	processContext, cancel := context.WithCancel(context.Background())
	inputs := support.FakeInputs(processContext, []string{
		"you", "run", "--dir", bootstrapDir, "--continuously", "--with-server",
		"--server", "http://127.0.0.1:1", "--quiet", "--no-record",
	})
	inputs.Input.Env = sharedRootCompositionEnvironment(homeDir)
	inputs.Input.WorkingDirectory = bootstrapDir
	fixture.processStarts.Add(1)
	fixture.command = startSharedRootCompositionHostedCommand(process, inputs.Input, cancel, func(err error) {
		fixture.processMu.Lock()
		fixture.processErr = err
		fixture.processMu.Unlock()
		fixture.processStops.Add(1)
	})

	baseURL, err := api.WaitForBaseURL(sharedRootCompositionFixtureShutdownTimeout)
	if err != nil {
		_ = fixture.close()
		return nil, fmt.Errorf("wait for loopback API: %w", err)
	}
	fixture.baseURL = baseURL
	return fixture, nil
}

func startSharedRootCompositionHostedCommand(
	process support.Process,
	input root.Input,
	cancel context.CancelFunc,
	onDone func(error),
) *sharedRootCompositionHostedCommand {
	parent := input.Context
	if parent == nil {
		parent = context.Background()
	}
	if cancel == nil {
		var derived context.CancelFunc
		parent, derived = context.WithCancel(parent)
		cancel = derived
	}
	input.Context = parent
	command := &sharedRootCompositionHostedCommand{cancel: cancel, done: make(chan struct{})}
	go func() {
		err := process.Execute(input)
		command.mu.Lock()
		command.err = err
		command.mu.Unlock()
		if onDone != nil {
			onDone(err)
		}
		close(command.done)
	}()
	return command
}

func (command *sharedRootCompositionHostedCommand) stop() error {
	if command == nil {
		return nil
	}
	command.cancel()
	// This bounded wait only protects fixture teardown from a missing process
	// exit; it is not used as workflow synchronization.
	select {
	case <-command.done:
		command.mu.Lock()
		err := command.err
		command.mu.Unlock()
		if err != nil && !errors.Is(err, context.Canceled) {
			return err
		}
		return nil
	case <-time.After(sharedRootCompositionFixtureShutdownTimeout):
		return fmt.Errorf("timed out waiting for shared root-composition host shutdown")
	}
}

func (fixture *sharedRootCompositionProcessFixture) startAPIServer(
	ctx context.Context,
	request platformhttpserver.StartRequest,
) error {
	fixture.apiStarts.Add(1)
	request.Handler = fixture.stream.wrap(request.Handler)
	err := fixture.api.Start(ctx, request)
	fixture.apiOnce.Do(func() { close(fixture.apiStops) })
	return err
}

func (fixture *sharedRootCompositionProcessFixture) close() error {
	if fixture == nil {
		return nil
	}
	var closeErr error
	if fixture.command != nil {
		closeErr = fixture.command.stop()
	}
	if fixture.process != nil {
		closeContext, cancel := context.WithTimeout(context.Background(), sharedRootCompositionFixtureShutdownTimeout)
		closeErr = errors.Join(closeErr, fixture.process.Close(closeContext))
		cancel()
	}
	if fixture.apiStarts.Load() != 0 {
		select {
		case <-fixture.apiStops:
		case <-time.After(sharedRootCompositionFixtureShutdownTimeout):
			closeErr = errors.Join(closeErr, fmt.Errorf("timed out waiting for shared root-composition API shutdown"))
		}
	}
	if got := fixture.apiStarts.Load(); got != 1 {
		closeErr = errors.Join(closeErr, fmt.Errorf("root-composition API starts = %d, want one", got))
	}
	if got := fixture.processStarts.Load(); got != 1 {
		closeErr = errors.Join(closeErr, fmt.Errorf("root-composition process starts = %d, want one", got))
	}
	if got := fixture.processStops.Load(); got != fixture.processStarts.Load() {
		closeErr = errors.Join(closeErr, fmt.Errorf("root-composition process stops = %d, starts = %d", got, fixture.processStarts.Load()))
	}
	fixture.processMu.Lock()
	processErr := fixture.processErr
	fixture.processMu.Unlock()
	if processErr != nil && !errors.Is(processErr, context.Canceled) {
		closeErr = errors.Join(closeErr, fmt.Errorf("root-composition Process.Execute shutdown: %w", processErr))
	}
	if got := fixture.router.routeCount(); got != 0 {
		closeErr = errors.Join(closeErr, fmt.Errorf("root-composition active command routes = %d", got))
	}
	closeErr = errors.Join(closeErr, fixture.sessionLifecycleError())
	if active := fixture.stream.active.Load(); active != 0 || fixture.stream.opened.Load() != fixture.stream.closed.Load() {
		closeErr = errors.Join(closeErr, fmt.Errorf(
			"root-composition SSE streams active=%d opened=%d closed=%d",
			active, fixture.stream.opened.Load(), fixture.stream.closed.Load(),
		))
	}

	listenerClosed := fixture.apiStarts.Load() == 0
	if fixture.apiStarts.Load() != 0 && strings.TrimSpace(fixture.baseURL) != "" {
		listenerClosed = true
		client := http.Client{Timeout: time.Second}
		response, err := client.Get(strings.TrimSuffix(fixture.baseURL, "/") + "/status")
		if err == nil {
			listenerClosed = false
			body, _ := io.ReadAll(response.Body)
			response.Body.Close()
			closeErr = errors.Join(closeErr, fmt.Errorf(
				"root-composition API listener remained available after shutdown: status=%d body=%q",
				response.StatusCode, strings.TrimSpace(string(body)),
			))
		}
	}
	if !listenerClosed {
		closeErr = errors.Join(closeErr, fmt.Errorf("root-composition API listener is still active"))
	}
	if err := os.RemoveAll(fixture.rootDir); err != nil {
		closeErr = errors.Join(closeErr, fmt.Errorf("remove root-composition fixture root: %w", err))
	} else if _, err := os.Stat(fixture.rootDir); err == nil {
		closeErr = errors.Join(closeErr, fmt.Errorf("root-composition fixture root still exists: %s", fixture.rootDir))
	} else if !errors.Is(err, os.ErrNotExist) {
		closeErr = errors.Join(closeErr, fmt.Errorf("probe removed root-composition fixture root: %w", err))
	}
	fixture.writeRuntimeReport()
	return closeErr
}

func (fixture *sharedRootCompositionProcessFixture) writeRuntimeReport() {
	fixture.sessionMu.Lock()
	sessions := make([]string, 0, len(fixture.openedSessionIDs))
	for sessionID := range fixture.openedSessionIDs {
		sessions = append(sessions, sessionID)
	}
	sort.Strings(sessions)
	closed := len(fixture.closedSessionIDs)
	fixture.sessionMu.Unlock()
	report := map[string]any{
		"processStarts":  fixture.processStarts.Load(),
		"processStops":   fixture.processStops.Load(),
		"apiStarts":      fixture.apiStarts.Load(),
		"sessions":       sessions,
		"closedSessions": closed,
		"activeRoutes":   fixture.router.routeCount(),
		"streamsOpened":  fixture.stream.opened.Load(),
		"streamsClosed":  fixture.stream.closed.Load(),
	}
	encoded, err := json.Marshal(report)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ROOT_COMPOSITION_RUNTIME_MATRIX encode error: %v\n", err)
		return
	}
	fmt.Fprintf(os.Stdout, "ROOT_COMPOSITION_RUNTIME_MATRIX %s\n", encoded)
}

func (fixture *sharedRootCompositionProcessFixture) recordSessionOpened(sessionID string) error {
	fixture.sessionMu.Lock()
	defer fixture.sessionMu.Unlock()
	if _, exists := fixture.openedSessionIDs[sessionID]; exists {
		return fmt.Errorf("Factory Session %q was opened twice", sessionID)
	}
	fixture.openedSessionIDs[sessionID] = struct{}{}
	return nil
}

func (fixture *sharedRootCompositionProcessFixture) recordSessionClosed(sessionID string) {
	fixture.sessionMu.Lock()
	defer fixture.sessionMu.Unlock()
	fixture.closedSessionIDs[sessionID] = struct{}{}
}

func (fixture *sharedRootCompositionProcessFixture) sessionLifecycleError() error {
	fixture.sessionMu.Lock()
	defer fixture.sessionMu.Unlock()
	if len(fixture.openedSessionIDs) != len(fixture.closedSessionIDs) {
		return fmt.Errorf(
			"root-composition Factory Session lifecycle opened %d sessions but closed %d",
			len(fixture.openedSessionIDs), len(fixture.closedSessionIDs),
		)
	}
	for sessionID := range fixture.openedSessionIDs {
		if _, closed := fixture.closedSessionIDs[sessionID]; !closed {
			return fmt.Errorf("root-composition Factory Session %q was not closed", sessionID)
		}
	}
	return nil
}

type sharedRootCompositionStreamLifecycle struct {
	active atomic.Int32
	opened atomic.Int32
	closed atomic.Int32
}

func (lifecycle *sharedRootCompositionStreamLifecycle) wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		isStream := strings.HasSuffix(request.URL.Path, "/events") ||
			strings.HasSuffix(request.URL.Path, "/response-events")
		if !isStream {
			next.ServeHTTP(writer, request)
			return
		}
		lifecycle.opened.Add(1)
		lifecycle.active.Add(1)
		defer func() {
			lifecycle.active.Add(-1)
			lifecycle.closed.Add(1)
		}()
		next.ServeHTTP(writer, request)
	})
}

type sharedRootCompositionCommandRoute struct {
	factoryDir    string
	providerCalls int
}

type sharedRootCompositionCommandRouter struct {
	mu      sync.Mutex
	routes  map[string]*sharedRootCompositionCommandRoute
	history map[string]sharedRootCompositionCommandRoute
}

func newSharedRootCompositionCommandRouter() *sharedRootCompositionCommandRouter {
	return &sharedRootCompositionCommandRouter{
		routes:  make(map[string]*sharedRootCompositionCommandRoute),
		history: make(map[string]sharedRootCompositionCommandRoute),
	}
}

func (router *sharedRootCompositionCommandRouter) register(factoryDir string) error {
	factoryDir = filepath.Clean(factoryDir)
	if factoryDir == "." || strings.TrimSpace(factoryDir) == "" {
		return errors.New("root-composition Factory directory is required")
	}
	router.mu.Lock()
	defer router.mu.Unlock()
	if _, exists := router.routes[factoryDir]; exists {
		return fmt.Errorf("root-composition command route for %q is already registered", factoryDir)
	}
	router.routes[factoryDir] = &sharedRootCompositionCommandRoute{factoryDir: factoryDir}
	return nil
}

func (router *sharedRootCompositionCommandRouter) unregister(factoryDir string) {
	router.mu.Lock()
	defer router.mu.Unlock()
	factoryDir = filepath.Clean(factoryDir)
	if route := router.routes[factoryDir]; route != nil {
		router.history[factoryDir] = *route
	}
	delete(router.routes, factoryDir)
}

func (router *sharedRootCompositionCommandRouter) routeCount() int {
	router.mu.Lock()
	defer router.mu.Unlock()
	return len(router.routes)
}

func (router *sharedRootCompositionCommandRouter) providerCallsFor(factoryDir string) int {
	router.mu.Lock()
	defer router.mu.Unlock()
	factoryDir = filepath.Clean(factoryDir)
	if route := router.routes[factoryDir]; route != nil {
		return route.providerCalls
	}
	return router.history[factoryDir].providerCalls
}

func (router *sharedRootCompositionCommandRouter) Run(
	ctx context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	if err := ctx.Err(); err != nil {
		return platformprocess.CommandResult{}, err
	}
	router.mu.Lock()
	route := router.routeForRequest(request)
	if route != nil {
		route.providerCalls++
	}
	router.mu.Unlock()
	if route == nil {
		return platformprocess.CommandResult{}, fmt.Errorf(
			"no root-composition command route matched request workdir=%q command=%q",
			request.WorkDir, request.Command,
		)
	}
	return platformprocess.CommandResult{
		Stdout: support.CodexSuccessStdout("Done. COMPLETE"),
	}, nil
}

func (router *sharedRootCompositionCommandRouter) routeForRequest(
	request platformprocess.CommandRequest,
) *sharedRootCompositionCommandRoute {
	var best *sharedRootCompositionCommandRoute
	for factoryDir, route := range router.routes {
		if !sharedRootCompositionPathBelongsTo(factoryDir, request.WorkDir) &&
			!sharedRootCompositionPathBelongsTo(factoryDir, request.Command) {
			continue
		}
		if best == nil || len(factoryDir) > len(best.factoryDir) {
			best = route
		}
	}
	return best
}

func sharedRootCompositionPathBelongsTo(factoryDir, candidate string) bool {
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

var _ platformprocess.CommandRunner = (*sharedRootCompositionCommandRouter)(nil)

type sharedRootCompositionSession struct {
	fixture    *sharedRootCompositionProcessFixture
	factoryDir string
	sessionID  string
	durable    bool

	closeOnce sync.Once
}

func openSharedRootCompositionLiveSession(t *testing.T, factoryDir string) *sharedRootCompositionSession {
	t.Helper()
	fixture := sharedRootCompositionFixtureForTest(t)
	if err := fixture.router.register(factoryDir); err != nil {
		t.Fatalf("register root-composition command route: %v", err)
	}
	// This cleanup also covers an open failure before a Session can be tracked.
	t.Cleanup(func() { fixture.router.unregister(factoryDir) })
	opened := support.OpenFactorySessionAt(t, fixture.baseURL, factoryDir)
	sessionID := opened.Session.Id
	if err := fixture.recordSessionOpened(sessionID); err != nil {
		support.CloseFactorySessionAt(t, fixture.baseURL, sessionID)
		t.Fatalf("record root-composition Factory Session %q: %v", sessionID, err)
	}
	session := &sharedRootCompositionSession{
		fixture:    fixture,
		factoryDir: filepath.Clean(factoryDir),
		sessionID:  sessionID,
	}
	t.Cleanup(func() { session.close(t) })
	return session
}

func openSharedRootCompositionJavaScriptSession(
	t *testing.T,
) (*sharedRootCompositionSession, factoryapi.FactorySessionSyncExecutionResponse) {
	t.Helper()
	fixture := sharedRootCompositionFixtureForTest(t)
	if err := fixture.router.register(fixture.jsFactoryDir); err != nil {
		t.Fatalf("register root-composition JavaScript command route: %v", err)
	}
	t.Cleanup(func() { fixture.router.unregister(fixture.jsFactoryDir) })
	response := postFactoryRuntimeJavaScriptSyncExecution(t, fixture.baseURL, map[string]any{
		"input": "structured-sync",
	})
	if strings.TrimSpace(response.SessionId) == "" {
		t.Fatalf("root-composition JavaScript sync response has empty session id: %#v", response)
	}
	if err := fixture.recordSessionOpened(response.SessionId); err != nil {
		t.Fatalf("record root-composition JavaScript Factory Session %q: %v", response.SessionId, err)
	}
	session := &sharedRootCompositionSession{
		fixture:    fixture,
		factoryDir: filepath.Clean(fixture.jsFactoryDir),
		sessionID:  response.SessionId,
		durable:    true,
	}
	t.Cleanup(func() { session.close(t) })
	return session, response
}

func (session *sharedRootCompositionSession) close(t testing.TB) {
	if session == nil {
		return
	}
	session.closeOnce.Do(func() {
		support.TerminateFactorySessionAt(t, session.fixture.baseURL, session.sessionID)
		if !session.durable {
			deleteSharedRootCompositionSessionAfterTerminate(t, session.fixture.baseURL, session.sessionID)
		}
		session.fixture.router.unregister(session.factoryDir)
		session.fixture.recordSessionClosed(session.sessionID)
	})
}

func deleteSharedRootCompositionSessionAfterTerminate(t testing.TB, baseURL, sessionID string) {
	t.Helper()
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID)
	deadline := time.NewTimer(sharedRootCompositionFixtureShutdownTimeout)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		request, err := http.NewRequest(http.MethodDelete, endpoint, nil)
		if err != nil {
			t.Fatalf("build DELETE root-composition Factory Session request: %v", err)
		}
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatalf("DELETE root-composition Factory Session %q: %v", sessionID, err)
		}
		body, readErr := io.ReadAll(response.Body)
		response.Body.Close()
		if readErr != nil {
			t.Fatalf("read DELETE root-composition Factory Session %q response: %v", sessionID, readErr)
		}
		if response.StatusCode == http.StatusNoContent {
			assertSharedRootCompositionSessionAbsent(t, endpoint, sessionID)
			return
		}
		if response.StatusCode != http.StatusConflict || !strings.Contains(string(body), "runtime is") {
			t.Fatalf(
				"DELETE root-composition Factory Session %q status = %d: %s",
				sessionID, response.StatusCode, strings.TrimSpace(string(body)),
			)
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatalf("timed out deleting root-composition Factory Session %q", sessionID)
		}
	}
}

func assertSharedRootCompositionSessionAbsent(t testing.TB, endpoint, sessionID string) {
	t.Helper()
	response, err := http.Get(endpoint)
	if err != nil {
		t.Fatalf("GET deleted root-composition Factory Session %q: %v", sessionID, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf(
			"GET deleted root-composition Factory Session %q status = %d, want 404: %s",
			sessionID,
			response.StatusCode,
			strings.TrimSpace(string(body)),
		)
	}
}

func sharedRootCompositionSessionStatus(
	t testing.TB,
	fixture *sharedRootCompositionProcessFixture,
	sessionID string,
) factoryapi.StatusResponse {
	t.Helper()
	return support.GetJSON[factoryapi.StatusResponse](
		t,
		strings.TrimSuffix(fixture.baseURL, "/")+"/factory-sessions/"+url.PathEscape(sessionID)+"/status",
	)
}

func sharedRootCompositionSessionRead(
	t testing.TB,
	fixture *sharedRootCompositionProcessFixture,
	sessionID string,
) factoryapi.FactorySession {
	t.Helper()
	response := support.GetJSON[factoryapi.FactorySessionGetResponse](
		t,
		strings.TrimSuffix(fixture.baseURL, "/")+"/factory-sessions/"+url.PathEscape(sessionID),
	)
	session, err := response.AsFactorySession()
	if err != nil {
		t.Fatalf("decode root-composition Factory Session %q: %v", sessionID, err)
	}
	return session
}

func sharedRootCompositionSessionWork(
	t testing.TB,
	fixture *sharedRootCompositionProcessFixture,
	sessionID string,
) factoryapi.ListWorkResponse {
	t.Helper()
	return support.GetJSON[factoryapi.ListWorkResponse](
		t,
		strings.TrimSuffix(fixture.baseURL, "/")+"/factory-sessions/"+url.PathEscape(sessionID)+"/work",
	)
}

func sharedRootCompositionEnvironment(homeDir string) []string {
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

func writeSharedRootCompositionBootstrapFactory(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	encoded, err := json.Marshal(factoryRuntimeLifecycleActivationFactoryConfig())
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "factory.json"), encoded, 0o644); err != nil {
		return err
	}
	workerConfig := filepath.Join(dir, "workers", "worker-a", "AGENTS.md")
	if err := os.MkdirAll(filepath.Dir(workerConfig), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(workerConfig, []byte("---\nmodel: test-model\nstopToken: COMPLETE\ntype: MODEL_WORKER\n---\nBootstrap worker.\n"), 0o644); err != nil {
		return err
	}
	workstationConfig := filepath.Join(dir, "workstations", "process", "AGENTS.md")
	if err := os.MkdirAll(filepath.Dir(workstationConfig), 0o755); err != nil {
		return err
	}
	return os.WriteFile(workstationConfig, []byte("---\ntype: MODEL_WORKSTATION\n---\nDo the work.\n"), 0o644)
}

func writeSharedRootCompositionJavaScriptFactory(homeDir string) (string, error) {
	factoryDir := filepath.Join(homeDir, ".you-agent-factory", "factories", factoryRuntimeJavaScriptFactoryName)
	if err := os.MkdirAll(factoryDir, 0o755); err != nil {
		return "", err
	}
	config := map[string]any{
		"name": factoryRuntimeJavaScriptFactoryName,
		"invocationSignature": map[string]any{
			"parameters": []any{map[string]any{
				"name":     "input",
				"required": true,
				"bindings": []any{map[string]any{"kind": "POSITIONAL", "position": 1}},
			}},
		},
		"orchestrator": map[string]any{
			"kind": "JAVASCRIPT",
			"javascript": map[string]any{
				"inlineSource": map[string]any{
					"encoding": "utf-8",
					"inline":   `workflow.final("` + factoryRuntimeJavaScriptSuccessResult + `");`,
				},
				"argsSchema": map[string]any{
					"type":                 "object",
					"properties":           map[string]any{"input": map[string]any{"type": "string"}},
					"additionalProperties": false,
				},
			},
		},
	}
	encoded, err := json.Marshal(config)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(factoryDir, "factory.json"), encoded, 0o644); err != nil {
		return "", err
	}
	return factoryDir, nil
}
