package factorysession_test

import (
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	mcpfactorysession "github.com/portpowered/infinite-you/pkg/mcp/factorysession"
)

func TestMockClient_StartAsync_RunningFixtureReturnsAcceptedSession(t *testing.T) {
	client := newFixtureMCPClient(t)

	response, err := client.StartAsync(factoryapi.FactorySessionExecutionRequest{
		RequestId: "req-petri-run-001",
		Source: factoryapi.FactorySessionExecutionSource{
			Kind:      factoryapi.FactorySessionExecutionSourceKindFactoryId,
			FactoryId: strPtr("customer-support-triage"),
		},
		Args: &map[string]any{"ticketId": "TKT-1001"},
	})
	if err != nil {
		t.Fatalf("StartAsync: %v", err)
	}
	if response.Error != nil {
		t.Fatalf("error = %#v, want success result", response.Error)
	}
	if response.Result == nil {
		t.Fatal("result = nil, want async execution response")
	}
	if response.Result.SessionId != "dur-sess-petri-run-001" {
		t.Fatalf("sessionId = %q, want dur-sess-petri-run-001", response.Result.SessionId)
	}
	if response.Result.Status != factoryapi.FactorySessionDurableLifecycleStatusRunning {
		t.Fatalf("status = %q, want RUNNING", response.Result.Status)
	}
	if response.Result.Links == nil || response.Result.Links.Session == nil {
		t.Fatal("links.session missing from async start response")
	}
}

func TestMockClient_StartAsync_MalformedRequestReturnsStableEnvelope(t *testing.T) {
	client := newFixtureMCPClient(t)

	response, err := client.StartAsync(factoryapi.FactorySessionExecutionRequest{
		Source: factoryapi.FactorySessionExecutionSource{
			Kind:      factoryapi.FactorySessionExecutionSourceKindFactoryId,
			FactoryId: strPtr("customer-support-triage"),
		},
	})
	if err != nil {
		t.Fatalf("StartAsync: %v", err)
	}
	if response.Result != nil {
		t.Fatalf("result = %#v, want request validation envelope", response.Result)
	}
	if response.Error == nil {
		t.Fatal("error = nil, want stable validation envelope")
	}
	if response.Error.Code != "BAD_REQUEST" {
		t.Fatalf("error code = %q, want BAD_REQUEST", response.Error.Code)
	}
}

func TestMockClient_GetSession_PollsRunningThenTerminalStatus(t *testing.T) {
	client := newFixtureMCPClient(t)

	assertRunningSessionPoll(t, client)
	assertTerminalSessionPoll(t, client)
}

func assertRunningSessionPoll(t *testing.T, client *mcpfactorysession.Client) {
	t.Helper()
	startResponse, startErr := client.StartAsync(factoryapi.FactorySessionExecutionRequest{
		RequestId: "req-petri-run-001",
		Source: factoryapi.FactorySessionExecutionSource{
			Kind:      factoryapi.FactorySessionExecutionSourceKindFactoryId,
			FactoryId: strPtr("customer-support-triage"),
		},
	})
	if startErr != nil {
		t.Fatalf("StartAsync running: %v", startErr)
	}
	if startResponse.Error != nil || startResponse.Result == nil {
		t.Fatalf("running start = %#v, want success", startResponse)
	}

	running, err := client.GetSession(mcpfactorysession.GetSessionInput{
		SessionID: startResponse.Result.SessionId,
	})
	if err != nil {
		t.Fatalf("GetSession running: %v", err)
	}
	if running.Error != nil || running.Result == nil {
		t.Fatalf("running read = %#v, want success", running)
	}
	if running.Result.Status != factoryapi.FactorySessionDurableLifecycleStatusRunning {
		t.Fatalf("status = %q, want RUNNING", running.Result.Status)
	}
}

func assertTerminalSessionPoll(t *testing.T, client *mcpfactorysession.Client) {
	t.Helper()
	terminalStart, err := client.StartAsync(petriSuccessSyncRequest())
	if err != nil {
		t.Fatalf("StartAsync terminal: %v", err)
	}
	if terminalStart.Error != nil || terminalStart.Result == nil {
		t.Fatalf("terminal start = %#v, want success", terminalStart)
	}

	terminal, err := client.GetSession(mcpfactorysession.GetSessionInput{
		SessionID: terminalStart.Result.SessionId,
	})
	if err != nil {
		t.Fatalf("GetSession terminal: %v", err)
	}
	if terminal.Error != nil || terminal.Result == nil {
		t.Fatalf("terminal read = %#v, want success", terminal)
	}
	if terminal.Result.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("status = %q, want SUCCEEDED", terminal.Result.Status)
	}
	if terminal.Result.ResultSummary == nil || terminal.Result.ResultSummary.ResultStatus != factoryapi.FactorySessionResultStatusFinal {
		t.Fatalf("resultSummary = %#v, want FINAL", terminal.Result.ResultSummary)
	}
}

func TestMockClient_GetResult_NotReadyBeforeCompletionReturnsStableEnvelope(t *testing.T) {
	client := newFixtureMCPClient(t)

	started, err := client.StartAsync(factoryapi.FactorySessionExecutionRequest{
		RequestId: "req-petri-run-001",
		Source: factoryapi.FactorySessionExecutionSource{
			Kind:      factoryapi.FactorySessionExecutionSourceKindFactoryId,
			FactoryId: strPtr("customer-support-triage"),
		},
	})
	if err != nil {
		t.Fatalf("StartAsync: %v", err)
	}
	if started.Error != nil || started.Result == nil {
		t.Fatalf("start = %#v, want success", started)
	}

	mode := factoryapi.FactorySessionResultModeFinal
	response, err := client.GetResult(mcpfactorysession.GetResultInput{
		SessionID: started.Result.SessionId,
		Mode:      &mode,
	})
	if err != nil {
		t.Fatalf("GetResult: %v", err)
	}
	if response.Result != nil {
		t.Fatalf("result = %#v, want not-ready error envelope", response.Result)
	}
	if response.Error == nil {
		t.Fatal("error = nil, want not-ready envelope")
	}
	if response.Error.Code != "factory_session.result.not_ready" {
		t.Fatalf("error code = %q, want factory_session.result.not_ready", response.Error.Code)
	}
	if !response.Error.Retryable {
		t.Fatal("retryable = false, want true for running session")
	}
	if response.Error.SessionID != started.Result.SessionId {
		t.Fatalf("sessionId = %q, want %q", response.Error.SessionID, started.Result.SessionId)
	}
	if response.Error.Details == nil || response.Error.Details["reason"] != "RESULT_NOT_READY" {
		t.Fatalf("details = %#v, want RESULT_NOT_READY reason", response.Error.Details)
	}
}

func TestMockClient_GetSession_MissingSessionReturnsStableEnvelope(t *testing.T) {
	client := newFixtureMCPClient(t)

	response, err := client.GetSession(mcpfactorysession.GetSessionInput{
		SessionID: "dur-sess-does-not-exist",
	})
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if response.Result != nil {
		t.Fatalf("result = %#v, want missing-session envelope", response.Result)
	}
	if response.Error == nil {
		t.Fatal("error = nil, want missing-session envelope")
	}
	if response.Error.Code != "factory_session.session.not_found" {
		t.Fatalf("error code = %q, want factory_session.session.not_found", response.Error.Code)
	}
	if response.Error.Retryable {
		t.Fatal("retryable = true, want false for missing session")
	}
	if response.Error.SessionID != "dur-sess-does-not-exist" {
		t.Fatalf("sessionId = %q, want dur-sess-does-not-exist", response.Error.SessionID)
	}
}

func TestMockClient_GetResult_MissingSessionReturnsStableEnvelope(t *testing.T) {
	client := newFixtureMCPClient(t)

	response, err := client.GetResult(mcpfactorysession.GetResultInput{
		SessionID: "dur-sess-does-not-exist",
	})
	if err != nil {
		t.Fatalf("GetResult: %v", err)
	}
	if response.Result != nil {
		t.Fatalf("result = %#v, want missing-session envelope", response.Result)
	}
	if response.Error == nil {
		t.Fatal("error = nil, want missing-session envelope")
	}
	if response.Error.Code != "factory_session.session.not_found" {
		t.Fatalf("error code = %q, want factory_session.session.not_found", response.Error.Code)
	}
}

func TestMockClient_StartAsync_WithoutServiceReturnsUnavailableEnvelope(t *testing.T) {
	client := mcpfactorysession.NewClient()

	response, err := client.StartAsync(factoryapi.FactorySessionExecutionRequest{
		RequestId: "req-petri-run-001",
		Source: factoryapi.FactorySessionExecutionSource{
			Kind:      factoryapi.FactorySessionExecutionSourceKindFactoryId,
			FactoryId: strPtr("customer-support-triage"),
		},
	})
	if err != nil {
		t.Fatalf("StartAsync: %v", err)
	}
	if response.Result != nil {
		t.Fatalf("result = %#v, want unavailable service envelope", response.Result)
	}
	if response.Error == nil {
		t.Fatal("error = nil, want unavailable service envelope")
	}
	if response.Error.Code != "factory_session.service.unavailable" {
		t.Fatalf("error code = %q, want factory_session.service.unavailable", response.Error.Code)
	}
}

func TestMockClient_ListSessions_DefaultsToLiveScope(t *testing.T) {
	client := newFixtureMCPClient(t)

	response, err := client.ListSessions(mcpfactorysession.ListSessionsInput{})
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if response.Error != nil || response.Result == nil {
		t.Fatalf("list = %#v, want success", response)
	}
	if response.Result.Scope == nil || *response.Result.Scope != factoryapi.FactorySessionListScopeLive {
		t.Fatalf("scope = %#v, want live", response.Result.Scope)
	}
}

func TestMockClient_ListSessions_ScopedPersistedAndAll(t *testing.T) {
	client := newFixtureMCPClient(t)
	seedRunningAndSuccessSessions(t, client)

	persistedScope := factoryapi.FactorySessionListScopePersisted
	persisted, err := client.ListSessions(mcpfactorysession.ListSessionsInput{Scope: &persistedScope})
	if err != nil {
		t.Fatalf("ListSessions persisted: %v", err)
	}
	persistedResult := requireListSessionsSuccess(t, persisted)
	assertListScope(t, persistedResult, factoryapi.FactorySessionListScopePersisted)
	if len(persistedResult.Sessions) != 0 {
		t.Fatalf("sessions = %#v, want none for persisted scope", persistedResult.Sessions)
	}
	assertContainsDurableSession(t, persistedResult, "dur-sess-petri-success-001")
	assertOmitsDurableSession(t, persistedResult, "dur-sess-petri-run-001")

	allScope := factoryapi.FactorySessionListScopeAll
	all, err := client.ListSessions(mcpfactorysession.ListSessionsInput{Scope: &allScope})
	if err != nil {
		t.Fatalf("ListSessions all: %v", err)
	}
	allResult := requireListSessionsSuccess(t, all)
	assertListScope(t, allResult, factoryapi.FactorySessionListScopeAll)
	if len(allResult.Sessions) != 1 || allResult.Sessions[0].Id != "dur-sess-petri-run-001" {
		t.Fatalf("sessions = %#v, want deduped live running row", allResult.Sessions)
	}
	assertContainsDurableSession(t, allResult, "dur-sess-petri-success-001")
}

func seedRunningAndSuccessSessions(t *testing.T, client *mcpfactorysession.Client) {
	t.Helper()
	if _, err := client.StartAsync(factoryapi.FactorySessionExecutionRequest{
		RequestId: "req-petri-run-001",
		Source: factoryapi.FactorySessionExecutionSource{
			Kind:      factoryapi.FactorySessionExecutionSourceKindFactoryId,
			FactoryId: strPtr("customer-support-triage"),
		},
		Args: &map[string]any{"ticketId": "TKT-1001"},
	}); err != nil {
		t.Fatalf("StartAsync running: %v", err)
	}
	if _, err := client.StartSync(petriSuccessSyncRequest()); err != nil {
		t.Fatalf("StartSync success: %v", err)
	}
}

func requireListSessionsSuccess(
	t *testing.T,
	response mcpfactorysession.ToolResponse[factoryapi.ListFactorySessionsResponse],
) *factoryapi.ListFactorySessionsResponse {
	t.Helper()
	if response.Error != nil || response.Result == nil {
		t.Fatalf("list = %#v, want success", response)
	}
	return response.Result
}

func assertListScope(t *testing.T, result *factoryapi.ListFactorySessionsResponse, scope factoryapi.FactorySessionListScope) {
	t.Helper()
	if result.Scope == nil || *result.Scope != scope {
		t.Fatalf("scope = %#v, want %q", result.Scope, scope)
	}
}

func assertContainsDurableSession(t *testing.T, result *factoryapi.ListFactorySessionsResponse, sessionID string) {
	t.Helper()
	if result.DurableSessions == nil {
		t.Fatalf("durableSessions = nil, want row %q", sessionID)
	}
	for _, row := range *result.DurableSessions {
		if row.SessionId == sessionID {
			return
		}
	}
	t.Fatalf("durableSessions = %#v, want row %q", result.DurableSessions, sessionID)
}

func assertOmitsDurableSession(t *testing.T, result *factoryapi.ListFactorySessionsResponse, sessionID string) {
	t.Helper()
	if result.DurableSessions == nil {
		return
	}
	for _, row := range *result.DurableSessions {
		if row.SessionId == sessionID {
			t.Fatalf("durableSessions unexpectedly contain %q", sessionID)
		}
	}
}

func TestMockClient_ListSessions_UnsupportedScopeReturnsStableEnvelope(t *testing.T) {
	client := newFixtureMCPClient(t)
	scope := factoryapi.FactorySessionListScope("workspace")
	response, err := client.ListSessions(mcpfactorysession.ListSessionsInput{Scope: &scope})
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if response.Result != nil {
		t.Fatalf("result = %#v, want validation envelope", response.Result)
	}
	if response.Error == nil {
		t.Fatal("error = nil, want stable validation envelope")
	}
	if response.Error.Code != "BAD_REQUEST" {
		t.Fatalf("error code = %q, want BAD_REQUEST", response.Error.Code)
	}
}

func TestMockClient_ListSessions_UnavailableServiceReturnsStableEnvelope(t *testing.T) {
	client := mcpfactorysession.NewClient()
	response, err := client.ListSessions(mcpfactorysession.ListSessionsInput{})
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if response.Result != nil {
		t.Fatalf("result = %#v, want unavailable service envelope", response.Result)
	}
	if response.Error == nil {
		t.Fatal("error = nil, want unavailable service envelope")
	}
	if response.Error.Code != "factory_session.service.unavailable" {
		t.Fatalf("error code = %q, want factory_session.service.unavailable", response.Error.Code)
	}
}
