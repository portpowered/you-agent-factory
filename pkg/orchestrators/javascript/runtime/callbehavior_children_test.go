package workflowruntime_test

import (
	"strings"
	"testing"

	workflowpolicy "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/policy"
	workflowruntime "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/runtime"
	"github.com/portpowered/infinite-you/pkg/orchestrators/javascript/runtime/callbehavior"
)

func TestCallBehavior_AgentRunInventoryMatchesExecution(t *testing.T) {
	record := callBehaviorRecord(t, "agent.run")
	assertCallBehaviorRecordDoesNotExposeHostContext(t, record)

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

	t.Run("async promise resolves child-result shape and emits child_dispatch", func(t *testing.T) {
		outcome := runFixtureWorkflow(t, "agent-run-fake-child", "agent-run-fake-child.workflow.js", workflowpolicy.DefaultEffectivePolicy())
		if !hasChildDispatchRecords(outcome.Records) {
			t.Fatalf("records = %#v, want child_dispatch records from inventory", outcome.Records)
		}
		projected := projectPrimaryJSON(t, "session-agent-run-fake-child", outcome.Value)
		child, ok := projected["child"].(map[string]any)
		if !ok {
			t.Fatalf("projected child = %#v, want object child-result", projected["child"])
		}
		if child["status"] != workflowruntime.ChildDispatchStatusCompleted {
			t.Fatalf("child status = %#v, want completed promise resolution", child["status"])
		}
		if child["dispatchId"] == "" || child["output"] == nil {
			t.Fatalf("child result = %#v, want dispatchId and output from inventory promise type", child)
		}
	})

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

	for _, policyCase := range agentRunPolicyCases(record.PolicyChecks) {
		t.Run("policy-"+policyCase.name, func(t *testing.T) {
			outcome := runFixtureWorkflowFailure(t, policyCase.fixture, policyCase.policy)
			if !failureMessageMatchesInventory(outcome.Failure.Message, policyCase.wantMessage) {
				t.Fatalf("failure message = %q, want inventory policy message %q", outcome.Failure.Message, policyCase.wantMessage)
			}
			if policyCase.deniedLabel != "" {
				assertNoSuccessfulChildDispatchRecordsForLabel(t, outcome.Records, policyCase.deniedLabel)
				return
			}
			assertNoSuccessfulChildDispatchRecords(t, outcome.Records)
		})
	}
}

func TestCallBehavior_ParallelInventoryMatchesExecution(t *testing.T) {
	record := callBehaviorRecord(t, "parallel")
	assertCallBehaviorRecordDoesNotExposeHostContext(t, record)

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

	t.Run("async promise resolves array and emits child_dispatch", func(t *testing.T) {
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
			if !ok || child["status"] != workflowruntime.ChildDispatchStatusCompleted {
				t.Fatalf("results[%d] = %#v, want completed child result", i, entry)
			}
		}
	})

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

	t.Run("policy-maxAgents", func(t *testing.T) {
		policy := workflowpolicy.DefaultEffectivePolicy()
		policy.MaxAgents = 2
		outcome := runFixtureWorkflowFailure(t, "parallel-max-agents-denied.workflow.js", policy)
		check := policyCheckMessage(t, record.PolicyChecks, "maxAgents")
		if !failureMessageMatchesInventory(outcome.Failure.Message, check) {
			t.Fatalf("failure message = %q, want inventory policy message %q", outcome.Failure.Message, check)
		}
		assertNoChildDispatchRecords(t, outcome.Records)
	})

	t.Run("policy-childRequest", func(t *testing.T) {
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
		if !ok || deniedChild["status"] != workflowruntime.ChildDispatchStatusFailed {
			t.Fatalf("results[1] = %#v, want failed child per inventory policy denial", results[1])
		}
		assertNoSuccessfulChildDispatchRecordsForLabel(t, outcome.Records, "denied-child")
	})
}

func TestCallBehavior_PipelineInventoryMatchesExecution(t *testing.T) {
	record := callBehaviorRecord(t, "pipeline")
	assertCallBehaviorRecordDoesNotExposeHostContext(t, record)

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

	t.Run("async promise resolves pipeline-result-array and emits child_dispatch", func(t *testing.T) {
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
			if !ok || item["status"] != workflowruntime.ChildDispatchStatusCompleted {
				t.Fatalf("results[%d] = %#v, want completed pipeline item", i, entry)
			}
			stages, ok := item["stages"].([]any)
			if !ok || len(stages) != 2 {
				t.Fatalf("results[%d].stages = %#v, want 2 ordered stages", i, item["stages"])
			}
		}
	})

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
			name:        "maxAgents",
			fixture:     "agent-run-policy-denied-max-agents.workflow.js",
			policy:      func() workflowpolicy.EffectivePolicy { p := workflowpolicy.DefaultEffectivePolicy(); p.MaxAgents = 1; p.Concurrency = 1; return p }(),
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

func runFixtureWorkflow(t *testing.T, name, fixture string, policy workflowpolicy.EffectivePolicy) workflowruntime.Outcome {
	t.Helper()
	req := workflowruntime.Request{
		Source:    readFixture(t, fixture),
		SourceRef: fixture,
		SessionID: "session-" + name,
		Args:      marshalArgs(t, map[string]any{"subject": "workflows"}),
		Metadata:  map[string]string{"name": name},
		Policy:    policy,
	}
	if strings.Contains(fixture, "agent-run-fake-child") {
		req.WorkerSettings = workflowruntime.WorkerSettingsConfig{Presets: map[string]workflowruntime.WorkerPreset{
			"careful": {},
		}}
	}
	return runSuccessful(t, req)
}

func runFixtureWorkflowFailure(t *testing.T, fixture string, policy workflowpolicy.EffectivePolicy) workflowruntime.Outcome {
	t.Helper()
	name := strings.TrimSuffix(fixture, ".workflow.js")
	req := workflowruntime.Request{
		Source:    readFixture(t, fixture),
		SourceRef: fixture,
		SessionID: "session-" + name,
		Args:      marshalArgs(t, map[string]any{"subject": "workflows"}),
		Metadata:  map[string]string{"name": name},
		Policy:    policy,
	}
	return runExecutionFailure(t, req)
}

func hasChildDispatchRecords(records []workflowruntime.RuntimeRecord) bool {
	for _, record := range records {
		if record.Kind == workflowruntime.RecordKindChildDispatch {
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
		return `return agent.run({ prompt: "review", modelProvider: "not-a-provider" });`
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

func assertInventoryFailureMessage(t *testing.T, outcome workflowruntime.Outcome, wantMessage string) {
	t.Helper()
	switch outcome.Failure.Code {
	case workflowruntime.CodeScriptError, workflowruntime.CodePreExecutionInvalid:
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
