package routing

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
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const sharedRoutingFixtureShutdownTimeout = 15 * time.Second
const sharedRoutingScenarioConcurrency = 2

var (
	sharedRoutingFixtureOnce  sync.Once
	sharedRoutingFixture      *sharedRoutingProcessFixture
	sharedRoutingFixtureErr   error
	sharedRoutingScenarioSlot = make(chan struct{}, sharedRoutingScenarioConcurrency)
	sharedRoutingRuntime      = newSharedRoutingRuntime()
)

// TestMain owns the one root-built process and loopback API for the package.
// Scenario directories, routes, and Factory Sessions remain case-local.
func TestMain(m *testing.M) {
	code := m.Run()
	if sharedRoutingFixture != nil {
		if err := sharedRoutingFixture.close(); err != nil {
			fmt.Fprintf(os.Stderr, "close shared Petri routing fixture: %v\n", err)
			if code == 0 {
				code = 1
			}
		}
	}
	if err := emitSharedRoutingRuntimeReport(); err != nil {
		fmt.Fprintf(os.Stderr, "emit Petri routing runtime matrix: %v\n", err)
		if code == 0 {
			code = 1
		}
	}
	os.Exit(code)
}

// enterSharedRoutingScenario bounds concurrent use of the shared host while
// retaining parallel subtests so path-keyed routes prove Session isolation.
func enterSharedRoutingScenario(t *testing.T) {
	t.Helper()
	t.Parallel()
	sharedRoutingScenarioSlot <- struct{}{}
	t.Cleanup(func() { <-sharedRoutingScenarioSlot })
}

type sharedRoutingProcessFixture struct {
	rootDir      string
	homeDir      string
	bootstrapDir string
	baseURL      string

	process support.ApplicationProcess
	command *sharedRoutingHostedCommand
	api     *support.ProcessAPIServer
	router  *sharedRoutingCommandRouter

	sessionMu        sync.Mutex
	openedSessionIDs map[string]struct{}
	closedSessionIDs map[string]struct{}
}

type sharedRoutingHostedCommand struct {
	cancel context.CancelFunc
	done   chan struct{}

	mu  sync.Mutex
	err error
}

func sharedRoutingProcess(t testing.TB) *sharedRoutingProcessFixture {
	t.Helper()
	sharedRoutingFixtureOnce.Do(func() {
		sharedRoutingFixture, sharedRoutingFixtureErr = newSharedRoutingProcessFixture(t)
	})
	if sharedRoutingFixtureErr != nil {
		t.Fatalf("start shared Petri routing fixture: %v", sharedRoutingFixtureErr)
	}
	if sharedRoutingFixture == nil {
		t.Fatal("shared Petri routing fixture is unavailable")
	}
	return sharedRoutingFixture
}

func newSharedRoutingProcessFixture(t testing.TB) (*sharedRoutingProcessFixture, error) {
	t.Helper()
	rootDir, err := os.MkdirTemp("", "you-functional-petri-routing-")
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
	if err := writeSharedRoutingBootstrapFactory(bootstrapDir); err != nil {
		cleanupRoot()
		return nil, fmt.Errorf("write bootstrap Factory: %w", err)
	}

	api := support.NewProcessAPIServer()
	router := newSharedRoutingCommandRouter()
	process, err := support.BuildProcessWithContext(context.Background(), serviceedges.Edges{
		APIServerStarter:      api.Start,
		ProviderCommandRunner: router,
		ScriptCommandRunner:   router,
	})
	if err != nil {
		cleanupRoot()
		return nil, fmt.Errorf("build root process: %w", err)
	}
	fixture := &sharedRoutingProcessFixture{
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
		"you", "run", "--dir", bootstrapDir, "--continuously", "--with-server",
		"--server", "http://127.0.0.1:1", "--quiet", "--no-record",
	})
	inputs.Input.Env = []string{"HOME=" + homeDir, "USERPROFILE=" + homeDir}
	inputs.Input.WorkingDirectory = bootstrapDir
	fixture.command = startSharedRoutingHostedCommand(process, inputs.Input)
	if err := sharedRoutingRuntime.processStarted(); err != nil {
		_ = fixture.close()
		cleanupRoot()
		return nil, err
	}
	baseURL, err := api.WaitForBaseURL(sharedRoutingFixtureShutdownTimeout)
	if err != nil {
		_ = fixture.close()
		cleanupRoot()
		return nil, fmt.Errorf("wait for loopback API: %w", err)
	}
	fixture.baseURL = baseURL
	support.WaitForStatus(t, baseURL, sharedRoutingFixtureShutdownTimeout, func(status factoryapi.StatusResponse) bool {
		return strings.TrimSpace(status.RuntimeStatus) != ""
	})
	return fixture, nil
}

func startSharedRoutingHostedCommand(
	process support.Process,
	input root.Input,
) *sharedRoutingHostedCommand {
	parent := input.Context
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	input.Context = ctx
	command := &sharedRoutingHostedCommand{cancel: cancel, done: make(chan struct{})}
	go func() {
		err := process.Execute(input)
		command.mu.Lock()
		command.err = err
		command.mu.Unlock()
		close(command.done)
	}()
	return command
}

func (fixture *sharedRoutingProcessFixture) close() error {
	if fixture == nil {
		return nil
	}
	var closeErr error
	if fixture.command != nil {
		closeErr = fixture.command.stop()
	}
	if fixture.process != nil {
		ctx, cancel := context.WithTimeout(context.Background(), sharedRoutingFixtureShutdownTimeout)
		closeErr = errors.Join(closeErr, fixture.process.Close(ctx))
		cancel()
		closeErr = errors.Join(closeErr, sharedRoutingRuntime.processStopped())
	}
	if fixture.router != nil && fixture.router.routeCount() != 0 {
		closeErr = errors.Join(closeErr, fmt.Errorf(
			"shared Petri routing command routes remaining after cleanup = %d",
			fixture.router.routeCount(),
		))
	}
	closeErr = errors.Join(closeErr, fixture.sessionLifecycleError())
	if err := os.RemoveAll(fixture.rootDir); err != nil {
		closeErr = errors.Join(closeErr, fmt.Errorf("remove fixture root: %w", err))
	} else if _, err := os.Stat(fixture.rootDir); err == nil {
		closeErr = errors.Join(closeErr, fmt.Errorf("fixture root still exists: %s", fixture.rootDir))
	} else if !os.IsNotExist(err) {
		closeErr = errors.Join(closeErr, fmt.Errorf("probe removed fixture root: %w", err))
	}
	return closeErr
}

func (command *sharedRoutingHostedCommand) stop() error {
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
	case <-time.After(sharedRoutingFixtureShutdownTimeout):
		return fmt.Errorf("timed out waiting for shared Petri routing host shutdown")
	}
}

func (fixture *sharedRoutingProcessFixture) sessionLifecycleError() error {
	fixture.sessionMu.Lock()
	defer fixture.sessionMu.Unlock()
	if len(fixture.openedSessionIDs) != len(fixture.closedSessionIDs) {
		return fmt.Errorf(
			"shared Petri routing Factory Session lifecycle opened %d sessions but closed %d",
			len(fixture.openedSessionIDs), len(fixture.closedSessionIDs),
		)
	}
	for sessionID := range fixture.openedSessionIDs {
		if _, closed := fixture.closedSessionIDs[sessionID]; !closed {
			return fmt.Errorf("shared Petri routing Factory Session %q was not closed", sessionID)
		}
	}
	return nil
}

func (fixture *sharedRoutingProcessFixture) recordSessionOpened(sessionID string) error {
	fixture.sessionMu.Lock()
	defer fixture.sessionMu.Unlock()
	if _, exists := fixture.openedSessionIDs[sessionID]; exists {
		return fmt.Errorf("Factory Session %q was opened twice", sessionID)
	}
	fixture.openedSessionIDs[sessionID] = struct{}{}
	return nil
}

func (fixture *sharedRoutingProcessFixture) recordSessionClosed(sessionID string) {
	fixture.sessionMu.Lock()
	defer fixture.sessionMu.Unlock()
	fixture.closedSessionIDs[sessionID] = struct{}{}
}

type sharedRoutingCommandResponder func(
	context.Context,
	platformprocess.CommandRequest,
) (platformprocess.CommandResult, error)

type sharedRoutingRouteConfig struct {
	provider sharedRoutingCommandResponder
	script   sharedRoutingCommandResponder
}

type sharedRoutingCommandRoute struct {
	factoryDir        string
	providerCalls     int
	requests          []platformprocess.CommandRequest
	providerResponder sharedRoutingCommandResponder
	scriptResponder   sharedRoutingCommandResponder
}

type sharedRoutingCommandRouter struct {
	mu      sync.Mutex
	routes  map[string]*sharedRoutingCommandRoute
	history map[string]sharedRoutingCommandRoute
}

func newSharedRoutingCommandRouter() *sharedRoutingCommandRouter {
	return &sharedRoutingCommandRouter{
		routes:  make(map[string]*sharedRoutingCommandRoute),
		history: make(map[string]sharedRoutingCommandRoute),
	}
}

func (router *sharedRoutingCommandRouter) register(
	factoryDir string,
	config sharedRoutingRouteConfig,
) error {
	factoryDir = filepath.Clean(factoryDir)
	if factoryDir == "." || strings.TrimSpace(factoryDir) == "" {
		return errors.New("shared Petri routing Factory directory is required")
	}
	router.mu.Lock()
	defer router.mu.Unlock()
	if _, exists := router.routes[factoryDir]; exists {
		return fmt.Errorf("shared Petri routing route for %q is already registered", factoryDir)
	}
	router.routes[factoryDir] = &sharedRoutingCommandRoute{
		factoryDir:        factoryDir,
		providerResponder: config.provider,
		scriptResponder:   config.script,
	}
	return nil
}

func (router *sharedRoutingCommandRouter) unregister(factoryDir string) {
	router.mu.Lock()
	defer router.mu.Unlock()
	factoryDir = filepath.Clean(factoryDir)
	if route := router.routes[factoryDir]; route != nil {
		router.history[factoryDir] = *route
	}
	delete(router.routes, factoryDir)
}

func (router *sharedRoutingCommandRouter) routeCount() int {
	router.mu.Lock()
	defer router.mu.Unlock()
	return len(router.routes)
}

func (router *sharedRoutingCommandRouter) providerCallsFor(factoryDir string) int {
	router.mu.Lock()
	defer router.mu.Unlock()
	factoryDir = filepath.Clean(factoryDir)
	if route := router.routes[factoryDir]; route != nil {
		return route.providerCalls
	}
	return router.history[factoryDir].providerCalls
}

func (router *sharedRoutingCommandRouter) requestsFor(factoryDir string) []platformprocess.CommandRequest {
	router.mu.Lock()
	defer router.mu.Unlock()
	factoryDir = filepath.Clean(factoryDir)
	route := router.routes[factoryDir]
	if route == nil {
		archived := router.history[factoryDir]
		route = &archived
	}
	requests := make([]platformprocess.CommandRequest, len(route.requests))
	for index, request := range route.requests {
		requests[index] = platformprocess.CommandRequest{
			Command: request.Command,
			Args:    append([]string(nil), request.Args...),
			Stdin:   append([]byte(nil), request.Stdin...),
			Env:     append([]string(nil), request.Env...),
			WorkDir: request.WorkDir,
		}
	}
	return requests
}

func (router *sharedRoutingCommandRouter) Run(
	ctx context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	if err := ctx.Err(); err != nil {
		return platformprocess.CommandResult{}, err
	}
	router.mu.Lock()
	route := router.routeForRequest(request)
	var responder sharedRoutingCommandResponder
	if route != nil {
		if isSharedRoutingProviderCommand(request.Command) {
			route.providerCalls++
			responder = route.providerResponder
		} else {
			responder = route.scriptResponder
		}
		route.requests = append(route.requests, cloneSharedRoutingRequest(request))
	}
	router.mu.Unlock()
	if route == nil {
		return platformprocess.CommandResult{}, errors.New("no shared Petri routing command route matched the request")
	}
	if responder != nil {
		return responder(ctx, request)
	}
	if isSharedRoutingProviderCommand(request.Command) {
		return platformprocess.CommandResult{
			Stdout: sharedRoutingProviderStdout(request.Command, "Done. <COMPLETE> COMPLETE ACCEPTED"),
		}, nil
	}
	return platformprocess.CommandResult{Stdout: []byte("script-output-ok")}, nil
}

func cloneSharedRoutingRequest(request platformprocess.CommandRequest) platformprocess.CommandRequest {
	return platformprocess.CommandRequest{
		Command: request.Command,
		Args:    append([]string(nil), request.Args...),
		Stdin:   append([]byte(nil), request.Stdin...),
		Env:     append([]string(nil), request.Env...),
		WorkDir: request.WorkDir,
	}
}

func (router *sharedRoutingCommandRouter) routeForRequest(
	request platformprocess.CommandRequest,
) *sharedRoutingCommandRoute {
	var best *sharedRoutingCommandRoute
	for factoryDir, route := range router.routes {
		if !sharedRoutingPathBelongsTo(factoryDir, request.WorkDir) &&
			!sharedRoutingPathBelongsTo(factoryDir, request.Command) {
			continue
		}
		if best == nil || len(factoryDir) > len(best.factoryDir) {
			best = route
		}
	}
	return best
}

func sharedRoutingPathBelongsTo(factoryDir, candidate string) bool {
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

func isSharedRoutingProviderCommand(command string) bool {
	switch strings.ToLower(filepath.Base(strings.TrimSpace(command))) {
	case "codex", "claude", "agy":
		return true
	default:
		return false
	}
}

func sharedRoutingProviderStdout(command, result string) []byte {
	if strings.EqualFold(filepath.Base(strings.TrimSpace(command)), "claude") {
		return support.ClaudeSuccessStdout(result)
	}
	return support.CodexSuccessStdout(result)
}

type sharedRoutingCommandResponse struct {
	result         platformprocess.CommandResult
	err            error
	providerOutput string
	shapeOutput    bool
}

func sharedRoutingProviderSequence(
	responses ...sharedRoutingCommandResponse,
) sharedRoutingCommandResponder {
	var mu sync.Mutex
	next := 0
	return func(ctx context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
		if err := ctx.Err(); err != nil {
			return platformprocess.CommandResult{}, err
		}
		mu.Lock()
		if next >= len(responses) {
			mu.Unlock()
			return platformprocess.CommandResult{}, fmt.Errorf("shared Petri routing provider response sequence exhausted for %q", filepath.Base(request.Command))
		}
		response := responses[next]
		next++
		mu.Unlock()
		result := response.result
		result.Stdout = append([]byte(nil), result.Stdout...)
		result.Stderr = append([]byte(nil), result.Stderr...)
		if response.shapeOutput {
			result.Stdout = sharedRoutingProviderStdout(request.Command, response.providerOutput)
		}
		return result, response.err
	}
}

func sharedRoutingProviderOutput(content string) sharedRoutingCommandResponse {
	return sharedRoutingCommandResponse{providerOutput: content, shapeOutput: true}
}

func sharedRoutingCommandResult(result platformprocess.CommandResult) sharedRoutingCommandResponse {
	return sharedRoutingCommandResponse{result: result}
}

func sharedRoutingCommandError(err error) sharedRoutingCommandResponse {
	return sharedRoutingCommandResponse{err: err}
}

type sharedRoutingSession struct {
	fixture    *sharedRoutingProcessFixture
	factoryDir string
	sessionID  string

	closeOnce sync.Once
}

func openSharedRoutingSessionWithRoute(
	t *testing.T,
	factoryDir string,
	config sharedRoutingRouteConfig,
) *sharedRoutingSession {
	t.Helper()
	fixture := sharedRoutingProcess(t)
	if err := fixture.router.register(factoryDir, config); err != nil {
		t.Fatalf("register shared Petri routing command route: %v", err)
	}
	opened := support.OpenFactorySessionAt(t, fixture.baseURL, factoryDir)
	sessionID := opened.Session.Id
	if err := fixture.recordSessionOpened(sessionID); err != nil {
		support.CloseFactorySessionAt(t, fixture.baseURL, sessionID)
		fixture.router.unregister(factoryDir)
		t.Fatalf("record shared Petri routing Factory Session %q: %v", sessionID, err)
	}
	if err := sharedRoutingRuntime.scenarioOpened(t.Name(), factoryDir, sessionID); err != nil {
		support.CloseFactorySessionAt(t, fixture.baseURL, sessionID)
		fixture.router.unregister(factoryDir)
		fixture.recordSessionClosed(sessionID)
		t.Fatalf("record shared Petri routing runtime row: %v", err)
	}
	session := &sharedRoutingSession{
		fixture:    fixture,
		factoryDir: filepath.Clean(factoryDir),
		sessionID:  sessionID,
	}
	t.Cleanup(func() { session.close(t) })
	return session
}

func (session *sharedRoutingSession) close(t testing.TB) {
	if session == nil {
		return
	}
	session.closeOnce.Do(func() {
		support.CloseFactorySessionAt(t, session.fixture.baseURL, session.sessionID)
		session.fixture.router.unregister(session.factoryDir)
		session.fixture.recordSessionClosed(session.sessionID)
		if err := sharedRoutingRuntime.scenarioClosed(session.sessionID); err != nil {
			t.Errorf("close shared Petri routing runtime row: %v", err)
		}
	})
}

func (session *sharedRoutingSession) closeAfterTerminal(t testing.TB) {
	t.Helper()
	if session == nil {
		return
	}
	session.closeOnce.Do(func() {
		// DELETE is the state-driven observation: an active-runtime conflict
		// means the public terminate control is still unwinding asynchronously.
		support.TerminateFactorySessionAt(t, session.fixture.baseURL, session.sessionID)
		deleteSharedRoutingSessionAfterTerminate(t, session.fixture.baseURL, session.sessionID)
		session.fixture.router.unregister(session.factoryDir)
		session.fixture.recordSessionClosed(session.sessionID)
		if err := sharedRoutingRuntime.scenarioClosed(session.sessionID); err != nil {
			t.Errorf("close shared Petri routing runtime row: %v", err)
		}
	})
}

func deleteSharedRoutingSessionAfterTerminate(t testing.TB, baseURL, sessionID string) {
	t.Helper()
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID)
	deadline := time.NewTimer(sharedRoutingFixtureShutdownTimeout)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		request, err := http.NewRequest(http.MethodDelete, endpoint, nil)
		if err != nil {
			t.Fatalf("build delete terminated Factory Session request: %v", err)
		}
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatalf("DELETE terminated Factory Session %q: %v", sessionID, err)
		}
		body, readErr := io.ReadAll(response.Body)
		response.Body.Close()
		if readErr != nil {
			t.Fatalf("read DELETE terminated Factory Session %q response: %v", sessionID, readErr)
		}
		if response.StatusCode == http.StatusNoContent {
			return
		}
		if response.StatusCode != http.StatusConflict || !strings.Contains(string(body), "runtime is") {
			t.Fatalf("DELETE terminated Factory Session %q status = %d: %s", sessionID, response.StatusCode, strings.TrimSpace(string(body)))
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatalf("timed out deleting terminated Factory Session %q", sessionID)
		}
	}
}

func runSharedRoutingFactoryToCompletionWithRouteAndWork(
	t *testing.T,
	factoryDir string,
	config sharedRoutingRouteConfig,
	timeout time.Duration,
) (factoryapi.StatusResponse, factoryapi.ListWorkResponse) {
	t.Helper()
	session := openSharedRoutingSessionWithRoute(t, factoryDir, config)
	status := support.WaitForSessionTerminalStatus(t, session.fixture.baseURL, session.sessionID, timeout)
	listed := listSharedRoutingSessionWork(t, session.fixture.baseURL, session.sessionID)
	session.closeAfterTerminal(t)
	assertSharedRoutingRouteRequests(t, factoryDir)
	return status, listed
}

func runSharedRoutingFactoryToCompletionWithRouteAndObservations(
	t *testing.T,
	factoryDir string,
	config sharedRoutingRouteConfig,
	timeout time.Duration,
) (factoryapi.StatusResponse, factoryapi.ListWorkResponse, []factoryapi.FactoryEvent) {
	t.Helper()
	session := openSharedRoutingSessionWithRoute(t, factoryDir, config)
	status := support.WaitForSessionTerminalStatus(t, session.fixture.baseURL, session.sessionID, timeout)
	listed := listSharedRoutingSessionWork(t, session.fixture.baseURL, session.sessionID)
	events := support.GetFactoryEventsForSessionAt(t, session.fixture.baseURL, session.sessionID)
	session.closeAfterTerminal(t)
	assertSharedRoutingRouteRequests(t, factoryDir)
	return status, listed, events
}

func assertSharedRoutingProviderCalls(t testing.TB, factoryDir string, want int) {
	t.Helper()
	if got := sharedRoutingProcess(t).router.providerCallsFor(factoryDir); got != want {
		t.Errorf("shared Petri routing provider command calls = %d, want %d", got, want)
	}
}

func assertSharedRoutingRouteRequests(t testing.TB, factoryDir string) {
	t.Helper()
	router := sharedRoutingProcess(t).router
	requests := router.requestsFor(factoryDir)
	if len(requests) == 0 {
		t.Errorf("shared Petri routing route %q recorded no command requests", factoryDir)
		return
	}
	for index, request := range requests {
		if sharedRoutingPathBelongsTo(factoryDir, request.WorkDir) ||
			sharedRoutingPathBelongsTo(factoryDir, request.Command) {
			continue
		}
		t.Errorf(
			"shared Petri routing route %q recorded request %d outside its identity (command=%q workdir=%q)",
			factoryDir, index, request.Command, request.WorkDir,
		)
	}
}

func listSharedRoutingSessionWork(
	t testing.TB,
	baseURL string,
	sessionID string,
) factoryapi.ListWorkResponse {
	t.Helper()
	return support.GetJSON[factoryapi.ListWorkResponse](
		t,
		strings.TrimSuffix(baseURL, "/")+"/factory-sessions/"+url.PathEscape(sessionID)+"/work",
	)
}

func writeSharedRoutingBootstrapFactory(dir string) error {
	if err := os.MkdirAll(filepath.Join(dir, "workstations", "process"), 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(dir, "workers", "worker-a"), 0o755); err != nil {
		return err
	}
	factory := map[string]any{
		"name": "shared-petri-routing-bootstrap",
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
	encoded, err := json.Marshal(factory)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "factory.json"), encoded, 0o644); err != nil {
		return err
	}
	return os.WriteFile(
		filepath.Join(dir, "workers", "worker-a", "AGENTS.md"),
		[]byte("---\nmodel: test-model\ntype: MODEL_WORKER\nstopToken: COMPLETE\n---\nBootstrap worker.\n"),
		0o644,
	)
}

var _ platformprocess.CommandRunner = (*sharedRoutingCommandRouter)(nil)

type sharedRoutingRuntimeLedger struct {
	mu       sync.Mutex
	starts   int
	stops    int
	sessions map[string]*sharedRoutingRuntimeSession
}

type sharedRoutingRuntimeSession struct {
	Scenario string `json:"scenario"`
	Factory  string `json:"factoryDir"`
	Session  string `json:"sessionId"`
	Closed   bool   `json:"closed"`
}

type sharedRoutingRuntimeReport struct {
	ProcessStarts int                           `json:"processStarts"`
	ProcessStops  int                           `json:"processStops"`
	Sessions      []sharedRoutingRuntimeSession `json:"sessions"`
}

func newSharedRoutingRuntime() *sharedRoutingRuntimeLedger {
	return &sharedRoutingRuntimeLedger{sessions: make(map[string]*sharedRoutingRuntimeSession)}
}

func (ledger *sharedRoutingRuntimeLedger) processStarted() error {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	if ledger.starts != 0 {
		return fmt.Errorf("shared Petri routing process started more than once")
	}
	ledger.starts++
	return nil
}

func (ledger *sharedRoutingRuntimeLedger) processStopped() error {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	if ledger.starts == 0 || ledger.stops != 0 {
		return fmt.Errorf("shared Petri routing process stopped without exactly one start")
	}
	ledger.stops++
	return nil
}

func (ledger *sharedRoutingRuntimeLedger) scenarioOpened(
	scenario, factoryDir, sessionID string,
) error {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	for _, row := range ledger.sessions {
		if row.Factory == factoryDir || row.Session == sessionID {
			return fmt.Errorf("shared Petri routing scenario identity was reused")
		}
	}
	ledger.sessions[sessionID] = &sharedRoutingRuntimeSession{
		Scenario: scenario,
		Factory:  factoryDir,
		Session:  sessionID,
	}
	return nil
}

func (ledger *sharedRoutingRuntimeLedger) scenarioClosed(sessionID string) error {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	row := ledger.sessions[sessionID]
	if row == nil || row.Closed {
		return fmt.Errorf("shared Petri routing session %q was closed more than once or never opened", sessionID)
	}
	row.Closed = true
	return nil
}

func emitSharedRoutingRuntimeReport() error {
	report, err := sharedRoutingRuntime.report()
	encoded, marshalErr := json.Marshal(report)
	if marshalErr != nil {
		return fmt.Errorf("encode Petri routing runtime matrix: %w", marshalErr)
	}
	fmt.Fprintf(os.Stdout, "PETRI_ROUTING_RUNTIME_MATRIX %s\n", encoded)
	return errors.Join(err, sharedRoutingRuntime.reportRouteInvariant())
}

func (ledger *sharedRoutingRuntimeLedger) report() (sharedRoutingRuntimeReport, error) {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	report := sharedRoutingRuntimeReport{
		ProcessStarts: ledger.starts,
		ProcessStops:  ledger.stops,
		Sessions:      make([]sharedRoutingRuntimeSession, 0, len(ledger.sessions)),
	}
	var reportErr error
	if ledger.starts != ledger.stops {
		reportErr = fmt.Errorf("shared Petri routing process starts=%d stops=%d", ledger.starts, ledger.stops)
	}
	for _, row := range ledger.sessions {
		report.Sessions = append(report.Sessions, *row)
		if !row.Closed {
			reportErr = errors.Join(reportErr, fmt.Errorf("shared Petri routing session %q remained open", row.Session))
		}
	}
	sort.Slice(report.Sessions, func(i, j int) bool {
		if report.Sessions[i].Scenario != report.Sessions[j].Scenario {
			return report.Sessions[i].Scenario < report.Sessions[j].Scenario
		}
		return report.Sessions[i].Factory < report.Sessions[j].Factory
	})
	return report, reportErr
}

func (ledger *sharedRoutingRuntimeLedger) reportRouteInvariant() error {
	if sharedRoutingFixture == nil || sharedRoutingFixture.router == nil {
		return nil
	}
	if got := sharedRoutingFixture.router.routeCount(); got != 0 {
		return fmt.Errorf("shared Petri routing active routes=%d", got)
	}
	return nil
}
