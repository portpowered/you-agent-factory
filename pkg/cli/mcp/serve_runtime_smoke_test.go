package mcpcli_test

import (
	"context"
	"os"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	mcpcli "github.com/portpowered/infinite-you/pkg/cli/mcp"
	mcpfactorysession "github.com/portpowered/infinite-you/pkg/mcp/factorysession"
)

const runtimeSmokeSimpleFinalWorkflowSource = `// Runtime-backed MCP serve smoke fixture: terminal async completion.
return {
  label: meta.name,
  description: meta.description,
  subject: args.subject,
  repeat: args.count,
  echo: args.prefix + ":" + args.subject,
};
`

// pkgmaintcheck:ignore-cyclomatic-complexity runtime smoke keeps discovery, async start, polling, and result assertions on one documented stdio path.
func TestRunServe_RuntimeSmoke_DiscoveryAsyncPollAndResult(t *testing.T) {
	projectRoot := t.TempDir()
	client, stdinWrite, serveErr := startRunServeRuntimeSmokeServer(t, projectRoot)
	assertInstallSmokeInitialize(t, client)
	assertInstallSmokeDiscovery(t, client)

	sessionID := assertRuntimeSmokeAsyncStart(t, client)
	assertRuntimeSmokePollObservesRunningOrTerminal(t, client, sessionID)
	closeRunServeSmokeServer(t, stdinWrite, serveErr)
}

func startRunServeRuntimeSmokeServer(
	t *testing.T,
	projectRoot string,
) (*stdioMCPClient, *os.File, <-chan error) {
	t.Helper()
	stdinRead, stdinWrite, err := os.Pipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	stdoutRead, stdoutWrite, err := os.Pipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	t.Cleanup(func() {
		_ = stdinRead.Close()
		_ = stdinWrite.Close()
		_ = stdoutRead.Close()
		_ = stdoutWrite.Close()
	})

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- mcpcli.RunServe(ctx, mcpcli.ServeConfig{
			RuntimeBacked: true,
			ProjectRoot:   projectRoot,
			Stdin:         stdinRead,
			Stdout:        stdoutWrite,
		})
	}()
	return newStdioMCPClient(t, stdinWrite, stdoutRead), stdinWrite, serveErr
}

func assertRuntimeSmokeAsyncStart(t *testing.T, client *stdioMCPClient) string {
	t.Helper()
	asyncStart := decodeToolResponse[factoryapi.FactorySessionExecutionResponse](
		t,
		client.callTool(mcpfactorysession.ToolStartAsync, runtimeSmokeInlineAsyncRequest()),
	)
	if asyncStart.Error != nil || asyncStart.Result == nil {
		t.Fatalf("start_async = %#v, want success", asyncStart)
	}
	if asyncStart.Result.Status != factoryapi.FactorySessionDurableLifecycleStatusRunning {
		t.Fatalf("start_async status = %q, want RUNNING", asyncStart.Result.Status)
	}
	if asyncStart.Result.SessionId == "" {
		t.Fatal("sessionId missing from async start response")
	}
	return asyncStart.Result.SessionId
}

func assertRuntimeSmokePollObservesRunningOrTerminal(t *testing.T, client *stdioMCPClient, sessionID string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	mode := factoryapi.FactorySessionResultModeFinal
	observedRunningNotReady := false
	observedTerminal := false

	for time.Now().Before(deadline) {
		statusResponse := decodeToolResponse[factoryapi.FactorySessionDurableReadModel](
			t,
			client.callTool(mcpfactorysession.ToolGetSession, map[string]any{"sessionId": sessionID}),
		)
		if statusResponse.Error != nil || statusResponse.Result == nil {
			t.Fatalf("get = %#v, want success", statusResponse)
		}

		switch statusResponse.Result.Status {
		case factoryapi.FactorySessionDurableLifecycleStatusRunning:
			notReady := decodeToolResponse[factoryapi.FactorySessionResult](
				t,
				client.callTool(mcpfactorysession.ToolGetResult, map[string]any{
					"sessionId": sessionID,
					"mode":      mode,
				}),
			)
			if notReady.Result != nil {
				t.Fatalf("get_result running = %#v, want not-ready envelope", notReady.Result)
			}
			if notReady.Error == nil || notReady.Error.Code != "factory_session.result.not_ready" {
				t.Fatalf("get_result error = %#v, want factory_session.result.not_ready", notReady.Error)
			}
			observedRunningNotReady = true
		case factoryapi.FactorySessionDurableLifecycleStatusSucceeded:
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
			observedTerminal = true
		default:
			t.Fatalf("get status = %q, want RUNNING or SUCCEEDED", statusResponse.Result.Status)
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

func runtimeSmokeInlineAsyncRequest() factoryapi.FactorySessionExecutionRequest {
	dialect := "you-workflow-v1"
	metadata := factoryapi.StringMap{
		"name":        "runtime-mcp-serve-smoke",
		"description": "runtime-backed MCP serve smoke fixture",
	}
	args := map[string]any{
		"subject": "workflows",
		"count":   2,
		"prefix":  "you",
	}
	return factoryapi.FactorySessionExecutionRequest{
		RequestId: "req-mcp-runtime-serve-smoke-001",
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
