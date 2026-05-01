package smoke

import (
	"fmt"
	"testing"
	"time"

	"github.com/portpowered/agent-factory/pkg/interfaces"
	"github.com/portpowered/agent-factory/pkg/testutil"
	"github.com/portpowered/agent-factory/tests/functional/internal/support"
)

func TestEndToEndDispatch_CompletesThroughServiceHarness(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "e2e"))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "E2E test"}`))

	provider := testutil.NewMockProvider(
		interfaces.InferenceResponse{Content: "E2E done. COMPLETE"},
	)
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().
		PlaceTokenCount("task:complete", 1).
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:failed")

	if provider.CallCount() != 1 {
		t.Errorf("expected provider called 1 time, got %d", provider.CallCount())
	}

	call := provider.LastCall()
	if call.Model != "test-model" {
		t.Errorf("expected model test-model, got %q", call.Model)
	}
}

func TestEndToEndDispatch_MultipleWorkItemsCompleteIndependently(t *testing.T) {
	dir := support.ScaffoldFactory(t, simpleEndToEndPipelineConfig())
	for i := 0; i < 3; i++ {
		testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
			WorkTypeID: "task",
			TraceID:    fmt.Sprintf("trace-e2e-batch-%d", i),
			Payload:    []byte(`{"title":"batch item"}`),
		})
	}

	h := testutil.NewServiceTestHarness(t, dir, testutil.WithFullWorkerPoolAndScriptWrap())

	h.RunUntilComplete(t, 15*time.Second)

	h.Assert().
		PlaceTokenCount("task:complete", 3).
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:failed")
}

func simpleEndToEndPipelineConfig() map[string]any {
	return map[string]any{
		"workTypes": []map[string]any{
			{
				"name": "task",
				"states": []map[string]string{
					{"name": "init", "type": "INITIAL"},
					{"name": "complete", "type": "TERMINAL"},
					{"name": "failed", "type": "FAILED"},
				},
			},
		},
		"workers": []map[string]string{
			{"name": "worker-a"},
		},
		"workstations": []map[string]any{
			{
				"name":      "process",
				"worker":    "worker-a",
				"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
				"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
				"onFailure": map[string]string{"workType": "task", "state": "failed"},
			},
		},
	}
}
