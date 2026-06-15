package interfaces

import "testing"

func TestWorkstationTaxonomyBehaviorProjection(t *testing.T) {
	tests := []struct {
		name          string
		workstationType string
		kind          WorkstationKind
		inference     bool
		agent         bool
		script        bool
		poller        bool
		behaviorClass string
	}{
		{
			name:          "inference run",
			workstationType: WorkstationTypeInference,
			inference:     true,
			behaviorClass: WorkstationTypeInference,
		},
		{
			name:          "legacy model invoke projects to inference",
			workstationType: WorkstationTypeInvoke,
			inference:     true,
			behaviorClass: WorkstationTypeInference,
		},
		{
			name:          "agent run",
			workstationType: WorkstationTypeAgent,
			agent:         true,
			behaviorClass: WorkstationTypeAgent,
		},
		{
			name:          "legacy model workstation projects to agent",
			workstationType: WorkstationTypeModel,
			agent:         true,
			behaviorClass: WorkstationTypeAgent,
		},
		{
			name:          "script run",
			workstationType: WorkstationTypeScript,
			script:        true,
			behaviorClass: WorkstationTypeScript,
		},
		{
			name:          "poller run",
			workstationType: WorkstationTypePoller,
			poller:        true,
			behaviorClass: WorkstationTypePoller,
		},
		{
			name:          "legacy poller kind without explicit type",
			workstationType: "",
			kind:          WorkstationKindPoller,
			poller:        true,
			behaviorClass: WorkstationTypePoller,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsInferenceRunWorkstationType(tt.workstationType); got != tt.inference {
				t.Fatalf("IsInferenceRunWorkstationType(%q) = %v, want %v", tt.workstationType, got, tt.inference)
			}
			if got := IsAgentRunWorkstationType(tt.workstationType); got != tt.agent {
				t.Fatalf("IsAgentRunWorkstationType(%q) = %v, want %v", tt.workstationType, got, tt.agent)
			}
			if got := IsScriptRunWorkstationType(tt.workstationType); got != tt.script {
				t.Fatalf("IsScriptRunWorkstationType(%q) = %v, want %v", tt.workstationType, got, tt.script)
			}
			if got := IsPollerRunWorkstationType(tt.workstationType, tt.kind); got != tt.poller {
				t.Fatalf("IsPollerRunWorkstationType(%q, %q) = %v, want %v", tt.workstationType, tt.kind, got, tt.poller)
			}
			if got := ProjectWorkstationBehaviorClass(tt.workstationType, tt.kind); got != tt.behaviorClass {
				t.Fatalf("ProjectWorkstationBehaviorClass(%q, %q) = %q, want %q", tt.workstationType, tt.kind, got, tt.behaviorClass)
			}
		})
	}
}
