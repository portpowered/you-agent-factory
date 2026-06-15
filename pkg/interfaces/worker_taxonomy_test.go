package interfaces

import "testing"

func TestWorkerTaxonomyBehaviorProjection(t *testing.T) {
	tests := []struct {
		name         string
		workerType   string
		inference    bool
		agent        bool
		script       bool
		poller       bool
		behaviorClass string
	}{
		{
			name:          "inference worker",
			workerType:    WorkerTypeInference,
			inference:     true,
			behaviorClass: WorkerTypeInference,
		},
		{
			name:          "legacy model worker projects to inference",
			workerType:    WorkerTypeModel,
			inference:     true,
			behaviorClass: WorkerTypeInference,
		},
		{
			name:          "agent worker",
			workerType:    WorkerTypeAgent,
			agent:         true,
			behaviorClass: WorkerTypeAgent,
		},
		{
			name:          "script worker",
			workerType:    WorkerTypeScript,
			script:        true,
			behaviorClass: WorkerTypeScript,
		},
		{
			name:          "poller worker",
			workerType:    WorkerTypePoller,
			poller:        true,
			behaviorClass: WorkerTypePoller,
		},
		{
			name:          "legacy hosted worker projects to poller",
			workerType:    WorkerTypeHosted,
			poller:        true,
			behaviorClass: WorkerTypePoller,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsInferenceWorkerType(tt.workerType); got != tt.inference {
				t.Fatalf("IsInferenceWorkerType(%q) = %v, want %v", tt.workerType, got, tt.inference)
			}
			if got := IsAgentWorkerType(tt.workerType); got != tt.agent {
				t.Fatalf("IsAgentWorkerType(%q) = %v, want %v", tt.workerType, got, tt.agent)
			}
			if got := IsScriptWorkerType(tt.workerType); got != tt.script {
				t.Fatalf("IsScriptWorkerType(%q) = %v, want %v", tt.workerType, got, tt.script)
			}
			if got := IsPollerWorkerType(tt.workerType); got != tt.poller {
				t.Fatalf("IsPollerWorkerType(%q) = %v, want %v", tt.workerType, got, tt.poller)
			}
			if got := ProjectWorkerBehaviorClass(tt.workerType); got != tt.behaviorClass {
				t.Fatalf("ProjectWorkerBehaviorClass(%q) = %q, want %q", tt.workerType, got, tt.behaviorClass)
			}
		})
	}
}
