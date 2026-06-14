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
