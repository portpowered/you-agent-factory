package smoke

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const globalConfigWorkflowSource = `return (async function () {
  const defaults = await agent.run({prompt: "use operator defaults", label: "defaults"});
  const preset = await agent.run({prompt: "use configured preset", label: "preset", preset: "careful-review"});
  return {defaults, preset};
})();`

func TestConfigDrivenExecution_GlobalConfigDrivesDefaultsAndWorkerPreset(t *testing.T) {
	dir := support.ScaffoldFactory(t, simpleServicePipelineConfig())
	support.WriteAgentConfig(t, dir, "worker-a", "---\ntype: MODEL_WORKER\n---\n")
	homeDir := writeRuntimeGlobalConfig(t)
	provider := testutil.NewMockProvider(
		workerexecution.InferenceResponse{Content: `{"text":"defaults complete"}`},
		workerexecution.InferenceResponse{Content: `{"text":"preset complete"}`},
	)
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		Env: append(os.Environ(),
			"HOME="+homeDir,
			"USERPROFILE="+homeDir,
		),
		Edges: serviceedges.Edges{ProviderOverride: provider},
	})

	started := startGlobalConfigWorkflow(t, server.URL())
	if started.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("session status = %q, want SUCCEEDED", started.Status)
	}
	dispatches := support.GetJSON[factoryapi.ListFactorySessionDispatchesResponse](
		t,
		strings.TrimSuffix(server.URL(), "/")+"/factory-sessions/"+started.SessionId+"/dispatches",
	)
	assertGlobalConfigDispatches(t, dispatches.Dispatches)
	assertGlobalConfigProviderRequests(t, provider.Calls())
}

func writeRuntimeGlobalConfig(t *testing.T) string {
	t.Helper()
	homeDir := t.TempDir()
	configDir := filepath.Join(homeDir, ".you-agent-factory")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatalf("mkdir global config directory: %v", err)
	}
	config := []byte(`{
  "defaults": {"workerModelProvider": "openai", "workerModel": "default-model"},
  "workerPresets": [{
    "id": "careful-review",
    "modelProvider": "codex",
    "model": "preset-model",
    "reasoningEffort": "medium"
  }]
}`)
	if err := os.WriteFile(filepath.Join(configDir, "config.json"), config, 0o600); err != nil {
		t.Fatalf("write global config: %v", err)
	}
	return homeDir
}

func startGlobalConfigWorkflow(t *testing.T, serverURL string) factoryapi.FactorySessionSyncExecutionResponse {
	t.Helper()
	dialect := "you-workflow-v1"
	payload, err := json.Marshal(factoryapi.FactorySessionExecutionRequest{
		RequestId: "global-config-runtime-selection",
		Source: factoryapi.FactorySessionExecutionSource{
			Kind: factoryapi.FactorySessionExecutionSourceKindInlineWorkflow,
			InlineWorkflow: &factoryapi.FactorySessionExecutionInlineWorkflow{
				Dialect: &dialect,
				InlineSource: factoryapi.FactoryOrchestratorJavaScriptInlineSource{
					Encoding: factoryapi.FactoryOrchestratorJavaScriptInlineSourceEncodingUtf8,
					Inline:   globalConfigWorkflowSource,
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal global config workflow request: %v", err)
	}
	endpoint := strings.TrimSuffix(serverURL, "/") + "/factory-sessions/sync"
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("build global config workflow request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("start global config workflow: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		var body bytes.Buffer
		_, _ = body.ReadFrom(response.Body)
		t.Fatalf("start global config workflow status = %d: %s", response.StatusCode, body.String())
	}
	var started factoryapi.FactorySessionSyncExecutionResponse
	if err := json.NewDecoder(response.Body).Decode(&started); err != nil {
		t.Fatalf("decode global config workflow response: %v", err)
	}
	return started
}

func assertGlobalConfigDispatches(t *testing.T, dispatches []factoryapi.FactorySessionDispatchSummary) {
	t.Helper()
	if len(dispatches) != 2 {
		t.Fatalf("dispatch count = %d, want 2", len(dispatches))
	}
	assertGlobalConfigDispatch(t, dispatches[0], "", "CODEX", "default-model", "")
	assertGlobalConfigDispatch(t, dispatches[1], "careful-review", "CODEX", "preset-model", "medium")
}

func assertGlobalConfigDispatch(
	t *testing.T,
	dispatch factoryapi.FactorySessionDispatchSummary,
	wantPreset, wantProvider, wantModel, wantEffort string,
) {
	t.Helper()
	gotPreset := dereferenceGlobalConfigValue(dispatch.PresetId)
	gotProvider := dereferenceGlobalConfigValue(dispatch.ModelProvider)
	gotModel := dereferenceGlobalConfigValue(dispatch.Model)
	gotEffort := dereferenceGlobalConfigValue(dispatch.ReasoningEffort)
	if gotPreset != wantPreset || gotProvider != wantProvider || gotModel != wantModel || gotEffort != wantEffort {
		t.Fatalf(
			"dispatch selection = preset=%q provider=%q model=%q effort=%q, want preset=%q provider=%q model=%q effort=%q",
			gotPreset, gotProvider, gotModel, gotEffort,
			wantPreset, wantProvider, wantModel, wantEffort,
		)
	}
}

func assertGlobalConfigProviderRequests(t *testing.T, calls []workerexecution.ProviderInferenceRequest) {
	t.Helper()
	if len(calls) != 2 {
		t.Fatalf("provider call count = %d, want 2", len(calls))
	}
	if calls[0].ModelProvider != "CODEX" || calls[0].Model != "default-model" {
		t.Fatalf("default provider request = provider %q model %q, want CODEX/default-model", calls[0].ModelProvider, calls[0].Model)
	}
	if calls[1].ModelProvider != "CODEX" || calls[1].Model != "preset-model" {
		t.Fatalf("preset provider request = provider %q model %q, want CODEX/preset-model", calls[1].ModelProvider, calls[1].Model)
	}
}

func dereferenceGlobalConfigValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func TestConfigDrivenExecution_HappyPath(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "happy_path"))

	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "Config-driven happy path"}`))

	provider := testutil.NewMockProvider(
		workerexecution.InferenceResponse{Content: "Step one done. COMPLETE"},
		workerexecution.InferenceResponse{Content: "Step two done. COMPLETE"},
	)

	status := runFactoryThroughCustomerProcess(t, dir, provider)
	if status.Categories.Terminal != 1 || status.Categories.Failed != 0 {
		t.Fatalf("status categories = %+v, want one terminal work item", status.Categories)
	}

	if provider.CallCount() != 2 {
		t.Errorf("expected provider called 2 times, got %d", provider.CallCount())
	}
}

func TestConfigDrivenExecution_HappyPathFailureRouting(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "happy_path"))

	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "Will fail"}`))

	provider := testutil.NewMockProviderWithErrors(
		[]workerexecution.InferenceResponse{{Content: ""}},
		[]error{fmt.Errorf("something went wrong")},
	)

	status := runFactoryThroughCustomerProcess(t, dir, provider)
	if status.Categories.Failed != 1 || status.Categories.Terminal != 0 {
		t.Fatalf("status categories = %+v, want one failed work item", status.Categories)
	}
	if provider.CallCount() != 1 {
		t.Errorf("expected failed provider called once, got %d", provider.CallCount())
	}
}

func TestConfigDrivenExecution_AddWorkType(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "multi_work_type"))

	testutil.WriteSeedFile(t, dir, "request", []byte(`{"title": "New request"}`))
	testutil.WriteSeedFile(t, dir, "review", []byte(`{"title": "New review"}`))

	provider := testutil.NewMockProvider(
		workerexecution.InferenceResponse{Content: "Request handled. COMPLETE"},
		workerexecution.InferenceResponse{Content: "Review handled. COMPLETE"},
	)

	status := runFactoryThroughCustomerProcess(t, dir, provider)
	if status.Categories.Terminal != 2 || status.Categories.Failed != 0 {
		t.Fatalf("status categories = %+v, want two terminal work items", status.Categories)
	}

	if provider.CallCount() != 2 {
		t.Errorf("expected provider called 2 times, got %d", provider.CallCount())
	}
}
