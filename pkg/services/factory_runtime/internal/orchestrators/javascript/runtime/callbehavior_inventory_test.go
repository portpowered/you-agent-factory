package workflowruntime_test

import (
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	workflowpolicy "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/orchestratorcontract"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/tooling/javascript/callbehavior"
)

func TestCallBehavior_WorkflowFinalInventoryMatchesExecution(t *testing.T) {
	record := callBehaviorRecord(t, "workflow.final")
	assertCallBehaviorRecordDoesNotExposeHostContext(t, record)

	if record.Return == nil || record.Return.SyncType != "undefined" {
		t.Fatalf("workflow.final return = %#v, want sync undefined", record.Return)
	}
	if len(record.Parameters) != 1 {
		t.Fatalf("workflow.final parameters = %#v, want one optional value parameter", record.Parameters)
	}
	if record.Parameters[0].Required {
		t.Fatalf("workflow.final value parameter required = true, want optional per inventory")
	}
	if record.Determinism == "" {
		t.Fatal("workflow.final missing determinism note")
	}

	t.Run("records terminal value from optional argument", func(t *testing.T) {
		outcome := runInlineWorkflow(t, "workflow-final-terminal", `
workflow.final({ label: "terminal", mechanism: "workflow.final", count: 2 });
return { label: "returned", mechanism: "return", count: 1 };
`)
		projected := projectPrimaryJSON(t, "session-workflow-final-terminal", outcome.Value)
		assertProjectedFields(t, projected, map[string]any{
			"label":     "terminal",
			"mechanism": "workflow.final",
			"count":     float64(2),
		})
	})

	t.Run("final wins over returned workflow value", func(t *testing.T) {
		outcome := runInlineWorkflow(t, "workflow-final-precedence", readFixture(t, "workflow-final-and-return.workflow.js"))
		projected := projectPrimaryJSON(t, "session-workflow-final-precedence", outcome.Value)
		assertProjectedFields(t, projected, map[string]any{
			"mechanism": "workflow.final",
		})
		if projected["mechanism"] == "return" {
			t.Fatalf("projected mechanism = return, want workflow.final precedence from inventory")
		}
	})
}

func TestCallBehavior_WorkflowCheckpointInventoryMatchesExecution(t *testing.T) {
	record := callBehaviorRecord(t, "workflow.checkpoint")
	assertCallBehaviorRecordDoesNotExposeHostContext(t, record)
	assertWorkflowCheckpointInventoryRecord(t, record)

	t.Run("emits checkpoint record with label and state", func(t *testing.T) {
		assertWorkflowCheckpointEmitsLabelAndState(t)
	})
	testWorkflowCheckpointInventoryErrors(t, record)
}

func assertWorkflowCheckpointInventoryRecord(t *testing.T, record callbehavior.CallBehaviorRecord) {
	t.Helper()
	if len(record.Parameters) != 1 || !record.Parameters[0].Required || record.Parameters[0].Type != "object" {
		t.Fatalf("workflow.checkpoint parameters = %#v, want one required object", record.Parameters)
	}
	if record.Return == nil || record.Return.SyncType != "undefined" {
		t.Fatalf("workflow.checkpoint return = %#v, want sync undefined", record.Return)
	}
	if len(record.EmittedRecords) != 1 || record.EmittedRecords[0] != "checkpoint" {
		t.Fatalf("workflow.checkpoint emittedRecords = %v, want [checkpoint]", record.EmittedRecords)
	}
	if record.ResumeNotes == "" {
		t.Fatal("workflow.checkpoint missing resume notes")
	}
}

func assertWorkflowCheckpointEmitsLabelAndState(t *testing.T) {
	t.Helper()
	outcome := runInlineWorkflow(t, "workflow-checkpoint-success", `
workflow.checkpoint({ label: "after-step", state: { step: 2, tag: "alpha" } });
return { ok: true };
`)
	checkpoint := findCheckpointRecord(t, outcome.Records, "after-step")
	if checkpoint.State["step"] != float64(2) || checkpoint.State["tag"] != "alpha" {
		t.Fatalf("checkpoint state = %#v, want step=2 tag=alpha", checkpoint.State)
	}
}

func testWorkflowCheckpointInventoryErrors(t *testing.T, record callbehavior.CallBehaviorRecord) {
	t.Helper()
	for _, errCase := range record.Errors {
		t.Run(errCase.Condition, func(t *testing.T) {
			assertWorkflowCheckpointInventoryError(t, errCase.Condition, errCase.Message)
		})
	}
}

func assertWorkflowCheckpointInventoryError(t *testing.T, condition, wantMessage string) {
	t.Helper()
	source := checkpointErrorSource(condition)
	if source == "" {
		t.Fatalf("missing inline source for inventory error condition %q", condition)
	}
	outcome := runInlineWorkflowFailure(t, "workflow-checkpoint-"+condition, source)
	if outcome.Failure.Code != factory.JavaScriptRuntimeCodeScriptError {
		t.Fatalf("failure code = %q, want %q", outcome.Failure.Code, factory.JavaScriptRuntimeCodeScriptError)
	}
	if !strings.Contains(outcome.Failure.Message, wantMessage) {
		t.Fatalf("failure message = %q, want inventory message %q", outcome.Failure.Message, wantMessage)
	}
	if hasCheckpointRecords(outcome.Records) {
		t.Fatalf("records = %#v, want no checkpoint record after %q", outcome.Records, condition)
	}
}

func TestCallBehavior_WorkflowResumeStateInventoryMatchesExecution(t *testing.T) {
	record := callBehaviorRecord(t, "workflow.resumeState")
	assertCallBehaviorRecordDoesNotExposeHostContext(t, record)

	if len(record.Parameters) != 0 {
		t.Fatalf("workflow.resumeState parameters = %#v, want zero parameters", record.Parameters)
	}
	if record.Return == nil || record.Return.SyncType != "object-or-undefined" {
		t.Fatalf("workflow.resumeState return = %#v, want object-or-undefined", record.Return)
	}
	if record.ResumeNotes == "" {
		t.Fatal("workflow.resumeState missing resume notes")
	}

	t.Run("returns undefined when resume state absent", func(t *testing.T) {
		outcome := runInlineWorkflow(t, "workflow-resume-state-absent", `
const resumed = workflow.resumeState();
return { hasResumeState: resumed !== undefined };
`)
		projected := projectPrimaryJSON(t, "session-workflow-resume-state-absent", outcome.Value)
		if projected["hasResumeState"] != false {
			t.Fatalf("projected hasResumeState = %#v, want false when absent", projected["hasResumeState"])
		}
	})

	t.Run("returns bound checkpoint state on resumed session", func(t *testing.T) {
		req := factory.JavaScriptRuntimeRequest{
			Source: `
const resumed = workflow.resumeState();
return {
  step: resumed ? resumed.step : null,
  firstLabel: resumed ? resumed.firstLabel : null,
};
`,
			SourceRef: "inline-resume-state-rehydrated",
			SessionID: "session-workflow-resume-state-rehydrated",
			Args:      marshalArgs(t, map[string]any{}),
			Metadata:  map[string]string{"name": "resume-state-rehydrated"},
			Policy:    workflowpolicy.DefaultEffectivePolicy(),
			Resume: &factory.JavaScriptResumeContext{
				CheckpointState: map[string]any{
					"step":       float64(1),
					"firstLabel": "step-one",
				},
			},
		}
		outcome := runSuccessful(t, req)
		projected := projectPrimaryJSON(t, req.SessionID, outcome.Value)
		assertProjectedFields(t, projected, map[string]any{
			"step":       float64(1),
			"firstLabel": "step-one",
		})
	})

	for _, errCase := range record.Errors {
		t.Run(errCase.Condition, func(t *testing.T) {
			source := resumeStateErrorSource(errCase.Condition)
			if source == "" {
				t.Fatalf("missing inline source for inventory error condition %q", errCase.Condition)
			}
			outcome := runInlineWorkflowFailure(t, "workflow-resume-state-"+errCase.Condition, source)
			if outcome.Failure.Code != factory.JavaScriptRuntimeCodeScriptError {
				t.Fatalf("failure code = %q, want %q", outcome.Failure.Code, factory.JavaScriptRuntimeCodeScriptError)
			}
			if !strings.Contains(outcome.Failure.Message, errCase.Message) {
				t.Fatalf("failure message = %q, want inventory message %q", outcome.Failure.Message, errCase.Message)
			}
		})
	}
}

func TestCallBehavior_AgentRunInventoryMatchesExecution(t *testing.T) {
	record := callBehaviorRecord(t, "agent.run")
	assertCallBehaviorRecordDoesNotExposeHostContext(t, record)
	assertAgentRunInventoryRecord(t, record)

	t.Run("async promise resolves child-result shape and emits child_dispatch", func(t *testing.T) {
		assertAgentRunAsyncPromiseResolvesChildResult(t)
	})
	testAgentRunInventoryErrors(t, record)
	testAgentRunPolicyDenials(t, record)
}

func assertAgentRunInventoryRecord(t *testing.T, record callbehavior.CallBehaviorRecord) {
	t.Helper()
	if len(record.Parameters) != 1 || !record.Parameters[0].Required || record.Parameters[0].Type != "object" {
		t.Fatalf("agent.run parameters = %#v, want one required object", record.Parameters)
	}
	if record.Return == nil || !record.Return.Async || record.Return.PromiseType != "child-result-object" {
		t.Fatalf("agent.run return = %#v, want async child-result-object promise", record.Return)
	}
	if len(record.EmittedRecords) != 1 || record.EmittedRecords[0] != "child_dispatch" {
		t.Fatalf("agent.run emittedRecords = %v, want [child_dispatch]", record.EmittedRecords)
	}
	if len(record.PolicyChecks) == 0 {
		t.Fatal("agent.run missing policy checks")
	}
}

func assertAgentRunAsyncPromiseResolvesChildResult(t *testing.T) {
	t.Helper()
	outcome := runFixtureWorkflow(t, "agent-run-fake-child", "agent-run-fake-child.workflow.js", workflowpolicy.DefaultEffectivePolicy())
	if !hasChildDispatchRecords(outcome.Records) {
		t.Fatalf("records = %#v, want child_dispatch records from inventory", outcome.Records)
	}
	projected := projectPrimaryJSON(t, "session-agent-run-fake-child", outcome.Value)
	child, ok := projected["child"].(map[string]any)
	if !ok {
		t.Fatalf("projected child = %#v, want object child-result", projected["child"])
	}
	if child["status"] != factory.JavaScriptChildDispatchStatusCompleted {
		t.Fatalf("child status = %#v, want completed promise resolution", child["status"])
	}
	if child["dispatchId"] == "" || child["output"] == nil {
		t.Fatalf("child result = %#v, want dispatchId and output from inventory promise type", child)
	}
}

func testAgentRunInventoryErrors(t *testing.T, record callbehavior.CallBehaviorRecord) {
	t.Helper()
	for _, errCase := range record.Errors {
		t.Run(errCase.Condition, func(t *testing.T) {
			source := agentRunErrorSource(errCase.Condition)
			if source == "" {
				t.Skip("condition not observable from installed JS host surface")
			}
			outcome := runInlineWorkflowFailure(t, "agent-run-"+errCase.Condition, source)
			assertInventoryFailureMessage(t, outcome, errCase.Message)
			assertNoChildDispatchRecords(t, outcome.Records)
		})
	}
}

func testAgentRunPolicyDenials(t *testing.T, record callbehavior.CallBehaviorRecord) {
	t.Helper()
	for _, policyCase := range agentRunPolicyCases(record.PolicyChecks) {
		t.Run("policy-"+policyCase.name, func(t *testing.T) {
			assertAgentRunPolicyDenial(t, policyCase)
		})
	}
}

func assertAgentRunPolicyDenial(t *testing.T, policyCase agentRunPolicyCase) {
	t.Helper()
	outcome := runFixtureWorkflowFailure(t, policyCase.fixture, policyCase.policy)
	if !failureMessageMatchesInventory(outcome.Failure.Message, policyCase.wantMessage) {
		t.Fatalf("failure message = %q, want inventory policy message %q", outcome.Failure.Message, policyCase.wantMessage)
	}
	if policyCase.deniedLabel != "" {
		assertNoSuccessfulChildDispatchRecordsForLabel(t, outcome.Records, policyCase.deniedLabel)
		return
	}
	assertNoSuccessfulChildDispatchRecords(t, outcome.Records)
}

func TestCallBehavior_ParallelInventoryMatchesExecution(t *testing.T) {
	record := callBehaviorRecord(t, "parallel")
	assertCallBehaviorRecordDoesNotExposeHostContext(t, record)
	assertParallelInventoryRecord(t, record)

	t.Run("async promise resolves array and emits child_dispatch", func(t *testing.T) {
		assertParallelAsyncPromiseResolvesArray(t)
	})
	testParallelInventoryErrors(t, record)
	t.Run("policy-maxAgents", func(t *testing.T) {
		assertParallelMaxAgentsPolicyDenial(t, record)
	})
	t.Run("policy-childRequest", func(t *testing.T) {
		assertParallelChildRequestPolicyDenial(t)
	})
}

func assertParallelInventoryRecord(t *testing.T, record callbehavior.CallBehaviorRecord) {
	t.Helper()
	if record.Callback == nil || record.Callback.Role != "item" {
		t.Fatalf("parallel callback = %#v, want item callback shape", record.Callback)
	}
	if record.Return == nil || !record.Return.Async || record.Return.PromiseType != "array" {
		t.Fatalf("parallel return = %#v, want async array promise", record.Return)
	}
	if len(record.EmittedRecords) != 1 || record.EmittedRecords[0] != "child_dispatch" {
		t.Fatalf("parallel emittedRecords = %v, want [child_dispatch]", record.EmittedRecords)
	}
	if record.Determinism == "" {
		t.Fatal("parallel missing determinism note")
	}
}

func assertParallelAsyncPromiseResolvesArray(t *testing.T) {
	t.Helper()
	policy := workflowpolicy.DefaultEffectivePolicy()
	policy.MaxAgents = 8
	outcome := runFixtureWorkflow(t, "parallel-fake-children", "parallel-fake-children.workflow.js", policy)
	if !hasChildDispatchRecords(outcome.Records) {
		t.Fatalf("records = %#v, want child_dispatch records", outcome.Records)
	}
	projected := projectPrimaryJSON(t, "session-parallel-fake-children", outcome.Value)
	results, ok := projected["results"].([]any)
	if !ok || len(results) != 4 {
		t.Fatalf("projected results = %#v, want 4 promise-resolved entries", projected["results"])
	}
	for i, entry := range results {
		child, ok := entry.(map[string]any)
		if !ok || child["status"] != factory.JavaScriptChildDispatchStatusCompleted {
			t.Fatalf("results[%d] = %#v, want completed child result", i, entry)
		}
	}
}

func testParallelInventoryErrors(t *testing.T, record callbehavior.CallBehaviorRecord) {
	t.Helper()
	for _, errCase := range record.Errors {
		t.Run(errCase.Condition, func(t *testing.T) {
			source := parallelErrorSource(errCase.Condition)
			if source == "" {
				t.Fatalf("missing inline source for inventory error condition %q", errCase.Condition)
			}
			outcome := runInlineWorkflowFailure(t, "parallel-"+errCase.Condition, source)
			assertInventoryFailureMessage(t, outcome, errCase.Message)
			assertNoChildDispatchRecords(t, outcome.Records)
		})
	}
}

func assertParallelMaxAgentsPolicyDenial(t *testing.T, record callbehavior.CallBehaviorRecord) {
	t.Helper()
	policy := workflowpolicy.DefaultEffectivePolicy()
	policy.MaxAgents = 2
	outcome := runFixtureWorkflowFailure(t, "parallel-max-agents-denied.workflow.js", policy)
	check := policyCheckMessage(t, record.PolicyChecks, "maxAgents")
	if !failureMessageMatchesInventory(outcome.Failure.Message, check) {
		t.Fatalf("failure message = %q, want inventory policy message %q", outcome.Failure.Message, check)
	}
	assertNoChildDispatchRecords(t, outcome.Records)
}

func assertParallelChildRequestPolicyDenial(t *testing.T) {
	t.Helper()
	policy := workflowpolicy.DefaultEffectivePolicy()
	policy.MaxAgents = 4
	policy.Concurrency = 2
	policy.AllowedModels = []string{"gpt-allowed"}
	outcome := runFixtureWorkflow(t, "parallel-policy-denied-item", "parallel-policy-denied-item.workflow.js", policy)
	projected := projectPrimaryJSON(t, "session-parallel-policy-denied-item", outcome.Value)
	results, ok := projected["results"].([]any)
	if !ok || len(results) != 2 {
		t.Fatalf("projected results = %#v, want 2 entries", projected["results"])
	}
	deniedChild, ok := results[1].(map[string]any)
	if !ok || deniedChild["status"] != factory.JavaScriptChildDispatchStatusFailed {
		t.Fatalf("results[1] = %#v, want failed child per inventory policy denial", results[1])
	}
	assertNoSuccessfulChildDispatchRecordsForLabel(t, outcome.Records, "denied-child")
}

func TestCallBehavior_PipelineInventoryMatchesExecution(t *testing.T) {
	record := callBehaviorRecord(t, "pipeline")
	assertCallBehaviorRecordDoesNotExposeHostContext(t, record)
	assertPipelineInventoryRecord(t, record)

	t.Run("async promise resolves pipeline-result-array and emits child_dispatch", func(t *testing.T) {
		assertPipelineAsyncPromiseResolvesStages(t)
	})
	testPipelineInventoryErrors(t, record)
}

func assertPipelineInventoryRecord(t *testing.T, record callbehavior.CallBehaviorRecord) {
	t.Helper()
	if len(record.Parameters) != 3 {
		t.Fatalf("pipeline parameters = %#v, want items, worker, optional next", record.Parameters)
	}
	if record.Callback == nil || record.Callback.Role != "worker" || len(record.Callback.Parameters) < 3 {
		t.Fatalf("pipeline callback = %#v, want worker callback with item and index", record.Callback)
	}
	if record.Return == nil || !record.Return.Async || record.Return.PromiseType != "pipeline-result-array" {
		t.Fatalf("pipeline return = %#v, want async pipeline-result-array promise", record.Return)
	}
	if record.Determinism == "" {
		t.Fatal("pipeline missing determinism note")
	}
}

func assertPipelineAsyncPromiseResolvesStages(t *testing.T) {
	t.Helper()
	policy := workflowpolicy.DefaultEffectivePolicy()
	policy.MaxAgents = 16
	outcome := runFixtureWorkflow(t, "pipeline-staged-fake-children", "pipeline-staged-fake-children.workflow.js", policy)
	if !hasChildDispatchRecords(outcome.Records) {
		t.Fatalf("records = %#v, want child_dispatch records", outcome.Records)
	}
	projected := projectPrimaryJSON(t, "session-pipeline-staged-fake-children", outcome.Value)
	results, ok := projected["results"].([]any)
	if !ok || len(results) != 3 {
		t.Fatalf("projected results = %#v, want 3 pipeline items", projected["results"])
	}
	for i, entry := range results {
		item, ok := entry.(map[string]any)
		if !ok || item["status"] != factory.JavaScriptChildDispatchStatusCompleted {
			t.Fatalf("results[%d] = %#v, want completed pipeline item", i, entry)
		}
		stages, ok := item["stages"].([]any)
		if !ok || len(stages) != 2 {
			t.Fatalf("results[%d].stages = %#v, want 2 ordered stages", i, item["stages"])
		}
	}
}

func testPipelineInventoryErrors(t *testing.T, record callbehavior.CallBehaviorRecord) {
	t.Helper()
	for _, errCase := range record.Errors {
		t.Run(errCase.Condition, func(t *testing.T) {
			source := pipelineErrorSource(errCase.Condition)
			if source == "" {
				t.Fatalf("missing inline source for inventory error condition %q", errCase.Condition)
			}
			outcome := runInlineWorkflowFailure(t, "pipeline-"+errCase.Condition, source)
			assertInventoryFailureMessage(t, outcome, errCase.Message)
			assertNoChildDispatchRecords(t, outcome.Records)
		})
	}
}

type agentRunPolicyCase struct {
	name        string
	fixture     string
	policy      workflowpolicy.EffectivePolicy
	wantMessage string
	deniedLabel string
}

func agentRunPolicyCases(checks []callbehavior.PolicyCheck) []agentRunPolicyCase {
	base := workflowpolicy.DefaultEffectivePolicy()
	base.MaxAgents = 4
	base.Concurrency = 2
	base.AllowedModels = []string{"gpt-allowed"}
	base.AllowedReasoningEfforts = []string{"low"}
	base.AllowedCommands = []string{"review"}

	cases := []agentRunPolicyCase{
		{
			name:    "maxAgents",
			fixture: "agent-run-policy-denied-max-agents.workflow.js",
			policy: func() workflowpolicy.EffectivePolicy {
				p := workflowpolicy.DefaultEffectivePolicy()
				p.MaxAgents = 1
				p.Concurrency = 1
				return p
			}(),
			wantMessage: policyCheckMessageFromKind(checks, "maxAgents"),
			deniedLabel: "second-child",
		},
		{
			name:        "allowedModels",
			fixture:     "agent-run-policy-denied-model.workflow.js",
			policy:      base,
			wantMessage: `policy denied: model "gpt-denied" is not listed in allowedModels`,
		},
		{
			name:        "allowedCommands",
			fixture:     "agent-run-policy-denied-command.workflow.js",
			policy:      base,
			wantMessage: `agent.run() does not support field "command"`,
		},
		{
			name:        "sandboxMode",
			fixture:     "agent-run-policy-denied-sandbox.workflow.js",
			policy:      base,
			wantMessage: `agent.run() does not support field "sandbox"`,
		},
		{
			name:        "writableRoots",
			fixture:     "agent-run-policy-denied-writable-roots.workflow.js",
			policy:      base,
			wantMessage: `agent.run() does not support field "writableRoots"`,
		},
		{
			name:        "allowNetwork",
			fixture:     "agent-run-policy-denied-network.workflow.js",
			policy:      base,
			wantMessage: `agent.run() does not support field "network"`,
		},
		{
			name:        "concurrency",
			fixture:     "agent-run-policy-denied-concurrency.workflow.js",
			policy:      base,
			wantMessage: `agent.run() does not support field "concurrency"`,
		},
		{
			name:        "allowedReasoningEfforts",
			fixture:     "agent-run-policy-denied-reasoning.workflow.js",
			policy:      base,
			wantMessage: `policy denied: reasoningEffort "high" is not listed in allowedReasoningEfforts`,
		},
	}
	return cases
}

func policyCheckMessage(t *testing.T, checks []callbehavior.PolicyCheck, kind string) string {
	t.Helper()
	for _, check := range checks {
		if check.Kind == kind && check.Message != "" {
			return check.Message
		}
	}
	t.Fatalf("policy check %q not found in inventory", kind)
	return ""
}

func policyCheckMessageFromKind(checks []callbehavior.PolicyCheck, kind string) string {
	for _, check := range checks {
		if check.Kind == kind && check.Message != "" {
			return check.Message
		}
	}
	return "policy denied"
}

func runFixtureWorkflow(t *testing.T, name, fixture string, policy workflowpolicy.EffectivePolicy) factory.JavaScriptRuntimeOutcome {
	t.Helper()
	req := factory.JavaScriptRuntimeRequest{
		Source:    readFixture(t, fixture),
		SourceRef: fixture,
		SessionID: "session-" + name,
		Args:      marshalArgs(t, map[string]any{"subject": "workflows"}),
		Metadata:  map[string]string{"name": name},
		Policy:    policy,
	}
	if strings.Contains(fixture, "agent-run-fake-child") {
		req.WorkerSettings = factory.JavaScriptWorkerSettings{Presets: map[string]factory.JavaScriptWorkerPreset{
			"careful": {},
		}}
	}
	return runSuccessful(t, req)
}

func runFixtureWorkflowFailure(t *testing.T, fixture string, policy workflowpolicy.EffectivePolicy) factory.JavaScriptRuntimeOutcome {
	t.Helper()
	name := strings.TrimSuffix(fixture, ".workflow.js")
	req := factory.JavaScriptRuntimeRequest{
		Source:    readFixture(t, fixture),
		SourceRef: fixture,
		SessionID: "session-" + name,
		Args:      marshalArgs(t, map[string]any{"subject": "workflows"}),
		Metadata:  map[string]string{"name": name},
		Policy:    policy,
	}
	return runExecutionFailure(t, req)
}

func hasChildDispatchRecords(records []factory.JavaScriptRuntimeRecord) bool {
	for _, record := range records {
		if record.Kind == factory.JavaScriptRecordKindChildDispatch {
			return true
		}
	}
	return false
}

func agentRunErrorSource(condition string) string {
	switch condition {
	case "missing-or-non-object-argument":
		return `const spec = 1; agent.run(spec); return { ok: true };`
	case "unsupported-field":
		return `const child = { prompt: "review" }; child["writableRoots"] = "/tmp"; return agent.run(child);`
	case "missing-prompt":
		return `const child = {}; child.label = "missing-prompt"; return agent.run(child);`
	case "unknown-preset":
		return ""
	case "unsupported-model-provider":
		return `return agent.run({ prompt: "review", modelProvider: "Not_A_Provider" });`
	case "unsupported-reasoning-effort":
		return `return agent.run({ prompt: "review", reasoningEffort: "not-an-effort" });`
	default:
		return ""
	}
}

func parallelErrorSource(condition string) string {
	switch condition {
	case "missing-or-non-array-argument":
		return `const items = 1; return parallel(items);`
	case "null-or-undefined-item":
		return `return parallel([{ prompt: "ok", label: "valid" }, null]);`
	case "invalid-item-shape":
		return `return parallel([{ notPrompt: "bad" }]);`
	default:
		return ""
	}
}

func pipelineErrorSource(condition string) string {
	switch condition {
	case "missing-items-or-worker":
		return `const p = pipeline; p(function () {}); return { ok: true };`
	case "missing-or-non-array-items":
		return `const items = 1; return pipeline(items, function () {});`
	case "missing-worker-function":
		return `return pipeline([], 1);`
	case "invalid-next-function":
		return `return pipeline([], function () {}, 1);`
	case "null-or-undefined-item":
		return `return pipeline([null], function () { return { ok: true }; });`
	default:
		return ""
	}
}

func assertInventoryFailureMessage(t *testing.T, outcome factory.JavaScriptRuntimeOutcome, wantMessage string) {
	t.Helper()
	switch outcome.Failure.Code {
	case factory.JavaScriptRuntimeCodeScriptError, factory.JavaScriptRuntimeCodePreExecutionInvalid:
	default:
		t.Fatalf("failure code = %q, want script or pre-execution error", outcome.Failure.Code)
	}
	if !failureMessageMatchesInventory(outcome.Failure.Message, wantMessage) {
		t.Fatalf("failure message = %q, want inventory message %q", outcome.Failure.Message, wantMessage)
	}
}

func failureMessageMatchesInventory(actual, inventory string) bool {
	if strings.Contains(actual, inventory) {
		return true
	}
	if strings.Contains(inventory, "exceeds maxAgents") {
		return strings.Contains(actual, "exceeds maxAgents")
	}
	return false
}

func callBehaviorRecord(t *testing.T, path string) callbehavior.CallBehaviorRecord {
	t.Helper()
	for _, record := range callbehavior.ProjectInstalledCallBehavior().Records {
		if record.Path == path {
			return record
		}
	}
	t.Fatalf("call-behavior record %q not found", path)
	return callbehavior.CallBehaviorRecord{}
}

func assertCallBehaviorRecordDoesNotExposeHostContext(t *testing.T, record callbehavior.CallBehaviorRecord) {
	t.Helper()
	for _, forbidden := range callbehavior.ForbiddenRootGlobals {
		if record.Path == forbidden || strings.HasPrefix(record.Path, forbidden+".") {
			t.Fatalf("record path %q exposes forbidden host context %q", record.Path, forbidden)
		}
	}
}

func runInlineWorkflow(t *testing.T, name, source string) factory.JavaScriptRuntimeOutcome {
	t.Helper()
	req := factory.JavaScriptRuntimeRequest{
		Source:    source,
		SourceRef: name + ".workflow.js",
		SessionID: "session-" + name,
		Args:      marshalArgs(t, map[string]any{}),
		Metadata:  map[string]string{"name": name},
		Policy:    workflowpolicy.DefaultEffectivePolicy(),
	}
	return runSuccessful(t, req)
}

func runInlineWorkflowFailure(t *testing.T, name, source string) factory.JavaScriptRuntimeOutcome {
	t.Helper()
	req := factory.JavaScriptRuntimeRequest{
		Source:    source,
		SourceRef: name + ".workflow.js",
		SessionID: "session-" + name,
		Args:      marshalArgs(t, map[string]any{}),
		Metadata:  map[string]string{"name": name},
		Policy:    workflowpolicy.DefaultEffectivePolicy(),
	}
	return runExecutionFailure(t, req)
}

func checkpointErrorSource(condition string) string {
	switch condition {
	case "missing-or-non-object-argument":
		return `const spec = 1; workflow.checkpoint(spec); return { ok: true };`
	case "missing-label":
		return `const spec = { state: { step: 1 } }; workflow.checkpoint(spec); return { ok: true };`
	case "non-json-state":
		return `workflow.checkpoint({ label: "bad-state", state: () => {} }); return { ok: true };`
	default:
		return ""
	}
}

func resumeStateErrorSource(condition string) string {
	switch condition {
	case "arguments-provided":
		return `workflow.resumeState.apply(workflow, [{}]); return { ok: true };`
	default:
		return ""
	}
}

func findCheckpointRecord(t *testing.T, records []factory.JavaScriptRuntimeRecord, label string) *factory.JavaScriptCheckpointRecord {
	t.Helper()
	for _, record := range records {
		if record.Kind == factory.JavaScriptRecordKindCheckpoint && record.Checkpoint != nil && record.Checkpoint.Label == label {
			return record.Checkpoint
		}
	}
	t.Fatalf("checkpoint record with label %q not found in %#v", label, records)
	return nil
}

func hasCheckpointRecords(records []factory.JavaScriptRuntimeRecord) bool {
	for _, record := range records {
		if record.Kind == factory.JavaScriptRecordKindCheckpoint {
			return true
		}
	}
	return false
}
