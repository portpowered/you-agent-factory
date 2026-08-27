package claude

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

// TestClaudeDefaultLaneSharedProcess proves the four ordinary Claude scenarios
// through one root-built process. Each subtest owns a
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
	opened     atomic.Int32
	closed     atomic.Int32

	ledgerMu sync.Mutex
	ledger   map[string]claudeScenarioObservation
}

type claudeScenario struct {
	name              string
	factoryDir        string
	model             string
	workID            string
	requestID         string
	traceID           string
	providerSessionID string
	runner            *claudeScenarioCommandRunner
	golden            *support.ProviderSessionCase
	wantWorkState     string
	wantOutcome       factoryapi.WorkOutcome
	wantFailure       string
	wantProviderCalls int
	wantDispatches    int
}

type claudeScenarioObservation struct {
	sessionID         string
	workID            string
	requestID         string
	dispatchIDs       []string
	providerSessionID string
	responseEventIDs  []string
}

func newClaudeDefaultLaneFixture(t *testing.T) *claudeDefaultLaneFixture {
	t.Helper()

	identities := &claudeIdentityGenerator{}
	structuredFailureGolden := loadClaudeGoldenCase(t, claudeGoldenStructuredFailureCase)
	assertClaudeGoldenManifest(t, structuredFailureGolden, "claude-structured-failure")
	timeoutGolden := loadClaudeGoldenCase(t, claudeGoldenTimeoutCase)
	assertClaudeGoldenManifest(t, timeoutGolden, "claude-timeout")

	structuredFailureExitCode := 1
	if structuredFailureGolden.Process.ExitCode != nil {
		structuredFailureExitCode = *structuredFailureGolden.Process.ExitCode
	}
	structuredFailureResult := platformprocess.CommandResult{
		Stdout:   append([]byte(nil), structuredFailureGolden.Stdout.Raw...),
		Stderr:   []byte(structuredFailureGolden.Stderr),
		ExitCode: structuredFailureExitCode,
	}
	timeoutExitCode := 124
	if timeoutGolden.Process.ExitCode != nil {
		timeoutExitCode = *timeoutGolden.Process.ExitCode
	}
	timeoutResult := platformprocess.CommandResult{
		Stdout:   append([]byte(nil), timeoutGolden.Stdout.Raw...),
		Stderr:   []byte(timeoutGolden.Stderr),
		ExitCode: timeoutExitCode,
	}
	timeoutResults := make([]platformprocess.CommandResult, claudeGoldenTimeoutCommandInvocations)
	for index := range timeoutResults {
		timeoutResults[index] = timeoutResult
	}

	fixtures := []struct {
		name              string
		model             string
		requestID         string
		workID            string
		traceID           string
		providerSessionID string
		results           []platformprocess.CommandResult
		runErr            error
		golden            *support.ProviderSessionCase
		wantWorkState     string
		wantOutcome       factoryapi.WorkOutcome
		wantFailure       string
		wantProviderCalls int
		wantDispatches    int
	}{
		{
			name:              "Success",
			model:             claudeConductorModel,
			requestID:         "claude-c03-success-request",
			workID:            "claude-c03-success-work",
			traceID:           "claude-c03-success-trace",
			providerSessionID: "claude-c03-success-provider-session",
			results: []platformprocess.CommandResult{{Stdout: []byte(
				`{"type":"result","subtype":"success","is_error":false,"result":"claude functional answer COMPLETE","session_id":"claude-c03-success-provider-session"}` + "\n",
			)}},
			wantWorkState:     "task:done",
			wantOutcome:       factoryapi.WorkOutcomeAccepted,
			wantProviderCalls: 1,
			wantDispatches:    1,
		},
		{
			name:              "Cancellation",
			model:             claudeConductorModel,
			requestID:         "claude-c03-cancellation-request",
			workID:            "claude-c03-cancellation-work",
			traceID:           "claude-c03-cancellation-trace",
			results:           []platformprocess.CommandResult{{}},
			runErr:            context.Canceled,
			wantWorkState:     "task:failed",
			wantOutcome:       factoryapi.WorkOutcomeFailed,
			wantFailure:       claudeCancellationMessage,
			wantProviderCalls: 1,
			wantDispatches:    1,
		},
		{
			name:              "StructuredFailure",
			model:             structuredFailureGolden.Process.Model,
			requestID:         "claude-c03-structured-failure-request",
			workID:            "claude-c03-structured-failure-work",
			traceID:           "claude-c03-structured-failure-trace",
			providerSessionID: "claude-golden-structured-failure-session",
			results:           []platformprocess.CommandResult{structuredFailureResult},
			golden:            &structuredFailureGolden,
			wantWorkState:     "task:failed",
			wantOutcome:       factoryapi.WorkOutcomeFailed,
			wantFailure:       "Reduce the request size below 20 MB.",
			wantProviderCalls: 1,
			wantDispatches:    1,
		},
		{
			name:              "Timeout",
			model:             timeoutGolden.Process.Model,
			requestID:         "claude-c03-timeout-request",
			workID:            "claude-c03-timeout-work",
			traceID:           "claude-c03-timeout-trace",
			providerSessionID: "claude-golden-timeout-session",
			results:           timeoutResults,
			golden:            &timeoutGolden,
			wantWorkState:     "task:failed",
			wantOutcome:       factoryapi.WorkOutcomeFailed,
			wantFailure:       "provider invocation timed out",
			wantProviderCalls: claudeGoldenTimeoutCommandInvocations,
			wantDispatches:    3,
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
		workerConfig := support.BuildModelWorkerConfig(
			modelprovider.ProviderClaude,
			fixture.model,
		)
		if fixture.golden != nil {
			workerConfig = strings.Replace(workerConfig, "stopToken: COMPLETE", "skipPermissions: true\nstopToken: COMPLETE", 1)
		}
		support.WriteAgentConfig(t, dir, "worker", workerConfig)
		testutil.WriteSeedRequest(t, dir, workservice.SubmitRequest{
			RequestID:  fixture.requestID,
			WorkID:     fixture.workID,
			Name:       fixture.workID,
			WorkTypeID: "task",
			TraceID:    fixture.traceID,
			Payload:    []byte(`{"title":"claude default lane"}`),
		})

		runner := newClaudeScenarioCommandRunner(
			fixture.results,
			fixture.runErr,
		)
		scenario := claudeScenario{
			name:              fixture.name,
			factoryDir:        dir,
			model:             fixture.model,
			workID:            fixture.workID,
			requestID:         fixture.requestID,
			traceID:           fixture.traceID,
			providerSessionID: fixture.providerSessionID,
			runner:            runner,
			golden:            fixture.golden,
			wantWorkState:     fixture.wantWorkState,
			wantOutcome:       fixture.wantOutcome,
			wantFailure:       fixture.wantFailure,
			wantProviderCalls: fixture.wantProviderCalls,
			wantDispatches:    fixture.wantDispatches,
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
	// The host's default session is only the server anchor. The four scenarios
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
	fixture.opened.Add(1)
	closed := false
	closeSession := func() {
		if closed {
			return
		}
		support.CloseFactorySessionAt(t, fixture.baseURL, sessionID)
		closed = true
		fixture.closed.Add(1)
	}
	t.Cleanup(func() {
		closeSession()
	})

	support.WaitForSessionTerminalStatus(t, fixture.baseURL, sessionID, claudeConductorRunTimeout)
	session := getClaudeSession(t, fixture.baseURL, sessionID)
	listed := listClaudeSessionWork(t, fixture.baseURL, sessionID)
	events := support.GetFactoryEventsForSessionAt(t, fixture.baseURL, sessionID)
	responseEvents := support.GetFactoryResponseEventsAt(t, fixture.baseURL, sessionID)

	assertClaudeWork(t, scenario, listed)
	dispatchIDs := assertClaudeDispatch(t, scenario, sessionID, events)
	assertClaudeCommand(t, fixture.router, scenario)
	providerSessionID := assertClaudeProviderSession(t, scenario, events)
	assertClaudeEventScope(t, scenario, sessionID, events)
	responseEventIDs := assertClaudeResponseEvents(t, scenario, sessionID, responseEvents)
	assertClaudeGoldenScenario(t, scenario, events, responseEvents)

	closeSession()
	assertClaudeSessionDeleted(t, fixture.baseURL, sessionID)
	fixture.recordObservation(claudeScenarioObservation{
		sessionID:         session.Id,
		workID:            scenario.workID,
		requestID:         scenario.requestID,
		dispatchIDs:       dispatchIDs,
		providerSessionID: providerSessionID,
		responseEventIDs:  responseEventIDs,
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
	if len(observations) == 0 {
		t.Fatal("shared-process scenario observations are empty")
	}

	seenSessions := make(map[string]string, len(observations))
	seenWorks := make(map[string]string, len(observations))
	seenRequests := make(map[string]string, len(observations))
	seenDispatches := make(map[string]string, len(observations))
	seenProviderSessions := make(map[string]string, len(observations))
	seenResponseEvents := make(map[string]string)
	for _, observation := range observations {
		assertClaudeUniqueIdentity(t, seenSessions, observation.sessionID, observation.requestID, "Factory Session")
		assertClaudeUniqueIdentity(t, seenWorks, observation.workID, observation.requestID, "Work")
		assertClaudeUniqueIdentity(t, seenRequests, observation.requestID, observation.requestID, "request")
		for _, dispatchID := range observation.dispatchIDs {
			assertClaudeUniqueIdentity(t, seenDispatches, dispatchID, observation.requestID, "dispatch")
		}
		if observation.providerSessionID != "" {
			assertClaudeUniqueIdentity(t, seenProviderSessions, observation.providerSessionID, observation.requestID, "Provider Session")
		}
		for _, responseEventID := range observation.responseEventIDs {
			assertClaudeUniqueIdentity(t, seenResponseEvents, responseEventID, observation.requestID, "response event")
		}
	}
	if len(observations) != len(fixture.scenarios) {
		// An anchored subtest intentionally exercises only the selected route;
		// the full parent gate below is the evidence for all four scenarios.
		return
	}
	if got := fixture.opened.Load(); got != int32(len(fixture.scenarios)) {
		t.Fatalf("Factory Session opens = %d, want %d", got, len(fixture.scenarios))
	}
	if got := fixture.closed.Load(); got != int32(len(fixture.scenarios)) {
		t.Fatalf("Factory Session closes = %d, want %d", got, len(fixture.scenarios))
	}
	if got := fixture.identities.sessionCount(); got < uint64(len(fixture.scenarios)) {
		t.Fatalf("Factory Session IDs generated = %d, want at least %d explicit sessions", got, len(fixture.scenarios))
	}
	if got := fixture.apiStarts.Load(); got != 1 {
		t.Fatalf("API server starts = %d, want exactly one shared process server", got)
	}
	wantProviderCalls := 0
	for _, scenario := range fixture.scenarios {
		wantProviderCalls += scenario.wantProviderCalls
	}
	if got := fixture.router.callCount(); got != wantProviderCalls {
		t.Fatalf("shared process routed provider calls = %d, want %d", got, wantProviderCalls)
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
		if scenario.wantFailure != "" {
			if item.FailureDetail == nil || !strings.Contains(item.FailureDetail.Message, scenario.wantFailure) {
				t.Fatalf("%s Work failure detail = %#v, want %q", scenario.name, item.FailureDetail, scenario.wantFailure)
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
) []string {
	t.Helper()

	dispatches := support.ObserveDispatchEvents(t, events)
	if len(dispatches) != scenario.wantDispatches {
		t.Fatalf("%s dispatch observations = %#v, want %d", scenario.name, dispatches, scenario.wantDispatches)
	}
	dispatchIDs := make([]string, 0, len(dispatches))
	for _, dispatch := range dispatches {
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
			if scenario.wantFailure == "" {
				t.Fatalf("%s failed dispatch has no expected failure message", scenario.name)
			}
			if dispatch.Response.FailureDetail == nil || !strings.Contains(dispatch.Response.FailureDetail.Message, scenario.wantFailure) {
				t.Fatalf("%s dispatch failure detail = %#v, want %q", scenario.name, dispatch.Response.FailureDetail, scenario.wantFailure)
			}
		}
		dispatchIDs = append(dispatchIDs, dispatch.DispatchID)
	}
	for _, event := range events {
		if event.Context.SessionId != nil && *event.Context.SessionId != sessionID {
			t.Fatalf("%s Factory Event %q session id = %q, want %q", scenario.name, event.Id, *event.Context.SessionId, sessionID)
		}
	}
	return dispatchIDs
}

func assertClaudeCommand(t *testing.T, router *claudeCommandRouter, scenario claudeScenario) {
	t.Helper()
	requests := scenario.runner.Requests()
	if len(requests) != scenario.wantProviderCalls {
		t.Fatalf("%s routed provider calls = %d, want %d; requests=%#v", scenario.name, len(requests), scenario.wantProviderCalls, requests)
	}
	routed := router.callsFor(scenario.factoryDir)
	if len(routed) != scenario.wantProviderCalls {
		t.Fatalf("%s immutable route calls = %d, want %d; calls=%#v", scenario.name, len(routed), scenario.wantProviderCalls, routed)
	}
	for index, routedCall := range routed {
		request := routedCall.request
		if request.WorkDir != requests[index].WorkDir {
			t.Fatalf("%s router WorkDir = %q, runner WorkDir = %q", scenario.name, request.WorkDir, requests[index].WorkDir)
		}
		if request.Command != claudeConductorProcessCommand {
			t.Fatalf("%s command = %q, want claude", scenario.name, request.Command)
		}
		if request.WorkDir != scenario.factoryDir {
			t.Fatalf("%s command WorkDir = %q, want scenario Factory directory %q", scenario.name, request.WorkDir, scenario.factoryDir)
		}
		if !containsArgPair(request.Args, "--model", scenario.model) {
			t.Fatalf("%s args = %#v, want --model %s", scenario.name, request.Args, scenario.model)
		}
		if !containsArgPair(request.Args, "--output-format", "stream-json") {
			t.Fatalf("%s args = %#v, want Claude stream-json invocation", scenario.name, request.Args)
		}
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
	if scenario.wantFailure == "" {
		t.Fatalf("%s failed Factory Event stream has no expected failure message", scenario.name)
	}
	payload, err := json.Marshal(events)
	if err != nil {
		t.Fatalf("%s marshal Factory Events: %v", scenario.name, err)
	}
	text := string(payload)
	if !strings.Contains(text, scenario.wantFailure) {
		t.Fatalf("%s Factory Events missing expected failure %q: %s", scenario.name, scenario.wantFailure, text)
	}
	if strings.Contains(text, "Claude command did not complete successfully") {
		t.Fatalf("%s Factory Events used Claude-local cancellation fallback: %s", scenario.name, text)
	}
}

func assertClaudeResponseEvents(
	t *testing.T,
	scenario claudeScenario,
	sessionID string,
	responseEvents []factoryapi.FactoryResponseEvent,
) []string {
	t.Helper()
	if len(responseEvents) == 0 {
		t.Fatalf("%s response-event stream is empty", scenario.name)
	}
	ids := make([]string, 0, len(responseEvents))
	seen := make(map[string]struct{}, len(responseEvents))
	var previousSequence int64
	for index, event := range responseEvents {
		if event.FactorySessionId != sessionID {
			t.Fatalf("%s response event[%d] session id = %q, want %q", scenario.name, index, event.FactorySessionId, sessionID)
		}
		if strings.TrimSpace(event.EventId) == "" {
			t.Fatalf("%s response event[%d] has empty event id", scenario.name, index)
		}
		if _, exists := seen[event.EventId]; exists {
			t.Fatalf("%s response event id %q is duplicated", scenario.name, event.EventId)
		}
		seen[event.EventId] = struct{}{}
		if index > 0 && event.Sequence <= previousSequence {
			t.Fatalf("%s response event sequence[%d] = %d, previous = %d", scenario.name, index, event.Sequence, previousSequence)
		}
		if scenario.providerSessionID != "" && event.ProviderSessionRef != nil && *event.ProviderSessionRef != scenario.providerSessionID {
			t.Fatalf("%s response event[%d] Provider Session ref = %q, want %q", scenario.name, index, *event.ProviderSessionRef, scenario.providerSessionID)
		}
		previousSequence = event.Sequence
		ids = append(ids, event.EventId)
	}
	return ids
}

func assertClaudeGoldenScenario(
	t *testing.T,
	scenario claudeScenario,
	events []factoryapi.FactoryEvent,
	responseEvents []factoryapi.FactoryResponseEvent,
) {
	t.Helper()
	if scenario.golden == nil {
		return
	}

	switch scenario.golden.Manifest.ID {
	case "claude-structured-failure":
		inferencePayload := claudeGoldenFailedInferenceObservation(t, events)
		if inferencePayload.Outcome != factoryapi.InferenceOutcomeFailed {
			t.Fatalf("%s inference outcome = %q, want FAILED", scenario.name, inferencePayload.Outcome)
		}
		if inferencePayload.FailureDetail == nil || inferencePayload.FailureDetail.Message != scenario.wantFailure {
			t.Fatalf("%s inference failure detail = %#v, want %q", scenario.name, inferencePayload.FailureDetail, scenario.wantFailure)
		}
		if inferencePayload.Response != nil && strings.Contains(*inferencePayload.Response, "COMPLETE") {
			t.Fatalf("%s structured failure treated COMPLETE-bearing output as success: %q", scenario.name, *inferencePayload.Response)
		}
		assertProviderSessionGoldensMatch(t, *scenario.golden, observeClaudeFailedProviderSessionGoldens(t, inferencePayload, responseEvents))
	case "claude-timeout":
		inferencePayload := claudeGoldenFailedInferenceObservationWithReason(
			t,
			events,
			factoryapi.WorkFailureTypeTimeout,
		)
		if inferencePayload.Outcome != factoryapi.InferenceOutcomeFailed {
			t.Fatalf("%s inference outcome = %q, want FAILED", scenario.name, inferencePayload.Outcome)
		}
		if inferencePayload.FailureDetail == nil || inferencePayload.FailureDetail.Message != scenario.wantFailure {
			t.Fatalf("%s inference failure detail = %#v, want %q", scenario.name, inferencePayload.FailureDetail, scenario.wantFailure)
		}
		if inferencePayload.Response != nil && strings.Contains(*inferencePayload.Response, "COMPLETE") {
			t.Fatalf("%s timeout treated COMPLETE-bearing output as success: %q", scenario.name, *inferencePayload.Response)
		}
		assertClaudeGoldenResponseStreamClosesWithoutSuccess(t, responseEvents)
		// The existing timeout response-event fixture intentionally captures the
		// legacy retry transcript, while this conductor asserts the normalized
		// retry and stream-closure behavior directly above.
	default:
		t.Fatalf("%s has unsupported Claude golden %q", scenario.name, scenario.golden.Manifest.ID)
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
	results []platformprocess.CommandResult
	err     error

	mu       sync.Mutex
	requests []platformprocess.CommandRequest
}

func newClaudeScenarioCommandRunner(
	results []platformprocess.CommandResult,
	runErr error,
) *claudeScenarioCommandRunner {
	clonedResults := make([]platformprocess.CommandResult, len(results))
	for index, result := range results {
		clonedResults[index] = cloneClaudeCommandResult(result)
	}
	return &claudeScenarioCommandRunner{
		results: clonedResults,
		err:     runErr,
	}
}

func (runner *claudeScenarioCommandRunner) Run(
	_ context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	runner.mu.Lock()
	runner.requests = append(runner.requests, cloneClaudeCommandRequest(request))
	resultIndex := len(runner.requests) - 1
	if resultIndex >= len(runner.results) {
		runner.mu.Unlock()
		return platformprocess.CommandResult{}, fmt.Errorf(
			"Claude scenario command result queue exhausted at call %d",
			resultIndex+1,
		)
	}
	result := cloneClaudeCommandResult(runner.results[resultIndex])
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
