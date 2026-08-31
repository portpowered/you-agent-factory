package mcp_resume_test

import (
	"testing"
	"time"

	mcpfactorysession "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/mcp"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

const runtimeSmokeSimpleFinalWorkflowSource = `// Runtime-backed MCP server smoke fixture: terminal async completion.
// runtimeSmokeProjectRoot removes persisted factory state before t.TempDir cleanup.
return {
  label: meta.name,
  description: meta.description,
  subject: args.subject,
  repeat: args.count,
  echo: args.prefix + ":" + args.subject,
};
`

func TestRunServe_RuntimeSmoke_DiscoveryAsyncPollAndResult(t *testing.T) {
	fixture := mcpResumePackageFixtureForTest(t)
	client := fixture.client

	sessionID := assertRuntimeSmokeAsyncStart(t, client, fixture, fixture.nextRequestID("runtime-smoke"))
	assertRuntimeSmokePollObservesRunningOrTerminal(t, client, sessionID)
	waitRuntimeSmokeTerminalCompletion(t, client, sessionID)
}

func waitRuntimeSmokeTerminalCompletion(t *testing.T, client *stdioMCPClient, sessionID string) {
	t.Helper()

	// The public session read and final result are the observable async
	// completion witnesses. The provider edge cannot prove that persistence and
	// result projection are complete, so this bounded poll intentionally yields
	// between public reads instead of using a fixed startup wait.
	deadline := time.Now().Add(5 * time.Second)
	mode := factoryapi.FactorySessionResultModeFinal
	for time.Now().Before(deadline) {
		status := runtimeSmokeSessionStatus(t, client, sessionID)
		switch status {
		case factoryapi.FactorySessionDurableLifecycleStatusSucceeded:
			assertRuntimeSmokeTerminalResult(t, client, sessionID, mode)
			return
		case factoryapi.FactorySessionDurableLifecycleStatusRunning:
			time.Sleep(10 * time.Millisecond)
			continue
		default:
			t.Fatalf("get status = %q, want RUNNING or SUCCEEDED before runtime smoke shutdown", status)
		}
	}
	t.Fatalf("session %s did not reach SUCCEEDED within 5s before runtime smoke shutdown", sessionID)
}

func assertRuntimeSmokeAsyncStart(
	t *testing.T,
	client *stdioMCPClient,
	fixture *mcpResumePackageFixture,
	requestID string,
) string {
	t.Helper()
	asyncStart := decodeToolResponse[factoryapi.FactorySessionExecutionResponse](
		t,
		client.callTool(mcpfactorysession.ToolStartAsync, runtimeSmokeInlineAsyncRequest(requestID)),
	)
	if asyncStart.Error != nil || asyncStart.Result == nil {
		t.Fatalf("start_async = %#v, want success", asyncStart)
	}
	if asyncStart.Result.SessionId == "" {
		t.Fatal("sessionId missing from async start response")
	}
	sessionID := asyncStart.Result.SessionId
	// Register cleanup before checking any post-start state so a failed status
	// or result observation cannot leave this durable session unowned.
	fixture.trackSession(t, client, sessionID)
	if asyncStart.Result.Status != factoryapi.FactorySessionDurableLifecycleStatusRunning {
		t.Fatalf("start_async status = %q, want RUNNING", asyncStart.Result.Status)
	}
	return sessionID
}

func assertRuntimeSmokePollObservesRunningOrTerminal(t *testing.T, client *stdioMCPClient, sessionID string) {
	t.Helper()

	// This checks the customer-visible RUNNING/not-ready and terminal/result
	// sequence. A provider completion signal cannot prove either public read
	// model, so the bounded poll must observe those MCP responses directly.
	deadline := time.Now().Add(5 * time.Second)
	mode := factoryapi.FactorySessionResultModeFinal
	observedRunningNotReady := false
	observedTerminal := false

	for time.Now().Before(deadline) {
		status := runtimeSmokeSessionStatus(t, client, sessionID)
		switch status {
		case factoryapi.FactorySessionDurableLifecycleStatusRunning:
			if runtimeSmokeResultIsFinal(t, client, sessionID, mode) {
				assertRuntimeSmokeTerminalResult(t, client, sessionID, mode)
				observedTerminal = true
			} else {
				assertRuntimeSmokeRunningNotReady(t, client, sessionID, mode)
				observedRunningNotReady = true
			}
		case factoryapi.FactorySessionDurableLifecycleStatusSucceeded:
			assertRuntimeSmokeTerminalResult(t, client, sessionID, mode)
			observedTerminal = true
		default:
			t.Fatalf("get status = %q, want RUNNING or SUCCEEDED", status)
		}

		if observedRunningNotReady || observedTerminal {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if !observedRunningNotReady && !observedTerminal {
		t.Fatalf("session %s did not reach RUNNING+not-ready or SUCCEEDED+terminal result within 5s", sessionID)
	}
}

func runtimeSmokeSessionStatus(t *testing.T, client *stdioMCPClient, sessionID string) factoryapi.FactorySessionDurableLifecycleStatus {
	t.Helper()
	statusResponse := decodeToolResponse[factoryapi.FactorySessionDurableReadModel](
		t,
		client.callTool(mcpfactorysession.ToolGetSession, map[string]any{"sessionId": sessionID}),
	)
	if statusResponse.Error != nil || statusResponse.Result == nil {
		t.Fatalf("get = %#v, want success", statusResponse)
	}
	return statusResponse.Result.Status
}

func runtimeSmokeResultIsFinal(
	t *testing.T,
	client *stdioMCPClient,
	sessionID string,
	mode factoryapi.FactorySessionResultMode,
) bool {
	t.Helper()
	response := decodeToolResponse[factoryapi.FactorySessionResult](
		t,
		client.callTool(mcpfactorysession.ToolGetResult, map[string]any{
			"sessionId": sessionID,
			"mode":      mode,
		}),
	)
	return response.Error == nil &&
		response.Result != nil &&
		response.Result.ResultStatus == factoryapi.FactorySessionResultStatusFinal
}

func assertRuntimeSmokeRunningNotReady(
	t *testing.T,
	client *stdioMCPClient,
	sessionID string,
	mode factoryapi.FactorySessionResultMode,
) bool {
	t.Helper()
	response := decodeToolResponse[factoryapi.FactorySessionResult](
		t,
		client.callTool(mcpfactorysession.ToolGetResult, map[string]any{
			"sessionId": sessionID,
			"mode":      mode,
		}),
	)
	if response.Result != nil && response.Result.ResultStatus == factoryapi.FactorySessionResultStatusFinal {
		if response.Result.PrimaryResult == nil {
			t.Fatal("primaryResult missing from terminal result")
		}
		return true
	}
	assertRuntimeSmokeRunningNotReadyResponse(t, response)
	return false
}

func assertRuntimeSmokeRunningNotReadyResponse(t *testing.T, response mcpfactorysession.ToolResponse[factoryapi.FactorySessionResult]) {
	t.Helper()
	if response.Result != nil {
		t.Fatalf("get_result running = %#v, want not-ready envelope", response.Result)
	}
	if response.Error == nil || response.Error.Code != "factory_session.result.not_ready" {
		t.Fatalf("get_result error = %#v, want factory_session.result.not_ready", response.Error)
	}
}

func assertRuntimeSmokeTerminalResult(
	t *testing.T,
	client *stdioMCPClient,
	sessionID string,
	mode factoryapi.FactorySessionResultMode,
) {
	t.Helper()
	completedResult := decodeToolResponse[factoryapi.FactorySessionResult](
		t,
		client.callTool(mcpfactorysession.ToolGetResult, map[string]any{
			"sessionId": sessionID,
			"mode":      mode,
		}),
	)
	if completedResult.Error != nil || completedResult.Result == nil {
		t.Fatalf("get_result terminal = %#v, want terminal result", completedResult)
	}
	if completedResult.Result.ResultStatus != factoryapi.FactorySessionResultStatusFinal {
		t.Fatalf("resultStatus = %q, want FINAL", completedResult.Result.ResultStatus)
	}
	if completedResult.Result.PrimaryResult == nil {
		t.Fatal("primaryResult missing from terminal result")
	}
}

func runtimeSmokeInlineAsyncRequest(requestID string) factoryapi.FactorySessionExecutionRequest {
	dialect := "you-workflow-v1"
	metadata := factoryapi.StringMap{
		"name":        "runtime-mcp-serve-smoke",
		"description": "runtime-backed MCP server smoke fixture",
	}
	args := map[string]any{
		"subject": "workflows",
		"count":   2,
		"prefix":  "you",
	}
	return factoryapi.FactorySessionExecutionRequest{
		RequestId: requestID,
		Source: factoryapi.FactorySessionExecutionSource{
			Kind: factoryapi.FactorySessionExecutionSourceKindInlineWorkflow,
			InlineWorkflow: &factoryapi.FactorySessionExecutionInlineWorkflow{
				InlineSource: factoryapi.FactoryOrchestratorJavaScriptInlineSource{
					Encoding: factoryapi.FactoryOrchestratorJavaScriptInlineSourceEncodingUtf8,
					Inline:   runtimeSmokeSimpleFinalWorkflowSource,
				},
				Dialect:  &dialect,
				Metadata: &metadata,
			},
		},
		Args: &args,
	}
}
