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
	"strings"
	"testing"
	"time"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

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
	status, body := postAgentWorkStatus(t, fixture.baseURL, sessionID, emptyRequest)
	if status != http.StatusCreated {
		t.Fatalf("empty Work submission status = %d, want current characterized 201 acceptance; body=%s", status, body)
	}
	var emptyResponse factoryapi.SubmitWorkResponse
	if err := json.Unmarshal([]byte(body), &emptyResponse); err != nil {
		t.Fatalf("decode accepted empty Work response: %v; body=%s", err, body)
	}
	if !emptyResponse.Accepted {
		t.Fatalf("empty Work response accepted = false; body=%s", body)
	}
	if emptyResponse.SessionId == nil || *emptyResponse.SessionId != sessionID {
		t.Fatalf("accepted empty Work session id = %#v, want %q", emptyResponse.SessionId, sessionID)
	}
	if emptyResponse.Name == nil || *emptyResponse.Name != name || emptyResponse.TraceId != traceID {
		t.Fatalf("accepted empty Work identity = %#v, want name=%q trace=%q", emptyResponse, name, traceID)
	}
	if strings.TrimSpace(emptyResponse.RequestId) == "" || strings.TrimSpace(support.StringPointerValue(emptyResponse.WorkId)) == "" {
		t.Fatalf("accepted empty Work response = %#v, want request and Work identities", emptyResponse)
	}
	content := agentTextContent(t, scenario.inputMarker)
	validRequest := factoryapi.SubmitWorkRequest{
		Name:         &name,
		TraceId:      &traceID,
		WorkTypeName: "task",
		Content:      &content,
	}
	submitted := support.SubmitSessionWorkAt(t, fixture.baseURL, sessionID, validRequest)
	if !submitted.Accepted {
		t.Fatalf("valid Work after accepted empty Work = %#v, want accepted response", submitted)
	}
	if submitted.SessionId == nil || *submitted.SessionId != sessionID || support.StringPointerValue(submitted.WorkId) == "" || strings.TrimSpace(submitted.RequestId) == "" {
		t.Fatalf("valid Work after accepted empty Work = %#v, want session/work/request identity", submitted)
	}
	support.WaitForSessionTerminalStatus(t, fixture.baseURL, sessionID, agentSharedProcessTimeout)
	listed := listAgentSessionWork(t, fixture.baseURL, sessionID)
	events := support.GetFactoryEventsForSessionAt(t, fixture.baseURL, sessionID)
	emptyWorkID := support.StringPointerValue(emptyResponse.WorkId)
	workID := support.StringPointerValue(submitted.WorkId)
	assertAgentEmptyScenarioWork(t, fixture.baseURL, sessionID, listed, emptyWorkID, workID, scenario.output)
	assertAgentEmptyScenarioDispatch(t, events, sessionID, emptyResponse.RequestId, emptyWorkID, submitted.RequestId, workID, scenario.output)
	assertAgentWorkerSession(t, fixture.baseURL, sessionID, emptyWorkID, scenario)
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

	opened := fixture.openSessionResponse(t, scenario.factoryDir)
	sessionID := opened.Session.Id
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
	if scenario.behavior == agentSharedCancel || scenario.name == "RuntimeRoot" {
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
	if scenario.name == "RuntimeRoot" {
		responseEvents = readAgentResponseEventsUntilTerminal(t, responseStream, agentSharedProcessTimeout)
		responseStream.Close()
		responseStream.WaitClosed(agentSharedProcessTimeout)
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
	if scenario.name == "RuntimeRoot" {
		publicSession := getAgentFactorySession(t, fixture.baseURL, sessionID)
		assertAgentRuntimeRootPublicIdentities(t, fixture.baseURL, publicSession, events, responseEvents, sessionID, requestID, workID)
	}
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
	return fixture.openSessionResponse(t, factoryDir).Session.Id
}

func (fixture *agentSharedProcessFixture) openSessionResponse(
	t *testing.T,
	factoryDir string,
) factoryapi.OpenFactorySessionResponse {
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
	return opened
}

func getAgentFactorySession(t *testing.T, baseURL, sessionID string) factoryapi.FactorySession {
	t.Helper()
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID)
	response := support.GetJSON[factoryapi.FactorySessionGetResponse](t, endpoint)
	session, err := response.AsFactorySession()
	if err != nil {
		t.Fatalf("decode Factory Session %q: %v", sessionID, err)
	}
	return session
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
	wantCalls := scenario.wantCalls
	if scenario.name == "Empty" {
		// The operator-authorized AG-08 characterization accepts and dispatches
		// the empty request, then dispatches the valid follow-up as well.
		wantCalls++
	}
	if len(calls) != wantCalls {
		t.Fatalf("%s immutable route calls = %d, want %d; calls=%#v", scenario.name, len(calls), wantCalls, calls)
	}
	markerCalls := 0
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
			if scenario.name == "Empty" {
				continue
			}
			t.Fatalf("%s provider request[%d] omitted input marker %q: %#v", scenario.name, index, scenario.inputMarker, request)
		}
		markerCalls++
	}
	if scenario.name == "Empty" && markerCalls != scenario.wantCalls {
		t.Fatalf("%s valid follow-up marker calls = %d, want %d; calls=%#v", scenario.name, markerCalls, scenario.wantCalls, calls)
	}
}
