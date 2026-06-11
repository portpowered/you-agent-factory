package workflowruntime_test

import (
	"strings"
	"testing"

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
