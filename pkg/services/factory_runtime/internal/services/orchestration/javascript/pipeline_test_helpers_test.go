package javascript_test

import (
	"context"
	"encoding/json"
	"testing"

	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	workflowpolicy "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/orchestratorcontract"
	runtimekit "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/testkit"
)

var pipelineWorkflows = runtimekit.JavaScriptWorkflows()

type pipelineRunResult struct {
	outcome factory.JavaScriptRuntimeOutcome
	err     error
}

func runPipelineWorkflow(
	t *testing.T,
	sessionID, source string,
	policy workflowpolicy.EffectivePolicy,
	executor factory.JavaScriptChildExecutor,
) factory.JavaScriptRuntimeOutcome {
	t.Helper()
	req := factory.JavaScriptRuntimeRequest{
		Source:    source,
		SourceRef: sessionID + ".workflow.js",
		SessionID: sessionID,
		Policy:    policy,
	}
	hooks := factory.JavaScriptRuntimeHooks{}
	if executor != nil {
		hooks.NewChildExecutor = func(_ string, sink factory.JavaScriptChildRecordSink, _ workflowpolicy.EffectivePolicy) factory.JavaScriptChildExecutor {
			if bindable, ok := executor.(interface {
				bindSink(factory.JavaScriptChildRecordSink)
			}); ok {
				bindable.bindSink(sink)
			}
			return executor
		}
	}
	outcome, err := pipelineWorkflows.Run(context.Background(), req, hooks)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !outcome.OK {
		t.Fatalf("Run() failure = %#v", outcome.Failure)
	}
	return outcome
}

func startPipelineWorkflow(
	sessionID, source string,
	policy workflowpolicy.EffectivePolicy,
	executor factory.JavaScriptChildExecutor,
) <-chan pipelineRunResult {
	req := factory.JavaScriptRuntimeRequest{
		Source:    source,
		SourceRef: sessionID + ".workflow.js",
		SessionID: sessionID,
		Policy:    policy,
	}
	hooks := factory.JavaScriptRuntimeHooks{
		NewChildExecutor: func(_ string, sink factory.JavaScriptChildRecordSink, _ workflowpolicy.EffectivePolicy) factory.JavaScriptChildExecutor {
			if bindable, ok := executor.(interface {
				bindSink(factory.JavaScriptChildRecordSink)
			}); ok {
				bindable.bindSink(sink)
			}
			return executor
		},
	}
	done := make(chan pipelineRunResult, 1)
	go func() {
		outcome, err := pipelineWorkflows.Run(context.Background(), req, hooks)
		done <- pipelineRunResult{outcome: outcome, err: err}
	}()
	return done
}

func pipelineJSON(t *testing.T, outcome factory.JavaScriptRuntimeOutcome) map[string]any {
	t.Helper()
	var value map[string]any
	if err := json.Unmarshal(outcome.Value.JSON, &value); err != nil {
		t.Fatalf("decode pipeline result: %v", err)
	}
	return value
}

func pipelineItems(t *testing.T, raw any, want int) []any {
	t.Helper()
	items, ok := raw.([]any)
	if !ok || len(items) != want {
		t.Fatalf("pipeline items = %#v, want %d entries", raw, want)
	}
	return items
}

func pipelineItem(t *testing.T, raw any) map[string]any {
	t.Helper()
	item, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("pipeline item = %#v, want object", raw)
	}
	return item
}

func pipelineStages(t *testing.T, item map[string]any, want int) []any {
	t.Helper()
	stages, ok := item["stages"].([]any)
	if !ok || len(stages) != want {
		t.Fatalf("pipeline stages = %#v, want %d entries", item["stages"], want)
	}
	return stages
}

func pipelineStage(t *testing.T, raw any, index int, status string) map[string]any {
	t.Helper()
	stage, ok := raw.(map[string]any)
	if !ok || stage["index"] != float64(index) || stage["status"] != status {
		t.Fatalf("pipeline stage = %#v, want index=%d status=%q", raw, index, status)
	}
	return stage
}

func pipelineMap(t *testing.T, raw any) map[string]any {
	t.Helper()
	value, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("pipeline callback result = %#v, want object", raw)
	}
	return value
}
