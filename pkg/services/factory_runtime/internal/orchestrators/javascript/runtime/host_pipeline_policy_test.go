package workflowruntime_test

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	workflowpolicy "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/orchestratorcontract"
)

// Parallel child execution assertions live in this existing policy test file so
// the runtime package stays within both the per-file and package file-count
// maintainability limits.
func assertParallelResultOrder(t *testing.T, sessionID string, outcome factory.JavaScriptRuntimeOutcome, wantLabels []string) {
	t.Helper()
	results, ok := projectPrimaryJSON(t, sessionID, outcome.Value)["results"].([]any)
	if !ok || len(results) != len(wantLabels) {
		t.Fatalf("projected results = %#v, want %d entries", results, len(wantLabels))
	}
	for i, wantLabel := range wantLabels {
		child, ok := results[i].(map[string]any)
		if !ok || child["label"] != wantLabel || child["status"] != factory.JavaScriptChildDispatchStatusCompleted {
			t.Fatalf("results[%d] = %#v, want completed child %q", i, results[i], wantLabel)
		}
	}
}

func peakConcurrentChildRunning(records []factory.JavaScriptRuntimeRecord) int {
	running, peak := 0, 0
	for _, record := range records {
		if record.Kind != factory.JavaScriptRecordKindChildDispatch || record.ChildDispatch == nil {
			continue
		}
		switch record.ChildDispatch.Status {
		case factory.JavaScriptChildDispatchStatusRunning:
			running++
			if running > peak {
				peak = running
			}
		case factory.JavaScriptChildDispatchStatusCompleted, factory.JavaScriptChildDispatchStatusFailed:
			running--
		}
	}
	return peak
}

func hasChildDispatchStatus(records []factory.JavaScriptRuntimeRecord, status string) bool {
	for _, record := range records {
		if record.Kind == factory.JavaScriptRecordKindChildDispatch && record.ChildDispatch != nil && record.ChildDispatch.Status == status {
			return true
		}
	}
	return false
}

func completedForLabel(records []factory.JavaScriptRuntimeRecord, label string) bool {
	for _, record := range records {
		if record.Kind == factory.JavaScriptRecordKindChildDispatch && record.ChildDispatch != nil && record.ChildDispatch.Label == label && record.ChildDispatch.Status == factory.JavaScriptChildDispatchStatusCompleted {
			return true
		}
	}
	return false
}

func TestRun_PipelineStagedFakeChildren_PreservesItemStageOrder(t *testing.T) {
	source := readFixture(t, "pipeline-staged-fake-children.workflow.js")
	policy := workflowpolicy.DefaultEffectivePolicy()
	policy.MaxAgents = 16

	req := factory.JavaScriptRuntimeRequest{
		Source:    source,
		SourceRef: "pipeline-staged-fake-children.workflow.js",
		SessionID: "session-pipeline-staged-fake-children",
		Args:      marshalArgs(t, map[string]any{"subject": "workflows"}),
		Metadata: map[string]string{
			"name": "pipeline-staged-fake-children",
		},
		Policy: policy,
	}

	first := runSuccessful(t, req)
	second := runSuccessful(t, req)

	assertPipelineItemOrder(t, first, []string{"alpha", "beta", "gamma"})
	assertPipelineStageTransitions(t, first, 3, 2)
	assertPipelineReviewUsesEditOutput(t, first)

	if string(first.Value.JSON) != string(second.Value.JSON) {
		t.Fatalf("value drift across runs: first=%s second=%s", first.Value.JSON, second.Value.JSON)
	}
}

func TestRun_PipelineStagedFakeChildren_RepresentsStageFailureExplicitly(t *testing.T) {
	source := readFixture(t, "pipeline-stage-failure.workflow.js")
	policy := workflowpolicy.DefaultEffectivePolicy()
	policy.MaxAgents = 16

	req := factory.JavaScriptRuntimeRequest{
		Source:    source,
		SourceRef: "pipeline-stage-failure.workflow.js",
		SessionID: "session-pipeline-stage-failure",
		Args:      marshalArgs(t, map[string]any{"subject": "workflows"}),
		Metadata: map[string]string{
			"name": "pipeline-stage-failure",
		},
		Policy: policy,
	}

	outcome := runSuccessful(t, req)
	projected := projectPrimaryJSON(t, req.SessionID, outcome.Value)
	results, ok := projected["results"].([]any)
	if !ok || len(results) != 2 {
		t.Fatalf("projected results = %#v, want 2 entries", projected["results"])
	}

	successItem, ok := results[0].(map[string]any)
	if !ok || successItem["status"] != factory.JavaScriptChildDispatchStatusCompleted {
		t.Fatalf("results[0] = %#v, want completed item", results[0])
	}
	failedItem, ok := results[1].(map[string]any)
	if !ok || failedItem["status"] != factory.JavaScriptChildDispatchStatusFailed {
		t.Fatalf("results[1].status = %#v, want FAILED", failedItem["status"])
	}
	stages, ok := failedItem["stages"].([]any)
	if !ok || len(stages) != 2 {
		t.Fatalf("results[1].stages = %#v, want 2 stages", failedItem["stages"])
	}
	editStage, ok := stages[0].(map[string]any)
	if !ok || editStage["status"] != factory.JavaScriptChildDispatchStatusCompleted {
		t.Fatalf("results[1].stages[0] = %#v, want completed edit stage", stages[0])
	}
	reviewStage, ok := stages[1].(map[string]any)
	if !ok || reviewStage["status"] != factory.JavaScriptChildDispatchStatusFailed {
		t.Fatalf("results[1].stages[1] = %#v, want failed review stage", stages[1])
	}
	if reviewStage["diagnostic"] == "" {
		t.Fatal("results[1].stages[1].diagnostic is empty")
	}
}

func assertPipelineItemOrder(t *testing.T, outcome factory.JavaScriptRuntimeOutcome, wantItems []string) {
	t.Helper()
	projected := projectPrimaryJSON(t, "session-pipeline-staged-fake-children", outcome.Value)
	results, ok := projected["results"].([]any)
	if !ok || len(results) != len(wantItems) {
		t.Fatalf("projected results = %#v, want %d entries", projected["results"], len(wantItems))
	}
	for i, wantItem := range wantItems {
		itemResult, ok := results[i].(map[string]any)
		if !ok {
			t.Fatalf("results[%d] = %#v, want object", i, results[i])
		}
		if itemResult["item"] != wantItem {
			t.Fatalf("results[%d].item = %#v, want %q", i, itemResult["item"], wantItem)
		}
		if itemResult["status"] != factory.JavaScriptChildDispatchStatusCompleted {
			t.Fatalf("results[%d].status = %#v, want COMPLETED", i, itemResult["status"])
		}
	}
}

func assertPipelineStageTransitions(t *testing.T, outcome factory.JavaScriptRuntimeOutcome, wantItems, wantStages int) {
	t.Helper()
	childRecords := childDispatchRecords(outcome.Records)
	wantChildCount := wantItems * wantStages
	if len(childRecords) != wantChildCount*3 {
		t.Fatalf("child_dispatch record count = %d, want %d transitions", len(childRecords), wantChildCount*3)
	}

	labels := completedChildLabels(childRecords)
	wantLabels := make([]string, 0, wantChildCount)
	for i := 0; i < wantItems; i++ {
		wantLabels = append(wantLabels, "edit-"+itoa(i), "review-"+itoa(i))
	}
	if len(labels) != len(wantLabels) {
		t.Fatalf("completed child labels = %#v, want %#v", labels, wantLabels)
	}
	for i, wantLabel := range wantLabels {
		if labels[i] != wantLabel {
			t.Fatalf("completed child labels[%d] = %q, want %q", i, labels[i], wantLabel)
		}
	}
}

func assertPipelineReviewUsesEditOutput(t *testing.T, outcome factory.JavaScriptRuntimeOutcome) {
	t.Helper()
	projected := projectPrimaryJSON(t, "session-pipeline-staged-fake-children", outcome.Value)
	editResult, reviewResult := pipelineFirstItemStageResults(t, projected)
	assertPipelineStageOutputsDiffer(t, editResult, reviewResult)
}

func pipelineFirstItemStageResults(t *testing.T, projected map[string]any) (map[string]any, map[string]any) {
	t.Helper()
	results, ok := projected["results"].([]any)
	if !ok || len(results) == 0 {
		t.Fatalf("projected results = %#v, want non-empty", projected["results"])
	}
	itemResult, ok := results[0].(map[string]any)
	if !ok {
		t.Fatalf("results[0] = %#v, want object", results[0])
	}
	stages, ok := itemResult["stages"].([]any)
	if !ok || len(stages) != 2 {
		t.Fatalf("results[0].stages = %#v, want 2 stages", itemResult["stages"])
	}
	editStage, ok := stages[0].(map[string]any)
	if !ok || editStage["status"] != factory.JavaScriptChildDispatchStatusCompleted {
		t.Fatalf("results[0].stages[0] = %#v, want completed edit stage", stages[0])
	}
	reviewStage, ok := stages[1].(map[string]any)
	if !ok || reviewStage["status"] != factory.JavaScriptChildDispatchStatusCompleted {
		t.Fatalf("results[0].stages[1] = %#v, want completed review stage", stages[1])
	}
	editResult, ok := editStage["result"].(map[string]any)
	if !ok {
		t.Fatalf("results[0].stages[0].result = %#v, want object", editStage["result"])
	}
	reviewResult, ok := reviewStage["result"].(map[string]any)
	if !ok {
		t.Fatalf("results[0].stages[1].result = %#v, want object", reviewStage["result"])
	}
	return editResult, reviewResult
}

func assertPipelineStageOutputsDiffer(t *testing.T, editResult, reviewResult map[string]any) {
	t.Helper()
	editOutput, ok := editResult["output"].(map[string]any)
	if !ok {
		t.Fatalf("edit output = %#v, want object", editResult["output"])
	}
	reviewOutput, ok := reviewResult["output"].(map[string]any)
	if !ok {
		t.Fatalf("review output = %#v, want object", reviewResult["output"])
	}
	editText, _ := editOutput["text"].(string)
	reviewText, _ := reviewOutput["text"].(string)
	if editText == "" || reviewText == "" {
		t.Fatal("edit or review output text is empty")
	}
	if reviewText == editText {
		t.Fatalf("review output %q should differ from edit output %q", reviewText, editText)
	}
	if editResult["label"] != "edit-0" || reviewResult["label"] != "review-0" {
		t.Fatalf("stage labels = edit %#v review %#v, want edit-0 and review-0", editResult["label"], reviewResult["label"])
	}
}

func childDispatchRecords(records []factory.JavaScriptRuntimeRecord) []factory.JavaScriptRuntimeRecord {
	var childRecords []factory.JavaScriptRuntimeRecord
	for _, record := range records {
		if record.Kind == factory.JavaScriptRecordKindChildDispatch {
			childRecords = append(childRecords, record)
		}
	}
	return childRecords
}

func completedChildLabels(records []factory.JavaScriptRuntimeRecord) []string {
	var labels []string
	for _, record := range records {
		if record.ChildDispatch == nil {
			continue
		}
		if record.ChildDispatch.Status != factory.JavaScriptChildDispatchStatusCompleted {
			continue
		}
		if record.ChildDispatch.Label != "" {
			labels = append(labels, record.ChildDispatch.Label)
		}
	}
	return labels
}

func itoa(value int) string {
	return strconv.Itoa(value)
}

func TestRun_PolicyDeniedChildOperations_ReturnStableDiagnostics(t *testing.T) {
	basePolicy := workflowpolicy.DefaultEffectivePolicy()
	basePolicy.MaxAgents = 4
	basePolicy.Concurrency = 2
	basePolicy.AllowedModels = []string{"gpt-allowed"}
	basePolicy.AllowedReasoningEfforts = []string{"low"}
	basePolicy.AllowedCommands = []string{"review"}

	cases := []struct {
		fixture string
		policy  workflowpolicy.EffectivePolicy
		want    string
		code    string
	}{
		{
			fixture: "agent-run-policy-denied-model.workflow.js",
			policy:  basePolicy,
			want:    `policy denied: model "gpt-denied" is not listed in allowedModels`,
		},
		{
			fixture: "agent-run-policy-denied-command.workflow.js",
			policy:  basePolicy,
			want:    `agent.run() does not support field "command"`,
			code:    factory.JavaScriptRuntimeCodePreExecutionInvalid,
		},
		{
			fixture: "agent-run-policy-denied-reasoning.workflow.js",
			policy:  basePolicy,
			want:    `policy denied: reasoningEffort "high" is not listed in allowedReasoningEfforts`,
		},
		{
			fixture: "agent-run-policy-denied-sandbox.workflow.js",
			policy:  basePolicy,
			want:    `agent.run() does not support field "sandbox"`,
			code:    factory.JavaScriptRuntimeCodePreExecutionInvalid,
		},
		{
			fixture: "agent-run-policy-denied-writable-roots.workflow.js",
			policy:  basePolicy,
			want:    `agent.run() does not support field "writableRoots"`,
			code:    factory.JavaScriptRuntimeCodePreExecutionInvalid,
		},
		{
			fixture: "agent-run-policy-denied-network.workflow.js",
			policy:  basePolicy,
			want:    `agent.run() does not support field "network"`,
			code:    factory.JavaScriptRuntimeCodePreExecutionInvalid,
		},
		{
			fixture: "agent-run-policy-denied-concurrency.workflow.js",
			policy:  basePolicy,
			want:    `agent.run() does not support field "concurrency"`,
			code:    factory.JavaScriptRuntimeCodePreExecutionInvalid,
		},
	}

	for _, tc := range cases {
		t.Run(tc.fixture, func(t *testing.T) {
			req := policyDeniedRequest(t, tc.fixture, tc.policy)
			outcome := runExecutionFailure(t, req)
			wantCode := tc.code
			if wantCode == "" {
				wantCode = factory.JavaScriptRuntimeCodeScriptError
			}
			if outcome.Failure.Code != wantCode {
				t.Fatalf("failure code = %q, want %q", outcome.Failure.Code, wantCode)
			}
			if !strings.Contains(outcome.Failure.Message, tc.want) {
				t.Fatalf("failure message = %q, want substring %q", outcome.Failure.Message, tc.want)
			}
			assertNoSuccessfulChildDispatchRecords(t, outcome.Records)
		})
	}
}

func TestRun_PolicyDeniedMaxAgents_SecondChildFailsWithoutDispatchRecords(t *testing.T) {
	policy := workflowpolicy.DefaultEffectivePolicy()
	policy.MaxAgents = 1
	policy.Concurrency = 1

	req := policyDeniedRequest(t, "agent-run-policy-denied-max-agents.workflow.js", policy)
	outcome := runExecutionFailure(t, req)
	want := "policy denied: requested fanout 1 exceeds maxAgents 1"
	if !strings.Contains(outcome.Failure.Message, want) {
		t.Fatalf("failure message = %q, want substring %q", outcome.Failure.Message, want)
	}
	if completedChildRecords(outcome.Records) != 1 {
		t.Fatalf("completed child records = %d, want 1 from first allowed child", completedChildRecords(outcome.Records))
	}
	assertNoSuccessfulChildDispatchRecordsForLabel(t, outcome.Records, "second-child")
}

func TestRun_PolicyDeniedArtifact_DoesNotEmitArtifactRecord(t *testing.T) {
	maxBytes := int64(16)
	policy := workflowpolicy.DefaultEffectivePolicy()
	policy.MaxArtifactBytes = &maxBytes

	req := policyDeniedRequest(t, "workflow-artifact-policy-denied-size.workflow.js", policy)
	outcome := runExecutionFailure(t, req)
	want := "policy denied: artifact content size"
	if !strings.Contains(outcome.Failure.Message, want) {
		t.Fatalf("failure message = %q, want substring %q", outcome.Failure.Message, want)
	}
	for _, record := range outcome.Records {
		if record.Kind == factory.JavaScriptRecordKindArtifact {
			t.Fatalf("records = %#v, want no artifact record after denial", outcome.Records)
		}
	}
}

func TestRun_PolicyDeniedChild_DoesNotRemovePriorProgressRecords(t *testing.T) {
	policy := workflowpolicy.DefaultEffectivePolicy()
	policy.MaxAgents = 4
	policy.AllowedModels = []string{"gpt-allowed"}

	req := policyDeniedRequest(t, "policy-denied-no-partial-records.workflow.js", policy)
	outcome := runExecutionFailure(t, req)
	if !strings.Contains(outcome.Failure.Message, "policy denied: model") {
		t.Fatalf("failure message = %q, want policy denial", outcome.Failure.Message)
	}

	if len(outcome.Records) != 2 {
		t.Fatalf("record count = %d, want phase+log only", len(outcome.Records))
	}
	if outcome.Records[0].Kind != factory.JavaScriptRecordKindPhase {
		t.Fatalf("records[0].kind = %q, want phase", outcome.Records[0].Kind)
	}
	if outcome.Records[1].Kind != factory.JavaScriptRecordKindLog {
		t.Fatalf("records[1].kind = %q, want log", outcome.Records[1].Kind)
	}
	assertNoChildDispatchRecords(t, outcome.Records)
}

func TestRun_ParallelPolicyDeniedItem_RepresentsFailureWithoutSuccessfulChildRecords(t *testing.T) {
	policy := workflowpolicy.DefaultEffectivePolicy()
	policy.MaxAgents = 4
	policy.Concurrency = 2
	policy.AllowedModels = []string{"gpt-allowed"}

	req := policyDeniedRequest(t, "parallel-policy-denied-item.workflow.js", policy)
	outcome := runSuccessful(t, req)

	projected := projectPrimaryJSON(t, req.SessionID, outcome.Value)
	results, ok := projected["results"].([]any)
	if !ok || len(results) != 2 {
		t.Fatalf("projected results = %#v, want 2 entries", projected["results"])
	}

	allowedChild, ok := results[0].(map[string]any)
	if !ok || allowedChild["status"] != factory.JavaScriptChildDispatchStatusCompleted {
		t.Fatalf("results[0] = %#v, want completed child", results[0])
	}
	deniedChild, ok := results[1].(map[string]any)
	if !ok || deniedChild["status"] != factory.JavaScriptChildDispatchStatusFailed {
		t.Fatalf("results[1] = %#v, want failed child", results[1])
	}
	if !strings.Contains(deniedChild["diagnostic"].(string), "policy denied: model") {
		t.Fatalf("results[1].diagnostic = %#v, want policy denial", deniedChild["diagnostic"])
	}

	assertNoSuccessfulChildDispatchRecordsForLabel(t, outcome.Records, "denied-child")
	if !completedForLabel(outcome.Records, "allowed-child") {
		t.Fatal("expected completed child_dispatch records for allowed parallel child")
	}
}

func policyDeniedRequest(t *testing.T, fixture string, policy workflowpolicy.EffectivePolicy) factory.JavaScriptRuntimeRequest {
	t.Helper()
	name := strings.TrimSuffix(fixture, ".workflow.js")
	return factory.JavaScriptRuntimeRequest{
		Source:    readFixture(t, fixture),
		SourceRef: fixture,
		SessionID: "session-" + name,
		Args:      marshalArgs(t, map[string]any{"subject": "workflows"}),
		Metadata: map[string]string{
			"name": name,
		},
		Policy: policy,
	}
}

func assertNoSuccessfulChildDispatchRecords(t *testing.T, records []factory.JavaScriptRuntimeRecord) {
	t.Helper()
	for _, record := range records {
		if record.Kind != factory.JavaScriptRecordKindChildDispatch || record.ChildDispatch == nil {
			continue
		}
		switch record.ChildDispatch.Status {
		case factory.JavaScriptChildDispatchStatusQueued,
			factory.JavaScriptChildDispatchStatusRunning,
			factory.JavaScriptChildDispatchStatusCompleted:
			t.Fatalf("records = %#v, want no successful child dispatch records after denial", records)
		}
	}
}

func assertNoSuccessfulChildDispatchRecordsForLabel(t *testing.T, records []factory.JavaScriptRuntimeRecord, label string) {
	t.Helper()
	for _, record := range records {
		if record.Kind != factory.JavaScriptRecordKindChildDispatch || record.ChildDispatch == nil {
			continue
		}
		if record.ChildDispatch.Label != label {
			continue
		}
		switch record.ChildDispatch.Status {
		case factory.JavaScriptChildDispatchStatusQueued,
			factory.JavaScriptChildDispatchStatusRunning,
			factory.JavaScriptChildDispatchStatusCompleted:
			t.Fatalf("records = %#v, want no successful child dispatch records for label %q", records, label)
		}
	}
}

func assertNoChildDispatchRecords(t *testing.T, records []factory.JavaScriptRuntimeRecord) {
	t.Helper()
	for _, record := range records {
		if record.Kind == factory.JavaScriptRecordKindChildDispatch {
			t.Fatalf("records = %#v, want no child dispatch records", records)
		}
	}
}

func completedChildRecords(records []factory.JavaScriptRuntimeRecord) int {
	count := 0
	for _, record := range records {
		if record.Kind == factory.JavaScriptRecordKindChildDispatch &&
			record.ChildDispatch != nil &&
			record.ChildDispatch.Status == factory.JavaScriptChildDispatchStatusCompleted {
			count++
		}
	}
	return count
}

func TestRun_ParallelFakeChildren_PreservesInputOrderAndConcurrency(t *testing.T) {
	source := readFixture(t, "parallel-fake-children.workflow.js")
	policy := workflowpolicy.DefaultEffectivePolicy()
	policy.MaxAgents = 8
	policy.Concurrency = 2

	req := factory.JavaScriptRuntimeRequest{
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

	if string(first.Value.JSON) != string(second.Value.JSON) {
		t.Fatalf("value drift across runs: first=%s second=%s", first.Value.JSON, second.Value.JSON)
	}
}

func TestRun_ParallelObjectChildren_ResolveWorkerSettings(t *testing.T) {
	var mu sync.Mutex
	captured := make(map[string]factory.JavaScriptChildExecutionRequest)
	req := factory.JavaScriptRuntimeRequest{
		Source: `return parallel([
			{label: "child-preset", preset: "child", prompt: "one"},
			{label: "factory-preset", preset: "factory", prompt: "two"},
			{label: "scalar-defaults", prompt: "three"}
		]);`,
		SessionID: "session-parallel-worker-settings",
		Agents: map[string]interfaces.FactoryOrchestratorJavaScriptAgent{
			"reviewer": {Preset: "factory"},
		},
		WorkerSettings: factory.JavaScriptWorkerSettings{
			Presets: map[string]factory.JavaScriptWorkerPreset{
				"child":   {ModelProvider: "claude", Model: "child-model", ReasoningEffort: "high"},
				"factory": {ModelProvider: "codex", Model: "factory-model", ReasoningEffort: "low"},
			},
			DefaultModelProvider: "gemini",
			DefaultModel:         "default-model",
		},
	}
	outcome, err := runtimeWorkflows.Run(context.Background(), req, factory.JavaScriptRuntimeHooks{
		NewChildExecutor: func(_ string, _ factory.JavaScriptChildRecordSink, _ workflowpolicy.EffectivePolicy) factory.JavaScriptChildExecutor {
			return childExecutorFunc(func(_ context.Context, child factory.JavaScriptChildExecutionRequest) (factory.JavaScriptChildExecutionResult, error) {
				mu.Lock()
				captured[child.Label] = child
				mu.Unlock()
				return factory.JavaScriptChildExecutionResult{Status: factory.JavaScriptChildDispatchStatusCompleted, Request: child}, nil
			})
		},
	})
	if err != nil || !outcome.OK {
		t.Fatalf("Run() outcome=%#v err=%v", outcome, err)
	}
	assertWorkerSelection(t, captured["child-preset"], "child", "claude", "child-model", "high")
	assertWorkerSelection(t, captured["factory-preset"], "factory", "codex", "factory-model", "low")
	assertWorkerSelection(t, captured["scalar-defaults"], "", "gemini", "default-model", "")
}

func TestRun_ParallelDynamicUnsupportedFieldDoesNotConsumeDispatchIdentity(t *testing.T) {
	stub := &stubChildExecutor{mode: stubChildExecutionMode}
	outcome, err := runtimeWorkflows.Run(context.Background(), factory.JavaScriptRuntimeRequest{
		Source: `
			const rejected = {prompt: "prompt-secret"};
			rejected["writable" + "Roots"] = ["value-secret"];
			return (async () => ({results: await parallel([rejected, {label: "valid", prompt: "review"}])}))();
		`,
		SessionID: "session-parallel-dynamic-unsupported-field",
	}, factory.JavaScriptRuntimeHooks{
		NewChildExecutor: func(_ string, _ factory.JavaScriptChildRecordSink, _ workflowpolicy.EffectivePolicy) factory.JavaScriptChildExecutor {
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
	if !ok || rejected["status"] != factory.JavaScriptChildDispatchStatusFailed {
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
	outcome, err := runtimeWorkflows.Run(context.Background(), factory.JavaScriptRuntimeRequest{
		Source: `
			const rejected = {prompt: "prompt-secret"};
			rejected["writable" + "Roots"] = ["value-secret"];
			return (async () => ({results: await parallel([rejected, {label: "valid", prompt: "review"}])}))();
		`,
		SessionID: "session-parallel-dynamic-unsupported-field-tight-fanout",
		Policy:    policy,
	}, factory.JavaScriptRuntimeHooks{
		NewChildExecutor: func(_ string, _ factory.JavaScriptChildRecordSink, _ workflowpolicy.EffectivePolicy) factory.JavaScriptChildExecutor {
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
	if !ok || rejected["status"] != factory.JavaScriptChildDispatchStatusFailed {
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
	outcome, err := runtimeWorkflows.Run(context.Background(), factory.JavaScriptRuntimeRequest{
		Source:    `return (async () => ({results: await parallel([{label: "denied", preset: "careful", prompt: "review"}])}))();`,
		SessionID: "session-parallel-worker-policy",
		Policy:    policy,
		WorkerSettings: factory.JavaScriptWorkerSettings{Presets: map[string]factory.JavaScriptWorkerPreset{
			"careful": {ModelProvider: "codex", Model: "denied-model", ReasoningEffort: "high"},
		}},
	}, factory.JavaScriptRuntimeHooks{
		NewChildExecutor: func(_ string, _ factory.JavaScriptChildRecordSink, _ workflowpolicy.EffectivePolicy) factory.JavaScriptChildExecutor {
			return childExecutorFunc(func(_ context.Context, child factory.JavaScriptChildExecutionRequest) (factory.JavaScriptChildExecutionResult, error) {
				calls++
				return factory.JavaScriptChildExecutionResult{Status: factory.JavaScriptChildDispatchStatusCompleted, Request: child}, nil
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
	if !ok || denied["status"] != factory.JavaScriptChildDispatchStatusFailed ||
		!strings.Contains(denied["diagnostic"].(string), `policy denied: model "denied-model"`) {
		t.Fatalf("denied result = %#v", results[0])
	}
}

func assertWorkerSelection(t *testing.T, got factory.JavaScriptChildExecutionRequest, preset, provider, model, effort string) {
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

	req := factory.JavaScriptRuntimeRequest{
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
	if outcome.Failure.Code != factory.JavaScriptRuntimeCodeScriptError {
		t.Fatalf("failure code = %q, want %q", outcome.Failure.Code, factory.JavaScriptRuntimeCodeScriptError)
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

	req := factory.JavaScriptRuntimeRequest{
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
	if !ok || successChild["status"] != factory.JavaScriptChildDispatchStatusCompleted {
		t.Fatalf("results[0] = %#v, want completed child", results[0])
	}
	failedChild, ok := results[1].(map[string]any)
	if !ok {
		t.Fatalf("results[1] = %#v, want failed child object", results[1])
	}
	if failedChild["status"] != factory.JavaScriptChildDispatchStatusFailed {
		t.Fatalf("results[1].status = %#v, want FAILED", failedChild["status"])
	}
	if failedChild["diagnostic"] == "" {
		t.Fatalf("results[1].diagnostic is empty")
	}
	if failedChild["artifactRef"] != nil {
		t.Fatalf("results[1].artifactRef = %#v, want absent on failed child", failedChild["artifactRef"])
	}

	if !hasChildDispatchStatus(outcome.Records, factory.JavaScriptChildDispatchStatusFailed) {
		t.Fatal("expected FAILED child_dispatch record for simulated child failure")
	}
	if completedForLabel(outcome.Records, "child-1") {
		t.Fatal("failed child should not emit COMPLETED child_dispatch record")
	}
}
