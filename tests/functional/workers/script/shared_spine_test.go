package script_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
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
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const scriptSharedSpineTimeout = 15 * time.Second

// TestScriptWorkerSharedSuccessSpine proves the first shared-process slice.
// Each child owns a separate Factory directory and explicit Factory Session,
// while all children use the same immutable root-built application and script
// command edge.
func TestScriptWorkerSharedSuccessSpine(t *testing.T) {
	fixture := newScriptSharedSpineFixture(t)

	for _, scenario := range fixture.scenarios {
		scenario := scenario
		t.Run(scenario.name, func(t *testing.T) {
			t.Parallel()
			fixture.runScenario(t, scenario)
		})
	}
}

func newScriptSharedSpineFixture(t *testing.T) *scriptSharedSpineFixture {
	t.Helper()

	hostDir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "script_executor_dir"))
	scenarios := newScriptSharedSpineScenarios(t)
	routes := make([]scriptCommandRoute, 0, len(scenarios))
	for _, scenario := range scenarios {
		routes = append(routes, scriptCommandRoute{
			selector: scenario.factoryDir,
			runner:   scenario.runner,
		})
	}
	router, err := newScriptCommandRouter(routes)
	if err != nil {
		t.Fatalf("newScriptCommandRouter: %v", err)
	}

	homeDir := t.TempDir()
	api := newScriptSharedHTTPServer()
	identities := &scriptSharedIdentityGenerator{}
	var processBuilds atomic.Int32
	process, err := support.BuildProcessWithContext(context.Background(), serviceedges.Edges{
		APIServerStarter:                         api.start,
		ScriptCommandRunner:                      router,
		FactorySessionIDGenerator:                identities.nextSessionID,
		FactorySessionRuntimeInstanceIDGenerator: identities.nextRuntimeID,
		FactorySessionResponseEventIDGenerator:   identities.nextResponseEventID,
		WorkRequestIDGenerator:                   identities.nextWorkRequestID,
	})
	if err != nil {
		t.Fatalf("BuildProcess: %v", err)
	}
	processBuilds.Add(1)

	fixture := &scriptSharedSpineFixture{
		process:       process,
		commandRouter: router,
		api:           api,
		identities:    identities,
		processBuilds: &processBuilds,
		hostDir:       hostDir,
		homeDir:       homeDir,
		scenarios:     scenarios,
		opened:        make(map[string]struct{}),
		closed:        make(map[string]struct{}),
		observations:  make(map[string]scriptSharedObservation, len(scenarios)),
	}
	t.Cleanup(func() { fixture.close(t) })

	inputs := support.FakeInputs(context.Background(), []string{
		"you", "run",
		"--dir", hostDir,
		"--continuously",
		"--with-server",
		"--server", "http://127.0.0.1:1",
		"--quiet",
		"--no-record",
	})
	inputs.Input.Env = sharedScriptProcessEnvironment(homeDir)
	inputs.Input.WorkingDirectory = hostDir
	fixture.command = support.StartProcessCommand(t, process, inputs.Input)
	fixture.baseURL = api.server.WaitForURL(t)
	assertScriptSharedDefaultSession(t, fixture.baseURL)
	return fixture
}

type scriptSharedSpineFixture struct {
	process       support.ApplicationProcess
	command       *support.ProcessCommand
	commandRouter *scriptCommandRouter
	api           *scriptSharedHTTPServer
	identities    *scriptSharedIdentityGenerator
	processBuilds *atomic.Int32
	baseURL       string
	hostDir       string
	homeDir       string
	scenarios     []scriptSharedScenario

	sessionMu    sync.Mutex
	opened       map[string]struct{}
	closed       map[string]struct{}
	observMu     sync.Mutex
	observations map[string]scriptSharedObservation
	closeOnce    sync.Once
}

type scriptSharedScenario struct {
	name           string
	factoryDir     string
	workName       string
	traceID        string
	expectedOutput string
	noInference    bool
	runner         *support.RecordingCommandRunner
}

type scriptSharedObservation struct {
	sessionID string
	workID    string
	requestID string
	traceID   string
}

func newScriptSharedSpineScenarios(t *testing.T) []scriptSharedScenario {
	t.Helper()

	cases := []struct {
		name           string
		workName       string
		traceID        string
		expectedOutput string
		noInference    bool
	}{
		{
			name:           "PrimaryResult",
			workName:       "shared-script-primary-result",
			traceID:        "shared-script-primary-trace",
			expectedOutput: "shared-script-primary-output",
		},
		{
			name:           "NoInferenceEvents",
			workName:       "shared-script-no-inference",
			traceID:        "shared-script-no-inference-trace",
			expectedOutput: "shared-script-no-inference-output",
			noInference:    true,
		},
		{
			name:           "EdgeAlignment",
			workName:       "shared-script-edge-alignment",
			traceID:        "shared-script-edge-alignment-trace",
			expectedOutput: "shared-script-edge-alignment-output",
		},
	}

	scenarios := make([]scriptSharedScenario, 0, len(cases))
	for _, testCase := range cases {
		scenarios = append(scenarios, scriptSharedScenario{
			name:           testCase.name,
			factoryDir:     testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "script_executor_dir")),
			workName:       testCase.workName,
			traceID:        testCase.traceID,
			expectedOutput: testCase.expectedOutput,
			noInference:    testCase.noInference,
			runner:         support.NewRecordingCommandRunner(testCase.expectedOutput),
		})
	}
	return scenarios
}

func (fixture *scriptSharedSpineFixture) runScenario(
	t *testing.T,
	scenario scriptSharedScenario,
) {
	t.Helper()

	sessionID := fixture.openSession(t, scenario.factoryDir)
	name := scenario.workName
	traceID := scenario.traceID
	submitted := support.SubmitSessionWorkAt(t, fixture.baseURL, sessionID, factoryapi.SubmitWorkRequest{
		Name:         &name,
		TraceId:      &traceID,
		WorkTypeName: "task",
		Payload:      map[string]string{"input": scenario.workName},
	})
	if submitted.SessionId == nil || *submitted.SessionId != sessionID {
		t.Fatalf("submitted Work session id = %#v, want %q", submitted.SessionId, sessionID)
	}
	workID := support.StringPointerValue(submitted.WorkId)
	if workID == "" || strings.TrimSpace(submitted.RequestId) == "" {
		t.Fatalf("submitted Work identity = work:%q request:%q, want both identities", workID, submitted.RequestId)
	}

	support.WaitForSessionTerminalStatus(t, fixture.baseURL, sessionID, scriptSharedSpineTimeout)
	listed := listScriptSessionWork(t, fixture.baseURL, sessionID)
	events := support.GetFactoryEventsForSessionAt(t, fixture.baseURL, sessionID)
	assertScriptSharedWork(t, scenario, submitted, listed)
	assertScriptSharedEvents(t, scenario, sessionID, submitted, events)
	assertScriptSharedCommand(t, fixture.commandRouter, scenario)

	fixture.recordObservation(scenario.name, scriptSharedObservation{
		sessionID: sessionID,
		workID:    workID,
		requestID: submitted.RequestId,
		traceID:   submitted.TraceId,
	})
	fixture.closeSession(t, sessionID)
	assertScriptSessionDeleted(t, fixture.baseURL, sessionID)
}

func (fixture *scriptSharedSpineFixture) openSession(t *testing.T, factoryDir string) string {
	t.Helper()
	opened := support.OpenFactorySessionAt(t, fixture.baseURL, factoryDir)
	if opened.Session == nil || strings.TrimSpace(opened.Session.Id) == "" {
		t.Fatalf("opened shared Factory Session for %q = %#v, want identity", factoryDir, opened)
	}
	sessionID := opened.Session.Id
	if sessionID == factorysessions.DefaultSessionID {
		t.Fatalf("opened shared Factory Session for %q = %q, want explicit session", factoryDir, sessionID)
	}
	fixture.sessionMu.Lock()
	if _, exists := fixture.opened[sessionID]; exists {
		fixture.sessionMu.Unlock()
		t.Fatalf("shared Factory Session id %q was reused", sessionID)
	}
	fixture.opened[sessionID] = struct{}{}
	fixture.sessionMu.Unlock()
	t.Cleanup(func() { fixture.closeSession(t, sessionID) })
	return sessionID
}

func (fixture *scriptSharedSpineFixture) closeSession(t testing.TB, sessionID string) {
	t.Helper()
	fixture.sessionMu.Lock()
	if _, exists := fixture.closed[sessionID]; exists {
		fixture.sessionMu.Unlock()
		return
	}
	fixture.sessionMu.Unlock()

	support.CloseFactorySessionAt(t, fixture.baseURL, sessionID)
	fixture.sessionMu.Lock()
	fixture.closed[sessionID] = struct{}{}
	fixture.sessionMu.Unlock()
}

func (fixture *scriptSharedSpineFixture) recordObservation(
	name string,
	observation scriptSharedObservation,
) {
	fixture.observMu.Lock()
	defer fixture.observMu.Unlock()
	fixture.observations[name] = observation
}

func (fixture *scriptSharedSpineFixture) close(t testing.TB) {
	t.Helper()
	fixture.closeOnce.Do(func() {
		if fixture.command != nil {
			fixture.command.Stop(t)
		}
		closeCtx, cancel := context.WithTimeout(context.Background(), scriptSharedSpineTimeout)
		defer cancel()
		if err := fixture.process.Close(closeCtx); err != nil {
			t.Errorf("close shared script application process: %v", err)
		}
		if err := fixture.api.waitClosed(closeCtx); err != nil {
			t.Errorf("wait for shared script API shutdown: %v", err)
		}
		fixture.assertTopology(t)
		assertScriptSharedListenerClosed(t, fixture.baseURL)
		removeScriptSharedPath(t, fixture.hostDir)
		removeScriptSharedPath(t, fixture.homeDir)
		for _, scenario := range fixture.scenarios {
			removeScriptSharedPath(t, scenario.factoryDir)
		}
	})
}

func (fixture *scriptSharedSpineFixture) assertTopology(t testing.TB) {
	t.Helper()
	if got := fixture.processBuilds.Load(); got != 1 {
		t.Errorf("root application builds = %d, want exactly one", got)
	}
	if got := fixture.api.starts.Load(); got != 1 {
		t.Errorf("shared API server starts = %d, want exactly one", got)
	}

	fixture.sessionMu.Lock()
	opened := len(fixture.opened)
	closed := len(fixture.closed)
	openedIDs := make([]string, 0, opened)
	for sessionID := range fixture.opened {
		openedIDs = append(openedIDs, sessionID)
	}
	fixture.sessionMu.Unlock()
	if opened != closed {
		t.Errorf("shared Factory Session lifecycle = opened:%d closed:%d, want equal", opened, closed)
	}
	assertUniqueScriptIDs(t, openedIDs, "Factory Session")

	fixture.observMu.Lock()
	observations := make([]scriptSharedObservation, 0, len(fixture.observations))
	for _, observation := range fixture.observations {
		observations = append(observations, observation)
	}
	fixture.observMu.Unlock()
	assertUniqueScriptObservations(t, observations)
	if got := fixture.commandRouter.routeCount(); got != len(fixture.scenarios) {
		t.Errorf("shared script route count = %d, want %d immutable routes", got, len(fixture.scenarios))
	}
	if got := fixture.commandRouter.callCount(); got != len(observations) {
		t.Errorf("shared script routed calls = %d, want %d observed scenarios", got, len(observations))
	}
}

func assertUniqueScriptIDs(t testing.TB, ids []string, label string) {
	t.Helper()
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if _, exists := seen[id]; exists {
			t.Errorf("%s id %q was reused", label, id)
		}
		seen[id] = struct{}{}
	}
}

func assertUniqueScriptObservations(t testing.TB, observations []scriptSharedObservation) {
	t.Helper()
	seenWorks := make(map[string]struct{}, len(observations))
	seenRequests := make(map[string]struct{}, len(observations))
	seenTraces := make(map[string]struct{}, len(observations))
	for _, observation := range observations {
		for label, value := range map[string]string{
			"Work": observation.workID, "request": observation.requestID, "trace": observation.traceID,
		} {
			if strings.TrimSpace(value) == "" {
				t.Errorf("shared %s identity is empty", label)
			}
		}
		assertUniqueScriptValue(t, seenWorks, observation.workID, "Work")
		assertUniqueScriptValue(t, seenRequests, observation.requestID, "request")
		assertUniqueScriptValue(t, seenTraces, observation.traceID, "trace")
	}
}

func assertUniqueScriptValue(t testing.TB, seen map[string]struct{}, value, label string) {
	t.Helper()
	if _, exists := seen[value]; exists {
		t.Errorf("shared %s identity %q was reused", label, value)
	}
	seen[value] = struct{}{}
}

func assertScriptSharedDefaultSession(t testing.TB, baseURL string) {
	t.Helper()
	session := support.GetDefaultSession(t, baseURL)
	if !session.IsDefault || strings.TrimSpace(session.Id) == "" {
		t.Fatalf("shared default Factory Session = %#v, want a live default session", session)
	}
}

func assertScriptSharedWork(
	t *testing.T,
	scenario scriptSharedScenario,
	submitted factoryapi.SubmitWorkResponse,
	listed factoryapi.ListWorkResponse,
) {
	t.Helper()
	assertSessionPlaces(t, listed, map[string]int{
		"task:done": 1, "task:init": 0, "task:failed": 0,
	})
	workID := support.StringPointerValue(submitted.WorkId)
	found := 0
	for _, item := range listed.Results {
		if support.StringPointerValue(item.WorkId) != workID {
			continue
		}
		found++
		if support.StringPointerValue(item.RequestId) != submitted.RequestId {
			t.Errorf("%s Work request id = %q, want %q", scenario.name, support.StringPointerValue(item.RequestId), submitted.RequestId)
		}
		if support.StringPointerValue(item.TraceId) != submitted.TraceId {
			t.Errorf("%s Work trace id = %q, want %q", scenario.name, support.StringPointerValue(item.TraceId), submitted.TraceId)
		}
	}
	if found != 1 {
		t.Fatalf("%s Work identity count = %d, want exactly one %q", scenario.name, found, workID)
	}
}

func assertScriptSharedEvents(
	t *testing.T,
	scenario scriptSharedScenario,
	sessionID string,
	submitted factoryapi.SubmitWorkResponse,
	events []factoryapi.FactoryEvent,
) {
	t.Helper()
	if len(events) == 0 {
		t.Fatalf("%s Factory Event history is empty", scenario.name)
	}
	for _, event := range events {
		if event.Context.SessionId != nil && *event.Context.SessionId != sessionID {
			t.Errorf("%s event %q session id = %q, want %q", scenario.name, event.Id, *event.Context.SessionId, sessionID)
		}
		if event.Context.RequestId != nil && *event.Context.RequestId != submitted.RequestId {
			t.Errorf("%s event %q request id = %q, want %q", scenario.name, event.Id, *event.Context.RequestId, submitted.RequestId)
		}
		if event.Context.WorkIds != nil {
			for _, workID := range *event.Context.WorkIds {
				if workID != support.StringPointerValue(submitted.WorkId) {
					t.Errorf("%s event %q Work id = %q, want %q", scenario.name, event.Id, workID, support.StringPointerValue(submitted.WorkId))
				}
			}
		}
	}
	dispatches := support.ObserveDispatchEvents(t, events)
	if len(dispatches) != 1 || dispatches[0].Response == nil {
		t.Fatalf("%s dispatch observations = %#v, want one completed dispatch", scenario.name, dispatches)
	}
	if dispatches[0].Response.Outcome != factoryapi.WorkOutcomeAccepted {
		t.Fatalf("%s dispatch outcome = %q, want ACCEPTED", scenario.name, dispatches[0].Response.Outcome)
	}
	if !support.DispatchObservationIncludesWork(dispatches[0], support.StringPointerValue(submitted.WorkId)) {
		t.Fatalf("%s dispatch omitted Work %q", scenario.name, support.StringPointerValue(submitted.WorkId))
	}
	assertDispatchOutput(t, events, scenario.expectedOutput)
	if scenario.noInference && (hasFactoryEventType(events, factoryapi.FactoryEventTypeInferenceRequest) || hasFactoryEventType(events, factoryapi.FactoryEventTypeInferenceResponse)) {
		t.Fatalf("%s emitted inference events: %v", scenario.name, factoryEventTypes(events))
	}
}

func assertScriptSharedCommand(
	t *testing.T,
	router *scriptCommandRouter,
	scenario scriptSharedScenario,
) {
	t.Helper()
	requests := scenario.runner.Requests()
	if len(requests) != 1 {
		t.Fatalf("%s script command calls = %d, want exactly one", scenario.name, len(requests))
	}
	request := requests[0]
	if request.Command != "echo" {
		t.Fatalf("%s script command = %q, want authored command echo", scenario.name, request.Command)
	}
	if strings.TrimSpace(request.WorkDir) == "" {
		t.Fatalf("%s script command WorkDir is empty", scenario.name)
	}
	assertCommandArgs(t, request, []string{"default-output"})
	if cleanScriptRouteSelector(request.WorkDir) != cleanScriptRouteSelector(scenario.factoryDir) {
		t.Fatalf("%s script WorkDir = %q, want scenario Factory directory %q", scenario.name, request.WorkDir, scenario.factoryDir)
	}
	if calls := router.callsFor(scenario.factoryDir); len(calls) != 1 {
		t.Fatalf("%s immutable route calls = %d, want exactly one", scenario.name, len(calls))
	}
}

func listScriptSessionWork(t testing.TB, baseURL, sessionID string) factoryapi.ListWorkResponse {
	t.Helper()
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID) + "/work"
	return support.GetJSON[factoryapi.ListWorkResponse](t, endpoint)
}

func assertScriptSessionDeleted(t testing.TB, baseURL, sessionID string) {
	t.Helper()
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID)
	response, err := http.Get(endpoint)
	if err != nil {
		t.Fatalf("GET deleted shared Factory Session %q: %v", sessionID, err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("GET deleted shared Factory Session %q status = %d, want 404: %s", sessionID, response.StatusCode, strings.TrimSpace(string(body)))
	}
}

func assertScriptSharedListenerClosed(t testing.TB, baseURL string) {
	t.Helper()
	client := &http.Client{Timeout: 250 * time.Millisecond}
	defer client.CloseIdleConnections()
	response, err := client.Get(strings.TrimSuffix(baseURL, "/") + "/status")
	if err != nil {
		return
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	t.Errorf("shared script listener remains reachable after cleanup: status=%d body=%q", response.StatusCode, strings.TrimSpace(string(body)))
}

func removeScriptSharedPath(t testing.TB, path string) {
	t.Helper()
	if strings.TrimSpace(path) == "" {
		return
	}
	if err := os.RemoveAll(path); err != nil {
		t.Errorf("remove shared script path %q: %v", path, err)
		return
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("shared script path %q remains after cleanup; stat error: %v", path, err)
	}
}

type scriptSharedHTTPServer struct {
	server  *support.ProcessAPIServer
	starts  atomic.Int32
	stopped chan struct{}
	once    sync.Once
}

func newScriptSharedHTTPServer() *scriptSharedHTTPServer {
	return &scriptSharedHTTPServer{
		server:  support.NewProcessAPIServer(),
		stopped: make(chan struct{}),
	}
}

func (server *scriptSharedHTTPServer) start(
	ctx context.Context,
	request platformhttpserver.StartRequest,
) error {
	server.starts.Add(1)
	defer server.once.Do(func() { close(server.stopped) })
	return server.server.Start(ctx, request)
}

func (server *scriptSharedHTTPServer) waitClosed(ctx context.Context) error {
	select {
	case <-server.stopped:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

type scriptSharedIdentityGenerator struct {
	sessions      atomic.Uint64
	runtimes      atomic.Uint64
	responseEvent atomic.Uint64
	workRequests  atomic.Uint64
}

func (generator *scriptSharedIdentityGenerator) nextSessionID() string {
	return fmt.Sprintf("00000000-0000-4000-8000-%012x", generator.sessions.Add(1))
}

func (generator *scriptSharedIdentityGenerator) nextRuntimeID() string {
	return fmt.Sprintf("c05-script-runtime-%d", generator.runtimes.Add(1))
}

func (generator *scriptSharedIdentityGenerator) nextResponseEventID() string {
	return fmt.Sprintf("c05-script-response-event-%d", generator.responseEvent.Add(1))
}

func (generator *scriptSharedIdentityGenerator) nextWorkRequestID() string {
	return fmt.Sprintf("c05-script-request-%d", generator.workRequests.Add(1))
}

func sharedScriptProcessEnvironment(homeDir string) []string {
	environment := append([]string(nil), os.Environ()...)
	return replaceScriptEnvironment(replaceScriptEnvironment(environment, "HOME", homeDir), "USERPROFILE", homeDir)
}

func replaceScriptEnvironment(environment []string, name, value string) []string {
	prefix := name + "="
	result := make([]string, 0, len(environment)+1)
	for _, entry := range environment {
		if strings.EqualFold(strings.SplitN(entry, "=", 2)[0]+"=", prefix) {
			continue
		}
		result = append(result, entry)
	}
	return append(result, name+"="+value)
}

type scriptCommandRoute struct {
	selector string
	runner   platformprocess.CommandRunner
}

type scriptCommandRouter struct {
	routes map[string]scriptCommandRoute

	mu    sync.Mutex
	calls []scriptRoutedCommand
}

type scriptRoutedCommand struct {
	selector string
	request  platformprocess.CommandRequest
}

// newScriptCommandRouter freezes every route before the root process is built.
// The map is read-only during execution; only the diagnostic call ledger is
// synchronized for concurrent scenario observations.
func newScriptCommandRouter(routes []scriptCommandRoute) (*scriptCommandRouter, error) {
	indexed := make(map[string]scriptCommandRoute, len(routes))
	for _, route := range routes {
		selector, err := normalizeScriptRouteSelector(route.selector)
		if err != nil {
			return nil, err
		}
		if route.runner == nil {
			return nil, fmt.Errorf("script route %q has no command runner", selector)
		}
		if _, exists := indexed[selector]; exists {
			return nil, fmt.Errorf("duplicate script route selector %q", scriptSelectorContext(route.selector))
		}
		route.selector = selector
		indexed[selector] = route
	}
	return &scriptCommandRouter{routes: indexed}, nil
}

func (router *scriptCommandRouter) Run(
	ctx context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	selector, err := normalizeScriptRouteSelector(request.WorkDir)
	if err != nil {
		return platformprocess.CommandResult{}, fmt.Errorf("script route selector %q is invalid", scriptSelectorContext(request.WorkDir))
	}
	route, ok := router.routes[selector]
	if !ok {
		return platformprocess.CommandResult{}, fmt.Errorf("unknown script route selector %q", scriptSelectorContext(request.WorkDir))
	}
	if err := ctx.Err(); err != nil {
		return platformprocess.CommandResult{}, err
	}
	router.mu.Lock()
	router.calls = append(router.calls, scriptRoutedCommand{
		selector: selector,
		request:  cloneScriptCommandRequest(request),
	})
	router.mu.Unlock()
	return route.runner.Run(ctx, request)
}

func (router *scriptCommandRouter) callsFor(selector string) []scriptRoutedCommand {
	cleaned, err := normalizeScriptRouteSelector(selector)
	if err != nil {
		return nil
	}
	router.mu.Lock()
	defer router.mu.Unlock()
	calls := make([]scriptRoutedCommand, 0)
	for _, call := range router.calls {
		if call.selector != cleaned {
			continue
		}
		call.request = cloneScriptCommandRequest(call.request)
		calls = append(calls, call)
	}
	return calls
}

func (router *scriptCommandRouter) callCount() int {
	router.mu.Lock()
	defer router.mu.Unlock()
	return len(router.calls)
}

func (router *scriptCommandRouter) routeCount() int {
	return len(router.routes)
}

func normalizeScriptRouteSelector(path string) (string, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return "", fmt.Errorf("script route selector is required")
	}
	abs, err := filepath.Abs(trimmed)
	if err != nil {
		return "", fmt.Errorf("normalize script route selector: %w", err)
	}
	cleaned := filepath.Clean(abs)
	if runtime.GOOS == "windows" {
		cleaned = strings.ToLower(cleaned)
	}
	return cleaned, nil
}

func cleanScriptRouteSelector(path string) string {
	cleaned, _ := normalizeScriptRouteSelector(path)
	return cleaned
}

func scriptSelectorContext(path string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return "<empty>"
	}
	base := filepath.Base(filepath.Clean(trimmed))
	if base == "." || base == string(filepath.Separator) || base == "\\" {
		return "<root>"
	}
	return base
}

func cloneScriptCommandRequest(request platformprocess.CommandRequest) platformprocess.CommandRequest {
	request.Args = append([]string(nil), request.Args...)
	request.Stdin = append([]byte(nil), request.Stdin...)
	request.Env = append([]string(nil), request.Env...)
	return request
}

func TestScriptCommandRouterRejectsUnknownAndDuplicateSelectors(t *testing.T) {
	firstSelector := t.TempDir()
	runner := support.NewRecordingCommandRunner("must-not-run")
	router, err := newScriptCommandRouter([]scriptCommandRoute{{
		selector: firstSelector,
		runner:   runner,
	}})
	if err != nil {
		t.Fatalf("newScriptCommandRouter: %v", err)
	}

	if _, err := newScriptCommandRouter([]scriptCommandRoute{
		{selector: firstSelector, runner: runner},
		{selector: firstSelector, runner: runner},
	}); err == nil {
		t.Fatal("duplicate script selector was accepted")
	}

	secret := "script-router-secret"
	unknown := filepath.Join(t.TempDir(), "unknown-selector")
	_, err = router.Run(context.Background(), platformprocess.CommandRequest{
		Command: "echo",
		Args:    []string{secret},
		Env:     []string{"ROUTER_SECRET=" + secret},
		WorkDir: unknown,
	})
	if err == nil {
		t.Fatal("unknown script selector was accepted")
	}
	if !strings.Contains(err.Error(), "unknown-selector") {
		t.Fatalf("unknown selector error = %v, want sanitized selector context", err)
	}
	if strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), filepath.Dir(unknown)) {
		t.Fatalf("unknown selector error leaked request or path context: %v", err)
	}
	if got := runner.CallCount(); got != 0 {
		t.Fatalf("runner calls after unknown selector = %d, want zero", got)
	}
	if got := router.callCount(); got != 0 {
		t.Fatalf("router calls after unknown selector = %d, want zero", got)
	}
}

var _ platformprocess.CommandRunner = (*scriptCommandRouter)(nil)
