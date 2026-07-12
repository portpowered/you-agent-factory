package workflowruntime_test

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/orchestrators/javascript/policy"
	"github.com/portpowered/infinite-you/pkg/orchestrators/javascript/runtime"
)

func TestRun_ParallelFakeChildren_PreservesInputOrderAndConcurrency(t *testing.T) {
	source := readFixture(t, "parallel-fake-children.workflow.js")
	policy := workflowpolicy.DefaultEffectivePolicy()
	policy.MaxAgents = 8
	policy.Concurrency = 2

	req := workflowruntime.Request{
		Source:    source,
		SourceRef: "parallel-fake-children.workflow.js",
		SessionID: "session-parallel-fake-children",
		Args:      marshalArgs(t, map[string]any{"subject": "workflows"}),
		Metadata: map[string]string{
			"name": "parallel-fake-children",
		},
		Policy: policy,
	}

	first := runSuccessful(t, req)
	second := runSuccessful(t, req)

	wantLabels := []string{"child-0", "child-1", "child-2", "child-3"}
	assertParallelResultOrder(t, req.SessionID, first, wantLabels)
	if peak := peakConcurrentChildRunning(first.Records); peak > policy.Concurrency {
		t.Fatalf("peak concurrent running children = %d, want <= %d", peak, policy.Concurrency)
	}
	completion := childCompletionOrder(first.Records)
	if len(completion) != len(wantLabels) {
		t.Fatalf("completion order = %#v, want %d children", completion, len(wantLabels))
	}
	if completion[0] == wantLabels[0] && completion[len(completion)-1] == wantLabels[len(wantLabels)-1] {
		t.Fatalf("completion order %#v matched input order; want differing completion order", completion)
	}

	if string(first.Value.JSON) != string(second.Value.JSON) {
		t.Fatalf("value drift across runs: first=%s second=%s", first.Value.JSON, second.Value.JSON)
	}
}

func TestRun_ParallelObjectChildren_ResolveWorkerSettings(t *testing.T) {
	var mu sync.Mutex
	captured := make(map[string]workflowruntime.ChildExecutionRequest)
	req := workflowruntime.Request{
		Source: `return parallel([
			{label: "child-preset", preset: "child", prompt: "one"},
			{label: "factory-preset", preset: "factory", prompt: "two"},
			{label: "scalar-defaults", prompt: "three"}
		]);`,
		SessionID: "session-parallel-worker-settings",
		Agents: map[string]interfaces.FactoryOrchestratorJavaScriptAgent{
			"reviewer": {Preset: "factory"},
		},
		WorkerSettings: workflowruntime.WorkerSettingsConfig{
			Presets: map[string]workflowruntime.WorkerPreset{
				"child":   {ModelProvider: "claude", Model: "child-model", ReasoningEffort: "high"},
				"factory": {ModelProvider: "codex", Model: "factory-model", ReasoningEffort: "low"},
			},
			DefaultModelProvider: "gemini",
			DefaultModel:         "default-model",
		},
	}
	outcome, err := workflowruntime.Run(context.Background(), req, workflowruntime.Hooks{
		NewChildExecutor: func(_ string, _ workflowruntime.ChildRecordSink, _ workflowpolicy.EffectivePolicy) workflowruntime.ChildExecutor {
			return childExecutorFunc(func(_ context.Context, child workflowruntime.ChildExecutionRequest) (workflowruntime.ChildExecutionResult, error) {
				mu.Lock()
				captured[child.Label] = child
				mu.Unlock()
				return workflowruntime.ChildExecutionResult{Status: workflowruntime.ChildDispatchStatusCompleted, Request: child}, nil
			})
		},
	})
	if err != nil || !outcome.OK {
		t.Fatalf("Run() outcome=%#v err=%v", outcome, err)
	}
	assertWorkerSelection(t, captured["child-preset"], "child", "CLAUDE", "child-model", "high")
	assertWorkerSelection(t, captured["factory-preset"], "factory", "CODEX", "factory-model", "low")
	assertWorkerSelection(t, captured["scalar-defaults"], "", "GEMINI", "default-model", "")
}

func TestRun_ParallelDynamicUnsupportedFieldDoesNotConsumeDispatchIdentity(t *testing.T) {
	stub := &stubChildExecutor{mode: stubChildExecutionMode}
	outcome, err := workflowruntime.Run(context.Background(), workflowruntime.Request{
		Source: `
			const rejected = {prompt: "prompt-secret"};
			rejected["writable" + "Roots"] = ["value-secret"];
			return (async () => ({results: await parallel([rejected, {label: "valid", prompt: "review"}])}))();
		`,
		SessionID: "session-parallel-dynamic-unsupported-field",
	}, workflowruntime.Hooks{
		NewChildExecutor: func(_ string, _ workflowruntime.ChildRecordSink, _ workflowpolicy.EffectivePolicy) workflowruntime.ChildExecutor {
			return stub
		},
	})
	if err != nil || !outcome.OK {
		t.Fatalf("Run() outcome=%#v err=%v", outcome, err)
	}

	results, ok := projectPrimaryJSON(t, "session-parallel-dynamic-unsupported-field", outcome.Value)["results"].([]any)
	if !ok || len(results) != 2 {
		t.Fatalf("results = %#v, want two entries", results)
	}
	rejected, ok := results[0].(map[string]any)
	if !ok || rejected["status"] != workflowruntime.ChildDispatchStatusFailed {
		t.Fatalf("rejected result = %#v, want failed child", results[0])
	}
	diagnostic, _ := rejected["diagnostic"].(string)
	if !strings.Contains(diagnostic, `agent.run() does not support field "writableRoots"`) ||
		strings.Contains(diagnostic, "value-secret") || strings.Contains(diagnostic, "prompt-secret") {
		t.Fatalf("diagnostic = %q, want field-specific redacted error", diagnostic)
	}
	assertStubChildResult(t, results[1], "valid", "stub-dispatch-1")

	requests := stub.executionRequests()
	if len(requests) != 1 || requests[0].Label != "valid" {
		t.Fatalf("executor requests = %#v, want only valid child", requests)
	}
	if requests[0].ReservedIdentity == nil || requests[0].ReservedIdentity.DispatchID != "dispatch-1" {
		t.Fatalf("reserved identity = %#v, want dispatch-1", requests[0].ReservedIdentity)
	}
	if len(outcome.Records) != 0 {
		t.Fatalf("records = %#v, want no lifecycle records from rejected item or stub executor", outcome.Records)
	}
}

func TestRun_ParallelDynamicUnsupportedFieldIsValidatedBeforeFanoutPolicy(t *testing.T) {
	stub := &stubChildExecutor{mode: stubChildExecutionMode}
	policy := workflowpolicy.DefaultEffectivePolicy()
	policy.MaxAgents = 1
	outcome, err := workflowruntime.Run(context.Background(), workflowruntime.Request{
		Source: `
			const rejected = {prompt: "prompt-secret"};
			rejected["writable" + "Roots"] = ["value-secret"];
			return (async () => ({results: await parallel([rejected, {label: "valid", prompt: "review"}])}))();
		`,
		SessionID: "session-parallel-dynamic-unsupported-field-tight-fanout",
		Policy:    policy,
	}, workflowruntime.Hooks{
		NewChildExecutor: func(_ string, _ workflowruntime.ChildRecordSink, _ workflowpolicy.EffectivePolicy) workflowruntime.ChildExecutor {
			return stub
		},
	})
	if err != nil || !outcome.OK {
		t.Fatalf("Run() outcome=%#v err=%v", outcome, err)
	}

	results, ok := projectPrimaryJSON(t, "session-parallel-dynamic-unsupported-field-tight-fanout", outcome.Value)["results"].([]any)
	if !ok || len(results) != 2 {
		t.Fatalf("results = %#v, want two entries", results)
	}
	rejected, ok := results[0].(map[string]any)
	if !ok || rejected["status"] != workflowruntime.ChildDispatchStatusFailed {
		t.Fatalf("rejected result = %#v, want failed child", results[0])
	}
	diagnostic, _ := rejected["diagnostic"].(string)
	if !strings.Contains(diagnostic, `agent.run() does not support field "writableRoots"`) {
		t.Fatalf("diagnostic = %q, want unsupported field", diagnostic)
	}
	for _, redacted := range []string{"value-secret", "prompt-secret", "maxAgents"} {
		if strings.Contains(diagnostic, redacted) {
			t.Fatalf("diagnostic = %q, must redact %q", diagnostic, redacted)
		}
	}
	assertStubChildResult(t, results[1], "valid", "stub-dispatch-1")

	requests := stub.executionRequests()
	if len(requests) != 1 {
		t.Fatalf("executor requests = %#v, want one valid child", requests)
	}
	if requests[0].Label != "valid" || requests[0].ReservedIdentity == nil || requests[0].ReservedIdentity.DispatchID != "dispatch-1" {
		t.Fatalf("executor request = %#v, want valid child with dispatch-1", requests[0])
	}
	if len(outcome.Records) != 0 {
		t.Fatalf("records = %#v, want no lifecycle records from rejected item or stub executor", outcome.Records)
	}
}

func TestRun_ParallelObjectChild_GatesResolvedPresetBeforeExecutor(t *testing.T) {
	calls := 0
	policy := policyWithWorkerAllowlists([]string{"allowed-model"}, []string{"high"})
	outcome, err := workflowruntime.Run(context.Background(), workflowruntime.Request{
		Source:    `return (async () => ({results: await parallel([{label: "denied", preset: "careful", prompt: "review"}])}))();`,
		SessionID: "session-parallel-worker-policy",
		Policy:    policy,
		WorkerSettings: workflowruntime.WorkerSettingsConfig{Presets: map[string]workflowruntime.WorkerPreset{
			"careful": {ModelProvider: "codex", Model: "denied-model", ReasoningEffort: "high"},
		}},
	}, workflowruntime.Hooks{
		NewChildExecutor: func(_ string, _ workflowruntime.ChildRecordSink, _ workflowpolicy.EffectivePolicy) workflowruntime.ChildExecutor {
			return childExecutorFunc(func(_ context.Context, child workflowruntime.ChildExecutionRequest) (workflowruntime.ChildExecutionResult, error) {
				calls++
				return workflowruntime.ChildExecutionResult{Status: workflowruntime.ChildDispatchStatusCompleted, Request: child}, nil
			})
		},
	})
	if err != nil || !outcome.OK {
		t.Fatalf("Run() outcome=%#v err=%v", outcome, err)
	}
	if calls != 0 || len(outcome.Records) != 0 {
		t.Fatalf("executor calls=%d records=%#v, want no dispatch side effects", calls, outcome.Records)
	}
	projected := projectPrimaryJSON(t, "session-parallel-worker-policy", outcome.Value)
	results, ok := projected["results"].([]any)
	if !ok || len(results) != 1 {
		t.Fatalf("results = %#v, want one denied child", projected["results"])
	}
	denied, ok := results[0].(map[string]any)
	if !ok || denied["status"] != workflowruntime.ChildDispatchStatusFailed ||
		!strings.Contains(denied["diagnostic"].(string), `policy denied: model "denied-model"`) {
		t.Fatalf("denied result = %#v", results[0])
	}
}

func assertWorkerSelection(t *testing.T, got workflowruntime.ChildExecutionRequest, preset, provider, model, effort string) {
	t.Helper()
	if got.Preset != preset || got.ModelProvider != provider || got.Model != model || got.ReasoningEffort != effort {
		t.Fatalf("worker selection = %#v, want preset=%q provider=%q model=%q effort=%q", got, preset, provider, model, effort)
	}
}

func TestRun_ParallelFakeChildren_DeniesFanoutAboveMaxAgents(t *testing.T) {
	source := readFixture(t, "parallel-max-agents-denied.workflow.js")
	policy := workflowpolicy.DefaultEffectivePolicy()
	policy.MaxAgents = 2
	policy.Concurrency = 2

	req := workflowruntime.Request{
		Source:    source,
		SourceRef: "parallel-max-agents-denied.workflow.js",
		SessionID: "session-parallel-max-agents-denied",
		Args:      marshalArgs(t, map[string]any{"subject": "workflows"}),
		Metadata: map[string]string{
			"name": "parallel-max-agents-denied",
		},
		Policy: policy,
	}

	outcome := runExecutionFailure(t, req)
	if outcome.Failure.Code != workflowruntime.CodeScriptError {
		t.Fatalf("failure code = %q, want %q", outcome.Failure.Code, workflowruntime.CodeScriptError)
	}
	want := "policy denied: requested fanout 3 exceeds maxAgents 2"
	if !strings.Contains(outcome.Failure.Message, want) {
		t.Fatalf("failure message = %q, want %q", outcome.Failure.Message, want)
	}
	if len(outcome.Records) != 0 {
		t.Fatalf("records = %#v, want none after maxAgents denial", outcome.Records)
	}
}

func TestRun_ParallelFakeChildren_RepresentsFailedChildExplicitly(t *testing.T) {
	source := readFixture(t, "parallel-child-failure.workflow.js")
	policy := workflowpolicy.DefaultEffectivePolicy()
	policy.MaxAgents = 8
	policy.Concurrency = 2

	req := workflowruntime.Request{
		Source:    source,
		SourceRef: "parallel-child-failure.workflow.js",
		SessionID: "session-parallel-child-failure",
		Args:      marshalArgs(t, map[string]any{"subject": "workflows"}),
		Metadata: map[string]string{
			"name": "parallel-child-failure",
		},
		Policy: policy,
	}

	outcome := runSuccessful(t, req)
	projected := projectPrimaryJSON(t, req.SessionID, outcome.Value)
	results, ok := projected["results"].([]any)
	if !ok || len(results) != 3 {
		t.Fatalf("projected results = %#v, want 3 entries", projected["results"])
	}

	successChild, ok := results[0].(map[string]any)
	if !ok || successChild["status"] != workflowruntime.ChildDispatchStatusCompleted {
		t.Fatalf("results[0] = %#v, want completed child", results[0])
	}
	failedChild, ok := results[1].(map[string]any)
	if !ok {
		t.Fatalf("results[1] = %#v, want failed child object", results[1])
	}
	if failedChild["status"] != workflowruntime.ChildDispatchStatusFailed {
		t.Fatalf("results[1].status = %#v, want FAILED", failedChild["status"])
	}
	if failedChild["diagnostic"] == "" {
		t.Fatalf("results[1].diagnostic is empty")
	}
	if failedChild["artifactRef"] != nil {
		t.Fatalf("results[1].artifactRef = %#v, want absent on failed child", failedChild["artifactRef"])
	}

	if !hasChildDispatchStatus(outcome.Records, workflowruntime.ChildDispatchStatusFailed) {
		t.Fatal("expected FAILED child_dispatch record for simulated child failure")
	}
	if completedForLabel(outcome.Records, "child-1") {
		t.Fatal("failed child should not emit COMPLETED child_dispatch record")
	}
}
