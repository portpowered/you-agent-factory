package guards_batch

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
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const sharedGuardsFixtureShutdownTimeout = 15 * time.Second
const sharedGuardsScenarioConcurrency = 2

var (
	sharedGuardsFixtureOnce  sync.Once
	sharedGuardsFixture      *sharedGuardsProcessFixture
	sharedGuardsFixtureErr   error
	sharedGuardsScenarioSlot = make(chan struct{}, sharedGuardsScenarioConcurrency)
	sharedGuardsScenarioID   atomic.Uint64
)

// TestMain owns the one root-built process and API listener for this package.
// Each behavior case still gets an independent fixture root and public Factory
// Session so mutable guard/resource state cannot cross scenario boundaries.
func TestMain(m *testing.M) {
	code := m.Run()
	if sharedGuardsFixture != nil {
		if err := sharedGuardsFixture.close(); err != nil {
			fmt.Fprintf(os.Stderr, "close shared guards fixture: %v\n", err)
			if code == 0 {
				code = 1
			}
		}
	}
	os.Exit(code)
}

func enterSharedGuardsScenario(t *testing.T) {
	t.Helper()
	t.Parallel()
	sharedGuardsScenarioSlot <- struct{}{}
	t.Cleanup(func() { <-sharedGuardsScenarioSlot })
}

type sharedGuardsProcessFixture struct {
	rootDir      string
	homeDir      string
	bootstrapDir string
	baseURL      string

	process support.ApplicationProcess
	command *sharedGuardsHostedCommand
	api     *support.ProcessAPIServer
	router  *sharedGuardsCommandRouter
	stream  sharedGuardsStreamLifecycle

	processStarts atomic.Int32
	processStops  atomic.Int32
	apiStarts     atomic.Int32
	apiStops      chan struct{}
	apiStopOnce   sync.Once

	processMu  sync.Mutex
	processErr error

	sessionMu        sync.Mutex
	openedSessionIDs map[string]struct{}
	closedSessionIDs map[string]struct{}
}

type sharedGuardsHostedCommand struct {
	cancel context.CancelFunc
	done   chan struct{}

	mu  sync.Mutex
	err error
}

func sharedGuardsProcess(t testing.TB) *sharedGuardsProcessFixture {
	t.Helper()
	sharedGuardsFixtureOnce.Do(func() {
		sharedGuardsFixture, sharedGuardsFixtureErr = newSharedGuardsProcessFixture()
	})
	if sharedGuardsFixtureErr != nil {
		t.Fatalf("start shared guards fixture: %v", sharedGuardsFixtureErr)
	}
	if sharedGuardsFixture == nil {
		t.Fatal("shared guards fixture is unavailable")
	}
	support.WaitForStatus(t, sharedGuardsFixture.baseURL, sharedGuardsFixtureShutdownTimeout, func(status factoryapi.StatusResponse) bool {
		return strings.TrimSpace(status.RuntimeStatus) != ""
	})
	return sharedGuardsFixture
}

func newSharedGuardsProcessFixture() (*sharedGuardsProcessFixture, error) {
	rootDir, err := os.MkdirTemp("", "you-functional-guards-batch-")
	if err != nil {
		return nil, fmt.Errorf("create fixture root: %w", err)
	}
	removeRoot := func() { _ = os.RemoveAll(rootDir) }
	homeDir := filepath.Join(rootDir, "home")
	bootstrapDir := filepath.Join(rootDir, "bootstrap")
	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		removeRoot()
		return nil, fmt.Errorf("create fixture home: %w", err)
	}
	if err := writeSharedGuardsBootstrapFactory(bootstrapDir); err != nil {
		removeRoot()
		return nil, fmt.Errorf("write bootstrap Factory: %w", err)
	}

	api := support.NewProcessAPIServer()
	fixture := &sharedGuardsProcessFixture{
		rootDir:          rootDir,
		homeDir:          homeDir,
		bootstrapDir:     bootstrapDir,
		api:              api,
		apiStops:         make(chan struct{}),
		router:           newSharedGuardsCommandRouter(),
		openedSessionIDs: make(map[string]struct{}),
		closedSessionIDs: make(map[string]struct{}),
	}
	process, err := support.BuildProcessWithContext(context.Background(), serviceedges.Edges{
		APIServerStarter:      fixture.startAPIServer,
		ProviderCommandRunner: fixture.router,
		ScriptCommandRunner:   fixture.router,
	})
	if err != nil {
		removeRoot()
		return nil, fmt.Errorf("build root process: %w", err)
	}
	fixture.process = process

	inputs := support.FakeInputs(context.Background(), []string{
		"you", "run", "--dir", bootstrapDir, "--continuously", "--with-server",
		"--server", "http://127.0.0.1:1", "--quiet", "--no-record",
	})
	inputs.Input.Env = []string{"HOME=" + homeDir, "USERPROFILE=" + homeDir}
	inputs.Input.WorkingDirectory = bootstrapDir
	fixture.processStarts.Add(1)
	fixture.command = startSharedGuardsHostedCommand(process, inputs.Input, func(err error) {
		fixture.processMu.Lock()
		fixture.processErr = err
		fixture.processMu.Unlock()
		fixture.processStops.Add(1)
	})
	baseURL, err := api.WaitForBaseURL(sharedGuardsFixtureShutdownTimeout)
	if err != nil {
		_ = fixture.close()
		removeRoot()
		return nil, fmt.Errorf("wait for loopback API: %w", err)
	}
	fixture.baseURL = baseURL
	return fixture, nil
}

func startSharedGuardsHostedCommand(
	process support.Process,
	input root.Input,
	onDone func(error),
) *sharedGuardsHostedCommand {
	parent := input.Context
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	input.Context = ctx
	command := &sharedGuardsHostedCommand{cancel: cancel, done: make(chan struct{})}
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

func (fixture *sharedGuardsProcessFixture) startAPIServer(
	ctx context.Context,
	request platformhttpserver.StartRequest,
) error {
	fixture.apiStarts.Add(1)
	request.Handler = fixture.stream.wrap(request.Handler)
	err := fixture.api.Start(ctx, request)
	fixture.apiStopOnce.Do(func() { close(fixture.apiStops) })
	return err
}

func (command *sharedGuardsHostedCommand) stop() error {
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
	case <-time.After(sharedGuardsFixtureShutdownTimeout):
		return fmt.Errorf("timed out waiting for shared guards host shutdown")
	}
}

func (fixture *sharedGuardsProcessFixture) close() error {
	if fixture == nil {
		return nil
	}
	var closeErr error
	if fixture.command != nil {
		closeErr = fixture.command.stop()
	}
	if fixture.process != nil {
		ctx, cancel := context.WithTimeout(context.Background(), sharedGuardsFixtureShutdownTimeout)
		closeErr = errors.Join(closeErr, fixture.process.Close(ctx))
		cancel()
	}
	if fixture.baseURL != "" {
		client := http.Client{Timeout: time.Second}
		response, err := client.Get(strings.TrimSuffix(fixture.baseURL, "/") + "/status")
		if err == nil {
			_ = response.Body.Close()
			closeErr = errors.Join(closeErr, errors.New("shared guards API port remains reachable after process close"))
		}
	}
	if fixture.apiStarts.Load() != 0 {
		select {
		case <-fixture.apiStops:
		case <-time.After(sharedGuardsFixtureShutdownTimeout):
			closeErr = errors.Join(closeErr, fmt.Errorf("timed out waiting for shared guards API shutdown"))
		}
	}
	if fixture.apiStarts.Load() != 1 {
		closeErr = errors.Join(closeErr, fmt.Errorf("shared guards API starts = %d, want one", fixture.apiStarts.Load()))
	}
	if fixture.processStarts.Load() != 1 || fixture.processStops.Load() != fixture.processStarts.Load() {
		closeErr = errors.Join(closeErr, fmt.Errorf(
			"shared guards process starts=%d stops=%d, want one balanced lifecycle",
			fixture.processStarts.Load(), fixture.processStops.Load(),
		))
	}
	fixture.processMu.Lock()
	processErr := fixture.processErr
	fixture.processMu.Unlock()
	if processErr != nil && !errors.Is(processErr, context.Canceled) {
		closeErr = errors.Join(closeErr, fmt.Errorf("shared guards Process.Execute: %w", processErr))
	}
	if got := fixture.router.routeCount(); got != 0 {
		closeErr = errors.Join(closeErr, fmt.Errorf("shared guards active command routes = %d", got))
	}
	closeErr = errors.Join(closeErr, fixture.sessionLifecycleError())
	if active := fixture.stream.active.Load(); active != 0 || fixture.stream.opened.Load() != fixture.stream.closed.Load() {
		closeErr = errors.Join(closeErr, fmt.Errorf(
			"shared guards SSE streams active=%d opened=%d closed=%d",
			active, fixture.stream.opened.Load(), fixture.stream.closed.Load(),
		))
	}
	if err := os.RemoveAll(fixture.rootDir); err != nil {
		closeErr = errors.Join(closeErr, fmt.Errorf("remove shared guards fixture root: %w", err))
	} else if _, err := os.Stat(fixture.rootDir); err == nil {
		closeErr = errors.Join(closeErr, fmt.Errorf("shared guards fixture root still exists: %s", fixture.rootDir))
	} else if !errors.Is(err, os.ErrNotExist) {
		closeErr = errors.Join(closeErr, fmt.Errorf("probe removed shared guards fixture root: %w", err))
	}
	fixture.emitRuntimeReport()
	return closeErr
}

func (fixture *sharedGuardsProcessFixture) emitRuntimeReport() {
	fixture.sessionMu.Lock()
	sessions := make([]string, 0, len(fixture.openedSessionIDs))
	for sessionID := range fixture.openedSessionIDs {
		sessions = append(sessions, sessionID)
	}
	closed := len(fixture.closedSessionIDs)
	fixture.sessionMu.Unlock()
	sort.Strings(sessions)
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
		fmt.Fprintf(os.Stderr, "shared guards runtime matrix encode error: %v\n", err)
		return
	}
	fmt.Fprintf(os.Stdout, "GUARDS_BATCH_RUNTIME_MATRIX %s\n", encoded)
}

func (fixture *sharedGuardsProcessFixture) recordSessionOpened(sessionID string) error {
	fixture.sessionMu.Lock()
	defer fixture.sessionMu.Unlock()
	if _, exists := fixture.openedSessionIDs[sessionID]; exists {
		return fmt.Errorf("Factory Session %q was opened twice", sessionID)
	}
	fixture.openedSessionIDs[sessionID] = struct{}{}
	return nil
}

func (fixture *sharedGuardsProcessFixture) recordSessionClosed(sessionID string) {
	fixture.sessionMu.Lock()
	defer fixture.sessionMu.Unlock()
	fixture.closedSessionIDs[sessionID] = struct{}{}
}

func (fixture *sharedGuardsProcessFixture) sessionLifecycleError() error {
	fixture.sessionMu.Lock()
	defer fixture.sessionMu.Unlock()
	if len(fixture.openedSessionIDs) != len(fixture.closedSessionIDs) {
		return fmt.Errorf(
			"shared guards Factory Session lifecycle opened %d sessions but closed %d",
			len(fixture.openedSessionIDs), len(fixture.closedSessionIDs),
		)
	}
	for sessionID := range fixture.openedSessionIDs {
		if _, closed := fixture.closedSessionIDs[sessionID]; !closed {
			return fmt.Errorf("shared guards Factory Session %q was not closed", sessionID)
		}
	}
	return nil
}

type sharedGuardsStreamLifecycle struct {
	active atomic.Int32
	opened atomic.Int32
	closed atomic.Int32
}

func (lifecycle *sharedGuardsStreamLifecycle) wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if !strings.HasSuffix(request.URL.Path, "/events") && !strings.HasSuffix(request.URL.Path, "/response-events") {
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

type sharedGuardsCommandResponder func(context.Context, platformprocess.CommandRequest) (platformprocess.CommandResult, error)

type sharedGuardsRouteConfig struct {
	provider sharedGuardsCommandResponder
	script   sharedGuardsCommandResponder
}

type sharedGuardsCommandResponse struct {
	result         platformprocess.CommandResult
	err            error
	providerOutput string
	shapeOutput    bool
}

func sharedGuardsProviderOutput(content string) sharedGuardsCommandResponse {
	return sharedGuardsCommandResponse{providerOutput: content, shapeOutput: true}
}

func sharedGuardsCommandResult(result platformprocess.CommandResult) sharedGuardsCommandResponse {
	return sharedGuardsCommandResponse{result: result}
}

func sharedGuardsCommandError(err error) sharedGuardsCommandResponse {
	return sharedGuardsCommandResponse{err: err}
}

func sharedGuardsProviderSequence(responses ...sharedGuardsCommandResponse) sharedGuardsCommandResponder {
	var mu sync.Mutex
	next := 0
	return func(ctx context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
		if err := ctx.Err(); err != nil {
			return platformprocess.CommandResult{}, err
		}
		mu.Lock()
		var response sharedGuardsCommandResponse
		if next < len(responses) {
			response = responses[next]
			next++
		} else {
			// Match ProviderCommandRunner's exhausted-sequence behavior. Some
			// retry witnesses intentionally assert the resulting later Work
			// failure rather than hiding an extra attempt.
			response = sharedGuardsCommandResult(platformprocess.CommandResult{Stdout: []byte("default mock response")})
		}
		mu.Unlock()
		result := cloneSharedGuardsCommandResult(response.result)
		if response.shapeOutput {
			result.Stdout = sharedGuardsProviderStdout(request.Command, response.providerOutput)
		}
		return result, response.err
	}
}

type sharedGuardsCommandRoute struct {
	factoryDir        string
	providerCalls     int
	providerRequests  []platformprocess.CommandRequest
	requests          []platformprocess.CommandRequest
	providerResponder sharedGuardsCommandResponder
	scriptResponder   sharedGuardsCommandResponder
}

type sharedGuardsCommandRouter struct {
	mu      sync.Mutex
	routes  map[string]*sharedGuardsCommandRoute
	history map[string]sharedGuardsCommandRoute
}

func newSharedGuardsCommandRouter() *sharedGuardsCommandRouter {
	return &sharedGuardsCommandRouter{
		routes:  make(map[string]*sharedGuardsCommandRoute),
		history: make(map[string]sharedGuardsCommandRoute),
	}
}

func (router *sharedGuardsCommandRouter) register(factoryDir string, config sharedGuardsRouteConfig) error {
	factoryDir = filepath.Clean(factoryDir)
	if factoryDir == "." || strings.TrimSpace(factoryDir) == "" {
		return errors.New("shared guards Factory directory is required")
	}
	router.mu.Lock()
	defer router.mu.Unlock()
	if _, exists := router.routes[factoryDir]; exists {
		return fmt.Errorf("shared guards route for %q is already registered", factoryDir)
	}
	router.routes[factoryDir] = &sharedGuardsCommandRoute{
		factoryDir:        factoryDir,
		providerResponder: config.provider,
		scriptResponder:   config.script,
	}
	return nil
}

func (router *sharedGuardsCommandRouter) unregister(factoryDir string) {
	router.mu.Lock()
	defer router.mu.Unlock()
	factoryDir = filepath.Clean(factoryDir)
	if route := router.routes[factoryDir]; route != nil {
		router.history[factoryDir] = *route
	}
	delete(router.routes, factoryDir)
}

func (router *sharedGuardsCommandRouter) routeCount() int {
	router.mu.Lock()
	defer router.mu.Unlock()
	return len(router.routes)
}

func (router *sharedGuardsCommandRouter) providerCallsFor(factoryDir string) int {
	router.mu.Lock()
	defer router.mu.Unlock()
	factoryDir = filepath.Clean(factoryDir)
	if route := router.routes[factoryDir]; route != nil {
		return route.providerCalls
	}
	return router.history[factoryDir].providerCalls
}

func (router *sharedGuardsCommandRouter) requestsFor(factoryDir string) []platformprocess.CommandRequest {
	router.mu.Lock()
	defer router.mu.Unlock()
	factoryDir = filepath.Clean(factoryDir)
	route := router.routes[factoryDir]
	if route == nil {
		archived := router.history[factoryDir]
		route = &archived
	}
	requests := make([]platformprocess.CommandRequest, len(route.providerRequests))
	for index, request := range route.providerRequests {
		requests[index] = cloneSharedGuardsCommandRequest(request)
	}
	return requests
}

func (router *sharedGuardsCommandRouter) Run(
	ctx context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	if err := ctx.Err(); err != nil {
		return platformprocess.CommandResult{}, err
	}
	router.mu.Lock()
	route := router.routeForRequest(request)
	var responder sharedGuardsCommandResponder
	if route != nil {
		if isSharedGuardsProviderCommand(request.Command) {
			route.providerCalls++
			route.providerRequests = append(route.providerRequests, cloneSharedGuardsCommandRequest(request))
			responder = route.providerResponder
		} else {
			responder = route.scriptResponder
		}
		route.requests = append(route.requests, cloneSharedGuardsCommandRequest(request))
	}
	router.mu.Unlock()
	if route == nil {
		return platformprocess.CommandResult{}, fmt.Errorf(
			"no shared guards command route matched workdir=%q command=%q",
			request.WorkDir, request.Command,
		)
	}
	if responder != nil {
		return responder(ctx, request)
	}
	if isSharedGuardsProviderCommand(request.Command) {
		return platformprocess.CommandResult{
			Stdout: sharedGuardsProviderStdout(request.Command, "Done. <COMPLETE> COMPLETE ACCEPTED"),
		}, nil
	}
	return platformprocess.CommandResult{Stdout: []byte("script-output-ok")}, nil
}

func (router *sharedGuardsCommandRouter) routeForRequest(request platformprocess.CommandRequest) *sharedGuardsCommandRoute {
	var best *sharedGuardsCommandRoute
	for factoryDir, route := range router.routes {
		if !sharedGuardsPathBelongsTo(factoryDir, request.WorkDir) && !sharedGuardsPathBelongsTo(factoryDir, request.Command) {
			continue
		}
		if best == nil || len(factoryDir) > len(best.factoryDir) {
			best = route
		}
	}
	return best
}

func sharedGuardsPathBelongsTo(factoryDir, candidate string) bool {
	candidate = strings.TrimSpace(candidate)
	if candidate == "" || !filepath.IsAbs(candidate) {
		return false
	}
	relative, err := filepath.Rel(filepath.Clean(factoryDir), filepath.Clean(candidate))
	if err != nil {
		return false
	}
	return relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)))
}

func isSharedGuardsProviderCommand(command string) bool {
	switch strings.ToLower(filepath.Base(strings.TrimSpace(command))) {
	case "codex", "claude", "agy":
		return true
	default:
		return false
	}
}

func sharedGuardsProviderStdout(command, result string) []byte {
	if strings.EqualFold(filepath.Base(strings.TrimSpace(command)), "claude") {
		return support.ClaudeSuccessStdout(result)
	}
	return support.CodexSuccessStdout(result)
}

func cloneSharedGuardsCommandRequest(request platformprocess.CommandRequest) platformprocess.CommandRequest {
	request.Args = append([]string(nil), request.Args...)
	request.Stdin = append([]byte(nil), request.Stdin...)
	request.Env = append([]string(nil), request.Env...)
	return request
}

func cloneSharedGuardsCommandResult(result platformprocess.CommandResult) platformprocess.CommandResult {
	result.Stdout = append([]byte(nil), result.Stdout...)
	result.Stderr = append([]byte(nil), result.Stderr...)
	return result
}

type sharedGuardsSession struct {
	fixture    *sharedGuardsProcessFixture
	factoryDir string
	sessionID  string

	closeOnce sync.Once
}

func openSharedGuardsSessionWithRoute(t *testing.T, factoryDir string, config sharedGuardsRouteConfig) *sharedGuardsSession {
	t.Helper()
	fixture := sharedGuardsProcess(t)
	if err := fixture.router.register(factoryDir, config); err != nil {
		t.Fatalf("register shared guards command route: %v", err)
	}
	opened := support.OpenFactorySessionAt(t, fixture.baseURL, factoryDir)
	sessionID := opened.Session.Id
	if err := fixture.recordSessionOpened(sessionID); err != nil {
		support.CloseFactorySessionAt(t, fixture.baseURL, sessionID)
		fixture.router.unregister(factoryDir)
		t.Fatalf("record shared guards Factory Session %q: %v", sessionID, err)
	}
	session := &sharedGuardsSession{
		fixture:    fixture,
		factoryDir: filepath.Clean(factoryDir),
		sessionID:  sessionID,
	}
	t.Cleanup(func() { session.close(t) })
	return session
}

func (session *sharedGuardsSession) close(t testing.TB) {
	if session == nil {
		return
	}
	session.closeOnce.Do(func() {
		support.CloseFactorySessionAt(t, session.fixture.baseURL, session.sessionID)
		session.finishCleanup(t)
	})
}

func (session *sharedGuardsSession) closeAfterTerminal(t testing.TB) {
	t.Helper()
	if session == nil {
		return
	}
	session.closeOnce.Do(func() {
		// A terminal Work projection can coexist with a live hosted runtime.
		// DELETE is retried only across the public runtime-state transition so
		// teardown does not add a fixed wait to every completed scenario.
		support.TerminateFactorySessionAt(t, session.fixture.baseURL, session.sessionID)
		deleteSharedGuardsSessionAfterTerminate(t, session.fixture.baseURL, session.sessionID)
		session.finishCleanup(t)
	})
}

func (session *sharedGuardsSession) finishCleanup(t testing.TB) {
	session.fixture.router.unregister(session.factoryDir)
	session.fixture.recordSessionClosed(session.sessionID)
	if err := os.RemoveAll(session.factoryDir); err != nil {
		t.Errorf("remove shared guards scenario root %q: %v", session.factoryDir, err)
	} else if _, err := os.Stat(session.factoryDir); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("shared guards scenario root %q still exists: %v", session.factoryDir, err)
	}
}

func deleteSharedGuardsSessionAfterTerminate(t testing.TB, baseURL, sessionID string) {
	t.Helper()
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID)
	deadline := time.NewTimer(sharedGuardsFixtureShutdownTimeout)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		request, err := http.NewRequest(http.MethodDelete, endpoint, nil)
		if err != nil {
			t.Fatalf("build DELETE Factory Session request: %v", err)
		}
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatalf("DELETE Factory Session %q: %v", sessionID, err)
		}
		body, readErr := io.ReadAll(response.Body)
		response.Body.Close()
		if readErr != nil {
			t.Fatalf("read DELETE Factory Session %q response: %v", sessionID, readErr)
		}
		if response.StatusCode == http.StatusNoContent {
			return
		}
		if response.StatusCode != http.StatusConflict || !strings.Contains(string(body), "runtime is") {
			t.Fatalf("DELETE Factory Session %q status = %d: %s", sessionID, response.StatusCode, strings.TrimSpace(string(body)))
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatalf("timed out deleting terminated Factory Session %q", sessionID)
		}
	}
}

func runSharedGuardsFactoryToCompletionWithRouteAndWork(
	t *testing.T,
	factoryDir string,
	config sharedGuardsRouteConfig,
	timeout time.Duration,
) (factoryapi.FactorySession, factoryapi.ListWorkResponse) {
	t.Helper()
	session := openSharedGuardsSessionWithRoute(t, factoryDir, config)
	support.WaitForSessionTerminalStatus(t, session.fixture.baseURL, session.sessionID, timeout)
	publicSession := getSharedGuardsSession(t, session.fixture.baseURL, session.sessionID)
	listed := listSharedGuardsSessionWork(t, session.fixture.baseURL, session.sessionID)
	session.closeAfterTerminal(t)
	return publicSession, listed
}

func runSharedGuardsFactoryToCompletionWithRouteAndObservations(
	t *testing.T,
	factoryDir string,
	config sharedGuardsRouteConfig,
	timeout time.Duration,
) (factoryapi.FactorySession, factoryapi.ListWorkResponse, []factoryapi.FactoryEvent) {
	t.Helper()
	session := openSharedGuardsSessionWithRoute(t, factoryDir, config)
	support.WaitForSessionTerminalStatus(t, session.fixture.baseURL, session.sessionID, timeout)
	publicSession := getSharedGuardsSession(t, session.fixture.baseURL, session.sessionID)
	listed := listSharedGuardsSessionWork(t, session.fixture.baseURL, session.sessionID)
	events := support.GetFactoryEventsForSessionAt(t, session.fixture.baseURL, session.sessionID)
	session.closeAfterTerminal(t)
	return publicSession, listed, events
}

func getSharedGuardsSession(t testing.TB, baseURL, sessionID string) factoryapi.FactorySession {
	t.Helper()
	response := support.GetJSON[factoryapi.FactorySessionGetResponse](
		t,
		strings.TrimSuffix(baseURL, "/")+"/factory-sessions/"+url.PathEscape(sessionID),
	)
	session, err := response.AsFactorySession()
	if err != nil {
		t.Fatalf("decode shared guards Factory Session: %v", err)
	}
	return session
}

func listSharedGuardsSessionWork(t testing.TB, baseURL, sessionID string) factoryapi.ListWorkResponse {
	t.Helper()
	return support.GetJSON[factoryapi.ListWorkResponse](
		t,
		strings.TrimSuffix(baseURL, "/")+"/factory-sessions/"+url.PathEscape(sessionID)+"/work",
	)
}

func assertSharedGuardsProviderCalls(t testing.TB, factoryDir string, want int) {
	t.Helper()
	if got := sharedGuardsProcess(t).router.providerCallsFor(factoryDir); got != want {
		t.Errorf("shared guards provider command calls = %d, want %d", got, want)
	}
}

func assertSharedGuardsDispatchTransitions(t testing.TB, events []factoryapi.FactoryEvent, wants ...string) {
	t.Helper()
	observations := support.ObserveDispatchEvents(t, events)
	if len(observations) != len(wants) {
		t.Fatalf("shared guards dispatch count = %d, want %d", len(observations), len(wants))
	}
	for index, want := range wants {
		if got := observations[index].Request.TransitionId; got != want {
			t.Errorf("shared guards dispatch %d transition = %q, want %q", index, got, want)
		}
	}
}

func sharedGuardsProviderRequests(t testing.TB, factoryDir string) []platformprocess.CommandRequest {
	t.Helper()
	return sharedGuardsProcess(t).router.requestsFor(factoryDir)
}

func sharedGuardsScenario(t *testing.T, fixtureName string) string {
	t.Helper()
	fixture := sharedGuardsProcess(t)
	name := fmt.Sprintf("scenario-%03d-%s", sharedGuardsScenarioID.Add(1), strings.NewReplacer("/", "-", "\\", "-", ":", "-").Replace(t.Name()))
	destination := filepath.Join(fixture.rootDir, "scenarios", name)
	if err := copySharedGuardsDirectory(support.LegacyFixtureDir(t, fixtureName), destination); err != nil {
		t.Fatalf("copy shared guards fixture %q: %v", fixtureName, err)
	}
	return destination
}

func copySharedGuardsDirectory(sourceDir, destinationDir string) error {
	return filepath.WalkDir(sourceDir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}
		destination := destinationDir
		if relative != "." {
			destination = filepath.Join(destinationDir, relative)
		}
		if entry.IsDir() {
			return os.MkdirAll(destination, 0o755)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(destination, contents, info.Mode().Perm())
	})
}

func writeSharedGuardsBootstrapFactory(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	factory := map[string]any{
		"name": "shared-guards-batch-bootstrap",
		"workTypes": []map[string]any{{
			"name":   "bootstrap",
			"states": []map[string]string{{"name": "ready", "type": "INITIAL"}},
		}},
		"workers":      []map[string]string{},
		"workstations": []map[string]any{},
	}
	encoded, err := json.Marshal(factory)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "factory.json"), encoded, 0o644); err != nil {
		return err
	}
	return nil
}

var _ platformprocess.CommandRunner = (*sharedGuardsCommandRouter)(nil)
