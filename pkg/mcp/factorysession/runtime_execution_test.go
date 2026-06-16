package factorysession_test

import (
	"context"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	mcpfactorysession "github.com/portpowered/infinite-you/pkg/mcp/factorysession"
)

const runtimeBusyLoopWorkflowSource = `// Busy loop fixture for runtime-backed MCP async polling tests.
var spin = 0;
while (true) {
  spin += 1;
}
`

const runtimeSimpleFinalWorkflowSource = `// Simple final-only workflow fixture for runtime-backed MCP async completion tests.
return {
  label: meta.name,
  description: meta.description,
  subject: args.subject,
  repeat: args.count,
  echo: args.prefix + ":" + args.subject,
};
`

func TestMockClient_RuntimeService_StartAsyncRunningObservesStatusAndNotReadyResult(t *testing.T) {
	client := newRuntimeMCPClient(t)

	started := assertRuntimeAsyncStartRunning(t, client, runtimeBusyLoopAsyncRequest("req-mcp-runtime-async-running-001"))
	assertRuntimeSessionStatus(t, client, started.Result.SessionId, factoryapi.FactorySessionDurableLifecycleStatusRunning)
	assertRuntimeNotReadyResult(t, client, started.Result.SessionId)
	cancelRuntimeSession(t, client, started.Result.SessionId)
}

func TestMockClient_RuntimeService_AsyncPollingObservesTerminalResult(t *testing.T) {
	client := newRuntimeMCPClient(t)
	request := runtimeSimpleFinalAsyncRequest("req-mcp-runtime-async-final-001")

	started := assertRuntimeAsyncStartRunning(t, client, request)
	service := runtimeServiceFromClient(t, client)
	session := waitUntilRuntimeSessionStatus(
		t,
		service,
		started.Result.SessionId,
		factorysessionexecution.LifecycleStatusSucceeded,
		5*time.Second,
	)
	assertRuntimeFinalResultSummary(t, session)
	assertRuntimeTerminalSessionReads(t, client, started.Result.SessionId)
}

func assertRuntimeAsyncStartRunning(
	t *testing.T,
	client *runtimeMCPClient,
	request factoryapi.FactorySessionExecutionRequest,
) mcpfactorysession.ToolResponse[factoryapi.FactorySessionExecutionResponse] {
	t.Helper()
	started, err := client.StartAsync(request)
	if err != nil {
		t.Fatalf("StartAsync: %v", err)
	}
	if started.Error != nil || started.Result == nil {
		t.Fatalf("start = %#v, want success", started)
	}
	if started.Result.Status != factoryapi.FactorySessionDurableLifecycleStatusRunning {
		t.Fatalf("start status = %q, want RUNNING", started.Result.Status)
	}
	if started.Result.SessionId == "" {
		t.Fatal("sessionId missing from async start response")
	}
	return started
}

func assertRuntimeSessionStatus(
	t *testing.T,
	client *runtimeMCPClient,
	sessionID string,
	want factoryapi.FactorySessionDurableLifecycleStatus,
) {
	t.Helper()
	status, err := client.GetSession(mcpfactorysession.GetSessionInput{SessionID: sessionID})
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if status.Error != nil || status.Result == nil {
		t.Fatalf("status = %#v, want success", status)
	}
	if status.Result.Status != want {
		t.Fatalf("status = %q, want %q", status.Result.Status, want)
	}
}

func assertRuntimeNotReadyResult(t *testing.T, client *runtimeMCPClient, sessionID string) {
	t.Helper()
	mode := factoryapi.FactorySessionResultModeFinal
	notReady, err := client.GetResult(mcpfactorysession.GetResultInput{
		SessionID: sessionID,
		Mode:      &mode,
	})
	if err != nil {
		t.Fatalf("GetResult: %v", err)
	}
	if notReady.Result != nil {
		t.Fatalf("result = %#v, want not-ready envelope", notReady.Result)
	}
	if notReady.Error == nil || notReady.Error.Code != "factory_session.result.not_ready" {
		t.Fatalf("error = %#v, want factory_session.result.not_ready", notReady.Error)
	}
	if !notReady.Error.Retryable {
		t.Fatal("retryable = false, want true for running session")
	}
}

func cancelRuntimeSession(t *testing.T, client *runtimeMCPClient, sessionID string) {
	t.Helper()
	service := runtimeServiceFromClient(t, client)
	cancelled, err := service.Cancel(context.Background(), sessionID, factorysessionexecution.ControlRequest{})
	if err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if cancelled.Outcome != factorysessionexecution.LifecycleControlOutcomeAccepted {
		t.Fatalf("cancel outcome = %q, want ACCEPTED", cancelled.Outcome)
	}
}

func assertRuntimeFinalResultSummary(t *testing.T, session factorysessionexecution.SessionReadResult) {
	t.Helper()
	if session.ResultSummary == nil ||
		session.ResultSummary.ResultStatus != string(factorysessionexecution.ResultStatusFinal) {
		t.Fatalf("resultSummary = %#v, want FINAL", session.ResultSummary)
	}
}

func assertRuntimeTerminalSessionReads(t *testing.T, client *runtimeMCPClient, sessionID string) {
	t.Helper()
	assertRuntimeSessionStatus(t, client, sessionID, factoryapi.FactorySessionDurableLifecycleStatusSucceeded)

	mode := factoryapi.FactorySessionResultModeFinal
	completedResult, err := client.GetResult(mcpfactorysession.GetResultInput{
		SessionID: sessionID,
		Mode:      &mode,
	})
	if err != nil {
		t.Fatalf("GetResult completed: %v", err)
	}
	if completedResult.Error != nil || completedResult.Result == nil {
		t.Fatalf("completed result = %#v, want terminal result", completedResult)
	}
	if completedResult.Result.ResultStatus != factoryapi.FactorySessionResultStatusFinal {
		t.Fatalf("resultStatus = %q, want FINAL", completedResult.Result.ResultStatus)
	}
	if completedResult.Result.PrimaryResult == nil {
		t.Fatal("primaryResult missing from terminal result")
	}
}

func newRuntimeMCPClient(t *testing.T) *runtimeMCPClient {
	t.Helper()
	service, err := factorysessionexecution.NewExecutionService(
		factorysessionexecution.ExecutionProviderJavaScriptRuntime,
		factorysessionexecution.ServiceConfig{ProjectRoot: t.TempDir()},
	)
	if err != nil {
		t.Fatalf("NewExecutionService: %v", err)
	}
	return &runtimeMCPClient{
		Client:  mcpfactorysession.NewClientWithService(service),
		service: service,
	}
}

type runtimeMCPClient struct {
	*mcpfactorysession.Client
	service factorysessionexecution.Service
}

func runtimeServiceFromClient(t *testing.T, client *runtimeMCPClient) factorysessionexecution.Service {
	t.Helper()
	if client.service == nil {
		t.Fatal("runtime service missing from MCP client wrapper")
	}
	return client.service
}

func runtimeBusyLoopAsyncRequest(requestID string) factoryapi.FactorySessionExecutionRequest {
	return runtimeInlineAsyncRequest(requestID, runtimeBusyLoopWorkflowSource, map[string]any{
		"subject": "workflows",
	})
}

func runtimeSimpleFinalAsyncRequest(requestID string) factoryapi.FactorySessionExecutionRequest {
	return runtimeInlineAsyncRequest(requestID, runtimeSimpleFinalWorkflowSource, map[string]any{
		"subject": "workflows",
		"count":   2,
		"prefix":  "you",
	})
}

func runtimeInlineAsyncRequest(
	requestID string,
	source string,
	args map[string]any,
) factoryapi.FactorySessionExecutionRequest {
	dialect := "you-workflow-v1"
	metadata := factoryapi.StringMap{
		"name":        "runtime-mcp-async-fixture",
		"description": "runtime-backed MCP async polling fixture",
	}
	return factoryapi.FactorySessionExecutionRequest{
		RequestId: requestID,
		Source: factoryapi.FactorySessionExecutionSource{
			Kind: factoryapi.FactorySessionExecutionSourceKindInlineWorkflow,
			InlineWorkflow: &factoryapi.FactorySessionExecutionInlineWorkflow{
				InlineSource: factoryapi.FactoryOrchestratorJavaScriptInlineSource{
					Encoding: factoryapi.FactoryOrchestratorJavaScriptInlineSourceEncodingUtf8,
					Inline:   source,
				},
				Dialect:  &dialect,
				Metadata: &metadata,
			},
		},
		Args: &args,
	}
}

func waitUntilRuntimeSessionStatus(
	t *testing.T,
	service factorysessionexecution.Service,
	sessionID string,
	want factorysessionexecution.LifecycleStatus,
	timeout time.Duration,
) factorysessionexecution.SessionReadResult {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		session, err := service.GetSession(context.Background(), sessionID)
		if err != nil {
			t.Fatalf("GetSession: %v", err)
		}
		if session.Status == want {
			return session
		}
		if factorysessionexecution.IsTerminalLifecycleStatus(session.Status) && session.Status != want {
			t.Fatalf("session %s reached terminal %q before %q", sessionID, session.Status, want)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("session %s did not reach status %q within %s", sessionID, want, timeout)
	return factorysessionexecution.SessionReadResult{}
}
