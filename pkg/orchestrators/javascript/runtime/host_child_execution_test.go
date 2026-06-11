package workflowruntime_test

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"testing"

	"github.com/portpowered/infinite-you/pkg/orchestrators/javascript/policy"
	"github.com/portpowered/infinite-you/pkg/orchestrators/javascript/runtime"
)

const stubChildExecutionMode = "stub-dispatch"

type stubChildExecutor struct {
	mu     sync.Mutex
	labels []string
	mode   string
}

func (s *stubChildExecutor) Execute(req workflowruntime.ChildExecutionRequest) (workflowruntime.ChildExecutionResult, error) {
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
		NewChildExecutor: func(_ string, _ func(workflowruntime.RuntimeRecord)) workflowruntime.ChildExecutor {
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
