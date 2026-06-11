package workflowruntime_test

import (
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/orchestrators/javascript/policy"
	"github.com/portpowered/infinite-you/pkg/orchestrators/javascript/runtime"
)

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
	}{
		{
			fixture: "agent-run-policy-denied-model.workflow.js",
			policy:  basePolicy,
			want:    `policy denied: model "gpt-denied" is not listed in allowedModels`,
		},
		{
			fixture: "agent-run-policy-denied-command.workflow.js",
			policy:  basePolicy,
			want:    `policy denied: command "deploy" is not listed in allowedCommands`,
		},
		{
			fixture: "agent-run-policy-denied-reasoning.workflow.js",
			policy:  basePolicy,
			want:    `policy denied: reasoningEffort "high" is not listed in allowedReasoningEfforts`,
		},
		{
			fixture: "agent-run-policy-denied-sandbox.workflow.js",
			policy:  basePolicy,
			want:    `policy denied: sandbox "workspace-write" is not allowed when policy.mode is READ_ONLY`,
		},
		{
			fixture: "agent-run-policy-denied-writable-roots.workflow.js",
			policy:  basePolicy,
			want:    "policy denied: writableRoots are not allowed by effective policy",
		},
		{
			fixture: "agent-run-policy-denied-network.workflow.js",
			policy:  basePolicy,
			want:    "policy denied: network access is not allowed by effective policy",
		},
		{
			fixture: "agent-run-policy-denied-concurrency.workflow.js",
			policy:  basePolicy,
			want:    "policy denied: requested concurrency 4 exceeds policy concurrency 2",
		},
	}

	for _, tc := range cases {
		t.Run(tc.fixture, func(t *testing.T) {
			req := policyDeniedRequest(t, tc.fixture, tc.policy)
			outcome := runExecutionFailure(t, req)
			if outcome.Failure.Code != workflowruntime.CodeScriptError {
				t.Fatalf("failure code = %q, want %q", outcome.Failure.Code, workflowruntime.CodeScriptError)
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
		if record.Kind == workflowruntime.RecordKindArtifact {
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
	if outcome.Records[0].Kind != workflowruntime.RecordKindPhase {
		t.Fatalf("records[0].kind = %q, want phase", outcome.Records[0].Kind)
	}
	if outcome.Records[1].Kind != workflowruntime.RecordKindLog {
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
	if !ok || allowedChild["status"] != workflowruntime.ChildDispatchStatusCompleted {
		t.Fatalf("results[0] = %#v, want completed child", results[0])
	}
	deniedChild, ok := results[1].(map[string]any)
	if !ok || deniedChild["status"] != workflowruntime.ChildDispatchStatusFailed {
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

func policyDeniedRequest(t *testing.T, fixture string, policy workflowpolicy.EffectivePolicy) workflowruntime.Request {
	t.Helper()
	name := strings.TrimSuffix(fixture, ".workflow.js")
	return workflowruntime.Request{
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

func assertNoSuccessfulChildDispatchRecords(t *testing.T, records []workflowruntime.RuntimeRecord) {
	t.Helper()
	for _, record := range records {
		if record.Kind != workflowruntime.RecordKindChildDispatch || record.ChildDispatch == nil {
			continue
		}
		switch record.ChildDispatch.Status {
		case workflowruntime.ChildDispatchStatusQueued,
			workflowruntime.ChildDispatchStatusRunning,
			workflowruntime.ChildDispatchStatusCompleted:
			t.Fatalf("records = %#v, want no successful child dispatch records after denial", records)
		}
	}
}

func assertNoSuccessfulChildDispatchRecordsForLabel(t *testing.T, records []workflowruntime.RuntimeRecord, label string) {
	t.Helper()
	for _, record := range records {
		if record.Kind != workflowruntime.RecordKindChildDispatch || record.ChildDispatch == nil {
			continue
		}
		if record.ChildDispatch.Label != label {
			continue
		}
		switch record.ChildDispatch.Status {
		case workflowruntime.ChildDispatchStatusQueued,
			workflowruntime.ChildDispatchStatusRunning,
			workflowruntime.ChildDispatchStatusCompleted:
			t.Fatalf("records = %#v, want no successful child dispatch records for label %q", records, label)
		}
	}
}

func assertNoChildDispatchRecords(t *testing.T, records []workflowruntime.RuntimeRecord) {
	t.Helper()
	for _, record := range records {
		if record.Kind == workflowruntime.RecordKindChildDispatch {
			t.Fatalf("records = %#v, want no child dispatch records", records)
		}
	}
}

func completedChildRecords(records []workflowruntime.RuntimeRecord) int {
	count := 0
	for _, record := range records {
		if record.Kind == workflowruntime.RecordKindChildDispatch &&
			record.ChildDispatch != nil &&
			record.ChildDispatch.Status == workflowruntime.ChildDispatchStatusCompleted {
			count++
		}
	}
	return count
}
