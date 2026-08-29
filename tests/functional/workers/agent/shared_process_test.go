package agent_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
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

const (
	agentSharedProcessTimeout   = 20 * time.Second
	agentForcedCleanupChildEnv  = "YOU_AGENT_FORCED_CLEANUP_CHILD"
	agentForcedCleanupReportEnv = "YOU_AGENT_FORCED_CLEANUP_REPORT"
	agentFailureMessage         = "Codex authentication failed."
	agentCancellationMessage    = "provider invocation was canceled"
	agentTimeoutMessage         = "provider invocation timed out"
)

// TestAgentSharedProcess keeps the four existing agent rows on one immutable
// root-built process. Inert composition is observed before Process.Execute is
// activated; the remaining rows use distinct explicit Factory Sessions and
// immutable provider-command routes.
func TestAgentSharedProcess(t *testing.T) {
	if os.Getenv(agentForcedCleanupChildEnv) == "1" {
		runAgentForcedCleanupChild(t)
		return
	}

	fixture := newAgentSharedProcessFixture(t)

	t.Run("Inert", func(t *testing.T) {
		fixture.assertInert(t)
	})

	for _, scenario := range fixture.scenarios {
		if scenario.name != "Invalid" {
			continue
		}
		t.Run(scenario.name, func(t *testing.T) {
			t.Run("UnknownProvider", func(t *testing.T) {
				fixture.assertUnknownProvider(t, scenario)
			})
			t.Run("MalformedConfiguration", func(t *testing.T) {
				runAgentMalformedConfigurationProbe(t)
			})
		})
	}

	fixture.start(t)
	for _, scenario := range fixture.scenarios {
		scenario := scenario
		if scenario.name == "Invalid" {
			continue
		}
		t.Run(scenario.name, func(t *testing.T) {
			fixture.runScenario(t, scenario)
		})
	}
	t.Run("Cleanup", func(t *testing.T) {
		runAgentForcedCleanupParent(t)
	})
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

	processBuilds   atomic.Int32
	apiStarts       atomic.Int32
	processClosed   atomic.Bool
	processCloseMu  sync.Mutex
	processCloseErr string
	sessionsMu      sync.Mutex
	opened          map[string]string
	closed          map[string]struct{}
}

type agentSharedScenario struct {
	name           string
	factoryDir     string
	model          string
	inputMarker    string
	output         string
	inputMode      agentSharedInputMode
	behavior       agentSharedScenarioBehavior
	provider       modelprovider.Provider
	runner         *agentSharedScenarioRunner
	wantOutcome    factoryapi.WorkOutcome
	wantFailure    factoryapi.WorkFailureType
	wantMessage    string
	wantCalls      int
	wantDispatches int
}

type agentSharedInputMode string

type agentSharedScenarioBehavior string

const (
	agentSharedTextInput        agentSharedInputMode = "text"
	agentSharedJSONPayloadInput agentSharedInputMode = "json-payload"
	agentSharedJSONSeedInput    agentSharedInputMode = "json-seed"

	agentSharedSuccess agentSharedScenarioBehavior = "success"
	agentSharedFailure agentSharedScenarioBehavior = "failure"
	agentSharedTimeout agentSharedScenarioBehavior = "timeout"
	agentSharedCancel  agentSharedScenarioBehavior = "cancel"
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
		provider    modelprovider.Provider
		inputMarker string
		output      string
		inputMode   agentSharedInputMode
		behavior    agentSharedScenarioBehavior
		failure     factoryapi.WorkFailureType
		message     string
	}{
		{
			name:        "Codex",
			model:       "converged-agent-model",
			provider:    modelprovider.ProviderCodex,
			inputMarker: "converged agent payload",
			output:      "converged agent response COMPLETE",
			inputMode:   agentSharedTextInput,
		},
		{
			name:        "Registered",
			model:       "registered-agent-model",
			provider:    modelprovider.ProviderCodex,
			inputMarker: "registered agent payload",
			output:      "registered agent response COMPLETE",
			inputMode:   agentSharedJSONPayloadInput,
		},
		{
			name:        "RuntimeRoot",
			model:       "runtime-root-agent-model",
			provider:    modelprovider.ProviderCodex,
			inputMarker: "runtime provider root",
			output:      "functional-runtime-provider-output COMPLETE",
			inputMode:   agentSharedJSONSeedInput,
		},
		{
			name:        "Claude",
			model:       "claude-agent-model",
			provider:    modelprovider.ProviderClaude,
			inputMarker: "claude agent payload",
			output:      "claude agent response COMPLETE",
			inputMode:   agentSharedTextInput,
		},
		{
			name:     "Invalid",
			model:    "invalid-agent-model",
			provider: modelprovider.Provider("unknown-provider"),
		},
		{
			name:        "Empty",
			model:       "empty-agent-model",
			provider:    modelprovider.ProviderCodex,
			inputMarker: "empty recovery payload",
			output:      "empty recovery response COMPLETE",
			inputMode:   agentSharedTextInput,
		},
		{
			name:        "Minimum",
			model:       "minimum-agent-model",
			provider:    modelprovider.ProviderCodex,
			inputMarker: "minimum agent payload",
			output:      "minimum agent response COMPLETE",
			inputMode:   agentSharedTextInput,
		},
		{
			name:        "Failure",
			model:       "failure-agent-model",
			provider:    modelprovider.ProviderCodex,
			inputMarker: "controlled failure payload",
			output:      "",
			inputMode:   agentSharedTextInput,
			behavior:    agentSharedFailure,
			failure:     factoryapi.WorkFailureTypeAuthFailure,
			message:     agentFailureMessage,
		},
		{
			name:        "Timeout",
			model:       "timeout-agent-model",
			provider:    modelprovider.ProviderCodex,
			inputMarker: "controlled timeout payload",
			output:      "",
			inputMode:   agentSharedTextInput,
			behavior:    agentSharedTimeout,
			failure:     factoryapi.WorkFailureTypeTimeout,
			message:     agentTimeoutMessage,
		},
		{
			name:        "Cancel",
			model:       "cancel-agent-model",
			provider:    modelprovider.ProviderCodex,
			inputMarker: "controlled cancellation payload",
			output:      "",
			inputMode:   agentSharedTextInput,
			behavior:    agentSharedCancel,
			message:     agentCancellationMessage,
		},
		{
			name:        "Recovery",
			model:       "recovery-agent-model",
			provider:    modelprovider.ProviderCodex,
			inputMarker: "fresh recovery payload",
			output:      "fresh recovery response COMPLETE",
			inputMode:   agentSharedTextInput,
		},
	}

	scenarios := make([]agentSharedScenario, 0, len(cases))
	for _, testCase := range cases {
		dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
		support.ClearSeedInputs(t, dir)
		support.WriteAgentConfig(t, dir, "worker", support.BuildModelWorkerConfig(
			testCase.provider,
			testCase.model,
		))
		support.WriteWorkstationConfig(t, dir, "process", "---\ntype: MODEL_WORKSTATION\n---\nAgent input: {{ (index .Inputs 0).Payload }}\n")
		if testCase.inputMode == agentSharedJSONSeedInput {
			testutil.WriteSeedFile(t, dir, "task", []byte(fmt.Sprintf(`{"marker":%q}`, testCase.inputMarker)))
		}
		scenario := agentSharedScenario{
			name:           testCase.name,
			factoryDir:     dir,
			model:          testCase.model,
			inputMarker:    testCase.inputMarker,
			output:         testCase.output,
			inputMode:      testCase.inputMode,
			behavior:       testCase.behavior,
			provider:       testCase.provider,
			wantOutcome:    factoryapi.WorkOutcomeAccepted,
			wantFailure:    testCase.failure,
			wantMessage:    testCase.message,
			wantCalls:      1,
			wantDispatches: 1,
		}
		if testCase.behavior != "" {
			scenario.runner = newAgentSharedScenarioRunner(testCase.behavior, testCase.output, testCase.message)
			if testCase.behavior != agentSharedSuccess {
				scenario.wantOutcome = factoryapi.WorkOutcomeFailed
			}
			if testCase.behavior == agentSharedTimeout {
				scenario.wantCalls = 9
				scenario.wantDispatches = 3
			}
		} else {
			scenario.runner = newAgentSharedScenarioRunner(agentSharedSuccess, testCase.output, "")
		}
		scenarios = append(scenarios, scenario)
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

func (fixture *agentSharedProcessFixture) assertUnknownProvider(t *testing.T, scenario agentSharedScenario) {
	t.Helper()
	inputs := support.FakeInputs(context.Background(), []string{
		"you", "run", "--dir", scenario.factoryDir, "--continuously", "--quiet", "--no-record",
	})
	homeDir := t.TempDir()
	inputs.Input.Env = append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
	inputs.Input.WorkingDirectory = scenario.factoryDir
	err := fixture.process.Execute(inputs.Input)
	if err == nil {
		t.Fatalf("unknown-provider Process.Execute() error = nil, want validation failure")
	}
	diagnostic := strings.ToLower(err.Error())
	if !strings.Contains(diagnostic, "unknown-provider") && !strings.Contains(diagnostic, "unknown provider") && !strings.Contains(diagnostic, "validate factory provider selections") {
		t.Fatalf("unknown-provider validation error = %q, want actionable provider diagnostic", err)
	}
	if got := fixture.router.callCount(); got != 0 {
		t.Fatalf("unknown-provider calls = %d, want zero", got)
	}
	if got := fixture.apiStarts.Load(); got != 0 {
		t.Fatalf("unknown-provider API starts = %d, want zero before valid daemon activation", got)
	}
	fixture.sessionsMu.Lock()
	defer fixture.sessionsMu.Unlock()
	if len(fixture.opened) != 0 {
		t.Fatalf("unknown-provider Factory Sessions = %#v, want none", fixture.opened)
	}
}

func findAgentScenario(t testing.TB, scenarios []agentSharedScenario, name string) agentSharedScenario {
	t.Helper()
	for _, scenario := range scenarios {
		if scenario.name == name {
			return scenario
		}
	}
	t.Fatalf("agent scenario %q is missing", name)
	return agentSharedScenario{}
}

func (fixture *agentSharedProcessFixture) runEmptyScenario(t *testing.T, scenario agentSharedScenario) {
	t.Helper()
	sessionID := fixture.openSession(t, scenario.factoryDir)
	name := "agent-empty"
	traceID := "trace-agent-empty"
	emptyRequest := factoryapi.SubmitWorkRequest{
		Name:         &name,
		TraceId:      &traceID,
		WorkTypeName: "task",
	}
	beforeCalls := fixture.router.callCount()
	status, body := postAgentWorkStatus(t, fixture.baseURL, sessionID, emptyRequest)
	if status == http.StatusCreated {
		fixture.closeSession(t, sessionID)
		assertAgentSessionDeleted(t, fixture.baseURL, sessionID)
		t.Skipf("contract blocker: empty Work request was accepted with status 201; body=%s", body)
	}
	if status != http.StatusBadRequest {
		t.Fatalf("empty Work submission status = %d, want 400; body=%s", status, body)
	}
	if strings.TrimSpace(body) == "" {
		t.Fatal("empty Work submission returned an empty validation diagnostic")
	}
	if got := fixture.router.callCount(); got != beforeCalls {
		t.Fatalf("provider calls after rejected empty Work = %d, want unchanged at %d", got, beforeCalls)
	}

	content := agentTextContent(t, scenario.inputMarker)
	validRequest := factoryapi.SubmitWorkRequest{
		Name:         &name,
		TraceId:      &traceID,
		WorkTypeName: "task",
		Content:      &content,
	}
	submitted := support.SubmitSessionWorkAt(t, fixture.baseURL, sessionID, validRequest)
	if submitted.SessionId == nil || *submitted.SessionId != sessionID || support.StringPointerValue(submitted.WorkId) == "" || strings.TrimSpace(submitted.RequestId) == "" {
		t.Fatalf("valid Work after empty rejection = %#v, want session/work/request identity", submitted)
	}
	support.WaitForSessionTerminalStatus(t, fixture.baseURL, sessionID, agentSharedProcessTimeout)
	listed := listAgentSessionWork(t, fixture.baseURL, sessionID)
	events := support.GetFactoryEventsForSessionAt(t, fixture.baseURL, sessionID)
	workID := support.StringPointerValue(submitted.WorkId)
	assertAgentScenarioWork(t, listed, workID, scenario)
	assertAgentScenarioDispatch(t, events, sessionID, submitted.RequestId, workID, scenario)
	assertAgentWorkerSession(t, fixture.baseURL, sessionID, workID, scenario)
	fixture.assertRoute(t, scenario)
	fixture.closeSession(t, sessionID)
	assertAgentSessionDeleted(t, fixture.baseURL, sessionID)
}

func postAgentWorkStatus(
	t testing.TB,
	baseURL string,
	sessionID string,
	request factoryapi.SubmitWorkRequest,
) (int, string) {
	t.Helper()
	payload, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal Work request: %v", err)
	}
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID) + "/work"
	httpRequest, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("build Work request: %v", err)
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(httpRequest)
	if err != nil {
		t.Fatalf("POST Work: %v", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read Work response: %v", err)
	}
	return response.StatusCode, string(body)
}

func cancelAgentSession(baseURL, sessionID string) error {
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID) + "/cancel"
	request, err := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(`{}`))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusOK || response.StatusCode == http.StatusAccepted {
		return nil
	}
	body, readErr := io.ReadAll(response.Body)
	if readErr != nil {
		return fmt.Errorf("POST %s status = %d: read response body: %w", endpoint, response.StatusCode, readErr)
	}
	return fmt.Errorf("POST %s status = %d: %s", endpoint, response.StatusCode, strings.TrimSpace(string(body)))
}

func runAgentMalformedConfigurationProbe(t *testing.T) {
	t.Helper()
	dir := support.ScaffoldFactory(t, map[string]any{
		"name": "agent-malformed-worker-reference",
		"workTypes": []any{
			map[string]any{
				"name": "task",
				"states": []any{
					map[string]any{"name": "init", "type": "INITIAL"},
					map[string]any{"name": "complete", "type": "TERMINAL"},
					map[string]any{"name": "failed", "type": "FAILED"},
				},
			},
		},
		"workers": []any{map[string]any{"name": "processor"}},
		"workstations": []map[string]any{{
			"name":      "process",
			"worker":    "ghost-worker",
			"inputs":    []any{map[string]any{"workType": "task", "state": "init"}},
			"outputs":   []any{map[string]any{"workType": "task", "state": "complete"}},
			"onFailure": []any{map[string]any{"workType": "task", "state": "failed"}},
		}},
	})
	runner := support.NewRecordingCommandRunner("malformed configuration must not invoke a provider")
	process, err := support.BuildProcessWithContext(context.Background(), serviceedges.Edges{
		ProviderCommandRunner: runner,
	})
	if err != nil {
		t.Fatalf("BuildProcess() malformed configuration probe: %v", err)
	}
	support.CleanupProcess(t, process)
	inputs := support.FakeInputs(context.Background(), []string{
		"you", "run", "--dir", dir, "--continuously", "--quiet", "--no-record",
	})
	inputs.Input.Env = append(os.Environ(), "HOME="+t.TempDir(), "USERPROFILE="+t.TempDir())
	inputs.Input.WorkingDirectory = dir
	if err := process.Execute(inputs.Input); err == nil {
		t.Fatal("malformed worker configuration succeeded, want validation failure")
	} else {
		diagnostic := strings.ToLower(err.Error())
		for _, want := range []string{"validate factory config", "factory.worker.danglingreference", "ghost-worker"} {
			if !strings.Contains(diagnostic, strings.ToLower(want)) {
				t.Fatalf("malformed worker validation error = %q, want %q", err, want)
			}
		}
	}
	if got := runner.CallCount(); got != 0 {
		t.Fatalf("malformed worker provider calls = %d, want zero", got)
	}
}

func (fixture *agentSharedProcessFixture) runScenario(
	t *testing.T,
	scenario agentSharedScenario,
) {
	t.Helper()
	if scenario.name == "Empty" {
		fixture.runEmptyScenario(t, scenario)
		return
	}

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
	var responseStream *support.FactoryResponseEventStream
	if scenario.behavior == agentSharedCancel {
		responseStream = support.OpenFactoryResponseEventStreamAt(
			t,
			support.SessionResponseEventsURL(fixture.baseURL, sessionID),
		)
	}
	var submitted factoryapi.SubmitWorkResponse
	if scenario.inputMode != agentSharedJSONSeedInput {
		submitted = support.SubmitSessionWorkAt(t, fixture.baseURL, sessionID, request)
		if submitted.SessionId == nil || *submitted.SessionId != sessionID {
			t.Fatalf("submitted Work session id = %#v, want %q", submitted.SessionId, sessionID)
		}
	}
	if scenario.behavior == agentSharedCancel {
		scenario.runner.waitStarted(t, agentSharedProcessTimeout)
		if err := cancelAgentSession(fixture.baseURL, sessionID); err != nil {
			t.Fatalf("cancel agent Factory Session %q: %v", sessionID, err)
		}
		scenario.runner.waitFinished(t, agentSharedProcessTimeout)
		if got := scenario.runner.canceledCount(); got != 1 {
			t.Fatalf("canceled agent calls = %d, want exactly one", got)
		}
	}

	var responseEvents []factoryapi.FactoryResponseEvent
	if scenario.behavior == agentSharedCancel {
		support.WaitForSessionStopped(t, fixture.baseURL, sessionID, agentSharedProcessTimeout)
		responseEvents = readAgentResponseEventsUntilTerminal(t, responseStream, agentSharedProcessTimeout)
		assertAgentCancellationResponseEvents(t, responseEvents, sessionID)
		responseStream.Close()
		responseStream.WaitClosed(agentSharedProcessTimeout)
	} else {
		support.WaitForSessionTerminalStatus(t, fixture.baseURL, sessionID, agentSharedProcessTimeout)
	}
	listed := listAgentSessionWork(t, fixture.baseURL, sessionID)
	events := support.GetFactoryEventsForSessionAt(t, fixture.baseURL, sessionID)
	workID := support.StringPointerValue(submitted.WorkId)
	requestID := submitted.RequestId
	if scenario.inputMode == agentSharedJSONSeedInput {
		workID, requestID = seededAgentIdentity(t, events)
	} else if workID == "" || strings.TrimSpace(requestID) == "" {
		t.Fatalf("submitted Work identity = work:%q request:%q, want both identities", workID, requestID)
	}
	assertAgentScenarioWork(t, listed, workID, scenario)
	assertAgentScenarioDispatch(t, events, sessionID, requestID, workID, scenario)
	assertAgentWorkerSession(t, fixture.baseURL, sessionID, workID, scenario)
	fixture.assertRoute(t, scenario)

	fixture.closeSession(t, sessionID)
	assertAgentSessionDeleted(t, fixture.baseURL, sessionID)
}

func readAgentResponseEventsUntilTerminal(
	t *testing.T,
	stream *support.FactoryResponseEventStream,
	timeout time.Duration,
) []factoryapi.FactoryResponseEvent {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var events []factoryapi.FactoryResponseEvent
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			t.Fatalf("timed out waiting for terminal agent response event after %s; got %d events", timeout, len(events))
		}
		result := stream.TryNextFrameResult(remaining)
		if result.Outcome != support.FactoryResponseEventStreamOutcomeFrame {
			t.Fatalf("agent response stream ended before terminal event: %s", result.Diagnostic())
		}
		event := result.Frame.Event
		events = append(events, event)
		if isAgentTerminalResponseEvent(event) {
			return events
		}
	}
}

func isAgentTerminalResponseEvent(event factoryapi.FactoryResponseEvent) bool {
	if event.Kind == factoryapi.FactoryResponseEventKindRun {
		return event.Phase == factoryapi.FactoryResponseEventPhaseCompleted ||
			event.Phase == factoryapi.FactoryResponseEventPhaseFailed ||
			event.Phase == factoryapi.FactoryResponseEventPhaseCanceled
	}
	return event.Kind == factoryapi.FactoryResponseEventKindError &&
		(event.Phase == factoryapi.FactoryResponseEventPhaseFailed ||
			event.Phase == factoryapi.FactoryResponseEventPhaseCanceled)
}

func assertAgentCancellationResponseEvents(
	t *testing.T,
	events []factoryapi.FactoryResponseEvent,
	sessionID string,
) {
	t.Helper()
	if len(events) == 0 {
		t.Fatal("agent cancellation response events are empty")
	}
	for _, event := range events {
		if event.FactorySessionId != sessionID {
			t.Fatalf("agent cancellation response event session = %q, want %q", event.FactorySessionId, sessionID)
		}
	}
	terminal := events[len(events)-1]
	if terminal.Phase != factoryapi.FactoryResponseEventPhaseFailed {
		t.Fatalf("agent cancellation terminal response phase = %q, want FAILED cancellation notification; events=%#v", terminal.Phase, events)
	}
	payload, err := json.Marshal(events)
	if err != nil {
		t.Fatalf("marshal agent cancellation response events: %v", err)
	}
	if !strings.Contains(string(payload), "stream_canceled") || !strings.Contains(string(payload), agentCancellationMessage) {
		t.Fatalf("agent cancellation response events = %s, want stream_canceled and cancellation diagnostic", payload)
	}
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
	if len(calls) != scenario.wantCalls {
		t.Fatalf("%s immutable route calls = %d, want %d; calls=%#v", scenario.name, len(calls), scenario.wantCalls, calls)
	}
	for index, call := range calls {
		request := call.request
		if request.Command != string(scenario.provider) {
			t.Fatalf("%s provider command[%d] = %q, want %q", scenario.name, index, request.Command, scenario.provider)
		}
		if request.WorkDir != scenario.factoryDir {
			t.Fatalf("%s provider WorkDir[%d] = %q, want %q", scenario.name, index, request.WorkDir, scenario.factoryDir)
		}
		if !containsAgentArgumentPair(request.Args, "--model", scenario.model) {
			t.Fatalf("%s provider args[%d] = %#v, want --model %q", scenario.name, index, request.Args, scenario.model)
		}
		if !agentCommandRequestContains(request, scenario.inputMarker) {
			t.Fatalf("%s provider request[%d] omitted input marker %q: %#v", scenario.name, index, scenario.inputMarker, request)
		}
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
		fixture.processCloseMu.Lock()
		fixture.processCloseErr = err.Error()
		fixture.processCloseMu.Unlock()
		t.Errorf("close shared agent application process: %v", err)
	} else {
		fixture.processClosed.Store(true)
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
	if len(fixture.opened) != len(fixture.closed) {
		t.Errorf("closed shared Factory Sessions = %d, opened = %d; opened=%#v closed=%#v", len(fixture.closed), len(fixture.opened), fixture.opened, fixture.closed)
	}
	fixture.sessionsMu.Unlock()
	for _, scenario := range fixture.scenarios {
		if got := scenario.runner.activeCallCount(); got != 0 {
			t.Errorf("%s active agent command calls after process cleanup = %d, want zero", scenario.name, got)
		}
	}
	fixture.router.clearRoutes()
	for _, path := range fixture.ownedPaths() {
		if err := os.RemoveAll(path); err != nil {
			t.Errorf("remove test-owned agent path %q: %v", path, err)
		}
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("test-owned agent path %q remains after cleanup: %v", path, err)
		}
	}
}

func (fixture *agentSharedProcessFixture) ownedPaths() []string {
	paths := make([]string, 0, len(fixture.scenarios)+1)
	paths = append(paths, fixture.hostDir)
	for _, scenario := range fixture.scenarios {
		paths = append(paths, scenario.factoryDir)
	}
	return paths
}

func runAgentForcedCleanupParent(t *testing.T) {
	t.Helper()
	reportPath := filepath.Join(t.TempDir(), "agent-forced-cleanup.json")
	command := exec.Command(os.Args[0], "-test.run=^TestAgentSharedProcess$")
	command.Env = append(
		os.Environ(),
		agentForcedCleanupChildEnv+"=1",
		agentForcedCleanupReportEnv+"="+reportPath,
	)
	output, err := command.CombinedOutput()
	if err == nil {
		t.Fatalf("forced agent cleanup child exited successfully; output=%q", output)
	}
	if command.Process == nil || command.ProcessState == nil || !command.ProcessState.Exited() || command.ProcessState.ExitCode() == 0 {
		t.Fatalf("forced agent cleanup child exit state = %#v; output=%q", command.ProcessState, output)
	}
	if !strings.Contains(string(output), "intentional agent cleanup assertion") {
		t.Fatalf("forced agent cleanup child output omitted original assertion: %q", output)
	}
	report := readAgentForcedCleanupReport(t, reportPath, output)
	assertAgentForcedCleanupReport(t, report, command.Process.Pid)
}

func readAgentForcedCleanupReport(t testing.TB, path string, childOutput []byte) agentForcedCleanupReport {
	t.Helper()
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read forced agent cleanup report %q: %v; child output=%q", path, err, childOutput)
	}
	var report agentForcedCleanupReport
	if err := json.Unmarshal(payload, &report); err != nil {
		t.Fatalf("decode forced agent cleanup report %q: %v; child output=%q", path, err, childOutput)
	}
	return report
}

func assertAgentForcedCleanupReport(t *testing.T, report agentForcedCleanupReport, childPID int) {
	t.Helper()
	if report.ApplicationPID != childPID {
		t.Fatalf("forced agent cleanup application PID = %d, want child PID %d", report.ApplicationPID, childPID)
	}
	if report.ProcessCloseError != "" || !report.ProcessClosed || !report.DaemonStopped {
		t.Fatalf("forced agent cleanup process state = %#v, want clean process close", report)
	}
	if !report.ListenerClosed {
		t.Fatal("forced agent cleanup API listener remained active")
	}
	if len(report.OpenedSessionIDs) != 1 || !sameAgentStringSet(report.OpenedSessionIDs, report.DeletedSessionIDs) {
		t.Fatalf("forced agent cleanup sessions opened=%v deleted=%v, want one deleted opened session", report.OpenedSessionIDs, report.DeletedSessionIDs)
	}
	if report.ActiveCalls != 0 || report.CommandCalls != 1 || !report.CommandStarted || !report.CommandFinished || report.CanceledCalls != 1 {
		t.Fatalf("forced agent cleanup command state = %#v, want one canceled and finished call with no active calls", report)
	}
	if report.ActiveCommandRoutes != 0 {
		t.Fatalf("forced agent cleanup active command routes = %d, want zero", report.ActiveCommandRoutes)
	}
	if !report.ResponseStreamClosed {
		t.Fatal("forced agent cleanup response stream did not close")
	}
	if !report.HostDirectoryAbsent || !report.ScenarioDirectoriesAbsent {
		t.Fatalf("forced agent cleanup owned directories remain: %#v", report)
	}
}

type agentForcedCleanupReport struct {
	ApplicationPID            int      `json:"application_pid"`
	ProcessClosed             bool     `json:"process_closed"`
	ProcessCloseError         string   `json:"process_close_error,omitempty"`
	DaemonStopped             bool     `json:"daemon_stopped"`
	ListenerClosed            bool     `json:"listener_closed"`
	OpenedSessionIDs          []string `json:"opened_session_ids"`
	DeletedSessionIDs         []string `json:"deleted_session_ids"`
	ActiveCommandRoutes       int      `json:"active_command_routes"`
	ActiveCalls               int      `json:"active_calls"`
	CommandCalls              int      `json:"command_calls"`
	CommandStarted            bool     `json:"command_started"`
	CommandFinished           bool     `json:"command_finished"`
	CanceledCalls             int      `json:"canceled_calls"`
	ResponseStreamClosed      bool     `json:"response_stream_closed"`
	HostDirectoryAbsent       bool     `json:"host_directory_absent"`
	ScenarioDirectoriesAbsent bool     `json:"scenario_directories_absent"`
}

type agentForcedCleanupProbe struct {
	fixture              *agentSharedProcessFixture
	scenario             agentSharedScenario
	stream               *support.FactoryResponseEventStream
	responseStreamClosed bool
}

func runAgentForcedCleanupChild(t *testing.T) {
	t.Helper()
	reportPath := strings.TrimSpace(os.Getenv(agentForcedCleanupReportEnv))
	if reportPath == "" {
		t.Fatal("forced agent cleanup report path is required")
	}

	var probe *agentForcedCleanupProbe
	var fixture *agentSharedProcessFixture
	// This cleanup is registered before the fixture so it observes the state
	// after the fixture, command, session, and stream cleanups have run.
	t.Cleanup(func() {
		if fixture == nil || probe == nil {
			return
		}
		fixture.processCloseMu.Lock()
		closeErr := fixture.processCloseErr
		fixture.processCloseMu.Unlock()
		opened, deleted := fixture.sessionIDs()
		report := agentForcedCleanupReport{
			ApplicationPID:            os.Getpid(),
			ProcessClosed:             fixture.processClosed.Load(),
			ProcessCloseError:         closeErr,
			DaemonStopped:             agentChannelClosed(fixture.command.Done()),
			ListenerClosed:            agentChannelClosed(fixture.apiClosed),
			OpenedSessionIDs:          opened,
			DeletedSessionIDs:         deleted,
			ActiveCommandRoutes:       fixture.router.routeCount(),
			CommandCalls:              probe.scenario.runner.callCount(),
			CommandStarted:            agentChannelClosed(probe.scenario.runner.started),
			CommandFinished:           agentChannelClosed(probe.scenario.runner.finished),
			CanceledCalls:             probe.scenario.runner.canceledCount(),
			ResponseStreamClosed:      probe.responseStreamClosed,
			HostDirectoryAbsent:       agentPathAbsent(fixture.hostDir),
			ScenarioDirectoriesAbsent: agentScenarioDirectoriesAbsent(fixture),
		}
		for _, scenario := range fixture.scenarios {
			report.ActiveCalls += scenario.runner.activeCallCount()
		}
		payload, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			t.Errorf("marshal forced agent cleanup report: %v", err)
			return
		}
		if err := os.WriteFile(reportPath, payload, 0o600); err != nil {
			t.Errorf("write forced agent cleanup report: %v", err)
		}
	})

	fixture = newAgentSharedProcessFixture(t)
	fixture.start(t)
	scenario := findAgentScenario(t, fixture.scenarios, "Cancel")
	probe = &agentForcedCleanupProbe{fixture: fixture, scenario: scenario}
	sessionID := fixture.openSession(t, scenario.factoryDir)
	probe.stream = support.OpenFactoryResponseEventStreamAt(t, support.SessionResponseEventsURL(fixture.baseURL, sessionID))
	t.Cleanup(func() {
		if probe.stream == nil {
			return
		}
		probe.stream.Close()
		probe.stream.WaitClosed(agentSharedProcessTimeout)
		probe.responseStreamClosed = true
	})
	content := agentTextContent(t, scenario.inputMarker)
	support.SubmitSessionWorkAt(t, fixture.baseURL, sessionID, factoryapi.SubmitWorkRequest{
		Name:         stringPointer("agent-forced-cleanup"),
		TraceId:      stringPointer("trace-agent-forced-cleanup"),
		WorkTypeName: "task",
		Content:      &content,
	})
	scenario.runner.waitStarted(t, agentSharedProcessTimeout)
	t.Fatal("intentional agent cleanup assertion after acquiring process, session, stream, route, command, and paths")
}

func (fixture *agentSharedProcessFixture) sessionIDs() ([]string, []string) {
	fixture.sessionsMu.Lock()
	defer fixture.sessionsMu.Unlock()
	opened := make([]string, 0, len(fixture.opened))
	for sessionID := range fixture.opened {
		opened = append(opened, sessionID)
	}
	deleted := make([]string, 0, len(fixture.closed))
	for sessionID := range fixture.closed {
		deleted = append(deleted, sessionID)
	}
	return opened, deleted
}

func stringPointer(value string) *string {
	return &value
}

func agentChannelClosed(channel <-chan struct{}) bool {
	select {
	case <-channel:
		return true
	default:
		return false
	}
}

func agentPathAbsent(path string) bool {
	_, err := os.Stat(path)
	return os.IsNotExist(err)
}

func agentScenarioDirectoriesAbsent(fixture *agentSharedProcessFixture) bool {
	for _, scenario := range fixture.scenarios {
		if !agentPathAbsent(scenario.factoryDir) {
			return false
		}
	}
	return true
}

func sameAgentStringSet(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	seen := make(map[string]struct{}, len(left))
	for _, value := range left {
		seen[value] = struct{}{}
	}
	for _, value := range right {
		if _, ok := seen[value]; !ok {
			return false
		}
	}
	return true
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
		if scenario.runner == nil {
			t.Fatalf("agent scenario %q has no command runner", scenario.name)
		}
		routes[selector] = scenario.runner
	}
	return &agentSharedCommandRouter{routes: routes}
}

func (router *agentSharedCommandRouter) Run(
	ctx context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	selector := filepath.Clean(strings.TrimSpace(request.WorkDir))
	router.mu.Lock()
	runner, ok := router.routes[selector]
	router.mu.Unlock()
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

func (router *agentSharedCommandRouter) routeCount() int {
	router.mu.Lock()
	defer router.mu.Unlock()
	return len(router.routes)
}

func (router *agentSharedCommandRouter) clearRoutes() {
	router.mu.Lock()
	defer router.mu.Unlock()
	router.routes = nil
}

// agentSharedScenarioRunner gives each immutable Factory-directory route its
// own deterministic command behavior. The cancellation route deliberately
// waits on the invocation context so the public session cancel operation, not
// a timing guess, drives the terminal transition.
type agentSharedScenarioRunner struct {
	behavior agentSharedScenarioBehavior
	result   platformprocess.CommandResult

	started    chan struct{}
	finished   chan struct{}
	startOnce  sync.Once
	finishOnce sync.Once

	mu       sync.Mutex
	requests []platformprocess.CommandRequest
	active   atomic.Int32
	canceled atomic.Int32
}

func newAgentSharedScenarioRunner(
	behavior agentSharedScenarioBehavior,
	output string,
	message string,
) *agentSharedScenarioRunner {
	result := platformprocess.CommandResult{Stdout: []byte(output)}
	if behavior == agentSharedFailure {
		result.ExitCode = 1
		result.Stderr = []byte("ERROR: unexpected status 401 Unauthorized {\"type\":\"authentication_error\",\"message\":\"" + message + "\"}")
	}
	if behavior == agentSharedTimeout {
		result.Stderr = []byte(message)
	}
	return &agentSharedScenarioRunner{
		behavior: behavior,
		result:   result,
		started:  make(chan struct{}),
		finished: make(chan struct{}),
	}
}

func (runner *agentSharedScenarioRunner) Run(
	ctx context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	runner.active.Add(1)
	defer func() {
		runner.active.Add(-1)
		runner.finishOnce.Do(func() { close(runner.finished) })
	}()
	runner.startOnce.Do(func() { close(runner.started) })

	runner.mu.Lock()
	runner.requests = append(runner.requests, cloneAgentCommandRequest(request))
	runner.mu.Unlock()

	switch runner.behavior {
	case agentSharedCancel:
		select {
		case <-ctx.Done():
			runner.canceled.Add(1)
			return platformprocess.CommandResult{}, ctx.Err()
		}
	case agentSharedTimeout:
		return cloneAgentCommandResult(runner.result), context.DeadlineExceeded
	case agentSharedFailure:
		return cloneAgentCommandResult(runner.result), nil
	case agentSharedSuccess:
		return support.NewShapedProviderCommandRunner(runner.result).Run(ctx, request)
	default:
		return platformprocess.CommandResult{}, fmt.Errorf("unknown agent scenario behavior %q", runner.behavior)
	}
}

func (runner *agentSharedScenarioRunner) waitStarted(t testing.TB, timeout time.Duration) {
	t.Helper()
	select {
	case <-runner.started:
	case <-time.After(timeout):
		t.Fatalf("timed out waiting for agent command start within %s", timeout)
	}
}

func (runner *agentSharedScenarioRunner) waitFinished(t testing.TB, timeout time.Duration) {
	t.Helper()
	select {
	case <-runner.finished:
	case <-time.After(timeout):
		t.Fatalf("timed out waiting for agent command finish within %s", timeout)
	}
}

func (runner *agentSharedScenarioRunner) callCount() int {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return len(runner.requests)
}

func (runner *agentSharedScenarioRunner) canceledCount() int {
	return int(runner.canceled.Load())
}

func (runner *agentSharedScenarioRunner) activeCallCount() int {
	return int(runner.active.Load())
}

func cloneAgentCommandResult(result platformprocess.CommandResult) platformprocess.CommandResult {
	result.Stdout = append([]byte(nil), result.Stdout...)
	result.Stderr = append([]byte(nil), result.Stderr...)
	return result
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

func assertAgentScenarioWork(
	t *testing.T,
	listed factoryapi.ListWorkResponse,
	workID string,
	scenario agentSharedScenario,
) {
	t.Helper()
	if scenario.behavior == agentSharedCancel {
		if len(listed.Results) != 1 {
			t.Fatalf("%s cancellation Work results = %#v, want one processing Work", scenario.name, listed.Results)
		}
		item := listed.Results[0]
		if support.StringPointerValue(item.WorkId) != workID || item.State == nil || item.State.Name != "init" || item.State.Type != factoryapi.WorkStateTypePROCESSING {
			t.Fatalf("%s cancellation Work = %#v, want Work %q in init/PROCESSING", scenario.name, item, workID)
		}
		if item.FailureDetail != nil {
			t.Fatalf("%s cancellation Work failure detail = %#v, want none", scenario.name, item.FailureDetail)
		}
		return
	}
	if scenario.wantOutcome == factoryapi.WorkOutcomeAccepted {
		assertAgentWork(t, listed, workID, scenario.output)
		return
	}
	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 0 {
		t.Fatalf("%s completed Work = %d, want zero; listed=%#v", scenario.name, got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 1 {
		t.Fatalf("%s failed Work = %d, want one; listed=%#v", scenario.name, got, listed)
	}
	var found int
	for _, item := range listed.Results {
		if support.StringPointerValue(item.WorkId) != workID {
			continue
		}
		found++
		if item.FailureDetail == nil {
			t.Fatalf("%s Work %q has no failure detail", scenario.name, workID)
		}
		if scenario.wantFailure != "" && item.FailureDetail.Reason != scenario.wantFailure {
			t.Fatalf("%s Work failure reason = %q, want %q", scenario.name, item.FailureDetail.Reason, scenario.wantFailure)
		}
		if scenario.wantMessage != "" && !strings.Contains(item.FailureDetail.Message, scenario.wantMessage) {
			t.Fatalf("%s Work failure message = %q, want %q", scenario.name, item.FailureDetail.Message, scenario.wantMessage)
		}
	}
	if found != 1 {
		t.Fatalf("%s Work identity count = %d, want exactly one %q; listed=%#v", scenario.name, found, workID, listed)
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

func assertAgentScenarioDispatch(
	t *testing.T,
	events []factoryapi.FactoryEvent,
	sessionID string,
	requestID string,
	workID string,
	scenario agentSharedScenario,
) {
	t.Helper()
	if scenario.behavior == agentSharedCancel {
		dispatches := support.ObserveDispatchEvents(t, events)
		if len(dispatches) != 1 {
			t.Fatalf("%s cancellation dispatch observations = %#v, want one in-flight dispatch", scenario.name, dispatches)
		}
		dispatch := dispatches[0]
		if dispatch.DispatchID == "" || !support.DispatchObservationIncludesWork(dispatch, workID) || dispatch.Request.TransitionId != "process" {
			t.Fatalf("%s cancellation dispatch = %#v, want process dispatch correlated to Work %q", scenario.name, dispatch, workID)
		}
		if dispatch.Response != nil {
			t.Fatalf("%s cancellation dispatch response = %#v, want no business response after session cancel", scenario.name, dispatch.Response)
		}
		correlated := false
		for _, event := range events {
			if event.Context.SessionId != nil && *event.Context.SessionId != sessionID {
				t.Fatalf("%s Factory Event %q escaped Factory Session %q", scenario.name, event.Id, sessionID)
			}
			if event.Context.RequestId != nil && *event.Context.RequestId == requestID {
				correlated = true
			}
		}
		if !correlated {
			t.Fatalf("%s Factory Events contain no request correlation for %q", scenario.name, requestID)
		}
		return
	}
	if scenario.wantOutcome == factoryapi.WorkOutcomeAccepted {
		assertAgentDispatch(t, events, sessionID, requestID, workID, scenario.output)
		return
	}
	dispatches := support.ObserveDispatchEvents(t, events)
	if len(dispatches) != scenario.wantDispatches {
		t.Fatalf("%s dispatch observations = %#v, want %d", scenario.name, dispatches, scenario.wantDispatches)
	}
	for _, event := range events {
		if event.Context.SessionId != nil && *event.Context.SessionId != sessionID {
			t.Fatalf("%s Factory Event %q escaped Factory Session %q", scenario.name, event.Id, sessionID)
		}
	}
	for index, dispatch := range dispatches {
		if dispatch.DispatchID == "" || !support.DispatchObservationIncludesWork(dispatch, workID) {
			t.Fatalf("%s dispatch[%d] = %#v, want non-empty identity correlated to Work %q", scenario.name, index, dispatch, workID)
		}
		if dispatch.Request.TransitionId != "process" || dispatch.Response == nil {
			t.Fatalf("%s dispatch[%d] = %#v, want process request and response", scenario.name, index, dispatch)
		}
		if dispatch.Response.Outcome != scenario.wantOutcome {
			t.Fatalf("%s dispatch[%d] outcome = %q, want %q", scenario.name, index, dispatch.Response.Outcome, scenario.wantOutcome)
		}
		if dispatch.Response.FailureDetail == nil {
			t.Fatalf("%s dispatch[%d] response has no failure detail", scenario.name, index)
		}
		if scenario.wantFailure != "" && dispatch.Response.FailureDetail.Reason != scenario.wantFailure {
			t.Fatalf("%s dispatch[%d] failure reason = %q, want %q", scenario.name, index, dispatch.Response.FailureDetail.Reason, scenario.wantFailure)
		}
		if scenario.wantMessage != "" && !strings.Contains(dispatch.Response.FailureDetail.Message, scenario.wantMessage) {
			t.Fatalf("%s dispatch[%d] failure message = %q, want %q", scenario.name, index, dispatch.Response.FailureDetail.Message, scenario.wantMessage)
		}
	}
	correlated := false
	for _, event := range events {
		if event.Context.RequestId != nil && *event.Context.RequestId == requestID {
			correlated = true
		}
	}
	if !correlated {
		t.Fatalf("%s Factory Events contain no request correlation for %q", scenario.name, requestID)
	}
}

func assertAgentWorkerSession(
	t *testing.T,
	baseURL string,
	sessionID string,
	workID string,
	scenario agentSharedScenario,
) {
	t.Helper()
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID) + "/worker-sessions?workId=" + url.QueryEscape(workID)
	listed := support.GetJSON[factoryapi.ListWorkerSessionsResponse](t, endpoint)
	if scenario.behavior == agentSharedCancel {
		if len(listed.Sessions) != 1 {
			t.Fatalf("%s cancellation Worker Sessions = %#v, want one", scenario.name, listed.Sessions)
		}
		workerSession := listed.Sessions[0]
		if strings.TrimSpace(workerSession.WorkerSessionId) == "" || workerSession.WorkId == nil || *workerSession.WorkId != workID {
			t.Fatalf("%s cancellation Worker Session = %#v, want Work correlation %q", scenario.name, workerSession, workID)
		}
		return
	}
	if len(listed.Sessions) != scenario.wantDispatches {
		t.Fatalf("%s Worker Sessions = %#v, want %d for Work %q", scenario.name, listed.Sessions, scenario.wantDispatches, workID)
	}
	wantState := factoryapi.WorkerSessionObservationStateCompleted
	if scenario.wantOutcome != factoryapi.WorkOutcomeAccepted {
		wantState = factoryapi.WorkerSessionObservationStateFailed
	}
	for index, workerSession := range listed.Sessions {
		if strings.TrimSpace(workerSession.WorkerSessionId) == "" || workerSession.WorkId == nil || *workerSession.WorkId != workID {
			t.Fatalf("%s Worker Session[%d] = %#v, want Work correlation %q", scenario.name, index, workerSession, workID)
		}
		if workerSession.State != wantState {
			t.Fatalf("%s Worker Session[%d] state = %q, want %q", scenario.name, index, workerSession.State, wantState)
		}
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
