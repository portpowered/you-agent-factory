package customer_lifecycles_test

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// Shared scenarios retain their t.Parallel coverage, but the shared
// application process should not be driven by every scenario at once.
// Bounding actual scenario execution keeps public session and dispatcher load
// controlled under -race without blocking a parent test on its own children.
const sharedPetriScenarioConcurrency = 2

var (
	sharedPetriFixtureOnce  sync.Once
	sharedPetriFixture      *sharedPetriProcessFixture
	sharedPetriFixtureErr   error
	sharedPetriScenarioSlot = make(chan struct{}, sharedPetriScenarioConcurrency)
)

func enterSharedPetriScenario(t *testing.T) {
	t.Helper()
	t.Parallel()
	sharedPetriScenarioSlot <- struct{}{}
	t.Cleanup(func() { <-sharedPetriScenarioSlot })
}

func initializeOrchestrationpetridispatchFixture(t *testing.T) {
	t.Cleanup(func() {
		code := 0
		if sharedPetriFixture != nil {
			if err := sharedPetriFixture.close(); err != nil {
				fmt.Fprintf(os.Stderr, "close shared Petri dispatch fixture: %v\n", err)
				if code == 0 {
					code = 1
				}
			}
		}
		if code != 0 {
			t.Error("customer fixture cleanup failed; see preceding diagnostic")
		}
	})
}

// sharedPetriProcessFixture owns dispatch routes and sessions on the common
// lifecycle host. The parent closes the host after all journey groups finish.
type sharedPetriProcessFixture struct {
	baseURL string

	router *sharedPetriCommandRouter

	sessionMu        sync.Mutex
	openedSessionIDs map[string]struct{}
	closedSessionIDs map[string]struct{}
}

func sharedPetriProcess(t testing.TB) *sharedPetriProcessFixture {
	t.Helper()
	sharedPetriFixtureOnce.Do(func() {
		sharedPetriFixture, sharedPetriFixtureErr = newSharedPetriProcessFixture(t)
	})
	if sharedPetriFixtureErr != nil {
		t.Fatalf("start shared Petri dispatch fixture: %v", sharedPetriFixtureErr)
	}
	if sharedPetriFixture == nil {
		t.Fatal("shared Petri dispatch fixture is unavailable")
	}
	return sharedPetriFixture
}

func newSharedPetriProcessFixture(t testing.TB) (*sharedPetriProcessFixture, error) {
	t.Helper()
	host := orchestrationpetricrossSharedCrossProcess(t)
	return &sharedPetriProcessFixture{
		baseURL:          host.baseURL,
		router:           host.dispatchRouter,
		openedSessionIDs: make(map[string]struct{}),
		closedSessionIDs: make(map[string]struct{}),
	}, nil
}

func (fixture *sharedPetriProcessFixture) close() error {
	if fixture == nil {
		return nil
	}
	var closeErr error
	if fixture.router != nil {
		if got := fixture.router.routeCount(); got != 0 {
			closeErr = errors.Join(
				closeErr,
				fmt.Errorf("shared Petri command routes remaining after cleanup = %d", got),
			)
		}
	}
	lifecycleErr := fixture.sessionLifecycleError()
	return errors.Join(closeErr, lifecycleErr)
}

func (fixture *sharedPetriProcessFixture) sessionLifecycleError() error {
	fixture.sessionMu.Lock()
	defer fixture.sessionMu.Unlock()
	if len(fixture.openedSessionIDs) != len(fixture.closedSessionIDs) {
		return fmt.Errorf(
			"shared Petri Factory Session lifecycle opened %d sessions but closed %d",
			len(fixture.openedSessionIDs),
			len(fixture.closedSessionIDs),
		)
	}
	for sessionID := range fixture.openedSessionIDs {
		if _, closed := fixture.closedSessionIDs[sessionID]; !closed {
			return fmt.Errorf("shared Petri Factory Session %q was not closed", sessionID)
		}
	}
	return nil
}

// sharedPetriCommandRouter is a synchronized, path-keyed edge. Each route is
// registered for a scenario Factory directory before its session opens. A
// request can therefore only consume the route belonging to the same
// scenario, even when multiple sessions dispatch concurrently.
type sharedPetriCommandRouter struct {
	mu      sync.Mutex
	routes  map[string]*sharedPetriCommandRoute
	history map[string]sharedPetriCommandRoute
}

type sharedPetriCommandResponder func(
	context.Context,
	platformprocess.CommandRequest,
) (platformprocess.CommandResult, error)

type sharedPetriRouteConfig struct {
	provider sharedPetriCommandResponder
	script   sharedPetriCommandResponder
}

type sharedPetriCommandRoute struct {
	factoryDir        string
	calls             int
	providerCalls     int
	scriptCalls       int
	requests          []platformprocess.CommandRequest
	providerResponder sharedPetriCommandResponder
	scriptResponder   sharedPetriCommandResponder
}

type sharedPetriCommandResponse struct {
	result              platformprocess.CommandResult
	err                 error
	providerOutput      string
	shapeProviderOutput bool
}

func newSharedPetriCommandRouter() *sharedPetriCommandRouter {
	return &sharedPetriCommandRouter{
		routes:  make(map[string]*sharedPetriCommandRoute),
		history: make(map[string]sharedPetriCommandRoute),
	}
}

func (router *sharedPetriCommandRouter) register(
	factoryDir string,
	configs ...sharedPetriRouteConfig,
) error {
	factoryDir = filepath.Clean(factoryDir)
	if factoryDir == "." || strings.TrimSpace(factoryDir) == "" {
		return errors.New("shared Petri route Factory directory is required")
	}
	if len(configs) > 1 {
		return errors.New("shared Petri route accepts at most one configuration")
	}
	var config sharedPetriRouteConfig
	if len(configs) == 1 {
		config = configs[0]
	}
	router.mu.Lock()
	defer router.mu.Unlock()
	if _, exists := router.routes[factoryDir]; exists {
		return fmt.Errorf("shared Petri route for %q is already registered", factoryDir)
	}
	router.routes[factoryDir] = &sharedPetriCommandRoute{
		factoryDir:        factoryDir,
		providerResponder: config.provider,
		scriptResponder:   config.script,
	}
	return nil
}

func (router *sharedPetriCommandRouter) unregister(factoryDir string) {
	router.mu.Lock()
	factoryDir = filepath.Clean(factoryDir)
	if route := router.routes[factoryDir]; route != nil {
		router.history[factoryDir] = *route
	}
	delete(router.routes, factoryDir)
	router.mu.Unlock()
}

func (router *sharedPetriCommandRouter) routeCount() int {
	router.mu.Lock()
	defer router.mu.Unlock()
	return len(router.routes)
}

func (router *sharedPetriCommandRouter) callsFor(factoryDir string) int {
	router.mu.Lock()
	defer router.mu.Unlock()
	route := router.routes[filepath.Clean(factoryDir)]
	if route == nil {
		return router.history[filepath.Clean(factoryDir)].calls
	}
	return route.calls
}

func (router *sharedPetriCommandRouter) providerCallsFor(factoryDir string) int {
	router.mu.Lock()
	defer router.mu.Unlock()
	if route := router.routes[filepath.Clean(factoryDir)]; route != nil {
		return route.providerCalls
	}
	return router.history[filepath.Clean(factoryDir)].providerCalls
}

func (router *sharedPetriCommandRouter) requestsFor(factoryDir string) []platformprocess.CommandRequest {
	router.mu.Lock()
	defer router.mu.Unlock()
	route := router.routes[filepath.Clean(factoryDir)]
	if route == nil {
		archived := router.history[filepath.Clean(factoryDir)]
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

func (router *sharedPetriCommandRouter) Run(
	ctx context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	if err := ctx.Err(); err != nil {
		return platformprocess.CommandResult{}, err
	}
	router.mu.Lock()
	route := router.routeForRequest(request)
	var responder sharedPetriCommandResponder
	if route != nil {
		route.calls++
		if isSharedPetriProviderCommand(request.Command) {
			route.providerCalls++
			responder = route.providerResponder
		} else {
			route.scriptCalls++
			responder = route.scriptResponder
		}
		route.requests = append(route.requests, platformprocess.CommandRequest{
			Command: request.Command,
			Args:    append([]string(nil), request.Args...),
			Stdin:   append([]byte(nil), request.Stdin...),
			Env:     append([]string(nil), request.Env...),
			WorkDir: request.WorkDir,
		})
	}
	router.mu.Unlock()
	if route == nil {
		return platformprocess.CommandResult{}, fmt.Errorf("no shared Petri command route matched the request")
	}
	if responder != nil {
		return responder(ctx, request)
	}
	if isSharedPetriProviderCommand(request.Command) {
		return platformprocess.CommandResult{
			Stdout: sharedPetriProviderStdout(request.Command, "Done. <COMPLETE> COMPLETE ACCEPTED"),
		}, nil
	}
	return platformprocess.CommandResult{Stdout: []byte("script-output-ok")}, nil
}

func sharedPetriProviderSequence(
	responses ...sharedPetriCommandResponse,
) sharedPetriCommandResponder {
	return sharedPetriCommandSequence(true, responses...)
}

func sharedPetriScriptSequence(
	responses ...sharedPetriCommandResponse,
) sharedPetriCommandResponder {
	return sharedPetriCommandSequence(false, responses...)
}

func sharedPetriCommandSequence(
	providerOutput bool,
	responses ...sharedPetriCommandResponse,
) sharedPetriCommandResponder {
	var mu sync.Mutex
	next := 0
	return func(ctx context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
		if err := ctx.Err(); err != nil {
			return platformprocess.CommandResult{}, err
		}
		mu.Lock()
		if next >= len(responses) {
			mu.Unlock()
			return platformprocess.CommandResult{}, fmt.Errorf(
				"shared Petri %s response sequence exhausted for %q",
				sharedPetriCommandKind(providerOutput),
				filepath.Base(request.Command),
			)
		}
		response := responses[next]
		next++
		mu.Unlock()

		result := cloneSharedPetriCommandResult(response.result)
		if providerOutput && response.shapeProviderOutput {
			result.Stdout = sharedPetriProviderStdout(request.Command, response.providerOutput)
		}
		return result, response.err
	}
}

func sharedPetriFixedCommandResult(result platformprocess.CommandResult) sharedPetriCommandResponder {
	return func(ctx context.Context, _ platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
		if err := ctx.Err(); err != nil {
			return platformprocess.CommandResult{}, err
		}
		return cloneSharedPetriCommandResult(result), nil
	}
}

func sharedPetriProviderOutput(content string) sharedPetriCommandResponse {
	return sharedPetriCommandResponse{
		providerOutput:      content,
		shapeProviderOutput: true,
	}
}

func sharedPetriCommandResult(result platformprocess.CommandResult) sharedPetriCommandResponse {
	return sharedPetriCommandResponse{result: result}
}

func sharedPetriCommandError(err error) sharedPetriCommandResponse {
	return sharedPetriCommandResponse{err: err}
}

func sharedPetriCommandKind(provider bool) string {
	if provider {
		return "provider"
	}
	return "script"
}

func cloneSharedPetriCommandResult(result platformprocess.CommandResult) platformprocess.CommandResult {
	result.Stdout = append([]byte(nil), result.Stdout...)
	result.Stderr = append([]byte(nil), result.Stderr...)
	return result
}

func sharedPetriProviderStdout(command, result string) []byte {
	switch strings.ToLower(filepath.Base(strings.TrimSpace(command))) {
	case "claude":
		return support.ClaudeSuccessStdout(result)
	default:
		return support.CodexSuccessStdout(result)
	}
}

func (router *sharedPetriCommandRouter) routeForRequest(
	request platformprocess.CommandRequest,
) *sharedPetriCommandRoute {
	var best *sharedPetriCommandRoute
	for factoryDir, route := range router.routes {
		if !sharedPetriPathBelongsTo(factoryDir, request.WorkDir) &&
			!sharedPetriPathBelongsTo(factoryDir, request.Command) {
			continue
		}
		if best == nil || len(factoryDir) > len(best.factoryDir) {
			best = route
		}
	}
	return best
}

func sharedPetriPathBelongsTo(factoryDir, candidate string) bool {
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

func isSharedPetriProviderCommand(command string) bool {
	switch strings.ToLower(filepath.Base(strings.TrimSpace(command))) {
	case "codex", "claude", "agy":
		return true
	default:
		return false
	}
}

type sharedPetriSession struct {
	fixture    *sharedPetriProcessFixture
	factoryDir string
	sessionID  string

	closeOnce sync.Once
}

func openSharedPetriSession(t *testing.T, fixtureDir string) *sharedPetriSession {
	return openSharedPetriSessionWithRoute(t, fixtureDir, sharedPetriRouteConfig{})
}

func openSharedPetriSessionWithRoute(
	t *testing.T,
	fixtureDir string,
	config sharedPetriRouteConfig,
) *sharedPetriSession {
	t.Helper()
	fixture := sharedPetriProcess(t)
	if err := fixture.router.register(fixtureDir, config); err != nil {
		t.Fatalf("register shared Petri command route: %v", err)
	}
	opened := support.OpenFactorySessionAt(t, fixture.baseURL, fixtureDir)
	sessionID := opened.Session.Id
	if err := fixture.recordSessionOpened(sessionID); err != nil {
		support.CloseFactorySessionAt(t, fixture.baseURL, sessionID)
		fixture.router.unregister(fixtureDir)
		t.Fatalf("record shared Petri Factory Session %q: %v", sessionID, err)
	}
	session := &sharedPetriSession{
		fixture:    fixture,
		factoryDir: filepath.Clean(fixtureDir),
		sessionID:  sessionID,
	}
	t.Cleanup(func() { session.close(t) })
	return session
}

func assertSharedPetriCommandCalls(t *testing.T, dir string, want int) {
	t.Helper()
	if calls := sharedPetriProcess(t).router.callsFor(dir); calls != want {
		t.Errorf("shared command call count = %d, want %d", calls, want)
	}
}

func assertSharedPetriProviderCalls(t *testing.T, dir string, want int) {
	t.Helper()
	router := sharedPetriProcess(t).router
	if calls := router.providerCallsFor(dir); calls != want {
		var commands []string
		for _, request := range router.requestsFor(dir) {
			commands = append(commands, filepath.Base(request.Command))
		}
		t.Errorf("shared provider command call count = %d, want %d (commands=%v)", calls, want, commands)
	}
}

func sharedPetriProviderRequest(t testing.TB, dir string) platformprocess.CommandRequest {
	t.Helper()
	for _, request := range sharedPetriProcess(t).router.requestsFor(dir) {
		if isSharedPetriProviderCommand(request.Command) {
			return request
		}
	}
	t.Fatalf("shared Petri route for %q recorded no provider command", dir)
	return platformprocess.CommandRequest{}
}

func (session *sharedPetriSession) close(t testing.TB) {
	if session == nil {
		return
	}
	session.closeOnce.Do(func() {
		support.CloseFactorySessionAt(t, session.fixture.baseURL, session.sessionID)
		session.fixture.router.unregister(session.factoryDir)
		session.fixture.recordSessionClosed(session.sessionID)
	})
}

func (fixture *sharedPetriProcessFixture) recordSessionOpened(sessionID string) error {
	fixture.sessionMu.Lock()
	defer fixture.sessionMu.Unlock()
	if _, exists := fixture.openedSessionIDs[sessionID]; exists {
		return fmt.Errorf("Factory Session was opened twice")
	}
	fixture.openedSessionIDs[sessionID] = struct{}{}
	return nil
}

func (fixture *sharedPetriProcessFixture) recordSessionClosed(sessionID string) {
	fixture.sessionMu.Lock()
	defer fixture.sessionMu.Unlock()
	fixture.closedSessionIDs[sessionID] = struct{}{}
}

func runSharedPetriFactoryToCompletionWithEdgesAndWork(
	t *testing.T,
	dir string,
	_ serviceedges.Edges,
	timeout time.Duration,
) (factoryapi.StatusResponse, factoryapi.ListWorkResponse) {
	t.Helper()
	selection := openSharedPetriSession(t, dir)
	status := support.WaitForSessionTerminalStatus(t, selection.fixture.baseURL, selection.sessionID, timeout)
	listed := listSharedPetriSessionWork(t, selection.fixture.baseURL, selection.sessionID)
	selection.close(t)
	assertSharedPetriRouteRequests(t, dir)
	return status, listed
}

func runSharedPetriFactoryToCompletionWithEdgesAndListedWork(
	t *testing.T,
	dir string,
	_ serviceedges.Edges,
	timeout time.Duration,
) factoryapi.ListWorkResponse {
	t.Helper()
	selection := openSharedPetriSession(t, dir)
	support.WaitForSessionTerminalStatus(t, selection.fixture.baseURL, selection.sessionID, timeout)
	listed := listSharedPetriSessionWork(t, selection.fixture.baseURL, selection.sessionID)
	selection.close(t)
	assertSharedPetriRouteRequests(t, dir)
	return listed
}

func runSharedPetriFactoryToCompletionWithEdgesAndObservations(
	t *testing.T,
	dir string,
	_ serviceedges.Edges,
	timeout time.Duration,
) (factoryapi.StatusResponse, factoryapi.ListWorkResponse, []factoryapi.FactoryEvent) {
	return runSharedPetriFactoryToCompletionWithRouteAndObservations(
		t,
		dir,
		sharedPetriRouteConfig{},
		timeout,
	)
}

func runSharedPetriFactoryToCompletionWithRouteAndObservations(
	t *testing.T,
	dir string,
	config sharedPetriRouteConfig,
	timeout time.Duration,
) (factoryapi.StatusResponse, factoryapi.ListWorkResponse, []factoryapi.FactoryEvent) {
	t.Helper()
	selection := openSharedPetriSessionWithRoute(t, dir, config)
	status := support.WaitForSessionTerminalStatus(t, selection.fixture.baseURL, selection.sessionID, timeout)
	listed := listSharedPetriSessionWork(t, selection.fixture.baseURL, selection.sessionID)
	events := support.GetFactoryEventsForSessionAt(t, selection.fixture.baseURL, selection.sessionID)
	selection.close(t)
	assertSharedPetriRouteRequests(t, dir)
	return status, listed, events
}

func runSharedPetriFactoryToCompletionWithRouteAndWork(
	t *testing.T,
	dir string,
	config sharedPetriRouteConfig,
	timeout time.Duration,
) (factoryapi.StatusResponse, factoryapi.ListWorkResponse) {
	t.Helper()
	selection := openSharedPetriSessionWithRoute(t, dir, config)
	status := support.WaitForSessionTerminalStatus(t, selection.fixture.baseURL, selection.sessionID, timeout)
	listed := listSharedPetriSessionWork(t, selection.fixture.baseURL, selection.sessionID)
	selection.close(t)
	assertSharedPetriRouteRequests(t, dir)
	return status, listed
}

func runSharedPetriFactoryToCompletionWithRouteAndListedWork(
	t *testing.T,
	dir string,
	config sharedPetriRouteConfig,
	timeout time.Duration,
) factoryapi.ListWorkResponse {
	t.Helper()
	selection := openSharedPetriSessionWithRoute(t, dir, config)
	support.WaitForSessionTerminalStatus(t, selection.fixture.baseURL, selection.sessionID, timeout)
	listed := listSharedPetriSessionWork(t, selection.fixture.baseURL, selection.sessionID)
	selection.close(t)
	assertSharedPetriRouteRequests(t, dir)
	return listed
}

func assertSharedPetriRouteRequests(t testing.TB, dir string) {
	t.Helper()
	router := sharedPetriProcess(t).router
	for index, request := range router.requestsFor(dir) {
		if sharedPetriPathBelongsTo(dir, request.WorkDir) || sharedPetriPathBelongsTo(dir, request.Command) {
			continue
		}
		t.Errorf(
			"shared Petri route %q recorded request %d outside its identity (command=%q workdir=%q)",
			dir,
			index,
			request.Command,
			request.WorkDir,
		)
	}
}

func listSharedPetriSessionWork(t testing.TB, baseURL, sessionID string) factoryapi.ListWorkResponse {
	t.Helper()
	return support.GetJSON[factoryapi.ListWorkResponse](
		t,
		strings.TrimSuffix(baseURL, "/")+"/factory-sessions/"+url.PathEscape(sessionID)+"/work",
	)
}

var _ platformprocess.CommandRunner = (*sharedPetriCommandRouter)(nil)

func resetorchestrationpetridispatch5State() {

	sharedPetriFixtureOnce = sync.Once{}

	sharedPetriFixture = nil

	sharedPetriFixtureErr = nil

	sharedPetriScenarioSlot = make(chan struct{}, sharedPetriScenarioConcurrency)

}
