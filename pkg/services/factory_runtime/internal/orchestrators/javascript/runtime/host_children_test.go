package workflowruntime_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	workflowruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/javascript/runtime"
	workflowpolicy "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/orchestratorcontract"
)

func TestRun_DocumentedSimpleFinalWorkflow(t *testing.T) {
	outcome := runDocumentedExample(t, "simple-final.workflow.js", map[string]any{
		"greeting": "hello", "subject": "factory authors",
	}, nil)
	projected := projectPrimaryJSON(t, "session-documented-simple-final", outcome.Value)
	if projected["greeting"] != "hello" || projected["subject"] != "factory authors" {
		t.Fatalf("documented simple result = %#v", projected)
	}
	if len(outcome.Records) != 1 || outcome.Records[0].Kind != factory.JavaScriptRecordKindPhase {
		t.Fatalf("documented simple records = %#v", outcome.Records)
	}
}

func TestRun_DocumentedOrderedFanoutWorkflow(t *testing.T) {
	outcome := runDocumentedExample(t, "ordered-fanout.workflow.js", map[string]any{
		"topics": []string{"alpha", "beta", "gamma"},
	}, nil)
	projected := projectPrimaryJSON(t, "session-documented-ordered-fanout", outcome.Value)
	reviews, ok := projected["reviews"].([]any)
	if !ok || len(reviews) != 3 {
		t.Fatalf("documented reviews = %#v", projected["reviews"])
	}
	for index, topic := range []string{"alpha", "beta", "gamma"} {
		review, ok := reviews[index].(map[string]any)
		if !ok || review["label"] != "review-"+topic {
			t.Fatalf("documented reviews[%d] = %#v", index, reviews[index])
		}
	}
	if synthesis, ok := projected["synthesis"].(map[string]any); !ok || synthesis["label"] != "synthesize" {
		t.Fatalf("documented synthesis = %#v", projected["synthesis"])
	}
	if childTerminalRecordCount(outcome.Records) != 4 {
		t.Fatalf("completed child dispatches = %d, want 4", childTerminalRecordCount(outcome.Records))
	}
}

func TestRun_DocumentedCheckpointResumeWorkflow(t *testing.T) {
	args := map[string]any{"topics": []string{"alpha", "beta"}}
	fresh := runDocumentedExample(t, "checkpoint-resume.workflow.js", args, nil)
	var checkpoint *factory.JavaScriptCheckpointRecord
	for _, record := range fresh.Records {
		if record.Checkpoint != nil {
			checkpoint = record.Checkpoint
		}
	}
	if checkpoint == nil || checkpoint.State["completedTopics"] == nil {
		t.Fatalf("documented checkpoint records = %#v", fresh.Records)
	}
	resumed := runDocumentedExample(t, "checkpoint-resume.workflow.js", args, &factory.JavaScriptResumeContext{
		CheckpointState: checkpoint.State,
	})
	projected := projectPrimaryJSON(t, "session-documented-checkpoint-resume", resumed.Value)
	if projected["path"] != "resumed" {
		t.Fatalf("documented resumed result = %#v", projected)
	}
}

func runDocumentedExample(t *testing.T, name string, args map[string]any, resume *factory.JavaScriptResumeContext) factory.JavaScriptRuntimeOutcome {
	t.Helper()
	path := filepath.Join("..", "..", "..", "..", "..", "..", "..", "docs", "examples", "javascript-workflows", name)
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read documented workflow %s: %v", name, err)
	}
	return runSuccessful(t, factory.JavaScriptRuntimeRequest{
		Source: string(source), SourceRef: path,
		SessionID: "session-documented-" + strings.TrimSuffix(name, ".workflow.js"),
		Args:      marshalArgs(t, args),
		Metadata:  map[string]string{"name": strings.TrimSuffix(name, ".workflow.js")},
		Policy:    workflowpolicy.DefaultEffectivePolicy(), Resume: resume,
	})
}

func childTerminalRecordCount(records []factory.JavaScriptRuntimeRecord) int {
	count := 0
	for _, record := range records {
		if record.ChildDispatch != nil && record.ChildDispatch.Status == factory.JavaScriptChildDispatchStatusCompleted {
			count++
		}
	}
	return count
}

func TestResolveChildWorkerSettings_FieldByFieldPrecedence(t *testing.T) {
	agents := map[string]interfaces.FactoryOrchestratorJavaScriptAgent{"reviewer": {Preset: "factory"}}
	config := factory.JavaScriptWorkerSettings{
		Presets: map[string]factory.JavaScriptWorkerPreset{
			"child":   {ModelProvider: "claude", Model: "child-model", ReasoningEffort: "HIGH"},
			"factory": {ModelProvider: "codex", Model: "factory-model", ReasoningEffort: "low"},
		}, DefaultModelProvider: "gemini", DefaultModel: "scalar-model",
	}
	tests := []struct {
		name      string
		req, want factory.JavaScriptChildExecutionRequest
	}{
		{"explicit fields", factory.JavaScriptChildExecutionRequest{ModelProvider: "kiro-cli", Model: "explicit-model", ReasoningEffort: "minimal"}, factory.JavaScriptChildExecutionRequest{ModelProvider: "kiro-cli", Model: "explicit-model", ReasoningEffort: "minimal"}},
		{"child preset", factory.JavaScriptChildExecutionRequest{Preset: "child"}, factory.JavaScriptChildExecutionRequest{Preset: "child", ModelProvider: "claude", Model: "child-model", ReasoningEffort: "high"}},
		{"factory preset", factory.JavaScriptChildExecutionRequest{AgentID: "reviewer"}, factory.JavaScriptChildExecutionRequest{AgentID: "reviewer", Preset: "factory", ModelProvider: "codex", Model: "factory-model", ReasoningEffort: "low"}},
		{"mixed fields", factory.JavaScriptChildExecutionRequest{AgentID: "reviewer", Preset: "child", Model: "explicit-model"}, factory.JavaScriptChildExecutionRequest{AgentID: "reviewer", Preset: "child", ModelProvider: "claude", Model: "explicit-model", ReasoningEffort: "high"}},
		{"scalar defaults", factory.JavaScriptChildExecutionRequest{}, factory.JavaScriptChildExecutionRequest{ModelProvider: "gemini", Model: "scalar-model"}},
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

func TestResolveChildWorkerSettings_CanonicalizesExplicitModelProvider(t *testing.T) {
	got, err := workflowruntime.ResolveChildWorkerSettings(
		factory.JavaScriptChildExecutionRequest{ModelProvider: " CODEX "},
		nil,
		factory.JavaScriptWorkerSettings{},
	)
	if err != nil {
		t.Fatalf("ResolveChildWorkerSettings() error = %v", err)
	}
	if got.ModelProvider != "codex" {
		t.Fatalf("modelProvider = %q, want canonical codex", got.ModelProvider)
	}
}

func TestResolveChildWorkerSettings_UnknownPresetNamesSource(t *testing.T) {
	got, err := workflowruntime.ResolveChildWorkerSettings(factory.JavaScriptChildExecutionRequest{Preset: "missing"}, nil, factory.JavaScriptWorkerSettings{})
	if err != nil || got.Preset != "missing" {
		t.Fatalf("explicit child preset = %#v, error = %v", got, err)
	}
	_, err = workflowruntime.ResolveChildWorkerSettings(factory.JavaScriptChildExecutionRequest{AgentID: "reviewer"}, map[string]interfaces.FactoryOrchestratorJavaScriptAgent{"reviewer": {Preset: "missing"}}, factory.JavaScriptWorkerSettings{})
	if err == nil || !strings.Contains(err.Error(), `"missing" from factory agent`) {
		t.Fatalf("error = %v", err)
	}
}

func TestFailureBaseline_AbsentDefault_GoalAgentRunLeavesEmptyModelProviderWithoutOperatorDefaults(t *testing.T) {
	got, err := workflowruntime.ResolveChildWorkerSettings(
		factory.JavaScriptChildExecutionRequest{
			AgentID: "goal-planner",
			Prompt:  "plan the goal",
		},
		map[string]interfaces.FactoryOrchestratorJavaScriptAgent{
			"goal-planner": {},
		},
		factory.JavaScriptWorkerSettings{})

	if err != nil {
		t.Fatalf("ResolveChildWorkerSettings() error = %v", err)
	}
	if got.ModelProvider != "" {
		t.Fatalf("modelProvider = %q, want empty when operator defaults are absent", got.ModelProvider)
	}
	if got.Model != "" {
		t.Fatalf("model = %q, want empty when operator defaults are absent", got.Model)
	}
	if got.Command != "" {
		t.Fatalf("command = %q, want empty provider command when operator defaults are absent", got.Command)
	}
}

func TestAgentRun_RejectsFactoryAgentIDBeforeDispatch(t *testing.T) {
	called := false
	outcome, err := runtimeWorkflows.Run(context.Background(), factory.JavaScriptRuntimeRequest{
		Source:    `agent.run({agentId: "reviewer", prompt: "review"}); return {ok: true};`,
		SessionID: "session-unsupported-agent-id",
	}, factory.JavaScriptRuntimeHooks{NewChildExecutor: func(_ string, _ factory.JavaScriptChildRecordSink, _ workflowpolicy.EffectivePolicy) factory.JavaScriptChildExecutor {
		return childExecutorFunc(func(_ context.Context, child factory.JavaScriptChildExecutionRequest) (factory.JavaScriptChildExecutionResult, error) {
			called = true
			return factory.JavaScriptChildExecutionResult{}, nil
		})
	}})
	if err != nil || outcome.OK || called || !strings.Contains(outcome.Failure.Message, `agent.run() does not support field "agentId"`) {
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
			var captured factory.JavaScriptChildExecutionRequest
			outcome, err := runtimeWorkflows.Run(context.Background(), factory.JavaScriptRuntimeRequest{
				Source: tc.source, SessionID: "session-policy-preset", Policy: tc.policy,
				WorkerSettings: factory.JavaScriptWorkerSettings{Presets: map[string]factory.JavaScriptWorkerPreset{
					"careful": {ModelProvider: "codex", Model: "preset-model", ReasoningEffort: "high"},
				}},
			}, factory.JavaScriptRuntimeHooks{NewChildExecutor: func(_ string, _ factory.JavaScriptChildRecordSink, _ workflowpolicy.EffectivePolicy) factory.JavaScriptChildExecutor {
				return childExecutorFunc(func(_ context.Context, req factory.JavaScriptChildExecutionRequest) (factory.JavaScriptChildExecutionResult, error) {
					calls++
					captured = req
					return factory.JavaScriptChildExecutionResult{Status: factory.JavaScriptChildDispatchStatusCompleted, Request: req}, nil
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

type childExecutorFunc func(context.Context, factory.JavaScriptChildExecutionRequest) (factory.JavaScriptChildExecutionResult, error)

func (f childExecutorFunc) Execute(ctx context.Context, req factory.JavaScriptChildExecutionRequest) (factory.JavaScriptChildExecutionResult, error) {
	return f(ctx, req)
}

func TestRun_AgentRunFakeChild_EmitsOrderedChildDispatchRecords(t *testing.T) {
	source := readFixture(t, "agent-run-fake-child.workflow.js")
	req := factory.JavaScriptRuntimeRequest{
		Source:    source,
		SourceRef: "agent-run-fake-child.workflow.js",
		SessionID: "session-agent-run-fake-child",
		Args:      marshalArgs(t, map[string]any{"subject": "workflows"}),
		Metadata: map[string]string{
			"name": "agent-run-fake-child",
		},
		Policy: workflowpolicy.DefaultEffectivePolicy(),
		WorkerSettings: factory.JavaScriptWorkerSettings{Presets: map[string]factory.JavaScriptWorkerPreset{
			"careful": {},
		}},
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

func assertFakeChildDispatchRecords(t *testing.T, records []factory.JavaScriptRuntimeRecord, sessionID string) string {
	t.Helper()

	wantStatuses := []string{
		factory.JavaScriptChildDispatchStatusQueued,
		factory.JavaScriptChildDispatchStatusRunning,
		factory.JavaScriptChildDispatchStatusCompleted,
	}
	wantPromptDigest := records[0].ChildDispatch.PromptDigest
	wantArtifactRef := factory.FormatArtifactURI(sessionID, "child-artifact-1")

	for i, wantStatus := range wantStatuses {
		assertFakeChildDispatchRecord(t, records[i], i, wantStatus, wantPromptDigest, wantArtifactRef)
	}
	return wantPromptDigest
}

func assertFakeChildDispatchRecord(
	t *testing.T,
	record factory.JavaScriptRuntimeRecord,
	index int,
	wantStatus string,
	wantPromptDigest string,
	wantArtifactRef string,
) {
	t.Helper()
	if record.Kind != factory.JavaScriptRecordKindChildDispatch {
		t.Fatalf("records[%d].kind = %q, want %q", index, record.Kind, factory.JavaScriptRecordKindChildDispatch)
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
	child *factory.JavaScriptChildDispatchRecord,
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
}

func assertFakeChildProjectedValue(t *testing.T, sessionID string, value factory.TypedValue, wantPromptDigest string) {
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
	wantArtifactRef := factory.FormatArtifactURI(sessionID, "child-artifact-1")
	if child["status"] != factory.JavaScriptChildDispatchStatusCompleted {
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
	if child["reasoningEffort"] != "medium" {
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
	if output["schemaValidated"] != false {
		t.Fatalf("child output schemaValidated = %#v", output["schemaValidated"])
	}
}

const stubChildExecutionMode = "stub-dispatch"

type stubChildExecutor struct {
	mu       sync.Mutex
	labels   []string
	requests []factory.JavaScriptChildExecutionRequest
	mode     string
}

func (s *stubChildExecutor) Execute(_ context.Context, req factory.JavaScriptChildExecutionRequest) (factory.JavaScriptChildExecutionResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.labels = append(s.labels, req.Label)
	s.requests = append(s.requests, req)
	index := len(s.labels)
	return factory.JavaScriptChildExecutionResult{
		DispatchID:    fmt.Sprintf("stub-dispatch-%d", index),
		ChildIndex:    index,
		Status:        factory.JavaScriptChildDispatchStatusCompleted,
		ExecutionMode: s.mode,
		Output: map[string]any{
			"text":  fmt.Sprintf("stub:%s", req.Label),
			"label": req.Label,
		},
		Request: req,
	}, nil
}

func (s *stubChildExecutor) executionRequests() []factory.JavaScriptChildExecutionRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]factory.JavaScriptChildExecutionRequest(nil), s.requests...)
}

func TestRun_AgentRunDynamicObjectCarriesCanonicalFieldsToExecutor(t *testing.T) {
	stub := &stubChildExecutor{mode: stubChildExecutionMode}
	source := `const child = { prompt: " review ", label: " reviewer ", preset: " careful ", executorProvider: " cursor-acp ", modelProvider: " codex ", model: " gpt-test ", reasoningEffort: " high " }; agent.run(child); return { ok: true };`
	outcome, err := runtimeWorkflows.Run(context.Background(), factory.JavaScriptRuntimeRequest{
		Source: source, SourceRef: "inline", SessionID: "canonical-child-fields",
		Policy: workflowpolicy.DefaultEffectivePolicy(),
		WorkerSettings: factory.JavaScriptWorkerSettings{Presets: map[string]factory.JavaScriptWorkerPreset{
			"careful": {},
		}},
	}, factory.JavaScriptRuntimeHooks{NewChildExecutor: func(string, factory.JavaScriptChildRecordSink, workflowpolicy.EffectivePolicy) factory.JavaScriptChildExecutor {
		return stub
	}})
	if err != nil || !outcome.OK {
		t.Fatalf("Run() = outcome %#v, error %v", outcome, err)
	}
	requests := stub.executionRequests()
	if len(requests) != 1 {
		t.Fatalf("executor request count = %d, want 1", len(requests))
	}
	got := requests[0]
	if got.Prompt != "review" || got.Label != "reviewer" || got.Preset != "careful" || got.ExecutorProvider != "cursor-acp" || got.ModelProvider != "codex" || got.Model != "gpt-test" || got.ReasoningEffort != "high" {
		t.Fatalf("executor request = %#v, want normalized canonical fields", got)
	}
}

func TestRun_AgentRunDynamicObjectRejectsUnsupportedFieldsBeforeDispatch(t *testing.T) {
	unsupported := []string{
		"writableRoots", "allowNetwork", "network", "allowDangerFullAccess", "dangerFullAccess",
		"schema", "outputSchema", "concurrency", "maxAgents", "duration", "timeout", "timeoutMs",
	}
	for _, field := range unsupported {
		t.Run(field, func(t *testing.T) {
			stub := &stubChildExecutor{mode: stubChildExecutionMode}
			source := fmt.Sprintf(`const child = { prompt: "prompt-secret" }; child[%q] = "value-secret"; agent.run(child);`, field)
			outcome, err := runtimeWorkflows.Run(context.Background(), factory.JavaScriptRuntimeRequest{
				Source: source, SourceRef: "inline", SessionID: "unsupported-child-field",
				Policy: workflowpolicy.DefaultEffectivePolicy(),
			}, factory.JavaScriptRuntimeHooks{NewChildExecutor: func(string, factory.JavaScriptChildRecordSink, workflowpolicy.EffectivePolicy) factory.JavaScriptChildExecutor {
				return stub
			}})
			if err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			if outcome.OK {
				t.Fatalf("Run() outcome = %#v, want script failure", outcome)
			}
			want := `agent.run() does not support field "` + field + `"`
			if !strings.Contains(outcome.Failure.Message, want) {
				t.Fatalf("failure message = %q, want %q", outcome.Failure.Message, want)
			}
			if strings.Contains(outcome.Failure.Message, "value-secret") || strings.Contains(outcome.Failure.Message, "prompt-secret") {
				t.Fatalf("failure message = %q, want redacted diagnostic", outcome.Failure.Message)
			}
			if len(stub.executionRequests()) != 0 {
				t.Fatalf("executor requests = %#v, want none", stub.executionRequests())
			}
			assertNoChildDispatchRecords(t, outcome.Records)
		})
	}
}

func (s *stubChildExecutor) labelOrder() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.labels...)
}

func TestRun_ChildExecutionBoundary_RoutesAgentRunParallelAndPipelineThroughHooks(t *testing.T) {
	source := readFixture(t, "child-execution-boundary.workflow.js")
	stub := &stubChildExecutor{mode: stubChildExecutionMode}
	req := factory.JavaScriptRuntimeRequest{
		Source:    source,
		SourceRef: "child-execution-boundary.workflow.js",
		SessionID: "session-child-execution-boundary",
		Args:      marshalArgs(t, map[string]any{"subject": "workflows"}),
		Metadata: map[string]string{
			"name": "child-execution-boundary",
		},
		Policy: workflowpolicy.DefaultEffectivePolicy(),
	}
	hooks := factory.JavaScriptRuntimeHooks{
		NewChildExecutor: func(_ string, _ factory.JavaScriptChildRecordSink, _ workflowpolicy.EffectivePolicy) factory.JavaScriptChildExecutor {
			return stub
		},
	}

	outcome, err := runtimeWorkflows.Run(context.Background(), req, hooks)
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
	if !ok || item["status"] != factory.JavaScriptChildDispatchStatusCompleted {
		t.Fatalf("pipeline item = %#v, want completed status", pipeline[0])
	}
	stages, ok := item["stages"].([]any)
	if !ok || len(stages) != 2 {
		t.Fatalf("pipeline stages = %#v, want 2 stages", item["stages"])
	}
	editStage, ok := stages[0].(map[string]any)
	if !ok || editStage["status"] != factory.JavaScriptChildDispatchStatusCompleted {
		t.Fatalf("pipeline edit stage = %#v", stages[0])
	}
	assertStubChildResult(t, editStage["result"], "pipeline-edit-boundary", "stub-dispatch-4")

	reviewStage, ok := stages[1].(map[string]any)
	if !ok || reviewStage["status"] != factory.JavaScriptChildDispatchStatusCompleted {
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
	if child["status"] != factory.JavaScriptChildDispatchStatusCompleted {
		t.Fatalf("child status = %#v, want %q", child["status"], factory.JavaScriptChildDispatchStatusCompleted)
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
