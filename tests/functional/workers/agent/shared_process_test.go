package agent_test

import (
	"context"
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
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const agentSharedProcessTimeout = 20 * time.Second

// TestAgentSharedProcess keeps the four existing agent rows on one immutable
// root-built process. Inert composition is observed before Process.Execute is
// activated; the remaining rows use distinct explicit Factory Sessions and
// immutable provider-command routes.
func TestAgentSharedProcess(t *testing.T) {
	fixture := newAgentSharedProcessFixture(t)

	t.Run("Inert", func(t *testing.T) {
		fixture.assertInert(t)
	})

	fixture.start(t)
	for _, scenario := range fixture.scenarios {
		scenario := scenario
		t.Run(scenario.name, func(t *testing.T) {
			fixture.runScenario(t, scenario)
		})
	}
}

type agentSharedProcessFixture struct {
	process    support.ApplicationProcess
	command    *support.ProcessCommand
	api        *support.ProcessAPIServer
	apiClosed  chan struct{}
	apiClose   sync.Once
	baseURL    string
	hostDir    string
	homeDir    string
	router     *agentSharedCommandRouter
	identities *agentSharedIdentityGenerator
	scenarios  []agentSharedScenario

	processBuilds atomic.Int32
	apiStarts     atomic.Int32
	sessionsMu    sync.Mutex
	opened        map[string]string
	closed        map[string]struct{}
}

type agentSharedScenario struct {
	name        string
	factoryDir  string
	model       string
	inputMarker string
	output      string
	inputMode   agentSharedInputMode
}

type agentSharedInputMode string

const (
	agentSharedTextInput        agentSharedInputMode = "text"
	agentSharedJSONPayloadInput agentSharedInputMode = "json-payload"
	agentSharedJSONSeedInput    agentSharedInputMode = "json-seed"
)

func newAgentSharedProcessFixture(t *testing.T) *agentSharedProcessFixture {
	t.Helper()

	hostDir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	support.ClearSeedInputs(t, hostDir)
	scenarios := newAgentSharedScenarios(t)
	router := newAgentSharedCommandRouter(t, scenarios)
	api := support.NewProcessAPIServer()
	apiClosed := make(chan struct{})
	identities := &agentSharedIdentityGenerator{}
	fixture := &agentSharedProcessFixture{
		api:        api,
		apiClosed:  apiClosed,
		hostDir:    hostDir,
		homeDir:    t.TempDir(),
		router:     router,
		identities: identities,
		scenarios:  scenarios,
		opened:     make(map[string]string, len(scenarios)),
		closed:     make(map[string]struct{}, len(scenarios)),
	}

	process, err := support.BuildProcessWithContext(context.Background(), serviceedges.Edges{
		APIServerStarter: func(ctx context.Context, request platformhttpserver.StartRequest) error {
			fixture.apiStarts.Add(1)
			err := api.Start(ctx, request)
			fixture.apiClose.Do(func() { close(apiClosed) })
			return err
		},
		ProviderCommandRunner:                    router,
		FactorySessionIDGenerator:                identities.nextSessionID,
		FactorySessionRuntimeInstanceIDGenerator: identities.nextRuntimeID,
		FactorySessionResponseEventIDGenerator:   identities.nextResponseEventID,
	})
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}
	fixture.process = process
	fixture.processBuilds.Add(1)
	t.Cleanup(func() { fixture.close(t) })
	return fixture
}

func newAgentSharedScenarios(t *testing.T) []agentSharedScenario {
	t.Helper()

	cases := []struct {
		name        string
		model       string
		inputMarker string
		output      string
		inputMode   agentSharedInputMode
	}{
		{
			name:        "Codex",
			model:       "converged-agent-model",
			inputMarker: "converged agent payload",
			output:      "converged agent response COMPLETE",
			inputMode:   agentSharedTextInput,
		},
		{
			name:        "Registered",
			model:       "registered-agent-model",
			inputMarker: "registered agent payload",
			output:      "registered agent response COMPLETE",
			inputMode:   agentSharedJSONPayloadInput,
		},
		{
			name:        "RuntimeRoot",
			model:       "runtime-root-agent-model",
			inputMarker: "runtime provider root",
			output:      "functional-runtime-provider-output COMPLETE",
			inputMode:   agentSharedJSONSeedInput,
		},
	}

	scenarios := make([]agentSharedScenario, 0, len(cases))
	for _, testCase := range cases {
		dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
		support.ClearSeedInputs(t, dir)
		support.WriteAgentConfig(t, dir, "worker", support.BuildModelWorkerConfig(
			modelprovider.ProviderCodex,
			testCase.model,
		))
		support.WriteWorkstationConfig(t, dir, "process", "---\ntype: MODEL_WORKSTATION\n---\nAgent input: {{ (index .Inputs 0).Payload }}\n")
		if testCase.inputMode == agentSharedJSONSeedInput {
			testutil.WriteSeedFile(t, dir, "task", []byte(fmt.Sprintf(`{"marker":%q}`, testCase.inputMarker)))
		}
		scenarios = append(scenarios, agentSharedScenario{
			name:        testCase.name,
			factoryDir:  dir,
			model:       testCase.model,
			inputMarker: testCase.inputMarker,
			output:      testCase.output,
			inputMode:   testCase.inputMode,
		})
	}
	return scenarios
}

func (fixture *agentSharedProcessFixture) assertInert(t *testing.T) {
	t.Helper()
	if fixture.process == nil || fixture.process.ProviderRegistry() == nil {
		t.Fatal("root-built process or provider registry = nil, want inert composition")
	}
	for _, providerID := range []string{"claude", "codex"} {
		if got, err := fixture.process.ProviderRegistry().CanonicalIdentity(providerID); err != nil || got != providerID {
			t.Fatalf("CanonicalIdentity(%q) = (%q, %v), want (%q, nil)", providerID, got, err, providerID)
		}
	}
	if _, err := fixture.process.ProviderRegistry().CanonicalIdentity("missing.provider"); err == nil {
		t.Fatal("CanonicalIdentity(missing.provider) error = nil, want unknown-provider failure")
	}
	if got := fixture.apiStarts.Load(); got != 0 {
		t.Fatalf("API server starts before activation = %d, want 0", got)
	}
	if got := fixture.router.callCount(); got != 0 {
		t.Fatalf("provider calls before activation = %d, want 0", got)
	}
	fixture.sessionsMu.Lock()
	defer fixture.sessionsMu.Unlock()
	if len(fixture.opened) != 0 {
		t.Fatalf("Factory Sessions before activation = %#v, want none", fixture.opened)
	}
}

func (fixture *agentSharedProcessFixture) start(t *testing.T) {
	t.Helper()
	if fixture.command != nil {
		t.Fatal("shared agent process started more than once")
	}
	inputs := support.FakeInputs(context.Background(), []string{
		"you", "run",
		"--dir", fixture.hostDir,
		"--continuously",
		"--with-server",
		"--server", "http://127.0.0.1:1",
		"--quiet",
		"--no-record",
	})
	inputs.Input.Env = append(os.Environ(), "HOME="+fixture.homeDir, "USERPROFILE="+fixture.homeDir)
	inputs.Input.WorkingDirectory = fixture.hostDir
	fixture.command = support.StartProcessCommand(t, fixture.process, inputs.Input)
	fixture.baseURL = fixture.api.WaitForURL(t)
	defaultSession := support.GetDefaultSession(t, fixture.baseURL)
	if !defaultSession.IsDefault || strings.TrimSpace(defaultSession.Id) == "" {
		t.Fatalf("default Factory Session = %#v, want default flag and identity", defaultSession)
	}
}

func (fixture *agentSharedProcessFixture) runScenario(
	t *testing.T,
	scenario agentSharedScenario,
) {
	t.Helper()

	sessionID := fixture.openSession(t, scenario.factoryDir)
	name := "agent-" + strings.ToLower(scenario.name)
	traceID := "trace-agent-" + strings.ToLower(scenario.name)
	request := factoryapi.SubmitWorkRequest{
		Name:         &name,
		TraceId:      &traceID,
		WorkTypeName: "task",
	}
	switch scenario.inputMode {
	case agentSharedTextInput:
		content := agentTextContent(t, scenario.inputMarker)
		request.Content = &content
	case agentSharedJSONPayloadInput:
		request.Payload = map[string]string{"marker": scenario.inputMarker}
	case agentSharedJSONSeedInput:
		// The seed file is consumed while this explicit session opens.
	default:
		t.Fatalf("%s has unsupported input mode %q", scenario.name, scenario.inputMode)
	}
	var submitted factoryapi.SubmitWorkResponse
	if scenario.inputMode != agentSharedJSONSeedInput {
		submitted = support.SubmitSessionWorkAt(t, fixture.baseURL, sessionID, request)
		if submitted.SessionId == nil || *submitted.SessionId != sessionID {
			t.Fatalf("submitted Work session id = %#v, want %q", submitted.SessionId, sessionID)
		}
	}

	support.WaitForSessionTerminalStatus(t, fixture.baseURL, sessionID, agentSharedProcessTimeout)
	listed := listAgentSessionWork(t, fixture.baseURL, sessionID)
	events := support.GetFactoryEventsForSessionAt(t, fixture.baseURL, sessionID)
	workID := support.StringPointerValue(submitted.WorkId)
	requestID := submitted.RequestId
	if scenario.inputMode == agentSharedJSONSeedInput {
		workID, requestID = seededAgentIdentity(t, events)
	} else if workID == "" || strings.TrimSpace(requestID) == "" {
		t.Fatalf("submitted Work identity = work:%q request:%q, want both identities", workID, requestID)
	}
	assertAgentWork(t, listed, workID, scenario.output)
	assertAgentDispatch(t, events, sessionID, requestID, workID, scenario.output)
	fixture.assertRoute(t, scenario)

	fixture.closeSession(t, sessionID)
	assertAgentSessionDeleted(t, fixture.baseURL, sessionID)
}

func seededAgentIdentity(t *testing.T, events []factoryapi.FactoryEvent) (string, string) {
	t.Helper()
	dispatches := support.ObserveDispatchEvents(t, events)
	if len(dispatches) != 1 {
		t.Fatalf("seeded agent dispatch observations = %#v, want one", dispatches)
	}
	dispatch := dispatches[0]
	workID := ""
	if len(dispatch.WorkIDs) > 0 {
		workID = strings.TrimSpace(dispatch.WorkIDs[0])
	}
	if workID == "" && len(dispatch.Request.Inputs) > 0 {
		workID = strings.TrimSpace(dispatch.Request.Inputs[0].WorkId)
	}
	requestID := ""
	for _, event := range events {
		if event.Context.RequestId != nil && strings.TrimSpace(*event.Context.RequestId) != "" {
			requestID = strings.TrimSpace(*event.Context.RequestId)
			break
		}
	}
	if workID == "" || requestID == "" {
		t.Fatalf("seeded agent identities = work:%q request:%q, want both identities", workID, requestID)
	}
	return workID, requestID
}

func (fixture *agentSharedProcessFixture) openSession(t *testing.T, factoryDir string) string {
	t.Helper()
	opened := support.OpenFactorySessionAt(t, fixture.baseURL, factoryDir)
	if opened.Session == nil || strings.TrimSpace(opened.Session.Id) == "" {
		t.Fatalf("opened Factory Session for %q = %#v, want identity", factoryDir, opened)
	}
	sessionID := opened.Session.Id
	if sessionID == factorysessions.DefaultSessionID {
		t.Fatalf("opened Factory Session for %q = %q, want explicit session", factoryDir, sessionID)
	}
	fixture.sessionsMu.Lock()
	defer fixture.sessionsMu.Unlock()
	if _, exists := fixture.opened[sessionID]; exists {
		t.Fatalf("Factory Session id %q was reused", sessionID)
	}
	fixture.opened[sessionID] = factoryDir
	t.Cleanup(func() { fixture.closeSession(t, sessionID) })
	return sessionID
}

func (fixture *agentSharedProcessFixture) closeSession(t testing.TB, sessionID string) {
	t.Helper()
	fixture.sessionsMu.Lock()
	if _, exists := fixture.closed[sessionID]; exists {
		fixture.sessionsMu.Unlock()
		return
	}
	fixture.sessionsMu.Unlock()
	support.CloseFactorySessionAt(t, fixture.baseURL, sessionID)
	fixture.sessionsMu.Lock()
	fixture.closed[sessionID] = struct{}{}
	fixture.sessionsMu.Unlock()
}

func (fixture *agentSharedProcessFixture) assertRoute(
	t *testing.T,
	scenario agentSharedScenario,
) {
	t.Helper()
	calls := fixture.router.callsFor(scenario.factoryDir)
	if len(calls) != 1 {
		t.Fatalf("%s immutable route calls = %d, want one; calls=%#v", scenario.name, len(calls), calls)
	}
	request := calls[0].request
	if request.Command != string(modelprovider.ProviderCodex) {
		t.Fatalf("%s provider command = %q, want %q", scenario.name, request.Command, modelprovider.ProviderCodex)
	}
	if request.WorkDir != scenario.factoryDir {
		t.Fatalf("%s provider WorkDir = %q, want %q", scenario.name, request.WorkDir, scenario.factoryDir)
	}
	if !containsAgentArgumentPair(request.Args, "--model", scenario.model) {
		t.Fatalf("%s provider args = %#v, want --model %q", scenario.name, request.Args, scenario.model)
	}
	if !agentCommandRequestContains(request, scenario.inputMarker) {
		t.Fatalf("%s provider request omitted input marker %q: %#v", scenario.name, scenario.inputMarker, request)
	}
}

func (fixture *agentSharedProcessFixture) close(t testing.TB) {
	t.Helper()
	if fixture.process == nil {
		return
	}
	if fixture.command != nil {
		// StartProcessCommand registers its cleanup after this fixture cleanup,
		// so its LIFO cleanup stops the invocation before the root closes.
		// Calling Stop here also makes this boundary safe if setup ordering is
		// changed by a future package-level fixture.
		fixture.command.Stop(t)
	}
	closeCtx, cancel := context.WithTimeout(context.Background(), agentSharedProcessTimeout)
	defer cancel()
	if err := fixture.process.Close(closeCtx); err != nil {
		t.Errorf("close shared agent application process: %v", err)
	}
	if fixture.command != nil {
		select {
		case <-fixture.apiClosed:
		case <-closeCtx.Done():
			t.Errorf("shared agent API server did not close: %v", closeCtx.Err())
		}
	}
	if got := fixture.processBuilds.Load(); got != 1 {
		t.Errorf("shared agent process builds = %d, want exactly one", got)
	}
	if fixture.command != nil && fixture.apiStarts.Load() != 1 {
		t.Errorf("shared agent API starts = %d, want exactly one", fixture.apiStarts.Load())
	}
	fixture.sessionsMu.Lock()
	defer fixture.sessionsMu.Unlock()
	if len(fixture.opened) != len(fixture.closed) {
		t.Errorf("closed shared Factory Sessions = %d, opened = %d; opened=%#v closed=%#v", len(fixture.closed), len(fixture.opened), fixture.opened, fixture.closed)
	}
}

type agentSharedCommandRouter struct {
	routes map[string]platformprocess.CommandRunner

	mu    sync.Mutex
	calls []agentSharedRoutedCall
}

type agentSharedRoutedCall struct {
	selector string
	request  platformprocess.CommandRequest
}

func newAgentSharedCommandRouter(
	t *testing.T,
	scenarios []agentSharedScenario,
) *agentSharedCommandRouter {
	t.Helper()
	routes := make(map[string]platformprocess.CommandRunner, len(scenarios))
	for _, scenario := range scenarios {
		selector := filepath.Clean(strings.TrimSpace(scenario.factoryDir))
		if selector == "." || selector == "" {
			t.Fatalf("agent scenario %q has empty Factory selector", scenario.name)
		}
		if _, exists := routes[selector]; exists {
			t.Fatalf("duplicate agent Factory selector %q", selector)
		}
		routes[selector] = support.NewShapedProviderCommandRunner(platformprocess.CommandResult{
			Stdout: []byte(scenario.output),
		})
	}
	return &agentSharedCommandRouter{routes: routes}
}

func (router *agentSharedCommandRouter) Run(
	ctx context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	selector := filepath.Clean(strings.TrimSpace(request.WorkDir))
	runner, ok := router.routes[selector]
	if !ok {
		return platformprocess.CommandResult{}, fmt.Errorf(
			"unknown agent scenario selector %q; refusing to consume another route",
			request.WorkDir,
		)
	}
	router.mu.Lock()
	router.calls = append(router.calls, agentSharedRoutedCall{
		selector: selector,
		request:  cloneAgentCommandRequest(request),
	})
	router.mu.Unlock()
	return runner.Run(ctx, request)
}

func (router *agentSharedCommandRouter) callsFor(selector string) []agentSharedRoutedCall {
	router.mu.Lock()
	defer router.mu.Unlock()
	selector = filepath.Clean(strings.TrimSpace(selector))
	calls := make([]agentSharedRoutedCall, 0)
	for _, call := range router.calls {
		if call.selector != selector {
			continue
		}
		call.request = cloneAgentCommandRequest(call.request)
		calls = append(calls, call)
	}
	return calls
}

func (router *agentSharedCommandRouter) callCount() int {
	router.mu.Lock()
	defer router.mu.Unlock()
	return len(router.calls)
}

type agentSharedIdentityGenerator struct {
	sessions      atomic.Uint64
	runtimes      atomic.Uint64
	responseEvent atomic.Uint64
}

func (generator *agentSharedIdentityGenerator) nextSessionID() string {
	return fmt.Sprintf("00000000-0000-4000-8000-%012x", generator.sessions.Add(1))
}

func (generator *agentSharedIdentityGenerator) nextRuntimeID() string {
	return fmt.Sprintf("agent-shared-runtime-%d", generator.runtimes.Add(1))
}

func (generator *agentSharedIdentityGenerator) nextResponseEventID() string {
	return fmt.Sprintf("agent-shared-response-event-%d", generator.responseEvent.Add(1))
}

func listAgentSessionWork(t testing.TB, baseURL, sessionID string) factoryapi.ListWorkResponse {
	t.Helper()
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID) + "/work"
	return support.GetJSON[factoryapi.ListWorkResponse](t, endpoint)
}

func agentTextContent(t *testing.T, text string) factoryapi.WorkContent {
	t.Helper()
	part := factoryapi.WorkContentPart{}
	if err := part.FromWorkTextContentPart(factoryapi.WorkTextContentPart{
		Text: text,
		Type: factoryapi.WorkContentPartTypeText,
	}); err != nil {
		t.Fatalf("encode text Work content: %v", err)
	}
	return factoryapi.WorkContent{part}
}

func assertAgentWork(
	t *testing.T,
	listed factoryapi.ListWorkResponse,
	workID string,
	wantOutput string,
) {
	t.Helper()
	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 1 {
		t.Fatalf("completed Work = %d, want one; listed=%#v", got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 0 {
		t.Fatalf("failed Work = %d, want zero; listed=%#v", got, listed)
	}
	var found int
	for _, item := range listed.Results {
		if support.StringPointerValue(item.WorkId) != workID {
			continue
		}
		found++
		if item.Content == nil || len(*item.Content) != 1 {
			t.Fatalf("Work %q content = %#v, want one text part", workID, item.Content)
		}
		textPart, err := (*item.Content)[0].AsWorkTextContentPart()
		if err != nil {
			t.Fatalf("decode Work %q text content: %v", workID, err)
		}
		if !strings.Contains(textPart.Text, wantOutput) {
			t.Fatalf("Work %q text = %q, want content %q", workID, textPart.Text, wantOutput)
		}
	}
	if found != 1 {
		t.Fatalf("Work identity count = %d, want exactly one %q; listed=%#v", found, workID, listed)
	}
}

func assertAgentDispatch(
	t *testing.T,
	events []factoryapi.FactoryEvent,
	sessionID string,
	requestID string,
	workID string,
	wantOutput string,
) {
	t.Helper()
	dispatches := support.ObserveDispatchEvents(t, events)
	if len(dispatches) != 1 {
		t.Fatalf("agent dispatch observations = %#v, want one", dispatches)
	}
	for _, event := range events {
		if event.Context.SessionId != nil && *event.Context.SessionId != sessionID {
			t.Fatalf("Factory Event %q escaped Factory Session %q", event.Id, sessionID)
		}
	}
	dispatch := dispatches[0]
	if dispatch.DispatchID == "" || !support.DispatchObservationIncludesWork(dispatch, workID) {
		t.Fatalf("agent dispatch = %#v, want non-empty identity correlated to Work %q", dispatch, workID)
	}
	if dispatch.Request.TransitionId != "process" || dispatch.Response == nil {
		t.Fatalf("agent dispatch = %#v, want process request and response", dispatch)
	}
	if dispatch.Response.Outcome != factoryapi.WorkOutcomeAccepted {
		t.Fatalf("agent dispatch outcome = %s, want ACCEPTED", dispatch.Response.Outcome)
	}
	if got := support.StringPointerValue(dispatch.Response.Output); !strings.Contains(got, wantOutput) {
		t.Fatalf("agent dispatch output = %q, want content %q", got, wantOutput)
	}
	correlated := false
	for _, event := range events {
		if event.Context.RequestId != nil && *event.Context.RequestId == requestID {
			correlated = true
		}
	}
	if !correlated {
		t.Fatalf("Factory Events contain no request correlation for %q", requestID)
	}
}

func assertAgentSessionDeleted(t *testing.T, baseURL, sessionID string) {
	t.Helper()
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID)
	response, err := http.Get(endpoint)
	if err != nil {
		t.Fatalf("GET deleted Factory Session %q: %v", sessionID, err)
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, response.Body)
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("GET deleted Factory Session %q status = %d, want 404", sessionID, response.StatusCode)
	}
}

func agentCommandRequestContains(request platformprocess.CommandRequest, marker string) bool {
	if strings.Contains(string(request.Stdin), marker) {
		return true
	}
	for _, arg := range request.Args {
		if strings.Contains(arg, marker) {
			return true
		}
	}
	return false
}

func containsAgentArgumentPair(args []string, name, value string) bool {
	for index := 0; index+1 < len(args); index++ {
		if args[index] == name && args[index+1] == value {
			return true
		}
	}
	return false
}

func cloneAgentCommandRequest(request platformprocess.CommandRequest) platformprocess.CommandRequest {
	request.Args = append([]string(nil), request.Args...)
	request.Stdin = append([]byte(nil), request.Stdin...)
	request.Env = append([]string(nil), request.Env...)
	return request
}

var _ platformprocess.CommandRunner = (*agentSharedCommandRouter)(nil)
