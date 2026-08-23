package workflowruntime_test

import (
	"context"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	workflowpolicy "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/orchestratorcontract"
)

// Parallel child execution assertions live in this existing policy test file so
// the runtime package stays within both the per-file and package file-count
// maintainability limits.
func assertParallelResultOrder(t *testing.T, sessionID string, outcome factory.JavaScriptRuntimeOutcome, wantLabels []string) {
	t.Helper()
	results, ok := projectPrimaryJSON(t, sessionID, outcome.Value)["results"].([]any)
	if !ok || len(results) != len(wantLabels) {
		t.Fatalf("projected results = %#v, want %d entries", results, len(wantLabels))
	}
	for i, wantLabel := range wantLabels {
		child, ok := results[i].(map[string]any)
		if !ok || child["label"] != wantLabel || child["status"] != factory.JavaScriptChildDispatchStatusCompleted {
			t.Fatalf("results[%d] = %#v, want completed child %q", i, results[i], wantLabel)
		}
	}
}

func peakConcurrentChildRunning(records []factory.JavaScriptRuntimeRecord) int {
	running, peak := 0, 0
	for _, record := range records {
		if record.Kind != factory.JavaScriptRecordKindChildDispatch || record.ChildDispatch == nil {
			continue
		}
		switch record.ChildDispatch.Status {
		case factory.JavaScriptChildDispatchStatusRunning:
			running++
			if running > peak {
				peak = running
			}
		case factory.JavaScriptChildDispatchStatusCompleted, factory.JavaScriptChildDispatchStatusFailed:
			running--
		}
	}
	return peak
}

func hasChildDispatchStatus(records []factory.JavaScriptRuntimeRecord, status string) bool {
	for _, record := range records {
		if record.Kind == factory.JavaScriptRecordKindChildDispatch && record.ChildDispatch != nil && record.ChildDispatch.Status == status {
			return true
		}
	}
	return false
}

func completedForLabel(records []factory.JavaScriptRuntimeRecord, label string) bool {
	for _, record := range records {
		if record.Kind == factory.JavaScriptRecordKindChildDispatch && record.ChildDispatch != nil && record.ChildDispatch.Label == label && record.ChildDispatch.Status == factory.JavaScriptChildDispatchStatusCompleted {
			return true
		}
	}
	return false
}

func TestRun_PipelineLegacyOneAndTwoStageShapesRemainCompatible(t *testing.T) {
	source := `return (async function () {
  let callbackCalls = 0;
  const oneStage = await pipeline(
    ["alpha", "beta"],
    async function (item, index, unexpected) {
      callbackCalls = callbackCalls + 1;
      return {
        stage: "one",
        item: item,
        index: index,
        noUnexpectedArgument: unexpected === undefined,
      };
    }
  );
  const twoStage = await pipeline(
    ["gamma", "delta"],
    function (item, index, unexpected) {
      callbackCalls = callbackCalls + 1;
      return {
        stage: "one",
        item: item,
        index: index,
        noUnexpectedArgument: unexpected === undefined,
      };
    },
    async function (previous, item, index, unexpected) {
      callbackCalls = callbackCalls + 1;
      return {
        stage: "two",
        previousStage: previous.stage,
        previousItem: previous.item,
        item: item,
        index: index,
        noUnexpectedArgument: unexpected === undefined,
      };
    }
  );
  const explicitUndefinedNext = await pipeline(
    ["legacy"],
    function (item, index, unexpected) {
      callbackCalls = callbackCalls + 1;
      return {
        stage: "one",
        item: item,
        index: index,
        noUnexpectedArgument: unexpected === undefined,
      };
    },
    undefined
  );
  const empty = await pipeline([], function () {
    callbackCalls = callbackCalls + 1;
    return { shouldNotRun: true };
  });
  return {
    callbackCalls: callbackCalls,
    oneStage: oneStage,
    twoStage: twoStage,
    explicitUndefinedNext: explicitUndefinedNext,
    empty: empty,
  };
})();`
	outcome := runInlineWorkflow(t, "pipeline-legacy-shapes", source)
	projected := projectPrimaryJSON(t, "session-pipeline-legacy-shapes", outcome.Value)

	if projected["callbackCalls"] != float64(7) {
		t.Fatalf("callbackCalls = %#v, want 7 callbacks across one-/two-stage calls and explicit undefined next", projected["callbackCalls"])
	}
	assertLegacyOneStageResults(t, projected["oneStage"], []string{"alpha", "beta"})
	assertLegacyTwoStageResults(t, projected["twoStage"], []string{"gamma", "delta"})
	assertLegacyOneStageResults(t, projected["explicitUndefinedNext"], []string{"legacy"})
	empty, ok := projected["empty"].([]any)
	if !ok || len(empty) != 0 {
		t.Fatalf("empty pipeline result = %#v, want empty ordered result", projected["empty"])
	}
	if hasChildDispatchStatus(outcome.Records, factory.JavaScriptChildDispatchStatusQueued) {
		t.Fatalf("records = %#v, pure compatibility callbacks should not create child dispatches", outcome.Records)
	}
}

func assertLegacyOneStageResults(t *testing.T, raw any, wantItems []string) {
	t.Helper()
	results, ok := raw.([]any)
	if !ok || len(results) != len(wantItems) {
		t.Fatalf("one-stage results = %#v, want %d ordered items", raw, len(wantItems))
	}
	for index, wantItem := range wantItems {
		itemResult, ok := results[index].(map[string]any)
		if !ok || itemResult["index"] != float64(index) || itemResult["item"] != wantItem || itemResult["status"] != factory.JavaScriptChildDispatchStatusCompleted {
			t.Fatalf("one-stage results[%d] = %#v, want ordered completed item %q", index, results[index], wantItem)
		}
		stages, ok := itemResult["stages"].([]any)
		if !ok || len(stages) != 1 {
			t.Fatalf("one-stage results[%d].stages = %#v, want one stage", index, itemResult["stages"])
		}
		stage, ok := stages[0].(map[string]any)
		if !ok || stage["index"] != float64(0) || stage["status"] != factory.JavaScriptChildDispatchStatusCompleted {
			t.Fatalf("one-stage results[%d].stages[0] = %#v, want indexed completed stage", index, stages[0])
		}
		result, ok := stage["result"].(map[string]any)
		if !ok || result["stage"] != "one" || result["item"] != wantItem || result["index"] != float64(index) || result["noUnexpectedArgument"] != true {
			t.Fatalf("one-stage results[%d].stages[0].result = %#v, want legacy two-argument callback result", index, stage["result"])
		}
	}
}

func assertLegacyTwoStageResults(t *testing.T, raw any, wantItems []string) {
	t.Helper()
	results, ok := raw.([]any)
	if !ok || len(results) != len(wantItems) {
		t.Fatalf("two-stage results = %#v, want %d ordered items", raw, len(wantItems))
	}
	for index, wantItem := range wantItems {
		itemResult, ok := results[index].(map[string]any)
		if !ok || itemResult["index"] != float64(index) || itemResult["item"] != wantItem || itemResult["status"] != factory.JavaScriptChildDispatchStatusCompleted {
			t.Fatalf("two-stage results[%d] = %#v, want ordered completed item %q", index, results[index], wantItem)
		}
		stages, ok := itemResult["stages"].([]any)
		if !ok || len(stages) != 2 {
			t.Fatalf("two-stage results[%d].stages = %#v, want two stages", index, itemResult["stages"])
		}
		for stageIndex, rawStage := range stages {
			stage, ok := rawStage.(map[string]any)
			if !ok || stage["index"] != float64(stageIndex) || stage["status"] != factory.JavaScriptChildDispatchStatusCompleted {
				t.Fatalf("two-stage results[%d].stages[%d] = %#v, want indexed completed stage", index, stageIndex, rawStage)
			}
		}
		firstResult, ok := stages[0].(map[string]any)
		if !ok {
			t.Fatalf("two-stage results[%d].stages[0] = %#v, want stage result", index, stages[0])
		}
		firstValue, ok := firstResult["result"].(map[string]any)
		if !ok || firstValue["noUnexpectedArgument"] != true {
			t.Fatalf("two-stage results[%d].stages[0].result = %#v, want two first-stage arguments", index, firstResult["result"])
		}
		secondResult, ok := stages[1].(map[string]any)
		if !ok {
			t.Fatalf("two-stage results[%d].stages[1] = %#v, want stage result", index, stages[1])
		}
		secondValue, ok := secondResult["result"].(map[string]any)
		if !ok || secondValue["stage"] != "two" || secondValue["previousStage"] != "one" || secondValue["previousItem"] != wantItem || secondValue["item"] != wantItem || secondValue["index"] != float64(index) || secondValue["noUnexpectedArgument"] != true {
			t.Fatalf("two-stage results[%d].stages[1].result = %#v, want prior result and legacy three-argument callback result", index, secondResult["result"])
		}
	}
}

func TestRun_PipelineThreeStages_PassesPreviousResultOriginalItemAndIndex(t *testing.T) {
	source := `return (async function () {
  const results = await pipeline(
    ["alpha", "beta"],
    async function (item, index) {
      return { stage: "one", item: item, index: index };
    },
    async function (previous, item, index) {
      return {
        stage: "two",
        previousStage: previous.stage,
        previousItem: previous.item,
        item: item,
        index: index,
      };
    },
    async function (previous, item, index) {
      return {
        stage: "three",
        previousStage: previous.stage,
        previousItem: previous.previousItem,
        item: item,
        index: index,
      };
    }
  );
  return { results: results };
})();`
	outcome := runInlineWorkflow(t, "pipeline-three-stage-callback-contract", source)
	projected := projectPrimaryJSON(t, "session-pipeline-three-stage-callback-contract", outcome.Value)
	results, ok := projected["results"].([]any)
	if !ok || len(results) != 2 {
		t.Fatalf("projected results = %#v, want two ordered items", projected["results"])
	}

	for index, wantItem := range []string{"alpha", "beta"} {
		itemResult, ok := results[index].(map[string]any)
		if !ok {
			t.Fatalf("results[%d] = %#v, want pipeline item result", index, results[index])
		}
		if itemResult["index"] != float64(index) || itemResult["item"] != wantItem || itemResult["status"] != factory.JavaScriptChildDispatchStatusCompleted {
			t.Fatalf("results[%d] identity/status = %#v, want index=%d item=%q completed", index, itemResult, index, wantItem)
		}
		stages, ok := itemResult["stages"].([]any)
		if !ok || len(stages) != 3 {
			t.Fatalf("results[%d].stages = %#v, want three ordered stages", index, itemResult["stages"])
		}
		for stageIndex, entry := range stages {
			stage, ok := entry.(map[string]any)
			if !ok || stage["index"] != float64(stageIndex) || stage["status"] != factory.JavaScriptChildDispatchStatusCompleted {
				t.Fatalf("results[%d].stages[%d] = %#v, want indexed completed stage", index, stageIndex, entry)
			}
		}

		first, ok := stages[0].(map[string]any)
		if !ok {
			t.Fatalf("results[%d].stages[0] = %#v, want object", index, stages[0])
		}
		firstResult := first["result"].(map[string]any)
		if firstResult["stage"] != "one" || firstResult["item"] != wantItem || firstResult["index"] != float64(index) {
			t.Fatalf("results[%d].stages[0].result = %#v, want stage-one callback arguments", index, firstResult)
		}

		second := stages[1].(map[string]any)
		secondResult := second["result"].(map[string]any)
		if secondResult["stage"] != "two" || secondResult["previousStage"] != "one" || secondResult["previousItem"] != wantItem || secondResult["item"] != wantItem || secondResult["index"] != float64(index) {
			t.Fatalf("results[%d].stages[1].result = %#v, want prior result and original callback arguments", index, secondResult)
		}

		third := stages[2].(map[string]any)
		thirdResult := third["result"].(map[string]any)
		if thirdResult["stage"] != "three" || thirdResult["previousStage"] != "two" || thirdResult["previousItem"] != wantItem || thirdResult["item"] != wantItem || thirdResult["index"] != float64(index) {
			t.Fatalf("results[%d].stages[2].result = %#v, want prior result and original callback arguments", index, thirdResult)
		}
	}
}

func TestRun_PipelineThreeStages_AdvancesChainsWithoutStageBarrier(t *testing.T) {
	labels := []string{
		"stage1-0", "stage1-1",
		"stage2-0", "stage2-1",
		"stage3-0", "stage3-1",
	}
	children := newTimedPipelineChildExecutor(labels)
	t.Cleanup(children.releaseAll)

	source := `return (async function () {
  const results = await pipeline(
    ["fast-chain", "slow-chain"],
    function (item, index) {
      return agent.run({ prompt: "stage one " + item, label: "stage1-" + index });
    },
    function (previous, item, index) {
      return agent.run({ prompt: "stage two " + previous.label, label: "stage2-" + index });
    },
    function (previous, item, index) {
      return agent.run({ prompt: "stage three " + previous.label, label: "stage3-" + index });
    }
  );
  return { results: results };
})();`
	req := factory.JavaScriptRuntimeRequest{
		Source:    source,
		SourceRef: "pipeline-three-stage-overlap.workflow.js",
		SessionID: "session-pipeline-three-stage-overlap",
		Policy:    workflowpolicy.DefaultEffectivePolicy(),
	}

	type runResult struct {
		outcome factory.JavaScriptRuntimeOutcome
		err     error
	}
	runDone := make(chan runResult, 1)
	go func() {
		outcome, err := runtimeWorkflows.Run(context.Background(), req, factory.JavaScriptRuntimeHooks{
			NewChildExecutor: func(_ string, sink factory.JavaScriptChildRecordSink, _ workflowpolicy.EffectivePolicy) factory.JavaScriptChildExecutor {
				children.sink = sink
				return children
			},
		})
		runDone <- runResult{outcome: outcome, err: err}
	}()

	children.waitForStarted(t, "stage1-0", "stage1-1")
	children.releaseLabel("stage1-0")
	children.waitForStarted(t, "stage2-0")
	children.releaseLabel("stage2-0")
	children.waitForStarted(t, "stage3-0")
	if children.hasEnded("stage1-1") {
		t.Fatal("stage1-1 completed before stage3-0 started; item chains were separated by a barrier")
	}
	children.releaseLabel("stage3-0")
	children.releaseLabel("stage1-1")
	children.waitForStarted(t, "stage2-1")
	children.releaseLabel("stage2-1")
	children.waitForStarted(t, "stage3-1")
	children.releaseLabel("stage3-1")

	completed := <-runDone
	if completed.err != nil {
		t.Fatalf("Run() error = %v", completed.err)
	}
	if !completed.outcome.OK {
		t.Fatalf("Run() failure = %#v", completed.outcome.Failure)
	}

	projected := projectPrimaryJSON(t, req.SessionID, completed.outcome.Value)
	results, ok := projected["results"].([]any)
	if !ok || len(results) != 2 {
		t.Fatalf("projected results = %#v, want two ordered items", projected["results"])
	}
	for itemIndex, result := range results {
		item, ok := result.(map[string]any)
		if !ok || item["index"] != float64(itemIndex) || item["status"] != factory.JavaScriptChildDispatchStatusCompleted {
			t.Fatalf("results[%d] = %#v, want ordered completed item", itemIndex, result)
		}
		stages, ok := item["stages"].([]any)
		if !ok || len(stages) != 3 {
			t.Fatalf("results[%d].stages = %#v, want three stages", itemIndex, item["stages"])
		}
		for stageIndex, rawStage := range stages {
			stage, ok := rawStage.(map[string]any)
			if !ok || stage["index"] != float64(stageIndex) || stage["status"] != factory.JavaScriptChildDispatchStatusCompleted {
				t.Fatalf("results[%d].stages[%d] = %#v, want ordered completed stage", itemIndex, stageIndex, rawStage)
			}
		}
	}

	times := children.snapshot()
	if times["stage3-0"].start >= times["stage1-1"].end {
		t.Fatalf("stage3-0 start = %s, stage1-1 end = %s; want cross-stage overlap", times["stage3-0"].start, times["stage1-1"].end)
	}

	stageMaxima := make(map[int]time.Duration)
	chainDurations := make(map[int]time.Duration)
	var firstDispatchStart, lastDispatchEnd time.Duration
	firstDispatchSeen := false
	for label, timing := range times {
		parts := strings.Split(label, "-")
		stageIndex, err := strconv.Atoi(strings.TrimPrefix(parts[0], "stage"))
		if err != nil {
			t.Fatalf("parse stage label %q: %v", label, err)
		}
		itemIndex, err := strconv.Atoi(parts[1])
		if err != nil {
			t.Fatalf("parse item label %q: %v", label, err)
		}
		duration := timing.end - timing.start
		if !firstDispatchSeen || timing.start < firstDispatchStart {
			firstDispatchStart = timing.start
			firstDispatchSeen = true
		}
		if timing.end > lastDispatchEnd {
			lastDispatchEnd = timing.end
		}
		if duration > stageMaxima[stageIndex] {
			stageMaxima[stageIndex] = duration
		}
		chainDurations[itemIndex] += duration
	}
	barrierDuration := time.Duration(0)
	for _, duration := range stageMaxima {
		barrierDuration += duration
	}
	slowestChain := time.Duration(0)
	for _, duration := range chainDurations {
		if duration > slowestChain {
			slowestChain = duration
		}
	}
	observedDuration := lastDispatchEnd - firstDispatchStart
	t.Logf("pipeline timing evidence: observed=%s slowest-chain=%s barrier-sum-of-stage-maxima=%s", observedDuration, slowestChain, barrierDuration)
	if observedDuration >= barrierDuration {
		t.Fatalf("observed pipeline duration = %s, want less than barrier comparison = %s", observedDuration, barrierDuration)
	}
	const timingTolerance = 100 * time.Millisecond
	if observedDuration > slowestChain+timingTolerance {
		t.Fatalf("observed pipeline duration = %s, slowest chain = %s, tolerance = %s", observedDuration, slowestChain, timingTolerance)
	}
}

type timedPipelineChildExecutor struct {
	sink       factory.JavaScriptChildRecordSink
	started    chan string
	mu         sync.Mutex
	release    map[string]chan struct{}
	released   map[string]bool
	timestamps map[string]pipelineChildTiming
	planned    map[string]time.Duration
}

type pipelineChildTiming struct {
	start     time.Duration
	end       time.Duration
	completed bool
}

func newTimedPipelineChildExecutor(labels []string) *timedPipelineChildExecutor {
	release := make(map[string]chan struct{}, len(labels))
	for _, label := range labels {
		release[label] = make(chan struct{})
	}
	return &timedPipelineChildExecutor{
		started:    make(chan string, len(labels)),
		release:    release,
		released:   make(map[string]bool, len(labels)),
		timestamps: make(map[string]pipelineChildTiming, len(labels)),
		planned: map[string]time.Duration{
			"stage1-0": 80 * time.Millisecond,
			"stage1-1": 100 * time.Millisecond,
			"stage2-0": 10 * time.Millisecond,
			"stage2-1": 80 * time.Millisecond,
			"stage3-0": 80 * time.Millisecond,
			"stage3-1": 10 * time.Millisecond,
		},
	}
}

func (e *timedPipelineChildExecutor) Execute(ctx context.Context, req factory.JavaScriptChildExecutionRequest) (factory.JavaScriptChildExecutionResult, error) {
	dispatchID, childIndex := e.sink.NextChildDispatchIdentity()
	base := factory.JavaScriptChildDispatchRecord{
		DispatchID:    dispatchID,
		ChildIndex:    childIndex,
		Label:         req.Label,
		ExecutionMode: factory.JavaScriptChildExecutionModeFake,
	}
	e.sink.AppendChildDispatch(base, factory.JavaScriptChildDispatchStatusQueued)
	e.sink.AppendChildDispatch(base, factory.JavaScriptChildDispatchStatusRunning)
	parts := strings.Split(req.Label, "-")
	stageIndex, _ := strconv.Atoi(strings.TrimPrefix(parts[0], "stage"))
	itemIndex, _ := strconv.Atoi(parts[1])
	e.mu.Lock()
	start := time.Duration(0)
	if stageIndex > 1 {
		start = e.timestamps[fmt.Sprintf("stage%d-%d", stageIndex-1, itemIndex)].end
	}
	e.timestamps[req.Label] = pipelineChildTiming{
		start: start,
		end:   start + e.planned[req.Label],
	}
	e.mu.Unlock()
	e.started <- req.Label

	select {
	case <-e.release[req.Label]:
	case <-ctx.Done():
		return factory.JavaScriptChildExecutionResult{}, ctx.Err()
	}

	e.mu.Lock()
	timing := e.timestamps[req.Label]
	timing.completed = true
	e.timestamps[req.Label] = timing
	e.mu.Unlock()
	completed := base
	completed.Status = factory.JavaScriptChildDispatchStatusCompleted
	completed.Output = map[string]any{"label": req.Label}
	e.sink.Append(factory.JavaScriptRuntimeRecord{Kind: factory.JavaScriptRecordKindChildDispatch, ChildDispatch: &completed})
	return factory.JavaScriptChildExecutionResult{
		DispatchID:    dispatchID,
		ChildIndex:    childIndex,
		Status:        factory.JavaScriptChildDispatchStatusCompleted,
		ExecutionMode: factory.JavaScriptChildExecutionModeFake,
		Output:        completed.Output,
		Request:       req,
	}, nil
}

func (e *timedPipelineChildExecutor) waitForStarted(t *testing.T, want ...string) {
	t.Helper()
	wanted := make(map[string]bool, len(want))
	for _, label := range want {
		wanted[label] = true
	}
	for len(wanted) > 0 {
		label := <-e.started
		if !wanted[label] {
			t.Fatalf("unexpected child start %q while waiting for %#v", label, wanted)
		}
		delete(wanted, label)
	}
}

func (e *timedPipelineChildExecutor) releaseLabel(label string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.released[label] {
		return
	}
	e.released[label] = true
	close(e.release[label])
}

func (e *timedPipelineChildExecutor) releaseAll() {
	for label := range e.release {
		e.releaseLabel(label)
	}
}

func (e *timedPipelineChildExecutor) hasEnded(label string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.timestamps[label].completed
}

func (e *timedPipelineChildExecutor) snapshot() map[string]pipelineChildTiming {
	e.mu.Lock()
	defer e.mu.Unlock()
	copyOf := make(map[string]pipelineChildTiming, len(e.timestamps))
	for label, timing := range e.timestamps {
		copyOf[label] = timing
	}
	return copyOf
}

func TestRun_PipelineStagedFakeChildren_PreservesItemStageOrder(t *testing.T) {
	source := readFixture(t, "pipeline-staged-fake-children.workflow.js")
	policy := workflowpolicy.DefaultEffectivePolicy()
	policy.MaxAgents = 16

	req := factory.JavaScriptRuntimeRequest{
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

	firstProjected := projectPrimaryJSON(t, "session-pipeline-staged-fake-children", first.Value)
	secondProjected := projectPrimaryJSON(t, "session-pipeline-staged-fake-children", second.Value)
	if !reflect.DeepEqual(normalizePipelineDispatchIdentity(firstProjected), normalizePipelineDispatchIdentity(secondProjected)) {
		t.Fatalf("pipeline value drift beyond dispatch identity: first=%s second=%s", first.Value.JSON, second.Value.JSON)
	}
}

func normalizePipelineDispatchIdentity(value any) any {
	switch typed := value.(type) {
	case []any:
		normalized := make([]any, len(typed))
		for index, entry := range typed {
			normalized[index] = normalizePipelineDispatchIdentity(entry)
		}
		return normalized
	case map[string]any:
		normalized := make(map[string]any, len(typed))
		for key, entry := range typed {
			switch key {
			case "artifactRef", "childIndex", "dispatchId", "providerSessionRef":
				continue
			}
			normalized[key] = normalizePipelineDispatchIdentity(entry)
		}
		return normalized
	default:
		return value
	}
}

func TestRun_PipelineStagedFakeChildren_RepresentsStageFailureExplicitly(t *testing.T) {
	source := readFixture(t, "pipeline-stage-failure.workflow.js")
	policy := workflowpolicy.DefaultEffectivePolicy()
	policy.MaxAgents = 16

	req := factory.JavaScriptRuntimeRequest{
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
	if !ok || successItem["status"] != factory.JavaScriptChildDispatchStatusCompleted {
		t.Fatalf("results[0] = %#v, want completed item", results[0])
	}
	failedItem, ok := results[1].(map[string]any)
	if !ok || failedItem["status"] != factory.JavaScriptChildDispatchStatusFailed {
		t.Fatalf("results[1].status = %#v, want FAILED", failedItem["status"])
	}
	stages, ok := failedItem["stages"].([]any)
	if !ok || len(stages) != 2 {
		t.Fatalf("results[1].stages = %#v, want 2 stages", failedItem["stages"])
	}
	editStage, ok := stages[0].(map[string]any)
	if !ok || editStage["status"] != factory.JavaScriptChildDispatchStatusCompleted {
		t.Fatalf("results[1].stages[0] = %#v, want completed edit stage", stages[0])
	}
	reviewStage, ok := stages[1].(map[string]any)
	if !ok || reviewStage["status"] != factory.JavaScriptChildDispatchStatusFailed {
		t.Fatalf("results[1].stages[1] = %#v, want failed review stage", stages[1])
	}
	if reviewStage["diagnostic"] == "" {
		t.Fatal("results[1].stages[1].diagnostic is empty")
	}
}

func TestRun_PipelineStageFailuresIsolateEachItem(t *testing.T) {
	source := `return (async function () {
  const results = await pipeline(
    ["sync-fail", "promise-fail", "healthy"],
    function (item, index) {
      return { stage: 0, item: item, index: index };
    },
    function (previous, item, index) {
      if (item === "sync-fail") {
        throw new Error("sync stage failure");
      }
      return { stage: 1, previousStage: previous.stage, item: item, index: index };
    },
    async function (previous, item, index) {
      if (item === "promise-fail") {
        throw new Error("promise stage failure");
      }
      return { stage: 2, previousStage: previous.stage, item: item, index: index };
    }
  );
  return { results: results };
})();`

	outcome := runInlineWorkflow(t, "pipeline-stage-failure-isolation", source)
	projected := projectPrimaryJSON(t, "session-pipeline-stage-failure-isolation", outcome.Value)
	results, ok := projected["results"].([]any)
	if !ok || len(results) != 3 {
		t.Fatalf("projected results = %#v, want three ordered items", projected["results"])
	}

	assertPipelineFailedItem(t, results[0], "sync-fail", 0, 2, "sync stage failure")
	assertPipelineFailedItem(t, results[1], "promise-fail", 1, 3, "promise stage failure")
	assertPipelineCompletedItem(t, results[2], "healthy", 2, 3)
}

func TestRun_PipelineFailedChildResultStopsOnlyThatItem(t *testing.T) {
	source := `return (async function () {
  const results = await pipeline(
    ["bad", "good"],
    function (item, index) {
      return agent.run({ label: "stage1-" + item, prompt: "stage1-" + item });
    },
    function (previous, item, index) {
      return agent.run({ label: "stage2-" + item, prompt: "stage2-" + item });
    },
    function (previous, item, index) {
      return agent.run({ label: "stage3-" + item, prompt: "stage3-" + item });
    }
  );
  return { results: results };
})();`

	var (
		mu     sync.Mutex
		labels []string
	)
	outcome, err := runtimeWorkflows.Run(context.Background(), factory.JavaScriptRuntimeRequest{
		Source:    source,
		SourceRef: "pipeline-failed-child-result.workflow.js",
		SessionID: "session-pipeline-failed-child-result",
		Policy:    workflowpolicy.DefaultEffectivePolicy(),
	}, factory.JavaScriptRuntimeHooks{
		NewChildExecutor: func(_ string, _ factory.JavaScriptChildRecordSink, _ workflowpolicy.EffectivePolicy) factory.JavaScriptChildExecutor {
			return childExecutorFunc(func(_ context.Context, request factory.JavaScriptChildExecutionRequest) (factory.JavaScriptChildExecutionResult, error) {
				mu.Lock()
				labels = append(labels, request.Label)
				childIndex := len(labels)
				mu.Unlock()
				result := factory.JavaScriptChildExecutionResult{
					DispatchID:    fmt.Sprintf("pipeline-dispatch-%d", childIndex),
					ChildIndex:    childIndex,
					Status:        factory.JavaScriptChildDispatchStatusCompleted,
					ExecutionMode: "pipeline-test",
					Output:        map[string]any{"label": request.Label},
					Request:       request,
				}
				if request.Label == "stage2-bad" {
					result.Status = factory.JavaScriptChildDispatchStatusFailed
					result.Diagnostic = "stage 2 child rejected"
				}
				return result, nil
			})
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !outcome.OK {
		t.Fatalf("Run() failure = %#v", outcome.Failure)
	}

	projected := projectPrimaryJSON(t, "session-pipeline-failed-child-result", outcome.Value)
	results, ok := projected["results"].([]any)
	if !ok || len(results) != 2 {
		t.Fatalf("projected results = %#v, want two ordered items", projected["results"])
	}
	assertPipelineFailedItem(t, results[0], "bad", 0, 2, "stage 2 child rejected")
	assertPipelineCompletedItem(t, results[1], "good", 1, 3)

	mu.Lock()
	gotLabels := append([]string(nil), labels...)
	mu.Unlock()
	seen := make(map[string]bool, len(gotLabels))
	for _, label := range gotLabels {
		seen[label] = true
	}
	for _, wantLabel := range []string{"stage1-bad", "stage1-good", "stage2-bad", "stage2-good", "stage3-good"} {
		if !seen[wantLabel] {
			t.Fatalf("child labels = %#v, want %q to be dispatched", gotLabels, wantLabel)
		}
	}
	if seen["stage3-bad"] {
		t.Fatalf("child labels = %#v, failed item's later stage was dispatched", gotLabels)
	}
}

func assertPipelineFailedItem(t *testing.T, raw any, wantItem string, wantIndex, wantStageCount int, wantDiagnostic string) {
	t.Helper()
	item, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("pipeline item = %#v, want object", raw)
	}
	if item["index"] != float64(wantIndex) || item["item"] != wantItem || item["status"] != factory.JavaScriptChildDispatchStatusFailed {
		t.Fatalf("pipeline item = %#v, want index=%d item=%q FAILED", item, wantIndex, wantItem)
	}
	stages, ok := item["stages"].([]any)
	if !ok || len(stages) != wantStageCount {
		t.Fatalf("pipeline item stages = %#v, want %d entries", item["stages"], wantStageCount)
	}
	for stageIndex, rawStage := range stages[:wantStageCount-1] {
		stage, ok := rawStage.(map[string]any)
		if !ok || stage["index"] != float64(stageIndex) || stage["status"] != factory.JavaScriptChildDispatchStatusCompleted {
			t.Fatalf("pipeline item stages[%d] = %#v, want completed stage", stageIndex, rawStage)
		}
	}
	failedStage, ok := stages[wantStageCount-1].(map[string]any)
	if !ok || failedStage["index"] != float64(wantStageCount-1) || failedStage["status"] != factory.JavaScriptChildDispatchStatusFailed {
		t.Fatalf("pipeline failed stage = %#v, want failed stage index %d", stages[wantStageCount-1], wantStageCount-1)
	}
	if !strings.Contains(failedStage["diagnostic"].(string), wantDiagnostic) {
		t.Fatalf("pipeline failed stage diagnostic = %#v, want %q", failedStage["diagnostic"], wantDiagnostic)
	}
	if _, exists := failedStage["result"]; exists {
		t.Fatalf("pipeline failed stage = %#v, want no result after failure", failedStage)
	}
}

func assertPipelineCompletedItem(t *testing.T, raw any, wantItem string, wantIndex, wantStageCount int) {
	t.Helper()
	item, ok := raw.(map[string]any)
	if !ok || item["index"] != float64(wantIndex) || item["item"] != wantItem || item["status"] != factory.JavaScriptChildDispatchStatusCompleted {
		t.Fatalf("pipeline item = %#v, want index=%d item=%q COMPLETED", raw, wantIndex, wantItem)
	}
	stages, ok := item["stages"].([]any)
	if !ok || len(stages) != wantStageCount {
		t.Fatalf("pipeline completed stages = %#v, want %d entries", item["stages"], wantStageCount)
	}
	for stageIndex, rawStage := range stages {
		stage, ok := rawStage.(map[string]any)
		if !ok || stage["index"] != float64(stageIndex) || stage["status"] != factory.JavaScriptChildDispatchStatusCompleted {
			t.Fatalf("pipeline completed stages[%d] = %#v, want completed stage", stageIndex, rawStage)
		}
	}
}

func assertPipelineItemOrder(t *testing.T, outcome factory.JavaScriptRuntimeOutcome, wantItems []string) {
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
		if itemResult["status"] != factory.JavaScriptChildDispatchStatusCompleted {
			t.Fatalf("results[%d].status = %#v, want COMPLETED", i, itemResult["status"])
		}
	}
}

func assertPipelineStageTransitions(t *testing.T, outcome factory.JavaScriptRuntimeOutcome, wantItems, wantStages int) {
	t.Helper()
	childRecords := childDispatchRecords(outcome.Records)
	wantChildCount := wantItems * wantStages
	if len(childRecords) != wantChildCount*3 {
		t.Fatalf("child_dispatch record count = %d, want %d transitions", len(childRecords), wantChildCount*3)
	}

	completedLabels := make(map[string]int, wantChildCount)
	completedDispatches := make(map[string]string, wantChildCount)
	for _, label := range completedChildLabels(childRecords) {
		completedLabels[label]++
	}
	for _, record := range childRecords {
		if record.ChildDispatch == nil || record.ChildDispatch.Status != factory.JavaScriptChildDispatchStatusCompleted {
			continue
		}
		completedDispatches[record.ChildDispatch.DispatchID] = record.ChildDispatch.Label
	}
	if len(completedLabels) != wantChildCount {
		t.Fatalf("completed child labels = %#v, want one completion for each of %d stages", completedLabels, wantChildCount)
	}
	for i := 0; i < wantItems; i++ {
		for _, wantLabel := range []string{"edit-" + itoa(i), "review-" + itoa(i)} {
			if completedLabels[wantLabel] != 1 {
				t.Fatalf("completed child label %q count = %d, want exactly one", wantLabel, completedLabels[wantLabel])
			}
		}
	}

	projected := projectPrimaryJSON(t, "session-pipeline-staged-fake-children", outcome.Value)
	results, ok := projected["results"].([]any)
	if !ok || len(results) != wantItems {
		t.Fatalf("projected results = %#v, want %d ordered items", projected["results"], wantItems)
	}
	for itemIndex, rawItem := range results {
		item, ok := rawItem.(map[string]any)
		if !ok {
			t.Fatalf("results[%d] = %#v, want item result", itemIndex, rawItem)
		}
		stages, ok := item["stages"].([]any)
		if !ok || len(stages) != wantStages {
			t.Fatalf("results[%d].stages = %#v, want %d ordered stages", itemIndex, item["stages"], wantStages)
		}
		for stageIndex, rawStage := range stages {
			stage, ok := rawStage.(map[string]any)
			if !ok || stage["index"] != float64(stageIndex) || stage["status"] != factory.JavaScriptChildDispatchStatusCompleted {
				t.Fatalf("results[%d].stages[%d] = %#v, want completed stage in item order", itemIndex, stageIndex, rawStage)
			}
			stageResult, ok := stage["result"].(map[string]any)
			if !ok {
				t.Fatalf("results[%d].stages[%d].result = %#v, want child result", itemIndex, stageIndex, stage["result"])
			}
			dispatchID, _ := stageResult["dispatchId"].(string)
			if completedDispatches[dispatchID] != stageResult["label"] {
				t.Fatalf("results[%d].stages[%d] dispatch association = %#v, want completed record label %q", itemIndex, stageIndex, stageResult, completedDispatches[dispatchID])
			}
		}
	}
}

func assertPipelineReviewUsesEditOutput(t *testing.T, outcome factory.JavaScriptRuntimeOutcome) {
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
	if !ok || editStage["status"] != factory.JavaScriptChildDispatchStatusCompleted {
		t.Fatalf("results[0].stages[0] = %#v, want completed edit stage", stages[0])
	}
	reviewStage, ok := stages[1].(map[string]any)
	if !ok || reviewStage["status"] != factory.JavaScriptChildDispatchStatusCompleted {
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

func childDispatchRecords(records []factory.JavaScriptRuntimeRecord) []factory.JavaScriptRuntimeRecord {
	var childRecords []factory.JavaScriptRuntimeRecord
	for _, record := range records {
		if record.Kind == factory.JavaScriptRecordKindChildDispatch {
			childRecords = append(childRecords, record)
		}
	}
	return childRecords
}

func completedChildLabels(records []factory.JavaScriptRuntimeRecord) []string {
	var labels []string
	for _, record := range records {
		if record.ChildDispatch == nil {
			continue
		}
		if record.ChildDispatch.Status != factory.JavaScriptChildDispatchStatusCompleted {
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
			code:    factory.JavaScriptRuntimeCodePreExecutionInvalid,
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
			code:    factory.JavaScriptRuntimeCodePreExecutionInvalid,
		},
		{
			fixture: "agent-run-policy-denied-writable-roots.workflow.js",
			policy:  basePolicy,
			want:    `agent.run() does not support field "writableRoots"`,
			code:    factory.JavaScriptRuntimeCodePreExecutionInvalid,
		},
		{
			fixture: "agent-run-policy-denied-network.workflow.js",
			policy:  basePolicy,
			want:    `agent.run() does not support field "network"`,
			code:    factory.JavaScriptRuntimeCodePreExecutionInvalid,
		},
		{
			fixture: "agent-run-policy-denied-concurrency.workflow.js",
			policy:  basePolicy,
			want:    `agent.run() does not support field "concurrency"`,
			code:    factory.JavaScriptRuntimeCodePreExecutionInvalid,
		},
	}

	for _, tc := range cases {
		t.Run(tc.fixture, func(t *testing.T) {
			req := policyDeniedRequest(t, tc.fixture, tc.policy)
			outcome := runExecutionFailure(t, req)
			wantCode := tc.code
			if wantCode == "" {
				wantCode = factory.JavaScriptRuntimeCodeScriptError
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

func TestRun_SkipPermissionsPermissionIsChildScopedAndDoesNotBypassRoutingPolicy(t *testing.T) {
	policy := workflowpolicy.DefaultEffectivePolicy()
	policy.MaxAgents = 4
	policy.AllowedModels = []string{"gpt-allowed"}

	var requests []factory.JavaScriptChildExecutionRequest
	outcome, err := runtimeWorkflows.Run(t.Context(), factory.JavaScriptRuntimeRequest{
		Source: `const autonomous = agent.run({
    prompt: "autonomous review",
    label: "autonomous-child",
    model: "gpt-allowed",
    permissions: "SKIP_PERMISSIONS",
  });
agent.run({
    prompt: "ordinary review",
    label: "ordinary-child",
    model: "gpt-denied",
  });
return { autonomous };`,
		SourceRef: "skip-permissions-policy-scope.workflow.js",
		SessionID: "session-skip-permissions-policy-scope",
		Policy:    policy,
	}, factory.JavaScriptRuntimeHooks{
		NewChildExecutor: func(string, factory.JavaScriptChildRecordSink, workflowpolicy.EffectivePolicy) factory.JavaScriptChildExecutor {
			return childExecutorFunc(func(_ context.Context, req factory.JavaScriptChildExecutionRequest) (factory.JavaScriptChildExecutionResult, error) {
				requests = append(requests, req)
				return factory.JavaScriptChildExecutionResult{
					Status:  factory.JavaScriptChildDispatchStatusCompleted,
					Request: req,
				}, nil
			})
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if outcome.OK || !strings.Contains(outcome.Failure.Message, `policy denied: model "gpt-denied" is not listed in allowedModels`) {
		t.Fatalf("Run() outcome = %#v, want ordinary sibling policy denial", outcome)
	}
	if len(requests) != 1 || !requests[0].SkipPermissions || requests[0].Label != "autonomous-child" {
		t.Fatalf("provider requests = %#v, want only the first child with SKIP_PERMISSIONS", requests)
	}
}

func TestRun_AllowedPermissionsRejectsSkipPermissionsBeforeDispatch(t *testing.T) {
	policy := workflowpolicy.DefaultEffectivePolicy()
	policy.AllowedPermissions = []string{workflowpolicy.PermissionModeDefault}

	called := false
	outcome, err := runtimeWorkflows.Run(t.Context(), factory.JavaScriptRuntimeRequest{
		Source: `return (async () => {
  await agent.run({ prompt: "autonomous review", label: "blocked-child", permissions: "SKIP_PERMISSIONS" });
  return { ok: true };
})();`,
		SourceRef:   "allowed-permissions.workflow.js",
		SessionID:   "session-allowed-permissions",
		FactoryName: "policy-factory",
		Policy:      policy,
	}, factory.JavaScriptRuntimeHooks{
		NewChildExecutor: func(string, factory.JavaScriptChildRecordSink, workflowpolicy.EffectivePolicy) factory.JavaScriptChildExecutor {
			return childExecutorFunc(func(_ context.Context, req factory.JavaScriptChildExecutionRequest) (factory.JavaScriptChildExecutionResult, error) {
				called = true
				return factory.JavaScriptChildExecutionResult{Status: factory.JavaScriptChildDispatchStatusCompleted, Request: req}, nil
			})
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	want := `policy denied: Factory "policy-factory" child "blocked-child" requested permission "SKIP_PERMISSIONS" not listed in allowedPermissions`
	if outcome.OK || !strings.Contains(outcome.Failure.Message, want) {
		t.Fatalf("Run() outcome = %#v, want pre-dispatch permission denial", outcome)
	}
	if called || len(outcome.Records) != 0 {
		t.Fatalf("executor called=%v records=%#v, want no dispatch side effects", called, outcome.Records)
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
		if record.Kind == factory.JavaScriptRecordKindArtifact {
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
	if outcome.Records[0].Kind != factory.JavaScriptRecordKindPhase {
		t.Fatalf("records[0].kind = %q, want phase", outcome.Records[0].Kind)
	}
	if outcome.Records[1].Kind != factory.JavaScriptRecordKindLog {
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
	if !ok || allowedChild["status"] != factory.JavaScriptChildDispatchStatusCompleted {
		t.Fatalf("results[0] = %#v, want completed child", results[0])
	}
	deniedChild, ok := results[1].(map[string]any)
	if !ok || deniedChild["status"] != factory.JavaScriptChildDispatchStatusFailed {
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

func policyDeniedRequest(t *testing.T, fixture string, policy workflowpolicy.EffectivePolicy) factory.JavaScriptRuntimeRequest {
	t.Helper()
	name := strings.TrimSuffix(fixture, ".workflow.js")
	return factory.JavaScriptRuntimeRequest{
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

func assertNoSuccessfulChildDispatchRecords(t *testing.T, records []factory.JavaScriptRuntimeRecord) {
	t.Helper()
	for _, record := range records {
		if record.Kind != factory.JavaScriptRecordKindChildDispatch || record.ChildDispatch == nil {
			continue
		}
		switch record.ChildDispatch.Status {
		case factory.JavaScriptChildDispatchStatusQueued,
			factory.JavaScriptChildDispatchStatusRunning,
			factory.JavaScriptChildDispatchStatusCompleted:
			t.Fatalf("records = %#v, want no successful child dispatch records after denial", records)
		}
	}
}

func assertNoSuccessfulChildDispatchRecordsForLabel(t *testing.T, records []factory.JavaScriptRuntimeRecord, label string) {
	t.Helper()
	for _, record := range records {
		if record.Kind != factory.JavaScriptRecordKindChildDispatch || record.ChildDispatch == nil {
			continue
		}
		if record.ChildDispatch.Label != label {
			continue
		}
		switch record.ChildDispatch.Status {
		case factory.JavaScriptChildDispatchStatusQueued,
			factory.JavaScriptChildDispatchStatusRunning,
			factory.JavaScriptChildDispatchStatusCompleted:
			t.Fatalf("records = %#v, want no successful child dispatch records for label %q", records, label)
		}
	}
}

func assertNoChildDispatchRecords(t *testing.T, records []factory.JavaScriptRuntimeRecord) {
	t.Helper()
	for _, record := range records {
		if record.Kind == factory.JavaScriptRecordKindChildDispatch {
			t.Fatalf("records = %#v, want no child dispatch records", records)
		}
	}
}

func completedChildRecords(records []factory.JavaScriptRuntimeRecord) int {
	count := 0
	for _, record := range records {
		if record.Kind == factory.JavaScriptRecordKindChildDispatch &&
			record.ChildDispatch != nil &&
			record.ChildDispatch.Status == factory.JavaScriptChildDispatchStatusCompleted {
			count++
		}
	}
	return count
}

func TestRun_ParallelFakeChildren_PreservesInputOrderAndConcurrency(t *testing.T) {
	source := readFixture(t, "parallel-fake-children.workflow.js")
	policy := workflowpolicy.DefaultEffectivePolicy()
	policy.MaxAgents = 8
	policy.Concurrency = 2

	req := factory.JavaScriptRuntimeRequest{
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

	if string(first.Value.JSON) != string(second.Value.JSON) {
		t.Fatalf("value drift across runs: first=%s second=%s", first.Value.JSON, second.Value.JSON)
	}
}

func TestRun_ParallelObjectChildren_ResolveWorkerSettings(t *testing.T) {
	var mu sync.Mutex
	captured := make(map[string]factory.JavaScriptChildExecutionRequest)
	req := factory.JavaScriptRuntimeRequest{
		Source: `return parallel([
			{label: "child-preset", preset: "child", prompt: "one"},
			{label: "factory-preset", preset: "factory", prompt: "two"},
			{label: "scalar-defaults", prompt: "three"}
		]);`,
		SessionID: "session-parallel-worker-settings",
		Agents: map[string]interfaces.FactoryOrchestratorJavaScriptAgent{
			"reviewer": {Preset: "factory"},
		},
		WorkerSettings: factory.JavaScriptWorkerSettings{
			Presets: map[string]factory.JavaScriptWorkerPreset{
				"child":   {ModelProvider: "claude", Model: "child-model", ReasoningEffort: "high"},
				"factory": {ModelProvider: "codex", Model: "factory-model", ReasoningEffort: "low"},
			},
			DefaultModelProvider: "gemini",
			DefaultModel:         "default-model",
		},
	}
	outcome, err := runtimeWorkflows.Run(context.Background(), req, factory.JavaScriptRuntimeHooks{
		NewChildExecutor: func(_ string, _ factory.JavaScriptChildRecordSink, _ workflowpolicy.EffectivePolicy) factory.JavaScriptChildExecutor {
			return childExecutorFunc(func(_ context.Context, child factory.JavaScriptChildExecutionRequest) (factory.JavaScriptChildExecutionResult, error) {
				mu.Lock()
				captured[child.Label] = child
				mu.Unlock()
				return factory.JavaScriptChildExecutionResult{Status: factory.JavaScriptChildDispatchStatusCompleted, Request: child}, nil
			})
		},
	})
	if err != nil || !outcome.OK {
		t.Fatalf("Run() outcome=%#v err=%v", outcome, err)
	}
	assertWorkerSelection(t, captured["child-preset"], "child", "claude", "child-model", "high")
	assertWorkerSelection(t, captured["factory-preset"], "factory", "codex", "factory-model", "low")
	assertWorkerSelection(t, captured["scalar-defaults"], "", "gemini", "default-model", "")
}

func TestRun_ParallelDynamicUnsupportedFieldDoesNotConsumeDispatchIdentity(t *testing.T) {
	stub := &stubChildExecutor{mode: stubChildExecutionMode}
	outcome, err := runtimeWorkflows.Run(context.Background(), factory.JavaScriptRuntimeRequest{
		Source: `
			const rejected = {prompt: "prompt-secret"};
			rejected["writable" + "Roots"] = ["value-secret"];
			return (async () => ({results: await parallel([rejected, {label: "valid", prompt: "review"}])}))();
		`,
		SessionID: "session-parallel-dynamic-unsupported-field",
	}, factory.JavaScriptRuntimeHooks{
		NewChildExecutor: func(_ string, _ factory.JavaScriptChildRecordSink, _ workflowpolicy.EffectivePolicy) factory.JavaScriptChildExecutor {
			return stub
		},
	})
	if err != nil || !outcome.OK {
		t.Fatalf("Run() outcome=%#v err=%v", outcome, err)
	}

	results, ok := projectPrimaryJSON(t, "session-parallel-dynamic-unsupported-field", outcome.Value)["results"].([]any)
	if !ok || len(results) != 2 {
		t.Fatalf("results = %#v, want two entries", results)
	}
	rejected, ok := results[0].(map[string]any)
	if !ok || rejected["status"] != factory.JavaScriptChildDispatchStatusFailed {
		t.Fatalf("rejected result = %#v, want failed child", results[0])
	}
	diagnostic, _ := rejected["diagnostic"].(string)
	if !strings.Contains(diagnostic, `agent.run() does not support field "writableRoots"`) ||
		strings.Contains(diagnostic, "value-secret") || strings.Contains(diagnostic, "prompt-secret") {
		t.Fatalf("diagnostic = %q, want field-specific redacted error", diagnostic)
	}
	assertStubChildResult(t, results[1], "valid", "stub-dispatch-1")

	requests := stub.executionRequests()
	if len(requests) != 1 || requests[0].Label != "valid" {
		t.Fatalf("executor requests = %#v, want only valid child", requests)
	}
	if requests[0].ReservedIdentity == nil || requests[0].ReservedIdentity.DispatchID != "dispatch-1" {
		t.Fatalf("reserved identity = %#v, want dispatch-1", requests[0].ReservedIdentity)
	}
	if len(outcome.Records) != 0 {
		t.Fatalf("records = %#v, want no lifecycle records from rejected item or stub executor", outcome.Records)
	}
}

func TestRun_ParallelDynamicUnsupportedFieldIsValidatedBeforeFanoutPolicy(t *testing.T) {
	stub := &stubChildExecutor{mode: stubChildExecutionMode}
	policy := workflowpolicy.DefaultEffectivePolicy()
	policy.MaxAgents = 1
	outcome, err := runtimeWorkflows.Run(context.Background(), factory.JavaScriptRuntimeRequest{
		Source: `
			const rejected = {prompt: "prompt-secret"};
			rejected["writable" + "Roots"] = ["value-secret"];
			return (async () => ({results: await parallel([rejected, {label: "valid", prompt: "review"}])}))();
		`,
		SessionID: "session-parallel-dynamic-unsupported-field-tight-fanout",
		Policy:    policy,
	}, factory.JavaScriptRuntimeHooks{
		NewChildExecutor: func(_ string, _ factory.JavaScriptChildRecordSink, _ workflowpolicy.EffectivePolicy) factory.JavaScriptChildExecutor {
			return stub
		},
	})
	if err != nil || !outcome.OK {
		t.Fatalf("Run() outcome=%#v err=%v", outcome, err)
	}

	results, ok := projectPrimaryJSON(t, "session-parallel-dynamic-unsupported-field-tight-fanout", outcome.Value)["results"].([]any)
	if !ok || len(results) != 2 {
		t.Fatalf("results = %#v, want two entries", results)
	}
	rejected, ok := results[0].(map[string]any)
	if !ok || rejected["status"] != factory.JavaScriptChildDispatchStatusFailed {
		t.Fatalf("rejected result = %#v, want failed child", results[0])
	}
	diagnostic, _ := rejected["diagnostic"].(string)
	if !strings.Contains(diagnostic, `agent.run() does not support field "writableRoots"`) {
		t.Fatalf("diagnostic = %q, want unsupported field", diagnostic)
	}
	for _, redacted := range []string{"value-secret", "prompt-secret", "maxAgents"} {
		if strings.Contains(diagnostic, redacted) {
			t.Fatalf("diagnostic = %q, must redact %q", diagnostic, redacted)
		}
	}
	assertStubChildResult(t, results[1], "valid", "stub-dispatch-1")

	requests := stub.executionRequests()
	if len(requests) != 1 {
		t.Fatalf("executor requests = %#v, want one valid child", requests)
	}
	if requests[0].Label != "valid" || requests[0].ReservedIdentity == nil || requests[0].ReservedIdentity.DispatchID != "dispatch-1" {
		t.Fatalf("executor request = %#v, want valid child with dispatch-1", requests[0])
	}
	if len(outcome.Records) != 0 {
		t.Fatalf("records = %#v, want no lifecycle records from rejected item or stub executor", outcome.Records)
	}
}

func TestRun_ParallelObjectChild_GatesResolvedPresetBeforeExecutor(t *testing.T) {
	calls := 0
	policy := policyWithWorkerAllowlists([]string{"allowed-model"}, []string{"high"})
	outcome, err := runtimeWorkflows.Run(context.Background(), factory.JavaScriptRuntimeRequest{
		Source:    `return (async () => ({results: await parallel([{label: "denied", preset: "careful", prompt: "review"}])}))();`,
		SessionID: "session-parallel-worker-policy",
		Policy:    policy,
		WorkerSettings: factory.JavaScriptWorkerSettings{Presets: map[string]factory.JavaScriptWorkerPreset{
			"careful": {ModelProvider: "codex", Model: "denied-model", ReasoningEffort: "high"},
		}},
	}, factory.JavaScriptRuntimeHooks{
		NewChildExecutor: func(_ string, _ factory.JavaScriptChildRecordSink, _ workflowpolicy.EffectivePolicy) factory.JavaScriptChildExecutor {
			return childExecutorFunc(func(_ context.Context, child factory.JavaScriptChildExecutionRequest) (factory.JavaScriptChildExecutionResult, error) {
				calls++
				return factory.JavaScriptChildExecutionResult{Status: factory.JavaScriptChildDispatchStatusCompleted, Request: child}, nil
			})
		},
	})
	if err != nil || !outcome.OK {
		t.Fatalf("Run() outcome=%#v err=%v", outcome, err)
	}
	if calls != 0 || len(outcome.Records) != 0 {
		t.Fatalf("executor calls=%d records=%#v, want no dispatch side effects", calls, outcome.Records)
	}
	projected := projectPrimaryJSON(t, "session-parallel-worker-policy", outcome.Value)
	results, ok := projected["results"].([]any)
	if !ok || len(results) != 1 {
		t.Fatalf("results = %#v, want one denied child", projected["results"])
	}
	denied, ok := results[0].(map[string]any)
	if !ok || denied["status"] != factory.JavaScriptChildDispatchStatusFailed ||
		!strings.Contains(denied["diagnostic"].(string), `policy denied: model "denied-model"`) {
		t.Fatalf("denied result = %#v", results[0])
	}
}

func assertWorkerSelection(t *testing.T, got factory.JavaScriptChildExecutionRequest, preset, provider, model, effort string) {
	t.Helper()
	if got.Preset != preset || got.ModelProvider != provider || got.Model != model || got.ReasoningEffort != effort {
		t.Fatalf("worker selection = %#v, want preset=%q provider=%q model=%q effort=%q", got, preset, provider, model, effort)
	}
}

func TestRun_ParallelFakeChildren_DeniesFanoutAboveMaxAgents(t *testing.T) {
	source := readFixture(t, "parallel-max-agents-denied.workflow.js")
	policy := workflowpolicy.DefaultEffectivePolicy()
	policy.MaxAgents = 2
	policy.Concurrency = 2

	req := factory.JavaScriptRuntimeRequest{
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
	if outcome.Failure.Code != factory.JavaScriptRuntimeCodeScriptError {
		t.Fatalf("failure code = %q, want %q", outcome.Failure.Code, factory.JavaScriptRuntimeCodeScriptError)
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

	req := factory.JavaScriptRuntimeRequest{
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
	if !ok || successChild["status"] != factory.JavaScriptChildDispatchStatusCompleted {
		t.Fatalf("results[0] = %#v, want completed child", results[0])
	}
	failedChild, ok := results[1].(map[string]any)
	if !ok {
		t.Fatalf("results[1] = %#v, want failed child object", results[1])
	}
	if failedChild["status"] != factory.JavaScriptChildDispatchStatusFailed {
		t.Fatalf("results[1].status = %#v, want FAILED", failedChild["status"])
	}
	if failedChild["diagnostic"] == "" {
		t.Fatalf("results[1].diagnostic is empty")
	}
	if failedChild["artifactRef"] != nil {
		t.Fatalf("results[1].artifactRef = %#v, want absent on failed child", failedChild["artifactRef"])
	}

	if !hasChildDispatchStatus(outcome.Records, factory.JavaScriptChildDispatchStatusFailed) {
		t.Fatal("expected FAILED child_dispatch record for simulated child failure")
	}
	if completedForLabel(outcome.Records, "child-1") {
		t.Fatal("failed child should not emit COMPLETED child_dispatch record")
	}
}

func TestRun_AgentRunPermissionsResolveCanonicalValues(t *testing.T) {
	tests := []struct {
		name       string
		source     string
		wantBypass bool
		permission factory.JavaScriptChildPermission
	}{
		{
			name:       "static default",
			source:     `return agent.run({ prompt: "review", permissions: "DEFAULT" });`,
			permission: factory.JavaScriptChildPermissionDefault,
		},
		{
			name:       "dynamic skip",
			source:     `const child = { prompt: "review", permissions: "SKIP_PERMISSIONS" }; return agent.run(child);`,
			wantBypass: true,
			permission: factory.JavaScriptChildPermissionSkipPermissions,
		},
		{
			name:       "omitted",
			source:     `return agent.run({ prompt: "review" });`,
			permission: factory.JavaScriptChildPermissionDefault,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stub := &stubChildExecutor{mode: stubChildExecutionMode}
			outcome, err := runtimeWorkflows.Run(context.Background(), factory.JavaScriptRuntimeRequest{
				Source: test.source, SourceRef: "inline", SessionID: "permissions-resolution-" + test.name,
				Policy: workflowpolicy.DefaultEffectivePolicy(),
			}, factory.JavaScriptRuntimeHooks{NewChildExecutor: func(string, factory.JavaScriptChildRecordSink, workflowpolicy.EffectivePolicy) factory.JavaScriptChildExecutor {
				return stub
			}})
			if err != nil || !outcome.OK {
				t.Fatalf("Run() outcome = %#v, error = %v", outcome, err)
			}
			requests := stub.executionRequests()
			if len(requests) != 1 {
				t.Fatalf("executor request count = %d, want 1", len(requests))
			}
			request := requests[0]
			if request.SkipPermissions != test.wantBypass || request.Permissions != test.permission {
				t.Fatalf("executor request = %#v, want permission=%q bypass=%v", request, test.permission, test.wantBypass)
			}
		})
	}
}
