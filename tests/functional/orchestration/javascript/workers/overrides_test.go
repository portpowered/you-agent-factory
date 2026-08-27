// Package workers holds customer functional scenarios for JavaScript per-child
// worker override behavior through the public process and invocation boundary.
package workers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
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

func runJavaScriptAntigravitySuccess(t *testing.T, fixture *javascriptSharedProcessFixture) {
	for _, executorProvider := range []string{"", "SCRIPT_WRAP"} {
		t.Run("executorProvider="+executorProvider, func(t *testing.T) {
			prompt := "shared Antigravity answer " + strings.ToLower(strings.ReplaceAll(executorProvider, "_", "-"))
			runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{
				Stdout: []byte(`{"event":"result","result":{"conversation_id":"js-agy-child","status":"SUCCESS","response":"` + agyJavaScriptChildOutput + `","duration_seconds":1,"num_turns":1,"usage":{"input_tokens":1,"output_tokens":1,"thinking_tokens":0,"cache_read_tokens":0,"total_tokens":2}}}` + "\n"),
			})
			if err := fixture.router.register(prompt, runner); err != nil {
				t.Fatalf("register Antigravity success route: %v", err)
			}
			t.Cleanup(func() {
				if err := fixture.router.unregister(prompt); err != nil {
					t.Errorf("unregister Antigravity success route: %v", err)
				}
			})

			workflow := `return (async function () {
			return await agent.run({
				prompt: "` + prompt + `",
				label: "javascript-antigravity-child",
  executorProvider: "` + executorProvider + `",
  modelProvider: "ANTIGRAVITY",
  model: "` + agyJavaScriptChildModel + `",
  reasoningEffort: "high",
  permissions: "SKIP_PERMISSIONS"
			});
		})();`
			started := fixture.executeInline(t, "antigravity-success-"+executorProvider, workflow)
			if started.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
				session := readOverridesDurableSession(t, fixture.baseURL, started.SessionId)
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
			assertJavaScriptSharedCompletedDispatch(t, fixture, started.SessionId, "antigravity", agyJavaScriptChildModel, "javascript-antigravity-child")
		})
	}
}

func runJavaScriptAntigravityRejection(t *testing.T, fixture *javascriptSharedProcessFixture) {
	const rejection = "Agy does not support a separate reasoning effort."

	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{
		Stderr:   []byte(rejection),
		ExitCode: 1,
	})
	prompt := "shared force the provider rejection"
	if err := fixture.router.register(prompt, runner); err != nil {
		t.Fatalf("register Antigravity rejection route: %v", err)
	}
	t.Cleanup(func() {
		if err := fixture.router.unregister(prompt); err != nil {
			t.Errorf("unregister Antigravity rejection route: %v", err)
		}
	})

	started := fixture.executeInline(t, "antigravity-rejection", `return (async function () {
  return await agent.run({
    prompt: "`+prompt+`",
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
		strings.TrimSuffix(fixture.baseURL, "/")+"/factory-sessions/"+started.SessionId+"/results?mode=final",
	)
	assertUnavailableFactoryResult(t, &durableResult)
	assertReadableAntigravityEvent(t, support.GetFactoryEventsForSessionAt(t, fixture.baseURL, started.SessionId), rejection)

	session := readOverridesDurableSession(t, fixture.baseURL, started.SessionId)
	if session.FailureDetail == nil || !strings.Contains(session.FailureDetail.Message, rejection) {
		t.Fatalf("session failure detail = %#v, want typed provider rejection", session.FailureDetail)
	}
	assertNoGoMapFailure(t, session.FailureDetail.Message)

	dispatches := support.GetJSON[factoryapi.ListFactorySessionDispatchesResponse](
		t,
		strings.TrimSuffix(fixture.baseURL, "/")+"/factory-sessions/"+started.SessionId+"/dispatches",
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

func runJavaScriptLiveProvider(t *testing.T, fixture *javascriptSharedProcessFixture) {
	prompt := "shared use the live provider command edge"
	runner := support.NewRecordingCommandRunner("live provider output")
	if err := fixture.router.register(prompt, runner); err != nil {
		t.Fatalf("register live provider route: %v", err)
	}
	t.Cleanup(func() {
		if err := fixture.router.unregister(prompt); err != nil {
			t.Errorf("unregister live provider route: %v", err)
		}
	})

	workflow := strings.ReplaceAll(liveProviderChildWorkflow, "use the live provider command edge", prompt)
	started := fixture.executeInline(t, "live-provider", workflow)
	if started.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("session status = %q, want SUCCEEDED", started.Status)
	}
	assertSucceededPrimaryContains(t, started, "live provider output")
	if runner.CallCount() != 1 {
		t.Fatalf("provider command runner call count = %d, want one live child invocation", runner.CallCount())
	}
	request := runner.LastRequest()
	if request.Command != "codex" {
		t.Fatalf("provider command = %q, want codex", request.Command)
	}

	dispatches := support.GetJSON[factoryapi.ListFactorySessionDispatchesResponse](
		t,
		strings.TrimSuffix(fixture.baseURL, "/")+"/factory-sessions/"+started.SessionId+"/dispatches",
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
}

func runJavaScriptProviderPermissionFlags(t *testing.T, fixture *javascriptSharedProcessFixture) {
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
			prompt := "shared permissions mapping " + strings.ReplaceAll(test.name, " ", "-")
			workflow := permissionsOverrideWorkflowWithPrompt(test.fields, test.dynamic, prompt)
			runner := support.NewRecordingCommandRunner("permissions provider output")
			if err := fixture.router.register(prompt, runner); err != nil {
				t.Fatalf("register permission flag route: %v", err)
			}
			t.Cleanup(func() {
				if err := fixture.router.unregister(prompt); err != nil {
					t.Errorf("unregister permission flag route: %v", err)
				}
			})
			started := fixture.executeInline(t, test.name, workflow)
			if started.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
				t.Fatalf("permission mapping status = %q, want SUCCEEDED; result=%#v", started.Status, started.Result)
			}
			assertSucceededPrimaryContains(t, started, "permissions provider output")
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
			assertJavaScriptSharedCompletedDispatch(t, fixture, started.SessionId, "codex", "permissions-child-model", "permissions-child")
		})
	}
}

func runJavaScriptInvalidPermissions(t *testing.T, fixture *javascriptSharedProcessFixture) {
	for _, test := range []struct {
		name  string
		value string
	}{
		{name: "invalid-enum", value: `"READ_ONLY"`},
		{name: "invalid-type", value: "true"},
	} {
		t.Run(test.name, func(t *testing.T) {
			prompt := "shared invalid permissions " + test.name
			workflow := invalidPermissionsOverrideWorkflowWithValue(test.value, prompt)
			beforeCalls := fixture.router.callCount()
			started := fixture.executeInline(t, test.name, workflow)
			if started.Status != factoryapi.FactorySessionDurableLifecycleStatusFailed {
				t.Fatalf("invalid permissions status = %q, want FAILED; result=%#v", started.Status, started.Result)
			}
			assertUnavailableFactoryResult(t, started.Result)
			if started.Result.PrimaryResult != nil {
				t.Fatalf("invalid permissions primary result = %#v, want nil", started.Result.PrimaryResult)
			}
			if fixture.router.callCount() != beforeCalls {
				t.Fatalf("provider command calls after invalid permissions = %d, want unchanged %d", fixture.router.callCount(), beforeCalls)
			}
			assertJavaScriptSharedNoDispatch(t, fixture, started.SessionId)
			session := readOverridesDurableSession(t, fixture.baseURL, started.SessionId)
			if session.FailureDetail == nil {
				t.Fatalf("invalid permissions session = %#v, want failure detail", session)
			}
			diagnostic := strings.ToLower(session.FailureDetail.Message)
			if !strings.Contains(diagnostic, "permissions") {
				t.Fatalf("invalid permissions diagnostic = %q, want field-specific permissions detail", diagnostic)
			}
		})
	}
}

func invalidPermissionsOverrideWorkflow() string {
	return invalidPermissionsOverrideWorkflowWithValue("true", "shared invalid permissions")
}

func invalidPermissionsOverrideWorkflowWithValue(value, prompt string) string {
	return `return (async function () { const child = { prompt: "` + prompt + `", modelProvider: "codex" }; child.permissions = ` + value + `; return await agent.run(child); })();`
}

func runJavaScriptDistinctProviderModels(t *testing.T, fixture *javascriptSharedProcessFixture) {
	codexPrompt := "shared codex provider and model"
	claudePrompt := "shared claude provider and model"
	codexRunner := support.NewRecordingCommandRunner("codex child complete")
	claudeRunner := support.NewRecordingCommandRunner("claude child complete")
	for selector, runner := range map[string]platformprocess.CommandRunner{
		codexPrompt:  codexRunner,
		claudePrompt: claudeRunner,
	} {
		if err := fixture.router.register(selector, runner); err != nil {
			t.Fatalf("register provider/model route %q: %v", selector, err)
		}
		selector := selector
		t.Cleanup(func() {
			if err := fixture.router.unregister(selector); err != nil {
				t.Errorf("unregister provider/model route %q: %v", selector, err)
			}
		})
	}
	workflow := strings.ReplaceAll(perChildProviderModelWorkflow, "use codex provider and model", codexPrompt)
	workflow = strings.ReplaceAll(workflow, "use claude provider and model", claudePrompt)
	beforeRequests := fixture.router.requestRecords()
	started := fixture.executeInline(t, "distinct-provider-model", workflow)
	if started.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("provider/model session status = %q, want SUCCEEDED", started.Status)
	}
	assertSucceededPrimaryContains(t, started, "codex child complete", "claude child complete")

	dispatches := support.GetJSON[factoryapi.ListFactorySessionDispatchesResponse](
		t,
		strings.TrimSuffix(fixture.baseURL, "/")+"/factory-sessions/"+started.SessionId+"/dispatches",
	)
	assertPerChildProviderModelDispatches(t, dispatches.Dispatches)
	afterRequests := fixture.router.requestRecords()
	if len(afterRequests) != len(beforeRequests)+2 {
		t.Fatalf("provider/model command request count = %d, want 2 new requests", len(afterRequests)-len(beforeRequests))
	}
	assertJavaScriptProviderModelCommandRequests(t, afterRequests[len(beforeRequests):], codexPrompt, claudePrompt)
}

func runJavaScriptPartialMockWorkers(t *testing.T, fixture *javascriptSharedProcessFixture) {
	runner := support.NewRecordingCommandRunner(livePassthroughChildText)
	if err := fixture.router.register(passthroughChildPrompt, runner); err != nil {
		t.Fatalf("register partial mock passthrough route: %v", err)
	}
	t.Cleanup(func() {
		if err := fixture.router.unregister(passthroughChildPrompt); err != nil {
			t.Errorf("unregister partial mock passthrough route: %v", err)
		}
	})
	started := fixture.executeInline(t, "partial-mock-workers", partialMockWorkersWorkflow)
	if started.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("session status = %q, want SUCCEEDED", started.Status)
	}
	if runner.CallCount() != 1 {
		t.Fatalf("provider command runner call count = %d, want 1 passthrough child dispatch", runner.CallCount())
	}

	dispatches := support.GetJSON[factoryapi.ListFactorySessionDispatchesResponse](
		t,
		strings.TrimSuffix(fixture.baseURL, "/")+"/factory-sessions/"+started.SessionId+"/dispatches",
	)
	assertPartialMockMixedChildDispatches(t, dispatches.Dispatches)
	assertPartialMockMixedPrimaryResult(t, started.Result)
}

func runJavaScriptUnknownProviderOverride(t *testing.T, fixture *javascriptSharedProcessFixture) {
	beforeCalls := fixture.router.callCount()
	started := fixture.executeInline(t, "unknown-provider-override", unknownWorkerOverrideWorkflow)
	if started.Status != factoryapi.FactorySessionDurableLifecycleStatusFailed {
		t.Fatalf("session status = %q, want FAILED", started.Status)
	}
	if fixture.router.callCount() != beforeCalls {
		t.Fatalf("provider command runner call count = %d, want unchanged %d before override validation", fixture.router.callCount(), beforeCalls)
	}

	dispatches := support.GetJSON[factoryapi.ListFactorySessionDispatchesResponse](
		t,
		strings.TrimSuffix(fixture.baseURL, "/")+"/factory-sessions/"+started.SessionId+"/dispatches",
	)
	if len(dispatches.Dispatches) != 0 {
		t.Fatalf("dispatch count = %d, want 0 for invalid worker override", len(dispatches.Dispatches))
	}

	assertJavaScriptSharedNoDispatch(t, fixture, started.SessionId)
	assertUnknownWorkerOverrideFailureRecord(t, fixture.baseURL, started.SessionId, started.Result)
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
	return permissionsOverrideWorkflowWithPrompt(fields, dynamic, "prove the provider permission mapping")
}

func permissionsOverrideWorkflowWithPrompt(fields string, dynamic bool, prompt string) string {
	child := `{
    prompt: "` + prompt + `",
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
	started, err := postOverridesWorkflow(t.Context(), serverURL, requestID, workflowSource)
	if err != nil {
		t.Fatalf("start overrides workflow: %v", err)
	}
	return started
}

func postOverridesWorkflow(
	ctx context.Context,
	serverURL, requestID, workflowSource string,
) (factoryapi.FactorySessionSyncExecutionResponse, error) {
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
		return factoryapi.FactorySessionSyncExecutionResponse{}, fmt.Errorf("marshal overrides workflow request: %w", err)
	}
	endpoint := strings.TrimSuffix(serverURL, "/") + "/factory-sessions/sync"
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return factoryapi.FactorySessionSyncExecutionResponse{}, fmt.Errorf("build overrides workflow request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return factoryapi.FactorySessionSyncExecutionResponse{}, fmt.Errorf("POST overrides workflow: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		var body bytes.Buffer
		_, _ = body.ReadFrom(response.Body)
		return factoryapi.FactorySessionSyncExecutionResponse{}, fmt.Errorf("overrides workflow status = %d: %s", response.StatusCode, body.String())
	}
	var started factoryapi.FactorySessionSyncExecutionResponse
	if err := json.NewDecoder(response.Body).Decode(&started); err != nil {
		return factoryapi.FactorySessionSyncExecutionResponse{}, fmt.Errorf("decode overrides workflow response: %w", err)
	}
	return started, nil
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

func assertJavaScriptProviderModelCommandRequests(
	t *testing.T,
	requests []platformprocess.CommandRequest,
	codexPrompt, claudePrompt string,
) {
	t.Helper()

	if len(requests) != 2 {
		t.Fatalf("provider command request count = %d, want 2", len(requests))
	}
	if requests[0].Command != "codex" || !javaScriptCommandRequestContains(requests[0], codexPrompt) ||
		!containsArgPair(requests[0].Args, "--model", "codex-child-model") {
		t.Fatalf(
			"first provider command request = %#v, want codex/%q with codex-child-model",
			requests[0],
			codexPrompt,
		)
	}
	if requests[1].Command != "claude" || !javaScriptCommandRequestContains(requests[1], claudePrompt) ||
		!containsArgPair(requests[1].Args, "--model", "claude-child-model") {
		t.Fatalf(
			"second provider command request = %#v, want claude/%q with claude-child-model",
			requests[1],
			claudePrompt,
		)
	}
}

func javaScriptCommandRequestContains(request platformprocess.CommandRequest, want string) bool {
	return bytes.Contains(request.Stdin, []byte(want)) || strings.Contains(strings.Join(request.Args, "\n"), want)
}

func dereferenceOverridesValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
