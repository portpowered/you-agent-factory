package runtime_api_test

import (
	"context"
	"sync"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func twoStagePipelineConfig() map[string]any {
	return map[string]any{
		"name": "factory",
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "stage1", "type": "PROCESSING"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]string{{"name": "worker-a"}, {"name": "worker-b"}},
		"workstations": []map[string]any{
			{
				"name":      "worker-a",
				"worker":    "worker-a",
				"behavior":  "STANDARD",
				"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
				"outputs":   []map[string]string{{"workType": "task", "state": "stage1"}},
				"onFailure": map[string]string{"workType": "task", "state": "failed"},
			},
			{
				"name":      "worker-b",
				"worker":    "worker-b",
				"behavior":  "STANDARD",
				"inputs":    []map[string]string{{"workType": "task", "state": "stage1"}},
				"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
				"onFailure": map[string]string{"workType": "task", "state": "failed"},
			},
		},
	}
}

type blockingExecutor struct {
	releaseCh <-chan struct{}
	mu        *sync.Mutex
	calls     *int
}

func (e *blockingExecutor) Execute(_ context.Context, d interfaces.WorkDispatch) (interfaces.WorkResult, error) {
	e.mu.Lock()
	*e.calls++
	e.mu.Unlock()

	<-e.releaseCh

	return interfaces.WorkResult{
		DispatchID:   d.DispatchID,
		TransitionID: d.TransitionID,
		Outcome:      interfaces.OutcomeAccepted,
	}, nil
}
