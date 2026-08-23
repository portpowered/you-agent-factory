package workflowruntime_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	workflowruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/javascript/runtime"
	workflowpolicy "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/orchestratorcontract"
)

func TestRun_AgentRunDisallowedPermissionFailsBeforeDispatch(t *testing.T) {
	const want = `policy denied: Factory "named-factory" child "skip-child" requested permission "SKIP_PERMISSIONS" not listed in allowedPermissions`

	policy := workflowpolicy.DefaultEffectivePolicy()
	policy.AllowedPermissions = []string{workflowpolicy.PermissionModeDefault}
	calls := 0
	outcome, err := runtimeWorkflows.Run(context.Background(), factory.JavaScriptRuntimeRequest{
		Source:      `agent.run({ prompt: "review", label: "skip-child", skipPermissions: true }); return { ok: true };`,
		SourceRef:   "inline",
		SessionID:   "session-disallowed-permission",
		FactoryName: "named-factory",
		Policy:      policy,
	}, factory.JavaScriptRuntimeHooks{NewChildExecutor: func(string, factory.JavaScriptChildRecordSink, workflowpolicy.EffectivePolicy) factory.JavaScriptChildExecutor {
		return childExecutorFunc(func(_ context.Context, _ factory.JavaScriptChildExecutionRequest) (factory.JavaScriptChildExecutionResult, error) {
			calls++
			return factory.JavaScriptChildExecutionResult{}, nil
		})
	}})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if outcome.OK || outcome.Failure.Message != want {
		t.Fatalf("Run() outcome = %#v, want exact diagnostic %q", outcome, want)
	}
	if calls != 0 {
		t.Fatalf("child executor calls = %d, want zero before dispatch admission", calls)
	}
	assertNoChildDispatchRecords(t, outcome.Records)
}

func TestRun_AgentRunAllowedAndOmittedPermissionPoliciesPreserveExecution(t *testing.T) {
	tests := []struct {
		name     string
		allowed  []string
		skip     bool
		wantSkip bool
	}{
		{
			name:    "allowed default",
			allowed: []string{workflowpolicy.PermissionModeDefault},
		},
		{
			name:     "allowed skip permissions",
			allowed:  []string{workflowpolicy.PermissionModeSkipPermissions},
			skip:     true,
			wantSkip: true,
		},
		{
			name:     "omitted allowlist",
			skip:     true,
			wantSkip: true,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			policy := workflowpolicy.DefaultEffectivePolicy()
			policy.AllowedPermissions = test.allowed
			calls := 0
			var captured factory.JavaScriptChildExecutionRequest
			outcome, err := runtimeWorkflows.Run(context.Background(), factory.JavaScriptRuntimeRequest{
				Source: fmt.Sprintf(`return agent.run({ prompt: "review", label: %q, skipPermissions: %t });`, test.name, test.skip),
				Policy: policy,
			}, factory.JavaScriptRuntimeHooks{NewChildExecutor: func(string, factory.JavaScriptChildRecordSink, workflowpolicy.EffectivePolicy) factory.JavaScriptChildExecutor {
				return childExecutorFunc(func(_ context.Context, req factory.JavaScriptChildExecutionRequest) (factory.JavaScriptChildExecutionResult, error) {
					calls++
					captured = req
					return factory.JavaScriptChildExecutionResult{Status: factory.JavaScriptChildDispatchStatusCompleted, Request: req}, nil
				})
			}})
			if err != nil || !outcome.OK {
				t.Fatalf("Run() outcome=%#v err=%v", outcome, err)
			}
			if calls != 1 || captured.SkipPermissions != test.wantSkip {
				t.Fatalf("child executor calls=%d request=%#v, want one call with skipPermissions=%t", calls, captured, test.wantSkip)
			}
		})
	}
}

func TestResolveChildWorkerSettings_AntigravityUsesModelEmbeddedEffort(t *testing.T) {
	for _, executorProvider := range []string{"", "SCRIPT_WRAP"} {
		t.Run("executorProvider="+executorProvider, func(t *testing.T) {
			got, err := workflowruntime.ResolveChildWorkerSettings(
				factory.JavaScriptChildExecutionRequest{
					ExecutorProvider: executorProvider,
					ModelProvider:    " ANTIGRAVITY ",
					Model:            "gemini-3.6-flash-medium",
					ReasoningEffort:  " HIGH ",
				},
				nil,
				factory.JavaScriptWorkerSettings{},
			)
			if err != nil {
				t.Fatalf("ResolveChildWorkerSettings() error = %v", err)
			}
			if got.ModelProvider != "antigravity" || got.Model != "gemini-3.6-flash-medium" {
				t.Fatalf("selection = %#v, want canonical Antigravity provider and exact model", got)
			}
			if got.ReasoningEffort != "" {
				t.Fatalf("selection reasoning effort = %q, want omitted for model-encoded Antigravity effort", got.ReasoningEffort)
			}
		})
	}
}

func TestResolveChildWorkerSettings_CodexRetainsReasoningEffort(t *testing.T) {
	got, err := workflowruntime.ResolveChildWorkerSettings(
		factory.JavaScriptChildExecutionRequest{
			ModelProvider:   "CODEX",
			Model:           "gpt-5.6-luna",
			ReasoningEffort: " XHIGH ",
		},
		nil,
		factory.JavaScriptWorkerSettings{},
	)
	if err != nil {
		t.Fatalf("ResolveChildWorkerSettings() error = %v", err)
	}
	if got.ModelProvider != "codex" || got.Model != "gpt-5.6-luna" || got.ReasoningEffort != "xhigh" {
		t.Fatalf("selection = %#v, want codex/gpt-5.6-luna/xhigh", got)
	}
}

func TestRun_ProgressPrimitives_EmitsOrderedRuntimeRecords(t *testing.T) {
	source := readFixture(t, "progress-primitives.workflow.js")
	maxTokens := int64(4096)
	maxRunDurationMs := int64(120000)
	policy := workflowpolicy.DefaultEffectivePolicy()
	policy.MaxTokens = &maxTokens
	policy.MaxRunDurationMs = &maxRunDurationMs
	policy.SandboxMode = "read-only"

	req := factory.JavaScriptRuntimeRequest{
		Source:    source,
		SourceRef: "progress-primitives.workflow.js",
		SessionID: "session-progress-primitives",
		Args:      marshalArgs(t, map[string]any{}),
		Metadata: map[string]string{
			"name": "progress-primitives",
		},
		Policy: policy,
	}

	var artifactKinds []string
	hooks := factory.JavaScriptRuntimeHooks{
		OnArtifact: func(kind string, content json.RawMessage) error {
			artifactKinds = append(artifactKinds, kind)
			return nil
		},
	}

	first := runSuccessfulWithHooks(t, req, hooks)
	if len(artifactKinds) != 1 || artifactKinds[0] != "log" {
		t.Fatalf("artifact hook kinds = %#v, want [log]", artifactKinds)
	}

	second := runSuccessfulWithHooks(t, req, factory.JavaScriptRuntimeHooks{})

	if len(first.Records) != 7 {
		t.Fatalf("record count = %d, want 7", len(first.Records))
	}
	assertRecordSequences(t, first.Records)
	wantURI := assertProgressPrimitiveRecords(t, first.Records, req.SessionID, policy, maxTokens, maxRunDurationMs)
	assertProgressPrimitiveProjection(t, req.SessionID, first.Value, wantURI, policy, maxTokens)

	if recordsJSON(first.Records) != recordsJSON(second.Records) {
		t.Fatalf("record drift across runs:\nfirst=%s\nsecond=%s", recordsJSON(first.Records), recordsJSON(second.Records))
	}
}

func runSuccessfulWithHooks(t *testing.T, req factory.JavaScriptRuntimeRequest, hooks factory.JavaScriptRuntimeHooks) factory.JavaScriptRuntimeOutcome {
	t.Helper()
	outcome, err := runtimeWorkflows.Run(t.Context(), req, hooks)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !outcome.OK {
		t.Fatalf("Run() failure = %#v", outcome.Failure)
	}
	return outcome
}

func assertRecordSequences(t *testing.T, records []factory.JavaScriptRuntimeRecord) {
	t.Helper()
	for i, record := range records {
		want := i + 1
		if record.Sequence != want {
			t.Fatalf("records[%d].sequence = %d, want %d", i, record.Sequence, want)
		}
	}
}

func recordsJSON(records []factory.JavaScriptRuntimeRecord) string {
	raw, err := json.Marshal(records)
	if err != nil {
		return ""
	}
	return string(raw)
}

func assertProgressPrimitiveRecords(
	t *testing.T,
	records []factory.JavaScriptRuntimeRecord,
	sessionID string,
	policy workflowpolicy.EffectivePolicy,
	maxTokens int64,
	maxRunDurationMs int64,
) string {
	t.Helper()

	wantKinds := []string{
		factory.JavaScriptRecordKindPhase,
		factory.JavaScriptRecordKindLog,
		factory.JavaScriptRecordKindLog,
		factory.JavaScriptRecordKindPhase,
		factory.JavaScriptRecordKindArtifact,
		factory.JavaScriptRecordKindCheckpoint,
		factory.JavaScriptRecordKindBudget,
	}
	for i, want := range wantKinds {
		if records[i].Kind != want {
			t.Fatalf("records[%d].kind = %q, want %q", i, records[i].Kind, want)
		}
	}

	assertProgressPhaseAndLogRecords(t, records)
	wantURI := assertProgressArtifactRecord(t, records[4], sessionID)
	assertProgressCheckpointRecord(t, records[5], wantURI)
	assertProgressBudgetRecord(t, records[6], policy, maxTokens, maxRunDurationMs)
	return wantURI
}

func assertProgressPhaseAndLogRecords(t *testing.T, records []factory.JavaScriptRuntimeRecord) {
	t.Helper()
	if records[0].Phase == nil || records[0].Phase.Name != "setup" {
		t.Fatalf("phase record = %#v", records[0].Phase)
	}
	if records[1].Log == nil || records[1].Log.Message != "starting workflow" {
		t.Fatalf("log record = %#v", records[1].Log)
	}
	if records[1].Log.Fields["step"] != float64(1) {
		t.Fatalf("log fields = %#v, want step=1", records[1].Log.Fields)
	}
	if records[2].Log == nil || records[2].Log.Message != "workflow step" {
		t.Fatalf("workflow.log record = %#v", records[2].Log)
	}
	if records[3].Phase == nil || records[3].Phase.Name != "execute" {
		t.Fatalf("second phase record = %#v", records[3].Phase)
	}
}

func assertProgressArtifactRecord(t *testing.T, record factory.JavaScriptRuntimeRecord, sessionID string) string {
	t.Helper()
	artifact := record.Artifact
	if artifact == nil {
		t.Fatal("expected artifact record")
	}
	wantURI := factory.FormatArtifactURI(sessionID, "artifact-1")
	if artifact.URI != wantURI {
		t.Fatalf("artifact uri = %q, want %q", artifact.URI, wantURI)
	}
	if artifact.Kind != "log" || artifact.Label != "step-output" {
		t.Fatalf("artifact metadata = %#v", artifact)
	}
	if artifact.Visibility != "WORKFLOW_RUNTIME" {
		t.Fatalf("artifact visibility = %q, want WORKFLOW_RUNTIME", artifact.Visibility)
	}
	if artifact.ContentHash == "" || artifact.SizeBytes <= 0 {
		t.Fatalf("artifact content metadata = %#v", artifact)
	}
	return wantURI
}

func assertProgressCheckpointRecord(t *testing.T, record factory.JavaScriptRuntimeRecord, wantURI string) {
	t.Helper()
	checkpoint := record.Checkpoint
	if checkpoint == nil {
		t.Fatal("expected checkpoint record")
	}
	if checkpoint.ID != "checkpoint-1" || checkpoint.Label != "after-artifact" {
		t.Fatalf("checkpoint record = %#v", checkpoint)
	}
	if checkpoint.State["artifactRef"] != wantURI {
		t.Fatalf("checkpoint state artifactRef = %#v, want %q", checkpoint.State["artifactRef"], wantURI)
	}
	if checkpoint.State["step"] != float64(2) {
		t.Fatalf("checkpoint state step = %#v", checkpoint.State["step"])
	}
	if strings.Contains(checkpoint.Summary, "goja") || strings.Contains(checkpoint.Summary, "Runtime") {
		t.Fatalf("checkpoint summary exposes VM internals: %q", checkpoint.Summary)
	}
}

func assertProgressBudgetRecord(
	t *testing.T,
	record factory.JavaScriptRuntimeRecord,
	policy workflowpolicy.EffectivePolicy,
	maxTokens int64,
	maxRunDurationMs int64,
) {
	t.Helper()
	budget := record.Budget
	if budget == nil {
		t.Fatal("expected budget record")
	}
	if budget.MaxAgents != policy.MaxAgents || budget.Concurrency != policy.Concurrency {
		t.Fatalf("budget record = %#v, want maxAgents=%d concurrency=%d", budget, policy.MaxAgents, policy.Concurrency)
	}
	if budget.MaxTokens == nil || *budget.MaxTokens != maxTokens {
		t.Fatalf("budget maxTokens = %#v, want %d", budget.MaxTokens, maxTokens)
	}
	if budget.MaxRunDurationMs == nil || *budget.MaxRunDurationMs != maxRunDurationMs {
		t.Fatalf("budget maxRunDurationMs = %#v, want %d", budget.MaxRunDurationMs, maxRunDurationMs)
	}
	if budget.SandboxMode != "read-only" {
		t.Fatalf("budget sandboxMode = %q, want read-only", budget.SandboxMode)
	}
}

func assertProgressPrimitiveProjection(
	t *testing.T,
	sessionID string,
	value factory.TypedValue,
	wantURI string,
	policy workflowpolicy.EffectivePolicy,
	maxTokens int64,
) {
	t.Helper()
	projected := projectPrimaryJSON(t, sessionID, value)
	if projected["artifactRef"] != wantURI {
		t.Fatalf("projected artifactRef = %#v, want %q", projected["artifactRef"], wantURI)
	}
	budgetValue, ok := projected["budget"].(map[string]any)
	if !ok {
		t.Fatalf("projected budget = %#v, want object", projected["budget"])
	}
	if budgetValue["maxAgents"] != float64(policy.MaxAgents) {
		t.Fatalf("projected budget maxAgents = %#v", budgetValue["maxAgents"])
	}
	if budgetValue["maxTokens"] != float64(maxTokens) {
		t.Fatalf("projected budget maxTokens = %#v", budgetValue["maxTokens"])
	}
}

func TestRun_AgentRunRejectsInvalidPermissionsBeforeDispatch(t *testing.T) {
	for _, source := range []string{
		`return agent.run({ prompt: "review", permissions: "READ_ONLY" });`,
		`return agent.run({ prompt: "review", permissions: true });`,
		`const child = { prompt: "review" }; child.permissions = "READ_ONLY"; return agent.run(child);`,
	} {
		t.Run(source, func(t *testing.T) {
			stub := &stubChildExecutor{mode: stubChildExecutionMode}
			outcome, err := runtimeWorkflows.Run(context.Background(), factory.JavaScriptRuntimeRequest{
				Source: source, SourceRef: "inline", SessionID: "invalid-permissions",
				Policy: workflowpolicy.DefaultEffectivePolicy(),
			}, factory.JavaScriptRuntimeHooks{NewChildExecutor: func(string, factory.JavaScriptChildRecordSink, workflowpolicy.EffectivePolicy) factory.JavaScriptChildExecutor {
				return stub
			}})
			if err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			if outcome.OK || !strings.Contains(outcome.Failure.Message, `"permissions"`) {
				t.Fatalf("Run() outcome = %#v, want field-specific permissions failure", outcome)
			}
			if len(stub.executionRequests()) != 0 {
				t.Fatalf("executor requests = %#v, want none", stub.executionRequests())
			}
		})
	}
}

func TestRun_AgentRunDynamicObjectRejectsUnsupportedFieldsBeforeDispatch(t *testing.T) {
	unsupported := []string{
		"writableRoots", "allowNetwork", "network", "allowDangerFullAccess", "dangerFullAccess",
		"outputSchema", "concurrency", "maxAgents", "duration", "timeout", "timeoutMs",
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
