package workers_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestJavaScriptAgentRunCodexCommandCharacterization(t *testing.T) {
	dir := support.ScaffoldFactory(t, overridesFactoryConfig())
	runner := support.NewRecordingCommandRunner("permission matrix child output")
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		Edges:                     serviceedges.Edges{ProviderCommandRunner: runner},
	})
	t.Cleanup(func() { server.Stop(t) })

	tests := []struct {
		name            string
		mode            string
		skipPermissions string
		wantArgs        []string
	}{
		{
			name:            "mode-unset/skipPermissions-absent",
			skipPermissions: "absent",
			wantArgs:        []string{"exec", "--json", "-"},
		},
		{
			name:            "mode-unset/skipPermissions-false",
			skipPermissions: "false",
			wantArgs:        []string{"exec", "--json", "-"},
		},
		{
			name:            "mode-unset/skipPermissions-true",
			skipPermissions: "true",
			wantArgs:        []string{"exec", "--json", "--dangerously-bypass-approvals-and-sandbox", "-"},
		},
		{
			name:            "READ_ONLY/skipPermissions-absent",
			mode:            "READ_ONLY",
			skipPermissions: "absent",
			wantArgs:        []string{"exec", "--json", "-"},
		},
		{
			name:            "READ_ONLY/skipPermissions-false",
			mode:            "READ_ONLY",
			skipPermissions: "false",
			wantArgs:        []string{"exec", "--json", "-"},
		},
		{
			name:            "READ_ONLY/skipPermissions-true",
			mode:            "READ_ONLY",
			skipPermissions: "true",
			wantArgs:        []string{"exec", "--json", "--dangerously-bypass-approvals-and-sandbox", "-"},
		},
	}

	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			before := len(runner.Requests())
			started := startPermissionMatrixWorkflow(
				t,
				server.URL(),
				"javascript-permission-matrix-"+string(rune('a'+index)),
				permissionMatrixWorkflow(test.skipPermissions),
				test.mode,
			)
			if started.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
				t.Fatalf("session status = %q, want SUCCEEDED; result=%#v", started.Status, started.Result)
			}

			requests := runner.Requests()
			if len(requests) != before+1 {
				t.Fatalf("provider command requests = %d, want one new request; requests=%#v", len(requests)-before, requests)
			}
			got := requests[before]
			if got.Command != "codex" || !reflect.DeepEqual(got.Args, test.wantArgs) {
				t.Fatalf("provider command = %q %#v, want codex %#v", got.Command, got.Args, test.wantArgs)
			}
		})
	}
}

func permissionMatrixWorkflow(skipPermissions string) string {
	field := ""
	if skipPermissions != "absent" {
		field = ", skipPermissions: " + skipPermissions
	}
	return `return (async function () {
  return await agent.run({
    prompt: "capture the current Codex command",
    label: "permission-matrix-child",
    modelProvider: "codex"` + field + `
  });
})();`
}

func startPermissionMatrixWorkflow(
	t *testing.T,
	serverURL, requestID, workflowSource, mode string,
) factoryapi.FactorySessionSyncExecutionResponse {
	t.Helper()

	dialect := "you-workflow-v1"
	request := factoryapi.FactorySessionExecutionRequest{
		RequestId: requestID,
		Source: factoryapi.FactorySessionExecutionSource{
			Kind: factoryapi.FactorySessionExecutionSourceKindInlineWorkflow,
			InlineWorkflow: &factoryapi.FactorySessionExecutionInlineWorkflow{
				Dialect: &dialect,
				InlineSource: factoryapi.FactoryOrchestratorJavaScriptInlineSource{
					Encoding: factoryapi.FactoryOrchestratorJavaScriptInlineSourceEncodingUtf8,
					Inline:   workflowSource,
				},
			},
		},
	}
	defaultPolicy := map[string]interface{}{}
	if mode != "" {
		defaultPolicy["mode"] = mode
	}
	request.Orchestrator = &factoryapi.FactoryOrchestrator{
		Kind: factoryapi.JAVASCRIPT,
		Javascript: &factoryapi.FactoryOrchestratorJavaScriptConfig{
			DefaultPolicy: &defaultPolicy,
		},
	}
	payload, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal permission matrix workflow request: %v", err)
	}
	endpoint := strings.TrimSuffix(serverURL, "/") + "/factory-sessions/sync"
	httpRequest, err := http.NewRequestWithContext(t.Context(), http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("build permission matrix workflow request: %v", err)
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(httpRequest)
	if err != nil {
		t.Fatalf("start permission matrix workflow: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("start permission matrix workflow status = %d", response.StatusCode)
	}
	var started factoryapi.FactorySessionSyncExecutionResponse
	if err := json.NewDecoder(response.Body).Decode(&started); err != nil {
		t.Fatalf("decode permission matrix workflow response: %v", err)
	}
	return started
}
