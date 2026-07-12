package workflowruntime_test

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/orchestrators/javascript/policy"
	"github.com/portpowered/infinite-you/pkg/orchestrators/javascript/result"
	"github.com/portpowered/infinite-you/pkg/orchestrators/javascript/runtime"
)

func TestResolveChildWorkerSettings_FieldByFieldPrecedence(t *testing.T) {
	agents := map[string]interfaces.FactoryOrchestratorJavaScriptAgent{"reviewer": {Preset: "factory"}}
	config := workflowruntime.WorkerSettingsConfig{
		Presets: map[string]workflowruntime.WorkerPreset{
			"child":   {ModelProvider: "claude", Model: "child-model", ReasoningEffort: "HIGH"},
			"factory": {ModelProvider: "codex", Model: "factory-model", ReasoningEffort: "low"},
		}, DefaultModelProvider: "gemini", DefaultModel: "scalar-model",
	}
	tests := []struct {
		name      string
		req, want workflowruntime.ChildExecutionRequest
	}{
		{"explicit fields", workflowruntime.ChildExecutionRequest{ModelProvider: "kiro-cli", Model: "explicit-model", ReasoningEffort: "minimal"}, workflowruntime.ChildExecutionRequest{ModelProvider: "KIRO", Model: "explicit-model", ReasoningEffort: "minimal"}},
		{"child preset", workflowruntime.ChildExecutionRequest{Preset: "child"}, workflowruntime.ChildExecutionRequest{Preset: "child", ModelProvider: "CLAUDE", Model: "child-model", ReasoningEffort: "high"}},
		{"factory preset", workflowruntime.ChildExecutionRequest{AgentID: "reviewer"}, workflowruntime.ChildExecutionRequest{AgentID: "reviewer", Preset: "factory", ModelProvider: "CODEX", Model: "factory-model", ReasoningEffort: "low"}},
		{"mixed fields", workflowruntime.ChildExecutionRequest{AgentID: "reviewer", Preset: "child", Model: "explicit-model"}, workflowruntime.ChildExecutionRequest{AgentID: "reviewer", Preset: "child", ModelProvider: "CLAUDE", Model: "explicit-model", ReasoningEffort: "high"}},
		{"scalar defaults", workflowruntime.ChildExecutionRequest{}, workflowruntime.ChildExecutionRequest{ModelProvider: "GEMINI", Model: "scalar-model"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := workflowruntime.ResolveChildWorkerSettings(tc.req, agents, config)
			if err != nil {
				t.Fatalf("ResolveChildWorkerSettings() error = %v", err)
			}
			if got.AgentID != tc.want.AgentID || got.Preset != tc.want.Preset || got.ModelProvider != tc.want.ModelProvider || got.Model != tc.want.Model || got.ReasoningEffort != tc.want.ReasoningEffort {
				t.Fatalf("selection = %#v, want %#v", got, tc.want)
			}
			again, err := workflowruntime.ResolveChildWorkerSettings(tc.req, agents, config)
			if err != nil || !reflect.DeepEqual(again, got) {
				t.Fatalf("repeated selection = %#v, %v; want %#v", again, err, got)
			}
		})
	}
}

func TestResolveChildWorkerSettings_UnknownPresetNamesSource(t *testing.T) {
	_, err := workflowruntime.ResolveChildWorkerSettings(workflowruntime.ChildExecutionRequest{Preset: "missing"}, nil, workflowruntime.WorkerSettingsConfig{})
	if err == nil || !strings.Contains(err.Error(), `"missing" from agent.run`) {
		t.Fatalf("error = %v", err)
	}
	_, err = workflowruntime.ResolveChildWorkerSettings(workflowruntime.ChildExecutionRequest{AgentID: "reviewer"}, map[string]interfaces.FactoryOrchestratorJavaScriptAgent{"reviewer": {Preset: "missing"}}, workflowruntime.WorkerSettingsConfig{})
	if err == nil || !strings.Contains(err.Error(), `"missing" from factory agent`) {
		t.Fatalf("error = %v", err)
	}
}

func TestAgentRun_InheritsFactoryNamedAgentPreset(t *testing.T) {
	req := workflowruntime.Request{
		Source:    `return (async function () { return agent.run({agentId: "reviewer", prompt: "review"}); })();`,
		SessionID: "session-named-agent",
		Agents: map[string]interfaces.FactoryOrchestratorJavaScriptAgent{
			"reviewer": {Preset: "careful-review"},
		},
		WorkerSettings: workflowruntime.WorkerSettingsConfig{Presets: map[string]workflowruntime.WorkerPreset{
			"careful-review": {ModelProvider: "CODEX"},
		}},
	}
	var captured workflowruntime.ChildExecutionRequest
	outcome, err := workflowruntime.Run(context.Background(), req, workflowruntime.Hooks{
		NewChildExecutor: func(_ string, _ workflowruntime.ChildRecordSink, _ workflowpolicy.EffectivePolicy) workflowruntime.ChildExecutor {
			return childExecutorFunc(func(_ context.Context, child workflowruntime.ChildExecutionRequest) (workflowruntime.ChildExecutionResult, error) {
				captured = child
				return workflowruntime.ChildExecutionResult{Status: "COMPLETED", Request: child}, nil
			})
		},
	})
	if err != nil || !outcome.OK {
		t.Fatalf("Run() outcome=%#v err=%v", outcome, err)
	}
	if captured.AgentID != "reviewer" || captured.Preset != "careful-review" {
		t.Fatalf("child selection = %#v", captured)
	}
}

func TestAgentRun_RejectsUnknownFactoryNamedAgentBeforeDispatch(t *testing.T) {
	called := false
	outcome, err := workflowruntime.Run(context.Background(), workflowruntime.Request{
		Source:    `agent.run({agentId: "missing", prompt: "review"}); return {ok: true};`,
		SessionID: "session-unknown-agent",
	}, workflowruntime.Hooks{NewChildExecutor: func(_ string, _ workflowruntime.ChildRecordSink, _ workflowpolicy.EffectivePolicy) workflowruntime.ChildExecutor {
		return childExecutorFunc(func(_ context.Context, child workflowruntime.ChildExecutionRequest) (workflowruntime.ChildExecutionResult, error) {
			called = true
			return workflowruntime.ChildExecutionResult{}, nil
		})
	}})
	if err != nil || outcome.OK || called || !strings.Contains(outcome.Failure.Message, `unknown factory agent "missing"`) {
		t.Fatalf("Run() outcome=%#v err=%v called=%v", outcome, err, called)
	}
}

func TestAgentRun_GatesResolvedPresetWorkerSettingsBeforeDispatch(t *testing.T) {
	tests := []struct {
		name       string
		source     string
		policy     workflowpolicy.EffectivePolicy
		wantErr    string
		wantModel  string
		wantEffort string
	}{
		{
			name:    "preset model denied",
			source:  `return agent.run({preset: "careful", prompt: "review"});`,
			policy:  policyWithWorkerAllowlists([]string{"allowed-model"}, []string{"high"}),
			wantErr: `policy denied: model "preset-model" is not listed in allowedModels`,
		},
		{
			name:    "preset reasoning denied",
			source:  `return agent.run({preset: "careful", prompt: "review"});`,
			policy:  policyWithWorkerAllowlists([]string{"preset-model"}, []string{"low"}),
			wantErr: `policy denied: reasoningEffort "high" is not listed in allowedReasoningEfforts`,
		},
		{
			name:       "explicit fields override preset and pass",
			source:     `return agent.run({preset: "careful", model: "allowed-model", reasoningEffort: "low", prompt: "review"});`,
			policy:     policyWithWorkerAllowlists([]string{"allowed-model"}, []string{"low"}),
			wantModel:  "allowed-model",
			wantEffort: "low",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			var captured workflowruntime.ChildExecutionRequest
			outcome, err := workflowruntime.Run(context.Background(), workflowruntime.Request{
				Source: tc.source, SessionID: "session-policy-preset", Policy: tc.policy,
				WorkerSettings: workflowruntime.WorkerSettingsConfig{Presets: map[string]workflowruntime.WorkerPreset{
					"careful": {ModelProvider: "codex", Model: "preset-model", ReasoningEffort: "high"},
				}},
			}, workflowruntime.Hooks{NewChildExecutor: func(_ string, _ workflowruntime.ChildRecordSink, _ workflowpolicy.EffectivePolicy) workflowruntime.ChildExecutor {
				return childExecutorFunc(func(_ context.Context, req workflowruntime.ChildExecutionRequest) (workflowruntime.ChildExecutionResult, error) {
					calls++
					captured = req
					return workflowruntime.ChildExecutionResult{Status: workflowruntime.ChildDispatchStatusCompleted, Request: req}, nil
				})
			}})
			if err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			if tc.wantErr != "" {
				if outcome.OK || calls != 0 || len(outcome.Records) != 0 || !strings.Contains(outcome.Failure.Message, tc.wantErr) {
					t.Fatalf("outcome=%#v calls=%d, want denial %q with no side effects", outcome, calls, tc.wantErr)
				}
				return
			}
			if !outcome.OK || calls != 1 || captured.Model != tc.wantModel || captured.ReasoningEffort != tc.wantEffort {
				t.Fatalf("outcome=%#v calls=%d request=%#v", outcome, calls, captured)
			}
		})
	}
}

func policyWithWorkerAllowlists(models, efforts []string) workflowpolicy.EffectivePolicy {
	policy := workflowpolicy.DefaultEffectivePolicy()
	policy.AllowedModels = models
	policy.AllowedReasoningEfforts = efforts
	return policy
}

type childExecutorFunc func(context.Context, workflowruntime.ChildExecutionRequest) (workflowruntime.ChildExecutionResult, error)

func (f childExecutorFunc) Execute(ctx context.Context, req workflowruntime.ChildExecutionRequest) (workflowruntime.ChildExecutionResult, error) {
	return f(ctx, req)
}

func TestRun_AgentRunFakeChild_EmitsOrderedChildDispatchRecords(t *testing.T) {
	source := readFixture(t, "agent-run-fake-child.workflow.js")
	req := workflowruntime.Request{
		Source:    source,
		SourceRef: "agent-run-fake-child.workflow.js",
		SessionID: "session-agent-run-fake-child",
		Args:      marshalArgs(t, map[string]any{"subject": "workflows"}),
		Metadata: map[string]string{
			"name": "agent-run-fake-child",
		},
		Policy: workflowpolicy.DefaultEffectivePolicy(),
	}

	first := runSuccessful(t, req)
	second := runSuccessful(t, req)

	if len(first.Records) != 3 {
		t.Fatalf("record count = %d, want 3 child dispatch records", len(first.Records))
	}
	assertRecordSequences(t, first.Records)

	wantPromptDigest := assertFakeChildDispatchRecords(t, first.Records, req.SessionID)
	assertFakeChildProjectedValue(t, req.SessionID, first.Value, wantPromptDigest)

	if recordsJSON(first.Records) != recordsJSON(second.Records) {
		t.Fatalf("record drift across runs:\nfirst=%s\nsecond=%s", recordsJSON(first.Records), recordsJSON(second.Records))
	}
	if string(first.Value.JSON) != string(second.Value.JSON) {
		t.Fatalf("value drift across runs: first=%s second=%s", first.Value.JSON, second.Value.JSON)
	}
}

func assertFakeChildDispatchRecords(t *testing.T, records []workflowruntime.RuntimeRecord, sessionID string) string {
	t.Helper()

	wantStatuses := []string{
		workflowruntime.ChildDispatchStatusQueued,
		workflowruntime.ChildDispatchStatusRunning,
		workflowruntime.ChildDispatchStatusCompleted,
	}
	wantPromptDigest := records[0].ChildDispatch.PromptDigest
	wantArtifactRef := workflowresult.FormatArtifactURI(sessionID, "child-artifact-1")

	for i, wantStatus := range wantStatuses {
		assertFakeChildDispatchRecord(t, records[i], i, wantStatus, wantPromptDigest, wantArtifactRef)
	}
	return wantPromptDigest
}

func assertFakeChildDispatchRecord(
	t *testing.T,
	record workflowruntime.RuntimeRecord,
	index int,
	wantStatus string,
	wantPromptDigest string,
	wantArtifactRef string,
) {
	t.Helper()
	if record.Kind != workflowruntime.RecordKindChildDispatch {
		t.Fatalf("records[%d].kind = %q, want %q", index, record.Kind, workflowruntime.RecordKindChildDispatch)
	}
	child := record.ChildDispatch
	if child == nil {
		t.Fatalf("records[%d] missing child dispatch payload", index)
	}
	if child.Status != wantStatus {
		t.Fatalf("records[%d].status = %q, want %q", index, child.Status, wantStatus)
	}
	if child.DispatchID != "dispatch-1" {
		t.Fatalf("records[%d].dispatchId = %q, want dispatch-1", index, child.DispatchID)
	}
	if child.ChildIndex != 1 {
		t.Fatalf("records[%d].childIndex = %d, want 1", index, child.ChildIndex)
	}
	assertFakeChildDispatchRecordMetadata(t, child, index, wantPromptDigest, wantArtifactRef)
}

func assertFakeChildDispatchRecordMetadata(
	t *testing.T,
	child *workflowruntime.ChildDispatchRecord,
	index int,
	wantPromptDigest string,
	wantArtifactRef string,
) {
	t.Helper()
	if child.Label != "summarize-findings" {
		t.Fatalf("records[%d].label = %q", index, child.Label)
	}
	if child.Model != "gpt-test" || child.ReasoningEffort != "medium" {
		t.Fatalf("records[%d] model metadata = %#v", index, child)
	}
	if child.Command != "review" || child.Sandbox != "read-only" {
		t.Fatalf("records[%d] command/sandbox = %#v", index, child)
	}
	if child.ExecutionMode != "fake" {
		t.Fatalf("records[%d].executionMode = %q, want fake", index, child.ExecutionMode)
	}
	if child.ProviderSessionRef != "fake-provider-session-1" {
		t.Fatalf("records[%d].providerSessionRef = %q, want fake-provider-session-1", index, child.ProviderSessionRef)
	}
	if child.ArtifactRef != wantArtifactRef {
		t.Fatalf("records[%d].artifactRef = %q, want %q", index, child.ArtifactRef, wantArtifactRef)
	}
	if child.PromptDigest != wantPromptDigest {
		t.Fatalf("records[%d].promptDigest = %q, want %q", index, child.PromptDigest, wantPromptDigest)
	}
	if child.SchemaDigest == "" {
		t.Fatalf("records[%d].schemaDigest is empty", index)
	}
}

func assertFakeChildProjectedValue(t *testing.T, sessionID string, value workflowresult.TypedValue, wantPromptDigest string) {
	t.Helper()
	projected := projectPrimaryJSON(t, sessionID, value)
	if projected["label"] != "agent-run-fake-child" {
		t.Fatalf("projected label = %#v", projected["label"])
	}
	if projected["subject"] != "workflows" {
		t.Fatalf("projected subject = %#v", projected["subject"])
	}
	child, ok := projected["child"].(map[string]any)
	if !ok {
		t.Fatalf("projected child = %#v, want object", projected["child"])
	}
	assertFakeChildProjectedMetadata(t, child, sessionID, wantPromptDigest)
	assertFakeChildProjectedOutput(t, child)
}

func assertFakeChildProjectedMetadata(t *testing.T, child map[string]any, sessionID string, wantPromptDigest string) {
	t.Helper()
	wantArtifactRef := workflowresult.FormatArtifactURI(sessionID, "child-artifact-1")
	if child["status"] != workflowruntime.ChildDispatchStatusCompleted {
		t.Fatalf("child status = %#v", child["status"])
	}
	if child["dispatchId"] != "dispatch-1" {
		t.Fatalf("child dispatchId = %#v, want dispatch-1", child["dispatchId"])
	}
	if child["executionMode"] != "fake" {
		t.Fatalf("child executionMode = %#v", child["executionMode"])
	}
	if child["providerSessionRef"] != "fake-provider-session-1" {
		t.Fatalf("child providerSessionRef = %#v, want fake-provider-session-1", child["providerSessionRef"])
	}
	if child["artifactRef"] != wantArtifactRef {
		t.Fatalf("child artifactRef = %#v, want %q", child["artifactRef"], wantArtifactRef)
	}
	if child["label"] != "summarize-findings" || child["model"] != "gpt-test" {
		t.Fatalf("child request metadata = %#v", child)
	}
	if child["reasoningEffort"] != "medium" || child["command"] != "review" || child["sandbox"] != "read-only" {
		t.Fatalf("child options = %#v", child)
	}
	if child["promptDigest"] != wantPromptDigest {
		t.Fatalf("child promptDigest = %#v, want %q", child["promptDigest"], wantPromptDigest)
	}
}

func assertFakeChildProjectedOutput(t *testing.T, child map[string]any) {
	t.Helper()
	output, ok := child["output"].(map[string]any)
	if !ok {
		t.Fatalf("child output = %#v, want object", child["output"])
	}
	if output["text"] != "fake:agent-run-fake-child:summarize-findings:summarize workflows:workflows" {
		t.Fatalf("child output text = %#v", output["text"])
	}
	if output["subject"] != "workflows" {
		t.Fatalf("child output subject = %#v", output["subject"])
	}
	if output["schemaValidated"] != true {
		t.Fatalf("child output schemaValidated = %#v", output["schemaValidated"])
	}
}

const stubChildExecutionMode = "stub-dispatch"

type stubChildExecutor struct {
	mu     sync.Mutex
	labels []string
	mode   string
}

func (s *stubChildExecutor) Execute(_ context.Context, req workflowruntime.ChildExecutionRequest) (workflowruntime.ChildExecutionResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.labels = append(s.labels, req.Label)
	index := len(s.labels)
	return workflowruntime.ChildExecutionResult{
		DispatchID:    fmt.Sprintf("stub-dispatch-%d", index),
		ChildIndex:    index,
		Status:        workflowruntime.ChildDispatchStatusCompleted,
		ExecutionMode: s.mode,
		Output: map[string]any{
			"text":  fmt.Sprintf("stub:%s", req.Label),
			"label": req.Label,
		},
		Request: req,
	}, nil
}

func (s *stubChildExecutor) labelOrder() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.labels...)
}

func TestRun_ChildExecutionBoundary_RoutesAgentRunParallelAndPipelineThroughHooks(t *testing.T) {
	source := readFixture(t, "child-execution-boundary.workflow.js")
	stub := &stubChildExecutor{mode: stubChildExecutionMode}
	req := workflowruntime.Request{
		Source:    source,
		SourceRef: "child-execution-boundary.workflow.js",
		SessionID: "session-child-execution-boundary",
		Args:      marshalArgs(t, map[string]any{"subject": "workflows"}),
		Metadata: map[string]string{
			"name": "child-execution-boundary",
		},
		Policy: workflowpolicy.DefaultEffectivePolicy(),
	}
	hooks := workflowruntime.Hooks{
		NewChildExecutor: func(_ string, _ workflowruntime.ChildRecordSink, _ workflowpolicy.EffectivePolicy) workflowruntime.ChildExecutor {
			return stub
		},
	}

	outcome, err := workflowruntime.Run(context.Background(), req, hooks)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !outcome.OK {
		t.Fatalf("Run() failure = %#v", outcome.Failure)
	}

	assertStubChildExecutorLabelOrder(t, stub.labelOrder())
	assertChildExecutionBoundaryProjection(t, projectPrimaryJSON(t, req.SessionID, outcome.Value))
}

func assertStubChildExecutorLabelOrder(t *testing.T, gotLabels []string) {
	t.Helper()
	if len(gotLabels) != 5 {
		t.Fatalf("child executor call count = %d, want 5", len(gotLabels))
	}
	if gotLabels[0] != "agent-run-boundary" {
		t.Fatalf("first child label = %q, want agent-run-boundary", gotLabels[0])
	}
	parallelLabels := append([]string(nil), gotLabels[1], gotLabels[2])
	sort.Strings(parallelLabels)
	if fmt.Sprint(parallelLabels) != fmt.Sprint([]string{"parallel-boundary-a", "parallel-boundary-b"}) {
		t.Fatalf("parallel child labels = %#v, want [parallel-boundary-a parallel-boundary-b]", parallelLabels)
	}
	if gotLabels[3] != "pipeline-edit-boundary" || gotLabels[4] != "pipeline-review-boundary" {
		t.Fatalf("pipeline child labels = %#v, want [pipeline-edit-boundary pipeline-review-boundary]", gotLabels[3:])
	}
}

func assertChildExecutionBoundaryProjection(t *testing.T, projected map[string]any) {
	t.Helper()
	if projected["label"] != "child-execution-boundary" {
		t.Fatalf("projected label = %#v", projected["label"])
	}

	single, ok := projected["single"].(map[string]any)
	if !ok {
		t.Fatalf("projected single = %#v, want object", projected["single"])
	}
	assertStubChildResult(t, single, "agent-run-boundary", "stub-dispatch-1")

	parallel, ok := projected["parallel"].([]any)
	if !ok || len(parallel) != 2 {
		t.Fatalf("projected parallel = %#v, want 2 entries", projected["parallel"])
	}
	assertStubChildResultByLabel(t, parallel[0], "parallel-boundary-a")
	assertStubChildResultByLabel(t, parallel[1], "parallel-boundary-b")

	pipeline, ok := projected["pipeline"].([]any)
	if !ok || len(pipeline) != 1 {
		t.Fatalf("projected pipeline = %#v, want 1 item", projected["pipeline"])
	}
	item, ok := pipeline[0].(map[string]any)
	if !ok || item["status"] != workflowruntime.ChildDispatchStatusCompleted {
		t.Fatalf("pipeline item = %#v, want completed status", pipeline[0])
	}
	stages, ok := item["stages"].([]any)
	if !ok || len(stages) != 2 {
		t.Fatalf("pipeline stages = %#v, want 2 stages", item["stages"])
	}
	editStage, ok := stages[0].(map[string]any)
	if !ok || editStage["status"] != workflowruntime.ChildDispatchStatusCompleted {
		t.Fatalf("pipeline edit stage = %#v", stages[0])
	}
	assertStubChildResult(t, editStage["result"], "pipeline-edit-boundary", "stub-dispatch-4")

	reviewStage, ok := stages[1].(map[string]any)
	if !ok || reviewStage["status"] != workflowruntime.ChildDispatchStatusCompleted {
		t.Fatalf("pipeline review stage = %#v", stages[1])
	}
	assertStubChildResult(t, reviewStage["result"], "pipeline-review-boundary", "stub-dispatch-5")
}

func assertStubChildResultByLabel(t *testing.T, value any, wantLabel string) {
	t.Helper()
	child, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("child result = %#v, want object", value)
	}
	if child["label"] != wantLabel {
		t.Fatalf("child label = %#v, want %q", child["label"], wantLabel)
	}
	assertStubChildResult(t, value, wantLabel, "")
}

func assertStubChildResult(t *testing.T, value any, wantLabel, wantDispatchID string) {
	t.Helper()
	child, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("child result = %#v, want object", value)
	}
	if child["status"] != workflowruntime.ChildDispatchStatusCompleted {
		t.Fatalf("child status = %#v, want %q", child["status"], workflowruntime.ChildDispatchStatusCompleted)
	}
	if child["executionMode"] != stubChildExecutionMode {
		t.Fatalf("child executionMode = %#v, want %q", child["executionMode"], stubChildExecutionMode)
	}
	if child["label"] != wantLabel {
		t.Fatalf("child label = %#v, want %q", child["label"], wantLabel)
	}
	if wantDispatchID != "" && child["dispatchId"] != wantDispatchID {
		t.Fatalf("child dispatchId = %#v, want %q", child["dispatchId"], wantDispatchID)
	}
	output, ok := child["output"].(map[string]any)
	if !ok {
		t.Fatalf("child output = %#v, want object", child["output"])
	}
	wantText := fmt.Sprintf("stub:%s", wantLabel)
	if output["text"] != wantText {
		t.Fatalf("child output text = %#v, want %q", output["text"], wantText)
	}
}

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

func assertParallelResultOrder(t *testing.T, sessionID string, outcome workflowruntime.Outcome, wantLabels []string) {
	t.Helper()
	projected := projectPrimaryJSON(t, sessionID, outcome.Value)
	results, ok := projected["results"].([]any)
	if !ok || len(results) != len(wantLabels) {
		t.Fatalf("projected results = %#v, want %d entries", projected["results"], len(wantLabels))
	}
	for i, wantLabel := range wantLabels {
		child, ok := results[i].(map[string]any)
		if !ok {
			t.Fatalf("results[%d] = %#v, want object", i, results[i])
		}
		if child["label"] != wantLabel {
			t.Fatalf("results[%d].label = %#v, want %q", i, child["label"], wantLabel)
		}
		if child["status"] != workflowruntime.ChildDispatchStatusCompleted {
			t.Fatalf("results[%d].status = %#v, want COMPLETED", i, child["status"])
		}
	}
}

func peakConcurrentChildRunning(records []workflowruntime.RuntimeRecord) int {
	running := 0
	peak := 0
	for _, record := range records {
		if record.Kind != workflowruntime.RecordKindChildDispatch || record.ChildDispatch == nil {
			continue
		}
		switch record.ChildDispatch.Status {
		case workflowruntime.ChildDispatchStatusRunning:
			running++
			if running > peak {
				peak = running
			}
		case workflowruntime.ChildDispatchStatusCompleted, workflowruntime.ChildDispatchStatusFailed:
			running--
		}
	}
	return peak
}

func childCompletionOrder(records []workflowruntime.RuntimeRecord) []string {
	var order []string
	for _, record := range records {
		if record.Kind != workflowruntime.RecordKindChildDispatch || record.ChildDispatch == nil {
			continue
		}
		if record.ChildDispatch.Status != workflowruntime.ChildDispatchStatusCompleted {
			continue
		}
		if record.ChildDispatch.Label != "" {
			order = append(order, record.ChildDispatch.Label)
		}
	}
	return order
}

func hasChildDispatchStatus(records []workflowruntime.RuntimeRecord, status string) bool {
	for _, record := range records {
		if record.Kind == workflowruntime.RecordKindChildDispatch &&
			record.ChildDispatch != nil &&
			record.ChildDispatch.Status == status {
			return true
		}
	}
	return false
}

func completedForLabel(records []workflowruntime.RuntimeRecord, label string) bool {
	for _, record := range records {
		if record.Kind != workflowruntime.RecordKindChildDispatch || record.ChildDispatch == nil {
			continue
		}
		if record.ChildDispatch.Label == label &&
			record.ChildDispatch.Status == workflowruntime.ChildDispatchStatusCompleted {
			return true
		}
	}
	return false
}
