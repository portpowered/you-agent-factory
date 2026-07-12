package workflowruntime_test

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/portpowered/infinite-you/pkg/orchestrators/javascript/policy"
	"github.com/portpowered/infinite-you/pkg/orchestrators/javascript/result"
	"github.com/portpowered/infinite-you/pkg/orchestrators/javascript/runtime"
)

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
	requests []workflowruntime.ChildExecutionRequest
	mode     string
}

func (s *stubChildExecutor) Execute(_ context.Context, req workflowruntime.ChildExecutionRequest) (workflowruntime.ChildExecutionResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.labels = append(s.labels, req.Label)
	s.requests = append(s.requests, req)
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

func (s *stubChildExecutor) executionRequests() []workflowruntime.ChildExecutionRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]workflowruntime.ChildExecutionRequest(nil), s.requests...)
}

func TestRun_AgentRunDynamicObjectCarriesCanonicalFieldsToExecutor(t *testing.T) {
	stub := &stubChildExecutor{mode: stubChildExecutionMode}
	source := `const child = { prompt: " review ", label: " reviewer ", preset: " careful ", modelProvider: " codex ", model: " gpt-test ", reasoningEffort: " high " }; agent.run(child); return { ok: true };`
	outcome, err := workflowruntime.Run(context.Background(), workflowruntime.Request{
		Source: source, SourceRef: "inline", SessionID: "canonical-child-fields",
		Policy: workflowpolicy.DefaultEffectivePolicy(),
	}, workflowruntime.Hooks{NewChildExecutor: func(string, workflowruntime.ChildRecordSink, workflowpolicy.EffectivePolicy) workflowruntime.ChildExecutor {
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
	if got.Prompt != "review" || got.Label != "reviewer" || got.Preset != "careful" || got.ModelProvider != "codex" || got.Model != "gpt-test" || got.ReasoningEffort != "high" {
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
			outcome, err := workflowruntime.Run(context.Background(), workflowruntime.Request{
				Source: source, SourceRef: "inline", SessionID: "unsupported-child-field",
				Policy: workflowpolicy.DefaultEffectivePolicy(),
			}, workflowruntime.Hooks{NewChildExecutor: func(string, workflowruntime.ChildRecordSink, workflowpolicy.EffectivePolicy) workflowruntime.ChildExecutor {
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
