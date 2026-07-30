// Package workers holds customer functional scenarios for JavaScript per-child
// worker override behavior through the public process and invocation boundary.
package workers_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	childCodexLabel           = "child-codex"
	childClaudeLabel          = "child-claude"
	childMockedLabel          = "child-mocked"
	childPassthroughLabel     = "child-passthrough"
	unknownOverrideChildLabel = "child-unknown-override"

	mockedWorkerPresetName       = "worker-a"
	mockedChildPrompt            = "mocked child prompt"
	passthroughChildPrompt       = "passthrough child prompt"
	unknownOverrideModelProvider = "Not_A_Provider"

	perChildProviderModelWorkflow = `return (async function () {
  const codexChild = await agent.run({
    prompt: "use codex provider and model",
    label: "` + childCodexLabel + `",
    modelProvider: "codex",
    model: "codex-child-model",
  });
  const claudeChild = await agent.run({
    prompt: "use claude provider and model",
    label: "` + childClaudeLabel + `",
    modelProvider: "claude",
    model: "claude-child-model",
  });
  return { codexChild, claudeChild };
})();`

	partialMockWorkersWorkflow = `return (async function () {
  const mocked = await agent.run({
    prompt: "` + mockedChildPrompt + `",
    label: "` + childMockedLabel + `",
    preset: "` + mockedWorkerPresetName + `",
  });
  const passthrough = await agent.run({
    prompt: "` + passthroughChildPrompt + `",
    label: "` + childPassthroughLabel + `",
  });
  return { mocked, passthrough };
})();`

	unknownWorkerOverrideWorkflow = `return (function () {
  agent.run({
    prompt: "use unknown worker provider override",
    label: "` + unknownOverrideChildLabel + `",
    modelProvider: "` + unknownOverrideModelProvider + `",
  });
  return { ok: true };
})();`

	mockWorkerAcceptedOutput = "mock worker accepted"
	livePassthroughChildText = "passthrough child provider output"
)

// TestJavaScriptChildrenSelectDifferentProvidersAndModels proves a JavaScript
// Factory with multiple child dispatches can select distinct per-child provider
// and model overrides so each child completes with public dispatch and provider
// evidence matching its configured worker selection rather than a shared default.
func TestJavaScriptChildrenSelectDifferentProvidersAndModels(t *testing.T) {
	t.Parallel()

	dir := support.ScaffoldFactory(t, overridesFactoryConfig())
	support.WriteAgentConfig(t, dir, "worker-a", "---\ntype: MODEL_WORKER\n---\n")

	provider := testutil.NewMockProvider(
		workers.InferenceResponse{Content: `{"text":"codex child complete"}`},
		workers.InferenceResponse{Content: `{"text":"claude child complete"}`},
	)
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		Edges:                     serviceedges.Edges{ProviderOverride: provider},
	})
	t.Cleanup(func() { server.Stop(t) })

	started := startOverridesWorkflow(t, server.URL(), "javascript-per-child-provider-model", perChildProviderModelWorkflow)
	if started.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("session status = %q, want SUCCEEDED", started.Status)
	}

	dispatches := support.GetJSON[factoryapi.ListFactorySessionDispatchesResponse](
		t,
		strings.TrimSuffix(server.URL(), "/")+"/factory-sessions/"+started.SessionId+"/dispatches",
	)
	assertPerChildProviderModelDispatches(t, dispatches.Dispatches)
	assertPerChildProviderModelRequests(t, provider.Calls())
}

// TestJavaScriptMockWorkersReplaceOnlyNamedChildren proves a partial
// --with-mock-workers configuration serves a named preset worker through the
// mock-worker accept path while an unmatched JavaScript child without that
// preset keeps the live-provider passthrough path when
// unmatchedDispatchPolicy is passthrough and a provider command edge is
// injected at the public process boundary.
func TestJavaScriptMockWorkersReplaceOnlyNamedChildren(t *testing.T) {
	t.Parallel()

	dir := support.ScaffoldFactory(t, overridesFactoryConfig())
	support.WriteAgentConfig(t, dir, mockedWorkerPresetName, "---\ntype: MODEL_WORKER\n---\n")
	homeDir := writePartialMockWorkersGlobalConfig(t)

	runner := support.NewRecordingCommandRunner(livePassthroughChildText)
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		UseMockWorkers:            true,
		MockWorkersConfig:         partialNamedJavaScriptMockWorkersConfig(),
		Edges:                     serviceedges.Edges{ProviderCommandRunner: runner},
		Env: append(os.Environ(),
			"HOME="+homeDir,
			"USERPROFILE="+homeDir,
		),
	})
	t.Cleanup(func() { server.Stop(t) })

	started := startOverridesWorkflow(
		t,
		server.URL(),
		"javascript-partial-mock-workers-mixed",
		partialMockWorkersWorkflow,
	)
	if started.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("session status = %q, want SUCCEEDED", started.Status)
	}
	if runner.CallCount() != 1 {
		t.Fatalf("provider command runner call count = %d, want 1 passthrough child dispatch", runner.CallCount())
	}

	dispatches := support.GetJSON[factoryapi.ListFactorySessionDispatchesResponse](
		t,
		strings.TrimSuffix(server.URL(), "/")+"/factory-sessions/"+started.SessionId+"/dispatches",
	)
	assertPartialMockMixedChildDispatches(t, dispatches.Dispatches)
	assertPartialMockMixedPrimaryResult(t, started.Result)
}

// TestJavaScriptUnknownWorkerOverrideFailsActionably proves configuring an
// invalid per-child worker override on agent.run fails at the public JavaScript
// boundary with a customer-readable diagnostic that identifies the bad override,
// does not emit a successful child dispatch, and does not leak private runtime
// internals as the primary failure signal.
func TestJavaScriptUnknownWorkerOverrideFailsActionably(t *testing.T) {
	t.Parallel()

	dir := support.ScaffoldFactory(t, overridesFactoryConfig())
	support.WriteAgentConfig(t, dir, "worker-a", "---\ntype: MODEL_WORKER\n---\n")

	runner := support.NewRecordingCommandRunner("unexpected live provider execution")
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		UseMockWorkers:            true,
		Edges:                     serviceedges.Edges{ProviderCommandRunner: runner},
	})
	t.Cleanup(func() { server.Stop(t) })

	started := startOverridesWorkflow(
		t,
		server.URL(),
		"javascript-unknown-worker-override",
		unknownWorkerOverrideWorkflow,
	)
	if started.Status != factoryapi.FactorySessionDurableLifecycleStatusFailed {
		t.Fatalf("session status = %q, want FAILED", started.Status)
	}
	if runner.CallCount() != 0 {
		t.Fatalf("provider command runner call count = %d, want 0 before override validation", runner.CallCount())
	}

	dispatches := support.GetJSON[factoryapi.ListFactorySessionDispatchesResponse](
		t,
		strings.TrimSuffix(server.URL(), "/")+"/factory-sessions/"+started.SessionId+"/dispatches",
	)
	if len(dispatches.Dispatches) != 0 {
		t.Fatalf("dispatch count = %d, want 0 for invalid worker override", len(dispatches.Dispatches))
	}

	assertUnknownWorkerOverrideFailureRecord(t, server.URL(), started.SessionId, started.Result)
}

func partialNamedJavaScriptMockWorkersConfig() *workers.MockWorkersConfig {
	return &workers.MockWorkersConfig{
		UnmatchedDispatchPolicy: workers.MockWorkerUnmatchedDispatchPolicyPassthrough,
		MockWorkers: []workers.MockWorkerConfig{{
			WorkerName: mockedWorkerPresetName,
			RunType:    workers.MockWorkerRunTypeAccept,
		}},
	}
}

func writePartialMockWorkersGlobalConfig(t *testing.T) string {
	t.Helper()

	homeDir := t.TempDir()
	configDir := filepath.Join(homeDir, ".you-agent-factory")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatalf("mkdir global config directory: %v", err)
	}
	config := []byte(`{
  "defaults": {"workerModelProvider": "codex", "workerModel": "default-model"},
  "workerPresets": [{
    "id": "` + mockedWorkerPresetName + `",
    "modelProvider": "codex",
    "model": "mocked-child-model"
  }]
}`)
	if err := os.WriteFile(filepath.Join(configDir, "config.json"), config, 0o600); err != nil {
		t.Fatalf("write global config: %v", err)
	}
	return homeDir
}

func assertPartialMockMixedChildDispatches(
	t *testing.T,
	dispatches []factoryapi.FactorySessionDispatchSummary,
) {
	t.Helper()

	if len(dispatches) != 2 {
		t.Fatalf("dispatch count = %d, want 2 child dispatches", len(dispatches))
	}
	byLabel := make(map[string]factoryapi.FactorySessionDispatchSummary, len(dispatches))
	for _, dispatch := range dispatches {
		if dispatch.Label == nil || strings.TrimSpace(*dispatch.Label) == "" {
			t.Fatalf("dispatch %s missing label, want labeled child dispatches", dispatch.Id)
		}
		if dispatch.Status != factoryapi.FactoryDispatchStatusCOMPLETED {
			t.Fatalf("dispatch %s status = %q, want COMPLETED", dispatch.Id, dispatch.Status)
		}
		byLabel[*dispatch.Label] = dispatch
	}
	assertPartialMockNamedChildDispatch(t, byLabel[childMockedLabel], childMockedLabel, mockedWorkerPresetName)
	assertPartialMockPassthroughChildDispatch(t, byLabel[childPassthroughLabel], childPassthroughLabel)
}

func assertPartialMockNamedChildDispatch(
	t *testing.T,
	dispatch factoryapi.FactorySessionDispatchSummary,
	wantLabel, wantPreset string,
) {
	t.Helper()

	if dispatch.Label == nil || *dispatch.Label != wantLabel {
		t.Fatalf("dispatch label = %#v, want %q", dispatch.Label, wantLabel)
	}
	gotPreset := dereferenceOverridesValue(dispatch.PresetId)
	if gotPreset != wantPreset {
		t.Fatalf("dispatch preset = %q, want %q", gotPreset, wantPreset)
	}
	if dispatch.Javascript == nil || dispatch.Javascript.ExecutionMode == nil ||
		*dispatch.Javascript.ExecutionMode != "live-provider" {
		t.Fatalf(
			"dispatch %s javascript projection = %#v, want live-provider execution mode",
			dispatch.Id,
			dispatch.Javascript,
		)
	}
}

func assertPartialMockPassthroughChildDispatch(
	t *testing.T,
	dispatch factoryapi.FactorySessionDispatchSummary,
	wantLabel string,
) {
	t.Helper()

	if dispatch.Label == nil || *dispatch.Label != wantLabel {
		t.Fatalf("dispatch label = %#v, want %q", dispatch.Label, wantLabel)
	}
	if dispatch.PresetId != nil && strings.TrimSpace(*dispatch.PresetId) != "" {
		t.Fatalf("dispatch preset = %#v, want empty preset for unmatched child", dispatch.PresetId)
	}
	if dispatch.Javascript == nil || dispatch.Javascript.ExecutionMode == nil ||
		*dispatch.Javascript.ExecutionMode != "live-provider" {
		t.Fatalf(
			"dispatch %s javascript projection = %#v, want live-provider execution mode",
			dispatch.Id,
			dispatch.Javascript,
		)
	}
}

func assertPartialMockMixedPrimaryResult(t *testing.T, result *factoryapi.FactorySessionResult) {
	t.Helper()

	if result == nil || result.ResultStatus != factoryapi.FactorySessionResultStatusFinal {
		t.Fatalf("result = %#v, want FINAL Factory Session result", result)
	}
	if result.PrimaryResult == nil || len(*result.PrimaryResult) != 1 {
		t.Fatalf("primary result = %#v, want exactly one content part", result.PrimaryResult)
	}
	part, err := (*result.PrimaryResult)[0].AsWorkJsonContentPart()
	if err != nil {
		t.Fatalf("decode primary result content part: %v", err)
	}
	encoded, err := json.Marshal(part.Json)
	if err != nil {
		t.Fatalf("encode primary result JSON: %v", err)
	}
	var evidence struct {
		Mocked      map[string]any `json:"mocked"`
		Passthrough map[string]any `json:"passthrough"`
	}
	if err := json.Unmarshal(encoded, &evidence); err != nil {
		t.Fatalf("decode partial mock primary result: %v", err)
	}
	assertPartialMockNamedChildResult(t, evidence.Mocked, childMockedLabel)
	assertPartialMockPassthroughChildResult(t, evidence.Passthrough, childPassthroughLabel)
}

func assertPartialMockNamedChildResult(
	t *testing.T,
	child map[string]any,
	wantLabel string,
) {
	t.Helper()

	if child == nil {
		t.Fatalf("child result = nil, want structured child object")
	}
	if label, _ := child["label"].(string); label != wantLabel {
		t.Fatalf("child label = %#v, want %q", child["label"], wantLabel)
	}
	if mode, _ := child["executionMode"].(string); mode != "live-provider" {
		t.Fatalf("child executionMode = %#v, want live-provider", child["executionMode"])
	}
	output, ok := child["output"].(map[string]any)
	if !ok || output == nil {
		t.Fatalf("child output = %#v, want structured output object", child["output"])
	}
	text, _ := output["text"].(string)
	if !strings.Contains(text, mockWorkerAcceptedOutput) {
		t.Fatalf("child output text = %#v, want mock-worker accept output", output["text"])
	}
	if strings.Contains(text, livePassthroughChildText) {
		t.Fatalf("child output text = %#v, want mock-worker output not passthrough provider text", output["text"])
	}
}

func assertPartialMockPassthroughChildResult(
	t *testing.T,
	child map[string]any,
	wantLabel string,
) {
	t.Helper()

	if child == nil {
		t.Fatalf("child result = nil, want structured child object")
	}
	if label, _ := child["label"].(string); label != wantLabel {
		t.Fatalf("child label = %#v, want %q", child["label"], wantLabel)
	}
	if mode, _ := child["executionMode"].(string); mode != "live-provider" {
		t.Fatalf("child executionMode = %#v, want live-provider", child["executionMode"])
	}
	output, ok := child["output"].(map[string]any)
	if !ok || output == nil {
		t.Fatalf("child output = %#v, want structured output object", child["output"])
	}
	text, _ := output["text"].(string)
	if !strings.Contains(text, livePassthroughChildText) {
		t.Fatalf("child output text = %#v, want injected provider output %q", output["text"], livePassthroughChildText)
	}
	if strings.Contains(text, mockWorkerAcceptedOutput) {
		t.Fatalf("child output text = %#v, want provider output not mock-worker accept text", output["text"])
	}
}

func assertUnknownWorkerOverrideFailureRecord(
	t *testing.T,
	serverURL, sessionID string,
	result *factoryapi.FactorySessionResult,
) {
	t.Helper()

	if result == nil {
		t.Fatal("result = nil, want failed Factory Session result")
	}
	if result.SessionStatus == nil ||
		*result.SessionStatus != factoryapi.FactorySessionDurableLifecycleStatusFailed {
		t.Fatalf("result sessionStatus = %#v, want FAILED", result.SessionStatus)
	}
	if result.ResultStatus != factoryapi.FactorySessionResultStatusUnavailable {
		t.Fatalf("result status = %q, want UNAVAILABLE on invalid worker override", result.ResultStatus)
	}
	if result.PrimaryResult != nil {
		t.Fatalf("primary result = %#v, want nil on invalid worker override", result.PrimaryResult)
	}

	session := readOverridesDurableSession(t, serverURL, sessionID)
	if session.FailureDetail == nil || strings.TrimSpace(session.FailureDetail.Message) == "" {
		t.Fatalf("session failureDetail = %#v, want actionable public failure record", session.FailureDetail)
	}
	message := session.FailureDetail.Message
	if !strings.Contains(message, "unsupported effective modelProvider") {
		t.Fatalf("session failure message = %#v, want unsupported modelProvider diagnostic", message)
	}
	if !strings.Contains(message, unknownOverrideModelProvider) {
		t.Fatalf(
			"session failure message = %#v, want override value %q",
			message,
			unknownOverrideModelProvider,
		)
	}
	for _, leaked := range []string{"stack", "heap", "goja", "VM"} {
		if strings.Contains(message, leaked) {
			t.Fatalf("session failure message leaked non-customer detail %q: %q", leaked, message)
		}
	}
}

func readOverridesDurableSession(
	t *testing.T,
	serverURL, sessionID string,
) factoryapi.FactorySessionDurableReadModel {
	t.Helper()

	response := support.GetJSON[factoryapi.FactorySessionGetResponse](
		t,
		strings.TrimSuffix(serverURL, "/")+"/factory-sessions/"+sessionID,
	)
	session, err := response.AsFactorySessionDurableReadModel()
	if err != nil {
		t.Fatalf("decode durable session read model: %v", err)
	}
	return session
}

func overridesFactoryConfig() map[string]any {
	return map[string]any{
		"workTypes": []map[string]any{
			{
				"name": "task",
				"states": []map[string]string{
					{"name": "init", "type": "INITIAL"},
					{"name": "complete", "type": "TERMINAL"},
					{"name": "failed", "type": "FAILED"},
				},
			},
		},
		"workers": []map[string]string{
			{"name": "worker-a"},
		},
		"workstations": []map[string]any{
			{
				"name":      "process",
				"worker":    "worker-a",
				"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
				"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
				"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
			},
		},
	}
}

func startOverridesWorkflow(
	t *testing.T,
	serverURL, requestID, workflowSource string,
) factoryapi.FactorySessionSyncExecutionResponse {
	t.Helper()

	dialect := "you-workflow-v1"
	payload, err := json.Marshal(factoryapi.FactorySessionExecutionRequest{
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
	})
	if err != nil {
		t.Fatalf("marshal overrides workflow request: %v", err)
	}
	endpoint := strings.TrimSuffix(serverURL, "/") + "/factory-sessions/sync"
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("build overrides workflow request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("start overrides workflow: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		var body bytes.Buffer
		_, _ = body.ReadFrom(response.Body)
		t.Fatalf("start overrides workflow status = %d: %s", response.StatusCode, body.String())
	}
	var started factoryapi.FactorySessionSyncExecutionResponse
	if err := json.NewDecoder(response.Body).Decode(&started); err != nil {
		t.Fatalf("decode overrides workflow response: %v", err)
	}
	return started
}

func assertPerChildProviderModelDispatches(
	t *testing.T,
	dispatches []factoryapi.FactorySessionDispatchSummary,
) {
	t.Helper()

	if len(dispatches) != 2 {
		t.Fatalf("dispatch count = %d, want 2 child dispatches", len(dispatches))
	}
	byLabel := make(map[string]factoryapi.FactorySessionDispatchSummary, len(dispatches))
	for _, dispatch := range dispatches {
		if dispatch.Label == nil || strings.TrimSpace(*dispatch.Label) == "" {
			t.Fatalf("dispatch %s missing label, want labeled child dispatches", dispatch.Id)
		}
		if dispatch.Status != factoryapi.FactoryDispatchStatusCOMPLETED {
			t.Fatalf("dispatch %s status = %q, want COMPLETED", dispatch.Id, dispatch.Status)
		}
		byLabel[*dispatch.Label] = dispatch
	}
	assertOverridesDispatchSelection(
		t,
		byLabel[childCodexLabel],
		childCodexLabel,
		"codex",
		"codex-child-model",
	)
	assertOverridesDispatchSelection(
		t,
		byLabel[childClaudeLabel],
		childClaudeLabel,
		"claude",
		"claude-child-model",
	)
}

func assertOverridesDispatchSelection(
	t *testing.T,
	dispatch factoryapi.FactorySessionDispatchSummary,
	wantLabel, wantProvider, wantModel string,
) {
	t.Helper()

	if dispatch.Label == nil || *dispatch.Label != wantLabel {
		t.Fatalf("dispatch label = %#v, want %q", dispatch.Label, wantLabel)
	}
	gotProvider := dereferenceOverridesValue(dispatch.ModelProvider)
	gotModel := dereferenceOverridesValue(dispatch.Model)
	if gotProvider != wantProvider || gotModel != wantModel {
		t.Fatalf(
			"dispatch %s selection = provider=%q model=%q, want provider=%q model=%q",
			dispatch.Id,
			gotProvider,
			gotModel,
			wantProvider,
			wantModel,
		)
	}
}

func assertPerChildProviderModelRequests(
	t *testing.T,
	calls []workers.ProviderInferenceRequest,
) {
	t.Helper()

	if len(calls) != 2 {
		t.Fatalf("provider call count = %d, want 2", len(calls))
	}
	if calls[0].ModelProvider != "codex" || calls[0].Model != "codex-child-model" {
		t.Fatalf(
			"first provider request = provider %q model %q, want codex/codex-child-model",
			calls[0].ModelProvider,
			calls[0].Model,
		)
	}
	if calls[1].ModelProvider != "claude" || calls[1].Model != "claude-child-model" {
		t.Fatalf(
			"second provider request = provider %q model %q, want claude/claude-child-model",
			calls[1].ModelProvider,
			calls[1].Model,
		)
	}
}

func dereferenceOverridesValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
