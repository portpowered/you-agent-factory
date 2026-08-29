package script_test

import (
	"bytes"
	"context"
	"encoding/json"
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

// scriptSharedSpineTimeout is a hard upper bound for a stuck asynchronous
// runtime, not a readiness delay. Work dispatch and Factory Session shutdown
// are customer-visible lifecycle transitions exposed only through the public
// status contract in this functional lane; the injected command edge cannot
// deterministically prove either transition. The bounded status observation
// therefore protects cleanup while returning as soon as the public terminal
// state is observed.
const scriptSharedSpineTimeout = 15 * time.Second

// TestScriptWorkerSharedSuccessSpine proves the complete short-lane shared
// process slice. Each child owns a separate Factory directory and explicit
// Factory Session, while all children use the same immutable root-built
// application and routed command edges.
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
		ProviderCommandRunner:                    router,
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
	name                    string
	factoryDir              string
	workName                string
	traceID                 string
	workTypeName            string
	terminalState           string
	expectedOutput          string
	expectedOutcome         factoryapi.WorkOutcome
	expectedCommand         string
	expectedArgs            []string
	expectedArgSequences    [][]string
	requireEmptyStdin       bool
	expectedFailureMessage  string
	environmentPrivacy      bool
	noInference             bool
	allowMultipleDispatches bool
	cancelAfterCommandStart bool
	runner                  *scriptSharedCommandRunner
	assertResult            scriptSharedResultAssertion
}

type scriptSharedResultAssertion func(
	t *testing.T,
	fixture *scriptSharedSpineFixture,
	scenario scriptSharedScenario,
	sessionID string,
	submitted factoryapi.SubmitWorkResponse,
	listed factoryapi.ListWorkResponse,
	events []factoryapi.FactoryEvent,
)

// scriptSharedCommandRunner records the exact external request while keeping
// the scenario-specific command effect behind the injected edge.
type scriptSharedCommandRunner struct {
	delegate platformprocess.CommandRunner

	mu       sync.Mutex
	requests []platformprocess.CommandRequest
}

func newScriptSharedCommandRunner(delegate platformprocess.CommandRunner) *scriptSharedCommandRunner {
	return &scriptSharedCommandRunner{delegate: delegate}
}

func (runner *scriptSharedCommandRunner) Run(
	ctx context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	if runner == nil || runner.delegate == nil {
		return platformprocess.CommandResult{}, fmt.Errorf("shared script command delegate is required")
	}
	runner.mu.Lock()
	runner.requests = append(runner.requests, cloneScriptCommandRequest(request))
	runner.mu.Unlock()
	return runner.delegate.Run(ctx, request)
}

func (runner *scriptSharedCommandRunner) Requests() []platformprocess.CommandRequest {
	if runner == nil {
		return nil
	}
	runner.mu.Lock()
	defer runner.mu.Unlock()
	requests := make([]platformprocess.CommandRequest, len(runner.requests))
	for index, request := range runner.requests {
		requests[index] = cloneScriptCommandRequest(request)
	}
	return requests
}

func (runner *scriptSharedCommandRunner) Delegate() platformprocess.CommandRunner {
	if runner == nil {
		return nil
	}
	return runner.delegate
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
			name:            testCase.name,
			factoryDir:      testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "script_executor_dir")),
			workName:        testCase.workName,
			traceID:         testCase.traceID,
			workTypeName:    "task",
			terminalState:   "done",
			expectedOutput:  testCase.expectedOutput,
			expectedOutcome: factoryapi.WorkOutcomeAccepted,
			expectedCommand: "echo",
			expectedArgs:    []string{"default-output"},
			noInference:     testCase.noInference,
			runner: newScriptSharedCommandRunner(
				support.NewRecordingCommandRunner(testCase.expectedOutput),
			),
		})
	}
	scenarios = append(scenarios, newScriptSharedExecutionScenarios(t)...)
	return append(scenarios, newScriptSharedEnvironmentScenarios(t)...)
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
		WorkTypeName: scenario.workTypeName,
		Payload:      map[string]string{"input": scenario.workName},
	})
	if submitted.SessionId == nil || *submitted.SessionId != sessionID {
		t.Fatalf("submitted Work session id = %#v, want %q", submitted.SessionId, sessionID)
	}
	workID := support.StringPointerValue(submitted.WorkId)
	if workID == "" || strings.TrimSpace(submitted.RequestId) == "" {
		t.Fatalf("submitted Work identity = work:%q request:%q, want both identities", workID, submitted.RequestId)
	}
	if scenario.cancelAfterCommandStart {
		cancelScriptSharedSessionAfterCommandStart(t, fixture, scenario, sessionID)
	}

	// Work dispatch and Factory Session lifecycle updates are asynchronous.
	// Observe the public terminal status rather than sleeping or handing off at
	// the command edge: only the status contract proves that all Work state has
	// become customer-visible and terminal. A public session cancellation
	// terminalizes the live runtime before its Work categories are projected, so
	// that scenario observes the stopped-runtime contract instead. The package
	// deadline above is a stuck-runtime safety bound, not timeout padding.
	if scenario.cancelAfterCommandStart {
		support.WaitForSessionStopped(t, fixture.baseURL, sessionID, scriptSharedSpineTimeout)
		assertScriptSharedCancellationStatus(t, fixture.baseURL, sessionID)
	} else {
		support.WaitForSessionTerminalStatus(t, fixture.baseURL, sessionID, scriptSharedSpineTimeout)
	}
	listed := listScriptSessionWork(t, fixture.baseURL, sessionID)
	events := support.GetFactoryEventsForSessionAt(t, fixture.baseURL, sessionID)
	assertScriptSharedWork(t, scenario, submitted, listed)
	assertScriptSharedEvents(t, scenario, sessionID, submitted, events)
	assertScriptSharedCommand(t, fixture.commandRouter, scenario)
	assertScriptSharedPublicPrivacy(t, scenario.name, listed, events)
	if scenario.assertResult != nil {
		scenario.assertResult(t, fixture, scenario, sessionID, submitted, listed, events)
	}

	fixture.recordObservation(scenario.name, scriptSharedObservation{
		sessionID: sessionID,
		workID:    workID,
		requestID: submitted.RequestId,
		traceID:   submitted.TraceId,
	})
	fixture.closeSession(t, sessionID)
	assertScriptSessionDeleted(t, fixture.baseURL, sessionID)
}

func cancelScriptSharedSessionAt(
	t *testing.T,
	baseURL string,
	sessionID string,
) factoryapi.FactorySessionLifecycleControlResponse {
	t.Helper()

	requestID := "shared-script-cancel-" + sessionID
	payload, err := json.Marshal(factoryapi.FactorySessionLifecycleControlRequest{RequestId: &requestID})
	if err != nil {
		t.Fatalf("marshal shared script cancellation control: %v", err)
	}
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID) + "/cancel"
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("build shared script cancellation control: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("cancel shared Factory Session %q: %v", sessionID, err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read shared script cancellation response: %v", err)
	}
	if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusAccepted {
		t.Fatalf(
			"cancel shared Factory Session %q status = %d, want 200 or 202: %s",
			sessionID,
			response.StatusCode,
			strings.TrimSpace(string(body)),
		)
	}
	var control factoryapi.FactorySessionLifecycleControlResponse
	if err := json.Unmarshal(body, &control); err != nil {
		t.Fatalf("decode shared script cancellation response: %v", err)
	}
	return control
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

	// The public close contract requires the runtime to report IDLE/FINISHED
	// before deletion. CloseFactorySessionAt performs that bounded lifecycle
	// observation; an injected command completion cannot prove the session is
	// stopped and safe to delete.
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
	wants := map[string]int{
		scenario.workTypeName + ":init":   0,
		scenario.workTypeName + ":failed": 0,
	}
	if scenario.cancelAfterCommandStart {
		wants[scenario.workTypeName+":init"] = 1
	} else if scenario.expectedOutcome == factoryapi.WorkOutcomeFailed {
		wants[scenario.workTypeName+":"+scenario.terminalState] = 0
		wants[scenario.workTypeName+":failed"] = 1
	} else {
		wants[scenario.workTypeName+":"+scenario.terminalState] = 1
	}
	assertSessionPlaces(t, listed, wants)
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
	if scenario.cancelAfterCommandStart {
		work := workByID(t, listed, workID)
		if work.State == nil || work.State.Name != "init" || work.State.Type != factoryapi.WorkStateTypePROCESSING {
			t.Fatalf("%s canceled Work state = %#v, want init/PROCESSING without a routed result", scenario.name, work.State)
		}
		if work.FailureDetail != nil {
			t.Fatalf("%s canceled Work failure detail = %#v, want no business failure", scenario.name, work.FailureDetail)
		}
		if work.StructuredResult != nil {
			t.Fatalf("%s canceled Work structured result = %#v, want no primary success result", scenario.name, work.StructuredResult)
		}
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
	if scenario.cancelAfterCommandStart {
		assertScriptSharedCancellationEvents(t, scenario, submitted, events)
		return
	}
	dispatches := support.ObserveDispatchEvents(t, events)
	if len(dispatches) == 0 || (!scenario.allowMultipleDispatches && len(dispatches) != 1) || dispatches[0].Response == nil {
		want := "one completed dispatch"
		if scenario.allowMultipleDispatches {
			want = "at least one terminal dispatch"
		}
		t.Fatalf("%s dispatch observations = %#v, want %s", scenario.name, dispatches, want)
	}
	if dispatches[0].Response.Outcome != scenario.expectedOutcome {
		t.Fatalf("%s dispatch outcome = %q, want %q", scenario.name, dispatches[0].Response.Outcome, scenario.expectedOutcome)
	}
	if !support.DispatchObservationIncludesWork(dispatches[0], support.StringPointerValue(submitted.WorkId)) {
		t.Fatalf("%s dispatch omitted Work %q", scenario.name, support.StringPointerValue(submitted.WorkId))
	}
	if scenario.expectedOutput != "" {
		assertDispatchOutput(t, events, scenario.expectedOutput)
	} else if scenario.expectedOutcome == factoryapi.WorkOutcomeFailed && dispatches[0].Response.Output != nil {
		t.Fatalf("%s dispatch output = %#v, want no primary result", scenario.name, dispatches[0].Response.Output)
	}
	if scenario.noInference && (hasFactoryEventType(events, factoryapi.FactoryEventTypeInferenceRequest) || hasFactoryEventType(events, factoryapi.FactoryEventTypeInferenceResponse)) {
		t.Fatalf("%s emitted inference events: %v", scenario.name, factoryEventTypes(events))
	}
}

func assertScriptSharedCancellationEvents(
	t *testing.T,
	scenario scriptSharedScenario,
	submitted factoryapi.SubmitWorkResponse,
	events []factoryapi.FactoryEvent,
) {
	t.Helper()

	dispatches := support.ObserveDispatchEvents(t, events)
	if len(dispatches) != 1 {
		t.Fatalf("%s dispatch observations = %#v, want exactly one admitted dispatch", scenario.name, dispatches)
	}
	dispatch := dispatches[0]
	if dispatch.Response != nil {
		t.Fatalf("%s canceled dispatch response = %#v, want no primary dispatch result", scenario.name, dispatch.Response)
	}
	if !support.DispatchObservationIncludesWork(dispatch, support.StringPointerValue(submitted.WorkId)) {
		t.Fatalf("%s canceled dispatch omitted Work %q", scenario.name, support.StringPointerValue(submitted.WorkId))
	}

	var scriptRequests, scriptResponses, agentResponses int
	for _, event := range events {
		switch event.Type {
		case factoryapi.FactoryEventTypeScriptRequest:
			payload, err := event.Payload.AsScriptRequestEventPayload()
			if err != nil {
				t.Fatalf("%s decode script request: %v", scenario.name, err)
			}
			scriptRequests++
			if payload.DispatchId != dispatch.DispatchID {
				t.Fatalf("%s script request dispatch id = %q, want %q", scenario.name, payload.DispatchId, dispatch.DispatchID)
			}
		case factoryapi.FactoryEventTypeScriptResponse:
			payload, err := event.Payload.AsScriptResponseEventPayload()
			if err != nil {
				t.Fatalf("%s decode script response: %v", scenario.name, err)
			}
			scriptResponses++
			if payload.DispatchId != dispatch.DispatchID {
				t.Fatalf("%s script response dispatch id = %q, want %q", scenario.name, payload.DispatchId, dispatch.DispatchID)
			}
			if payload.Outcome != factoryapi.ScriptExecutionOutcome("CANCELED") {
				t.Fatalf("%s script response outcome = %q, want CANCELED", scenario.name, payload.Outcome)
			}
			if payload.FailureType == nil || *payload.FailureType != factoryapi.ScriptFailureType("CANCELED") {
				t.Fatalf("%s script response failure type = %#v, want CANCELED", scenario.name, payload.FailureType)
			}
			if payload.ExitCode != nil || payload.Stdout != "" {
				t.Fatalf("%s script cancellation result = %#v, want no exit code or stdout", scenario.name, payload)
			}
		case factoryapi.FactoryEventTypeAgentRunResponse:
			payload, err := event.Payload.AsAgentRunResponseEventPayload()
			if err != nil {
				t.Fatalf("%s decode agent response: %v", scenario.name, err)
			}
			agentResponses++
			if payload.Outcome != scenario.expectedOutcome {
				t.Fatalf("%s agent response outcome = %q, want %q", scenario.name, payload.Outcome, scenario.expectedOutcome)
			}
		case factoryapi.FactoryEventTypeDispatchResponse:
			t.Fatalf("%s emitted a dispatch response after cancellation", scenario.name)
		}
	}
	if scriptRequests != 1 || scriptResponses != 1 || agentResponses != 1 {
		t.Fatalf(
			"%s cancellation event counts = script requests:%d responses:%d agent responses:%d, want 1/1/1",
			scenario.name,
			scriptRequests,
			scriptResponses,
			agentResponses,
		)
	}
	if scenario.noInference && (hasFactoryEventType(events, factoryapi.FactoryEventTypeInferenceRequest) || hasFactoryEventType(events, factoryapi.FactoryEventTypeInferenceResponse)) {
		t.Fatalf("%s emitted inference events: %v", scenario.name, factoryEventTypes(events))
	}
}

func assertScriptSharedCancellationStatus(t *testing.T, baseURL, sessionID string) {
	t.Helper()
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID) + "/status"
	status := support.GetJSON[factoryapi.StatusResponse](t, endpoint)
	// Live cancellation is represented by canceled Worker-owned event facts;
	// the live runtime itself reports its stopped state as COMPLETED/FINISHED.
	if status.FactoryState != "COMPLETED" || status.RuntimeStatus != "FINISHED" {
		t.Fatalf("canceled Factory Session status = %#v, want COMPLETED/FINISHED stopped state", status)
	}
}

func assertScriptSharedPublicPrivacy(
	t testing.TB,
	label string,
	listed factoryapi.ListWorkResponse,
	events []factoryapi.FactoryEvent,
) {
	t.Helper()
	for name, value := range map[string]any{
		"Work":   listed,
		"events": events,
	} {
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatalf("%s %s privacy witness encoding: %v", label, name, err)
		}
		assertScriptSharedNoUndeclaredValue(t, label+" "+name, string(encoded))
	}
}

func assertScriptSharedNoUndeclaredValue(t testing.TB, label string, values ...string) {
	t.Helper()
	for _, value := range values {
		if strings.Contains(value, undeclaredHostEnvValue) {
			t.Fatalf("%s leaked undeclared host environment value %q", label, undeclaredHostEnvValue)
		}
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
	if request.Command != scenario.expectedCommand {
		t.Fatalf("%s command = %q, want authored command %q", scenario.name, request.Command, scenario.expectedCommand)
	}
	if strings.TrimSpace(request.WorkDir) == "" {
		t.Fatalf("%s script command WorkDir is empty", scenario.name)
	}
	if scenario.expectedArgs != nil {
		assertCommandArgs(t, request, scenario.expectedArgs)
	}
	for _, sequence := range scenario.expectedArgSequences {
		support.AssertArgsContainSequence(t, request.Args, sequence)
	}
	if scenario.requireEmptyStdin && len(request.Stdin) != 0 {
		t.Fatalf("%s command stdin = %q, want empty stdin", scenario.name, string(request.Stdin))
	}
	if scenario.environmentPrivacy {
		if envContainsKey(request.Env, undeclaredHostEnvName) {
			t.Fatalf("%s command env contains undeclared host key %q", scenario.name, undeclaredHostEnvName)
		}
		assertScriptSharedNoUndeclaredValue(t, scenario.name+" command environment", request.Env...)
	}
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

func getScriptSharedWorkByID(t testing.TB, baseURL, sessionID, workID string) factoryapi.Work {
	t.Helper()
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" +
		url.PathEscape(sessionID) + "/work/" + url.PathEscape(workID)
	return support.GetJSON[factoryapi.Work](t, endpoint)
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

func executeScriptSharedWorkShow(
	t testing.TB,
	fixture *scriptSharedSpineFixture,
	sessionID string,
	workID string,
	jsonOutput bool,
) (string, string, error) {
	t.Helper()
	args := []string{
		"you",
		"--server", fixture.baseURL,
		"--session", sessionID,
	}
	if jsonOutput {
		args = append(args, "--json")
	}
	args = append(args, "work", "show", workID)
	inputs := support.FakeInputs(context.Background(), args)
	inputs.Input.Env = sharedScriptProcessEnvironment(fixture.homeDir)
	inputs.Input.WorkingDirectory = fixture.hostDir
	err := fixture.process.Execute(inputs.Input)
	return inputs.Stdout(), inputs.Stderr(), err
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
