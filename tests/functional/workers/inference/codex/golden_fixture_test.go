package codex

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	workservice "github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// Timeout retries are deliberately queued per provider attempt so the shared
// command edge never falls back to another golden scenario's result.
const codexGoldenTimeoutCommandInvocations = 12

func loadCodexGoldenCase(t *testing.T, caseName string) support.ProviderSessionCase {
	t.Helper()

	repoRoot := testutil.MustRepoRoot(t)
	caseDir := filepath.Join(
		repoRoot,
		filepath.FromSlash(support.ProviderSessionFixturePath("codex", caseName)),
	)
	loaded, err := support.LoadProviderSessionCase(caseDir)
	if err != nil {
		t.Fatalf("LoadProviderSessionCase(%q): %v", caseName, err)
	}
	return loaded
}

type codexGoldenFixture struct {
	process       support.ApplicationProcess
	command       *codexPackageProcessCommand
	baseURL       string
	hostDir       string
	apiStopped    <-chan struct{}
	router        *codexCommandRouter
	identities    *codexIdentityGenerator
	apiStarts     *atomic.Int32
	scenarios     []codexGoldenScenario
	opened        atomic.Int32
	closed        atomic.Int32
	streamsOpened atomic.Int32
	streamsClosed atomic.Int32

	ledgerMu sync.Mutex
	ledger   map[string]codexGoldenScenarioObservation
}

type codexGoldenScenario struct {
	name              string
	factoryDir        string
	model             string
	workID            string
	requestID         string
	traceID           string
	providerSessionID string
	loaded            support.ProviderSessionCase
	runner            *codexScenarioCommandRunner
	wantWorkState     string
	wantOutcome       factoryapi.WorkOutcome
	wantFailure       string
	wantProviderCalls int
	wantDispatches    int
}

type codexGoldenScenarioDefinition struct {
	name              string
	model             string
	requestID         string
	workID            string
	traceID           string
	providerSessionID string
	loaded            support.ProviderSessionCase
	results           []platformprocess.CommandResult
	wantWorkState     string
	wantOutcome       factoryapi.WorkOutcome
	wantFailure       string
	wantProviderCalls int
	wantDispatches    int
}

type codexGoldenScenarioObservation struct {
	sessionID         string
	workID            string
	requestID         string
	dispatchIDs       []string
	providerSessionID string
	responseEventIDs  []string
}

type codexGoldenReplayResult struct {
	Loaded         support.ProviderSessionCase
	Listed         factoryapi.ListWorkResponse
	FactoryEvents  []factoryapi.FactoryEvent
	ResponseEvents []factoryapi.FactoryResponseEvent
	Runner         *codexScenarioCommandRunner
	SessionID      string
}

func newCodexGoldenFixture(t *testing.T) *codexGoldenFixture {
	t.Helper()

	if value := os.Getenv(support.ProviderSessionUpdateFunctionalGoldensEnv); value != "" {
		t.Fatalf(
			"%s=%q must be unset for the golden fidelity gate",
			support.ProviderSessionUpdateFunctionalGoldensEnv,
			value,
		)
	}

	packageFixture := ensureCodexPackageFixture(t)
	packageFixture.beginGroup(t, "golden")
	return &codexGoldenFixture{
		process:    packageFixture.process,
		command:    packageFixture.command,
		baseURL:    packageFixture.baseURL,
		hostDir:    packageFixture.hostDir,
		apiStopped: packageFixture.apiStopped,
		router:     packageFixture.router,
		identities: packageFixture.identities,
		apiStarts:  packageFixture.apiStarts,
		scenarios:  packageFixture.goldenScenarios,
		ledger:     make(map[string]codexGoldenScenarioObservation, len(packageFixture.goldenScenarios)),
	}
}

func newCodexGoldenScenariosAt(t *testing.T, rootDir string) []codexGoldenScenario {
	t.Helper()

	definitions := codexGoldenScenarioDefinitions(t)
	scenarios := make([]codexGoldenScenario, 0, len(definitions))
	for _, definition := range definitions {
		scenarios = append(scenarios, materializeCodexGoldenScenario(t, rootDir, definition))
	}
	return scenarios
}

func codexGoldenScenarioDefinitions(t *testing.T) []codexGoldenScenarioDefinition {
	t.Helper()

	success := loadCodexGoldenCase(t, "success")
	assertCodexGoldenManifest(t, success, "codex-message-tool-success")
	structuredFailure := loadCodexGoldenCase(t, codexGoldenStructuredFailureCase)
	assertCodexGoldenManifest(t, structuredFailure, "codex-structured-failure")
	timeout := loadCodexGoldenCase(t, codexGoldenTimeoutCase)
	assertCodexGoldenManifest(t, timeout, "codex-timeout")

	timeoutResult := codexGoldenCommandResult(timeout, 1)
	timeoutResults := make([]platformprocess.CommandResult, codexGoldenTimeoutCommandInvocations)
	for index := range timeoutResults {
		timeoutResults[index] = timeoutResult
	}

	return []codexGoldenScenarioDefinition{
		{
			name:              "Success",
			model:             success.Process.Model,
			requestID:         "codex-c04-golden-success-request",
			workID:            "codex-c04-golden-success-work",
			traceID:           "codex-c04-golden-success-trace",
			providerSessionID: "session_fixture_codex_success",
			loaded:            success,
			results:           []platformprocess.CommandResult{codexGoldenCommandResult(success, 0)},
			wantWorkState:     "task:done",
			wantOutcome:       factoryapi.WorkOutcomeAccepted,
			wantProviderCalls: 1,
			wantDispatches:    1,
		},
		{
			name:              "StructuredFailure",
			model:             structuredFailure.Process.Model,
			requestID:         "codex-c04-golden-structured-failure-request",
			workID:            "codex-c04-golden-structured-failure-work",
			traceID:           "codex-c04-golden-structured-failure-trace",
			providerSessionID: "session_fixture_codex_structured_failure",
			loaded:            structuredFailure,
			results:           []platformprocess.CommandResult{codexGoldenCommandResult(structuredFailure, 1)},
			wantWorkState:     "task:failed",
			wantOutcome:       factoryapi.WorkOutcomeFailed,
			wantFailure:       "Codex authentication failed.",
			wantProviderCalls: 1,
			wantDispatches:    1,
		},
		{
			name:              "Timeout",
			model:             timeout.Process.Model,
			requestID:         "codex-c04-golden-timeout-request",
			workID:            "codex-c04-golden-timeout-work",
			traceID:           "codex-c04-golden-timeout-trace",
			providerSessionID: "session_fixture_codex_timeout",
			loaded:            timeout,
			results:           timeoutResults,
			wantWorkState:     "task:failed",
			wantOutcome:       factoryapi.WorkOutcomeFailed,
			wantFailure:       "provider invocation timed out",
		},
	}
}

func materializeCodexGoldenScenario(
	t *testing.T,
	rootDir string,
	definition codexGoldenScenarioDefinition,
) codexGoldenScenario {
	t.Helper()

	dir := copyCodexFixtureDir(
		t,
		support.LegacyFixtureDir(t, "executor_success"),
		rootDir,
		"golden-"+definition.name,
	)
	support.WriteAgentConfig(t, dir, "worker", support.BuildModelWorkerConfig(
		modelprovider.ProviderCodex,
		definition.model,
	))
	testutil.WriteSeedRequest(t, dir, workservice.SubmitRequest{
		RequestID:  definition.requestID,
		WorkID:     definition.workID,
		Name:       definition.workID,
		WorkTypeID: "task",
		TraceID:    definition.traceID,
		Payload:    []byte(`{"title":"codex golden shared process"}`),
	})

	return codexGoldenScenario{
		name:              definition.name,
		factoryDir:        dir,
		model:             definition.model,
		workID:            definition.workID,
		requestID:         definition.requestID,
		traceID:           definition.traceID,
		providerSessionID: definition.providerSessionID,
		loaded:            definition.loaded,
		runner:            newCodexScenarioCommandRunner(definition.results, nil),
		wantWorkState:     definition.wantWorkState,
		wantOutcome:       definition.wantOutcome,
		wantFailure:       definition.wantFailure,
		wantProviderCalls: definition.wantProviderCalls,
		wantDispatches:    definition.wantDispatches,
	}
}

func resetCodexGoldenScenario(t *testing.T, scenario codexGoldenScenario) {
	t.Helper()
	overwriteCodexFixtureDir(
		t,
		support.LegacyFixtureDir(t, "executor_success"),
		scenario.factoryDir,
	)
	support.WriteAgentConfig(t, scenario.factoryDir, "worker", support.BuildModelWorkerConfig(
		modelprovider.ProviderCodex,
		scenario.model,
	))
	testutil.WriteSeedRequest(t, scenario.factoryDir, workservice.SubmitRequest{
		RequestID:  scenario.requestID,
		WorkID:     scenario.workID,
		Name:       scenario.workID,
		WorkTypeID: "task",
		TraceID:    scenario.traceID,
		Payload:    []byte(`{"title":"codex golden shared process"}`),
	})
}

func assertCodexGoldenManifest(
	t *testing.T,
	loaded support.ProviderSessionCase,
	wantID string,
) {
	t.Helper()
	if loaded.Manifest.ID != wantID {
		t.Fatalf("manifest.ID = %q, want %q", loaded.Manifest.ID, wantID)
	}
	if loaded.Manifest.FidelityClass != support.ProviderSessionFidelityPartialStream {
		t.Fatalf(
			"manifest.fidelityClass = %q, want %q",
			loaded.Manifest.FidelityClass,
			support.ProviderSessionFidelityPartialStream,
		)
	}
}

func codexGoldenCommandResult(
	loaded support.ProviderSessionCase,
	fallbackExitCode int,
) platformprocess.CommandResult {
	exitCode := fallbackExitCode
	if loaded.Process.ExitCode != nil {
		exitCode = *loaded.Process.ExitCode
	}
	return platformprocess.CommandResult{
		Stdout:   append([]byte(nil), loaded.Stdout.Raw...),
		Stderr:   []byte(loaded.Stderr),
		ExitCode: exitCode,
	}
}

func (fixture *codexGoldenFixture) runScenario(
	t *testing.T,
	scenario codexGoldenScenario,
) codexGoldenReplayResult {
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
	t.Cleanup(closeSession)

	responseStream := support.OpenFactoryResponseEventStreamAt(
		t,
		support.SessionResponseEventsURL(fixture.baseURL, sessionID),
	)
	fixture.streamsOpened.Add(1)
	streamClosed := false
	closeResponseStream := func() {
		if streamClosed {
			return
		}
		responseStream.Close()
		streamClosed = true
		fixture.streamsClosed.Add(1)
	}
	t.Cleanup(closeResponseStream)

	// Attach the public response stream before releasing a fast controlled
	// result so success, failure, and timeout all have the same observation edge.
	scenario.runner.Release()
	support.WaitForSessionTerminalStatus(t, fixture.baseURL, sessionID, codexConductorRunTimeout)
	listed := listCodexSessionWork(t, fixture.baseURL, sessionID)
	events := support.GetFactoryEventsForSessionAt(t, fixture.baseURL, sessionID)
	responseEvents := readCodexGoldenResponseEvents(t, responseStream, scenario, codexConductorRunTimeout)

	assertCodexGoldenWork(t, scenario, listed)
	dispatchIDs := assertCodexGoldenDispatch(t, scenario, sessionID, events)
	assertCodexGoldenCommand(t, fixture.router, scenario)
	providerSessionID := assertCodexGoldenProviderSession(t, scenario, events)
	assertCodexGoldenEventScope(t, scenario, sessionID, events)
	responseEventIDs := assertCodexGoldenResponseEvents(t, scenario, sessionID, responseEvents)

	closeSession()
	responseStream.WaitClosed(codexConductorRunTimeout)
	streamClosed = true
	fixture.streamsClosed.Add(1)
	if extra := drainCodexGoldenResponseEvents(t, responseStream, scenario); len(extra) != 0 {
		t.Fatalf("%s response stream emitted %d events beyond its declared golden", scenario.name, len(extra))
	}
	assertCodexSessionDeleted(t, fixture.baseURL, sessionID)
	fixture.recordObservation(codexGoldenScenarioObservation{
		sessionID:         sessionID,
		workID:            scenario.workID,
		requestID:         scenario.requestID,
		dispatchIDs:       dispatchIDs,
		providerSessionID: providerSessionID,
		responseEventIDs:  responseEventIDs,
	})

	return codexGoldenReplayResult{
		Loaded:         scenario.loaded,
		Listed:         listed,
		FactoryEvents:  events,
		ResponseEvents: responseEvents,
		Runner:         scenario.runner,
		SessionID:      sessionID,
	}
}

func (fixture *codexGoldenFixture) recordObservation(
	observation codexGoldenScenarioObservation,
) {
	fixture.ledgerMu.Lock()
	defer fixture.ledgerMu.Unlock()
	fixture.ledger[observation.requestID] = observation
}

func (fixture *codexGoldenFixture) assertSharedIdentityLedger(t *testing.T) {
	t.Helper()

	fixture.ledgerMu.Lock()
	observations := make([]codexGoldenScenarioObservation, 0, len(fixture.ledger))
	for _, observation := range fixture.ledger {
		observations = append(observations, observation)
	}
	fixture.ledgerMu.Unlock()
	if len(observations) == 0 {
		t.Fatal("shared golden-process scenario observations are empty")
	}

	seenSessions := make(map[string]string, len(observations))
	seenWorks := make(map[string]string, len(observations))
	seenRequests := make(map[string]string, len(observations))
	seenDispatches := make(map[string]string, len(observations))
	seenProviderSessions := make(map[string]string, len(observations))
	seenResponseEvents := make(map[string]string)
	for _, observation := range observations {
		assertCodexUniqueIdentity(t, seenSessions, observation.sessionID, observation.requestID, "Factory Session")
		assertCodexUniqueIdentity(t, seenWorks, observation.workID, observation.requestID, "Work")
		assertCodexUniqueIdentity(t, seenRequests, observation.requestID, observation.requestID, "request")
		for _, dispatchID := range observation.dispatchIDs {
			assertCodexUniqueIdentity(t, seenDispatches, dispatchID, observation.requestID, "dispatch")
		}
		assertCodexUniqueIdentity(t, seenProviderSessions, observation.providerSessionID, observation.requestID, "Provider Session")
		for _, responseEventID := range observation.responseEventIDs {
			assertCodexUniqueIdentity(t, seenResponseEvents, responseEventID, observation.requestID, "response event")
		}
	}

	if got := fixture.opened.Load(); got != fixture.closed.Load() {
		t.Fatalf("Factory Session opens = %d, closes = %d", got, fixture.closed.Load())
	}
	if got := fixture.streamsOpened.Load(); got != fixture.streamsClosed.Load() {
		t.Fatalf("response collectors opened = %d, closed = %d", got, fixture.streamsClosed.Load())
	}
	if got := fixture.apiStarts.Load(); got != 1 {
		t.Fatalf("API server starts = %d, want exactly one shared process server", got)
	}
	for _, scenario := range fixture.scenarios {
		if got := scenario.runner.ActiveCallCount(); got != 0 {
			t.Fatalf("%s active Codex command calls after scenario cleanup = %d, want 0", scenario.name, got)
		}
	}
	if len(observations) != len(fixture.scenarios) {
		// A focused anchored run intentionally exercises only the selected route;
		// the combined parent gate proves the complete golden set.
		return
	}
	if got := fixture.opened.Load(); got != int32(len(fixture.scenarios)) {
		t.Fatalf("Factory Session opens = %d, want %d", got, len(fixture.scenarios))
	}
	if got := fixture.router.callCount(); got == 0 {
		t.Fatal("shared golden process routed no provider calls")
	}
}

func (fixture *codexGoldenFixture) assertSharedProcessCleanup(t *testing.T) {
	t.Helper()
}

func assertCodexGoldenWork(
	t *testing.T,
	scenario codexGoldenScenario,
	listed factoryapi.ListWorkResponse,
) {
	t.Helper()
	if got := support.CountWorkAtCustomerState(listed, scenario.wantWorkState); got != 1 {
		t.Fatalf("%s terminal Work state count = %d, want 1; listed=%#v", scenario.name, got, listed)
	}
	if scenario.wantWorkState == "task:done" && support.CountWorkAtCustomerState(listed, "task:failed") != 0 {
		t.Fatalf("%s produced failed Work alongside success", scenario.name)
	}
	if scenario.wantWorkState == "task:failed" && support.CountWorkAtCustomerState(listed, "task:done") != 0 {
		t.Fatalf("%s produced completed Work alongside failure", scenario.name)
	}

	found := 0
	for _, item := range listed.Results {
		if support.StringPointerValue(item.WorkId) != scenario.workID {
			continue
		}
		found++
		if support.StringPointerValue(item.RequestId) != scenario.requestID {
			t.Fatalf("%s Work request id = %q, want %q", scenario.name, support.StringPointerValue(item.RequestId), scenario.requestID)
		}
		if scenario.wantFailure != "" && (item.FailureDetail == nil || !strings.Contains(item.FailureDetail.Message, scenario.wantFailure)) {
			t.Fatalf("%s Work failure detail = %#v, want %q", scenario.name, item.FailureDetail, scenario.wantFailure)
		}
	}
	if found != 1 {
		t.Fatalf("%s Work identity count = %d, want exactly one %q", scenario.name, found, scenario.workID)
	}
}

func assertCodexGoldenDispatch(
	t *testing.T,
	scenario codexGoldenScenario,
	sessionID string,
	events []factoryapi.FactoryEvent,
) []string {
	t.Helper()

	dispatches := support.ObserveDispatchEvents(t, events)
	if scenario.wantDispatches > 0 && len(dispatches) != scenario.wantDispatches {
		t.Fatalf("%s dispatch observations = %#v, want %d", scenario.name, dispatches, scenario.wantDispatches)
	}
	if len(dispatches) == 0 {
		t.Fatalf("%s has no dispatch observation", scenario.name)
	}
	dispatchIDs := make([]string, 0, len(dispatches))
	for _, dispatch := range dispatches {
		if dispatch.DispatchID == "" {
			t.Fatalf("%s dispatch identity is empty", scenario.name)
		}
		if !support.DispatchObservationIncludesWork(dispatch, scenario.workID) {
			t.Fatalf("%s dispatch %q omitted Work %q: %#v", scenario.name, dispatch.DispatchID, scenario.workID, dispatch)
		}
		if dispatch.Response == nil || dispatch.Response.Outcome != scenario.wantOutcome {
			t.Fatalf("%s dispatch %q response = %#v, want outcome %q", scenario.name, dispatch.DispatchID, dispatch.Response, scenario.wantOutcome)
		}
		if scenario.wantFailure != "" && (dispatch.Response.FailureDetail == nil || !strings.Contains(dispatch.Response.FailureDetail.Message, scenario.wantFailure)) {
			t.Fatalf("%s dispatch %q failure detail = %#v, want %q", scenario.name, dispatch.DispatchID, dispatch.Response.FailureDetail, scenario.wantFailure)
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

func assertCodexGoldenCommand(
	t *testing.T,
	router *codexCommandRouter,
	scenario codexGoldenScenario,
) {
	t.Helper()

	requests := scenario.runner.Requests()
	if scenario.wantProviderCalls > 0 && len(requests) != scenario.wantProviderCalls {
		t.Fatalf("%s routed provider calls = %d, want %d; requests=%#v", scenario.name, len(requests), scenario.wantProviderCalls, requests)
	}
	if len(requests) == 0 {
		t.Fatalf("%s routed no provider calls", scenario.name)
	}
	routed := router.callsFor(scenario.factoryDir)
	if len(routed) != len(requests) {
		t.Fatalf("%s immutable route calls = %d, runner calls = %d", scenario.name, len(routed), len(requests))
	}
	for index, routedCall := range routed {
		request := routedCall.request
		if request.WorkDir != requests[index].WorkDir {
			t.Fatalf("%s router WorkDir = %q, runner WorkDir = %q", scenario.name, request.WorkDir, requests[index].WorkDir)
		}
		if request.Command != codexConductorProcessCommand {
			t.Fatalf("%s command = %q, want codex", scenario.name, request.Command)
		}
		if request.WorkDir != scenario.factoryDir {
			t.Fatalf("%s command WorkDir = %q, want scenario Factory directory %q", scenario.name, request.WorkDir, scenario.factoryDir)
		}
		if !containsArgPair(request.Args, "--model", scenario.model) || !containsArg(request.Args, "exec") || !containsArg(request.Args, "--json") {
			t.Fatalf("%s args = %#v, want codex exec --json --model %s invocation", scenario.name, request.Args, scenario.model)
		}
	}
}

func assertCodexGoldenProviderSession(
	t *testing.T,
	scenario codexGoldenScenario,
	events []factoryapi.FactoryEvent,
) string {
	t.Helper()
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
		if got == scenario.providerSessionID {
			return got
		}
	}
	t.Fatalf("%s missing Provider Session identity %q", scenario.name, scenario.providerSessionID)
	return ""
}

func assertCodexGoldenEventScope(
	t *testing.T,
	scenario codexGoldenScenario,
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
}

func assertCodexGoldenResponseEvents(
	t *testing.T,
	scenario codexGoldenScenario,
	sessionID string,
	events []factoryapi.FactoryResponseEvent,
) []string {
	t.Helper()
	if len(events) == 0 {
		t.Fatalf("%s response-event stream is empty", scenario.name)
	}
	ids := make([]string, 0, len(events))
	seen := make(map[string]struct{}, len(events))
	var previousSequence int64
	for index, event := range events {
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
		if event.ProviderSessionRef != nil && *event.ProviderSessionRef != scenario.providerSessionID {
			t.Fatalf("%s response event[%d] Provider Session ref = %q, want %q", scenario.name, index, *event.ProviderSessionRef, scenario.providerSessionID)
		}
		previousSequence = event.Sequence
		ids = append(ids, event.EventId)
	}
	return ids
}

func readCodexGoldenResponseEvents(
	t *testing.T,
	stream *support.FactoryResponseEventStream,
	scenario codexGoldenScenario,
	timeout time.Duration,
) []factoryapi.FactoryResponseEvent {
	t.Helper()
	want := len(scenario.loaded.Expected.ResponseEvents)
	if want == 0 {
		t.Fatalf("%s golden response-event fixture is empty", scenario.name)
	}
	deadline := time.Now().Add(timeout)
	events := make([]factoryapi.FactoryResponseEvent, 0, want)
	for len(events) < want {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			t.Fatalf("timed out waiting for %d %s golden response events; got %d", want, scenario.name, len(events))
		}
		result := stream.TryNextFrameResult(remaining)
		if result.Outcome != support.FactoryResponseEventStreamOutcomeFrame {
			t.Fatalf("%s golden response stream ended before %d events: %s", scenario.name, want, result.Diagnostic())
		}
		events = append(events, result.Frame.Event)
	}
	return events
}

func drainCodexGoldenResponseEvents(
	t *testing.T,
	stream *support.FactoryResponseEventStream,
	scenario codexGoldenScenario,
) []factoryapi.FactoryResponseEvent {
	t.Helper()
	var extra []factoryapi.FactoryResponseEvent
	for {
		result := stream.TryNextFrameResult(time.Nanosecond)
		if result.Outcome == support.FactoryResponseEventStreamOutcomeFrame {
			extra = append(extra, result.Frame.Event)
			continue
		}
		if result.Outcome != support.FactoryResponseEventStreamOutcomeEOF &&
			result.Outcome != support.FactoryResponseEventStreamOutcomeCanceled {
			t.Fatalf("%s response stream drain failed: %s", scenario.name, result.Diagnostic())
		}
		return extra
	}
}
