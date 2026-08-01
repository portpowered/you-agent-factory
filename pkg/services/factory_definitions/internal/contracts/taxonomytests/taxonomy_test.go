package taxonomytests

import (
	"strings"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/contracts"
)

func TestWorkerTaxonomyBehaviorProjection(t *testing.T) {
	tests := []struct {
		name          string
		workerType    string
		inference     bool
		agent         bool
		script        bool
		poller        bool
		behaviorClass string
	}{
		{
			name:          "inference worker",
			workerType:    interfaces.WorkerTypeInference,
			inference:     true,
			behaviorClass: interfaces.WorkerTypeInference,
		},
		{
			name:          "legacy model worker projects to inference",
			workerType:    interfaces.WorkerTypeModel,
			inference:     true,
			behaviorClass: interfaces.WorkerTypeInference,
		},
		{
			name:          "agent worker",
			workerType:    interfaces.WorkerTypeAgent,
			agent:         true,
			behaviorClass: interfaces.WorkerTypeAgent,
		},
		{
			name:          "script worker",
			workerType:    interfaces.WorkerTypeScript,
			script:        true,
			behaviorClass: interfaces.WorkerTypeScript,
		},
		{
			name:          "poller worker",
			workerType:    interfaces.WorkerTypePoller,
			poller:        true,
			behaviorClass: interfaces.WorkerTypePoller,
		},
		{
			name:          "legacy hosted worker projects to poller",
			workerType:    interfaces.WorkerTypeHosted,
			poller:        true,
			behaviorClass: interfaces.WorkerTypePoller,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := interfaces.IsInferenceWorkerType(tt.workerType); got != tt.inference {
				t.Fatalf("IsInferenceWorkerType(%q) = %v, want %v", tt.workerType, got, tt.inference)
			}
			if got := interfaces.IsAgentWorkerType(tt.workerType); got != tt.agent {
				t.Fatalf("IsAgentWorkerType(%q) = %v, want %v", tt.workerType, got, tt.agent)
			}
			if got := interfaces.IsProviderBackedWorkerType(tt.workerType); got != (tt.inference || tt.agent) {
				t.Fatalf("IsProviderBackedWorkerType(%q) = %v, want %v", tt.workerType, got, tt.inference || tt.agent)
			}
			wantLease := (tt.inference || tt.agent)
			if got := interfaces.UsesModelhostLease(tt.workerType, interfaces.ModelLocalityLocal); got != wantLease {
				t.Fatalf("UsesModelhostLease(%q, LOCAL) = %v, want %v", tt.workerType, got, wantLease)
			}
			if got := interfaces.UsesModelhostLease(tt.workerType, interfaces.ModelLocalityCloud); got {
				t.Fatalf("UsesModelhostLease(%q, CLOUD) = true, want false", tt.workerType)
			}
			if got := interfaces.IsScriptWorkerType(tt.workerType); got != tt.script {
				t.Fatalf("IsScriptWorkerType(%q) = %v, want %v", tt.workerType, got, tt.script)
			}
			if got := interfaces.IsPollerWorkerType(tt.workerType); got != tt.poller {
				t.Fatalf("IsPollerWorkerType(%q) = %v, want %v", tt.workerType, got, tt.poller)
			}
			if got := interfaces.ProjectWorkerBehaviorClass(tt.workerType); got != tt.behaviorClass {
				t.Fatalf("ProjectWorkerBehaviorClass(%q) = %q, want %q", tt.workerType, got, tt.behaviorClass)
			}
		})
	}
}

func TestWorkstationTaxonomyBehaviorProjection(t *testing.T) {
	tests := []struct {
		name            string
		workstationType string
		kind            interfaces.WorkstationKind
		inference       bool
		agent           bool
		script          bool
		poller          bool
		behaviorClass   string
	}{
		{
			name:            "inference run",
			workstationType: interfaces.WorkstationTypeInference,
			inference:       true,
			behaviorClass:   interfaces.WorkstationTypeInference,
		},
		{
			name:            "legacy model invoke projects to inference",
			workstationType: interfaces.WorkstationTypeInvoke,
			inference:       true,
			behaviorClass:   interfaces.WorkstationTypeInference,
		},
		{
			name:            "agent run",
			workstationType: interfaces.WorkstationTypeAgent,
			agent:           true,
			behaviorClass:   interfaces.WorkstationTypeAgent,
		},
		{
			name:            "legacy model workstation projects to agent",
			workstationType: interfaces.WorkstationTypeModel,
			agent:           true,
			behaviorClass:   interfaces.WorkstationTypeAgent,
		},
		{
			name:            "script run",
			workstationType: interfaces.WorkstationTypeScript,
			script:          true,
			behaviorClass:   interfaces.WorkstationTypeScript,
		},
		{
			name:            "poller run",
			workstationType: interfaces.WorkstationTypePoller,
			poller:          true,
			behaviorClass:   interfaces.WorkstationTypePoller,
		},
		{
			name:            "legacy poller kind without explicit type",
			workstationType: "",
			kind:            interfaces.WorkstationKindPoller,
			poller:          true,
			behaviorClass:   interfaces.WorkstationTypePoller,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := interfaces.IsInferenceRunWorkstationType(tt.workstationType); got != tt.inference {
				t.Fatalf("IsInferenceRunWorkstationType(%q) = %v, want %v", tt.workstationType, got, tt.inference)
			}
			if got := interfaces.IsAgentRunWorkstationType(tt.workstationType); got != tt.agent {
				t.Fatalf("IsAgentRunWorkstationType(%q) = %v, want %v", tt.workstationType, got, tt.agent)
			}
			if got := interfaces.IsScriptRunWorkstationType(tt.workstationType); got != tt.script {
				t.Fatalf("IsScriptRunWorkstationType(%q) = %v, want %v", tt.workstationType, got, tt.script)
			}
			if got := interfaces.IsPollerRunWorkstationType(tt.workstationType, tt.kind); got != tt.poller {
				t.Fatalf("IsPollerRunWorkstationType(%q, %q) = %v, want %v", tt.workstationType, tt.kind, got, tt.poller)
			}
			if got := interfaces.ProjectWorkstationBehaviorClass(tt.workstationType, tt.kind); got != tt.behaviorClass {
				t.Fatalf("ProjectWorkstationBehaviorClass(%q, %q) = %q, want %q", tt.workstationType, tt.kind, got, tt.behaviorClass)
			}
		})
	}
}

func TestCompatibleWorkerWorkstationBehavior(t *testing.T) {
	for _, tt := range compatibleWorkerWorkstationBehaviorCases() {
		t.Run(tt.name, func(t *testing.T) {
			got := interfaces.CompatibleWorkerWorkstationBehavior(
				tt.workerType, tt.workstationType, tt.kind,
			)
			if got != tt.wantCompatible {
				t.Fatalf("CompatibleWorkerWorkstationBehavior(%q, %q, %q) = %v, want %v",
					tt.workerType, tt.workstationType, tt.kind, got, tt.wantCompatible)
			}
		})
	}
}

type workerWorkstationBehaviorCase struct {
	name            string
	workerType      string
	workstationType string
	kind            interfaces.WorkstationKind
	wantCompatible  bool
}

func compatibleWorkerWorkstationBehaviorCases() []workerWorkstationBehaviorCase {
	cases := compatibleWorkerWorkstationBehaviorCompatibleCases()
	cases = append(cases, compatibleWorkerWorkstationBehaviorMismatchCases()...)
	return cases
}

func compatibleWorkerWorkstationBehaviorCompatibleCases() []workerWorkstationBehaviorCase {
	return []workerWorkstationBehaviorCase{
		{
			name:            "inference run with inference worker",
			workerType:      interfaces.WorkerTypeInference,
			workstationType: interfaces.WorkstationTypeInference,
			wantCompatible:  true,
		},
		{
			name:            "legacy model invoke with model worker",
			workerType:      interfaces.WorkerTypeModel,
			workstationType: interfaces.WorkstationTypeInvoke,
			wantCompatible:  true,
		},
		{
			name:            "agent run with agent worker",
			workerType:      interfaces.WorkerTypeAgent,
			workstationType: interfaces.WorkstationTypeAgent,
			wantCompatible:  true,
		},
		{
			name:            "legacy model workstation with model worker",
			workerType:      interfaces.WorkerTypeModel,
			workstationType: interfaces.WorkstationTypeModel,
			wantCompatible:  true,
		},
		{
			name:            "legacy model workstation with script worker",
			workerType:      interfaces.WorkerTypeScript,
			workstationType: interfaces.WorkstationTypeModel,
			wantCompatible:  true,
		},
		{
			name:            "script run with script worker",
			workerType:      interfaces.WorkerTypeScript,
			workstationType: interfaces.WorkstationTypeScript,
			wantCompatible:  true,
		},
		{
			name:            "poller run with poller worker",
			workerType:      interfaces.WorkerTypePoller,
			workstationType: interfaces.WorkstationTypePoller,
			wantCompatible:  true,
		},
		{
			name:            "legacy poller kind with hosted worker",
			workerType:      interfaces.WorkerTypeHosted,
			workstationType: "",
			kind:            interfaces.WorkstationKindPoller,
			wantCompatible:  true,
		},
		{
			name:            "legacy poller kind with script worker",
			workerType:      interfaces.WorkerTypeScript,
			workstationType: "",
			kind:            interfaces.WorkstationKindPoller,
			wantCompatible:  true,
		},
		{
			name:            "legacy default workstation with inference worker",
			workerType:      interfaces.WorkerTypeInference,
			workstationType: "",
			kind:            interfaces.WorkstationKindStandard,
			wantCompatible:  true,
		},
		{
			name:            "legacy default workstation with script worker",
			workerType:      interfaces.WorkerTypeScript,
			workstationType: "",
			kind:            interfaces.WorkstationKindStandard,
			wantCompatible:  true,
		},
	}
}

func compatibleWorkerWorkstationBehaviorMismatchCases() []workerWorkstationBehaviorCase {
	return []workerWorkstationBehaviorCase{
		{
			name:            "agent run with inference worker",
			workerType:      interfaces.WorkerTypeInference,
			workstationType: interfaces.WorkstationTypeAgent,
			wantCompatible:  false,
		},
		{
			name:            "inference run with agent worker",
			workerType:      interfaces.WorkerTypeAgent,
			workstationType: interfaces.WorkstationTypeInference,
			wantCompatible:  false,
		},
		{
			name:            "poller run with inference worker",
			workerType:      interfaces.WorkerTypeInference,
			workstationType: interfaces.WorkstationTypePoller,
			wantCompatible:  false,
		},
	}
}

func TestWorkerWorkstationBehaviorMismatchMessage_UsesBehaviorTerminology(t *testing.T) {
	msg := interfaces.WorkerWorkstationBehaviorMismatchMessage(
		"plan-task",
		interfaces.WorkstationTypeAgent,
		interfaces.WorkstationKindStandard,
		"executor",
		interfaces.WorkerTypeInference,
	)
	for _, term := range []string{"AGENT_RUN", "INFERENCE_WORKER", "agent-run", "inference worker"} {
		if !strings.Contains(msg, term) {
			t.Fatalf("message %q missing %q", msg, term)
		}
	}
}
