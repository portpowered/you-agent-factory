package javascript_test

import (
	"context"
	"strings"
	"sync"
	"testing"

	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	workflowpolicy "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/orchestratorcontract"
)

func TestPipelineCompatibilityAndVariadicCallbackArguments(t *testing.T) {
	source := `return (async function () {
  let calls = 0;
  const one = await pipeline(["alpha", "beta"], async function (item, index, extra) {
    calls = calls + 1;
    return { stage: "one", item: item, index: index, extra: extra === undefined };
  });
  const two = await pipeline(["gamma", "delta"], function (item, index, extra) {
    calls = calls + 1;
    return { stage: "one", item: item, index: index, extra: extra === undefined };
  }, async function (previous, item, index, extra) {
    calls = calls + 1;
    return { stage: "two", previous: previous.stage, item: item, index: index, extra: extra === undefined };
  });
  const three = await pipeline(["epsilon"], async function (item, index) {
    return { stage: "one", item: item, index: index };
  }, async function (previous, item, index) {
    return { stage: "two", previous: previous.stage, item: item, index: index };
  }, async function (previous, item, index) {
    return { stage: "three", previous: previous.stage, item: item, index: index };
  });
  const empty = await pipeline([], function () { calls = calls + 1; });
  return { calls: calls, one: one, two: two, three: three, empty: empty };
})();`
	outcome := runPipelineWorkflow(t, "pipeline-compatibility", source, workflowpolicy.DefaultEffectivePolicy(), nil)
	value := pipelineJSON(t, outcome)
	if value["calls"] != float64(6) {
		t.Fatalf("callback count = %#v, want 6", value["calls"])
	}
	assertPipelineItems(t, value["one"], []string{"alpha", "beta"}, 1)
	assertPipelineItems(t, value["two"], []string{"gamma", "delta"}, 2)
	assertPipelineItems(t, value["three"], []string{"epsilon"}, 3)
	if len(pipelineItems(t, value["empty"], 0)) != 0 {
		t.Fatal("empty pipeline returned items")
	}

	two := pipelineItem(t, pipelineItems(t, value["two"], 2)[1])
	second := pipelineMap(t, pipelineStage(t, pipelineStages(t, two, 2)[1], 1, factory.JavaScriptChildDispatchStatusCompleted)["result"])
	if second["previous"] != "one" || second["item"] != "delta" || second["index"] != float64(1) || second["extra"] != true {
		t.Fatalf("second-stage callback result = %#v, want prior result, original item, index, and no extra argument", second)
	}
}

func assertPipelineItems(t *testing.T, raw any, wantItems []string, wantStages int) {
	t.Helper()
	items := pipelineItems(t, raw, len(wantItems))
	for index, wantItem := range wantItems {
		item := pipelineItem(t, items[index])
		if item["index"] != float64(index) || item["item"] != wantItem || item["status"] != factory.JavaScriptChildDispatchStatusCompleted {
			t.Fatalf("pipeline item[%d] = %#v, want ordered completed item %q", index, item, wantItem)
		}
		stages := pipelineStages(t, item, wantStages)
		for stageIndex, rawStage := range stages {
			pipelineStage(t, rawStage, stageIndex, factory.JavaScriptChildDispatchStatusCompleted)
		}
	}
}

func TestPipelineStageFailuresStayWithTheirItems(t *testing.T) {
	source := `return (async function () {
  const results = await pipeline(["sync", "reject", "child", "healthy"],
    function (item, index) { return { stage: 0, item: item, index: index }; },
    function (previous, item, index) {
      if (item === "sync") throw new Error("sync failure");
      if (item === "reject") return (async function () { throw new Error("promise failure"); })();
      if (item === "child") return agent.run({ prompt: "child", label: "child-failure" });
      return { stage: 1, item: item, index: index };
    },
    function (previous, item, index) { return { stage: 2, item: item, index: index }; }
  );
  return { results: results };
})();`
	executor := &failedPipelineChildExecutor{}
	outcome := runPipelineWorkflow(t, "pipeline-failure-isolation", source, workflowpolicy.DefaultEffectivePolicy(), executor)
	items := pipelineItems(t, pipelineJSON(t, outcome)["results"], 4)
	assertFailedPipelineItem(t, items[0], "sync", "sync failure")
	assertFailedPipelineItem(t, items[1], "reject", "promise failure")
	assertFailedPipelineItem(t, items[2], "child", "child failure")
	assertCompletedPipelineItem(t, items[3], "healthy", 3)

	labels := executor.labelsSnapshot()
	if len(labels) != 1 || labels[0] != "child-failure" {
		t.Fatalf("child labels = %#v, want only failed item's stage dispatch", labels)
	}
}

func assertFailedPipelineItem(t *testing.T, raw any, wantItem, diagnostic string) {
	t.Helper()
	item := pipelineItem(t, raw)
	if item["item"] != wantItem || item["status"] != factory.JavaScriptChildDispatchStatusFailed {
		t.Fatalf("failed pipeline item = %#v, want item=%q FAILED", item, wantItem)
	}
	stages := pipelineStages(t, item, 2)
	pipelineStage(t, stages[0], 0, factory.JavaScriptChildDispatchStatusCompleted)
	failed := pipelineStage(t, stages[1], 1, factory.JavaScriptChildDispatchStatusFailed)
	if !strings.Contains(failed["diagnostic"].(string), diagnostic) {
		t.Fatalf("failed stage diagnostic = %#v, want %q", failed["diagnostic"], diagnostic)
	}
}

func assertCompletedPipelineItem(t *testing.T, raw any, wantItem string, wantStages int) {
	t.Helper()
	item := pipelineItem(t, raw)
	if item["item"] != wantItem || item["status"] != factory.JavaScriptChildDispatchStatusCompleted {
		t.Fatalf("completed pipeline item = %#v, want item=%q COMPLETED", item, wantItem)
	}
	for index, stage := range pipelineStages(t, item, wantStages) {
		pipelineStage(t, stage, index, factory.JavaScriptChildDispatchStatusCompleted)
	}
}

type failedPipelineChildExecutor struct {
	mu     sync.Mutex
	labels []string
}

func (e *failedPipelineChildExecutor) bindSink(factory.JavaScriptChildRecordSink) {}

func (e *failedPipelineChildExecutor) Execute(_ context.Context, req factory.JavaScriptChildExecutionRequest) (factory.JavaScriptChildExecutionResult, error) {
	e.mu.Lock()
	e.labels = append(e.labels, req.Label)
	e.mu.Unlock()
	return factory.JavaScriptChildExecutionResult{
		Status:     factory.JavaScriptChildDispatchStatusFailed,
		Diagnostic: "child failure",
		Request:    req,
	}, nil
}

func (e *failedPipelineChildExecutor) labelsSnapshot() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]string(nil), e.labels...)
}
