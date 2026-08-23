package javascript_test

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	workflowpolicy "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/orchestratorcontract"
)

func TestPipelineNestedInvocationKeepsEachExecutionContextSafe(t *testing.T) {
	source := `return (async function () {
  const results = await pipeline(["alpha", "beta"], function (item, index) {
    return pipeline([item], function (nestedItem) {
      return agent.run({ prompt: nestedItem, label: "inner-" + index });
    });
  }, function (previous, item, index) {
    return agent.run({ prompt: previous[0].stages[0].result.label, label: "outer-" + index });
  });
  return { results: results, after: await agent.run({ prompt: "after", label: "after-pipeline" }) };
})();`
	executor := newControlledPipelineChildExecutor(nil)
	done := startPipelineWorkflow("pipeline-nested-context", source, workflowpolicy.DefaultEffectivePolicy(), executor)
	executor.waitForStarted(t, "inner-0", "inner-1")
	executor.releaseAll()
	executor.waitForStarted(t, "outer-0", "outer-1")
	executor.releaseAll()
	executor.waitForStarted(t, "after-pipeline")
	executor.releaseLabel("after-pipeline")
	completed := <-done
	if completed.err != nil || !completed.outcome.OK {
		t.Fatalf("nested pipeline outcome=%#v err=%v", completed.outcome, completed.err)
	}
	assertCompletedPipelineResults(t, pipelineJSON(t, completed.outcome)["results"], 2, 2)
}

func TestPipelineParallelStageReleasesVMForSiblingItems(t *testing.T) {
	source := `return (async function () {
  const results = await pipeline(["alpha", "beta"], function (item, index) {
    return parallel([{ prompt: "parallel-" + item, label: "parallel-" + index }]);
  }, function (previous, item, index) {
    return agent.run({ prompt: item, label: "stage2-" + index });
  });
  return { results: results };
})();`
	executor := newControlledPipelineChildExecutor(nil)
	done := startPipelineWorkflow("pipeline-nested-parallel", source, workflowpolicy.DefaultEffectivePolicy(), executor)
	executor.waitForStarted(t, "parallel-0", "parallel-1")
	executor.releaseLabel("parallel-0")
	executor.waitForStarted(t, "stage2-0")
	if executor.hasEnded("parallel-1") {
		t.Fatal("parallel-1 completed before sibling stage2-0 ran")
	}
	executor.releaseLabel("parallel-1")
	executor.waitForStarted(t, "stage2-1")
	executor.releaseLabel("stage2-0")
	executor.releaseLabel("stage2-1")
	completed := <-done
	if completed.err != nil || !completed.outcome.OK {
		t.Fatalf("nested parallel outcome=%#v err=%v", completed.outcome, completed.err)
	}
	assertCompletedPipelineResults(t, pipelineJSON(t, completed.outcome)["results"], 2, 2)
}

func TestPipelineChildrenRespectEffectiveConcurrency(t *testing.T) {
	source := `return (async function () {
  const results = await pipeline(["a", "b", "c", "d"], function (item, index) {
    return agent.run({ prompt: item, label: "child-" + index });
  });
  return { results: results };
})();`
	policy := workflowpolicy.DefaultEffectivePolicy()
	policy.Concurrency = 2
	executor := newControlledPipelineChildExecutor(nil)
	done := startPipelineWorkflow("pipeline-concurrency-limit", source, policy, executor)
	executor.waitForCount(t, 2)
	if got := executor.activeCount(); got != 2 {
		t.Fatalf("active pipeline children = %d, want exactly %d before release", got, policy.Concurrency)
	}
	for index := 0; index < 4; index++ {
		executor.releaseLabel(fmt.Sprintf("child-%d", index))
	}
	completed := <-done
	if completed.err != nil || !completed.outcome.OK {
		t.Fatalf("concurrency-limited outcome=%#v err=%v", completed.outcome, completed.err)
	}
	if got := executor.peakCount(); got > policy.Concurrency {
		t.Fatalf("peak pipeline children = %d, want <= %d", got, policy.Concurrency)
	}
}

func TestPipelineDispatchTimestampsFollowSlowestItemChain(t *testing.T) {
	labels := []string{"stage1-0", "stage1-1", "stage2-0", "stage2-1", "stage3-0", "stage3-1"}
	durations := map[string]time.Duration{
		"stage1-0": 80 * time.Millisecond, "stage1-1": 100 * time.Millisecond,
		"stage2-0": 10 * time.Millisecond, "stage2-1": 80 * time.Millisecond,
		"stage3-0": 80 * time.Millisecond, "stage3-1": 10 * time.Millisecond,
	}
	source := `return (async function () {
  const results = await pipeline(["fast", "slow"], function (item, index) {
    return agent.run({ prompt: item, label: "stage1-" + index });
  }, function (previous, item, index) {
    return agent.run({ prompt: previous.label, label: "stage2-" + index });
  }, function (previous, item, index) {
    return agent.run({ prompt: previous.label, label: "stage3-" + index });
  });
  return { results: results };
})();`
	executor := newControlledPipelineChildExecutor(durations)
	done := startPipelineWorkflow("pipeline-timing-evidence", source, workflowpolicy.DefaultEffectivePolicy(), executor)
	executor.waitForStarted(t, labels[0], labels[1])
	executor.releaseLabel(labels[0])
	executor.waitForStarted(t, labels[2])
	executor.releaseLabel(labels[2])
	executor.waitForStarted(t, labels[4])
	if executor.hasEnded(labels[1]) {
		t.Fatal("slow first-stage child completed before fast item reached stage three")
	}
	executor.releaseLabel(labels[4])
	executor.releaseLabel(labels[1])
	executor.waitForStarted(t, labels[3])
	executor.releaseLabel(labels[3])
	executor.waitForStarted(t, labels[5])
	executor.releaseLabel(labels[5])
	completed := <-done
	if completed.err != nil || !completed.outcome.OK {
		t.Fatalf("timed pipeline outcome=%#v err=%v", completed.outcome, completed.err)
	}
	assertCompletedPipelineResults(t, pipelineJSON(t, completed.outcome)["results"], 2, 3)

	timings := executor.snapshotTimings()
	observed := lastDispatchEnd(timings) - firstDispatchStart(timings)
	barrier := stageMaximum(timings, 1) + stageMaximum(timings, 2) + stageMaximum(timings, 3)
	slowestChain := chainDuration(timings, 1) // item zero
	if other := chainDuration(timings, 2); other > slowestChain {
		slowestChain = other
	}
	t.Logf("pipeline timing evidence: observed=%s slowest-chain=%s barrier-sum-of-stage-maxima=%s", observed, slowestChain, barrier)
	if observed >= barrier || observed > slowestChain+100*time.Millisecond {
		t.Fatalf("timing evidence observed=%s slowest-chain=%s barrier=%s", observed, slowestChain, barrier)
	}
}

func assertCompletedPipelineResults(t *testing.T, raw any, items, stages int) {
	t.Helper()
	for index, rawItem := range pipelineItems(t, raw, items) {
		item := pipelineItem(t, rawItem)
		if item["index"] != float64(index) || item["status"] != factory.JavaScriptChildDispatchStatusCompleted {
			t.Fatalf("pipeline result[%d] = %#v, want completed item", index, item)
		}
		for stageIndex, rawStage := range pipelineStages(t, item, stages) {
			pipelineStage(t, rawStage, stageIndex, factory.JavaScriptChildDispatchStatusCompleted)
		}
	}
}

type controlledPipelineChildExecutor struct {
	sink       factory.JavaScriptChildRecordSink
	started    chan string
	mu         sync.Mutex
	release    map[string]chan struct{}
	released   map[string]bool
	timestamps map[string]pipelineTiming
	durations  map[string]time.Duration
	active     int
	peak       int
}

type pipelineTiming struct {
	start, end time.Duration
	completed  bool
}

func newControlledPipelineChildExecutor(durations map[string]time.Duration) *controlledPipelineChildExecutor {
	return &controlledPipelineChildExecutor{
		started:    make(chan string, 32),
		release:    make(map[string]chan struct{}),
		released:   make(map[string]bool),
		timestamps: make(map[string]pipelineTiming),
		durations:  durations,
	}
}

func (e *controlledPipelineChildExecutor) bindSink(sink factory.JavaScriptChildRecordSink) {
	e.mu.Lock()
	e.sink = sink
	e.mu.Unlock()
}

func (e *controlledPipelineChildExecutor) Execute(ctx context.Context, req factory.JavaScriptChildExecutionRequest) (factory.JavaScriptChildExecutionResult, error) {
	dispatchID, childIndex := e.sink.NextChildDispatchIdentity()
	base := factory.JavaScriptChildDispatchRecord{DispatchID: dispatchID, ChildIndex: childIndex, Label: req.Label, ExecutionMode: factory.JavaScriptChildExecutionModeFake}
	e.sink.AppendChildDispatch(base, factory.JavaScriptChildDispatchStatusQueued)
	e.sink.AppendChildDispatch(base, factory.JavaScriptChildDispatchStatusRunning)
	e.mu.Lock()
	release := e.releaseChannelLocked(req.Label)
	e.active++
	if e.active > e.peak {
		e.peak = e.active
	}
	e.timestamps[req.Label] = pipelineTiming{start: e.logicalStartLocked(req.Label), end: e.logicalEndLocked(req.Label)}
	e.mu.Unlock()
	e.started <- req.Label
	select {
	case <-release:
	case <-ctx.Done():
		e.finish(req.Label)
		return factory.JavaScriptChildExecutionResult{}, ctx.Err()
	}
	e.finish(req.Label)
	completed := base
	completed.Status = factory.JavaScriptChildDispatchStatusCompleted
	completed.Output = map[string]any{"label": req.Label}
	e.sink.Append(factory.JavaScriptRuntimeRecord{Kind: factory.JavaScriptRecordKindChildDispatch, ChildDispatch: &completed})
	return factory.JavaScriptChildExecutionResult{DispatchID: dispatchID, ChildIndex: childIndex, Status: factory.JavaScriptChildDispatchStatusCompleted, ExecutionMode: factory.JavaScriptChildExecutionModeFake, Output: completed.Output, Request: req}, nil
}

func (e *controlledPipelineChildExecutor) releaseChannelLocked(label string) chan struct{} {
	if channel, ok := e.release[label]; ok {
		return channel
	}
	channel := make(chan struct{})
	e.release[label] = channel
	return channel
}

func (e *controlledPipelineChildExecutor) logicalStartLocked(label string) time.Duration {
	previous := previousPipelineStage(label)
	if previous == "" {
		return 0
	}
	return e.timestamps[previous].end
}

func (e *controlledPipelineChildExecutor) logicalEndLocked(label string) time.Duration {
	duration := e.durations[label]
	if duration == 0 {
		duration = time.Millisecond
	}
	return e.logicalStartLocked(label) + duration
}

func previousPipelineStage(label string) string {
	parts := strings.Split(label, "-")
	if len(parts) != 2 {
		return ""
	}
	stage, err := strconv.Atoi(strings.TrimPrefix(parts[0], "stage"))
	if err != nil || stage <= 1 {
		return ""
	}
	return fmt.Sprintf("stage%d-%s", stage-1, parts[1])
}

func (e *controlledPipelineChildExecutor) finish(label string) {
	e.mu.Lock()
	e.active--
	timing := e.timestamps[label]
	timing.completed = true
	e.timestamps[label] = timing
	e.mu.Unlock()
}

func (e *controlledPipelineChildExecutor) waitForStarted(t *testing.T, want ...string) {
	t.Helper()
	wanted := make(map[string]bool, len(want))
	for _, label := range want {
		wanted[label] = true
	}
	for len(wanted) > 0 {
		label := <-e.started
		if !wanted[label] {
			t.Fatalf("unexpected child start %q while waiting for %#v", label, want)
		}
		delete(wanted, label)
	}
}

func (e *controlledPipelineChildExecutor) waitForCount(t *testing.T, count int) {
	t.Helper()
	for index := 0; index < count; index++ {
		if label := <-e.started; label == "" {
			t.Fatal("child start label is empty")
		}
	}
}

func (e *controlledPipelineChildExecutor) releaseLabel(label string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	channel := e.releaseChannelLocked(label)
	if !e.released[label] {
		e.released[label] = true
		close(channel)
	}
}

func (e *controlledPipelineChildExecutor) releaseAll() {
	e.mu.Lock()
	labels := make([]string, 0, len(e.release))
	for label := range e.release {
		labels = append(labels, label)
	}
	e.mu.Unlock()
	for _, label := range labels {
		e.releaseLabel(label)
	}
}

func (e *controlledPipelineChildExecutor) hasEnded(label string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.timestamps[label].completed
}

func (e *controlledPipelineChildExecutor) activeCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.active
}

func (e *controlledPipelineChildExecutor) peakCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.peak
}

func (e *controlledPipelineChildExecutor) snapshotTimings() map[string]pipelineTiming {
	e.mu.Lock()
	defer e.mu.Unlock()
	copyOf := make(map[string]pipelineTiming, len(e.timestamps))
	for label, timing := range e.timestamps {
		copyOf[label] = timing
	}
	return copyOf
}

func firstDispatchStart(timings map[string]pipelineTiming) time.Duration {
	start := time.Duration(0)
	for _, timing := range timings {
		if timing.start < start || start == 0 {
			start = timing.start
		}
	}
	return start
}

func lastDispatchEnd(timings map[string]pipelineTiming) time.Duration {
	end := time.Duration(0)
	for _, timing := range timings {
		if timing.end > end {
			end = timing.end
		}
	}
	return end
}

func stageMaximum(timings map[string]pipelineTiming, stage int) time.Duration {
	maximum := time.Duration(0)
	for label, timing := range timings {
		parts := strings.Split(label, "-")
		if len(parts) != 2 || !strings.HasPrefix(parts[0], "stage") {
			continue
		}
		current, _ := strconv.Atoi(strings.TrimPrefix(parts[0], "stage"))
		if current == stage && timing.end-timing.start > maximum {
			maximum = timing.end - timing.start
		}
	}
	return maximum
}

func chainDuration(timings map[string]pipelineTiming, item int) time.Duration {
	total := time.Duration(0)
	for stage := 1; stage <= 3; stage++ {
		timing := timings[fmt.Sprintf("stage%d-%d", stage, item)]
		total += timing.end - timing.start
	}
	return total
}
