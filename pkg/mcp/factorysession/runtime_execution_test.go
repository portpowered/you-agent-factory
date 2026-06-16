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

	started, err := client.StartAsync(runtimeBusyLoopAsyncRequest("req-mcp-runtime-async-running-001"))
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

	runningStatus, err := client.GetSession(mcpfactorysession.GetSessionInput{
		SessionID: started.Result.SessionId,
	})
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if runningStatus.Error != nil || runningStatus.Result == nil {
		t.Fatalf("running status = %#v, want success", runningStatus)
	}
	if runningStatus.Result.Status != factoryapi.FactorySessionDurableLifecycleStatusRunning {
		t.Fatalf("running status = %q, want RUNNING", runningStatus.Result.Status)
	}

	mode := factoryapi.FactorySessionResultModeFinal
	notReady, err := client.GetResult(mcpfactorysession.GetResultInput{
		SessionID: started.Result.SessionId,
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

	service := runtimeServiceFromClient(t, client)
	cancelled, err := service.Cancel(context.Background(), started.Result.SessionId, factorysessionexecution.ControlRequest{})
	if err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if cancelled.Outcome != factorysessionexecution.LifecycleControlOutcomeAccepted {
		t.Fatalf("cancel outcome = %q, want ACCEPTED", cancelled.Outcome)
	}
}

func TestMockClient_RuntimeService_AsyncPollingObservesTerminalResult(t *testing.T) {
	client := newRuntimeMCPClient(t)
	request := runtimeSimpleFinalAsyncRequest("req-mcp-runtime-async-final-001")

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

	service := runtimeServiceFromClient(t, client)
	session := waitUntilRuntimeSessionStatus(
		t,
		service,
		started.Result.SessionId,
		factorysessionexecution.LifecycleStatusSucceeded,
		5*time.Second,
	)
	if session.ResultSummary == nil ||
		session.ResultSummary.ResultStatus != string(factorysessionexecution.ResultStatusFinal) {
		t.Fatalf("resultSummary = %#v, want FINAL", session.ResultSummary)
	}

	completedStatus, err := client.GetSession(mcpfactorysession.GetSessionInput{
		SessionID: started.Result.SessionId,
	})
	if err != nil {
		t.Fatalf("GetSession completed: %v", err)
	}
	if completedStatus.Error != nil || completedStatus.Result == nil {
		t.Fatalf("completed status = %#v, want success", completedStatus)
	}
	if completedStatus.Result.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("completed status = %q, want SUCCEEDED", completedStatus.Result.Status)
	}

	mode := factoryapi.FactorySessionResultModeFinal
	completedResult, err := client.GetResult(mcpfactorysession.GetResultInput{
		SessionID: started.Result.SessionId,
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
