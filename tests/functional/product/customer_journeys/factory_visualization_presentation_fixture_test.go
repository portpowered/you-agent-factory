package customer_journeys_test

import (
	"sync/atomic"

	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
)

type visualizationEffectTracker struct {
	presentCount atomic.Int32
}

func (tracker *visualizationEffectTracker) PresentFactoryView(factoryvisualization.View) {
	tracker.presentCount.Add(1)
}

func (tracker *visualizationEffectTracker) presentations() int {
	return int(tracker.presentCount.Load())
}

func visualizationInertFactoryConfig() map[string]any {
	return map[string]any{
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]string{{"name": "worker-a"}},
		"workstations": []map[string]any{{
			"name":      "process",
			"worker":    "worker-a",
			"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
			"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
			"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
		}},
	}
}
