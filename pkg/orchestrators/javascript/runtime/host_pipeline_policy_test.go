package workflowruntime_test

import (
	"strconv"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/orchestrators/javascript/policy"
	"github.com/portpowered/infinite-you/pkg/orchestrators/javascript/runtime"
)

func TestRun_PipelineStagedFakeChildren_PreservesItemStageOrder(t *testing.T) {
	source := readFixture(t, "pipeline-staged-fake-children.workflow.js")
	policy := workflowpolicy.DefaultEffectivePolicy()
	policy.MaxAgents = 16

	req := workflowruntime.Request{
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

	req := workflowruntime.Request{
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
	if !ok || successItem["status"] != workflowruntime.ChildDispatchStatusCompleted {
		t.Fatalf("results[0] = %#v, want completed item", results[0])
	}
	failedItem, ok := results[1].(map[string]any)
	if !ok || failedItem["status"] != workflowruntime.ChildDispatchStatusFailed {
		t.Fatalf("results[1].status = %#v, want FAILED", failedItem["status"])
	}
	stages, ok := failedItem["stages"].([]any)
	if !ok || len(stages) != 2 {
		t.Fatalf("results[1].stages = %#v, want 2 stages", failedItem["stages"])
	}
	editStage, ok := stages[0].(map[string]any)
	if !ok || editStage["status"] != workflowruntime.ChildDispatchStatusCompleted {
		t.Fatalf("results[1].stages[0] = %#v, want completed edit stage", stages[0])
	}
	reviewStage, ok := stages[1].(map[string]any)
	if !ok || reviewStage["status"] != workflowruntime.ChildDispatchStatusFailed {
		t.Fatalf("results[1].stages[1] = %#v, want failed review stage", stages[1])
	}
	if reviewStage["diagnostic"] == "" {
		t.Fatal("results[1].stages[1].diagnostic is empty")
	}
}

func assertPipelineItemOrder(t *testing.T, outcome workflowruntime.Outcome, wantItems []string) {
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
		if itemResult["status"] != workflowruntime.ChildDispatchStatusCompleted {
			t.Fatalf("results[%d].status = %#v, want COMPLETED", i, itemResult["status"])
		}
	}
}

func assertPipelineStageTransitions(t *testing.T, outcome workflowruntime.Outcome, wantItems, wantStages int) {
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

func assertPipelineReviewUsesEditOutput(t *testing.T, outcome workflowruntime.Outcome) {
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
	if !ok || editStage["status"] != workflowruntime.ChildDispatchStatusCompleted {
		t.Fatalf("results[0].stages[0] = %#v, want completed edit stage", stages[0])
	}
	reviewStage, ok := stages[1].(map[string]any)
	if !ok || reviewStage["status"] != workflowruntime.ChildDispatchStatusCompleted {
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

func childDispatchRecords(records []workflowruntime.RuntimeRecord) []workflowruntime.RuntimeRecord {
	var childRecords []workflowruntime.RuntimeRecord
	for _, record := range records {
		if record.Kind == workflowruntime.RecordKindChildDispatch {
			childRecords = append(childRecords, record)
		}
	}
	return childRecords
}

func completedChildLabels(records []workflowruntime.RuntimeRecord) []string {
	var labels []string
	for _, record := range records {
		if record.ChildDispatch == nil {
			continue
		}
		if record.ChildDispatch.Status != workflowruntime.ChildDispatchStatusCompleted {
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
			code:    workflowruntime.CodePreExecutionInvalid,
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
			code:    workflowruntime.CodePreExecutionInvalid,
		},
		{
			fixture: "agent-run-policy-denied-writable-roots.workflow.js",
			policy:  basePolicy,
			want:    `agent.run() does not support field "writableRoots"`,
			code:    workflowruntime.CodePreExecutionInvalid,
		},
		{
			fixture: "agent-run-policy-denied-network.workflow.js",
			policy:  basePolicy,
			want:    `agent.run() does not support field "network"`,
			code:    workflowruntime.CodePreExecutionInvalid,
		},
		{
			fixture: "agent-run-policy-denied-concurrency.workflow.js",
			policy:  basePolicy,
			want:    `agent.run() does not support field "concurrency"`,
			code:    workflowruntime.CodePreExecutionInvalid,
		},
	}

	for _, tc := range cases {
		t.Run(tc.fixture, func(t *testing.T) {
			req := policyDeniedRequest(t, tc.fixture, tc.policy)
			outcome := runExecutionFailure(t, req)
			wantCode := tc.code
			if wantCode == "" {
				wantCode = workflowruntime.CodeScriptError
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
