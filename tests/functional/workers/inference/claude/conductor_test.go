package claude_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
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
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	workservice "github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	claudeConductorModel          = "claude-sonnet-4-5-20250514"
	claudeConductorRunTimeout     = 20 * time.Second
	claudeConductorProcessCommand = "claude"
	claudeCancellationMessage     = "provider invocation was canceled"
)

// TestClaudeDefaultLaneSharedProcess proves the ordinary Claude success and
// cancellation scenarios through one root-built process. Each subtest owns a
// separate Factory directory and opens an explicit non-default Factory Session
// so the process is shared while runtime state remains session-scoped.
func TestClaudeDefaultLaneSharedProcess(t *testing.T) {
	fixture := newClaudeDefaultLaneFixture(t)
	t.Cleanup(func() {
		fixture.assertSharedIdentityLedger(t)
	})

	for _, scenario := range fixture.scenarios {
		scenario := scenario
		t.Run(scenario.name, func(t *testing.T) {
			t.Parallel()
			fixture.runScenario(t, scenario)
		})
	}
}

// TestClaudeCommandRouterFailsClosed proves that the package-local command
// edge cannot silently fall back to another scenario when its immutable
// selector is absent or duplicated.
func TestClaudeCommandRouterFailsClosed(t *testing.T) {
	first := &claudeScenarioCommandRunner{}
	second := &claudeScenarioCommandRunner{}
	duplicate, err := newClaudeCommandRouter([]claudeCommandRoute{
		{selector: "duplicate-selector", runner: first},
		{selector: "duplicate-selector", runner: second},
	})
	if err == nil || !strings.Contains(err.Error(), "duplicate Claude scenario selector") {
		t.Fatalf("duplicate route construction error = %v, want fail-closed duplicate selector error", err)
	}
	if duplicate != nil {
		t.Fatal("duplicate route construction returned a usable router")
	}

	router, err := newClaudeCommandRouter([]claudeCommandRoute{
		{selector: "known-selector", runner: first},
	})
	if err != nil {
		t.Fatalf("newClaudeCommandRouter: %v", err)
	}
	_, err = router.Run(context.Background(), platformprocess.CommandRequest{
		Command: claudeConductorProcessCommand,
		WorkDir: "unknown-selector",
	})
	if err == nil || !strings.Contains(err.Error(), "unknown Claude scenario selector") {
		t.Fatalf("unknown route error = %v, want fail-closed selector error", err)
	}
	if got := first.CallCount(); got != 0 {
		t.Fatalf("known route calls after unknown selector = %d, want 0", got)
	}
}

type claudeDefaultLaneFixture struct {
	process    support.ApplicationProcess
	api        *support.ProcessAPIServer
	baseURL    string
	router     *claudeCommandRouter
	identities *claudeIdentityGenerator
	apiStarts  *atomic.Int32
	scenarios  []claudeScenario

	ledgerMu sync.Mutex
	ledger   map[string]claudeScenarioObservation
}

type claudeScenario struct {
	name              string
	factoryDir        string
	workID            string
	requestID         string
	traceID           string
	providerSessionID string
	runner            *claudeScenarioCommandRunner
	wantWorkState     string
	wantOutcome       factoryapi.WorkOutcome
}

type claudeScenarioObservation struct {
	sessionID         string
	workID            string
	requestID         string
	dispatchID        string
	providerSessionID string
}

func newClaudeDefaultLaneFixture(t *testing.T) *claudeDefaultLaneFixture {
	t.Helper()

	identities := &claudeIdentityGenerator{}
	fixtures := []struct {
		name              string
		requestID         string
		workID            string
		traceID           string
		providerSessionID string
		result            platformprocess.CommandResult
		runErr            error
		wantWorkState     string
		wantOutcome       factoryapi.WorkOutcome
	}{
		{
			name:              "Success",
			requestID:         "claude-c03-success-request",
			workID:            "claude-c03-success-work",
			traceID:           "claude-c03-success-trace",
			providerSessionID: "claude-c03-success-provider-session",
			result: platformprocess.CommandResult{Stdout: []byte(
				`{"type":"result","subtype":"success","is_error":false,"result":"claude functional answer COMPLETE","session_id":"claude-c03-success-provider-session"}` + "\n",
			)},
			wantWorkState: "task:done",
			wantOutcome:   factoryapi.WorkOutcomeAccepted,
		},
		{
			name:          "Cancellation",
			requestID:     "claude-c03-cancellation-request",
			workID:        "claude-c03-cancellation-work",
			traceID:       "claude-c03-cancellation-trace",
			runErr:        context.Canceled,
			wantWorkState: "task:failed",
			wantOutcome:   factoryapi.WorkOutcomeFailed,
		},
	}

	hostDir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	support.WriteAgentConfig(t, hostDir, "worker", support.BuildModelWorkerConfig(
		modelprovider.ProviderClaude,
		claudeConductorModel,
	))

	routes := make([]claudeCommandRoute, 0, len(fixtures))
	scenarios := make([]claudeScenario, 0, len(fixtures))
	for _, fixture := range fixtures {
		dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
		support.WriteAgentConfig(t, dir, "worker", support.BuildModelWorkerConfig(
			modelprovider.ProviderClaude,
			claudeConductorModel,
		))
		testutil.WriteSeedRequest(t, dir, workservice.SubmitRequest{
			RequestID:  fixture.requestID,
			WorkID:     fixture.workID,
			Name:       fixture.workID,
			WorkTypeID: "task",
			TraceID:    fixture.traceID,
			Payload:    []byte(`{"title":"claude default lane"}`),
		})

		runner := &claudeScenarioCommandRunner{
			result: fixture.result,
			err:    fixture.runErr,
		}
		scenario := claudeScenario{
			name:              fixture.name,
			factoryDir:        dir,
			workID:            fixture.workID,
			requestID:         fixture.requestID,
			traceID:           fixture.traceID,
			providerSessionID: fixture.providerSessionID,
			runner:            runner,
			wantWorkState:     fixture.wantWorkState,
			wantOutcome:       fixture.wantOutcome,
		}
		routes = append(routes, claudeCommandRoute{
			selector: dir,
			label:    fixture.name,
			runner:   runner,
		})
		scenarios = append(scenarios, scenario)
	}

	router, err := newClaudeCommandRouter(routes)
	if err != nil {
		t.Fatalf("newClaudeCommandRouter: %v", err)
	}

	api := support.NewProcessAPIServer()
	var apiStarts atomic.Int32
	edges := serviceedges.Edges{
		ProviderCommandRunner: router,
		APIServerStarter: func(ctx context.Context, request platformhttpserver.StartRequest) error {
			apiStarts.Add(1)
			return api.Start(ctx, request)
		},
		FactorySessionIDGenerator:                identities.nextSessionID,
		FactorySessionRuntimeInstanceIDGenerator: identities.nextRuntimeID,
		FactorySessionResponseEventIDGenerator:   identities.nextResponseEventID,
	}
	process := support.BuildProcess(t, edges)
	support.CleanupProcess(t, process)

	inputs := support.FakeInputs(t.Context(), []string{
		"you", "run",
		"--dir", hostDir,
		"--continuously",
		"--with-server",
		"--server", "http://127.0.0.1:1",
		"--quiet",
		"--no-record",
	})
	homeDir := t.TempDir()
	inputs.Input.Env = append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
	inputs.Input.WorkingDirectory = hostDir
	t.Cleanup(func() {
		if !t.Failed() {
			return
		}
		if stderr := strings.TrimSpace(inputs.Stderr()); stderr != "" {
			t.Logf("daemon stderr:\n%s", stderr)
		}
		if stdout := strings.TrimSpace(inputs.Stdout()); stdout != "" {
			t.Logf("daemon stdout:\n%s", stdout)
		}
	})
	support.StartProcessCommand(t, process, inputs.Input)
	baseURL := api.WaitForURL(t)
	// The host's default session is only the server anchor. The two scenarios
	// below always use the explicitly opened sessions returned by the API.
	defaultSession := support.GetDefaultSession(t, baseURL)
	if !defaultSession.IsDefault || strings.TrimSpace(defaultSession.Id) == "" {
		t.Fatalf("host default session = %#v, want a live default session with a runtime identity", defaultSession)
	}

	return &claudeDefaultLaneFixture{
		process:    process,
		api:        api,
		baseURL:    baseURL,
		router:     router,
		identities: identities,
		apiStarts:  &apiStarts,
		scenarios:  scenarios,
		ledger:     make(map[string]claudeScenarioObservation, len(scenarios)),
	}
}

func (fixture *claudeDefaultLaneFixture) runScenario(t *testing.T, scenario claudeScenario) {
	t.Helper()

	opened := support.OpenFactorySessionAt(t, fixture.baseURL, scenario.factoryDir)
	if opened.Session == nil {
		t.Fatalf("%s open response missing session: %#v", scenario.name, opened)
	}
	sessionID := opened.Session.Id
	if sessionID == "" || sessionID == factorysessions.DefaultSessionID {
		t.Fatalf("%s session id = %q, want unique non-default explicit session", scenario.name, sessionID)
	}
	closed := false
	t.Cleanup(func() {
		if closed {
			return
		}
		support.CloseFactorySessionAt(t, fixture.baseURL, sessionID)
	})

	support.WaitForSessionTerminalStatus(t, fixture.baseURL, sessionID, claudeConductorRunTimeout)
	session := getClaudeSession(t, fixture.baseURL, sessionID)
	listed := listClaudeSessionWork(t, fixture.baseURL, sessionID)
	events := support.GetFactoryEventsForSessionAt(t, fixture.baseURL, sessionID)

	assertClaudeWork(t, scenario, listed)
	dispatchID := assertClaudeDispatch(t, scenario, sessionID, events)
	assertClaudeCommand(t, fixture.router, scenario)
	providerSessionID := assertClaudeProviderSession(t, scenario, events)
	assertClaudeEventScope(t, scenario, sessionID, events)

	support.CloseFactorySessionAt(t, fixture.baseURL, sessionID)
	closed = true
	assertClaudeSessionDeleted(t, fixture.baseURL, sessionID)
	fixture.recordObservation(claudeScenarioObservation{
		sessionID:         session.Id,
		workID:            scenario.workID,
		requestID:         scenario.requestID,
		dispatchID:        dispatchID,
		providerSessionID: providerSessionID,
	})
}

func (fixture *claudeDefaultLaneFixture) recordObservation(observation claudeScenarioObservation) {
	fixture.ledgerMu.Lock()
	defer fixture.ledgerMu.Unlock()
	fixture.ledger[observation.requestID] = observation
}

func (fixture *claudeDefaultLaneFixture) assertSharedIdentityLedger(t *testing.T) {
	t.Helper()

	fixture.ledgerMu.Lock()
	observations := make([]claudeScenarioObservation, 0, len(fixture.ledger))
	for _, observation := range fixture.ledger {
		observations = append(observations, observation)
	}
	fixture.ledgerMu.Unlock()
	if len(observations) != len(fixture.scenarios) {
		t.Fatalf("shared-process scenario observations = %d, want %d", len(observations), len(fixture.scenarios))
	}

	seenSessions := make(map[string]string, len(observations))
	seenWorks := make(map[string]string, len(observations))
	seenRequests := make(map[string]string, len(observations))
	seenDispatches := make(map[string]string, len(observations))
	seenProviderSessions := make(map[string]string, len(observations))
	for _, observation := range observations {
		assertClaudeUniqueIdentity(t, seenSessions, observation.sessionID, observation.requestID, "Factory Session")
		assertClaudeUniqueIdentity(t, seenWorks, observation.workID, observation.requestID, "Work")
		assertClaudeUniqueIdentity(t, seenRequests, observation.requestID, observation.requestID, "request")
		assertClaudeUniqueIdentity(t, seenDispatches, observation.dispatchID, observation.requestID, "dispatch")
		if observation.providerSessionID != "" {
			assertClaudeUniqueIdentity(t, seenProviderSessions, observation.providerSessionID, observation.requestID, "Provider Session")
		}
	}
	if got := fixture.identities.sessionCount(); got < uint64(len(fixture.scenarios)) {
		t.Fatalf("Factory Session IDs generated = %d, want at least %d explicit sessions", got, len(fixture.scenarios))
	}
	if got := fixture.apiStarts.Load(); got != 1 {
		t.Fatalf("API server starts = %d, want exactly one shared process server", got)
	}
	if got := fixture.router.callCount(); got != len(fixture.scenarios) {
		t.Fatalf("shared process routed provider calls = %d, want %d", got, len(fixture.scenarios))
	}
}

func assertClaudeUniqueIdentity(t *testing.T, seen map[string]string, value, owner, kind string) {
	t.Helper()
	if strings.TrimSpace(value) == "" {
		t.Fatalf("%s identity for %s is empty", kind, owner)
	}
	if previous, ok := seen[value]; ok {
		t.Fatalf("%s identity %q is shared by %s and %s", kind, value, previous, owner)
	}
	seen[value] = owner
}

func assertClaudeWork(t *testing.T, scenario claudeScenario, listed factoryapi.ListWorkResponse) {
	t.Helper()
	if got := support.CountWorkAtCustomerState(listed, scenario.wantWorkState); got != 1 {
		t.Fatalf("%s terminal Work state count = %d, want 1; listed=%#v", scenario.name, got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:done"); scenario.wantWorkState == "task:failed" && got != 0 {
		t.Fatalf("%s completed Work count = %d, want 0", scenario.name, got)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:failed"); scenario.wantWorkState == "task:done" && got != 0 {
		t.Fatalf("%s failed Work count = %d, want 0", scenario.name, got)
	}

	var found int
	for _, item := range listed.Results {
		if support.StringPointerValue(item.WorkId) != scenario.workID {
			continue
		}
		found++
		if support.StringPointerValue(item.RequestId) != scenario.requestID {
			t.Fatalf("%s Work request id = %q, want %q", scenario.name, support.StringPointerValue(item.RequestId), scenario.requestID)
		}
		if scenario.wantWorkState == "task:failed" {
			if item.FailureDetail == nil || !strings.Contains(item.FailureDetail.Message, claudeCancellationMessage) {
				t.Fatalf("%s Work failure detail = %#v, want canonical cancellation message", scenario.name, item.FailureDetail)
			}
		}
	}
	if found != 1 {
		t.Fatalf("%s Work identity count = %d, want exactly one %q", scenario.name, found, scenario.workID)
	}
}

func assertClaudeDispatch(
	t *testing.T,
	scenario claudeScenario,
	sessionID string,
	events []factoryapi.FactoryEvent,
) string {
	t.Helper()

	dispatches := support.ObserveDispatchEvents(t, events)
	if len(dispatches) != 1 {
		t.Fatalf("%s dispatch observations = %#v, want exactly one", scenario.name, dispatches)
	}
	dispatch := dispatches[0]
	if dispatch.DispatchID == "" {
		t.Fatalf("%s dispatch identity is empty", scenario.name)
	}
	if !support.DispatchObservationIncludesWork(dispatch, scenario.workID) {
		t.Fatalf("%s dispatch %q omitted Work %q: %#v", scenario.name, dispatch.DispatchID, scenario.workID, dispatch)
	}
	if dispatch.Response == nil {
		t.Fatalf("%s dispatch %q has no response", scenario.name, dispatch.DispatchID)
	}
	if dispatch.Response.Outcome != scenario.wantOutcome {
		t.Fatalf("%s dispatch outcome = %q, want %q", scenario.name, dispatch.Response.Outcome, scenario.wantOutcome)
	}
	if scenario.wantWorkState == "task:failed" {
		if dispatch.Response.FailureDetail == nil || !strings.Contains(dispatch.Response.FailureDetail.Message, claudeCancellationMessage) {
			t.Fatalf("%s dispatch failure detail = %#v, want canonical cancellation message", scenario.name, dispatch.Response.FailureDetail)
		}
	}
	for _, event := range events {
		if event.Context.SessionId != nil && *event.Context.SessionId != sessionID {
			t.Fatalf("%s Factory Event %q session id = %q, want %q", scenario.name, event.Id, *event.Context.SessionId, sessionID)
		}
	}
	return dispatch.DispatchID
}

func assertClaudeCommand(t *testing.T, router *claudeCommandRouter, scenario claudeScenario) {
	t.Helper()
	requests := scenario.runner.Requests()
	if len(requests) != 1 {
		t.Fatalf("%s routed provider calls = %d, want exactly one; requests=%#v", scenario.name, len(requests), requests)
	}
	routed := router.callsFor(scenario.factoryDir)
	if len(routed) != 1 {
		t.Fatalf("%s immutable route calls = %d, want exactly one; calls=%#v", scenario.name, len(routed), routed)
	}
	request := routed[0].request
	if request.WorkDir != requests[0].WorkDir {
		t.Fatalf("%s router WorkDir = %q, runner WorkDir = %q", scenario.name, request.WorkDir, requests[0].WorkDir)
	}
	if request.Command != claudeConductorProcessCommand {
		t.Fatalf("%s command = %q, want claude", scenario.name, request.Command)
	}
	if request.WorkDir != scenario.factoryDir {
		t.Fatalf("%s command WorkDir = %q, want scenario Factory directory %q", scenario.name, request.WorkDir, scenario.factoryDir)
	}
	if !containsArgPair(request.Args, "--model", claudeConductorModel) {
		t.Fatalf("%s args = %#v, want --model %s", scenario.name, request.Args, claudeConductorModel)
	}
	if !containsArgPair(request.Args, "--output-format", "stream-json") {
		t.Fatalf("%s args = %#v, want Claude stream-json invocation", scenario.name, request.Args)
	}
}

func assertClaudeProviderSession(
	t *testing.T,
	scenario claudeScenario,
	events []factoryapi.FactoryEvent,
) string {
	t.Helper()
	if scenario.providerSessionID == "" {
		return ""
	}
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeInferenceResponse && event.Type != factoryapi.FactoryEventTypeModelResponse {
			continue
		}
		observation, err := support.AsInferenceResponseObservation(event)
		if err != nil {
			t.Fatalf("%s decode provider response: %v", scenario.name, err)
		}
		if observation.ProviderSession == nil || observation.ProviderSession.Id == nil {
			continue
		}
		got := strings.TrimSpace(*observation.ProviderSession.Id)
		if got != scenario.providerSessionID {
			t.Fatalf("%s Provider Session id = %q, want %q", scenario.name, got, scenario.providerSessionID)
		}
		return got
	}
	t.Fatalf("%s missing Provider Session identity %q", scenario.name, scenario.providerSessionID)
	return ""
}

func assertClaudeEventScope(
	t *testing.T,
	scenario claudeScenario,
	sessionID string,
	events []factoryapi.FactoryEvent,
) {
	t.Helper()
	if len(events) == 0 {
		t.Fatalf("%s Factory Event stream is empty", scenario.name)
	}
	for _, event := range events {
		if event.Context.SessionId != nil && *event.Context.SessionId != sessionID {
			t.Fatalf("%s event %q escaped Factory Session %q", scenario.name, event.Id, sessionID)
		}
		if event.Context.RequestId != nil && *event.Context.RequestId != scenario.requestID {
			t.Fatalf("%s event %q request id = %q, want %q", scenario.name, event.Id, *event.Context.RequestId, scenario.requestID)
		}
		if event.Context.WorkIds != nil {
			for _, workID := range *event.Context.WorkIds {
				if workID != scenario.workID {
					t.Fatalf("%s event %q Work id = %q, want %q", scenario.name, event.Id, workID, scenario.workID)
				}
			}
		}
	}
	if scenario.wantWorkState != "task:failed" {
		return
	}
	payload, err := json.Marshal(events)
	if err != nil {
		t.Fatalf("%s marshal Factory Events: %v", scenario.name, err)
	}
	text := string(payload)
	if !strings.Contains(text, claudeCancellationMessage) {
		t.Fatalf("%s Factory Events missing canonical cancellation outcome: %s", scenario.name, text)
	}
	if strings.Contains(text, "Claude command did not complete successfully") {
		t.Fatalf("%s Factory Events used Claude-local cancellation fallback: %s", scenario.name, text)
	}
}

func getClaudeSession(t *testing.T, baseURL, sessionID string) factoryapi.FactorySession {
	t.Helper()
	response := support.GetJSON[factoryapi.FactorySessionGetResponse](
		t,
		strings.TrimSuffix(baseURL, "/")+"/factory-sessions/"+url.PathEscape(sessionID),
	)
	session, err := response.AsFactorySession()
	if err != nil {
		t.Fatalf("decode Factory Session %q: %v", sessionID, err)
	}
	return session
}

func listClaudeSessionWork(t *testing.T, baseURL, sessionID string) factoryapi.ListWorkResponse {
	t.Helper()
	return support.GetJSON[factoryapi.ListWorkResponse](
		t,
		strings.TrimSuffix(baseURL, "/")+"/factory-sessions/"+url.PathEscape(sessionID)+"/work",
	)
}

func assertClaudeSessionDeleted(t *testing.T, baseURL, sessionID string) {
	t.Helper()
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID)
	response, err := http.Get(endpoint)
	if err != nil {
		t.Fatalf("GET deleted Factory Session %q: %v", sessionID, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("GET deleted Factory Session %q status = %d, want 404: %s", sessionID, response.StatusCode, strings.TrimSpace(string(body)))
	}
}

func containsArgPair(args []string, flag, value string) bool {
	for index := 0; index+1 < len(args); index++ {
		if args[index] == flag && args[index+1] == value {
			return true
		}
	}
	return false
}

type claudeCommandRoute struct {
	selector string
	label    string
	runner   *claudeScenarioCommandRunner
}

// claudeCommandRouter is immutable after construction. Its map is populated
// before root.BuildProcess and only read by concurrent provider attempts.
type claudeCommandRouter struct {
	routes map[string]claudeCommandRoute

	mu    sync.Mutex
	calls []claudeRoutedCommand
}

type claudeRoutedCommand struct {
	selector string
	request  platformprocess.CommandRequest
}

func newClaudeCommandRouter(routes []claudeCommandRoute) (*claudeCommandRouter, error) {
	indexed := make(map[string]claudeCommandRoute, len(routes))
	for _, route := range routes {
		selector := filepath.Clean(strings.TrimSpace(route.selector))
		if selector == "." || selector == "" {
			return nil, fmt.Errorf("Claude scenario selector is required")
		}
		if route.runner == nil {
			return nil, fmt.Errorf("Claude scenario selector %q has no command runner", selector)
		}
		if _, exists := indexed[selector]; exists {
			return nil, fmt.Errorf("duplicate Claude scenario selector %q", selector)
		}
		route.selector = selector
		indexed[selector] = route
	}
	return &claudeCommandRouter{routes: indexed}, nil
}

func (router *claudeCommandRouter) Run(
	ctx context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	selector := filepath.Clean(strings.TrimSpace(request.WorkDir))
	route, ok := router.routes[selector]
	if !ok {
		return platformprocess.CommandResult{}, fmt.Errorf("unknown Claude scenario selector %q; refusing to consume another route", request.WorkDir)
	}
	router.mu.Lock()
	router.calls = append(router.calls, claudeRoutedCommand{
		selector: route.selector,
		request:  cloneClaudeCommandRequest(request),
	})
	router.mu.Unlock()
	return route.runner.Run(ctx, request)
}

func (router *claudeCommandRouter) callsFor(selector string) []claudeRoutedCommand {
	router.mu.Lock()
	defer router.mu.Unlock()
	selector = filepath.Clean(strings.TrimSpace(selector))
	var calls []claudeRoutedCommand
	for _, call := range router.calls {
		if call.selector == selector {
			call.request = cloneClaudeCommandRequest(call.request)
			calls = append(calls, call)
		}
	}
	return calls
}

func (router *claudeCommandRouter) callCount() int {
	router.mu.Lock()
	defer router.mu.Unlock()
	return len(router.calls)
}

type claudeScenarioCommandRunner struct {
	result platformprocess.CommandResult
	err    error

	mu       sync.Mutex
	requests []platformprocess.CommandRequest
}

func (runner *claudeScenarioCommandRunner) Run(
	_ context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	runner.mu.Lock()
	runner.requests = append(runner.requests, cloneClaudeCommandRequest(request))
	result := cloneClaudeCommandResult(runner.result)
	err := runner.err
	runner.mu.Unlock()
	return result, err
}

func (runner *claudeScenarioCommandRunner) Requests() []platformprocess.CommandRequest {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	requests := make([]platformprocess.CommandRequest, len(runner.requests))
	for index, request := range runner.requests {
		requests[index] = cloneClaudeCommandRequest(request)
	}
	return requests
}

func (runner *claudeScenarioCommandRunner) CallCount() int {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return len(runner.requests)
}

func cloneClaudeCommandRequest(request platformprocess.CommandRequest) platformprocess.CommandRequest {
	request.Args = append([]string(nil), request.Args...)
	request.Stdin = append([]byte(nil), request.Stdin...)
	request.Env = append([]string(nil), request.Env...)
	return request
}

func cloneClaudeCommandResult(result platformprocess.CommandResult) platformprocess.CommandResult {
	result.Stdout = append([]byte(nil), result.Stdout...)
	result.Stderr = append([]byte(nil), result.Stderr...)
	return result
}

type claudeIdentityGenerator struct {
	sessions      atomic.Uint64
	runtimes      atomic.Uint64
	responseEvent atomic.Uint64
}

func (generator *claudeIdentityGenerator) nextSessionID() string {
	// Explicit live sessions persist this value directly, so use the UUID form
	// accepted by the durable-session store while keeping allocation deterministic.
	return fmt.Sprintf("00000000-0000-4000-8000-%012x", generator.sessions.Add(1))
}

func (generator *claudeIdentityGenerator) nextRuntimeID() string {
	return fmt.Sprintf("c03-claude-runtime-%d", generator.runtimes.Add(1))
}

func (generator *claudeIdentityGenerator) nextResponseEventID() string {
	return fmt.Sprintf("c03-claude-response-event-%d", generator.responseEvent.Add(1))
}

func (generator *claudeIdentityGenerator) sessionCount() uint64 {
	return generator.sessions.Load()
}

var _ platformprocess.CommandRunner = (*claudeCommandRouter)(nil)
