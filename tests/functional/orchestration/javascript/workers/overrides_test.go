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
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	agyJavaScriptChildModel  = "gemini-3.6-flash-medium"
	agyJavaScriptChildOutput = "real Antigravity JavaScript child output"

	childCodexLabel           = "child-codex"
	childClaudeLabel          = "child-claude"
	childMockedLabel          = "child-mocked"
	childPassthroughLabel     = "child-passthrough"
	unknownOverrideChildLabel = "child-unknown-override"

	mockedWorkerPresetName       = "worker-a"
	mockedChildPrompt            = "mocked child prompt"
	passthroughChildPrompt       = "passthrough child prompt"
	unknownOverrideModelProvider = "Not_A_Provider"
	liveProviderChildWorkflow    = `return (async function () {
  return await agent.run({
    prompt: "use the live provider command edge",
    label: "live-provider-child",
    modelProvider: "codex",
    model: "live-child-model",
  });
})();`

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

// TestJavaScriptAntigravityChildUsesModelEmbeddedEffortThroughRootProcess proves child execution preserves model-embedded effort.
func TestJavaScriptAntigravityChildUsesModelEmbeddedEffortThroughRootProcess(t *testing.T) {
	for _, executorProvider := range []string{"", "SCRIPT_WRAP"} {
		t.Run("executorProvider="+executorProvider, func(t *testing.T) {
			dir := support.ScaffoldFactory(t, overridesFactoryConfig())
			runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{
				Stdout: []byte(`{"event":"result","result":{"conversation_id":"js-agy-child","status":"SUCCESS","response":"` + agyJavaScriptChildOutput + `","duration_seconds":1,"num_turns":1,"usage":{"input_tokens":1,"output_tokens":1,"thinking_tokens":0,"cache_read_tokens":0,"total_tokens":2}}}` + "\n"),
			})
			server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
				FactoryDir:                dir,
				WaitForServiceModeRuntime: true,
				Edges:                     serviceedges.Edges{ProviderCommandRunner: runner},
			})
			t.Cleanup(func() { server.Stop(t) })

			workflow := `return (async function () {
			return await agent.run({
  prompt: "return a real Antigravity answer",
  label: "javascript-antigravity-child",
  executorProvider: "` + executorProvider + `",
  modelProvider: "ANTIGRAVITY",
  model: "` + agyJavaScriptChildModel + `",
  reasoningEffort: "high",
  permissions: "SKIP_PERMISSIONS"
});
})();`
			started := startOverridesWorkflow(t, server.URL(), "javascript-antigravity-"+strings.ToLower(strings.ReplaceAll(executorProvider, "_", "-")), workflow)
			if started.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
				session := readOverridesDurableSession(t, server.URL(), started.SessionId)
				t.Fatalf("session status = %q, want SUCCEEDED; result=%#v failure=%#v", started.Status, started.Result, session.FailureDetail)
			}
			if runner.CallCount() != 1 {
				t.Fatalf("provider command calls = %d, want one", runner.CallCount())
			}
			request := runner.LastRequest()
			if request.Command != "agy" {
				t.Fatalf("provider command = %q, want agy", request.Command)
			}
			if !containsArgPair(request.Args, "--model", agyJavaScriptChildModel) {
				t.Fatalf("provider argv = %#v, want exact model %q", request.Args, agyJavaScriptChildModel)
			}
			if containsArg(request.Args, "--effort") {
				t.Fatalf("provider argv = %#v, want no separate Antigravity effort", request.Args)
			}
			assertSucceededPrimaryContains(t, started, agyJavaScriptChildOutput)
		})
	}
}

// TestJavaScriptAntigravityCommandRejectionRemainsTypedThroughRootProcess proves child command rejection remains typed across the root process.
func TestJavaScriptAntigravityCommandRejectionRemainsTypedThroughRootProcess(t *testing.T) {
	const rejection = "Agy does not support a separate reasoning effort."

	dir := support.ScaffoldFactory(t, overridesFactoryConfig())
	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{
		Stderr:   []byte(rejection),
		ExitCode: 1,
	})
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		Edges:                     serviceedges.Edges{ProviderCommandRunner: runner},
	})
	t.Cleanup(func() { server.Stop(t) })

	started := startOverridesWorkflow(t, server.URL(), "javascript-antigravity-rejection", `return (async function () {
  return await agent.run({
    prompt: "force the provider rejection",
    label: "javascript-antigravity-rejection",
    modelProvider: "ANTIGRAVITY",
    model: "gemini-3.6-flash-medium",
    permissions: "SKIP_PERMISSIONS"
  });
})();`)
	if started.Status != factoryapi.FactorySessionDurableLifecycleStatusFailed {
		t.Fatalf("session status = %q, want FAILED; result=%#v", started.Status, started.Result)
	}
	if runner.CallCount() != 1 {
		t.Fatalf("provider command calls = %d, want one genuine Agy command attempt", runner.CallCount())
	}
	request := runner.LastRequest()
	if request.Command != "agy" || !containsArgPair(request.Args, "--model", agyJavaScriptChildModel) {
		t.Fatalf("provider command request = %#v, want Agy with the requested model", request)
	}
	assertUnavailableFactoryResult(t, started.Result)
	durableResult := support.GetJSON[factoryapi.FactorySessionResult](
		t,
		strings.TrimSuffix(server.URL(), "/")+"/factory-sessions/"+started.SessionId+"/results?mode=final",
	)
	assertUnavailableFactoryResult(t, &durableResult)
	assertReadableAntigravityEvent(t, support.GetFactoryEventsForSessionAt(t, server.URL(), started.SessionId), rejection)

	session := readOverridesDurableSession(t, server.URL(), started.SessionId)
	if session.FailureDetail == nil || !strings.Contains(session.FailureDetail.Message, rejection) {
		t.Fatalf("session failure detail = %#v, want typed provider rejection", session.FailureDetail)
	}
	assertNoGoMapFailure(t, session.FailureDetail.Message)

	dispatches := support.GetJSON[factoryapi.ListFactorySessionDispatchesResponse](
		t,
		strings.TrimSuffix(server.URL(), "/")+"/factory-sessions/"+started.SessionId+"/dispatches",
	)
	if len(dispatches.Dispatches) != 1 {
		t.Fatalf("dispatch count = %d, want one failed provider event", len(dispatches.Dispatches))
	}
	dispatch := dispatches.Dispatches[0]
	if dispatch.Status != factoryapi.FactoryDispatchStatusFAILED ||
		dispatch.Provider == nil || *dispatch.Provider != "antigravity" ||
		dispatch.FailureDetail == nil || !strings.Contains(dispatch.FailureDetail.Message, rejection) {
		t.Fatalf("failed dispatch = %#v, want Antigravity and typed rejection", dispatch)
	}
	assertNoGoMapFailure(t, dispatch.FailureDetail.Message)
}

func assertUnavailableFactoryResult(t *testing.T, result *factoryapi.FactorySessionResult) {
	t.Helper()
	if result == nil || result.ResultStatus != factoryapi.FactorySessionResultStatusUnavailable {
		t.Fatalf("Factory result = %#v, want UNAVAILABLE after provider rejection", result)
	}
}

func assertReadableAntigravityEvent(t *testing.T, events []factoryapi.FactoryEvent, reason string) {
	t.Helper()
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeDispatchReconciled {
			continue
		}
		payload, err := event.Payload.AsDispatchReconciledEventPayload()
		if err != nil {
			t.Fatalf("decode Antigravity dispatch reconciliation event: %v", err)
		}
		if payload.FailureDetail == nil || !strings.Contains(payload.FailureDetail.Message, reason) {
			continue
		}
		assertNoGoMapFailure(t, payload.FailureDetail.Message)
		return
	}
	t.Fatalf("Factory events = %#v, want a typed Antigravity inference rejection", events)
}

func assertNoGoMapFailure(t *testing.T, message string) {
	t.Helper()
	if strings.Contains(message, "map[value:") || strings.Contains(message, "type=unknown") {
		t.Fatalf("failure message = %q, must not expose a stringified Go map", message)
	}
}

// TestJavaScriptChildUsesProviderCommandEdgeThroughRootProcess proves the
// live provider-invocation route is assembled from the Providers root and
// reaches the injected command edge when no mock-worker or provider override
// is present.
func TestJavaScriptChildUsesProviderCommandEdgeThroughRootProcess(t *testing.T) {
	t.Parallel()

	dir := support.ScaffoldFactory(t, overridesFactoryConfig())
	runner := support.NewRecordingCommandRunner("live provider output")
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		Edges:                     serviceedges.Edges{ProviderCommandRunner: runner},
	})
	t.Cleanup(func() { server.Stop(t) })

	started := startOverridesWorkflow(t, server.URL(), "javascript-live-provider-root", liveProviderChildWorkflow)
	if started.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("session status = %q, want SUCCEEDED", started.Status)
	}
	if runner.CallCount() != 1 {
		t.Fatalf("provider command runner call count = %d, want one live child invocation", runner.CallCount())
	}
	request := runner.LastRequest()
	if request.Command != "codex" {
		t.Fatalf("provider command = %q, want codex", request.Command)
	}

	dispatches := support.GetJSON[factoryapi.ListFactorySessionDispatchesResponse](
		t,
		strings.TrimSuffix(server.URL(), "/")+"/factory-sessions/"+started.SessionId+"/dispatches",
	)
	if len(dispatches.Dispatches) != 1 {
		t.Fatalf("dispatch count = %d, want one live provider child", len(dispatches.Dispatches))
	}
	dispatch := dispatches.Dispatches[0]
	if dispatch.Status != factoryapi.FactoryDispatchStatusCOMPLETED ||
		dispatch.ModelProvider == nil || *dispatch.ModelProvider != "codex" ||
		dispatch.Model == nil || *dispatch.Model != "live-child-model" {
		t.Fatalf("live provider dispatch = %#v, want completed codex/live-child-model dispatch", dispatch)
	}

	exerciseJavaScriptChildPermissionsResolveToExistingProviderCommandFlag(t)
	exerciseJavaScriptChildInvalidPermissionsFailsBeforeProviderCommand(t)
}

func exerciseJavaScriptChildPermissionsResolveToExistingProviderCommandFlag(t *testing.T) {
	tests := []struct {
		name       string
		fields     string
		dynamic    bool
		wantBypass bool
	}{
		{name: "permissions default dynamic", fields: `permissions: "DEFAULT"`, dynamic: true, wantBypass: false},
		{name: "permissions skip dynamic", fields: `permissions: "SKIP_PERMISSIONS"`, dynamic: true, wantBypass: true},
		{name: "permissions default static", fields: `permissions: "DEFAULT"`, wantBypass: false},
		{name: "permissions skip static", fields: `permissions: "SKIP_PERMISSIONS"`, wantBypass: true},
		{name: "neither", fields: "", wantBypass: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			workflow := permissionsOverrideWorkflow(test.fields, test.dynamic)
			dir := support.ScaffoldFactory(t, permissionsOverrideFactoryConfig(workflow))
			runner := support.NewRecordingCommandRunner("permissions provider output")
			inputs := support.FakeInputs(t.Context(), []string{
				"you", "--json", "run",
				"--factory", filepath.Join(dir, "factory.json"),
				"--output", "primary",
				"--no-record",
				"permissions matrix prompt",
			})
			inputs.Input.WorkingDirectory = dir
			homeDir := t.TempDir()
			inputs.Input.Env = append(inputs.Input.Env, "HOME="+homeDir, "USERPROFILE="+homeDir)

			if err := support.BuildProcess(t, serviceedges.Edges{
				ProviderCommandRunner: runner,
			}).Execute(inputs.Input); err != nil {
				t.Fatalf("Process.Execute() error = %v\nstdout:\n%s\nstderr:\n%s", err, inputs.Stdout(), inputs.Stderr())
			}
			if runner.CallCount() != 1 {
				t.Fatalf("provider command calls = %d, want one child invocation", runner.CallCount())
			}
			request := runner.LastRequest()
			if request.Command != "codex" {
				t.Fatalf("provider command = %q, want codex", request.Command)
			}
			gotBypass := containsArg(request.Args, "--dangerously-bypass-approvals-and-sandbox")
			if gotBypass != test.wantBypass {
				t.Fatalf("provider argv = %#v, want bypass flag present=%v", request.Args, test.wantBypass)
			}
		})
	}
}

func exerciseJavaScriptChildInvalidPermissionsFailsBeforeProviderCommand(t *testing.T) {
	for _, source := range []string{
		`return (async function () { const child = { prompt: "invalid permissions", modelProvider: "codex" }; child.permissions = "READ_ONLY"; return await agent.run(child); })();`,
	} {
		t.Run(source, func(t *testing.T) {
			dir := support.ScaffoldFactory(t, permissionsOverrideFactoryConfig(source))
			runner := support.NewRecordingCommandRunner("unexpected provider execution")
			inputs := support.FakeInputs(t.Context(), []string{
				"you", "--json", "run",
				"--factory", filepath.Join(dir, "factory.json"),
				"--output", "primary",
				"--no-record",
				"invalid permissions prompt",
			})
			inputs.Input.WorkingDirectory = dir
			homeDir := t.TempDir()
			inputs.Input.Env = append(inputs.Input.Env, "HOME="+homeDir, "USERPROFILE="+homeDir)

			err := support.BuildProcess(t, serviceedges.Edges{
				ProviderCommandRunner: runner,
			}).Execute(inputs.Input)
			if err == nil {
				t.Fatal("Process.Execute() error = nil, want invalid permissions failure")
			}
			diagnostic := strings.ToLower(err.Error() + "\n" + inputs.Stderr())
			if !strings.Contains(diagnostic, "permissions") {
				t.Fatalf("invalid permissions diagnostic = %q, want field-specific permissions detail", diagnostic)
			}
			if runner.CallCount() != 0 {
				t.Fatalf("provider command calls = %d, want zero for invalid permissions", runner.CallCount())
			}
		})
	}
}

func invalidPermissionsOverrideWorkflow() string {
	return `return (async function () { const child = { prompt: "shared invalid permissions", modelProvider: "codex" }; child.permissions = true; return await agent.run(child); })();`
}

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

func permissionsOverrideFactoryConfig(source string) map[string]any {
	config := map[string]any{
		"name": "javascript-permissions",
	}
	config["invocationSignature"] = map[string]any{
		"parameters": []any{map[string]any{
			"name": "prompt", "required": false,
			"bindings": []any{map[string]any{"kind": "POSITIONAL", "position": 1}},
		}},
	}
	config["orchestrator"] = map[string]any{
		"kind": "JAVASCRIPT",
		"javascript": map[string]any{
			"inlineSource": map[string]any{
				"encoding": "utf-8",
				"inline":   source,
			},
			"argsSchema": map[string]any{
				"type":                 "object",
				"properties":           map[string]any{"prompt": map[string]any{"type": "string"}},
				"additionalProperties": false,
			},
		},
	}
	return config
}

func permissionsOverrideWorkflow(fields string, dynamic bool) string {
	child := `{
    prompt: "prove the provider permission mapping",
    label: "permissions-child",
    modelProvider: "codex",
    model: "permissions-child-model"`
	if fields != "" {
		child += ",\n    " + fields
	}
	child += "\n  }"

	workflow := `return (async function () {
  `
	if dynamic {
		workflow += "const child = " + child + ";\n  return await agent.run(child);\n"
	} else {
		workflow += "return await agent.run(" + child + ");\n"
	}
	return workflow + "})();"
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

func containsArg(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}

func containsArgPair(args []string, name, value string) bool {
	for index := 0; index+1 < len(args); index++ {
		if args[index] == name && args[index+1] == value {
			return true
		}
	}
	return false
}

func assertSucceededPrimaryContains(t *testing.T, response factoryapi.FactorySessionSyncExecutionResponse, fragments ...string) {
	t.Helper()
	if response.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("session status = %q, want SUCCEEDED; result = %#v", response.Status, response.Result)
	}
	if response.Result == nil || response.Result.PrimaryResult == nil || len(*response.Result.PrimaryResult) != 1 {
		t.Fatalf("primary result = %#v, want one JavaScript result", response.Result)
	}
	payload, err := json.Marshal((*response.Result.PrimaryResult)[0])
	if err != nil {
		t.Fatalf("marshal primary result: %v", err)
	}
	for _, fragment := range fragments {
		if !strings.Contains(string(payload), fragment) {
			t.Fatalf("primary result = %s, want %q", payload, fragment)
		}
	}
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
